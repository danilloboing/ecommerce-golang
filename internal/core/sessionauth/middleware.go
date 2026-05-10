package sessionauth

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
)

// CookieName is the cookie carrying the session id. The Cookies.SecurePrefix
// flag in config controls whether `__Secure-` is prepended at write time.
const CookieName = "session_id"

// Middleware reads the session_id cookie, looks up the session, refreshes its
// activity timestamp, and injects Session into the request context.
func Middleware(mgr Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			if err != nil || cookie.Value == "" {
				responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
				return
			}

			sess, err := mgr.Get(r.Context(), cookie.Value)
			switch {
			case errors.Is(err, ErrNotFound), errors.Is(err, ErrExpired):
				clearCookie(w, CookieName)
				responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
				return
			case err != nil:
				responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "session lookup failed", err)
				return
			}

			// Best-effort refresh; not fatal if it fails.
			_ = mgr.Refresh(r.Context(), sess.ID)

			ctx := ContextWithSession(r.Context(), sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireVerifiedEmail composes after Middleware and rejects sessions whose
// user has not verified their email. It re-reads the session and assumes the
// caller injected it. The flag itself is enforced earlier (login refuses
// unverified users), so this exists as defense in depth for routes that may
// be reached otherwise.
//
// In Phase 2a it has no required wiring (login already enforces). It will be
// applied to checkout in Phase 3.
func RequireVerifiedEmail(check func(userID string) (bool, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := SessionFromContext(r.Context())
			if !ok {
				responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
				return
			}
			verified, err := check(sess.UserID.String())
			if err != nil {
				responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "verify check failed", err)
				return
			}
			if !verified {
				responsex.Error(w, r, http.StatusForbidden, "email_not_verified", "email verification required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
