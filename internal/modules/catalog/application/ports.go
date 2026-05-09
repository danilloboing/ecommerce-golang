// Package application contains catalog use cases and ports.
package application

import (
	"context"
	"io"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	imagex "github.com/danilloboing/marketplace-golang/internal/platform/image"
)

// ProductReader exposes catalog read operations to other modules and HTTP layer.
type ProductReader interface {
	ListPublished(ctx context.Context, filters domain.ListFilters) ([]domain.Product, error)
	GetBySlug(ctx context.Context, slug domain.Slug) (domain.Product, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error)
	Search(ctx context.Context, query domain.SearchQuery) ([]domain.Product, error)
}

// CategoryReader exposes category navigation.
type CategoryReader interface {
	List(ctx context.Context) ([]domain.Category, error)
	GetCategoryBySlug(ctx context.Context, slug domain.Slug) (domain.Category, error)
}

// ProductWriter exposes catalog mutations for the admin service.
type ProductWriter interface {
	Create(ctx context.Context, p domain.Product) error
	Update(ctx context.Context, p domain.Product) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// CategoryWriter exposes category mutations for the admin service.
type CategoryWriter interface {
	CreateCategory(ctx context.Context, c domain.Category) error
	UpdateCategory(ctx context.Context, c domain.Category) error
	DeleteCategory(ctx context.Context, id uuid.UUID) error
}

// ImageStorage abstracts the storage backend for image bytes.
type ImageStorage interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	URL(key string) string
	Delete(ctx context.Context, key string) error
}

// ImageProcessor abstracts variant generation.
type ImageProcessor interface {
	Generate(src io.Reader) ([]imagex.Variant, error)
}

// ImageRepository persists image rows tied to products.
type ImageRepository interface {
	AttachImage(ctx context.Context, productID uuid.UUID, img domain.Image) error
}
