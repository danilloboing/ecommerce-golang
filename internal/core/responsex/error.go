// Package responsex centralises HTTP error responses across modules.
package responsex

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
)

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// WriteError maps domain or application errors to JSON HTTP responses.
// Unknown errors are rendered as 500 to avoid leaking internals.
func WriteError(w http.ResponseWriter, err error) {
	status, code := classify(err)

	body := errorBody{}
	body.Error.Code = code
	body.Error.Message = err.Error()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func classify(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrInvalidProduct),
		errors.Is(err, domain.ErrInvalidCategory),
		errors.Is(err, domain.ErrInvalidCurrency),
		errors.Is(err, domain.ErrNegativeAmount),
		errors.Is(err, domain.ErrInvalidSlug),
		errors.Is(err, domain.ErrDuplicateSKU),
		errors.Is(err, domain.ErrCurrencyMismatch),
		errors.Is(err, application.ErrBlankSearchQuery):
		return http.StatusBadRequest, "invalid_request"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}
