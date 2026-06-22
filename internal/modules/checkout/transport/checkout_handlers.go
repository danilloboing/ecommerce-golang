package transport

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
)

// CheckoutUseCase is the slice of CheckoutService consumed by handlers.
// Satisfied by *application.CheckoutService.
type CheckoutUseCase interface {
	Quote(ctx context.Context, in application.QuoteInput) (application.QuoteResult, error)
	Confirm(ctx context.Context, in application.ConfirmInput) (application.ConfirmResult, error)
}

// CheckoutHandlers exposes the authenticated /checkout surface.
type CheckoutHandlers struct {
	svc CheckoutUseCase
}

// NewCheckoutHandlers builds CheckoutHandlers.
func NewCheckoutHandlers(svc CheckoutUseCase) *CheckoutHandlers {
	return &CheckoutHandlers{svc: svc}
}

// RegisterCheckoutRoutes wires routes onto r. The caller wraps r with sessionauth + csrf middlewares.
func (h *CheckoutHandlers) RegisterCheckoutRoutes(r chi.Router) {
	r.Post("/checkout/quote", h.quote)
	r.Post("/checkout/confirm", h.confirm)
}

// quoteBody is the decoded JSON body for POST /checkout/quote.
type quoteBody struct {
	ShippingAddressID string `json:"shipping_address_id"`
	ShippingServiceID string `json:"shipping_service_id"`
	CouponCode        string `json:"coupon_code"`
}

func (h *CheckoutHandlers) quote(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}

	var b quoteBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}

	addressID, err := uuid.Parse(b.ShippingAddressID)
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid shipping_address_id")
		return
	}

	result, err := h.svc.Quote(r.Context(), application.QuoteInput{
		UserID:     sess.UserID,
		AddressID:  addressID,
		ServiceID:  b.ShippingServiceID,
		CouponCode: b.CouponCode,
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	responsex.JSON(w, http.StatusOK, toQuoteResponse(result))
}

// confirmBody is the decoded JSON body for POST /checkout/confirm.
type confirmBody struct {
	QuoteID       string `json:"quote_id"`
	PaymentMethod string `json:"payment_method"`
}

func (h *CheckoutHandlers) confirm(w http.ResponseWriter, r *http.Request) {
	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		responsex.Error(w, r, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required")
		return
	}

	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}

	var b confirmBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}

	quoteID, err := uuid.Parse(b.QuoteID)
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid quote_id")
		return
	}

	method := b.PaymentMethod
	if method == "" {
		method = "pix"
	}

	result, err := h.svc.Confirm(r.Context(), application.ConfirmInput{
		UserID:         sess.UserID,
		QuoteID:        quoteID,
		IdempotencyKey: idemKey,
		PaymentMethod:  method,
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}

	responsex.JSON(w, http.StatusCreated, toConfirmResponse(result))
}

func (h *CheckoutHandlers) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := mapErrorToHTTP(err)
	responsex.ErrorWithCause(w, r, status, code, msg, err)
}
