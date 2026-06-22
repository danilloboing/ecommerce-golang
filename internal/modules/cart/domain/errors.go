package domain

import "errors"

// Sentinel errors for the cart bounded context.
var (
	ErrCartNotFound    = errors.New("cart: not found")
	ErrItemNotFound    = errors.New("cart: item not found")
	ErrInvalidQuantity = errors.New("cart: invalid quantity")
	ErrVariantNotFound = errors.New("cart: variant not found")
)
