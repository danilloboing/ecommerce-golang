package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
)

// PublicReader is the slice of PublicService consumed by HTTP handlers.
type PublicReader interface {
	List(ctx context.Context, filters domain.ListFilters) ([]domain.Product, error)
	GetBySlug(ctx context.Context, slug domain.Slug) (domain.Product, error)
	Search(ctx context.Context, query string, filters domain.ListFilters) ([]domain.Product, error)
	ListCategories(ctx context.Context) ([]domain.Category, error)
}

// PublicHandler exposes catalog endpoints to the public.
type PublicHandler struct {
	svc PublicReader
}

// NewPublicHandler builds the handler.
func NewPublicHandler(svc PublicReader) *PublicHandler {
	return &PublicHandler{svc: svc}
}

// RegisterPublicRoutes mounts public routes on the given router.
func (h *PublicHandler) RegisterPublicRoutes(r chi.Router) {
	r.Get("/products", h.list)
	r.Get("/products/{slug}", h.getBySlug)
	r.Get("/search", h.search)
	r.Get("/categories", h.listCategories)
}

func (h *PublicHandler) list(w http.ResponseWriter, r *http.Request) {
	filters, err := parseListFilters(r)
	if err != nil {
		responsex.WriteError(w, err)
		return
	}
	products, err := h.svc.List(r.Context(), filters)
	if err != nil {
		responsex.WriteError(w, err)
		return
	}
	out := make([]ProductResponse, 0, len(products))
	for _, p := range products {
		out = append(out, toProductResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *PublicHandler) getBySlug(w http.ResponseWriter, r *http.Request) {
	slug, err := domain.ParseSlug(chi.URLParam(r, "slug"))
	if err != nil {
		responsex.WriteError(w, err)
		return
	}
	p, err := h.svc.GetBySlug(r.Context(), slug)
	if err != nil {
		responsex.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProductResponse(p))
}

func (h *PublicHandler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	filters, err := parseListFilters(r)
	if err != nil {
		responsex.WriteError(w, err)
		return
	}
	products, err := h.svc.Search(r.Context(), q, filters)
	if err != nil {
		responsex.WriteError(w, err)
		return
	}
	out := make([]ProductResponse, 0, len(products))
	for _, p := range products {
		out = append(out, toProductResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *PublicHandler) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.svc.ListCategories(r.Context())
	if err != nil {
		responsex.WriteError(w, err)
		return
	}
	out := make([]CategoryResponse, 0, len(cats))
	for _, c := range cats {
		out = append(out, toCategoryResponse(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func parseListFilters(r *http.Request) (domain.ListFilters, error) {
	q := r.URL.Query()
	f := domain.ListFilters{Limit: 20}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 100 {
			return domain.ListFilters{}, domain.ErrInvalidProduct
		}
		f.Limit = n
	}
	if v := q.Get("categoryId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return domain.ListFilters{}, domain.ErrInvalidProduct
		}
		f.CategoryID = &id
	}
	if v := q.Get("brand"); v != "" {
		brand := v
		f.Brand = &brand
	}
	if v := q.Get("minPriceCents"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return domain.ListFilters{}, domain.ErrInvalidProduct
		}
		f.MinPriceCents = &n
	}
	if v := q.Get("maxPriceCents"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return domain.ListFilters{}, domain.ErrInvalidProduct
		}
		f.MaxPriceCents = &n
	}
	return f, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
