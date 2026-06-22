package transport_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	checkoutdomain "github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/transport"
	addrdomain "github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
	orderingdomain "github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

type fakeCheckoutUseCase struct {
	quoteResult   application.QuoteResult
	quoteErr      error
	confirmResult application.ConfirmResult
	confirmErr    error
}

func (f *fakeCheckoutUseCase) Quote(_ context.Context, _ application.QuoteInput) (application.QuoteResult, error) {
	return f.quoteResult, f.quoteErr
}

func (f *fakeCheckoutUseCase) Confirm(_ context.Context, _ application.ConfirmInput) (application.ConfirmResult, error) {
	return f.confirmResult, f.confirmErr
}

// withSession injects a session into the request context.
func withSession(s sessionauth.Session) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := sessionauth.ContextWithSession(r.Context(), s)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// withDiscardLogger injects a no-op slog logger so responsex.ErrorWithCause
// does not emit WARN/ERROR lines to stderr during tests.
func withDiscardLogger(next http.Handler) http.Handler {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := observability.WithLogger(r.Context(), discard)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// buildRouter builds a chi router with both checkout routes mounted, optionally
// injecting a session middleware.
func buildRouter(uc transport.CheckoutUseCase, sess *sessionauth.Session) chi.Router {
	h := transport.NewCheckoutHandlers(uc)
	r := chi.NewRouter()
	r.Use(withDiscardLogger)
	if sess != nil {
		r.Use(withSession(*sess))
	}
	h.RegisterCheckoutRoutes(r)
	return r
}

// ---------------------------------------------------------------------------
// POST /checkout/quote
// ---------------------------------------------------------------------------

func TestQuote_Happy(t *testing.T) {
	userID := uuid.New()
	quoteID := uuid.New()
	addressID := uuid.New()

	result := application.QuoteResult{
		QuoteID:   quoteID,
		Lines:     []checkoutdomain.QuoteLine{{VariantID: uuid.New(), Quantity: 1, UnitPriceCents: 5000}},
		Subtotal:  5000,
		Shipping:  1500,
		Discount:  0,
		Total:     6500,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	svc := &fakeCheckoutUseCase{quoteResult: result}
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"shipping_address_id":"` + addressID.String() + `"}`)
	resp, err := http.Post(srv.URL+"/checkout/quote", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var out transport.QuoteResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Equal(t, quoteID, out.QuoteID)
	assert.Equal(t, int64(6500), out.Total)
}

func TestQuote_NoSession_401(t *testing.T) {
	svc := &fakeCheckoutUseCase{}
	r := buildRouter(svc, nil)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"shipping_address_id":"` + uuid.NewString() + `"}`)
	resp, err := http.Post(srv.URL+"/checkout/quote", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestQuote_CartEmpty_422(t *testing.T) {
	svc := &fakeCheckoutUseCase{quoteErr: checkoutdomain.ErrCartEmpty}
	userID := uuid.New()
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"shipping_address_id":"` + uuid.NewString() + `"}`)
	resp, err := http.Post(srv.URL+"/checkout/quote", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	var errBody struct {
		Error struct{ Code string `json:"code"` } `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	assert.Equal(t, "cart_empty", errBody.Error.Code)
}

func TestQuote_AddressNotFound_404(t *testing.T) {
	svc := &fakeCheckoutUseCase{quoteErr: addrdomain.ErrAddressNotFound}
	userID := uuid.New()
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"shipping_address_id":"` + uuid.NewString() + `"}`)
	resp, err := http.Post(srv.URL+"/checkout/quote", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	var errBody struct {
		Error struct{ Code string `json:"code"` } `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	assert.Equal(t, "not_found", errBody.Error.Code)
}

func TestQuote_InsufficientStock_422(t *testing.T) {
	svc := &fakeCheckoutUseCase{quoteErr: checkoutdomain.ErrInsufficientStock}
	userID := uuid.New()
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"shipping_address_id":"` + uuid.NewString() + `"}`)
	resp, err := http.Post(srv.URL+"/checkout/quote", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	var errBody struct {
		Error struct{ Code string `json:"code"` } `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	assert.Equal(t, "insufficient_stock", errBody.Error.Code)
}

func TestQuote_CouponInvalid_422(t *testing.T) {
	svc := &fakeCheckoutUseCase{quoteErr: checkoutdomain.ErrCouponInvalid}
	userID := uuid.New()
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"shipping_address_id":"` + uuid.NewString() + `","coupon_code":"BAD"}`)
	resp, err := http.Post(srv.URL+"/checkout/quote", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// POST /checkout/confirm
// ---------------------------------------------------------------------------

func TestConfirm_MissingIdempotencyKey_400(t *testing.T) {
	svc := &fakeCheckoutUseCase{}
	userID := uuid.New()
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"quote_id":"` + uuid.NewString() + `"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/checkout/confirm", body)
	req.Header.Set("Content-Type", "application/json")
	// Deliberately no Idempotency-Key header

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var errBody struct {
		Error struct{ Code string `json:"code"` } `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	assert.Equal(t, "missing_idempotency_key", errBody.Error.Code)
}

func TestConfirm_Happy_201(t *testing.T) {
	userID := uuid.New()
	quoteID := uuid.New()
	orderID := uuid.New()

	result := application.ConfirmResult{
		Order: orderingdomain.Order{
			ID:       orderID,
			UserID:   userID,
			Status:   "pending_payment",
			Subtotal: 5000,
			Shipping: 1500,
			Discount: 0,
			Total:    6500,
		},
		Charge: application.ChargeView{
			ChargeID: uuid.New(),
			OrderID:  orderID,
			Amount:   6500,
			Method:   "pix",
			Status:   "pending",
		},
	}
	svc := &fakeCheckoutUseCase{confirmResult: result}
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"quote_id":"` + quoteID.String() + `"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/checkout/confirm", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var out transport.ConfirmResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Equal(t, orderID, out.OrderID)
	assert.Equal(t, int64(6500), out.Total)
}

func TestConfirm_NoSession_401(t *testing.T) {
	svc := &fakeCheckoutUseCase{}
	r := buildRouter(svc, nil)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"quote_id":"` + uuid.NewString() + `"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/checkout/confirm", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestConfirm_QuoteExpired_409(t *testing.T) {
	svc := &fakeCheckoutUseCase{confirmErr: checkoutdomain.ErrQuoteExpired}
	userID := uuid.New()
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"quote_id":"` + uuid.NewString() + `"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/checkout/confirm", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	var errBody struct {
		Error struct{ Code string `json:"code"` } `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	assert.Equal(t, "quote_expired", errBody.Error.Code)
}

func TestConfirm_IdempotencyConflict_409(t *testing.T) {
	svc := &fakeCheckoutUseCase{confirmErr: checkoutdomain.ErrIdempotencyConflict}
	userID := uuid.New()
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"quote_id":"` + uuid.NewString() + `"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/checkout/confirm", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	var errBody struct {
		Error struct{ Code string `json:"code"` } `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	assert.Equal(t, "idempotency_conflict", errBody.Error.Code)
}

func TestConfirm_CartChanged_409(t *testing.T) {
	svc := &fakeCheckoutUseCase{confirmErr: checkoutdomain.ErrCartChanged}
	userID := uuid.New()
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"quote_id":"` + uuid.NewString() + `"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/checkout/confirm", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestConfirm_CouponUnavailable_422(t *testing.T) {
	svc := &fakeCheckoutUseCase{confirmErr: checkoutdomain.ErrCouponUnavailable}
	userID := uuid.New()
	sess := sessionauth.Session{ID: "sid", UserID: userID}
	r := buildRouter(svc, &sess)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := strings.NewReader(`{"quote_id":"` + uuid.NewString() + `"}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/checkout/confirm", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	var errBody struct {
		Error struct{ Code string `json:"code"` } `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	assert.Equal(t, "coupon_unavailable", errBody.Error.Code)
}
