package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// QuoteLine is one locked line inside a persisted Quote.
type QuoteLine struct {
	VariantID       uuid.UUID
	Quantity        int
	UnitPriceCents  int64
	ProductSnapshot json.RawMessage
}

// Quote is the persisted result of a pricing snapshot for a user's cart.
// It locks line prices, chosen shipping, and discount at quote time.
type Quote struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	CartFingerprint  string
	Lines            []QuoteLine
	CouponCode       string
	AddressSnapshot  json.RawMessage
	ShippingSnapshot json.RawMessage
	Subtotal         int64
	Shipping         int64
	Discount         int64
	Total            int64
	ExpiresAt        time.Time
	CreatedAt        time.Time
}
