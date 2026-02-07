package httpserver

import (
	"log"
	"net/http"
	"net/url"

	"github.com/justinpopa/duh/internal/db"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if !s.authEnabled() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	_, key := s.getAuthState()
	if s.validateSession(r, key) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	data := map[string]any{
		"Error": r.URL.Query().Get("error"),
	}
	if err := s.Templates.ExecuteTemplate(w, "login", data); err != nil {
		log.Printf("http: render login: %v", err)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	hash, _ := s.getAuthState()
	if hash == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	password := r.FormValue("password")
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		http.Redirect(w, r, "/login?error=invalid", http.StatusFound)
		return
	}
	key, err := s.ensureSigningKey()
	if err != nil {
		log.Printf("http: ensure signing key: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	s.createSession(w, key)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSession(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func setupRedirect(w http.ResponseWriter, r *http.Request, msg, msgType string) {
	v := url.Values{}
	v.Set(msgType, msg)
	http.Redirect(w, r, "/setup?"+v.Encode(), http.StatusFound)
}

func (s *Server) handleSetPassword(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")
	if password == "" {
		setupRedirect(w, r, "Password cannot be empty.", "error")
		return
	}
	if password != confirm {
		setupRedirect(w, r, "Passwords do not match.", "error")
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("http: bcrypt hash: %v", err)
		setupRedirect(w, r, "Internal error.", "error")
		return
	}
	if err := db.SetSetting(s.DB, "password_hash", string(hashed)); err != nil {
		log.Printf("http: set password_hash: %v", err)
		setupRedirect(w, r, "Internal error.", "error")
		return
	}
	s.resetAuthCache()
	key, err := s.ensureSigningKey()
	if err != nil {
		log.Printf("http: ensure signing key: %v", err)
		setupRedirect(w, r, "Internal error.", "error")
		return
	}
	s.createSession(w, key)
	setupRedirect(w, r, "Password set successfully.", "success")
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	current := r.FormValue("current")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")
	hash, _ := s.getAuthState()
	if hash == "" {
		setupRedirect(w, r, "No password is set.", "error")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)); err != nil {
		setupRedirect(w, r, "Current password is incorrect.", "error")
		return
	}
	if password == "" {
		setupRedirect(w, r, "New password cannot be empty.", "error")
		return
	}
	if password != confirm {
		setupRedirect(w, r, "Passwords do not match.", "error")
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("http: bcrypt hash: %v", err)
		setupRedirect(w, r, "Internal error.", "error")
		return
	}
	if err := db.SetSetting(s.DB, "password_hash", string(hashed)); err != nil {
		log.Printf("http: set password_hash: %v", err)
		setupRedirect(w, r, "Internal error.", "error")
		return
	}
	// Regenerate signing key to invalidate all sessions
	if err := db.DeleteSetting(s.DB, "session_key"); err != nil {
		log.Printf("http: delete session_key: %v", err)
	}
	s.resetAuthCache()
	key, err := s.ensureSigningKey()
	if err != nil {
		log.Printf("http: ensure signing key: %v", err)
		setupRedirect(w, r, "Internal error.", "error")
		return
	}
	s.createSession(w, key)
	setupRedirect(w, r, "Password changed. All other sessions have been invalidated.", "success")
}

func (s *Server) handleRemovePassword(w http.ResponseWriter, r *http.Request) {
	current := r.FormValue("current")
	hash, _ := s.getAuthState()
	if hash == "" {
		setupRedirect(w, r, "No password is set.", "error")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)); err != nil {
		setupRedirect(w, r, "Current password is incorrect.", "error")
		return
	}
	if err := db.DeleteSetting(s.DB, "password_hash"); err != nil {
		log.Printf("http: delete password_hash: %v", err)
		setupRedirect(w, r, "Internal error.", "error")
		return
	}
	if err := db.DeleteSetting(s.DB, "session_key"); err != nil {
		log.Printf("http: delete session_key: %v", err)
	}
	s.resetAuthCache()
	clearSession(w)
	setupRedirect(w, r, "Password removed. Authentication is now disabled.", "success")
}
