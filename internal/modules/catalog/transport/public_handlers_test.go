package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/transport"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubReader struct {
	listResult []domain.Product
	listErr    error
	getResult  domain.Product
	getErr     error
	searchRes  []domain.Product
	searchErr  error
	categories []domain.Category
}

func (s *stubReader) List(_ context.Context, _ domain.ListFilters) ([]domain.Product, error) {
	return s.listResult, s.listErr
}

func (s *stubReader) GetBySlug(_ context.Context, _ domain.Slug) (domain.Product, error) {
	return s.getResult, s.getErr
}

func (s *stubReader) Search(_ context.Context, _ string, _ domain.ListFilters) ([]domain.Product, error) {
	return s.searchRes, s.searchErr
}

func (s *stubReader) ListCategories(_ context.Context) ([]domain.Category, error) {
	return s.categories, nil
}

func newProduct(t *testing.T) domain.Product {
	t.Helper()
	slug, _ := domain.ParseSlug("vestido")
	price, _ := domain.NewMoney(9990, "BRL")
	now := time.Now()
	p, err := domain.NewProduct(domain.NewProductInput{
		ID:         uuid.New(),
		Slug:       slug,
		Name:       "Vestido",
		CategoryID: uuid.New(),
		BasePrice:  price,
		Status:     domain.ProductStatusPublished,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	require.NoError(t, err)
	return p
}

func TestPublicHandler_GetProduct_ReturnsJSON(t *testing.T) {
	p := newProduct(t)
	h := transport.NewPublicHandler(&stubReader{getResult: p})

	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/products/vestido", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body transport.ProductResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "vestido", body.Slug)
}

func TestPublicHandler_GetProduct_NotFoundReturns404(t *testing.T) {
	h := transport.NewPublicHandler(&stubReader{getErr: domain.ErrNotFound})
	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/products/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPublicHandler_ListProducts_ReturnsArray(t *testing.T) {
	p := newProduct(t)
	h := transport.NewPublicHandler(&stubReader{listResult: []domain.Product{p}})

	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/products?limit=5", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body []transport.ProductResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body, 1)
}

func TestPublicHandler_Search_RequiresQuery(t *testing.T) {
	h := transport.NewPublicHandler(&stubReader{searchErr: application.ErrBlankSearchQuery})
	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPublicHandler_Search_PassesQuery(t *testing.T) {
	p := newProduct(t)
	h := transport.NewPublicHandler(&stubReader{searchRes: []domain.Product{p}})

	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/search?q=vestido", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestPublicHandler_ListCategories_ReturnsArray(t *testing.T) {
	slug, _ := domain.ParseSlug("vestidos")
	c, _ := domain.NewCategory(domain.NewCategoryInput{ID: uuid.New(), Slug: slug, Name: "Vestidos"})
	h := transport.NewPublicHandler(&stubReader{categories: []domain.Category{c}})
	r := chi.NewRouter()
	h.RegisterPublicRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body []transport.CategoryResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Len(t, body, 1)
}
