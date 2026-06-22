package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

// CartUseCase is the slice of CartService consumed by handlers.
type CartUseCase interface {
	Get(ctx context.Context, owner domain.Owner) (domain.Cart, error)
	AddItem(ctx context.Context, owner domain.Owner, variantID uuid.UUID, qty int) (domain.Cart, error)
	UpdateItem(ctx context.Context, owner domain.Owner, itemID uuid.UUID, qty int) (domain.Cart, error)
	RemoveItem(ctx context.Context, owner domain.Owner, itemID uuid.UUID) (domain.Cart, error)
	Clear(ctx context.Context, owner domain.Owner) error
}

// CartHandlers exposes cart endpoints to anon + user visitors.
type CartHandlers struct {
	svc            CartUseCase
	anonCookieName string
}

// NewCartHandlers builds CartHandlers.
func NewCartHandlers(svc CartUseCase, anonCookieName string) *CartHandlers {
	return &CartHandlers{svc: svc, anonCookieName: anonCookieName}
}

// RegisterCartRoutes mounts cart routes (caller wraps with ResolveCartIdentity).
func (h *CartHandlers) RegisterCartRoutes(r chi.Router) {
	r.Get("/cart", h.get)
	r.Post("/cart/items", h.addItem)
	r.Patch("/cart/items/{id}", h.updateItem)
	r.Delete("/cart/items/{id}", h.removeItem)
	r.Delete("/cart", h.clear)
}

func (h *CartHandlers) get(w http.ResponseWriter, r *http.Request) {
	owner, ok := OwnerFromContext(r.Context())
	if !ok {
		responsex.JSON(w, http.StatusOK, CartResponse{Items: []CartItemResponse{}})
		return
	}
	cart, err := h.svc.Get(r.Context(), owner)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toCartResponse(cart))
}

type addItemInput struct {
	VariantID string `json:"variant_id"`
	Quantity  int    `json:"quantity"`
}

func (h *CartHandlers) addItem(w http.ResponseWriter, r *http.Request) {
	var in addItemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	variantID, err := uuid.Parse(in.VariantID)
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid variant id")
		return
	}
	owner, ok := OwnerFromContext(r.Context())
	if !ok {
		anon, err := newAnonID()
		if err != nil {
			responsex.ErrorWithCause(w, r, http.StatusInternalServerError, "internal_error", "cart id gen failed", err)
			return
		}
		h.setAnonCookie(w, anon)
		owner = domain.Owner{AnonID: &anon}
	}
	cart, err := h.svc.AddItem(r.Context(), owner, variantID, in.Quantity)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toCartResponse(cart))
}

type updateItemInput struct {
	Quantity int `json:"quantity"`
}

func (h *CartHandlers) updateItem(w http.ResponseWriter, r *http.Request) {
	owner, ok := OwnerFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusNotFound, "cart_not_found", "cart not found")
		return
	}
	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid item id")
		return
	}
	var in updateItemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	cart, err := h.svc.UpdateItem(r.Context(), owner, itemID, in.Quantity)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toCartResponse(cart))
}

func (h *CartHandlers) removeItem(w http.ResponseWriter, r *http.Request) {
	owner, ok := OwnerFromContext(r.Context())
	if !ok {
		responsex.Error(w, r, http.StatusNotFound, "cart_not_found", "cart not found")
		return
	}
	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid item id")
		return
	}
	cart, err := h.svc.RemoveItem(r.Context(), owner, itemID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toCartResponse(cart))
}

func (h *CartHandlers) clear(w http.ResponseWriter, r *http.Request) {
	owner, ok := OwnerFromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.svc.Clear(r.Context(), owner); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CartHandlers) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := mapErrorToHTTP(err)
	responsex.ErrorWithCause(w, r, status, code, msg, err)
}

func (h *CartHandlers) setAnonCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.anonCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	})
}

func newAnonID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
