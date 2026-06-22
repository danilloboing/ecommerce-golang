package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

// ---------------------------------------------------------------------------
// Inbound / view types
// ---------------------------------------------------------------------------

// CartLine represents a single item line in the user's active cart.
type CartLine struct {
	VariantID uuid.UUID
	Quantity  int
}

// CartView is a checkout-local projection of the active cart.
// The cart adapter (Task 18) maps the cart domain aggregate → CartView.
type CartView struct {
	Lines []CartLine
}

// ShippingOption is a single carrier service quote.
type ShippingOption struct {
	ServiceID  string
	Name       string
	PriceCents int64
	ETADays    int
}

// AddressView is a checkout-local projection of a user address.
type AddressView struct {
	PostalCode string
	Snapshot   json.RawMessage
}

// ---------------------------------------------------------------------------
// Input / output DTOs
// ---------------------------------------------------------------------------

// QuoteInput carries the parameters needed to compute a pricing quote.
type QuoteInput struct {
	UserID     uuid.UUID
	AddressID  uuid.UUID
	ServiceID  string // optional; defaults to cheapest option when empty
	CouponCode string // optional
}

// QuoteLine is one locked line inside a QuoteResult.
type QuoteLine struct {
	VariantID      uuid.UUID
	Quantity       int
	UnitPriceCents int64
}

// QuoteResult is the computed and persisted quote returned to callers.
type QuoteResult struct {
	QuoteID   uuid.UUID
	Lines     []QuoteLine
	Options   []ShippingOption
	Chosen    ShippingOption
	Subtotal  int64
	Shipping  int64
	Discount  int64
	Total     int64
	ExpiresAt time.Time
}

// NewQuote carries all fields required to persist a checkout_quotes row.
type NewQuote struct {
	UserID          uuid.UUID
	CartFingerprint string
	Lines           []QuoteLine
	Chosen          ShippingOption
	CouponCode      string
	Subtotal        int64
	Shipping        int64
	Discount        int64
	Total           int64
	ExpiresAt       time.Time
}

// ---------------------------------------------------------------------------
// Port interfaces
// ---------------------------------------------------------------------------

// CartReader fetches the active cart for a user as a checkout-local view.
// Implemented by the cart infrastructure adapter (Task 18).
type CartReader interface {
	ActiveCart(ctx context.Context, userID uuid.UUID) (CartView, error)
}

// PriceReader provides authoritative unit prices for product variants.
// Price always comes from this reader — clients never supply prices (C3).
type PriceReader interface {
	UnitPrice(ctx context.Context, variantID uuid.UUID) (int64, error)
}

// ShippingQuoter returns available shipping options for a postal code and subtotal.
type ShippingQuoter interface {
	Quote(ctx context.Context, postalCode string, subtotalCents int64) ([]ShippingOption, error)
}

// AddressReader fetches a user address and verifies ownership.
type AddressReader interface {
	Get(ctx context.Context, addressID, userID uuid.UUID) (AddressView, error)
}

// QuoteRepository is the persistence contract for checkout_quotes rows.
type QuoteRepository interface {
	Create(ctx context.Context, in NewQuote) (domain.Quote, error)
	GetUserQuote(ctx context.Context, id, userID uuid.UUID) (domain.Quote, error)
}

// CouponValidator validates a coupon code against a subtotal and returns the
// discount in cents. Implemented by *CouponService (Task 15).
type CouponValidator interface {
	Validate(ctx context.Context, code string, subtotalCents int64) (int64, error)
}
