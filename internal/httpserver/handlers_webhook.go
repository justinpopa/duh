package httpserver

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/justinpopa/duh/internal/db"
	"github.com/justinpopa/duh/internal/webhook"
)

func (s *Server) handleWebhooksPage(w http.ResponseWriter, r *http.Request) {
	webhooks, err := db.ListWebhooks(s.DB)
	if err != nil {
		log.Printf("http: list webhooks: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	hash, _ := s.getAuthState()
	data := map[string]any{
		"Webhooks":    webhooks,
		"AuthEnabled": hash != "",
	}
	if err := s.Templates.ExecuteTemplate(w, "webhooks", data); err != nil {
		log.Printf("http: render webhooks: %v", err)
	}
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimSpace(r.FormValue("url"))
	secret := r.FormValue("secret")
	events := r.FormValue("events")
	if url == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}
	if events == "" {
		events = "*"
	}

	id, err := db.CreateWebhook(s.DB, url, secret, events)
	if err != nil {
		log.Printf("http: create webhook: %v", err)
		http.Error(w, "Failed to create webhook", http.StatusInternalServerError)
		return
	}
	wh, err := db.GetWebhook(s.DB, id)
	if err != nil || wh == nil {
		log.Printf("http: get created webhook: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data := map[string]any{"Webhook": wh}
	if err := s.Templates.ExecuteTemplate(w, "webhook_row", data); err != nil {
		log.Printf("http: render webhook row: %v", err)
	}
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if err := db.DeleteWebhook(s.DB, id); err != nil {
		log.Printf("http: delete webhook: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	wh, err := db.GetWebhook(s.DB, id)
	if err != nil || wh == nil {
		http.Error(w, "Webhook not found", http.StatusNotFound)
		return
	}
	event := webhook.Event{
		Type: "test",
		Data: map[string]any{
			"message": "This is a test webhook from duh",
		},
	}
	if err := webhook.DeliverSingle(*wh, event); err != nil {
		log.Printf("http: test webhook %d: %v", id, err)
		http.Error(w, "Delivery failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	w.Write([]byte("Sent!"))
}

func (s *Server) handleToggleWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	wh, err := db.GetWebhook(s.DB, id)
	if err != nil || wh == nil {
		http.Error(w, "Webhook not found", http.StatusNotFound)
		return
	}
	if err := db.UpdateWebhook(s.DB, id, wh.URL, wh.Secret, wh.Events, !wh.Enabled); err != nil {
		log.Printf("http: toggle webhook: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	wh.Enabled = !wh.Enabled
	data := map[string]any{"Webhook": wh}
	if err := s.Templates.ExecuteTemplate(w, "webhook_row", data); err != nil {
		log.Printf("http: render webhook row: %v", err)
	}
}
