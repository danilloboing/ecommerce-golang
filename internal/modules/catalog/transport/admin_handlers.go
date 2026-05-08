package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
)

// AdminWriter is the slice of AdminService used by the handler.
type AdminWriter interface {
	CreateProduct(ctx context.Context, in application.CreateProductInput) (domain.Product, error)
	UpdateProduct(ctx context.Context, id uuid.UUID, in application.UpdateProductInput) (domain.Product, error)
	DeleteProduct(ctx context.Context, id uuid.UUID) error
	CreateCategory(ctx context.Context, in application.CreateCategoryInput) (domain.Category, error)
}

// AdminHandler exposes admin catalog routes.
type AdminHandler struct {
	svc AdminWriter
}

// NewAdminHandler builds an AdminHandler.
func NewAdminHandler(svc AdminWriter) *AdminHandler {
	return &AdminHandler{svc: svc}
}

// RegisterAdminRoutes mounts admin routes on the given router (already wrapped
// with the admin auth middleware by the caller).
func (h *AdminHandler) RegisterAdminRoutes(r chi.Router) {
	r.Post("/admin/products", h.createProduct)
	r.Put("/admin/products/{id}", h.updateProduct)
	r.Delete("/admin/products/{id}", h.deleteProduct)
	r.Post("/admin/categories", h.createCategory)
}

type productCreateBody struct {
	Slug           string        `json:"slug"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Brand          string        `json:"brand"`
	CategoryID     uuid.UUID     `json:"categoryId"`
	BasePriceCents int64         `json:"basePriceCents"`
	Currency       string        `json:"currency"`
	Status         string        `json:"status"`
	Variants       []variantBody `json:"variants"`
	Images         []imageBody   `json:"images"`
}

type variantBody struct {
	SKU        string `json:"sku"`
	Size       string `json:"size"`
	Color      string `json:"color"`
	PriceCents *int64 `json:"priceCents,omitempty"`
}

type imageBody struct {
	URL      string `json:"url"`
	AltText  string `json:"altText"`
	Position int    `json:"position"`
}

type categoryCreateBody struct {
	Slug     string     `json:"slug"`
	Name     string     `json:"name"`
	ParentID *uuid.UUID `json:"parentId,omitempty"`
}

func (h *AdminHandler) createProduct(w http.ResponseWriter, r *http.Request) {
	var body productCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		responsex.WriteError(w, errors.Join(domain.ErrInvalidProduct, err))
		return
	}
	in := toCreateInput(body)
	p, err := h.svc.CreateProduct(r.Context(), in)
	if err != nil {
		responsex.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toProductResponse(p))
}

func (h *AdminHandler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.WriteError(w, domain.ErrInvalidProduct)
		return
	}
	var body productCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		responsex.WriteError(w, errors.Join(domain.ErrInvalidProduct, err))
		return
	}
	in := toCreateInput(body)
	p, err := h.svc.UpdateProduct(r.Context(), id, in)
	if err != nil {
		responsex.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProductResponse(p))
}

func (h *AdminHandler) deleteProduct(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.WriteError(w, domain.ErrInvalidProduct)
		return
	}
	if err := h.svc.DeleteProduct(r.Context(), id); err != nil {
		responsex.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) createCategory(w http.ResponseWriter, r *http.Request) {
	var body categoryCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		responsex.WriteError(w, errors.Join(domain.ErrInvalidCategory, err))
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), application.CreateCategoryInput{
		Slug:     body.Slug,
		Name:     body.Name,
		ParentID: body.ParentID,
	})
	if err != nil {
		responsex.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCategoryResponse(c))
}

func toCreateInput(body productCreateBody) application.CreateProductInput {
	variants := make([]application.VariantInput, 0, len(body.Variants))
	for _, v := range body.Variants {
		variants = append(variants, application.VariantInput{
			SKU:        v.SKU,
			Size:       v.Size,
			Color:      v.Color,
			PriceCents: v.PriceCents,
		})
	}
	images := make([]application.ImageInput, 0, len(body.Images))
	for _, img := range body.Images {
		images = append(images, application.ImageInput{
			URL:      img.URL,
			AltText:  img.AltText,
			Position: img.Position,
		})
	}
	return application.CreateProductInput{
		Slug:           body.Slug,
		Name:           body.Name,
		Description:    body.Description,
		Brand:          body.Brand,
		CategoryID:     body.CategoryID,
		BasePriceCents: body.BasePriceCents,
		Currency:       body.Currency,
		Status:         body.Status,
		Variants:       variants,
		Images:         images,
	}
}
