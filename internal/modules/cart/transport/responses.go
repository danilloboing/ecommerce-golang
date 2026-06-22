package transport

import "github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"

// CartResponse is the JSON shape returned by cart endpoints.
type CartResponse struct {
	Items         []CartItemResponse `json:"items"`
	SubtotalCents int64              `json:"subtotal_cents"`
}

// CartItemResponse is a single cart line.
type CartItemResponse struct {
	ID             string `json:"id"`
	VariantID      string `json:"variant_id"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

func toCartResponse(c domain.Cart) CartResponse {
	items := make([]CartItemResponse, 0, len(c.Items))
	for _, it := range c.Items {
		items = append(items, CartItemResponse{
			ID:             it.ID.String(),
			VariantID:      it.VariantID.String(),
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		})
	}
	return CartResponse{Items: items, SubtotalCents: c.SubtotalCents()}
}
