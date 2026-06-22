package transport

import (
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	checkoutdomain "github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

// QuoteLineResponse is one locked line in a QuoteResponse.
type QuoteLineResponse struct {
	VariantID      uuid.UUID `json:"variant_id"`
	Quantity       int       `json:"quantity"`
	UnitPriceCents int64     `json:"unit_price_cents"`
}

// ShippingOptionResponse is a single carrier service option in a QuoteResponse.
type ShippingOptionResponse struct {
	ServiceID  string `json:"service_id"`
	Name       string `json:"name"`
	PriceCents int64  `json:"price_cents"`
	ETADays    int    `json:"eta_days"`
}

// QuoteResponse is the JSON shape returned by POST /checkout/quote.
type QuoteResponse struct {
	QuoteID   uuid.UUID                `json:"quote_id"`
	Lines     []QuoteLineResponse      `json:"lines"`
	Options   []ShippingOptionResponse `json:"options"`
	Chosen    ShippingOptionResponse   `json:"chosen"`
	Subtotal  int64                    `json:"subtotal_cents"`
	Shipping  int64                    `json:"shipping_cents"`
	Discount  int64                    `json:"discount_cents"`
	Total     int64                    `json:"total_cents"`
	ExpiresAt time.Time                `json:"expires_at"`
}

// ConfirmResponse is the JSON shape returned by POST /checkout/confirm.
type ConfirmResponse struct {
	OrderID   uuid.UUID `json:"order_id"`
	Status    string    `json:"status"`
	Subtotal  int64     `json:"subtotal_cents"`
	Shipping  int64     `json:"shipping_cents"`
	Discount  int64     `json:"discount_cents"`
	Total     int64     `json:"total_cents"`
	ChargeID  uuid.UUID `json:"charge_id"`
	Method    string    `json:"payment_method"`
	CreatedAt time.Time `json:"created_at"`
}

// CouponResponse is the JSON shape returned by POST /admin/coupons.
type CouponResponse struct {
	Code          string     `json:"code"`
	Type          string     `json:"type"`
	Value         int64      `json:"value"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	UsageLimit    *int       `json:"usage_limit,omitempty"`
	UsedCount     int        `json:"used_count"`
	MinOrderCents *int64     `json:"min_order_cents,omitempty"`
	Active        bool       `json:"active"`
}

// toQuoteResponse maps a QuoteResult to its wire representation.
func toQuoteResponse(r application.QuoteResult) QuoteResponse {
	lines := make([]QuoteLineResponse, 0, len(r.Lines))
	for _, l := range r.Lines {
		lines = append(lines, toQuoteLineResponse(l))
	}
	opts := make([]ShippingOptionResponse, 0, len(r.Options))
	for _, o := range r.Options {
		opts = append(opts, toShippingOptionResponse(o))
	}
	return QuoteResponse{
		QuoteID:   r.QuoteID,
		Lines:     lines,
		Options:   opts,
		Chosen:    toShippingOptionResponse(r.Chosen),
		Subtotal:  r.Subtotal,
		Shipping:  r.Shipping,
		Discount:  r.Discount,
		Total:     r.Total,
		ExpiresAt: r.ExpiresAt,
	}
}

func toQuoteLineResponse(l checkoutdomain.QuoteLine) QuoteLineResponse {
	return QuoteLineResponse{
		VariantID:      l.VariantID,
		Quantity:       l.Quantity,
		UnitPriceCents: l.UnitPriceCents,
	}
}

func toShippingOptionResponse(o application.ShippingOption) ShippingOptionResponse {
	return ShippingOptionResponse{
		ServiceID:  o.ServiceID,
		Name:       o.Name,
		PriceCents: o.PriceCents,
		ETADays:    o.ETADays,
	}
}

// toConfirmResponse maps a ConfirmResult to its wire representation.
func toConfirmResponse(r application.ConfirmResult) ConfirmResponse {
	return ConfirmResponse{
		OrderID:   r.Order.ID,
		Status:    string(r.Order.Status),
		Subtotal:  r.Order.Subtotal,
		Shipping:  r.Order.Shipping,
		Discount:  r.Order.Discount,
		Total:     r.Order.Total,
		ChargeID:  r.Charge.ChargeID,
		Method:    r.Charge.Method,
		CreatedAt: r.Order.CreatedAt,
	}
}

// toCouponResponse maps a domain Coupon to its wire representation.
func toCouponResponse(c *checkoutdomain.Coupon) CouponResponse {
	return CouponResponse{
		Code:          c.Code,
		Type:          string(c.Type),
		Value:         c.Value,
		ExpiresAt:     c.ExpiresAt,
		UsageLimit:    c.UsageLimit,
		UsedCount:     c.UsedCount,
		MinOrderCents: c.MinOrderCents,
		Active:        c.Active,
	}
}
