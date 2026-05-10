package transport

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
)

// AuthHandlers handles unauthenticated identity endpoints.
type AuthHandlers struct {
	svc      *application.IdentityService
	sessions sessionauth.Manager
	cookies  CookieConfig
}

// CookieConfig controls cookie names and flags written by handlers.
type CookieConfig struct {
	SessionName  string
	CSRFName     string
	SecurePrefix bool
	Domain       string
}

// NewAuthHandlers builds AuthHandlers.
func NewAuthHandlers(svc *application.IdentityService, sessions sessionauth.Manager, cookies CookieConfig) *AuthHandlers {
	return &AuthHandlers{svc: svc, sessions: sessions, cookies: cookies}
}

// RegisterAuthRoutes wires routes onto r.
func (h *AuthHandlers) RegisterAuthRoutes(r chi.Router) {
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/verify-email", h.VerifyEmail)
	r.Post("/auth/verify-email/resend", h.ResendVerifyEmail)
	r.Post("/auth/password-reset/request", h.RequestPasswordReset)
	r.Post("/auth/password-reset/confirm", h.ConfirmPasswordReset)
	r.Get("/auth/csrf", h.CSRF)
}

// --- handlers ---

type registerInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var in registerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	user, err := h.svc.Register(r.Context(), application.RegisterInput{
		Email: strings.TrimSpace(in.Email), Password: in.Password, Name: strings.TrimSpace(in.Name),
	})
	if err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	responsex.JSON(w, http.StatusCreated, userResponse(user))
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember bool   `json:"remember,omitempty"`
}

func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var in loginInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	user, err := h.svc.Login(r.Context(), application.LoginInput{
		Email: strings.TrimSpace(in.Email), Password: in.Password,
	})
	if err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	sess, err := h.sessions.Create(r.Context(), sessionauth.CreateParams{
		UserID:     user.ID,
		RememberMe: in.Remember,
		UserAgent:  r.UserAgent(),
		IP:         r.RemoteAddr,
	})
	if err != nil {
		responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "session creation failed", err)
		return
	}
	h.setSessionCookies(w, sess)
	responsex.JSON(w, http.StatusOK, userResponse(user))
}

type verifyEmailInput struct {
	Token string `json:"token"`
}

func (h *AuthHandlers) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var in verifyEmailInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), in.Token); err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

type resendVerifyInput struct {
	Email string `json:"email"`
}

func (h *AuthHandlers) ResendVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var in resendVerifyInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		// even on bad payload, return 202 to preserve anti-enumeration. Log internally.
		responsex.ErrorWithCause(w, r, http.StatusAccepted, "accepted", "accepted", err)
		return
	}
	// Best-effort send; service swallows unknown emails.
	_ = h.svc.ResendVerifyEmail(r.Context(), strings.TrimSpace(in.Email))
	w.WriteHeader(http.StatusAccepted)
}

type passwordResetRequestInput struct {
	Email string `json:"email"`
}

func (h *AuthHandlers) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var in passwordResetRequestInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	_ = h.svc.RequestPasswordReset(r.Context(), strings.TrimSpace(in.Email))
	w.WriteHeader(http.StatusAccepted)
}

type passwordResetConfirmInput struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (h *AuthHandlers) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var in passwordResetConfirmInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	if err := h.svc.ConfirmPasswordReset(r.Context(), in.Token, in.NewPassword); err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// CSRF returns a fresh CSRF token in a cookie. Used by SPA bootstrap before login.
// The cookie value is a random 32-byte hex; not bound to a session because there
// isn't one yet. Once the user logs in, the session-bound csrf_token replaces it.
func (h *AuthHandlers) CSRF(w http.ResponseWriter, r *http.Request) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "csrf gen failed", err)
		return
	}
	value := hex.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookies.CSRFName,
		Value:    value,
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(time.Hour.Seconds()),
	})
	responsex.JSON(w, http.StatusOK, map[string]string{"csrf_token": value})
}

// setSessionCookies writes the session_id and csrf_token cookies.
func (h *AuthHandlers) setSessionCookies(w http.ResponseWriter, s sessionauth.Session) {
	maxAge := int(time.Until(s.ExpiresAt).Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookies.SessionName,
		Value:    s.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookies.CSRFName,
		Value:    s.CSRFToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// Logout deletes the current session and clears cookies. Mounted on the
// authenticated branch (handler signature lives here for cohesion).
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	if err := h.sessions.Delete(r.Context(), sess.ID); err != nil {
		responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "logout failed", err)
		return
	}
	clearCookie(w, h.cookies.SessionName)
	clearCookie(w, h.cookies.CSRFName)
	w.WriteHeader(http.StatusNoContent)
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// helper: keep the linter happy if uuid/errors are not yet referenced in early skeletons.
var _ = uuid.Nil
var _ = errors.Is
var _ domain.UserStatus = ""

