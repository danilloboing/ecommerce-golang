package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	checkoutdomain "github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
)

// CouponUseCase is the slice of CouponService consumed by the coupon handler.
// Satisfied by *application.CouponService.
type CouponUseCase interface {
	Create(ctx context.Context, in application.NewCoupon) (*checkoutdomain.Coupon, error)
}

// CouponHandler exposes admin-only coupon management endpoints.
type CouponHandler struct {
	svc CouponUseCase
}

// NewCouponHandler builds a CouponHandler.
func NewCouponHandler(svc CouponUseCase) *CouponHandler {
	return &CouponHandler{svc: svc}
}

// RegisterCouponRoutes wires routes onto r.
// The caller wraps r with adminauth.RequireToken middleware.
func (h *CouponHandler) RegisterCouponRoutes(r chi.Router) {
	r.Post("/admin/coupons", h.create)
}

// createCouponBody is the decoded JSON body for POST /admin/coupons.
type createCouponBody struct {
	Code          string     `json:"code"`
	Type          string     `json:"type"`
	Value         int64      `json:"value"`
	ExpiresAt     *time.Time `json:"expires_at"`
	UsageLimit    *int       `json:"usage_limit"`
	MinOrderCents *int64     `json:"min_order_cents"`
}

func (h *CouponHandler) create(w http.ResponseWriter, r *http.Request) {
	var b createCouponBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	if b.Code == "" {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "code is required")
		return
	}

	coupon, err := h.svc.Create(r.Context(), application.NewCoupon{
		Code:          b.Code,
		Type:          checkoutdomain.CouponType(b.Type),
		Value:         b.Value,
		ExpiresAt:     b.ExpiresAt,
		UsageLimit:    b.UsageLimit,
		MinOrderCents: b.MinOrderCents,
	})
	if err != nil {
		responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "internal error", err)
		return
	}

	responsex.JSON(w, http.StatusCreated, toCouponResponse(coupon))
}
