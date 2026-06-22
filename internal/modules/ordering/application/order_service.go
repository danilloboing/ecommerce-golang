package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
)

// OrderService orchestrates order read flows.
type OrderService struct {
	repo OrderRepository
}

// NewOrderService builds an OrderService.
func NewOrderService(repo OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

// GetForUser returns an order with its items if the user owns it.
// Cross-user access or missing order returns ErrOrderNotFound (I7).
func (s *OrderService) GetForUser(ctx context.Context, id, userID uuid.UUID) (domain.Order, []domain.OrderItem, error) {
	order, err := s.repo.GetUserOrder(ctx, id, userID)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			return domain.Order{}, nil, domain.ErrOrderNotFound
		}
		return domain.Order{}, nil, err
	}

	items, err := s.repo.ListItems(ctx, id)
	if err != nil {
		return domain.Order{}, nil, err
	}

	return order, items, nil
}

// ListForUser returns the user's orders up to limit.
func (s *OrderService) ListForUser(ctx context.Context, userID uuid.UUID, limit int) ([]domain.Order, error) {
	return s.repo.ListByUser(ctx, userID, limit)
}
