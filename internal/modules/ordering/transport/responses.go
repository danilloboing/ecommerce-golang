// Package transport contains HTTP handlers for the ordering module.
package transport

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
)

// OrderItemResponse is the JSON shape for a single order item.
type OrderItemResponse struct {
	ID        uuid.UUID       `json:"id"`
	OrderID   uuid.UUID       `json:"order_id"`
	ProductID uuid.UUID       `json:"product_id"`
	VariantID uuid.UUID       `json:"variant_id"`
	Quantity  int32           `json:"quantity"`
	UnitPrice int64           `json:"unit_price"`
	Snapshot  json.RawMessage `json:"snapshot,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// OrderResponse is the JSON shape for a full order (with items).
type OrderResponse struct {
	ID               uuid.UUID           `json:"id"`
	UserID           uuid.UUID           `json:"user_id"`
	Status           domain.OrderStatus  `json:"status"`
	Subtotal         int64               `json:"subtotal"`
	Shipping         int64               `json:"shipping"`
	Discount         int64               `json:"discount"`
	Total            int64               `json:"total"`
	CouponCode       *string             `json:"coupon_code,omitempty"`
	AddressSnapshot  json.RawMessage     `json:"address_snapshot,omitempty"`
	ShippingSnapshot json.RawMessage     `json:"shipping_snapshot,omitempty"`
	Items            []OrderItemResponse `json:"items"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

// OrderListItem is the JSON shape for an order in a list (no items).
type OrderListItem struct {
	ID         uuid.UUID          `json:"id"`
	Status     domain.OrderStatus `json:"status"`
	Subtotal   int64              `json:"subtotal"`
	Shipping   int64              `json:"shipping"`
	Discount   int64              `json:"discount"`
	Total      int64              `json:"total"`
	CouponCode *string            `json:"coupon_code,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

func toOrderItemResponse(item domain.OrderItem) OrderItemResponse {
	return OrderItemResponse{
		ID:        item.ID,
		OrderID:   item.OrderID,
		ProductID: item.ProductID,
		VariantID: item.VariantID,
		Quantity:  item.Quantity,
		UnitPrice: item.UnitPrice,
		Snapshot:  item.Snapshot,
		CreatedAt: item.CreatedAt,
	}
}

func toOrderResponse(order domain.Order, items []domain.OrderItem) OrderResponse {
	itemsResp := make([]OrderItemResponse, 0, len(items))
	for _, item := range items {
		itemsResp = append(itemsResp, toOrderItemResponse(item))
	}
	return OrderResponse{
		ID:               order.ID,
		UserID:           order.UserID,
		Status:           order.Status,
		Subtotal:         order.Subtotal,
		Shipping:         order.Shipping,
		Discount:         order.Discount,
		Total:            order.Total,
		CouponCode:       order.CouponCode,
		AddressSnapshot:  order.AddressSnapshot,
		ShippingSnapshot: order.ShippingSnapshot,
		Items:            itemsResp,
		CreatedAt:        order.CreatedAt,
		UpdatedAt:        order.UpdatedAt,
	}
}

func toOrderListItem(order domain.Order) OrderListItem {
	return OrderListItem{
		ID:         order.ID,
		Status:     order.Status,
		Subtotal:   order.Subtotal,
		Shipping:   order.Shipping,
		Discount:   order.Discount,
		Total:      order.Total,
		CouponCode: order.CouponCode,
		CreatedAt:  order.CreatedAt,
		UpdatedAt:  order.UpdatedAt,
	}
}
