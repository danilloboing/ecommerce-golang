// Package domain holds the catalog bounded context's pure domain types.
package domain

import "errors"

// ErrNotFound is returned when a domain entity cannot be located.
var ErrNotFound = errors.New("catalog: not found")

// ErrInvalidCurrency is returned when currency code is missing or not ISO 4217.
var ErrInvalidCurrency = errors.New("catalog: invalid currency")

// ErrNegativeAmount is returned when a Money amount is negative.
var ErrNegativeAmount = errors.New("catalog: negative amount")

// ErrCurrencyMismatch is returned when two Money values with different
// currencies are combined.
var ErrCurrencyMismatch = errors.New("catalog: currency mismatch")

// ErrInvalidSlug is returned when a slug is empty or contains invalid characters.
var ErrInvalidSlug = errors.New("catalog: invalid slug")
