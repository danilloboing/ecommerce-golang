package transport

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
)

// statusForError maps payment domain errors to HTTP status codes.
func statusForError(err error) int {
	if errors.Is(err, domain.ErrInvalidSignature) {
		return http.StatusUnauthorized
	}
	return http.StatusInternalServerError
}
