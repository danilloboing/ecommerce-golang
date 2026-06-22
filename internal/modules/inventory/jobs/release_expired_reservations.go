// Package jobs holds river workers for the inventory module.
package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// ReleaseExpiredReservationsArgs is the river job payload.
type ReleaseExpiredReservationsArgs struct{}

// Kind implements river.JobArgs.
func (ReleaseExpiredReservationsArgs) Kind() string { return "inventory.release_expired_reservations" }

// ReleaseExpiredReservationsWorker expires pending_payment orders whose held
// reservations are past their expiry and have no paid charge (paid wins).
type ReleaseExpiredReservationsWorker struct {
	river.WorkerDefaults[ReleaseExpiredReservationsArgs]
	pool *pgxpool.Pool
}

// NewReleaseExpiredReservationsWorker builds the worker.
func NewReleaseExpiredReservationsWorker(pool *pgxpool.Pool) *ReleaseExpiredReservationsWorker {
	return &ReleaseExpiredReservationsWorker{pool: pool}
}

// Work runs once per scheduled tick.
func (w *ReleaseExpiredReservationsWorker) Work(ctx context.Context, _ *river.Job[ReleaseExpiredReservationsArgs]) error {
	_, err := RunReleaseExpiredOnce(ctx, w.pool, time.Now())
	return err
}

// RunReleaseExpiredOnce expires pending_payment orders whose held reservations
// are past expiry AND have no paid charge (paid wins, C2). Returns the number
// of orders expired. Used by the worker and by integration tests.
func RunReleaseExpiredOnce(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int64, error) {
	q := queries.New(pool)

	orderIDs, err := q.ListExpiredHeldOrderIDs(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("inventory jobs: list expired: %w", err)
	}

	var expired int64
	for _, orderID := range orderIDs {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return expired, fmt.Errorf("inventory jobs: begin tx: %w", err)
		}
		qx := q.WithTx(tx)

		// Paid-wins guard: if a paid charge exists, leave this order for the webhook.
		paid, err := qx.HasPaidCharge(ctx, orderID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return expired, fmt.Errorf("inventory jobs: has paid charge: %w", err)
		}
		if paid {
			_ = tx.Rollback(ctx)
			continue
		}

		// Guarded status transition: only move if still pending_payment.
		n, err := qx.TransitionOrderStatus(ctx, queries.TransitionOrderStatusParams{
			ID:         orderID,
			FromStatus: "pending_payment",
			ToStatus:   "expired",
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			return expired, fmt.Errorf("inventory jobs: transition order %s: %w", orderID, err)
		}
		if n == 0 {
			// Already transitioned by a concurrent webhook — nothing to do.
			_ = tx.Rollback(ctx)
			continue
		}

		// Release held reservations back to available stock.
		reservations, err := qx.ListReservationsByOrder(ctx, orderID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return expired, fmt.Errorf("inventory jobs: list reservations for order %s: %w", orderID, err)
		}
		for _, rv := range reservations {
			if rv.Status != "held" {
				continue
			}
			if err := qx.ReleaseReservedStock(ctx, queries.ReleaseReservedStockParams{
				VariantID: rv.VariantID,
				Qty:       rv.Quantity,
			}); err != nil {
				_ = tx.Rollback(ctx)
				return expired, fmt.Errorf("inventory jobs: release stock for variant %s: %w", rv.VariantID, err)
			}
		}

		// Mark all held reservations for this order as released.
		if _, err := qx.SetReservationStatus(ctx, queries.SetReservationStatusParams{
			OrderID:   orderID,
			NewStatus: "released",
		}); err != nil {
			_ = tx.Rollback(ctx)
			return expired, fmt.Errorf("inventory jobs: set reservation status for order %s: %w", orderID, err)
		}

		// Release coupon redemption if the order had one.
		ord, err := qx.GetOrderByID(ctx, orderID)
		if err == nil && ord.CouponCode != nil {
			_ = qx.ReleaseCoupon(ctx, *ord.CouponCode)
		}

		// Audit trail.
		fromStatus := strPtr("pending_payment")
		if err := qx.RecordTransition(ctx, queries.RecordTransitionParams{
			OrderID:    orderID,
			FromStatus: fromStatus,
			ToStatus:   "expired",
			Reason:     "reservation_expired",
			Actor:      "job",
		}); err != nil {
			_ = tx.Rollback(ctx)
			return expired, fmt.Errorf("inventory jobs: record transition for order %s: %w", orderID, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return expired, fmt.Errorf("inventory jobs: commit for order %s: %w", orderID, err)
		}
		expired++
	}
	return expired, nil
}

func strPtr(s string) *string { return &s }
