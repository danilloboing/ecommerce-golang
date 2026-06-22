package domain

import "errors"

// Sentinel errors for the payment bounded context.
var (
	ErrInvalidSignature = errors.New("payment: invalid signature")
	ErrChargeNotFound   = errors.New("payment: charge not found")
	ErrAmountMismatch   = errors.New("payment: amount mismatch")
)
