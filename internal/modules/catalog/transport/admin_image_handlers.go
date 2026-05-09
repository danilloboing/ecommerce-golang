package transport

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
)

const maxUploadBytes = 10 << 20 // 10 MiB

// ImageUploader is the slice of ImageService consumed by the handler.
type ImageUploader interface {
	Upload(ctx context.Context, in application.UploadImageInput) (domain.Image, error)
}

// AdminImageHandler exposes the image upload endpoint.
type AdminImageHandler struct {
	svc ImageUploader
}

// NewAdminImageHandler builds the handler.
func NewAdminImageHandler(svc ImageUploader) *AdminImageHandler {
	return &AdminImageHandler{svc: svc}
}

// RegisterAdminImageRoutes mounts admin image routes.
func (h *AdminImageHandler) RegisterAdminImageRoutes(r chi.Router) {
	r.Post("/admin/products/{id}/images", h.upload)
}

func (h *AdminImageHandler) upload(w http.ResponseWriter, r *http.Request) {
	productID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		responsex.WriteError(w, domain.ErrInvalidProduct)
		return
	}

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		responsex.WriteError(w, errors.Join(domain.ErrInvalidProduct, err))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		responsex.WriteError(w, errors.Join(domain.ErrInvalidProduct, err))
		return
	}
	defer file.Close()

	if header.Size > maxUploadBytes {
		responsex.WriteError(w, domain.ErrInvalidProduct)
		return
	}

	position, _ := strconv.Atoi(r.FormValue("position"))

	img, err := h.svc.Upload(r.Context(), application.UploadImageInput{
		ProductID:   productID,
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Body:        file,
		Size:        header.Size,
		AltText:     r.FormValue("altText"),
		Position:    position,
	})
	if err != nil {
		responsex.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toImageResponse(img))
}
