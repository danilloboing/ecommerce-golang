package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
	orderingDomain "github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// ConfirmRepo executes the §5 confirm transaction atomically across the cart,
// ordering, inventory, coupon, and payment tables. This is the money-critical
// write path: every step shares one tx, so any failure rolls the whole thing
// back and nothing is persisted.
type ConfirmRepo struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

var _ application.ConfirmRepository = (*ConfirmRepo)(nil)

// NewConfirmRepo builds a ConfirmRepo from a pgx pool.
func NewConfirmRepo(pool *pgxpool.Pool) *ConfirmRepo {
	return &ConfirmRepo{pool: pool, q: queries.New(pool)}
}

// ConfirmTx runs the whole §5 confirm atomically. The order id is plan.OrderID
// (minted by the service so the charge's provider_charge_id is replay-stable),
// and the idempotency response is the ConfirmResult{Order, Charge} this method
// builds from the order it creates plus the plan's charge.
func (r *ConfirmRepo) ConfirmTx(ctx context.Context, plan application.ConfirmPlan) (orderingDomain.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return orderingDomain.Order{}, fmt.Errorf("checkout repo: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	// Cart-guard idempotency: the active cart must still exist. A concurrent
	// confirm that already converted it leaves none → ErrCartChanged.
	uid := plan.UserID
	cart, err := q.GetActiveCartByUser(ctx, &uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return orderingDomain.Order{}, domain.ErrCartChanged
	}
	if err != nil {
		return orderingDomain.Order{}, fmt.Errorf("checkout repo: get cart: %w", err)
	}

	// Order first (reservations FK to it).
	ord, err := q.CreateOrder(ctx, queries.CreateOrderParams{
		ID:               plan.OrderID,
		UserID:           plan.UserID,
		SubtotalCents:    plan.Quote.Subtotal,
		ShippingCents:    plan.Quote.Shipping,
		DiscountCents:    plan.Quote.Discount,
		TotalCents:       plan.Quote.Total,
		CouponCode:       nilIfEmpty(plan.Quote.CouponCode),
		AddressSnapshot:  jsonOrEmptyObject(plan.Quote.AddressSnapshot),
		ShippingSnapshot: jsonOrEmptyObject(plan.Quote.ShippingSnapshot),
	})
	if err != nil {
		return orderingDomain.Order{}, fmt.Errorf("checkout repo: create order: %w", err)
	}

	// Reserve stock — ascending variant order (I2: deterministic lock order
	// prevents deadlocks), conditional decrement (I3: 0 rows = oversell).
	lines := append([]domain.QuoteLine(nil), plan.Quote.Lines...)
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].VariantID.String() < lines[j].VariantID.String()
	})
	for _, l := range lines {
		if _, err := q.ReserveStock(ctx, queries.ReserveStockParams{
			VariantID: l.VariantID,
			Qty:       int32(l.Quantity),
		}); errors.Is(err, pgx.ErrNoRows) {
			return orderingDomain.Order{}, domain.ErrInsufficientStock
		} else if err != nil {
			return orderingDomain.Order{}, fmt.Errorf("checkout repo: reserve: %w", err)
		}
		if _, err := q.CreateReservation(ctx, queries.CreateReservationParams{
			OrderID:   plan.OrderID,
			VariantID: l.VariantID,
			Quantity:  int32(l.Quantity),
			ExpiresAt: plan.ReservationExpiresAt,
		}); err != nil {
			return orderingDomain.Order{}, fmt.Errorf("checkout repo: reservation: %w", err)
		}
	}

	// Order items (original line order, not the reserve sort).
	for _, l := range plan.Quote.Lines {
		if _, err := q.CreateOrderItem(ctx, queries.CreateOrderItemParams{
			OrderID:         plan.OrderID,
			VariantID:       l.VariantID,
			Quantity:        int32(l.Quantity),
			UnitPriceCents:  l.UnitPriceCents,
			ProductSnapshot: jsonOrEmptyObject(l.ProductSnapshot),
		}); err != nil {
			return orderingDomain.Order{}, fmt.Errorf("checkout repo: order item: %w", err)
		}
	}

	// Coupon redeem — atomic conditional (C4). 0 rows = inactive/expired/at-limit.
	if plan.Quote.CouponCode != "" {
		if _, err := q.RedeemCoupon(ctx, plan.Quote.CouponCode); errors.Is(err, pgx.ErrNoRows) {
			return orderingDomain.Order{}, domain.ErrCouponUnavailable
		} else if err != nil {
			return orderingDomain.Order{}, fmt.Errorf("checkout repo: redeem coupon: %w", err)
		}
	}

	// Convert the cart (cart-guard: unique active index → one winner).
	if err := q.SetCartStatus(ctx, queries.SetCartStatusParams{ID: cart.ID, Status: "converted"}); err != nil {
		return orderingDomain.Order{}, fmt.Errorf("checkout repo: convert cart: %w", err)
	}

	// Persist the (mock, in-process) charge. provider_charge_id is the
	// service-minted charge id, so an idempotent retry maps to the same row.
	if _, err := q.CreateCharge(ctx, queries.CreateChargeParams{
		OrderID:          plan.OrderID,
		Provider:         mockChargeProvider,
		ProviderChargeID: plan.Charge.ChargeID.String(),
		Method:           plan.Charge.Method,
		AmountCents:      plan.Charge.Amount,
		RawPayload:       nil,
	}); err != nil {
		return orderingDomain.Order{}, fmt.Errorf("checkout repo: charge: %w", err)
	}

	// Build the replay payload from the order we just created plus the plan's
	// charge, then claim the idempotency key. (user,key) PK conflict means a
	// concurrent confirm won the race → ErrIdempotencyConflict.
	mapped := mapOrder(ord)
	result := application.ConfirmResult{Order: mapped, Charge: plan.Charge}
	responseJSON, err := json.Marshal(result)
	if err != nil {
		return orderingDomain.Order{}, fmt.Errorf("checkout repo: marshal idempotency response: %w", err)
	}
	if err := q.PutIdempotencyKey(ctx, queries.PutIdempotencyKeyParams{
		UserID:      plan.UserID,
		Key:         plan.IdempotencyKey,
		RequestHash: plan.RequestHash,
		OrderID:     &plan.OrderID,
		Response:    responseJSON,
	}); isUniqueViolation(err) {
		return orderingDomain.Order{}, domain.ErrIdempotencyConflict
	} else if err != nil {
		return orderingDomain.Order{}, fmt.Errorf("checkout repo: idempotency: %w", err)
	}

	// Record the initial status transition (nil → pending_payment).
	if err := q.RecordTransition(ctx, queries.RecordTransitionParams{
		OrderID:    plan.OrderID,
		FromStatus: nil,
		ToStatus:   "pending_payment",
		Reason:     "checkout_confirmed",
		Actor:      "system",
	}); err != nil {
		return orderingDomain.Order{}, fmt.Errorf("checkout repo: transition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return orderingDomain.Order{}, fmt.Errorf("checkout repo: commit: %w", err)
	}
	return mapped, nil
}
