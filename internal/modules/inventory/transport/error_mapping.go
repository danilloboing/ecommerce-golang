package transport

import (
	"errors"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrStockConflict):
		return http.StatusConflict, "stock_conflict", "stock version conflict"
	case errors.Is(err, domain.ErrStockNotFound):
		return http.StatusNotFound, "not_found", "stock not found"
	case errors.Is(err, domain.ErrInsufficientStock):
		return http.StatusUnprocessableEntity, "insufficient_stock", "insufficient stock"
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}
