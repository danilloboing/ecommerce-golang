package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// QuoteRepo is the Postgres-backed store for checkout_quotes rows. Lines and the
// chosen shipping option are persisted as JSONB snapshots so a confirm can honor
// the exact prices locked at quote time (C3).
type QuoteRepo struct {
	q *queries.Queries
}

var _ application.QuoteRepository = (*QuoteRepo)(nil)

// NewQuoteRepo builds a QuoteRepo from a pgx pool.
func NewQuoteRepo(pool *pgxpool.Pool) *QuoteRepo {
	return &QuoteRepo{q: queries.New(pool)}
}

// Create persists a new quote and returns the stored domain aggregate.
func (r *QuoteRepo) Create(ctx context.Context, in application.NewQuote) (domain.Quote, error) {
	linesJSON, err := json.Marshal(in.Lines)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("checkout quote repo: marshal lines: %w", err)
	}
	shippingJSON, err := json.Marshal(in.Chosen)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("checkout quote repo: marshal shipping: %w", err)
	}

	row, err := r.q.CreateQuote(ctx, queries.CreateQuoteParams{
		UserID:           in.UserID,
		CartFingerprint:  in.CartFingerprint,
		LinesSnapshot:    linesJSON,
		ShippingSnapshot: shippingJSON,
		CouponCode:       nilIfEmpty(in.CouponCode),
		SubtotalCents:    in.Subtotal,
		ShippingCents:    in.Shipping,
		DiscountCents:    in.Discount,
		TotalCents:       in.Total,
		AddressSnapshot:  jsonOrEmptyObject(in.AddressSnapshot),
		ExpiresAt:        in.ExpiresAt,
	})
	if err != nil {
		return domain.Quote{}, fmt.Errorf("checkout quote repo: create: %w", err)
	}

	return mapQuote(row)
}

// GetUserQuote returns the user's quote by id, scoped to the owner.
func (r *QuoteRepo) GetUserQuote(ctx context.Context, id, userID uuid.UUID) (domain.Quote, error) {
	row, err := r.q.GetUserQuote(ctx, queries.GetUserQuoteParams{ID: id, UserID: userID})
	if err != nil {
		return domain.Quote{}, fmt.Errorf("checkout quote repo: get user quote: %w", err)
	}
	return mapQuote(row)
}

// mapQuote converts a persisted checkout_quotes row into the domain aggregate,
// decoding the locked line snapshot back into typed quote lines.
func mapQuote(row queries.CheckoutQuote) (domain.Quote, error) {
	var lines []domain.QuoteLine
	if len(row.LinesSnapshot) > 0 {
		if err := json.Unmarshal(row.LinesSnapshot, &lines); err != nil {
			return domain.Quote{}, fmt.Errorf("checkout quote repo: unmarshal lines: %w", err)
		}
	}

	coupon := ""
	if row.CouponCode != nil {
		coupon = *row.CouponCode
	}

	return domain.Quote{
		ID:               row.ID,
		UserID:           row.UserID,
		CartFingerprint:  row.CartFingerprint,
		Lines:            lines,
		CouponCode:       coupon,
		AddressSnapshot:  json.RawMessage(row.AddressSnapshot),
		ShippingSnapshot: json.RawMessage(row.ShippingSnapshot),
		Subtotal:         row.SubtotalCents,
		Shipping:         row.ShippingCents,
		Discount:         row.DiscountCents,
		Total:            row.TotalCents,
		ExpiresAt:        row.ExpiresAt,
		CreatedAt:        row.CreatedAt,
	}, nil
}
