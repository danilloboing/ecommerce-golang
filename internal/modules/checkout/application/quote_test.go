package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeCartReader satisfies CartReader.
type fakeCartReader struct {
	view application.CartView
	err  error
}

func (f *fakeCartReader) ActiveCart(_ context.Context, _ uuid.UUID) (application.CartView, error) {
	return f.view, f.err
}

// fakePriceReader satisfies PriceReader.
type fakePriceReader struct {
	prices map[uuid.UUID]int64
	err    error
}

func (f *fakePriceReader) UnitPrice(_ context.Context, variantID uuid.UUID) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	p, ok := f.prices[variantID]
	if !ok {
		return 0, errors.New("variant not found")
	}
	return p, nil
}

// fakeShippingQuoter satisfies ShippingQuoter.
type fakeShippingQuoter struct {
	options []application.ShippingOption
	err     error
}

func (f *fakeShippingQuoter) Quote(_ context.Context, _ string, _ int64) ([]application.ShippingOption, error) {
	return f.options, f.err
}

// fakeAddressReader satisfies AddressReader.
type fakeAddressReader struct {
	view application.AddressView
	err  error
}

func (f *fakeAddressReader) Get(_ context.Context, _, _ uuid.UUID) (application.AddressView, error) {
	return f.view, f.err
}

// fakeQuoteRepository satisfies QuoteRepository.
type fakeQuoteRepository struct {
	created []domain.Quote
	byID    map[uuid.UUID]domain.Quote
	err     error
}

func (f *fakeQuoteRepository) Create(_ context.Context, nq application.NewQuote) (domain.Quote, error) {
	if f.err != nil {
		return domain.Quote{}, f.err
	}
	q := domain.Quote{
		ID:              uuid.New(),
		UserID:          nq.UserID,
		CartFingerprint: nq.CartFingerprint,
		Subtotal:        nq.Subtotal,
		Shipping:        nq.Shipping,
		Discount:        nq.Discount,
		Total:           nq.Total,
		ExpiresAt:       nq.ExpiresAt,
		CreatedAt:       time.Now(),
	}
	f.created = append(f.created, q)
	if f.byID == nil {
		f.byID = make(map[uuid.UUID]domain.Quote)
	}
	f.byID[q.ID] = q
	return q, nil
}

func (f *fakeQuoteRepository) GetUserQuote(_ context.Context, id, _ uuid.UUID) (domain.Quote, error) {
	q, ok := f.byID[id]
	if !ok {
		return domain.Quote{}, domain.ErrQuoteNotFound
	}
	return q, nil
}

// fakeCouponSvc is a minimal interface for tests — satisfied by *CouponService.
type fakeCouponSvc struct {
	discount int64
	err      error
}

func (f *fakeCouponSvc) Validate(_ context.Context, _ string, _ int64) (int64, error) {
	return f.discount, f.err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func buildDeps(
	cart *fakeCartReader,
	prices *fakePriceReader,
	shipping *fakeShippingQuoter,
	address *fakeAddressReader,
	quotes *fakeQuoteRepository,
	coupons application.CouponValidator,
) application.CheckoutDeps {
	return application.CheckoutDeps{
		Carts:    cart,
		Prices:   prices,
		Shipping: shipping,
		Addresses: address,
		Quotes:   quotes,
		Coupons:  coupons,
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCheckoutService_Quote_HappyPath(t *testing.T) {
	// Arrange
	variantA := uuid.New()
	variantB := uuid.New()
	userID := uuid.New()
	addrID := uuid.New()

	cart := &fakeCartReader{
		view: application.CartView{
			Lines: []application.CartLine{
				{VariantID: variantA, Quantity: 2},
				{VariantID: variantB, Quantity: 1},
			},
		},
	}
	// variantA: 1000 cents, variantB: 2000 cents → subtotal = 4000
	prices := &fakePriceReader{
		prices: map[uuid.UUID]int64{
			variantA: 1000,
			variantB: 2000,
		},
	}
	shippingOpts := []application.ShippingOption{
		{ServiceID: "PAC", Name: "PAC", PriceCents: 1500, ETADays: 7},
		{ServiceID: "SEDEX", Name: "SEDEX", PriceCents: 3000, ETADays: 2},
	}
	shipper := &fakeShippingQuoter{options: shippingOpts}
	address := &fakeAddressReader{
		view: application.AddressView{PostalCode: "01310-100"},
	}
	quotesRepo := &fakeQuoteRepository{}
	// coupon: 10% off subtotal 4000 → discount 400
	couponSvc := &fakeCouponSvc{discount: 400}

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := application.NewCheckoutService(buildDeps(cart, prices, shipper, address, quotesRepo, couponSvc),
		application.WithNow(func() time.Time { return now }),
		application.WithQuoteTTL(30*time.Minute),
	)

	in := application.QuoteInput{
		UserID:     userID,
		AddressID:  addrID,
		ServiceID:  "PAC",
		CouponCode: "SAVE10",
	}

	// Act
	result, err := svc.Quote(context.Background(), in)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Subtotal != 4000 {
		t.Errorf("expected subtotal 4000, got %d", result.Subtotal)
	}
	if result.Shipping != 1500 {
		t.Errorf("expected shipping 1500 (PAC), got %d", result.Shipping)
	}
	if result.Discount != 400 {
		t.Errorf("expected discount 400, got %d", result.Discount)
	}
	// total = 4000 + 1500 - 400 = 5100
	if result.Total != 5100 {
		t.Errorf("expected total 5100, got %d", result.Total)
	}
	if len(result.Lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(result.Lines))
	}
	if len(result.Options) != 2 {
		t.Errorf("expected 2 shipping options, got %d", len(result.Options))
	}
	if result.Chosen.ServiceID != "PAC" {
		t.Errorf("expected chosen service PAC, got %s", result.Chosen.ServiceID)
	}
	if result.QuoteID == uuid.Nil {
		t.Error("expected non-nil QuoteID")
	}
	expectedExpiry := now.Add(30 * time.Minute)
	if result.ExpiresAt != expectedExpiry {
		t.Errorf("expected ExpiresAt %v, got %v", expectedExpiry, result.ExpiresAt)
	}

	// Verify quote was persisted
	if len(quotesRepo.created) != 1 {
		t.Fatalf("expected 1 persisted quote, got %d", len(quotesRepo.created))
	}
	persisted := quotesRepo.created[0]
	if persisted.Total != 5100 {
		t.Errorf("persisted total mismatch: got %d", persisted.Total)
	}
}

func TestCheckoutService_Quote_DefaultCheapestShipping(t *testing.T) {
	// Arrange: no ServiceID given → must pick cheapest option
	variantA := uuid.New()
	userID := uuid.New()
	addrID := uuid.New()

	cart := &fakeCartReader{
		view: application.CartView{
			Lines: []application.CartLine{
				{VariantID: variantA, Quantity: 1},
			},
		},
	}
	prices := &fakePriceReader{prices: map[uuid.UUID]int64{variantA: 5000}}
	shippingOpts := []application.ShippingOption{
		{ServiceID: "SEDEX", Name: "SEDEX", PriceCents: 3000, ETADays: 2},
		{ServiceID: "PAC", Name: "PAC", PriceCents: 800, ETADays: 7},
	}
	shipper := &fakeShippingQuoter{options: shippingOpts}
	address := &fakeAddressReader{view: application.AddressView{PostalCode: "01310-100"}}
	quotesRepo := &fakeQuoteRepository{}
	couponSvc := &fakeCouponSvc{} // no coupon

	svc := application.NewCheckoutService(buildDeps(cart, prices, shipper, address, quotesRepo, couponSvc),
		application.WithNow(func() time.Time { return time.Now() }),
		application.WithQuoteTTL(15*time.Minute),
	)

	in := application.QuoteInput{
		UserID:    userID,
		AddressID: addrID,
		// ServiceID empty → default cheapest
	}

	// Act
	result, err := svc.Quote(context.Background(), in)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// cheapest is PAC at 800
	if result.Chosen.ServiceID != "PAC" {
		t.Errorf("expected cheapest PAC, got %s", result.Chosen.ServiceID)
	}
	if result.Shipping != 800 {
		t.Errorf("expected shipping 800, got %d", result.Shipping)
	}
}

func TestCheckoutService_Quote_EmptyCart(t *testing.T) {
	// Arrange
	cart := &fakeCartReader{
		view: application.CartView{Lines: []application.CartLine{}},
	}
	prices := &fakePriceReader{prices: map[uuid.UUID]int64{}}
	shipper := &fakeShippingQuoter{}
	address := &fakeAddressReader{}
	quotesRepo := &fakeQuoteRepository{}
	couponSvc := &fakeCouponSvc{}

	svc := application.NewCheckoutService(buildDeps(cart, prices, shipper, address, quotesRepo, couponSvc),
		application.WithNow(func() time.Time { return time.Now() }),
		application.WithQuoteTTL(15*time.Minute),
	)

	in := application.QuoteInput{
		UserID:    uuid.New(),
		AddressID: uuid.New(),
	}

	// Act
	_, err := svc.Quote(context.Background(), in)

	// Assert
	if !errors.Is(err, domain.ErrCartEmpty) {
		t.Fatalf("expected ErrCartEmpty, got %v", err)
	}
}

func TestCheckoutService_Quote_NoCoupon(t *testing.T) {
	// Arrange: no coupon code → discount = 0
	variantA := uuid.New()
	cart := &fakeCartReader{
		view: application.CartView{
			Lines: []application.CartLine{
				{VariantID: variantA, Quantity: 3},
			},
		},
	}
	prices := &fakePriceReader{prices: map[uuid.UUID]int64{variantA: 1000}}
	shippingOpts := []application.ShippingOption{
		{ServiceID: "PAC", Name: "PAC", PriceCents: 500, ETADays: 7},
	}
	shipper := &fakeShippingQuoter{options: shippingOpts}
	address := &fakeAddressReader{view: application.AddressView{PostalCode: "01310-100"}}
	quotesRepo := &fakeQuoteRepository{}
	couponSvc := &fakeCouponSvc{} // no coupon

	svc := application.NewCheckoutService(buildDeps(cart, prices, shipper, address, quotesRepo, couponSvc),
		application.WithNow(func() time.Time { return time.Now() }),
		application.WithQuoteTTL(15*time.Minute),
	)

	in := application.QuoteInput{
		UserID:     uuid.New(),
		AddressID:  uuid.New(),
		CouponCode: "", // empty
	}

	// Act
	result, err := svc.Quote(context.Background(), in)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Discount != 0 {
		t.Errorf("expected discount 0, got %d", result.Discount)
	}
	// total = 3000 + 500 - 0 = 3500
	if result.Total != 3500 {
		t.Errorf("expected total 3500, got %d", result.Total)
	}
}

func TestCheckoutService_Quote_InvalidServiceID(t *testing.T) {
	// Arrange: ServiceID given but not in returned options → error
	variantA := uuid.New()
	cart := &fakeCartReader{
		view: application.CartView{
			Lines: []application.CartLine{
				{VariantID: variantA, Quantity: 1},
			},
		},
	}
	prices := &fakePriceReader{prices: map[uuid.UUID]int64{variantA: 2000}}
	shippingOpts := []application.ShippingOption{
		{ServiceID: "PAC", Name: "PAC", PriceCents: 500, ETADays: 7},
	}
	shipper := &fakeShippingQuoter{options: shippingOpts}
	address := &fakeAddressReader{view: application.AddressView{PostalCode: "01310-100"}}
	quotesRepo := &fakeQuoteRepository{}
	couponSvc := &fakeCouponSvc{}

	svc := application.NewCheckoutService(buildDeps(cart, prices, shipper, address, quotesRepo, couponSvc),
		application.WithNow(func() time.Time { return time.Now() }),
		application.WithQuoteTTL(15*time.Minute),
	)

	in := application.QuoteInput{
		UserID:    uuid.New(),
		AddressID: uuid.New(),
		ServiceID: "UNKNOWN_SVC",
	}

	// Act
	_, err := svc.Quote(context.Background(), in)

	// Assert: should return an error (shipping service not available)
	if err == nil {
		t.Fatal("expected error for unknown service ID, got nil")
	}
}

func TestCheckoutService_Quote_FingerprintDeterministic(t *testing.T) {
	// Two quotes with the same cart lines in different order must produce
	// the same fingerprint (sorted before hashing).
	variantA := uuid.New()
	variantB := uuid.New()
	userID := uuid.New()
	addrID := uuid.New()

	shippingOpts := []application.ShippingOption{
		{ServiceID: "PAC", Name: "PAC", PriceCents: 500, ETADays: 7},
	}
	prices := &fakePriceReader{prices: map[uuid.UUID]int64{
		variantA: 1000,
		variantB: 2000,
	}}
	address := &fakeAddressReader{view: application.AddressView{PostalCode: "01310-100"}}
	couponSvc := &fakeCouponSvc{}

	svc1Cart := &fakeCartReader{view: application.CartView{Lines: []application.CartLine{
		{VariantID: variantA, Quantity: 1},
		{VariantID: variantB, Quantity: 2},
	}}}
	repo1 := &fakeQuoteRepository{}
	svc1 := application.NewCheckoutService(buildDeps(svc1Cart, prices, &fakeShippingQuoter{options: shippingOpts}, address, repo1, couponSvc),
		application.WithNow(func() time.Time { return time.Now() }),
		application.WithQuoteTTL(15*time.Minute),
	)

	svc2Cart := &fakeCartReader{view: application.CartView{Lines: []application.CartLine{
		{VariantID: variantB, Quantity: 2},
		{VariantID: variantA, Quantity: 1},
	}}}
	repo2 := &fakeQuoteRepository{}
	svc2 := application.NewCheckoutService(buildDeps(svc2Cart, prices, &fakeShippingQuoter{options: shippingOpts}, address, repo2, couponSvc),
		application.WithNow(func() time.Time { return time.Now() }),
		application.WithQuoteTTL(15*time.Minute),
	)

	in := application.QuoteInput{UserID: userID, AddressID: addrID}
	_, err := svc1.Quote(context.Background(), in)
	if err != nil {
		t.Fatalf("svc1 quote error: %v", err)
	}
	_, err = svc2.Quote(context.Background(), in)
	if err != nil {
		t.Fatalf("svc2 quote error: %v", err)
	}

	if len(repo1.created) == 0 || len(repo2.created) == 0 {
		t.Fatal("expected quotes persisted")
	}
	fp1 := repo1.created[0].CartFingerprint
	fp2 := repo2.created[0].CartFingerprint
	if fp1 != fp2 {
		t.Errorf("fingerprints differ: %q vs %q", fp1, fp2)
	}
}
