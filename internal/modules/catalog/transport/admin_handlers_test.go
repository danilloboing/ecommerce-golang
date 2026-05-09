package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/transport"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAdmin struct {
	createdProduct domain.Product
	createErr      error
}

func (s *stubAdmin) CreateProduct(_ context.Context, in application.CreateProductInput) (domain.Product, error) {
	if s.createErr != nil {
		return domain.Product{}, s.createErr
	}
	slug, _ := domain.ParseSlug(in.Slug)
	price, _ := domain.NewMoney(in.BasePriceCents, in.Currency)
	p, _ := domain.NewProduct(domain.NewProductInput{
		ID: uuid.New(), Slug: slug, Name: in.Name,
		CategoryID: in.CategoryID, BasePrice: price,
		Status: domain.ProductStatus(in.Status),
	})
	s.createdProduct = p
	return p, nil
}

func (s *stubAdmin) UpdateProduct(_ context.Context, _ uuid.UUID, _ application.UpdateProductInput) (domain.Product, error) {
	return s.createdProduct, nil
}

func (s *stubAdmin) DeleteProduct(_ context.Context, _ uuid.UUID) error { return nil }

func (s *stubAdmin) CreateCategory(_ context.Context, in application.CreateCategoryInput) (domain.Category, error) {
	slug, _ := domain.ParseSlug(in.Slug)
	c, _ := domain.NewCategory(domain.NewCategoryInput{
		ID: uuid.New(), Slug: slug, Name: in.Name,
	})
	return c, nil
}

func TestAdminHandler_CreateProduct_201(t *testing.T) {
	svc := &stubAdmin{}
	h := transport.NewAdminHandler(svc)
	r := chi.NewRouter()
	h.RegisterAdminRoutes(r)

	body := map[string]any{
		"slug":           "vestido-novo",
		"name":           "Vestido Novo",
		"categoryId":     uuid.New().String(),
		"basePriceCents": 9990,
		"currency":       "BRL",
		"status":         "published",
		"variants": []map[string]any{
			{"sku": "VN-P", "size": "P", "color": "Azul"},
		},
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/products", bytes.NewReader(buf))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp transport.ProductResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "vestido-novo", resp.Slug)
}

func TestAdminHandler_CreateProduct_InvalidJSONReturns400(t *testing.T) {
	svc := &stubAdmin{}
	h := transport.NewAdminHandler(svc)
	r := chi.NewRouter()
	h.RegisterAdminRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/admin/products", bytes.NewReader([]byte("{not-json")))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminHandler_CreateCategory_201(t *testing.T) {
	svc := &stubAdmin{}
	h := transport.NewAdminHandler(svc)
	r := chi.NewRouter()
	h.RegisterAdminRoutes(r)

	body := map[string]any{"slug": "acessorios", "name": "Acessórios"}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/categories", bytes.NewReader(buf))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestAdminHandler_DeleteProduct_204(t *testing.T) {
	svc := &stubAdmin{}
	h := transport.NewAdminHandler(svc)
	r := chi.NewRouter()
	h.RegisterAdminRoutes(r)

	id := uuid.New().String()
	req := httptest.NewRequest(http.MethodDelete, "/admin/products/"+id, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}
