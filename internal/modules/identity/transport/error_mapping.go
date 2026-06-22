package transport

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
)

// mapErrorToHTTP returns (status, code, userMessage) for a service error.
// Internal errors collapse to 500 + internal_error.
func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials),
		errors.Is(err, domain.ErrSessionExpired),
		errors.Is(err, domain.ErrSessionNotFound):
		return http.StatusUnauthorized, "invalid_credentials", "invalid credentials"
	case errors.Is(err, domain.ErrEmailNotVerified):
		return http.StatusForbidden, "email_not_verified", "email verification required"
	case errors.Is(err, domain.ErrEmailAlreadyTaken):
		return http.StatusConflict, "email_already_taken", "email already taken"
	case errors.Is(err, domain.ErrTokenExpired),
		errors.Is(err, domain.ErrTokenAlreadyUsed),
		errors.Is(err, domain.ErrTokenNotFound):
		return http.StatusBadRequest, "invalid_token", "invalid token"
	case errors.Is(err, domain.ErrPasswordTooWeak):
		return http.StatusUnprocessableEntity, "password_policy", "password does not meet policy"
	case errors.Is(err, domain.ErrUserNotFound):
		return http.StatusNotFound, "not_found", "user not found"
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}
