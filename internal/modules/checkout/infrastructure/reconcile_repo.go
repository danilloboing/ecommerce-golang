package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	orderingdomain "github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
	paymentdomain "github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// ReconcileRepo applies a verified payment event atomically (C5 dedup+amount+
// forward-only; C2 paid-after-expiry via savepoint). It satisfies
// payment/application.EventApplier.
type ReconcileRepo struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

// NewReconcileRepo builds a ReconcileRepo from a pgx pool.
func NewReconcileRepo(pool *pgxpool.Pool) *ReconcileRepo {
	return &ReconcileRepo{pool: pool, q: queries.New(pool)}
}

// Apply processes one verified payment event inside a single database
// transaction. Returns nil on no-op (dup event_id, unknown charge, amount
// mismatch, or invalid forward-only transition).
func (r *ReconcileRepo) Apply(ctx context.Context, ev paymentdomain.Event) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("reconcile: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	// C5 idempotency: insert a dedup row keyed on event_id. ON CONFLICT DO
	// NOTHING returns 0 rows — this event was already processed; commit and
	// return.
	n, err := q.InsertWebhookEvent(ctx, queries.InsertWebhookEventParams{
		EventID:  ev.ID,
		Provider: "mock",
		ChargeID: nil,
	})
	if err != nil {
		return fmt.Errorf("reconcile: dedup insert: %w", err)
	}
	if n == 0 {
		return tx.Commit(ctx)
	}

	// Look up the charge by provider + provider_charge_id.
	charge, err := q.GetChargeByProviderID(ctx, queries.GetChargeByProviderIDParams{
		Provider:         "mock",
		ProviderChargeID: ev.ProviderChargeID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Unknown charge — record the event but apply nothing.
		return tx.Commit(ctx)
	}
	if err != nil {
		return fmt.Errorf("reconcile: get charge: %w", err)
	}

	ord, err := q.GetOrderByID(ctx, charge.OrderID)
	if err != nil {
		return fmt.Errorf("reconcile: get order: %w", err)
	}

	// C5 amount integrity: event.AmountCents must equal the charge row AND the
	// order total. A mismatch is recorded (anti-replay) but we apply nothing.
	if ev.AmountCents != charge.AmountCents || charge.AmountCents != ord.TotalCents {
		return tx.Commit(ctx)
	}

	switch ev.Type {
	case "failed":
		if orderingdomain.OrderStatus(ord.Status) != orderingdomain.PendingPayment {
			return tx.Commit(ctx) // forward-only
		}
		if err := releaseHeld(ctx, q, ord.ID); err != nil {
			return fmt.Errorf("reconcile: release held: %w", err)
		}
		if ord.CouponCode != nil {
			if err := q.ReleaseCoupon(ctx, *ord.CouponCode); err != nil {
				return fmt.Errorf("reconcile: release coupon: %w", err)
			}
		}
		if err := q.SetChargeStatus(ctx, queries.SetChargeStatusParams{
			ID:     charge.ID,
			Status: "failed",
		}); err != nil {
			return fmt.Errorf("reconcile: set charge failed: %w", err)
		}
		if _, err := q.TransitionOrderStatus(ctx, queries.TransitionOrderStatusParams{
			ID:         ord.ID,
			FromStatus: string(orderingdomain.PendingPayment),
			ToStatus:   string(orderingdomain.PaymentFailed),
		}); err != nil {
			return fmt.Errorf("reconcile: transition to payment_failed: %w", err)
		}
		if err := q.RecordTransition(ctx, queries.RecordTransitionParams{
			OrderID:    ord.ID,
			FromStatus: strPtr(string(orderingdomain.PendingPayment)),
			ToStatus:   string(orderingdomain.PaymentFailed),
			Reason:     "charge_failed",
			Actor:      "webhook",
		}); err != nil {
			return fmt.Errorf("reconcile: record failed transition: %w", err)
		}
		return tx.Commit(ctx)

	case "paid":
		switch orderingdomain.OrderStatus(ord.Status) {
		case orderingdomain.PendingPayment:
			if err := commitHeld(ctx, q, ord.ID); err != nil {
				return fmt.Errorf("reconcile: commit held: %w", err)
			}
			if err := q.SetChargeStatus(ctx, queries.SetChargeStatusParams{
				ID:     charge.ID,
				Status: "paid",
			}); err != nil {
				return fmt.Errorf("reconcile: set charge paid: %w", err)
			}
			if _, err := q.TransitionOrderStatus(ctx, queries.TransitionOrderStatusParams{
				ID:         ord.ID,
				FromStatus: string(orderingdomain.PendingPayment),
				ToStatus:   string(orderingdomain.Paid),
			}); err != nil {
				return fmt.Errorf("reconcile: transition to paid: %w", err)
			}
			if err := q.RecordTransition(ctx, queries.RecordTransitionParams{
				OrderID:    ord.ID,
				FromStatus: strPtr(string(orderingdomain.PendingPayment)),
				ToStatus:   string(orderingdomain.Paid),
				Reason:     "charge_paid",
				Actor:      "webhook",
			}); err != nil {
				return fmt.Errorf("reconcile: record paid transition: %w", err)
			}
			return tx.Commit(ctx)

		case orderingdomain.Expired:
			// C2: payment landed after expiry. Attempt to re-reserve + immediately
			// commit all order items in a savepoint. If stock is unavailable the
			// savepoint is rolled back and the order becomes paid_awaiting_stock.
			// The payment is NEVER dropped.
			allOk, err := reReserveSavepoint(ctx, tx, q, ord.ID)
			if err != nil {
				return fmt.Errorf("reconcile: re-reserve savepoint: %w", err)
			}
			if err := q.SetChargeStatus(ctx, queries.SetChargeStatusParams{
				ID:     charge.ID,
				Status: "paid",
			}); err != nil {
				return fmt.Errorf("reconcile: set charge paid (expiry): %w", err)
			}
			toStatus := orderingdomain.Paid
			reason := "charge_paid_after_expiry"
			if !allOk {
				toStatus = orderingdomain.PaidAwaitingStock
				reason = "paid_no_stock"
			}
			if _, err := q.TransitionOrderStatus(ctx, queries.TransitionOrderStatusParams{
				ID:         ord.ID,
				FromStatus: string(orderingdomain.Expired),
				ToStatus:   string(toStatus),
			}); err != nil {
				return fmt.Errorf("reconcile: transition expired→%s: %w", toStatus, err)
			}
			if err := q.RecordTransition(ctx, queries.RecordTransitionParams{
				OrderID:    ord.ID,
				FromStatus: strPtr(string(orderingdomain.Expired)),
				ToStatus:   string(toStatus),
				Reason:     reason,
				Actor:      "webhook",
			}); err != nil {
				return fmt.Errorf("reconcile: record expiry transition: %w", err)
			}
			return tx.Commit(ctx)

		default:
			// Already paid / failed / paid_awaiting_stock → forward-only no-op.
			return tx.Commit(ctx)
		}
	}

	return tx.Commit(ctx)
}

// commitHeld commits the reserved stock for all held reservations on the order
// and marks them as committed.
func commitHeld(ctx context.Context, q *queries.Queries, orderID uuid.UUID) error {
	reservations, err := q.ListReservationsByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	for _, rv := range reservations {
		if rv.Status != "held" {
			continue
		}
		if err := q.CommitReservedStock(ctx, queries.CommitReservedStockParams{
			VariantID: rv.VariantID,
			Qty:       rv.Quantity,
		}); err != nil {
			return err
		}
	}
	if _, err := q.SetReservationStatus(ctx, queries.SetReservationStatusParams{
		OrderID:   orderID,
		NewStatus: "committed",
	}); err != nil {
		return err
	}
	return nil
}

// releaseHeld restores reserved stock for all held reservations on the order
// and marks them as released.
func releaseHeld(ctx context.Context, q *queries.Queries, orderID uuid.UUID) error {
	reservations, err := q.ListReservationsByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	for _, rv := range reservations {
		if rv.Status != "held" {
			continue
		}
		if err := q.ReleaseReservedStock(ctx, queries.ReleaseReservedStockParams{
			VariantID: rv.VariantID,
			Qty:       rv.Quantity,
		}); err != nil {
			return err
		}
	}
	if _, err := q.SetReservationStatus(ctx, queries.SetReservationStatusParams{
		OrderID:   orderID,
		NewStatus: "released",
	}); err != nil {
		return err
	}
	return nil
}

// reReserveSavepoint tries to re-reserve + immediately commit all order items
// in a savepoint. Returns true if all items were successfully committed. On any
// stock shortfall the savepoint is rolled back (no stock mutated) and false is
// returned. Items are sorted by variant_id to prevent deadlocks (I2).
func reReserveSavepoint(ctx context.Context, tx pgx.Tx, q *queries.Queries, orderID uuid.UUID) (bool, error) {
	items, err := q.ListOrderItems(ctx, orderID)
	if err != nil {
		return false, err
	}
	// Deterministic lock order (I2): ascending variant_id.
	sort.Slice(items, func(i, j int) bool {
		return items[i].VariantID.String() < items[j].VariantID.String()
	})

	sp, err := tx.Begin(ctx) // nested Begin creates a SAVEPOINT in pgx
	if err != nil {
		return false, err
	}
	qsp := q.WithTx(sp)

	for _, it := range items {
		_, err := qsp.ReserveStock(ctx, queries.ReserveStockParams{
			VariantID: it.VariantID,
			Qty:       it.Quantity,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Insufficient stock — roll back the savepoint cleanly.
			_ = sp.Rollback(ctx)
			return false, nil
		}
		if err != nil {
			_ = sp.Rollback(ctx)
			return false, err
		}
		if err := qsp.CommitReservedStock(ctx, queries.CommitReservedStockParams{
			VariantID: it.VariantID,
			Qty:       it.Quantity,
		}); err != nil {
			_ = sp.Rollback(ctx)
			return false, err
		}
	}

	if err := sp.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// strPtr returns a pointer to a copy of s. Used for the nullable from_status
// column in order_status_transitions.
func strPtr(s string) *string { return &s }
