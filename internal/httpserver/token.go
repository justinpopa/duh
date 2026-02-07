package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const tokenExpiry = 1 * time.Hour

// signURL appends a tok= query parameter containing an HMAC-signed token
// bound to the URL path with a 1-hour expiry.
func (s *Server) signURL(rawURL string) string {
	_, key := s.getAuthState()
	if len(key) == 0 {
		return rawURL
	}

	// Split URL into path and existing query
	path := rawURL
	query := ""
	if i := strings.IndexByte(rawURL, '?'); i >= 0 {
		path = rawURL[:i]
		query = rawURL[i+1:]
	}

	expiry := time.Now().Add(tokenExpiry).Unix()
	payload := fmt.Sprintf("%d|%s", expiry, path)

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)

	// Token format: base64url(expiry.signature)
	token := base64.RawURLEncoding.EncodeToString(
		[]byte(fmt.Sprintf("%d.%s", expiry, base64.RawURLEncoding.EncodeToString(sig))),
	)

	sep := "?"
	if query != "" {
		sep = "&"
		return path + "?" + query + sep + "tok=" + token
	}
	return rawURL + sep + "tok=" + token
}

// validateToken checks the tok= query parameter against the request path.
// Returns true if the token is valid and not expired, or if auth is not enabled.
func (s *Server) validateToken(r *http.Request) bool {
	_, key := s.getAuthState()
	if len(key) == 0 {
		// No signing key means auth isn't set up; allow through
		return true
	}

	tok := r.URL.Query().Get("tok")
	if tok == "" {
		return false
	}

	// Decode the outer base64
	decoded, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return false
	}

	// Split into expiry and signature parts
	parts := strings.SplitN(string(decoded), ".", 2)
	if len(parts) != 2 {
		return false
	}

	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}

	if time.Now().Unix() > expiry {
		return false
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	// Reconstruct the expected payload and verify
	payload := fmt.Sprintf("%d|%s", expiry, r.URL.Path)
	expected := hmac.New(sha256.New, key)
	expected.Write([]byte(payload))

	return hmac.Equal(sigBytes, expected.Sum(nil))
}
