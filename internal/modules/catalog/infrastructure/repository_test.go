//go:build integration

package infrastructure_test

import (
	"context"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/infrastructure"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRepo(t *testing.T) *infrastructure.Repository {
	t.Helper()
	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)

	cfg := config.Database{URL: dsn, MaxOpenConns: 5, MaxIdleConns: 1, ConnMaxLifetime: time.Minute}
	pool, err := internalpostgres.NewPool(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return infrastructure.New(pool)
}

func TestRepository_CreateAndGetProduct(t *testing.T) {
	repo := newRepo(t)

	cat := makeCategory(t)
	require.NoError(t, repo.CreateCategory(context.Background(), cat))

	p := makeProduct(t, cat.ID())
	require.NoError(t, repo.Create(context.Background(), p))

	got, err := repo.GetBySlug(context.Background(), p.Slug())
	require.NoError(t, err)
	assert.Equal(t, p.Name(), got.Name())
	assert.Len(t, got.Variants(), 2)
}

func TestRepository_ListPublished_FiltersByCategory(t *testing.T) {
	repo := newRepo(t)

	cat1 := makeCategoryWith(t, "vestidos", "Vestidos")
	cat2 := makeCategoryWith(t, "blusas", "Blusas")
	require.NoError(t, repo.CreateCategory(context.Background(), cat1))
	require.NoError(t, repo.CreateCategory(context.Background(), cat2))

	require.NoError(t, repo.Create(context.Background(), makeProductIn(t, cat1.ID(), "vestido-azul")))
	require.NoError(t, repo.Create(context.Background(), makeProductIn(t, cat2.ID(), "blusa-rosa")))

	id := cat1.ID()
	got, err := repo.ListPublished(context.Background(), domain.ListFilters{CategoryID: &id, Limit: 10})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "vestido-azul", got[0].Slug().String())
}

func TestRepository_Search_FindsByName(t *testing.T) {
	repo := newRepo(t)

	cat := makeCategory(t)
	require.NoError(t, repo.CreateCategory(context.Background(), cat))
	require.NoError(t, repo.Create(context.Background(), makeProductIn(t, cat.ID(), "vestido-floral-azul")))

	got, err := repo.Search(context.Background(), domain.SearchQuery{Query: "vestido floral"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "vestido-floral-azul", got[0].Slug().String())
}

func TestRepository_GetBySlug_NotFound(t *testing.T) {
	repo := newRepo(t)
	slug, _ := domain.ParseSlug("missing")
	_, err := repo.GetBySlug(context.Background(), slug)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func makeCategory(t *testing.T) domain.Category {
	t.Helper()
	return makeCategoryWith(t, "vestidos", "Vestidos")
}

func makeCategoryWith(t *testing.T, slugStr, name string) domain.Category {
	t.Helper()
	slug, err := domain.ParseSlug(slugStr)
	require.NoError(t, err)
	c, err := domain.NewCategory(domain.NewCategoryInput{
		ID:   uuid.New(),
		Slug: slug,
		Name: name,
	})
	require.NoError(t, err)
	return c
}

func makeProduct(t *testing.T, categoryID uuid.UUID) domain.Product {
	return makeProductIn(t, categoryID, "vestido-floral-azul")
}

func makeProductIn(t *testing.T, categoryID uuid.UUID, slugStr string) domain.Product {
	t.Helper()
	slug, err := domain.ParseSlug(slugStr)
	require.NoError(t, err)
	price, err := domain.NewMoney(9990, "BRL")
	require.NoError(t, err)
	now := time.Now()

	p, err := domain.NewProduct(domain.NewProductInput{
		ID:          uuid.New(),
		Slug:        slug,
		Name:        "Vestido Floral Azul",
		Description: "Vestido midi floral.",
		Brand:       "AcmeFashion",
		CategoryID:  categoryID,
		BasePrice:   price,
		Status:      domain.ProductStatusPublished,
		Variants: []domain.Variant{
			{ID: uuid.New(), SKU: "VFA-P", Size: "P", Color: "Azul"},
			{ID: uuid.New(), SKU: "VFA-M", Size: "M", Color: "Azul"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
	return p
}
