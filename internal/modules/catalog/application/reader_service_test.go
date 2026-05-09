package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRepo struct {
	listResult []domain.Product
	listErr    error
	getResult  domain.Product
	getErr     error
	searchRes  []domain.Product
	searchErr  error
	categories []domain.Category
	categoryBy domain.Category
}

func (s *stubRepo) ListPublished(_ context.Context, _ domain.ListFilters) ([]domain.Product, error) {
	return s.listResult, s.listErr
}

func (s *stubRepo) GetBySlug(_ context.Context, _ domain.Slug) (domain.Product, error) {
	return s.getResult, s.getErr
}

func (s *stubRepo) GetByID(_ context.Context, _ uuid.UUID) (domain.Product, error) {
	return s.getResult, s.getErr
}

func (s *stubRepo) Search(_ context.Context, _ domain.SearchQuery) ([]domain.Product, error) {
	return s.searchRes, s.searchErr
}

func (s *stubRepo) List(_ context.Context) ([]domain.Category, error) {
	return s.categories, nil
}

func (s *stubRepo) GetCategoryBySlug(_ context.Context, _ domain.Slug) (domain.Category, error) {
	return s.categoryBy, nil
}

func TestPublicService_List_PassesThroughResult(t *testing.T) {
	cat := uuid.New()
	price, _ := domain.NewMoney(1000, "BRL")
	slug, _ := domain.ParseSlug("p")
	p, _ := domain.NewProduct(domain.NewProductInput{
		ID: uuid.New(), Slug: slug, Name: "P", CategoryID: cat,
		BasePrice: price, Status: domain.ProductStatusPublished,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	repo := &stubRepo{listResult: []domain.Product{p}}
	svc := application.NewPublicService(repo, repo)

	got, err := svc.List(context.Background(), domain.ListFilters{Limit: 10})

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "p", got[0].Slug().String())
}

func TestPublicService_GetBySlug_PropagatesNotFound(t *testing.T) {
	repo := &stubRepo{getErr: domain.ErrNotFound}
	svc := application.NewPublicService(repo, repo)

	slug, _ := domain.ParseSlug("missing")
	_, err := svc.GetBySlug(context.Background(), slug)

	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestPublicService_Search_RejectsBlankQuery(t *testing.T) {
	repo := &stubRepo{}
	svc := application.NewPublicService(repo, repo)

	_, err := svc.Search(context.Background(), "   ", domain.ListFilters{})

	require.ErrorIs(t, err, application.ErrBlankSearchQuery)
}
