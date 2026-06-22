package transport

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

// mapErrorToHTTP returns (status, code, userMessage) for a service error.
// Internal errors collapse to 500 + internal_error.
func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidAddress):
		return http.StatusUnprocessableEntity, "invalid_address", "invalid address data"
	case errors.Is(err, domain.ErrAddressNotFound):
		return http.StatusNotFound, "not_found", "address not found"
	case errors.Is(err, domain.ErrInvalidCEP):
		return http.StatusBadRequest, "invalid_cep", "invalid cep"
	case errors.Is(err, domain.ErrCEPNotFound):
		return http.StatusNotFound, "cep_not_found", "cep not found"
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}
