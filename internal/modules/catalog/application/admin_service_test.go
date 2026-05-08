package application_test

import (
	"context"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type adminRepo struct {
	stubRepo
	created    []domain.Product
	updated    []domain.Product
	deleted    []uuid.UUID
	catCreated []domain.Category
}

func (a *adminRepo) Create(_ context.Context, p domain.Product) error {
	a.created = append(a.created, p)
	return nil
}

func (a *adminRepo) Update(_ context.Context, p domain.Product) error {
	a.updated = append(a.updated, p)
	return nil
}

func (a *adminRepo) Delete(_ context.Context, id uuid.UUID) error {
	a.deleted = append(a.deleted, id)
	return nil
}

func (a *adminRepo) CreateCategory(_ context.Context, c domain.Category) error {
	a.catCreated = append(a.catCreated, c)
	return nil
}

func (a *adminRepo) UpdateCategory(_ context.Context, _ domain.Category) error { return nil }
func (a *adminRepo) DeleteCategory(_ context.Context, _ uuid.UUID) error       { return nil }

func TestAdminService_CreateProduct_PersistsValidProduct(t *testing.T) {
	repo := &adminRepo{}
	svc := application.NewAdminService(repo)

	in := application.CreateProductInput{
		Slug:           "vestido-novo",
		Name:           "Vestido Novo",
		Description:    "x",
		Brand:          "AcmeFashion",
		CategoryID:     uuid.New(),
		BasePriceCents: 9990,
		Currency:       "BRL",
		Status:         "published",
		Variants: []application.VariantInput{
			{SKU: "VN-P", Size: "P", Color: "Azul"},
		},
	}

	got, err := svc.CreateProduct(context.Background(), in)

	require.NoError(t, err)
	assert.Equal(t, "vestido-novo", got.Slug().String())
	assert.Len(t, repo.created, 1)
}

func TestAdminService_CreateProduct_RejectsInvalidStatus(t *testing.T) {
	repo := &adminRepo{}
	svc := application.NewAdminService(repo)

	_, err := svc.CreateProduct(context.Background(), application.CreateProductInput{
		Slug:           "x",
		Name:           "x",
		CategoryID:     uuid.New(),
		BasePriceCents: 1,
		Currency:       "BRL",
		Status:         "garbage",
	})

	require.ErrorIs(t, err, domain.ErrInvalidProduct)
}

func TestAdminService_CreateCategory_PersistsCategory(t *testing.T) {
	repo := &adminRepo{}
	svc := application.NewAdminService(repo)

	got, err := svc.CreateCategory(context.Background(), application.CreateCategoryInput{
		Slug: "acessorios",
		Name: "Acessórios",
	})

	require.NoError(t, err)
	assert.Equal(t, "acessorios", got.Slug().String())
	assert.Len(t, repo.catCreated, 1)
}
