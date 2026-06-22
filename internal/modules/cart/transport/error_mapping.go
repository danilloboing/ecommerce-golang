package transport

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

// mapErrorToHTTP returns (status, code, message) for a cart error.
func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidQuantity):
		return http.StatusUnprocessableEntity, "invalid_quantity", "quantity must be between 1 and 99"
	case errors.Is(err, domain.ErrVariantNotFound):
		return http.StatusNotFound, "variant_not_found", "variant not found"
	case errors.Is(err, domain.ErrItemNotFound):
		return http.StatusNotFound, "item_not_found", "cart item not found"
	case errors.Is(err, domain.ErrCartNotFound):
		return http.StatusNotFound, "cart_not_found", "cart not found"
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}
