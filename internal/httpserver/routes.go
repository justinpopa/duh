package httpserver

import (
	"net/http"
)

func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return s.AuthMiddleware(h)
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// --- Public (no auth) ---

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(s.StaticFS)))

	// Health check
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	// Auth pages
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("POST /logout", s.handleLogout)

	// Boot endpoints (machines can't do cookies)
	mux.HandleFunc("GET /boot.ipxe", s.handleBootScript)
	mux.HandleFunc("GET /ipxe.efi", s.handleServeIPXE)
	mux.HandleFunc("GET /ipxe-arm64.efi", s.handleServeIPXEArm64)
	mux.HandleFunc("GET /undionly.kpxe", s.handleServeUndionly)

	// Image/config/overlay file serving (used by booting machines)
	mux.HandleFunc("GET /images/{id}/file/{name}", s.handleServeImageFile)
	mux.HandleFunc("GET /config/{id}", s.handleServeConfig)
	mux.HandleFunc("GET /profiles/{id}/overlay/{name}", s.handleServeOverlayFile)

	// API callbacks
	mux.HandleFunc("POST /api/v1/systems/{mac}/callback", s.handleCallback)

	// --- Protected (auth required) ---

	// Web UI pages
	mux.HandleFunc("GET /{$}", s.auth(s.handleDashboard))
	mux.HandleFunc("GET /images", s.auth(s.handleImagesPage))
	mux.HandleFunc("GET /profiles", s.auth(s.handleProfilesPage))
	mux.HandleFunc("GET /setup", s.auth(s.handleSetupPage))
	mux.HandleFunc("POST /dhcp/test", s.auth(s.handleDHCPTest))

	// System CRUD (htmx)
	mux.HandleFunc("POST /systems", s.auth(s.handleCreateSystem))
	mux.HandleFunc("PUT /systems/{id}", s.auth(s.handleUpdateSystem))
	mux.HandleFunc("DELETE /systems/{id}", s.auth(s.handleDeleteSystem))
	mux.HandleFunc("PUT /systems/{id}/state", s.auth(s.handleSystemStateAction))
	mux.HandleFunc("PUT /settings/confirm-reimage", s.auth(s.handleToggleConfirmGlobal))

	// Image CRUD
	mux.HandleFunc("POST /images/upload", s.auth(s.handleUploadImage))
	mux.HandleFunc("GET /images/{id}/row", s.auth(s.handleImageRow))
	mux.HandleFunc("PUT /images/{id}", s.auth(s.handleUpdateImage))
	mux.HandleFunc("DELETE /images/{id}", s.auth(s.handleDeleteImage))

	// Profile CRUD
	mux.HandleFunc("GET /profiles/new", s.auth(s.handleProfileEditorNew))
	mux.HandleFunc("GET /profiles/{id}", s.auth(s.handleProfileEditor))
	mux.HandleFunc("POST /profiles", s.auth(s.handleCreateProfile))
	mux.HandleFunc("POST /profiles/{id}", s.auth(s.handleUpdateProfile))
	mux.HandleFunc("DELETE /profiles/{id}", s.auth(s.handleDeleteProfile))

	// Catalog
	mux.HandleFunc("POST /catalog/pull", s.auth(s.handleCatalogPull))

	// Webhooks
	mux.HandleFunc("GET /webhooks", s.auth(s.handleWebhooksPage))
	mux.HandleFunc("POST /webhooks", s.auth(s.handleCreateWebhook))
	mux.HandleFunc("DELETE /webhooks/{id}", s.auth(s.handleDeleteWebhook))
	mux.HandleFunc("POST /webhooks/{id}/test", s.auth(s.handleTestWebhook))
	mux.HandleFunc("PUT /webhooks/{id}/toggle", s.auth(s.handleToggleWebhook))

	// Password management
	mux.HandleFunc("POST /auth/set-password", s.auth(s.handleSetPassword))
	mux.HandleFunc("POST /auth/change-password", s.auth(s.handleChangePassword))
	mux.HandleFunc("POST /auth/remove-password", s.auth(s.handleRemovePassword))
}
