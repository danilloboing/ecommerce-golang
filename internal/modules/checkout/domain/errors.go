// Package domain holds checkout value types, invariants, and sentinel errors.
package domain

import "errors"

// Sentinel errors for the checkout bounded context.
var (
	ErrCouponInvalid       = errors.New("checkout: coupon invalid")
	ErrCouponUnavailable   = errors.New("checkout: coupon unavailable")
	ErrQuoteExpired        = errors.New("checkout: quote expired")
	ErrQuoteNotFound       = errors.New("checkout: quote not found")
	ErrCartChanged         = errors.New("checkout: cart changed since quote")
	ErrCartEmpty           = errors.New("checkout: cart is empty")
	ErrIdempotencyConflict = errors.New("checkout: idempotency conflict")
	ErrInsufficientStock   = errors.New("checkout: insufficient stock")
)
