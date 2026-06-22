// Package application holds inventory use cases and ports.
package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

// ReserveItem is a single (variant, quantity) pair in a reservation request.
type ReserveItem struct {
	VariantID uuid.UUID
	Quantity  int
}

// StockRepository persists stock and reservations atomically.
type StockRepository interface {
	Reserve(ctx context.Context, items []ReserveItem, orderID uuid.UUID, expiresAt time.Time) error
	CommitForOrder(ctx context.Context, orderID uuid.UUID) error
	ReleaseForOrder(ctx context.Context, orderID uuid.UUID) error
	SetStock(ctx context.Context, variantID uuid.UUID, available, expectedVersion int) (domain.Stock, error)
	Get(ctx context.Context, variantID uuid.UUID) (domain.Stock, error)
}
