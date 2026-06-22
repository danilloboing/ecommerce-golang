// Package application contains address use cases and ports.
package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

// AddressRepository is the persistence contract for addresses.
// Create and SetDefault maintain the single-default invariant atomically.
type AddressRepository interface {
	Create(ctx context.Context, a domain.Address) (domain.Address, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (domain.Address, error)
	List(ctx context.Context, userID uuid.UUID) ([]domain.Address, error)
	Update(ctx context.Context, a domain.Address) (domain.Address, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	SetDefault(ctx context.Context, id, userID uuid.UUID) (domain.Address, error)
}
