package transport

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
)

// mapErrorToHTTP returns (status, code, userMessage) for a service error.
// Internal errors collapse to 500 + internal_error.
func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrOrderNotFound):
		return http.StatusNotFound, "not_found", "order not found"
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}
