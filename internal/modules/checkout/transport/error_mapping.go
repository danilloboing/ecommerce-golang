package transport

import (
	"errors"

	addrdomain "github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
	checkoutdomain "github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

// mapErrorToHTTP returns (status, code, userMessage) for a service error.
// Internal errors collapse to 500 + internal_error.
func mapErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, checkoutdomain.ErrCartEmpty):
		return 422, "cart_empty", "cart is empty"
	case errors.Is(err, checkoutdomain.ErrQuoteExpired), errors.Is(err, checkoutdomain.ErrQuoteNotFound):
		return 409, "quote_expired", "quote expired — re-quote"
	case errors.Is(err, checkoutdomain.ErrCartChanged):
		return 409, "cart_changed", "cart changed — re-quote"
	case errors.Is(err, checkoutdomain.ErrInsufficientStock):
		return 422, "insufficient_stock", "insufficient stock"
	case errors.Is(err, checkoutdomain.ErrCouponInvalid):
		return 422, "coupon_invalid", "coupon invalid"
	case errors.Is(err, checkoutdomain.ErrCouponUnavailable):
		return 422, "coupon_unavailable", "coupon unavailable"
	case errors.Is(err, checkoutdomain.ErrIdempotencyConflict):
		return 409, "idempotency_conflict", "idempotency key reused with a different request"
	case errors.Is(err, addrdomain.ErrAddressNotFound):
		return 404, "not_found", "address not found"
	default:
		return 500, "internal_error", "internal error"
	}
}
