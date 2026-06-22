// Package infrastructure wires checkout application ports to Postgres (via the
// shared sqlc layer) and to neighbouring bounded contexts through thin adapters.
package infrastructure

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	orderingDomain "github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// mockChargeProvider is the only payment provider wired in Phase 3a. Real
// providers arrive in Phase 5; until then charges are minted in-process.
const mockChargeProvider = "mock"

// ---------------------------------------------------------------------------
// CartReader adapter
// ---------------------------------------------------------------------------

// CartReaderAdapter reads a user's active cart from the shared data layer and
// projects it into checkout's CartView. It deliberately bypasses the cart
// module's infrastructure: checkout only needs the locked (variant, quantity)
// lines, not the full cart aggregate.
type CartReaderAdapter struct {
	q *queries.Queries
}

var _ application.CartReader = (*CartReaderAdapter)(nil)

// NewCartReaderAdapter builds a CartReaderAdapter from a pgx pool.
func NewCartReaderAdapter(pool *pgxpool.Pool) *CartReaderAdapter {
	return &CartReaderAdapter{q: queries.New(pool)}
}

// ActiveCart returns the user's active cart lines. A user with no active cart
// yields an empty view so the service layer can decide (an empty cart maps to
// ErrCartEmpty on quote, and to a fingerprint mismatch on confirm).
func (a *CartReaderAdapter) ActiveCart(ctx context.Context, userID uuid.UUID) (application.CartView, error) {
	uid := userID
	cart, err := a.q.GetActiveCartByUser(ctx, &uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.CartView{}, nil
	}
	if err != nil {
		return application.CartView{}, err
	}
	items, err := a.q.ListCartItems(ctx, cart.ID)
	if err != nil {
		return application.CartView{}, err
	}
	lines := make([]application.CartLine, 0, len(items))
	for _, it := range items {
		lines = append(lines, application.CartLine{
			VariantID: it.VariantID,
			Quantity:  int(it.Quantity),
		})
	}
	return application.CartView{Lines: lines}, nil
}

// ---------------------------------------------------------------------------
// AddressReader adapter
// ---------------------------------------------------------------------------

// AddressLookup fetches an owned address as a checkout AddressView. It is the
// seam onto the address bounded context, injected so checkout never imports
// address infrastructure directly.
type AddressLookup func(ctx context.Context, addressID, userID uuid.UUID) (application.AddressView, error)

// AddressReaderAdapter adapts an AddressLookup function into AddressReader.
type AddressReaderAdapter struct {
	lookup AddressLookup
}

var _ application.AddressReader = (*AddressReaderAdapter)(nil)

// NewAddressReaderAdapter builds an AddressReaderAdapter from a lookup func.
func NewAddressReaderAdapter(lookup AddressLookup) *AddressReaderAdapter {
	return &AddressReaderAdapter{lookup: lookup}
}

// Get fetches the address, delegating to the injected lookup.
func (a *AddressReaderAdapter) Get(ctx context.Context, addressID, userID uuid.UUID) (application.AddressView, error) {
	return a.lookup(ctx, addressID, userID)
}

// ---------------------------------------------------------------------------
// ShippingQuoter adapter
// ---------------------------------------------------------------------------

// ShippingLookup returns shipping options for a postal code and subtotal. It is
// the seam onto the shipping bounded context, injected for the same reason as
// AddressLookup.
type ShippingLookup func(ctx context.Context, postalCode string, subtotalCents int64) ([]application.ShippingOption, error)

// ShippingQuoterAdapter adapts a ShippingLookup function into ShippingQuoter.
type ShippingQuoterAdapter struct {
	lookup ShippingLookup
}

var _ application.ShippingQuoter = (*ShippingQuoterAdapter)(nil)

// NewShippingQuoterAdapter builds a ShippingQuoterAdapter from a lookup func.
func NewShippingQuoterAdapter(lookup ShippingLookup) *ShippingQuoterAdapter {
	return &ShippingQuoterAdapter{lookup: lookup}
}

// Quote returns the available shipping options, delegating to the injected lookup.
func (a *ShippingQuoterAdapter) Quote(ctx context.Context, postalCode string, subtotalCents int64) ([]application.ShippingOption, error) {
	return a.lookup(ctx, postalCode, subtotalCents)
}

// ---------------------------------------------------------------------------
// Charger adapter (Phase 3a mock)
// ---------------------------------------------------------------------------

// MockCharger is the Phase 3a payment adapter. It mints a pending charge in
// process without contacting a provider; the persisted charge row is written
// later, atomically, inside ConfirmTx. ChargeID is generated here so it stays
// stable across idempotent replays (the service calls CreateCharge once before
// ConfirmTx, then reuses the same ChargeView).
type MockCharger struct{}

var _ application.Charger = (*MockCharger)(nil)

// NewMockCharger builds a MockCharger.
func NewMockCharger() *MockCharger { return &MockCharger{} }

// CreateCharge returns a pending mock charge for the given order and amount.
func (MockCharger) CreateCharge(_ context.Context, orderID uuid.UUID, amount int64, method string) (application.ChargeView, error) {
	return application.ChargeView{
		ChargeID: uuid.New(),
		OrderID:  orderID,
		Amount:   amount,
		Method:   method,
		Status:   "pending",
	}, nil
}

// ---------------------------------------------------------------------------
// Mappers and helpers
// ---------------------------------------------------------------------------

// mapOrder converts a persisted orders row into the ordering domain aggregate.
func mapOrder(row queries.Order) orderingDomain.Order {
	return orderingDomain.Order{
		ID:               row.ID,
		UserID:           row.UserID,
		Status:           orderingDomain.OrderStatus(row.Status),
		Subtotal:         row.SubtotalCents,
		Shipping:         row.ShippingCents,
		Discount:         row.DiscountCents,
		Total:            row.TotalCents,
		CouponCode:       row.CouponCode,
		AddressSnapshot:  json.RawMessage(row.AddressSnapshot),
		ShippingSnapshot: json.RawMessage(row.ShippingSnapshot),
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

// nilIfEmpty returns nil for an empty string, otherwise a pointer to it. Used to
// map checkout's "" sentinel for "no coupon" onto a nullable Postgres column.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// jsonOrEmptyObject returns raw if it is non-empty, otherwise a JSON empty
// object. JSONB NOT NULL columns reject a nil/empty payload, so snapshots that
// were never populated still need a valid document.
func jsonOrEmptyObject(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505). Used to map an idempotency-key PK conflict onto
// domain.ErrIdempotencyConflict.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
