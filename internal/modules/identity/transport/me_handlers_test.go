package transport_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withSession(s sessionauth.Session) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := sessionauth.ContextWithSession(r.Context(), s)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TestMeHandler_GetMeReturnsCurrentUser(t *testing.T) {
	users := newMemUserRepo()
	auths := newMemAuthRepo()
	verify := newMemVerifyRepo()
	sender := &captureSender{}
	svc := newSvc(users, auths, verify, memResetRepo{}, sender)
	sessions := &stubSessions{}

	// Seed a verified user.
	u, err := users.Insert(t.Context(), "ana@example.com", "Ana")
	require.NoError(t, err)
	require.NoError(t, users.MarkEmailVerified(t.Context(), u.ID))

	h := transport.NewMeHandlers(svc, sessions, transport.CookieConfig{
		SessionName: "session_id", CSRFName: "csrf_token",
	})
	r := chi.NewRouter()
	r.Group(func(grp chi.Router) {
		grp.Use(withSession(sessionauth.Session{ID: "sid", UserID: u.ID, CSRFToken: "ct"}))
		h.RegisterMeRoutes(grp)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body transport.UserResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, u.ID, body.ID)
	assert.Equal(t, "Ana", body.Name)
}

func TestMeHandler_PatchMeUpdatesName(t *testing.T) {
	users := newMemUserRepo()
	auths := newMemAuthRepo()
	verify := newMemVerifyRepo()
	sender := &captureSender{}
	svc := newSvc(users, auths, verify, memResetRepo{}, sender)
	sessions := &stubSessions{}

	u, err := users.Insert(t.Context(), "ana@example.com", "Ana")
	require.NoError(t, err)

	h := transport.NewMeHandlers(svc, sessions, transport.CookieConfig{SessionName: "session_id", CSRFName: "csrf_token"})
	r := chi.NewRouter()
	r.Group(func(grp chi.Router) {
		grp.Use(withSession(sessionauth.Session{ID: "sid", UserID: u.ID, CSRFToken: "ct"}))
		h.RegisterMeRoutes(grp)
	})

	body, _ := json.Marshal(map[string]string{"name": "Ana Lima"})
	req := httptest.NewRequest(http.MethodPatch, "/me", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got transport.UserResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "Ana Lima", got.Name)
}

func TestMeHandler_DeleteAllSessions(t *testing.T) {
	users := newMemUserRepo()
	auths := newMemAuthRepo()
	verify := newMemVerifyRepo()
	sender := &captureSender{}
	svc := newSvc(users, auths, verify, memResetRepo{}, sender)
	sessions := &stubSessions{}

	uid := uuid.New()
	h := transport.NewMeHandlers(svc, sessions, transport.CookieConfig{SessionName: "session_id", CSRFName: "csrf_token"})
	r := chi.NewRouter()
	r.Group(func(grp chi.Router) {
		grp.Use(withSession(sessionauth.Session{ID: "sid", UserID: uid, CSRFToken: "ct"}))
		h.RegisterMeRoutes(grp)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/auth/sessions/all", nil))
	require.Equal(t, http.StatusNoContent, rec.Code)
}
