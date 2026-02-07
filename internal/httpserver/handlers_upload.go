package httpserver

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/justinpopa/duh/internal/db"
)

func (s *Server) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 8 << 30 // 8 GB
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		http.Error(w, "Upload too large or failed to parse form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	description := r.FormValue("description")
	bootType := r.FormValue("boot_type")
	if bootType == "" {
		bootType = db.BootTypeLinux
	}
	cmdline := r.FormValue("cmdline")
	ipxeScript := r.FormValue("ipxe_script")

	// Collect uploaded filenames for metadata
	var fileNames []string
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		for _, headers := range r.MultipartForm.File["files"] {
			fileNames = append(fileNames, headers.Filename)
		}
	}

	id, err := db.CreateImage(s.DB, name, description, bootType,
		strings.Join(fileNames, ", "), "", cmdline, ipxeScript)
	if err != nil {
		log.Printf("http: create image: %v", err)
		http.Error(w, "Failed to create image", http.StatusInternalServerError)
		return
	}

	imageDir := filepath.Join(s.DataDir, "images", fmt.Sprintf("%d", id))
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		log.Printf("http: create image dir: %v", err)
		http.Error(w, "Failed to save files", http.StatusInternalServerError)
		return
	}

	// Save all uploaded files with their original names
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		for _, header := range r.MultipartForm.File["files"] {
			f, err := header.Open()
			if err != nil {
				log.Printf("http: open uploaded file %s: %v", header.Filename, err)
				http.Error(w, "Failed to read uploaded file", http.StatusInternalServerError)
				return
			}
			// Sanitize: use only the base name, no path traversal
			safeName := filepath.Base(header.Filename)
			if err := saveFile(filepath.Join(imageDir, safeName), f); err != nil {
				f.Close()
				log.Printf("http: save file %s: %v", safeName, err)
				http.Error(w, "Failed to save file", http.StatusInternalServerError)
				return
			}
			f.Close()
		}
	}

	s.renderImageRow(w, id)
}

func (s *Server) handleUpdateImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	description := r.FormValue("description")
	bootType := r.FormValue("boot_type")
	if bootType == "" {
		bootType = db.BootTypeLinux
	}
	cmdline := r.FormValue("cmdline")
	ipxeScript := r.FormValue("ipxe_script")
	if err := db.UpdateImage(s.DB, id, name, description, bootType, cmdline, ipxeScript); err != nil {
		log.Printf("http: update image: %v", err)
		http.Error(w, "Failed to update image", http.StatusInternalServerError)
		return
	}
	s.renderImageRow(w, id)
}

func (s *Server) renderImageRow(w http.ResponseWriter, id int64) {
	img, err := db.GetImage(s.DB, id)
	if err != nil {
		log.Printf("http: get image: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if img == nil {
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}
	// Mark stale downloads as errors (e.g. server crashed mid-pull)
	if img.Status == db.ImageStatusDownloading {
		if updated, err := time.Parse("2006-01-02 15:04:05", img.UpdatedAt); err == nil {
			if time.Since(updated) > 35*time.Minute {
				db.UpdateImageStatus(s.DB, id, db.ImageStatusError, "Download timed out")
				img.Status = db.ImageStatusError
				img.StatusDetail = "Download timed out"
			}
		}
	}
	data := map[string]any{"Image": img}
	if err := s.Templates.ExecuteTemplate(w, "image_row", data); err != nil {
		log.Printf("http: render image row: %v", err)
	}
}

func (s *Server) handleImageRow(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	s.renderImageRow(w, id)
}

func (s *Server) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := db.DeleteImage(s.DB, id); err != nil {
		log.Printf("http: delete image: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	imageDir := filepath.Join(s.DataDir, "images", fmt.Sprintf("%d", id))
	os.RemoveAll(imageDir)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleServeImageFile(w http.ResponseWriter, r *http.Request) {
	if !s.validateToken(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	idNum, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")

	// Prevent path traversal
	name = filepath.Base(name)
	if name == "." || name == ".." {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	path := filepath.Join(s.DataDir, "images", fmt.Sprintf("%d", idNum), name)
	http.ServeFile(w, r, path)
}

func saveFile(dst string, src io.Reader) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err = io.Copy(f, src); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
