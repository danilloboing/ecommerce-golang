package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

// InventoryService orchestrates stock reservation, commitment, and release.
type InventoryService struct {
	repo StockRepository
}

// NewInventoryService creates an InventoryService backed by the given repository.
func NewInventoryService(repo StockRepository) *InventoryService {
	return &InventoryService{repo: repo}
}

// Reserve holds stock for all items atomically (all-or-nothing).
// Returns domain.ErrInsufficientStock if any item cannot be satisfied.
func (s *InventoryService) Reserve(ctx context.Context, items []ReserveItem, orderID uuid.UUID, expiresAt time.Time) error {
	return s.repo.Reserve(ctx, items, orderID, expiresAt)
}

// Commit permanently deducts held stock for the given order.
func (s *InventoryService) Commit(ctx context.Context, orderID uuid.UUID) error {
	return s.repo.CommitForOrder(ctx, orderID)
}

// Release cancels held stock for the given order, returning it to available.
func (s *InventoryService) Release(ctx context.Context, orderID uuid.UUID) error {
	return s.repo.ReleaseForOrder(ctx, orderID)
}

// SetStock overwrites available stock for a variant, enforcing optimistic locking.
// Returns domain.ErrStockConflict if expectedVersion does not match the stored version.
func (s *InventoryService) SetStock(ctx context.Context, variantID uuid.UUID, available, version int) (domain.Stock, error) {
	return s.repo.SetStock(ctx, variantID, available, version)
}
