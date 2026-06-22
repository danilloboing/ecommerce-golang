package domain

import (
	"time"

	"github.com/google/uuid"
)

// Quote is the persisted result of a pricing snapshot for a user's cart.
// It locks line prices, chosen shipping, and discount at quote time.
type Quote struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	CartFingerprint string
	Subtotal        int64
	Shipping        int64
	Discount        int64
	Total           int64
	ExpiresAt       time.Time
	CreatedAt       time.Time
}
