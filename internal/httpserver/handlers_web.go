package httpserver

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/justinpopa/duh/internal/catalog"
	"github.com/justinpopa/duh/internal/db"
	"github.com/justinpopa/duh/internal/proxydhcp"
	"github.com/justinpopa/duh/internal/webhook"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	systems, err := db.ListSystems(s.DB)
	if err != nil {
		log.Printf("http: list systems: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	images, err := db.ListImages(s.DB)
	if err != nil {
		log.Printf("http: list images: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	profiles, err := db.ListProfiles(s.DB)
	if err != nil {
		log.Printf("http: list profiles: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	imageNames := make(map[int64]string, len(images))
	for _, img := range images {
		imageNames[img.ID] = img.Name
	}
	profileNames := make(map[int64]string, len(profiles))
	for _, p := range profiles {
		profileNames[p.ID] = p.Name
	}
	hash, _ := s.getAuthState()
	data := map[string]any{
		"Systems":      systems,
		"Images":       images,
		"Profiles":     profiles,
		"ImageNames":   imageNames,
		"ProfileNames": profileNames,
		"AuthEnabled":  hash != "",
	}
	if err := s.Templates.ExecuteTemplate(w, "dashboard", data); err != nil {
		log.Printf("http: render dashboard: %v", err)
	}
}

func (s *Server) handleImagesPage(w http.ResponseWriter, r *http.Request) {
	images, err := db.ListImages(s.DB)
	if err != nil {
		log.Printf("http: list images: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	imgHash, _ := s.getAuthState()
	data := map[string]any{
		"Images":      images,
		"AuthEnabled": imgHash != "",
	}

	// Merge catalog data if configured
	if s.CatalogURL != "" {
		var entries []catalog.Entry
		var fetchErr string
		cat, err := catalog.Fetch(s.CatalogURL)
		if err != nil {
			log.Printf("http: fetch catalog: %v", err)
			fetchErr = err.Error()
		} else {
			entries = cat.Entries
		}
		pulled := make(map[string]*db.Image)
		for i := range images {
			if images[i].CatalogID != "" {
				pulled[images[i].CatalogID] = &images[i]
			}
		}

		// Sort: unpulled first, then pulled
		sort.SliceStable(entries, func(i, j int) bool {
			_, iPulled := pulled[entries[i].ID]
			_, jPulled := pulled[entries[j].ID]
			return !iPulled && jPulled
		})
		data["CatalogEntries"] = entries
		data["CatalogPulled"] = pulled
		data["CatalogFetchErr"] = fetchErr
	}

	if err := s.Templates.ExecuteTemplate(w, "images", data); err != nil {
		log.Printf("http: render images: %v", err)
	}
}

func (s *Server) handleCreateSystem(w http.ResponseWriter, r *http.Request) {
	mac := r.FormValue("mac")
	hostname := r.FormValue("hostname")
	if mac == "" {
		http.Error(w, "MAC address is required", http.StatusBadRequest)
		return
	}
	sys, err := db.CreateSystem(s.DB, mac, hostname)
	if err != nil {
		log.Printf("http: create system: %v", err)
		http.Error(w, "Failed to create system", http.StatusBadRequest)
		return
	}
	s.fireSystemEvent(sys, "discovered")
	data := map[string]any{
		"System":       sys,
		"ImageNames":   map[int64]string{},
		"ProfileNames": map[int64]string{},
	}
	if err := s.Templates.ExecuteTemplate(w, "system_row", data); err != nil {
		log.Printf("http: render system row: %v", err)
	}
}

func (s *Server) handleUpdateSystem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	mac := r.FormValue("mac")
	hostname := r.FormValue("hostname")
	vars := r.FormValue("vars")
	if err := db.UpdateSystemInfo(s.DB, id, mac, hostname); err != nil {
		log.Printf("http: update system info: %v", err)
		http.Error(w, "Failed to update system", http.StatusBadRequest)
		return
	}
	if err := db.UpdateSystemVars(s.DB, id, vars); err != nil {
		log.Printf("http: update system vars: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// Update image assignment
	imageIDStr := r.FormValue("image_id")
	var imageID *int64
	if imageIDStr != "" && imageIDStr != "0" {
		v, err := strconv.ParseInt(imageIDStr, 10, 64)
		if err == nil {
			imageID = &v
		}
	}
	if err := db.UpdateSystemImage(s.DB, id, imageID); err != nil {
		log.Printf("http: update system image: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// Update profile assignment
	profileIDStr := r.FormValue("profile_id")
	var profileID *int64
	if profileIDStr != "" && profileIDStr != "0" {
		v, err := strconv.ParseInt(profileIDStr, 10, 64)
		if err == nil {
			profileID = &v
		}
	}
	if err := db.UpdateSystemProfile(s.DB, id, profileID); err != nil {
		log.Printf("http: update system profile: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	s.renderSystemRow(w, id)
}

func (s *Server) handleDeleteSystem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if err := db.DeleteSystem(s.DB, id); err != nil {
		log.Printf("http: delete system: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSystemStateAction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	action := r.FormValue("action")

	sys, err := db.GetSystemByID(s.DB, id)
	if err != nil || sys == nil {
		http.Error(w, "System not found", http.StatusNotFound)
		return
	}

	var newState string
	switch action {
	case "queue":
		if sys.State != "discovered" && sys.State != "ready" && sys.State != "failed" {
			http.Error(w, fmt.Sprintf("Cannot queue from state %s", sys.State), http.StatusBadRequest)
			return
		}
		if sys.ImageID == nil || sys.Hostname == "" {
			http.Error(w, "Image and hostname must be set before queuing", http.StatusBadRequest)
			return
		}
		newState = "queued"
	case "cancel":
		if sys.State != "queued" {
			http.Error(w, "Can only cancel from queued state", http.StatusBadRequest)
			return
		}
		if sys.Hostname != "" {
			newState = "ready"
		} else {
			newState = "discovered"
		}
	case "retry":
		if sys.State != "failed" {
			http.Error(w, "Can only retry from failed state", http.StatusBadRequest)
			return
		}
		newState = "queued"
	case "mark_failed":
		if sys.State != "provisioning" {
			http.Error(w, "Can only mark failed from provisioning state", http.StatusBadRequest)
			return
		}
		newState = "failed"
	case "reimage":
		if sys.State != "ready" {
			http.Error(w, "Can only reimage from ready state", http.StatusBadRequest)
			return
		}
		newState = "queued"
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	if err := db.UpdateSystemState(s.DB, id, newState); err != nil {
		log.Printf("http: state action %s: %v", action, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	s.fireSystemEvent(sys, newState)
	s.renderSystemRow(w, id)
}

func (s *Server) handleToggleConfirmGlobal(w http.ResponseWriter, r *http.Request) {
	val := "0"
	if r.FormValue("value") == "true" {
		val = "1"
	}
	if err := db.SetSetting(s.DB, "confirm_reimage", val); err != nil {
		log.Printf("http: toggle global confirm: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data := map[string]any{
		"ConfirmGlobal": val == "1",
	}
	if err := s.Templates.ExecuteTemplate(w, "confirm_global", data); err != nil {
		log.Printf("http: render confirm_global: %v", err)
	}
}

func (s *Server) renderSystemRow(w http.ResponseWriter, id int64) {
	sys, err := db.GetSystemByID(s.DB, id)
	if err != nil {
		log.Printf("http: get system: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if sys == nil {
		http.Error(w, "System not found", http.StatusNotFound)
		return
	}
	images, err := db.ListImages(s.DB)
	if err != nil {
		log.Printf("http: list images: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	profiles, err := db.ListProfiles(s.DB)
	if err != nil {
		log.Printf("http: list profiles: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	imageNames := make(map[int64]string, len(images))
	for _, img := range images {
		imageNames[img.ID] = img.Name
	}
	profileNames := make(map[int64]string, len(profiles))
	for _, p := range profiles {
		profileNames[p.ID] = p.Name
	}
	data := map[string]any{
		"System":       sys,
		"ImageNames":   imageNames,
		"ProfileNames": profileNames,
	}
	if err := s.Templates.ExecuteTemplate(w, "system_row", data); err != nil {
		log.Printf("http: render system row: %v", err)
	}
}

func (s *Server) handleSetupPage(w http.ResponseWriter, r *http.Request) {
	// Detect server IP
	var serverIP string
	_, ip, err := proxydhcp.DetectInterface()
	if err != nil {
		serverIP = "SERVER_IP"
	} else {
		serverIP = ip.String()
	}

	// Parse ports from addr strings (format ":8080" or "0.0.0.0:8080")
	tftpPort := "69"
	if i := strings.LastIndex(s.TFTPAddr, ":"); i >= 0 {
		tftpPort = s.TFTPAddr[i+1:]
	}
	httpPort := "8080"
	if i := strings.LastIndex(s.HTTPAddr, ":"); i >= 0 {
		httpPort = s.HTTPAddr[i+1:]
	}

	serverURL := s.ServerURL
	if serverURL == "" {
		serverURL = fmt.Sprintf("http://%s:%s", serverIP, httpPort)
	}

	setupHash, _ := s.getAuthState()
	globalConfirm, _ := db.GetSetting(s.DB, "confirm_reimage")
	data := map[string]any{
		"ServerIP":       serverIP,
		"TFTPPort":       tftpPort,
		"HTTPPort":       httpPort,
		"ServerURL":      serverURL,
		"ProxyDHCP":      s.ProxyDHCP,
		"AuthEnabled":    setupHash != "",
		"HasPassword":    setupHash != "",
		"ConfirmGlobal":  globalConfirm == "1",
		"Error":          r.URL.Query().Get("error"),
		"Success":        r.URL.Query().Get("success"),
	}
	if err := s.Templates.ExecuteTemplate(w, "setup", data); err != nil {
		log.Printf("http: render setup: %v", err)
	}
}

func (s *Server) fireSystemEvent(sys *db.System, state string) {
	s.Webhook.Fire(webhook.Event{
		Type: "system." + state,
		Data: map[string]any{
			"id":       sys.ID,
			"mac":      sys.MAC,
			"hostname": sys.Hostname,
			"ip_addr":  sys.IPAddr,
			"state":    state,
		},
	})
}
