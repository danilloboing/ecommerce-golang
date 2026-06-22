// Package domain holds cart value types and invariants.
package domain

import "github.com/google/uuid"

// MaxItemQuantity is the hard per-line quantity cap (schema enforces the same).
const MaxItemQuantity = 99

// Status is a cart lifecycle state.
type Status string

// Cart lifecycle states.
const (
	StatusActive    Status = "active"
	StatusMerged    Status = "merged"
	StatusAbandoned Status = "abandoned"
	StatusConverted Status = "converted"
)

// CartItem is a single line in a cart with a price snapshot.
type CartItem struct {
	ID             uuid.UUID
	VariantID      uuid.UUID
	Quantity       int
	UnitPriceCents int64
}

// Cart is an active shopping cart owned by a user OR an anonymous session.
type Cart struct {
	ID            uuid.UUID
	UserID        *uuid.UUID
	AnonSessionID *string
	Status        Status
	Items         []CartItem
}

// SubtotalCents sums line totals (unit price × quantity) across all items.
func (c Cart) SubtotalCents() int64 {
	var total int64
	for _, it := range c.Items {
		total += it.UnitPriceCents * int64(it.Quantity)
	}
	return total
}

// ValidateQuantity enforces 1..MaxItemQuantity.
func ValidateQuantity(q int) error {
	if q < 1 || q > MaxItemQuantity {
		return ErrInvalidQuantity
	}
	return nil
}

// Owner identifies who a cart belongs to: exactly one of UserID or AnonID is set.
type Owner struct {
	UserID *uuid.UUID
	AnonID *string
}

// Valid reports whether exactly one identity is present.
func (o Owner) Valid() bool {
	return (o.UserID != nil) != (o.AnonID != nil)
}
