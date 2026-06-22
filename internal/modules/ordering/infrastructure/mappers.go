// Package infrastructure adapts sqlc queries to the ordering domain.
package infrastructure

import (
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

func mapOrder(row queries.Order) domain.Order {
	return domain.Order{
		ID:               row.ID,
		UserID:           row.UserID,
		Status:           domain.OrderStatus(row.Status),
		Subtotal:         row.SubtotalCents,
		Shipping:         row.ShippingCents,
		Discount:         row.DiscountCents,
		Total:            row.TotalCents,
		CouponCode:       row.CouponCode,
		AddressSnapshot:  row.AddressSnapshot,
		ShippingSnapshot: row.ShippingSnapshot,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func mapOrderItem(row queries.OrderItem) domain.OrderItem {
	return domain.OrderItem{
		ID:        row.ID,
		OrderID:   row.OrderID,
		VariantID: row.VariantID,
		Quantity:  row.Quantity,
		UnitPrice: row.UnitPriceCents,
		Snapshot:  row.ProductSnapshot,
		CreatedAt: row.CreatedAt,
	}
}
