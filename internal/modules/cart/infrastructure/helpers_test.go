//go:build integration

package infrastructure_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type pgxIDs struct {
	userID    uuid.UUID
	productID uuid.UUID
	variantID uuid.UUID
}

// seedCatalog inserts the minimal rows cart tests depend on: a category, a user,
// a product, and a variant with an explicit price_cents of 9900.
func seedCatalog(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *pgxIDs {
	t.Helper()
	ids := &pgxIDs{userID: uuid.New(), productID: uuid.New(), variantID: uuid.New()}
	categoryID := uuid.New()

	_, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'Test')`,
		ids.userID, "u-"+ids.userID.String()+"@test.local")
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO catalog_categories (id, slug, name) VALUES ($1, $2, 'Cat')`,
		categoryID, "cat-"+categoryID.String())
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO catalog_products
		(id, slug, name, description, brand, category_id, base_price_cents, currency, status)
		VALUES ($1, $2, 'P', 'D', 'B', $3, 5000, 'BRL', 'published')`,
		ids.productID, "slug-"+ids.productID.String(), categoryID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO catalog_variants (id, product_id, sku, size, color, price_cents)
		VALUES ($1, $2, $3, 'M', 'Red', 9900)`,
		ids.variantID, ids.productID, "sku-"+ids.variantID.String())
	require.NoError(t, err)

	return ids
}
