package infrastructure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// Repository is the Postgres-backed charge store.
type Repository struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

var _ application.ChargeRepository = (*Repository)(nil)

// New builds a Repository from a pgx pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: queries.New(pool)}
}

// Create persists a charge via sqlc and returns the DB-assigned row (id, status).
func (r *Repository) Create(ctx context.Context, c domain.Charge) (domain.Charge, error) {
	row, err := r.q.CreateCharge(ctx, queries.CreateChargeParams{
		OrderID:          c.OrderID,
		Provider:         c.Provider,
		ProviderChargeID: c.ProviderChargeID,
		Method:           c.Method,
		AmountCents:      c.AmountCents,
		RawPayload:       []byte("{}"),
	})
	if err != nil {
		return domain.Charge{}, fmt.Errorf("charge repo: create: %w", err)
	}
	return mapCharge(row), nil
}

func mapCharge(row queries.Charge) domain.Charge {
	return domain.Charge{
		ID:               row.ID,
		OrderID:          row.OrderID,
		Provider:         row.Provider,
		ProviderChargeID: row.ProviderChargeID,
		Method:           row.Method,
		Status:           domain.ChargeStatus(row.Status),
		AmountCents:      row.AmountCents,
	}
}
