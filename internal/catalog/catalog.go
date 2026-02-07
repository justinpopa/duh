package catalog

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justinpopa/duh/internal/db"
	"github.com/justinpopa/duh/internal/safenet"
)

type Catalog struct {
	SchemaVersion int     `json:"schema_version"`
	Entries       []Entry `json:"entries"`
}

type File struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
}

type VarDef struct {
	Key         string   `json:"key"`
	Label       string   `json:"label,omitempty"`
	Type        string   `json:"type,omitempty"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type Entry struct {
	ID             string   `json:"id"`
	Icon           string   `json:"icon,omitempty"`
	IconColor      string   `json:"icon_color,omitempty"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Version        string   `json:"version"`
	Arch           string   `json:"arch"`
	BootType       string   `json:"boot_type"`
	Cmdline        string   `json:"cmdline"`
	IPXEScript     string   `json:"ipxe_script"`
	Files          []File   `json:"files"`
	OSFamily       string   `json:"os_family,omitempty"`
	KernelParams   string   `json:"kernel_params,omitempty"`
	ConfigTemplate string   `json:"config_template,omitempty"`
	Vars           []VarDef `json:"vars,omitempty"`
}

// ProfileData holds the profile-related fields extracted from a catalog entry.
type ProfileData struct {
	Name           string
	Description    string
	OSFamily       string
	KernelParams   string
	ConfigTemplate string
	DefaultVars    string // JSON
	VarSchema      string // JSON
}

// ProfileDataFromEntry extracts profile fields from a catalog entry.
// Returns nil if the entry has no config_template and no kernel_params.
func ProfileDataFromEntry(entry Entry) *ProfileData {
	if entry.ConfigTemplate == "" && entry.KernelParams == "" {
		return nil
	}

	osFamily := entry.OSFamily
	if osFamily == "" {
		osFamily = "custom"
	}

	// Build default vars JSON from VarDef defaults
	vars := make(map[string]string)
	for _, v := range entry.Vars {
		if v.Default != "" {
			vars[v.Key] = v.Default
		}
	}
	defaultVars := "{}"
	if len(vars) > 0 {
		b, _ := json.Marshal(vars)
		defaultVars = string(b)
	}

	// Build var schema JSON
	varSchema := ""
	if len(entry.Vars) > 0 {
		b, _ := json.Marshal(entry.Vars)
		varSchema = string(b)
	}

	return &ProfileData{
		Name:           entry.Name + " Profile",
		Description:    "Auto-created from catalog: " + entry.Name,
		OSFamily:       osFamily,
		KernelParams:   entry.KernelParams,
		ConfigTemplate: entry.ConfigTemplate,
		DefaultVars:    defaultVars,
		VarSchema:      varSchema,
	}
}

// Hash returns a deterministic SHA-256 hash of the entry's content fields.
// Empty new fields produce the same hash as before for backward compat.
func (e Entry) Hash() string {
	h := sha256.New()
	h.Write([]byte(e.Name))
	h.Write([]byte{0})
	h.Write([]byte(e.Description))
	h.Write([]byte{0})
	h.Write([]byte(e.Version))
	h.Write([]byte{0})
	h.Write([]byte(e.Arch))
	h.Write([]byte{0})
	h.Write([]byte(e.BootType))
	h.Write([]byte{0})
	h.Write([]byte(e.Cmdline))
	h.Write([]byte{0})
	h.Write([]byte(e.IPXEScript))
	h.Write([]byte{0})
	sorted := make([]File, len(e.Files))
	copy(sorted, e.Files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, f := range sorted {
		h.Write([]byte(f.Name))
		h.Write([]byte{0})
		h.Write([]byte(f.URL))
		h.Write([]byte{0})
	}
	// New fields — empty values don't change the hash (backward compat)
	if e.OSFamily != "" || e.KernelParams != "" || e.ConfigTemplate != "" || len(e.Vars) > 0 {
		h.Write([]byte(e.OSFamily))
		h.Write([]byte{0})
		h.Write([]byte(e.KernelParams))
		h.Write([]byte{0})
		h.Write([]byte(e.ConfigTemplate))
		h.Write([]byte{0})
		for _, v := range e.Vars {
			h.Write([]byte(v.Key))
			h.Write([]byte{0})
			h.Write([]byte(v.Default))
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func Fetch(catalogURL string) (*Catalog, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(catalogURL)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog returned %d", resp.StatusCode)
	}

	var cat Catalog
	if err := json.NewDecoder(resp.Body).Decode(&cat); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	return &cat, nil
}

func Pull(database *sql.DB, dataDir string, entry Entry, force bool) (int64, error) {
	hash := entry.Hash()

	// Check if already pulled
	existing, err := db.GetImageByCatalogID(database, entry.ID)
	if err != nil {
		return 0, err
	}
	if existing != nil {
		if existing.Status == db.ImageStatusDownloading {
			return existing.ID, fmt.Errorf("already downloading")
		}
		if existing.Status == db.ImageStatusReady && !force {
			// Update icon if catalog has newer data
			if entry.Icon != existing.Icon || entry.IconColor != existing.IconColor {
				db.UpdateImageIcon(database, existing.ID, entry.Icon, entry.IconColor)
			}
			return existing.ID, fmt.Errorf("already pulled")
		}
		if existing.Status == db.ImageStatusError {
			// Error state: delete and recreate
			imageDir := filepath.Join(dataDir, "images", fmt.Sprintf("%d", existing.ID))
			os.RemoveAll(imageDir)
			db.DeleteImage(database, existing.ID)
		}
	}

	var id int64
	if existing != nil && existing.Status != db.ImageStatusError {
		// Force update: reset in place to preserve ID
		id = existing.ID
		imageDir := filepath.Join(dataDir, "images", fmt.Sprintf("%d", id))
		os.RemoveAll(imageDir)
		if err := db.ResetCatalogImage(database, id, entry.Name, entry.Description,
			entry.BootType, entry.Cmdline, entry.IPXEScript, hash, entry.Icon, entry.IconColor); err != nil {
			return 0, err
		}
	} else {
		var err error
		id, err = db.CreateCatalogImage(database, entry.Name, entry.Description,
			entry.BootType, entry.Cmdline, entry.IPXEScript, entry.ID, hash, entry.Icon, entry.IconColor)
		if err != nil {
			return 0, err
		}
	}

	// Download in background
	go func() {
		imageDir := filepath.Join(dataDir, "images", fmt.Sprintf("%d", id))
		if err := os.MkdirAll(imageDir, 0755); err != nil {
			db.UpdateImageStatus(database, id, db.ImageStatusError, err.Error())
			return
		}

		var downloaded []string
		for i, f := range entry.Files {
			log.Printf("catalog: downloading %s for %s", f.Name, entry.Name)
			db.UpdateImageStatus(database, id, db.ImageStatusDownloading,
				fmt.Sprintf("%d/%d %s 0%%", i+1, len(entry.Files), f.Name))

			var lastPct int64
			var lastUpdate time.Time
			onProgress := func(dl, total int64) {
				pct := dl * 100 / total
				if pct != lastPct && time.Since(lastUpdate) > time.Second {
					lastPct = pct
					lastUpdate = time.Now()
					db.UpdateImageStatus(database, id, db.ImageStatusDownloading,
						fmt.Sprintf("%d/%d %s %d%%", i+1, len(entry.Files), f.Name, pct))
				}
			}

			safeName := filepath.Base(f.Name)
			if err := downloadFile(filepath.Join(imageDir, safeName), f.URL, onProgress); err != nil {
				log.Printf("catalog: download %s failed: %v", f.Name, err)
				db.UpdateImageStatus(database, id, db.ImageStatusError,
					fmt.Sprintf("Failed to download %s: %v", f.Name, err))
				return
			}
			downloaded = append(downloaded, safeName)
		}

		db.UpdateImageFiles(database, id, strings.Join(downloaded, ", "))
		db.UpdateImageStatus(database, id, db.ImageStatusReady, "")
		log.Printf("catalog: %s ready (%d files)", entry.Name, len(downloaded))
	}()

	return id, nil
}

// validateDownloadURL checks that a URL is safe to fetch (http/https only).
func validateDownloadURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("URL has no host")
	}
	return nil
}

type progressFunc func(downloaded, total int64)

func downloadFile(dst, rawURL string, onProgress progressFunc) error {
	if err := validateDownloadURL(rawURL); err != nil {
		return err
	}

	client := safenet.NewClient(30 * time.Minute)
	resp, err := client.Get(rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	if onProgress == nil || resp.ContentLength <= 0 {
		_, err = io.Copy(f, resp.Body)
		return err
	}

	var written int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
			onProgress(written, resp.ContentLength)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}
