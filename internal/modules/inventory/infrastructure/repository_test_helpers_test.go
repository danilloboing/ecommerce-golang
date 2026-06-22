//go:build integration

package infrastructure_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/infrastructure"
)

// mkOrder inserts a minimal user + order so stock_reservations.order_id FK holds.
func mkOrder(t *testing.T, ctx context.Context, repo *infrastructure.Repository, orderID uuid.UUID) error {
	t.Helper()
	pool := repo.Pool() // expose pool for tests (see repository.go)
	user := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,email,name) VALUES ($1,$2,'U')`, user, "u-"+user.String()+"@t.local"); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `INSERT INTO orders (id,user_id,status,subtotal_cents,shipping_cents,discount_cents,total_cents,address_snapshot,shipping_snapshot)
		VALUES ($1,$2,'pending_payment',0,0,0,0,'{}','{}')`, orderID, user)
	return err
}
