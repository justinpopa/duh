package httpserver

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/justinpopa/duh/internal/db"
	"github.com/justinpopa/duh/internal/profile"
)

func (s *Server) handleProfilesPage(w http.ResponseWriter, r *http.Request) {
	profiles, err := db.ListProfiles(s.DB)
	if err != nil {
		log.Printf("http: list profiles: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	profHash, _ := s.getAuthState()
	data := map[string]any{
		"Profiles":    profiles,
		"AuthEnabled": profHash != "",
	}
	if err := s.Templates.ExecuteTemplate(w, "profiles", data); err != nil {
		log.Printf("http: render profiles: %v", err)
	}
}

func (s *Server) handleProfileEditorNew(w http.ResponseWriter, r *http.Request) {
	profHash, _ := s.getAuthState()
	data := map[string]any{
		"Profile":     &db.Profile{DefaultVars: "{}", OSFamily: "custom"},
		"IsNew":       true,
		"AuthEnabled": profHash != "",
	}
	if err := s.Templates.ExecuteTemplate(w, "profile_editor", data); err != nil {
		log.Printf("http: render profile editor (new): %v", err)
	}
}

func (s *Server) handleProfileEditor(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	p, err := db.GetProfile(s.DB, id)
	if err != nil {
		log.Printf("http: get profile: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

	profHash, _ := s.getAuthState()
	data := map[string]any{
		"Profile":     p,
		"IsNew":       false,
		"AuthEnabled": profHash != "",
	}
	if err := s.Templates.ExecuteTemplate(w, "profile_editor", data); err != nil {
		log.Printf("http: render profile editor: %v", err)
	}
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 1 << 30 // 1 GB
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
	osFamily := r.FormValue("os_family")
	configTemplate := r.FormValue("config_template")
	kernelParams := r.FormValue("kernel_params")
	defaultVars := r.FormValue("default_vars")
	varSchema := r.FormValue("var_schema")

	var overlayFileName string
	file, header, err := r.FormFile("overlay_file")
	if err == nil {
		defer file.Close()
		overlayFileName = filepath.Base(header.Filename)
	}

	id, err := db.CreateProfile(s.DB, name, description, osFamily, configTemplate, kernelParams, defaultVars, overlayFileName, varSchema, "")
	if err != nil {
		log.Printf("http: create profile: %v", err)
		http.Error(w, "Failed to create profile: "+err.Error(), http.StatusBadRequest)
		return
	}

	if overlayFileName != "" {
		profileDir := filepath.Join(s.DataDir, "profiles", fmt.Sprintf("%d", id))
		if err := os.MkdirAll(profileDir, 0755); err != nil {
			log.Printf("http: create profile dir: %v", err)
			http.Error(w, "Failed to save overlay file", http.StatusInternalServerError)
			return
		}
		if err := saveFile(filepath.Join(profileDir, overlayFileName), file); err != nil {
			log.Printf("http: save overlay file: %v", err)
			http.Error(w, "Failed to save overlay file", http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/profiles/%d", id), http.StatusSeeOther)
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	const maxUpload = 1 << 30 // 1 GB
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
	osFamily := r.FormValue("os_family")
	configTemplate := r.FormValue("config_template")
	kernelParams := r.FormValue("kernel_params")
	defaultVars := r.FormValue("default_vars")
	varSchema := r.FormValue("var_schema")

	existing, err := db.GetProfile(s.DB, id)
	if err != nil || existing == nil {
		log.Printf("http: get profile for update: %v", err)
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

	overlayFileName := existing.OverlayFile
	profileDir := filepath.Join(s.DataDir, "profiles", fmt.Sprintf("%d", id))

	// Handle overlay removal
	if r.FormValue("remove_overlay") == "true" {
		if overlayFileName != "" {
			os.RemoveAll(profileDir)
		}
		overlayFileName = ""
	}

	// Handle new overlay upload (replaces existing)
	file, header, err := r.FormFile("overlay_file")
	if err == nil {
		defer file.Close()
		// Remove old overlay dir if it exists
		if existing.OverlayFile != "" {
			os.RemoveAll(profileDir)
		}
		overlayFileName = filepath.Base(header.Filename)
		if err := os.MkdirAll(profileDir, 0755); err != nil {
			log.Printf("http: create profile dir: %v", err)
			http.Error(w, "Failed to save overlay file", http.StatusInternalServerError)
			return
		}
		if err := saveFile(filepath.Join(profileDir, overlayFileName), file); err != nil {
			log.Printf("http: save overlay file: %v", err)
			http.Error(w, "Failed to save overlay file", http.StatusInternalServerError)
			return
		}
	}

	if err := db.UpdateProfile(s.DB, id, name, description, osFamily, configTemplate, kernelParams, defaultVars, overlayFileName, varSchema); err != nil {
		log.Printf("http: update profile: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/profiles", http.StatusSeeOther)
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := db.DeleteProfile(s.DB, id); err != nil {
		log.Printf("http: delete profile: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	profileDir := filepath.Join(s.DataDir, "profiles", fmt.Sprintf("%d", id))
	os.RemoveAll(profileDir)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleServeConfig(w http.ResponseWriter, r *http.Request) {
	if !s.validateToken(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	sys, err := db.GetSystemByID(s.DB, id)
	if err != nil {
		log.Printf("http: config system lookup: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if sys == nil {
		http.Error(w, "System not found", http.StatusNotFound)
		return
	}
	if sys.ProfileID == nil {
		http.Error(w, "No profile assigned", http.StatusNotFound)
		return
	}

	prof, err := db.GetProfile(s.DB, *sys.ProfileID)
	if err != nil {
		log.Printf("http: config profile lookup: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if prof == nil {
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

	serverURL := s.ServerURL
	if serverURL == "" {
		serverURL = "http://" + r.Host
	}

	vars, err := profile.BuildVars(prof.DefaultVars, sys.Vars)
	if err != nil {
		log.Printf("http: config build vars: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var imageID int64
	if sys.ImageID != nil {
		imageID = *sys.ImageID
	}

	tv := profile.TemplateVars{
		MAC:         sys.MAC,
		Hostname:    sys.Hostname,
		IP:          sys.IPAddr,
		SystemID:    sys.ID,
		ImageID:     imageID,
		ServerURL:   serverURL,
		ConfigURL:   s.signURL(fmt.Sprintf("%s/config/%d", serverURL, sys.ID)),
		CallbackURL: s.signURL(fmt.Sprintf("%s/api/v1/systems/%s/callback", serverURL, sys.MAC)),
		Vars:        vars,
	}

	rendered, err := profile.RenderConfigTemplate(prof.ConfigTemplate, tv)
	if err != nil {
		log.Printf("http: config render: %v", err)
		http.Error(w, "Template render error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(rendered))
}

func (s *Server) handleServeOverlayFile(w http.ResponseWriter, r *http.Request) {
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

	name = filepath.Base(name)
	if name == "." || name == ".." {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	path := filepath.Join(s.DataDir, "profiles", fmt.Sprintf("%d", idNum), name)
	http.ServeFile(w, r, path)
}
