// Package transport adapts inventory services to HTTP.
package transport

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

// StockSetter is the slice of InventoryService used by StockHandler.
type StockSetter interface {
	SetStock(ctx context.Context, variantID uuid.UUID, available, version int) (domain.Stock, error)
}

// StockHandler exposes admin inventory routes.
type StockHandler struct {
	svc StockSetter
}

// NewStockHandler builds a StockHandler.
func NewStockHandler(svc StockSetter) *StockHandler {
	return &StockHandler{svc: svc}
}

// RegisterStockRoutes mounts admin inventory routes on the given router (already wrapped
// with the admin auth middleware by the caller).
func (h *StockHandler) RegisterStockRoutes(r chi.Router) {
	r.Put("/admin/variants/{id}/stock", h.setStock)
}

type setStockBody struct {
	Available int `json:"available"`
	Version   int `json:"version"`
}

type stockResponse struct {
	VariantID string `json:"variantId"`
	Available int    `json:"available"`
	Reserved  int    `json:"reserved"`
	Version   int    `json:"version"`
}

func (h *StockHandler) setStock(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_id", "invalid variant id")
		return
	}

	var body setStockBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	stock, err := h.svc.SetStock(r.Context(), id, body.Available, body.Version)
	if err != nil {
		status, code, message := mapErrorToHTTP(err)
		responsex.ErrorWithCause(w, r, status, code, message, err)
		return
	}

	responsex.JSON(w, http.StatusOK, toStockResponse(stock))
}

func toStockResponse(s domain.Stock) stockResponse {
	return stockResponse{
		VariantID: s.VariantID.String(),
		Available: s.Available,
		Reserved:  s.Reserved,
		Version:   s.Version,
	}
}
