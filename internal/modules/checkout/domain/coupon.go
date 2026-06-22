package domain

import "time"

// CouponType distinguishes fixed-amount from percentage discounts.
type CouponType string

// Supported coupon discount types.
const (
	Fixed   CouponType = "fixed"
	Percent CouponType = "percent"
)

// Coupon represents a promotional discount code.
type Coupon struct {
	Code          string
	Type          CouponType
	Value         int64
	ExpiresAt     *time.Time
	UsageLimit    *int
	UsedCount     int
	MinOrderCents *int64
	Active        bool
}
