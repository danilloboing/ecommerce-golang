package transport

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

// AddressUseCase is the read slice of AddressService consumed by handlers.
type AddressUseCase interface {
	List(ctx context.Context, userID uuid.UUID) ([]domain.Address, error)
}

// AddressWriter adds the mutating use cases.
type AddressWriter interface {
	AddressUseCase
	Create(ctx context.Context, in application.CreateInput) (domain.Address, error)
	Update(ctx context.Context, in application.UpdateInput) (domain.Address, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	SetDefault(ctx context.Context, id, userID uuid.UUID) (domain.Address, error)
}

// AddressHandlers exposes the authenticated /me/addresses surface.
type AddressHandlers struct {
	svc AddressWriter
}

// NewAddressHandlers builds AddressHandlers.
func NewAddressHandlers(svc AddressWriter) *AddressHandlers {
	return &AddressHandlers{svc: svc}
}

// RegisterAddressRoutes mounts routes. The caller wraps the router with sessionauth + csrf middlewares.
func (h *AddressHandlers) RegisterAddressRoutes(r chi.Router) {
	r.Get("/me/addresses", h.list)
	r.Post("/me/addresses", h.create)
	r.Patch("/me/addresses/{id}", h.update)
	r.Delete("/me/addresses/{id}", h.delete)
	r.Post("/me/addresses/{id}/default", h.setDefault)
}

func (h *AddressHandlers) userID(r *http.Request) (uuid.UUID, bool) {
	sess, ok := sessionauth.SessionFromContext(r.Context())
	if !ok {
		return uuid.Nil, false
	}
	return sess.UserID, true
}

func (h *AddressHandlers) list(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	items, err := h.svc.List(r.Context(), uid)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	out := make([]AddressResponse, 0, len(items))
	for _, a := range items {
		out = append(out, toAddressResponse(a))
	}
	responsex.JSON(w, http.StatusOK, out)
}

type addressBody struct {
	RecipientName *string `json:"recipient_name"`
	PostalCode    *string `json:"postal_code"`
	Street        *string `json:"street"`
	Number        *string `json:"number"`
	Complement    *string `json:"complement"`
	Neighborhood  *string `json:"neighborhood"`
	City          *string `json:"city"`
	State         *string `json:"state"`
	IsDefault     bool    `json:"is_default"`
}

func (h *AddressHandlers) create(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	var b addressBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	a, err := h.svc.Create(r.Context(), application.CreateInput{
		UserID:        uid,
		RecipientName: deref(b.RecipientName),
		PostalCode:    deref(b.PostalCode),
		Street:        deref(b.Street),
		Number:        deref(b.Number),
		Complement:    b.Complement,
		Neighborhood:  deref(b.Neighborhood),
		City:          deref(b.City),
		State:         deref(b.State),
		IsDefault:     b.IsDefault,
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusCreated, toAddressResponse(a))
}

func (h *AddressHandlers) update(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid address id")
		return
	}
	var b addressBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid request body")
		return
	}
	a, err := h.svc.Update(r.Context(), application.UpdateInput{
		UserID:        uid,
		ID:            id,
		RecipientName: b.RecipientName,
		PostalCode:    b.PostalCode,
		Street:        b.Street,
		Number:        b.Number,
		Complement:    b.Complement,
		Neighborhood:  b.Neighborhood,
		City:          b.City,
		State:         b.State,
	})
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toAddressResponse(a))
}

func (h *AddressHandlers) delete(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid address id")
		return
	}
	if err := h.svc.Delete(r.Context(), id, uid); err != nil {
		h.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AddressHandlers) setDefault(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.userID(r)
	if !ok {
		responsex.Error(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.Error(w, r, http.StatusBadRequest, "invalid_payload", "invalid address id")
		return
	}
	a, err := h.svc.SetDefault(r.Context(), id, uid)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	responsex.JSON(w, http.StatusOK, toAddressResponse(a))
}

func (h *AddressHandlers) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	status, code, msg := mapErrorToHTTP(err)
	responsex.ErrorWithCause(w, r, status, code, msg, err)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
