// Package application contains ordering use cases and ports.
package application

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
)

// OrderRepository is the persistence contract for orders and their items.
type OrderRepository interface {
	Create(ctx context.Context, no NewOrder) (domain.Order, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.Order, error)
	GetUserOrder(ctx context.Context, id, userID uuid.UUID) (domain.Order, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit int) ([]domain.Order, error)
	CreateItem(ctx context.Context, orderID uuid.UUID, item NewOrderItem) (domain.OrderItem, error)
	ListItems(ctx context.Context, orderID uuid.UUID) ([]domain.OrderItem, error)
}

// NewOrder is the input struct for creating an order.
// Fields mirror the orders table columns.
type NewOrder struct {
	UserID           uuid.UUID
	Subtotal         int64
	Shipping         int64
	Discount         int64
	Total            int64
	CouponCode       *string
	AddressSnapshot  json.RawMessage
	ShippingSnapshot json.RawMessage
}

// NewOrderItem is the input struct for creating an order item.
// Fields mirror the order_items table columns.
type NewOrderItem struct {
	VariantID        uuid.UUID
	Quantity         int32
	UnitPriceCents   int64
	ProductSnapshot  json.RawMessage
}
