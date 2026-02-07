package httpserver

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/justinpopa/duh/internal/db"
	"github.com/justinpopa/duh/internal/webhook"
)

type Server struct {
	DB         *sql.DB
	DataDir    string
	ServerURL  string
	CatalogURL string
	TFTPAddr   string
	HTTPAddr   string
	ProxyDHCP  bool
	Templates  *template.Template
	StaticFS   fs.FS
	Webhook    *webhook.Dispatcher

	authMu       sync.RWMutex
	passwordHash string
	signingKey   []byte
	authLoaded   bool
}

func New(database *sql.DB, dataDir, serverURL, catalogURL, tftpAddr, httpAddr string, proxyDHCP bool, tmplFS fs.FS, staticFS fs.FS) (*Server, error) {
	funcMap := template.FuncMap{
		"deref": func(p *int64) int64 {
			if p == nil {
				return 0
			}
			return *p
		},
		"jsonAttr": func(v any) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
		"dict": func(pairs ...any) map[string]any {
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i+1 < len(pairs); i += 2 {
				if k, ok := pairs[i].(string); ok {
					m[k] = pairs[i+1]
				}
			}
			return m
		},
		"timeSince": func(t string) string {
			if t == "" {
				return ""
			}
			parsed, err := time.Parse("2006-01-02 15:04:05", t)
			if err != nil {
				return ""
			}
			d := time.Since(parsed)
			switch {
			case d < time.Minute:
				return fmt.Sprintf("%ds", int(d.Seconds()))
			case d < time.Hour:
				return fmt.Sprintf("%dm", int(d.Minutes()))
			case d < 24*time.Hour:
				return fmt.Sprintf("%dh", int(d.Hours()))
			default:
				return fmt.Sprintf("%dd", int(math.Floor(d.Hours()/24)))
			}
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(tmplFS, "*.html")
	if err != nil {
		return nil, err
	}

	return &Server{
		DB:         database,
		DataDir:    dataDir,
		ServerURL:  serverURL,
		CatalogURL: catalogURL,
		TFTPAddr:   tftpAddr,
		HTTPAddr:   httpAddr,
		ProxyDHCP:  proxyDHCP,
		Templates:  tmpl,
		StaticFS:   staticFS,
		Webhook:    webhook.NewDispatcher(database),
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return LoggingMiddleware(RecoveryMiddleware(CSRFMiddleware(mux)))
}

// loadAuthCache reads password_hash and session_key from DB into memory.
func (s *Server) loadAuthCache() {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if s.authLoaded {
		return
	}
	hash, err := db.GetSetting(s.DB, "password_hash")
	if err != nil {
		log.Printf("http: load password_hash: %v", err)
	}
	s.passwordHash = hash

	keyHex, err := db.GetSetting(s.DB, "session_key")
	if err != nil {
		log.Printf("http: load session_key: %v", err)
	}
	if keyHex != "" {
		s.signingKey, _ = hex.DecodeString(keyHex)
	}
	s.authLoaded = true
}

// resetAuthCache forces a reload from DB on the next request.
func (s *Server) resetAuthCache() {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.authLoaded = false
}

// getAuthState returns the cached password hash and signing key.
func (s *Server) getAuthState() (hash string, key []byte) {
	s.loadAuthCache()
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	return s.passwordHash, s.signingKey
}

// authEnabled returns true if a password has been set.
func (s *Server) authEnabled() bool {
	hash, _ := s.getAuthState()
	return hash != ""
}

// ensureSigningKey generates and persists a signing key if one doesn't exist.
func (s *Server) ensureSigningKey() ([]byte, error) {
	_, key := s.getAuthState()
	if len(key) > 0 {
		return key, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := db.SetSetting(s.DB, "session_key", hex.EncodeToString(b)); err != nil {
		return nil, err
	}
	s.resetAuthCache()
	_, key = s.getAuthState()
	return key, nil
}
