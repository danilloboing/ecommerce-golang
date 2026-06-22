package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
)

// MeHandlers handles authenticated identity endpoints.
type MeHandlers struct {
	svc      *application.IdentityService
	sessions sessionauth.Manager
	cookies  CookieConfig
}

// NewMeHandlers builds MeHandlers.
func NewMeHandlers(svc *application.IdentityService, sessions sessionauth.Manager, cookies CookieConfig) *MeHandlers {
	return &MeHandlers{svc: svc, sessions: sessions, cookies: cookies}
}

// RegisterMeRoutes wires routes onto r. The caller wraps r with sessionauth + csrf middlewares.
func (h *MeHandlers) RegisterMeRoutes(r chi.Router) {
	r.Get("/me", h.GetMe)
	r.Patch("/me", h.UpdateProfile)
	r.Post("/me/change-password", h.ChangePassword)
	r.Post("/auth/logout", h.Logout)
	r.Delete("/auth/sessions/all", h.DeleteAllSessions)
}

// GetMe returns the currently authenticated user.
func (h *MeHandlers) GetMe(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	user, err := h.svc.GetMe(r.Context(), sess.UserID)
	if err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	responsex.JSON(w, http.StatusOK, userResponse(user))
}

type updateProfileInput struct {
	Name *string `json:"name,omitempty"`
}

// UpdateProfile applies a partial profile update for the current user.
func (h *MeHandlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	var in updateProfileInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	if in.Name == nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "no fields to update")
		return
	}
	user, err := h.svc.UpdateProfile(r.Context(), application.UpdateProfileInput{
		UserID: sess.UserID,
		Name:   strings.TrimSpace(*in.Name),
	})
	if err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	responsex.JSON(w, http.StatusOK, userResponse(user))
}

type changePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword rotates the password for the current user, keeping the
// caller's session alive while every other session is revoked.
func (h *MeHandlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	var in changePasswordInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	if err := h.svc.ChangePassword(r.Context(), application.ChangePasswordInput{
		UserID:          sess.UserID,
		CurrentPassword: in.CurrentPassword,
		NewPassword:     in.NewPassword,
		KeepSessionID:   sess.ID,
	}); err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Logout deletes the current session and clears auth cookies.
func (h *MeHandlers) Logout(w http.ResponseWriter, r *http.Request) {
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

// DeleteAllSessions revokes every session for the current user (including the
// caller) and clears cookies on this device.
func (h *MeHandlers) DeleteAllSessions(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	if err := h.sessions.DeleteAllForUser(r.Context(), sess.UserID); err != nil {
		responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "session purge failed", err)
		return
	}
	clearCookie(w, h.cookies.SessionName)
	clearCookie(w, h.cookies.CSRFName)
	w.WriteHeader(http.StatusNoContent)
}
