package transport

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
)

const defaultListLimit = 50

// OrderReader is the slice of OrderService consumed by handlers.
// Satisfied by *application.OrderService.
type OrderReader interface {
	GetForUser(ctx context.Context, id, userID uuid.UUID) (domain.Order, []domain.OrderItem, error)
	ListForUser(ctx context.Context, userID uuid.UUID, limit int) ([]domain.Order, error)
}

// OrderHandlers handles authenticated order read endpoints.
type OrderHandlers struct {
	svc OrderReader
}

// NewOrderHandlers builds OrderHandlers.
func NewOrderHandlers(svc OrderReader) *OrderHandlers {
	return &OrderHandlers{svc: svc}
}

// RegisterOrderRoutes wires routes onto r. The caller wraps r with sessionauth middleware.
func (h *OrderHandlers) RegisterOrderRoutes(r chi.Router) {
	r.Get("/me/orders", h.listOrders)
	r.Get("/me/orders/{id}", h.getOrder)
}

// getOrder returns a single order with items for the authenticated user.
func (h *OrderHandlers) getOrder(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	orderID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid order id")
		return
	}
	order, items, err := h.svc.GetForUser(r.Context(), orderID, sess.UserID)
	if err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toOrderResponse(order, items))
}

// listOrders returns the authenticated user's orders (up to defaultListLimit).
func (h *OrderHandlers) listOrders(w http.ResponseWriter, r *http.Request) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	orders, err := h.svc.ListForUser(r.Context(), sess.UserID, defaultListLimit)
	if err != nil {
		status, code, msg := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, msg, err)
		return
	}
	resp := make([]OrderListItem, 0, len(orders))
	for _, o := range orders {
		resp = append(resp, toOrderListItem(o))
	}
	responsex.JSON(w, http.StatusOK, resp)
}
