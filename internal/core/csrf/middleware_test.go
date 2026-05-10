package csrf_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	allowedOrigin = "https://app.example"
	csrfCookie    = "csrf_token"
	csrfHeader    = "X-CSRF-Token"
)

func cfg() csrf.Config {
	return csrf.Config{
		AllowedOrigins: []string{allowedOrigin},
		CookieName:     csrfCookie,
	}
}

func mwWithSession(s sessionauth.Session) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := sessionauth.ContextWithSession(r.Context(), s)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCSRF_PassesGetRequests(t *testing.T) {
	h := csrf.Middleware(cfg())(okHandler())
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRF_RejectsMutationWithMismatchedOrigin(t *testing.T) {
	h := csrf.Middleware(cfg())(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRF_RejectsMutationWithoutHeader(t *testing.T) {
	h := csrf.Middleware(cfg())(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "abc"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRF_RejectsMutationWhenCookieAndHeaderDiffer(t *testing.T) {
	h := csrf.Middleware(cfg())(okHandler())
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set(csrfHeader, "abc")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "different"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRF_AcceptsMutationWhenAllChecksPass(t *testing.T) {
	sess := sessionauth.Session{ID: "sid", UserID: uuid.New(), CSRFToken: "abc"}
	h := mwWithSession(sess)(csrf.Middleware(cfg())(okHandler()))

	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set(csrfHeader, "abc")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "abc"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRF_RejectsMutationWhenSessionTokenDiffers(t *testing.T) {
	sess := sessionauth.Session{ID: "sid", UserID: uuid.New(), CSRFToken: "session-token"}
	h := mwWithSession(sess)(csrf.Middleware(cfg())(okHandler()))

	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set(csrfHeader, "abc")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "abc"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
