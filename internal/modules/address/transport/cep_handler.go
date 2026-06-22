package transport

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
)

// CEPHandler exposes the public postal-code lookup.
type CEPHandler struct {
	lookuper viacep.Lookuper
}

// NewCEPHandler builds a CEPHandler.
func NewCEPHandler(l viacep.Lookuper) *CEPHandler {
	return &CEPHandler{lookuper: l}
}

// RegisterCEPRoutes mounts the public CEP route.
func (h *CEPHandler) RegisterCEPRoutes(r chi.Router) {
	r.Get("/address/cep/{cep}", h.lookup)
}

func (h *CEPHandler) lookup(w http.ResponseWriter, r *http.Request) {
	cep := chi.URLParam(r, "cep")
	addr, err := h.lookuper.Lookup(r.Context(), cep)
	if err != nil {
		switch {
		case errors.Is(err, viacep.ErrInvalidCEP):
			responsex.Error(w, r, http.StatusBadRequest, "invalid_cep", "invalid cep")
		case errors.Is(err, viacep.ErrCEPNotFound):
			responsex.Error(w, r, http.StatusNotFound, "cep_not_found", "cep not found")
		default:
			responsex.ErrorWithCause(w, r, http.StatusBadGateway, "cep_lookup_failed", "cep lookup failed", err)
		}
		return
	}
	responsex.JSON(w, http.StatusOK, CEPResponse{
		PostalCode:   addr.PostalCode,
		Street:       addr.Street,
		Neighborhood: addr.Neighborhood,
		City:         addr.City,
		State:        addr.State,
	})
}
