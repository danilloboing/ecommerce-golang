package application

import (
	"context"
	"errors"
	"strings"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
)

// ErrBlankSearchQuery is returned when search receives no terms.
var ErrBlankSearchQuery = errors.New("catalog: blank search query")

// PublicService implements catalog read flows for the public API.
type PublicService struct {
	products   ProductReader
	categories CategoryReader
}

// NewPublicService builds a PublicService.
func NewPublicService(products ProductReader, categories CategoryReader) *PublicService {
	return &PublicService{products: products, categories: categories}
}

// List returns published products applying filters.
func (s *PublicService) List(ctx context.Context, filters domain.ListFilters) ([]domain.Product, error) {
	return s.products.ListPublished(ctx, filters)
}

// GetBySlug returns a product by slug.
func (s *PublicService) GetBySlug(ctx context.Context, slug domain.Slug) (domain.Product, error) {
	return s.products.GetBySlug(ctx, slug)
}

// Search performs a free-text query plus filters.
func (s *PublicService) Search(ctx context.Context, query string, filters domain.ListFilters) ([]domain.Product, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, ErrBlankSearchQuery
	}
	return s.products.Search(ctx, domain.SearchQuery{Query: q, Filters: filters})
}

// ListCategories returns the full category set.
func (s *PublicService) ListCategories(ctx context.Context) ([]domain.Category, error) {
	return s.categories.List(ctx)
}
