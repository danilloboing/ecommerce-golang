package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// Repository is the Postgres-backed inventory store.
type Repository struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

var _ application.StockRepository = (*Repository)(nil)

// New builds a Repository from a pgx pool.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool, q: queries.New(pool)} }

// Pool exposes the pool for integration-test seeding only.
func (r *Repository) Pool() *pgxpool.Pool { return r.pool }

// Reserve holds stock for every item in ONE tx, locking variants in ascending
// id order (I2 deadlock avoidance). The conditional ReserveStock is the oversell
// guard (I3): zero rows → ErrInsufficientStock → whole tx rolls back.
func (r *Repository) Reserve(ctx context.Context, items []application.ReserveItem, orderID uuid.UUID, expiresAt time.Time) error {
	ordered := make([]application.ReserveItem, len(items))
	copy(ordered, items)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].VariantID.String() < ordered[j].VariantID.String() })

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inventory repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	for _, it := range ordered {
		_, err := q.ReserveStock(ctx, queries.ReserveStockParams{VariantID: it.VariantID, Qty: int32(it.Quantity)})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrInsufficientStock
		}
		if err != nil {
			return fmt.Errorf("inventory repo: reserve %s: %w", it.VariantID, err)
		}
		if _, err := q.CreateReservation(ctx, queries.CreateReservationParams{
			OrderID: orderID, VariantID: it.VariantID, Quantity: int32(it.Quantity), ExpiresAt: expiresAt,
		}); err != nil {
			return fmt.Errorf("inventory repo: create reservation: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// CommitForOrder turns held reservations into committed (stock leaves), guarded
// by status (I6). Idempotent: a second call finds no held rows → no-op.
func (r *Repository) CommitForOrder(ctx context.Context, orderID uuid.UUID) error {
	return r.resolve(ctx, orderID, domain.StatusCommitted)
}

// ReleaseForOrder returns held reservations to available, guarded by status (I6).
func (r *Repository) ReleaseForOrder(ctx context.Context, orderID uuid.UUID) error {
	return r.resolve(ctx, orderID, domain.StatusReleased)
}

func (r *Repository) resolve(ctx context.Context, orderID uuid.UUID, to domain.ReservationStatus) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inventory repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	res, err := q.ListReservationsByOrder(ctx, orderID)
	if err != nil {
		return fmt.Errorf("inventory repo: list reservations: %w", err)
	}
	for _, rv := range res {
		if domain.ReservationStatus(rv.Status) != domain.StatusHeld {
			continue
		}
		switch to {
		case domain.StatusCommitted:
			if err := q.CommitReservedStock(ctx, queries.CommitReservedStockParams{VariantID: rv.VariantID, Qty: rv.Quantity}); err != nil {
				return fmt.Errorf("inventory repo: commit stock: %w", err)
			}
		case domain.StatusReleased:
			if err := q.ReleaseReservedStock(ctx, queries.ReleaseReservedStockParams{VariantID: rv.VariantID, Qty: rv.Quantity}); err != nil {
				return fmt.Errorf("inventory repo: release stock: %w", err)
			}
		}
	}
	if _, err := q.SetReservationStatus(ctx, queries.SetReservationStatusParams{OrderID: orderID, NewStatus: string(to)}); err != nil {
		return fmt.Errorf("inventory repo: set reservation status: %w", err)
	}
	return tx.Commit(ctx)
}

// SetStock upserts the available quantity for a variant with optimistic locking.
// ErrStockConflict is returned when expectedVersion does not match the current version.
func (r *Repository) SetStock(ctx context.Context, variantID uuid.UUID, available, expectedVersion int) (domain.Stock, error) {
	row, err := r.q.UpsertStock(ctx, queries.UpsertStockParams{
		VariantID: variantID, Available: int32(available), ExpectedVersion: int32(expectedVersion),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Stock{}, domain.ErrStockConflict
	}
	if err != nil {
		return domain.Stock{}, fmt.Errorf("inventory repo: set stock: %w", err)
	}
	return mapStock(row), nil
}

// Get retrieves the current stock snapshot for a variant.
func (r *Repository) Get(ctx context.Context, variantID uuid.UUID) (domain.Stock, error) {
	row, err := r.q.GetStock(ctx, variantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Stock{}, domain.ErrStockNotFound
	}
	if err != nil {
		return domain.Stock{}, fmt.Errorf("inventory repo: get stock: %w", err)
	}
	return mapStock(row), nil
}
