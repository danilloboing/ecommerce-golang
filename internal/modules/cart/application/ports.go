// Package application contains cart use cases and ports.
package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

// CartRepository is the persistence contract for carts and their items.
// FindActive/EnsureActive return a Cart with Items loaded. EnsureActive
// lazily creates an active cart for the owner if none exists.
type CartRepository interface {
	FindActive(ctx context.Context, owner domain.Owner) (domain.Cart, error)
	EnsureActive(ctx context.Context, owner domain.Owner) (domain.Cart, error)
	VariantUnitPrice(ctx context.Context, variantID uuid.UUID) (int64, error)
	UpsertItem(ctx context.Context, cartID, variantID uuid.UUID, qty int, unitPrice int64) error
	UpdateItemQuantity(ctx context.Context, cartID, itemID uuid.UUID, qty int) error
	DeleteItem(ctx context.Context, cartID, itemID uuid.UUID) error
	ClearItems(ctx context.Context, cartID uuid.UUID) error
	Merge(ctx context.Context, anonID string, userID uuid.UUID) error
}
