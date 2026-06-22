// Package infrastructure adapts sqlc queries to the cart domain.
package infrastructure

import (
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

func mapCart(row queries.Cart, items []queries.CartItem) domain.Cart {
	c := domain.Cart{
		ID:            row.ID,
		UserID:        row.UserID,
		AnonSessionID: row.AnonSessionID,
		Status:        domain.Status(row.Status),
		Items:         make([]domain.CartItem, 0, len(items)),
	}
	for _, it := range items {
		c.Items = append(c.Items, domain.CartItem{
			ID:             it.ID,
			VariantID:      it.VariantID,
			Quantity:       int(it.Quantity),
			UnitPriceCents: it.UnitPriceCents,
		})
	}
	return c
}
