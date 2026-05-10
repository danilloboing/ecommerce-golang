package sessionauth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubManager struct {
	getResp      sessionauth.Session
	getErr       error
	refreshErr   error
	deleteErr    error
	getCalls     int
	refreshCalls int
}

func (s *stubManager) Create(context.Context, sessionauth.CreateParams) (sessionauth.Session, error) {
	return sessionauth.Session{}, errors.New("not used")
}
func (s *stubManager) Get(_ context.Context, id string) (sessionauth.Session, error) {
	s.getCalls++
	return s.getResp, s.getErr
}
func (s *stubManager) Refresh(_ context.Context, id string) error {
	s.refreshCalls++
	return s.refreshErr
}
func (s *stubManager) Delete(context.Context, string) error                            { return s.deleteErr }
func (s *stubManager) DeleteAllForUser(context.Context, uuid.UUID) error               { return nil }
func (s *stubManager) DeleteAllForUserExcept(context.Context, uuid.UUID, string) error { return nil }

func TestMiddleware_AttachesSessionWhenCookieValid(t *testing.T) {
	mgr := &stubManager{getResp: sessionauth.Session{
		ID: "sid", UserID: uuid.New(), CSRFToken: "ct",
		LastActivityAt: time.Now(),
	}}

	called := false
	handler := sessionauth.Middleware(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		s, ok := sessionauth.SessionFromContext(r.Context())
		require.True(t, ok)
		assert.Equal(t, "sid", s.ID)
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.AddCookie(&http.Cookie{Name: "session_id", Value: "sid"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, called)
	assert.Equal(t, 1, mgr.getCalls)
	assert.Equal(t, 1, mgr.refreshCalls)
}

func TestMiddleware_Returns401WhenCookieMissing(t *testing.T) {
	mgr := &stubManager{}
	handler := sessionauth.Middleware(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner should not run")
	}))

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddleware_Returns401WhenSessionNotFound(t *testing.T) {
	mgr := &stubManager{getErr: sessionauth.ErrNotFound}
	handler := sessionauth.Middleware(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner should not run")
	}))

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.AddCookie(&http.Cookie{Name: "session_id", Value: "missing"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Cookie should be cleared (Set-Cookie with Max-Age=0).
	cookies := rec.Result().Cookies()
	require.NotEmpty(t, cookies)
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "session_id" {
			found = c
			break
		}
	}
	require.NotNil(t, found)
	assert.True(t, found.MaxAge < 0 || found.MaxAge == 0 && found.Value == "")
}
