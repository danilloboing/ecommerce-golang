package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

const defaultQuoteTTL = 30 * time.Minute

// CheckoutDeps groups all port dependencies for CheckoutService.
type CheckoutDeps struct {
	Carts     CartReader
	Prices    PriceReader
	Shipping  ShippingQuoter
	Addresses AddressReader
	Quotes    QuoteRepository
	Coupons   CouponValidator
}

// CheckoutService orchestrates the checkout quote flow.
type CheckoutService struct {
	carts     CartReader
	prices    PriceReader
	shipping  ShippingQuoter
	addresses AddressReader
	quotes    QuoteRepository
	coupons   CouponValidator
	now       func() time.Time
	quoteTTL  time.Duration
}

// Option is a functional option for CheckoutService.
type Option func(*CheckoutService)

// WithNow overrides the clock (useful in tests).
func WithNow(fn func() time.Time) Option {
	return func(s *CheckoutService) { s.now = fn }
}

// WithQuoteTTL sets how long a quote stays valid.
func WithQuoteTTL(d time.Duration) Option {
	return func(s *CheckoutService) { s.quoteTTL = d }
}

// NewCheckoutService builds a CheckoutService with the given ports.
func NewCheckoutService(deps CheckoutDeps, opts ...Option) *CheckoutService {
	s := &CheckoutService{
		carts:     deps.Carts,
		prices:    deps.Prices,
		shipping:  deps.Shipping,
		addresses: deps.Addresses,
		quotes:    deps.Quotes,
		coupons:   deps.Coupons,
		now:       time.Now,
		quoteTTL:  defaultQuoteTTL,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Quote computes a pricing snapshot for the user's active cart.
//
// Invariants enforced:
//   - Prices are always fetched from PriceReader (C3 — client never supplies price).
//   - Shipping price is always re-derived server-side (C3).
//   - Cart fingerprint is deterministic: sha256 of sorted "variantID:qty" pairs.
//   - The resulting quote row is persisted before returning.
func (s *CheckoutService) Quote(ctx context.Context, in QuoteInput) (QuoteResult, error) {
	cart, err := s.carts.ActiveCart(ctx, in.UserID)
	if err != nil {
		return QuoteResult{}, err
	}
	if len(cart.Lines) == 0 {
		return QuoteResult{}, domain.ErrCartEmpty
	}

	addr, err := s.addresses.Get(ctx, in.AddressID, in.UserID)
	if err != nil {
		return QuoteResult{}, err
	}

	var subtotal int64
	lines := make([]QuoteLine, 0, len(cart.Lines))
	for _, l := range cart.Lines {
		price, err := s.prices.UnitPrice(ctx, l.VariantID)
		if err != nil {
			return QuoteResult{}, err
		}
		subtotal += price * int64(l.Quantity)
		lines = append(lines, QuoteLine{
			VariantID:      l.VariantID,
			Quantity:       l.Quantity,
			UnitPriceCents: price,
		})
	}

	opts, err := s.shipping.Quote(ctx, addr.PostalCode, subtotal)
	if err != nil {
		return QuoteResult{}, err
	}

	chosen, err := pickShipping(opts, in.ServiceID)
	if err != nil {
		return QuoteResult{}, err
	}

	var discount int64
	if in.CouponCode != "" {
		discount, err = s.coupons.Validate(ctx, in.CouponCode, subtotal)
		if err != nil {
			return QuoteResult{}, err
		}
	}

	total := domain.ComputeTotal(subtotal, chosen.PriceCents, discount)
	fp := fingerprint(cart.Lines)

	q, err := s.quotes.Create(ctx, NewQuote{
		UserID:          in.UserID,
		CartFingerprint: fp,
		Lines:           lines,
		Chosen:          chosen,
		CouponCode:      in.CouponCode,
		Subtotal:        subtotal,
		Shipping:        chosen.PriceCents,
		Discount:        discount,
		Total:           total,
		ExpiresAt:       s.now().Add(s.quoteTTL),
	})
	if err != nil {
		return QuoteResult{}, err
	}

	return QuoteResult{
		QuoteID:   q.ID,
		Lines:     lines,
		Options:   opts,
		Chosen:    chosen,
		Subtotal:  subtotal,
		Shipping:  chosen.PriceCents,
		Discount:  discount,
		Total:     total,
		ExpiresAt: q.ExpiresAt,
	}, nil
}

// pickShipping returns the requested shipping option, or the cheapest one when
// serviceID is empty. Returns an error when a specific serviceID is not found.
func pickShipping(opts []ShippingOption, serviceID string) (ShippingOption, error) {
	if serviceID == "" {
		if len(opts) == 0 {
			return ShippingOption{}, fmt.Errorf("checkout: no shipping options available")
		}
		cheapest := opts[0]
		for _, o := range opts[1:] {
			if o.PriceCents < cheapest.PriceCents {
				cheapest = o
			}
		}
		return cheapest, nil
	}
	for _, o := range opts {
		if o.ServiceID == serviceID {
			return o, nil
		}
	}
	return ShippingOption{}, fmt.Errorf("checkout: shipping service %q not available", serviceID)
}

// fingerprint returns hex(sha256) of sorted "variantID:qty" entries.
// Sorting ensures the same cart lines produce the same fingerprint regardless
// of the order in which they were added.
func fingerprint(lines []CartLine) string {
	entries := make([]string, len(lines))
	for i, l := range lines {
		entries[i] = fmt.Sprintf("%s:%d", l.VariantID.String(), l.Quantity)
	}
	sort.Strings(entries)
	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e))
	}
	return hex.EncodeToString(h.Sum(nil))
}
