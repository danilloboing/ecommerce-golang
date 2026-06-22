package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Order struct {
	ID                uuid.UUID       `json:"id"`
	UserID            uuid.UUID       `json:"user_id"`
	Status            OrderStatus     `json:"status"`
	Subtotal          int64           `json:"subtotal"`
	Shipping          int64           `json:"shipping"`
	Discount          int64           `json:"discount"`
	Total             int64           `json:"total"`
	CouponCode        *string         `json:"coupon_code"`
	AddressSnapshot   json.RawMessage `json:"address_snapshot"`
	ShippingSnapshot  json.RawMessage `json:"shipping_snapshot"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type OrderItem struct {
	ID        uuid.UUID       `json:"id"`
	OrderID   uuid.UUID       `json:"order_id"`
	ProductID uuid.UUID       `json:"product_id"`
	VariantID uuid.UUID       `json:"variant_id"`
	Quantity  int32           `json:"quantity"`
	UnitPrice int64           `json:"unit_price"`
	Snapshot  json.RawMessage `json:"snapshot"`
	CreatedAt time.Time       `json:"created_at"`
}
