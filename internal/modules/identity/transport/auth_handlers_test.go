package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakes shared with application tests would normally live in a testutil pkg.
// For brevity, redeclare minimal in-memory fakes here.

type memUserRepo struct {
	users map[string]domain.User
}

func newMemUserRepo() *memUserRepo { return &memUserRepo{users: map[string]domain.User{}} }
func (r *memUserRepo) Insert(_ context.Context, e, n string) (domain.User, error) {
	if _, ok := r.users[strings.ToLower(e)]; ok {
		return domain.User{}, domain.ErrEmailAlreadyTaken
	}
	u := domain.User{ID: uuid.New(), Email: e, Name: n, Status: domain.UserStatusActive}
	r.users[strings.ToLower(e)] = u
	return u, nil
}
func (r *memUserRepo) FindByID(_ context.Context, id uuid.UUID) (domain.User, error) {
	for _, u := range r.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}
func (r *memUserRepo) FindByEmail(_ context.Context, e string) (domain.User, error) {
	if u, ok := r.users[strings.ToLower(e)]; ok {
		return u, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}
func (r *memUserRepo) MarkEmailVerified(_ context.Context, id uuid.UUID) error {
	for k, u := range r.users {
		if u.ID == id {
			now := time.Now().UTC()
			u.EmailVerifiedAt = &now
			r.users[k] = u
		}
	}
	return nil
}
func (r *memUserRepo) UpdateName(_ context.Context, id uuid.UUID, name string) (domain.User, error) {
	for k, u := range r.users {
		if u.ID == id {
			u.Name = name
			r.users[k] = u
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

type memAuthRepo struct {
	byUser map[uuid.UUID]domain.AuthMethod
}

func newMemAuthRepo() *memAuthRepo { return &memAuthRepo{byUser: map[uuid.UUID]domain.AuthMethod{}} }
func (r *memAuthRepo) InsertPassword(_ context.Context, uid uuid.UUID, hash string) (domain.AuthMethod, error) {
	am := domain.AuthMethod{ID: uuid.New(), UserID: uid, Provider: domain.AuthProviderPassword, PasswordHash: &hash}
	r.byUser[uid] = am
	return am, nil
}
func (r *memAuthRepo) FindForUser(_ context.Context, uid uuid.UUID, _ domain.AuthProvider) (domain.AuthMethod, error) {
	am, ok := r.byUser[uid]
	if !ok {
		return domain.AuthMethod{}, domain.ErrUserNotFound
	}
	return am, nil
}
func (r *memAuthRepo) UpdatePassword(_ context.Context, uid uuid.UUID, hash string) error {
	am := r.byUser[uid]
	am.PasswordHash = &hash
	r.byUser[uid] = am
	return nil
}
func (r *memAuthRepo) TouchLastUsed(_ context.Context, _ uuid.UUID) error { return nil }

type memVerifyRepo struct {
	byHash map[string]domain.EmailVerifyToken
}

func newMemVerifyRepo() *memVerifyRepo {
	return &memVerifyRepo{byHash: map[string]domain.EmailVerifyToken{}}
}
func (r *memVerifyRepo) Insert(_ context.Context, h []byte, uid uuid.UUID, e string, exp time.Time) error {
	r.byHash[string(h)] = domain.EmailVerifyToken{
		TokenHash: h, UserID: uid, Email: e, ExpiresAt: exp,
	}
	return nil
}
func (r *memVerifyRepo) Find(_ context.Context, h []byte) (domain.EmailVerifyToken, error) {
	t, ok := r.byHash[string(h)]
	if !ok {
		return domain.EmailVerifyToken{}, domain.ErrTokenNotFound
	}
	return t, nil
}
func (r *memVerifyRepo) Consume(_ context.Context, h []byte) error {
	t, ok := r.byHash[string(h)]
	if !ok {
		return domain.ErrTokenNotFound
	}
	now := time.Now().UTC()
	t.ConsumedAt = &now
	r.byHash[string(h)] = t
	return nil
}

type memResetRepo struct{}

func (memResetRepo) Insert(context.Context, []byte, uuid.UUID, time.Time) error {
	return nil
}
func (memResetRepo) Find(context.Context, []byte) (domain.PasswordResetToken, error) {
	return domain.PasswordResetToken{}, domain.ErrTokenNotFound
}
func (memResetRepo) Consume(context.Context, []byte) error { return nil }

type captureSender struct{ msgs []email.Message }

func (s *captureSender) Send(_ context.Context, m email.Message) error {
	s.msgs = append(s.msgs, m)
	return nil
}

type stubSessions struct {
	created sessionauth.Session
	deleted []string
}

func (s *stubSessions) Create(_ context.Context, _ sessionauth.CreateParams) (sessionauth.Session, error) {
	s.created = sessionauth.Session{
		ID: "sid", UserID: uuid.New(), CSRFToken: "ct",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	return s.created, nil
}
func (s *stubSessions) Get(context.Context, string) (sessionauth.Session, error) {
	return sessionauth.Session{}, sessionauth.ErrNotFound
}
func (s *stubSessions) Refresh(context.Context, string) error { return nil }
func (s *stubSessions) Delete(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubSessions) DeleteAllForUser(context.Context, uuid.UUID) error               { return nil }
func (s *stubSessions) DeleteAllForUserExcept(context.Context, uuid.UUID, string) error { return nil }

func newSvc(users application.UserRepository, auths application.AuthMethodRepository, verify application.EmailVerifyTokenRepository, reset application.PasswordResetTokenRepository, sender email.Sender) *application.IdentityService {
	return application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: auths,
		VerifyTokens: verify, ResetTokens: reset, Email: sender,
		VerifyLinkBaseURL: "https://app/verify",
		ResetLinkBaseURL:  "https://app/reset",
		Now:               time.Now,
	})
}

func TestRegisterHandler_HappyPath(t *testing.T) {
	users := newMemUserRepo()
	auths := newMemAuthRepo()
	verify := newMemVerifyRepo()
	sender := &captureSender{}
	svc := newSvc(users, auths, verify, memResetRepo{}, sender)

	h := transport.NewAuthHandlers(svc, &stubSessions{}, transport.CookieConfig{
		SessionName: "session_id", CSRFName: "csrf_token",
	})
	r := chi.NewRouter()
	h.RegisterAuthRoutes(r)

	body, _ := json.Marshal(map[string]string{
		"email": "ana@example.com", "password": "S3cretPass!", "name": "Ana",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Len(t, sender.msgs, 1)
	assert.Contains(t, sender.msgs[0].TextBody, "https://app/verify?token=")
}

func TestLoginHandler_SetsSessionCookies(t *testing.T) {
	users := newMemUserRepo()
	auths := newMemAuthRepo()
	verify := newMemVerifyRepo()
	sender := &captureSender{}
	svc := newSvc(users, auths, verify, memResetRepo{}, sender)
	sessions := &stubSessions{}

	// Bootstrap user via real Register flow to keep password hashes in sync.
	h := transport.NewAuthHandlers(svc, sessions, transport.CookieConfig{
		SessionName: "session_id", CSRFName: "csrf_token",
	})
	r := chi.NewRouter()
	h.RegisterAuthRoutes(r)

	body, _ := json.Marshal(map[string]string{
		"email": "ana@example.com", "password": "S3cretPass!", "name": "Ana",
	})
	registerReq := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(string(body)))
	registerReq.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), registerReq)

	// Verify email out-of-band — flip via mem repo to keep test focused on cookies.
	for _, u := range users.users {
		require.NoError(t, users.MarkEmailVerified(context.Background(), u.ID))
	}

	loginBody, _ := json.Marshal(map[string]string{
		"email": "ana@example.com", "password": "S3cretPass!",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(string(loginBody)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	cookies := rec.Result().Cookies()
	var session, csrf *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case "session_id":
			session = c
		case "csrf_token":
			csrf = c
		}
	}
	require.NotNil(t, session)
	require.NotNil(t, csrf)
	assert.Equal(t, "sid", session.Value)
	assert.True(t, session.HttpOnly)
	assert.False(t, csrf.HttpOnly)
}
