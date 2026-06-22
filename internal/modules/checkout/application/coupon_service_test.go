package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

// fakeCouponRepo is an in-memory stub that satisfies CouponRepository.
type fakeCouponRepo struct {
	coupons map[string]*domain.Coupon
}

func (f *fakeCouponRepo) GetByCode(_ context.Context, code string) (*domain.Coupon, error) {
	c, ok := f.coupons[code]
	if !ok {
		return nil, errors.New("not found")
	}
	return c, nil
}

func (f *fakeCouponRepo) Redeem(_ context.Context, code string) error { return nil }
func (f *fakeCouponRepo) Release(_ context.Context, code string) error { return nil }
func (f *fakeCouponRepo) Create(_ context.Context, in application.NewCoupon) (*domain.Coupon, error) {
	c := &domain.Coupon{
		Code:          in.Code,
		Type:          in.Type,
		Value:         in.Value,
		ExpiresAt:     in.ExpiresAt,
		UsageLimit:    in.UsageLimit,
		UsedCount:     0,
		MinOrderCents: in.MinOrderCents,
		Active:        true,
	}
	f.coupons[c.Code] = c
	return c, nil
}

func newFakeRepo(coupons ...*domain.Coupon) *fakeCouponRepo {
	m := make(map[string]*domain.Coupon, len(coupons))
	for _, c := range coupons {
		m[c.Code] = c
	}
	return &fakeCouponRepo{coupons: m}
}

func ptr[T any](v T) *T { return &v }

func TestCouponService_Validate_ValidPercentCoupon(t *testing.T) {
	// Arrange
	future := time.Now().Add(24 * time.Hour)
	coupon := &domain.Coupon{
		Code:      "SAVE10",
		Type:      domain.Percent,
		Value:     10,
		ExpiresAt: &future,
		Active:    true,
	}
	svc := application.NewCouponService(newFakeRepo(coupon))

	// Act
	discount, err := svc.Validate(context.Background(), "SAVE10", 1000)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// 10% of 1000 = 100
	if discount != 100 {
		t.Fatalf("expected discount 100, got %d", discount)
	}
}

func TestCouponService_Validate_ExpiredCoupon(t *testing.T) {
	// Arrange
	past := time.Now().Add(-1 * time.Hour)
	coupon := &domain.Coupon{
		Code:      "EXPIRED",
		Type:      domain.Fixed,
		Value:     500,
		ExpiresAt: &past,
		Active:    true,
	}
	svc := application.NewCouponService(newFakeRepo(coupon))

	// Act
	_, err := svc.Validate(context.Background(), "EXPIRED", 1000)

	// Assert
	if !errors.Is(err, domain.ErrCouponInvalid) {
		t.Fatalf("expected ErrCouponInvalid, got %v", err)
	}
}

func TestCouponService_Validate_BelowMinOrder(t *testing.T) {
	// Arrange
	future := time.Now().Add(24 * time.Hour)
	coupon := &domain.Coupon{
		Code:          "MINORDER",
		Type:          domain.Fixed,
		Value:         200,
		ExpiresAt:     &future,
		MinOrderCents: ptr(int64(5000)),
		Active:        true,
	}
	svc := application.NewCouponService(newFakeRepo(coupon))

	// Act — subtotal 1000 < min 5000
	_, err := svc.Validate(context.Background(), "MINORDER", 1000)

	// Assert
	if !errors.Is(err, domain.ErrCouponInvalid) {
		t.Fatalf("expected ErrCouponInvalid, got %v", err)
	}
}

func TestCouponService_Validate_InactiveCoupon(t *testing.T) {
	// Arrange
	coupon := &domain.Coupon{
		Code:   "INACTIVE",
		Type:   domain.Fixed,
		Value:  300,
		Active: false,
	}
	svc := application.NewCouponService(newFakeRepo(coupon))

	// Act
	_, err := svc.Validate(context.Background(), "INACTIVE", 1000)

	// Assert
	if !errors.Is(err, domain.ErrCouponInvalid) {
		t.Fatalf("expected ErrCouponInvalid, got %v", err)
	}
}

func TestCouponService_Validate_UsageLimitReached(t *testing.T) {
	// Arrange
	future := time.Now().Add(24 * time.Hour)
	coupon := &domain.Coupon{
		Code:       "MAXUSED",
		Type:       domain.Percent,
		Value:      15,
		ExpiresAt:  &future,
		UsageLimit: ptr(10),
		UsedCount:  10, // limit reached
		Active:     true,
	}
	svc := application.NewCouponService(newFakeRepo(coupon))

	// Act
	_, err := svc.Validate(context.Background(), "MAXUSED", 1000)

	// Assert
	if !errors.Is(err, domain.ErrCouponInvalid) {
		t.Fatalf("expected ErrCouponInvalid, got %v", err)
	}
}
