package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
	orderingDomain "github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
)

// ---------------------------------------------------------------------------
// Fakes for Confirm-specific ports
// ---------------------------------------------------------------------------

// fakeIdempotency satisfies Idempotency.
type fakeIdempotency struct {
	hit application.IdemHit
	err error
}

func (f *fakeIdempotency) Lookup(_ context.Context, _ uuid.UUID, _, _ string) (application.IdemHit, error) {
	return f.hit, f.err
}

// fakeCharger satisfies Charger.
type fakeCharger struct {
	view application.ChargeView
	err  error
}

func (f *fakeCharger) CreateCharge(_ context.Context, orderID uuid.UUID, amount int64, method string) (application.ChargeView, error) {
	if f.err != nil {
		return application.ChargeView{}, f.err
	}
	v := f.view
	if v.OrderID == uuid.Nil {
		v.OrderID = orderID
	}
	if v.Amount == 0 {
		v.Amount = amount
	}
	if v.Method == "" {
		v.Method = method
	}
	return v, nil
}

// fakeConfirmRepo satisfies ConfirmRepository.
type fakeConfirmRepo struct {
	order     orderingDomain.Order
	err       error
	callCount int
}

func (f *fakeConfirmRepo) ConfirmTx(_ context.Context, _ application.ConfirmPlan) (orderingDomain.Order, error) {
	f.callCount++
	return f.order, f.err
}

// fakeConfirmQuoteRepo satisfies QuoteRepository for Confirm tests.
// It returns a pre-configured quote by ID or an error.
type fakeConfirmQuoteRepo struct {
	quote domain.Quote
	err   error
}

func (f *fakeConfirmQuoteRepo) Create(_ context.Context, _ application.NewQuote) (domain.Quote, error) {
	return domain.Quote{}, errors.New("not implemented in confirm tests")
}

func (f *fakeConfirmQuoteRepo) GetUserQuote(_ context.Context, id, _ uuid.UUID) (domain.Quote, error) {
	if f.err != nil {
		return domain.Quote{}, f.err
	}
	if f.quote.ID != id {
		return domain.Quote{}, domain.ErrQuoteNotFound
	}
	return f.quote, nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// buildConfirmService assembles a minimal CheckoutService for Confirm tests.
func buildConfirmService(
	t *testing.T,
	idem *fakeIdempotency,
	quotesRepo application.QuoteRepository,
	cartReader *fakeCartReader,
	charger *fakeCharger,
	confirmRepo *fakeConfirmRepo,
	nowFn func() time.Time,
) *application.CheckoutService {
	t.Helper()
	deps := application.CheckoutDeps{
		Carts:       cartReader,
		Prices:      &fakePriceReader{prices: map[uuid.UUID]int64{}},
		Shipping:    &fakeShippingQuoter{},
		Addresses:   &fakeAddressReader{},
		Quotes:      quotesRepo,
		Coupons:     &fakeCouponSvc{},
		ConfirmRepo: confirmRepo,
		Idempotency: idem,
		Charger:     charger,
	}
	return application.NewCheckoutService(deps, application.WithNow(nowFn))
}

// makeQuote creates a domain.Quote with the given fingerprint, valid until expiresAt.
func makeQuote(userID uuid.UUID, fp string, expiresAt time.Time) domain.Quote {
	return domain.Quote{
		ID:              uuid.New(),
		UserID:          userID,
		CartFingerprint: fp,
		Total:           5000,
		ExpiresAt:       expiresAt,
		CreatedAt:       time.Now(),
	}
}

// computeTestFingerprint mirrors the internal fingerprint logic so tests can
// build matching cart fingerprints without exporting the internal function.
func computeTestFingerprint(lines []application.CartLine) string {
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// (a) Idempotency replay: same key+hash → returns stored result, ConfirmTx NOT called.
func TestCheckoutService_Confirm_IdempotencyReplay(t *testing.T) {
	userID := uuid.New()
	quoteID := uuid.New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	storedOrder := orderingDomain.Order{ID: uuid.New(), UserID: userID}
	storedCharge := application.ChargeView{ChargeID: uuid.New(), Amount: 5000}
	storedResult := &application.ConfirmResult{Order: storedOrder, Charge: storedCharge}

	idem := &fakeIdempotency{
		hit: application.IdemHit{Found: true, Replay: true, StoredResult: storedResult},
	}
	confirmRepo := &fakeConfirmRepo{}

	svc := buildConfirmService(t,
		idem,
		&fakeConfirmQuoteRepo{},
		&fakeCartReader{},
		&fakeCharger{},
		confirmRepo,
		func() time.Time { return now },
	)

	in := application.ConfirmInput{
		UserID:         userID,
		QuoteID:        quoteID,
		IdempotencyKey: "key-abc",
		PaymentMethod:  "pix",
	}

	result, err := svc.Confirm(context.Background(), in)
	if err != nil {
		t.Fatalf("expected no error on replay, got %v", err)
	}
	if result.Order.ID != storedOrder.ID {
		t.Errorf("expected stored order ID %s, got %s", storedOrder.ID, result.Order.ID)
	}
	if result.Charge.ChargeID != storedCharge.ChargeID {
		t.Errorf("expected stored charge ID %s, got %s", storedCharge.ChargeID, result.Charge.ChargeID)
	}
	if confirmRepo.callCount != 0 {
		t.Errorf("expected ConfirmTx NOT called on replay, but was called %d time(s)", confirmRepo.callCount)
	}
}

// (b) Key reused with different quote → ErrIdempotencyConflict.
func TestCheckoutService_Confirm_IdempotencyConflict(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	idem := &fakeIdempotency{
		hit: application.IdemHit{Found: true, Conflict: true},
	}
	confirmRepo := &fakeConfirmRepo{}

	svc := buildConfirmService(t,
		idem,
		&fakeConfirmQuoteRepo{},
		&fakeCartReader{},
		&fakeCharger{},
		confirmRepo,
		func() time.Time { return now },
	)

	in := application.ConfirmInput{
		UserID:         userID,
		QuoteID:        uuid.New(),
		IdempotencyKey: "key-conflict",
		PaymentMethod:  "pix",
	}

	_, err := svc.Confirm(context.Background(), in)
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("expected ErrIdempotencyConflict, got %v", err)
	}
	if confirmRepo.callCount != 0 {
		t.Errorf("expected ConfirmTx NOT called, but was called %d time(s)", confirmRepo.callCount)
	}
}

// (c) Quote not found → ErrQuoteNotFound.
func TestCheckoutService_Confirm_QuoteNotFound(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	idem := &fakeIdempotency{hit: application.IdemHit{}}
	quotesRepo := &fakeConfirmQuoteRepo{err: domain.ErrQuoteNotFound}

	svc := buildConfirmService(t,
		idem,
		quotesRepo,
		&fakeCartReader{},
		&fakeCharger{},
		&fakeConfirmRepo{},
		func() time.Time { return now },
	)

	in := application.ConfirmInput{
		UserID:         userID,
		QuoteID:        uuid.New(),
		IdempotencyKey: "key-notfound",
		PaymentMethod:  "pix",
	}

	_, err := svc.Confirm(context.Background(), in)
	if !errors.Is(err, domain.ErrQuoteNotFound) {
		t.Fatalf("expected ErrQuoteNotFound, got %v", err)
	}
}

// (c) Expired quote → ErrQuoteExpired.
func TestCheckoutService_Confirm_QuoteExpired(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	idem := &fakeIdempotency{hit: application.IdemHit{}}

	expiredQuote := makeQuote(userID, "fp", now.Add(-1*time.Second)) // expired 1s ago
	quotesRepo := &fakeConfirmQuoteRepo{quote: expiredQuote}

	svc := buildConfirmService(t,
		idem,
		quotesRepo,
		&fakeCartReader{},
		&fakeCharger{},
		&fakeConfirmRepo{},
		func() time.Time { return now },
	)

	in := application.ConfirmInput{
		UserID:         userID,
		QuoteID:        expiredQuote.ID,
		IdempotencyKey: "key-expired",
		PaymentMethod:  "pix",
	}

	_, err := svc.Confirm(context.Background(), in)
	if !errors.Is(err, domain.ErrQuoteExpired) {
		t.Fatalf("expected ErrQuoteExpired, got %v", err)
	}
}

// (d) Cart fingerprint mismatch → ErrCartChanged.
func TestCheckoutService_Confirm_CartChanged(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	idem := &fakeIdempotency{hit: application.IdemHit{}}

	// Quote has fingerprint "quote-fp", but cart produces a different fingerprint.
	q := makeQuote(userID, "quote-fingerprint-that-wont-match", now.Add(30*time.Minute))
	quotesRepo := &fakeConfirmQuoteRepo{quote: q}

	// Cart lines that produce a different fingerprint than the quote.
	variantX := uuid.New()
	cartReader := &fakeCartReader{view: application.CartView{Lines: []application.CartLine{
		{VariantID: variantX, Quantity: 99},
	}}}

	svc := buildConfirmService(t,
		idem,
		quotesRepo,
		cartReader,
		&fakeCharger{},
		&fakeConfirmRepo{},
		func() time.Time { return now },
	)

	in := application.ConfirmInput{
		UserID:         userID,
		QuoteID:        q.ID,
		IdempotencyKey: "key-cartchanged",
		PaymentMethod:  "pix",
	}

	_, err := svc.Confirm(context.Background(), in)
	if !errors.Is(err, domain.ErrCartChanged) {
		t.Fatalf("expected ErrCartChanged, got %v", err)
	}
}

// (e) ConfirmTx returns ErrInsufficientStock → propagated.
func TestCheckoutService_Confirm_InsufficientStock(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	idem := &fakeIdempotency{hit: application.IdemHit{}}

	variantA := uuid.New()
	lines := []application.CartLine{{VariantID: variantA, Quantity: 2}}
	cartReader := &fakeCartReader{view: application.CartView{Lines: lines}}
	fp := computeTestFingerprint(lines)

	q := makeQuote(userID, fp, now.Add(30*time.Minute))
	quotesRepo := &fakeConfirmQuoteRepo{quote: q}

	charge := application.ChargeView{ChargeID: uuid.New(), Amount: q.Total, Method: "pix"}
	charger := &fakeCharger{view: charge}
	confirmRepo := &fakeConfirmRepo{err: domain.ErrInsufficientStock}

	svc := buildConfirmService(t,
		idem,
		quotesRepo,
		cartReader,
		charger,
		confirmRepo,
		func() time.Time { return now },
	)

	in := application.ConfirmInput{
		UserID:         userID,
		QuoteID:        q.ID,
		IdempotencyKey: "key-stock",
		PaymentMethod:  "pix",
	}

	_, err := svc.Confirm(context.Background(), in)
	if !errors.Is(err, domain.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}
}

// (f) Happy path → returns order + charge.
func TestCheckoutService_Confirm_HappyPath(t *testing.T) {
	userID := uuid.New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	idem := &fakeIdempotency{hit: application.IdemHit{}} // no prior key

	variantA := uuid.New()
	lines := []application.CartLine{{VariantID: variantA, Quantity: 1}}
	cartReader := &fakeCartReader{view: application.CartView{Lines: lines}}
	fp := computeTestFingerprint(lines)

	q := makeQuote(userID, fp, now.Add(30*time.Minute))
	quotesRepo := &fakeConfirmQuoteRepo{quote: q}

	expectedCharge := application.ChargeView{
		ChargeID: uuid.New(),
		Amount:   q.Total,
		Method:   "pix",
		Status:   "pending",
	}
	charger := &fakeCharger{view: expectedCharge}

	expectedOrder := orderingDomain.Order{
		ID:     uuid.New(),
		UserID: userID,
		Total:  q.Total,
	}
	confirmRepo := &fakeConfirmRepo{order: expectedOrder}

	svc := buildConfirmService(t,
		idem,
		quotesRepo,
		cartReader,
		charger,
		confirmRepo,
		func() time.Time { return now },
	)

	in := application.ConfirmInput{
		UserID:         userID,
		QuoteID:        q.ID,
		IdempotencyKey: "key-happy",
		PaymentMethod:  "pix",
	}

	result, err := svc.Confirm(context.Background(), in)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Order.ID != expectedOrder.ID {
		t.Errorf("expected order ID %s, got %s", expectedOrder.ID, result.Order.ID)
	}
	if result.Charge.ChargeID != expectedCharge.ChargeID {
		t.Errorf("expected charge ID %s, got %s", expectedCharge.ChargeID, result.Charge.ChargeID)
	}
	if confirmRepo.callCount != 1 {
		t.Errorf("expected ConfirmTx called exactly once, got %d", confirmRepo.callCount)
	}
}
