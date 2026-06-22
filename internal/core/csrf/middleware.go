// Package csrf provides double-submit cookie + Origin check middleware.
package csrf

import (
	"crypto/subtle"
	"net/http"
	"slices"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
)

// HeaderName is the request header that must mirror the CSRF cookie.
const HeaderName = "X-CSRF-Token"

// Config controls the middleware.
type Config struct {
	AllowedOrigins []string
	CookieName     string // e.g. "csrf_token" or "__Secure-csrf_token"
}

// Middleware enforces double-submit + Origin for unsafe HTTP methods.
//
// On safe methods (GET/HEAD/OPTIONS) the request is passed through.
// On unsafe methods:
//  1. If Origin header is set, it MUST match cfg.AllowedOrigins.
//  2. The csrf_token cookie and X-CSRF-Token header MUST be present, equal,
//     and equal to the session's stored csrf_token (if a session is present).
func Middleware(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafe(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			if origin := r.Header.Get("Origin"); origin != "" {
				if !slices.Contains(cfg.AllowedOrigins, origin) {
					responsex.Error(w, r, http.StatusForbidden, "csrf_origin_invalid", "origin not allowed")
					return
				}
			}

			cookie, err := r.Cookie(cfg.CookieName)
			if err != nil || cookie.Value == "" {
				responsex.Error(w, r, http.StatusForbidden, "csrf_invalid", "csrf cookie missing")
				return
			}
			header := r.Header.Get(HeaderName)
			if header == "" {
				responsex.Error(w, r, http.StatusForbidden, "csrf_invalid", "csrf header missing")
				return
			}

			if !equal(cookie.Value, header) {
				responsex.Error(w, r, http.StatusForbidden, "csrf_invalid", "csrf token mismatch")
				return
			}

			if sess, ok := sessionauth.SessionFromContext(r.Context()); ok {
				if !equal(sess.CSRFToken, cookie.Value) {
					responsex.Error(w, r, http.StatusForbidden, "csrf_invalid", "csrf token does not match session")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isSafe(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
