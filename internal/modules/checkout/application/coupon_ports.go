// Package application contains checkout use cases and ports.
package application

import (
	"context"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

// CouponRepository is the persistence contract for coupons.
// Redeem performs an atomic conditional UPDATE that increments used_count;
// it maps a zero-rows result to ErrCouponUnavailable (C4 — confirmed in Task 18).
type CouponRepository interface {
	GetByCode(ctx context.Context, code string) (*domain.Coupon, error)
	Redeem(ctx context.Context, couponID, userID string) error
	Release(ctx context.Context, couponID, userID string) error
	Create(ctx context.Context, in NewCoupon) (*domain.Coupon, error)
}

// NewCoupon carries the fields needed to persist a new promotional code.
type NewCoupon struct {
	Code          string
	Type          domain.CouponType
	Value         int64
	ExpiresAt     *time.Time
	UsageLimit    *int
	MinOrderCents *int64
}
