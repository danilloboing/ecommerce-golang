package application

import (
	"context"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

// CouponService orchestrates coupon validation and creation flows.
type CouponService struct {
	repo CouponRepository
}

// NewCouponService builds a CouponService.
func NewCouponService(repo CouponRepository) *CouponService {
	return &CouponService{repo: repo}
}

// Validate returns the discount for an applicable coupon, or ErrCouponInvalid.
// It does NOT redeem — redemption is the atomic step inside confirm (C4, Task 18).
func (s *CouponService) Validate(ctx context.Context, code string, subtotalCents int64) (int64, error) {
	c, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return 0, domain.ErrCouponInvalid
	}
	if !c.Active {
		return 0, domain.ErrCouponInvalid
	}
	if c.ExpiresAt != nil && !c.ExpiresAt.After(time.Now()) {
		return 0, domain.ErrCouponInvalid
	}
	if c.UsageLimit != nil && c.UsedCount >= *c.UsageLimit {
		return 0, domain.ErrCouponInvalid
	}
	if c.MinOrderCents != nil && subtotalCents < *c.MinOrderCents {
		return 0, domain.ErrCouponInvalid
	}
	return domain.ComputeDiscount(c.Type, c.Value, subtotalCents), nil
}

// Create persists a new coupon for admin use.
func (s *CouponService) Create(ctx context.Context, in NewCoupon) (*domain.Coupon, error) {
	return s.repo.Create(ctx, in)
}
