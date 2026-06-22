package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// PriceReader resolves authoritative per-variant unit prices from the catalog.
// Price always comes from here — clients never supply prices (C3).
type PriceReader struct {
	q *queries.Queries
}

var _ application.PriceReader = (*PriceReader)(nil)

// NewPriceReader builds a PriceReader from a pgx pool.
func NewPriceReader(pool *pgxpool.Pool) *PriceReader {
	return &PriceReader{q: queries.New(pool)}
}

// UnitPrice returns the variant's price in cents, falling back to the product's
// base price when the variant has no override (resolved in SQL).
func (r *PriceReader) UnitPrice(ctx context.Context, variantID uuid.UUID) (int64, error) {
	price, err := r.q.GetVariantUnitPrice(ctx, variantID)
	if err != nil {
		return 0, fmt.Errorf("checkout price reader: unit price: %w", err)
	}
	return price, nil
}
