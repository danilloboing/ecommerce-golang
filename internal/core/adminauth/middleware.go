// Package adminauth provides static-token-based admin authentication.
package adminauth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

// RequireToken returns a middleware that requires an exact match against the
// configured admin API token. expectedToken MUST not be empty (constructor
// panics otherwise — startup misconfiguration must fail loudly).
func RequireToken(expectedToken string) func(next http.Handler) http.Handler {
	if expectedToken == "" {
		panic("adminauth: expectedToken must not be empty")
	}
	tokenBytes := []byte(expectedToken)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" || !strings.HasPrefix(header, bearerPrefix) {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			provided := []byte(strings.TrimPrefix(header, bearerPrefix))
			if subtle.ConstantTimeEq(int32(len(provided)), int32(len(tokenBytes))) != 1 ||
				subtle.ConstantTimeCompare(provided, tokenBytes) != 1 {
				http.Error(w, "invalid token", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
