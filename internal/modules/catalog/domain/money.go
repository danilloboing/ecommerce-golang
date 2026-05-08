package domain

import (
	"fmt"
)

// Money represents a non-negative monetary amount in minor units (e.g. cents
// for BRL/USD/EUR) tagged with an ISO 4217 currency code.
type Money struct {
	amountCents int64
	currency    string
}

// NewMoney constructs a Money. amountCents must be >= 0 and currency must be a
// 3-letter ISO 4217 code.
func NewMoney(amountCents int64, currency string) (Money, error) {
	if len(currency) != 3 {
		return Money{}, ErrInvalidCurrency
	}
	if amountCents < 0 {
		return Money{}, ErrNegativeAmount
	}
	return Money{amountCents: amountCents, currency: currency}, nil
}

// AmountCents returns the amount in minor units.
func (m Money) AmountCents() int64 { return m.amountCents }

// Currency returns the ISO 4217 currency code.
func (m Money) Currency() string { return m.currency }

// Add returns the sum of two Money values, requiring matching currencies.
func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	return Money{amountCents: m.amountCents + other.amountCents, currency: m.currency}, nil
}

// String returns a human-readable representation, e.g. "BRL 99.90".
func (m Money) String() string {
	return fmt.Sprintf("%s %d.%02d", m.currency, m.amountCents/100, m.amountCents%100)
}
