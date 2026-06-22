package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// Repository is the Postgres-backed order store.
type Repository struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

var _ application.OrderRepository = (*Repository)(nil)

// New builds a Repository from a pgx pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: queries.New(pool)}
}

// Create persists a new order and returns the saved record.
func (r *Repository) Create(ctx context.Context, no application.NewOrder) (domain.Order, error) {
	row, err := r.q.CreateOrder(ctx, queries.CreateOrderParams{
		ID:               uuid.New(),
		UserID:           no.UserID,
		SubtotalCents:    no.Subtotal,
		ShippingCents:    no.Shipping,
		DiscountCents:    no.Discount,
		TotalCents:       no.Total,
		CouponCode:       no.CouponCode,
		AddressSnapshot:  []byte(no.AddressSnapshot),
		ShippingSnapshot: []byte(no.ShippingSnapshot),
	})
	if err != nil {
		return domain.Order{}, fmt.Errorf("order repo: create: %w", err)
	}
	return mapOrder(row), nil
}

// GetByID returns the order with the given ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (domain.Order, error) {
	row, err := r.q.GetOrderByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("order repo: get by id: %w", err)
	}
	return mapOrder(row), nil
}

// GetUserOrder returns the order scoped to the given user.
func (r *Repository) GetUserOrder(ctx context.Context, id, userID uuid.UUID) (domain.Order, error) {
	row, err := r.q.GetUserOrderByID(ctx, queries.GetUserOrderByIDParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("order repo: get user order: %w", err)
	}
	return mapOrder(row), nil
}

// ListByUser returns up to limit orders for the user, newest first.
func (r *Repository) ListByUser(ctx context.Context, userID uuid.UUID, limit int) ([]domain.Order, error) {
	rows, err := r.q.ListOrdersByUser(ctx, queries.ListOrdersByUserParams{
		UserID: userID,
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("order repo: list by user: %w", err)
	}
	out := make([]domain.Order, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapOrder(row))
	}
	return out, nil
}

// CreateItem persists a new order item.
func (r *Repository) CreateItem(ctx context.Context, orderID uuid.UUID, item application.NewOrderItem) (domain.OrderItem, error) {
	row, err := r.q.CreateOrderItem(ctx, queries.CreateOrderItemParams{
		OrderID:         orderID,
		VariantID:       item.VariantID,
		Quantity:        item.Quantity,
		UnitPriceCents:  item.UnitPriceCents,
		ProductSnapshot: []byte(item.ProductSnapshot),
	})
	if err != nil {
		return domain.OrderItem{}, fmt.Errorf("order repo: create item: %w", err)
	}
	return mapOrderItem(row), nil
}

// ListItems returns all items for an order.
func (r *Repository) ListItems(ctx context.Context, orderID uuid.UUID) ([]domain.OrderItem, error) {
	rows, err := r.q.ListOrderItems(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("order repo: list items: %w", err)
	}
	out := make([]domain.OrderItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapOrderItem(row))
	}
	return out, nil
}
