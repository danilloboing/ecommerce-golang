package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

// CartService orchestrates cart read/write flows.
type CartService struct {
	repo CartRepository
}

// NewCartService builds a CartService.
func NewCartService(repo CartRepository) *CartService {
	return &CartService{repo: repo}
}

// Get returns the owner's active cart, or an empty cart when none exists.
func (s *CartService) Get(ctx context.Context, owner domain.Owner) (domain.Cart, error) {
	cart, err := s.repo.FindActive(ctx, owner)
	if err != nil {
		if errors.Is(err, domain.ErrCartNotFound) {
			return domain.Cart{}, nil
		}
		return domain.Cart{}, err
	}
	return cart, nil
}

// AddItem validates quantity, snapshots the variant price, lazily creates the
// cart, and upserts the line (summing on conflict).
func (s *CartService) AddItem(ctx context.Context, owner domain.Owner, variantID uuid.UUID, qty int) (domain.Cart, error) {
	if err := domain.ValidateQuantity(qty); err != nil {
		return domain.Cart{}, err
	}
	price, err := s.repo.VariantUnitPrice(ctx, variantID)
	if err != nil {
		return domain.Cart{}, err
	}
	cart, err := s.repo.EnsureActive(ctx, owner)
	if err != nil {
		return domain.Cart{}, err
	}
	if err := s.repo.UpsertItem(ctx, cart.ID, variantID, qty, price); err != nil {
		return domain.Cart{}, err
	}
	return s.repo.FindActive(ctx, owner)
}

// UpdateItem sets a line quantity. Cross-cart item IDs surface as ErrItemNotFound.
func (s *CartService) UpdateItem(ctx context.Context, owner domain.Owner, itemID uuid.UUID, qty int) (domain.Cart, error) {
	if err := domain.ValidateQuantity(qty); err != nil {
		return domain.Cart{}, err
	}
	cart, err := s.repo.FindActive(ctx, owner)
	if err != nil {
		return domain.Cart{}, err
	}
	if err := s.repo.UpdateItemQuantity(ctx, cart.ID, itemID, qty); err != nil {
		return domain.Cart{}, err
	}
	return s.repo.FindActive(ctx, owner)
}

// RemoveItem drops a line from the cart.
func (s *CartService) RemoveItem(ctx context.Context, owner domain.Owner, itemID uuid.UUID) (domain.Cart, error) {
	cart, err := s.repo.FindActive(ctx, owner)
	if err != nil {
		return domain.Cart{}, err
	}
	if err := s.repo.DeleteItem(ctx, cart.ID, itemID); err != nil {
		return domain.Cart{}, err
	}
	return s.repo.FindActive(ctx, owner)
}

// Clear removes all items from the owner's cart. No-op when no cart exists.
func (s *CartService) Clear(ctx context.Context, owner domain.Owner) error {
	cart, err := s.repo.FindActive(ctx, owner)
	if err != nil {
		if errors.Is(err, domain.ErrCartNotFound) {
			return nil
		}
		return err
	}
	return s.repo.ClearItems(ctx, cart.ID)
}

// Merge folds an anonymous cart into the user's active cart (summing lines)
// and marks the anon cart merged. No-op when the anon cart is empty/absent.
func (s *CartService) Merge(ctx context.Context, anonID string, userID uuid.UUID) error {
	return s.repo.Merge(ctx, anonID, userID)
}
