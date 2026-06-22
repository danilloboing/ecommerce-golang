package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// CouponRepo is the Postgres-backed coupon store. Redeem and Release are atomic
// conditional updates so concurrent confirms cannot oversell a usage-limited
// code (C4). Phase 3a tracks usage globally by code; per-user is Phase 5.
type CouponRepo struct {
	q *queries.Queries
}

var _ application.CouponRepository = (*CouponRepo)(nil)

// NewCouponRepo builds a CouponRepo from a pgx pool.
func NewCouponRepo(pool *pgxpool.Pool) *CouponRepo {
	return &CouponRepo{q: queries.New(pool)}
}

// GetByCode returns the coupon for a code, or ErrCouponInvalid when absent.
func (r *CouponRepo) GetByCode(ctx context.Context, code string) (*domain.Coupon, error) {
	row, err := r.q.GetCouponByCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrCouponInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("checkout coupon repo: get by code: %w", err)
	}
	coupon := mapCoupon(row)
	return &coupon, nil
}

// Redeem atomically increments used_count, mapping a no-op (coupon inactive,
// expired, or at its usage limit) to ErrCouponUnavailable.
func (r *CouponRepo) Redeem(ctx context.Context, code string) error {
	_, err := r.q.RedeemCoupon(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrCouponUnavailable
	}
	if err != nil {
		return fmt.Errorf("checkout coupon repo: redeem: %w", err)
	}
	return nil
}

// Release decrements used_count, compensating a redeemed coupon when the
// surrounding confirm is rolled back outside its transaction.
func (r *CouponRepo) Release(ctx context.Context, code string) error {
	if err := r.q.ReleaseCoupon(ctx, code); err != nil {
		return fmt.Errorf("checkout coupon repo: release: %w", err)
	}
	return nil
}

// Create persists a new coupon and returns the stored aggregate.
func (r *CouponRepo) Create(ctx context.Context, in application.NewCoupon) (*domain.Coupon, error) {
	row, err := r.q.CreateCoupon(ctx, queries.CreateCouponParams{
		Code:          in.Code,
		Type:          string(in.Type),
		Value:         in.Value,
		ExpiresAt:     in.ExpiresAt,
		UsageLimit:    intPtrToInt32Ptr(in.UsageLimit),
		MinOrderCents: in.MinOrderCents,
	})
	if err != nil {
		return nil, fmt.Errorf("checkout coupon repo: create: %w", err)
	}
	coupon := mapCoupon(row)
	return &coupon, nil
}

// mapCoupon converts a persisted coupons row into the domain aggregate.
func mapCoupon(row queries.Coupon) domain.Coupon {
	return domain.Coupon{
		Code:          row.Code,
		Type:          domain.CouponType(row.Type),
		Value:         row.Value,
		ExpiresAt:     row.ExpiresAt,
		UsageLimit:    int32PtrToIntPtr(row.UsageLimit),
		UsedCount:     int(row.UsedCount),
		MinOrderCents: row.MinOrderCents,
		Active:        row.Active,
	}
}

// intPtrToInt32Ptr narrows a nullable int into a nullable int32 for sqlc params.
func intPtrToInt32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

// int32PtrToIntPtr widens a nullable int32 from a row into a nullable int.
func int32PtrToIntPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}
