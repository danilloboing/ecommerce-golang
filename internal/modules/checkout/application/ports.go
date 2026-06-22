package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
	orderingDomain "github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
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
// Deprecated: prefer domain.QuoteLine. Kept for backward compatibility.
type QuoteLine = domain.QuoteLine

// QuoteResult is the computed and persisted quote returned to callers.
type QuoteResult struct {
	QuoteID   uuid.UUID
	Lines     []domain.QuoteLine
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
	UserID           uuid.UUID
	CartFingerprint  string
	Lines            []domain.QuoteLine
	Chosen           ShippingOption
	CouponCode       string
	AddressSnapshot  json.RawMessage
	ShippingSnapshot json.RawMessage
	Subtotal         int64
	Shipping         int64
	Discount         int64
	Total            int64
	ExpiresAt        time.Time
}

// ConfirmInput carries the parameters needed to confirm an order from a quote.
type ConfirmInput struct {
	UserID         uuid.UUID
	QuoteID        uuid.UUID
	IdempotencyKey string
	PaymentMethod  string // e.g. "pix"
}

// ConfirmResult is the result of a successful order confirmation.
type ConfirmResult struct {
	Order  orderingDomain.Order
	Charge ChargeView
}

// ConfirmPlan is the full plan passed to ConfirmRepository.ConfirmTx for atomic execution.
type ConfirmPlan struct {
	UserID               uuid.UUID
	OrderID              uuid.UUID
	Quote                domain.Quote
	Cart                 CartView
	Charge               ChargeView
	IdempotencyKey       string
	RequestHash          string // sha256 of quoteID — idempotency request fingerprint
	ReservationExpiresAt time.Time
}

// IdemHit is the result of an idempotency key lookup.
type IdemHit struct {
	Found        bool
	Replay       bool          // true = same hash, return stored result
	Conflict     bool          // true = different hash, return ErrIdempotencyConflict
	StoredResult *ConfirmResult // non-nil if Replay=true
}

// ChargeView is a checkout-local projection of a payment charge.
type ChargeView struct {
	ChargeID  uuid.UUID
	OrderID   uuid.UUID
	Amount    int64
	Method    string
	Status    string
	CreatedAt time.Time
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

// ConfirmRepository handles the atomic confirm transaction.
// The implementation (Task 18) executes idempotency insert, stock reservation,
// coupon redemption, order+items creation, cart conversion, and charge persist
// all inside a single database transaction.
type ConfirmRepository interface {
	// ConfirmTx inserts the idempotency row, persisting its `response` column as
	// the JSON-marshaled ConfirmResult{Order, Charge} built from the order it
	// creates + the plan's Charge, so replays return the full result.
	// (Task 18 implements this.)
	ConfirmTx(ctx context.Context, plan ConfirmPlan) (orderingDomain.Order, error)
}

// Idempotency handles idempotency key lookups before the confirm transaction.
type Idempotency interface {
	Lookup(ctx context.Context, userID uuid.UUID, key string, requestHash string) (IdemHit, error)
}

// Charger creates payment charges with the payment provider.
type Charger interface {
	CreateCharge(ctx context.Context, orderID uuid.UUID, amount int64, method string) (ChargeView, error)
}
