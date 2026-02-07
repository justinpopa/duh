package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("http: %s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start).Round(time.Millisecond))
	})
}

func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("http: panic: %v\n%s", err, debug.Stack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

const (
	sessionCookieName = "duh_session"
	sessionMaxAge     = 30 * 24 * 60 * 60 // 30 days in seconds
)

// HTTPSRedirectMiddleware redirects browser HTTP requests to HTTPS.
// The entire boot/provisioning chain is excluded: iPXE clients (by User-Agent)
// and machine-to-machine paths (configs, images, API callbacks, iPXE binaries).
func HTTPSRedirectMiddleware(httpsPort string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip redirect for iPXE clients
		if strings.Contains(r.UserAgent(), "iPXE") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip redirect for boot-chain paths (hit by installers, not browsers)
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") ||
			strings.HasPrefix(p, "/config/") ||
			strings.HasPrefix(p, "/images/") ||
			strings.HasPrefix(p, "/profiles/") && strings.Contains(p, "/overlay/") ||
			p == "/boot.ipxe" ||
			p == "/ipxe.efi" ||
			p == "/ipxe-arm64.efi" ||
			p == "/undionly.kpxe" {
			next.ServeHTTP(w, r)
			return
		}

		host := r.Host
		// Strip existing port if present
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if httpsPort != "" && httpsPort != "443" && httpsPort != ":443" {
			host = net.JoinHostPort(host, httpsPort)
		}

		target := "https://" + host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

// AuthMiddleware wraps a handler to require authentication when a password is set.
func (s *Server) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hash, key := s.getAuthState()
		if hash == "" {
			next(w, r)
			return
		}
		if s.validateSession(r, key) {
			next(w, r)
			return
		}
		// Not authenticated
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", "/login")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// createSession creates a signed session cookie value.
func (s *Server) createSession(w http.ResponseWriter, key []byte) {
	expiry := time.Now().Add(time.Duration(sessionMaxAge) * time.Second).Unix()
	payload := fmt.Sprintf("%d", expiry)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + base64.RawURLEncoding.EncodeToString(sig)))
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   sessionMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// validateSession checks if the request has a valid session cookie.
func (s *Server) validateSession(r *http.Request, key []byte) bool {
	if len(key) == 0 {
		return false
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return false
	}
	expiryStr, sigB64 := parts[0], parts[1]
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expiry {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(expiryStr))
	return hmac.Equal(mac.Sum(nil), sig)
}

// clearSession removes the session cookie.
func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// CSRFMiddleware checks Origin/Referer on state-changing requests to prevent
// cross-site request forgery. Boot-chain paths that use HMAC auth are skipped.
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		// Skip boot-chain paths (authenticated via HMAC, not cookies)
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == "" {
			// Fall back to Referer
			ref := r.Header.Get("Referer")
			if ref != "" {
				if u, err := url.Parse(ref); err == nil {
					origin = u.Scheme + "://" + u.Host
				}
			}
		}

		if origin == "" {
			// No Origin or Referer — allow same-site form submissions
			// from browsers that don't send these headers (rare)
			next.ServeHTTP(w, r)
			return
		}

		u, err := url.Parse(origin)
		if err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		requestHost := r.Host
		originHost := u.Host
		if originHost == "" {
			originHost = u.Hostname()
		}

		// Normalize: strip default ports for comparison
		if h, port, err := net.SplitHostPort(requestHost); err == nil {
			if port == "80" || port == "443" {
				requestHost = h
			}
		}
		if h, port, err := net.SplitHostPort(originHost); err == nil {
			if port == "80" || port == "443" {
				originHost = h
			}
		}

		if !strings.EqualFold(originHost, requestHost) {
			log.Printf("http: CSRF blocked: origin %q does not match host %q", origin, r.Host)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
