package httpserver

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/justinpopa/duh/internal/db"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	stats, err := db.GetStats(s.DB)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("http: healthz: %v", err)
		json.NewEncoder(w).Encode(map[string]string{"status": "error"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "healthy",
		"stats":  stats,
	})
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	if !s.validateToken(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	mac := r.PathValue("mac")
	if mac == "" {
		http.Error(w, "MAC address required", http.StatusBadRequest)
		return
	}

	if err := db.TransitionSystemStateByMAC(s.DB, mac, "provisioning", "ready"); err != nil {
		log.Printf("http: callback state transition: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sys, _ := db.GetSystemByMAC(s.DB, mac)
	if sys != nil {
		s.fireSystemEvent(sys, "ready")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
