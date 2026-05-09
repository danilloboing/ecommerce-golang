package domain_test

import (
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newValidProductInput(t *testing.T) domain.NewProductInput {
	t.Helper()
	categoryID := uuid.New()
	price, err := domain.NewMoney(9990, "BRL")
	require.NoError(t, err)

	slug, err := domain.ParseSlug("vestido-floral-azul")
	require.NoError(t, err)

	return domain.NewProductInput{
		ID:          uuid.New(),
		Slug:        slug,
		Name:        "Vestido Floral Azul",
		Description: "Vestido midi com estampa floral.",
		Brand:       "AcmeFashion",
		CategoryID:  categoryID,
		BasePrice:   price,
		Status:      domain.ProductStatusPublished,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func TestNewProduct_AcceptsValidInput(t *testing.T) {
	in := newValidProductInput(t)

	p, err := domain.NewProduct(in)

	require.NoError(t, err)
	assert.Equal(t, in.ID, p.ID())
	assert.Equal(t, "vestido-floral-azul", p.Slug().String())
	assert.Equal(t, domain.ProductStatusPublished, p.Status())
}

func TestNewProduct_RejectsEmptyName(t *testing.T) {
	in := newValidProductInput(t)
	in.Name = "   "

	_, err := domain.NewProduct(in)

	require.ErrorIs(t, err, domain.ErrInvalidProduct)
}

func TestNewProduct_RejectsZeroCategoryID(t *testing.T) {
	in := newValidProductInput(t)
	in.CategoryID = uuid.Nil

	_, err := domain.NewProduct(in)

	require.ErrorIs(t, err, domain.ErrInvalidProduct)
}

func TestNewProduct_RejectsUnknownStatus(t *testing.T) {
	in := newValidProductInput(t)
	in.Status = "garbage"

	_, err := domain.NewProduct(in)

	require.ErrorIs(t, err, domain.ErrInvalidProduct)
}

func TestProduct_AddVariant_AppendsAndKeepsOrder(t *testing.T) {
	in := newValidProductInput(t)
	p, err := domain.NewProduct(in)
	require.NoError(t, err)

	v1 := domain.Variant{ID: uuid.New(), SKU: "BL-A-P", Size: "P", Color: "Azul"}
	v2 := domain.Variant{ID: uuid.New(), SKU: "BL-A-M", Size: "M", Color: "Azul"}

	require.NoError(t, p.AddVariant(v1))
	require.NoError(t, p.AddVariant(v2))

	variants := p.Variants()
	require.Len(t, variants, 2)
	assert.Equal(t, "BL-A-P", variants[0].SKU)
	assert.Equal(t, "BL-A-M", variants[1].SKU)
}

func TestProduct_AddVariant_RejectsDuplicateSKU(t *testing.T) {
	in := newValidProductInput(t)
	p, _ := domain.NewProduct(in)
	v := domain.Variant{ID: uuid.New(), SKU: "DUPE", Size: "P", Color: "Azul"}

	require.NoError(t, p.AddVariant(v))
	err := p.AddVariant(v)

	require.ErrorIs(t, err, domain.ErrDuplicateSKU)
}

func TestNewCategory_AcceptsValidInput(t *testing.T) {
	id := uuid.New()
	slug, _ := domain.ParseSlug("vestidos")

	c, err := domain.NewCategory(domain.NewCategoryInput{
		ID:   id,
		Slug: slug,
		Name: "Vestidos",
	})

	require.NoError(t, err)
	assert.Equal(t, id, c.ID())
	assert.Equal(t, "vestidos", c.Slug().String())
	assert.Equal(t, "Vestidos", c.Name())
}

func TestNewCategory_RejectsBlankName(t *testing.T) {
	slug, _ := domain.ParseSlug("vestidos")
	_, err := domain.NewCategory(domain.NewCategoryInput{
		ID:   uuid.New(),
		Slug: slug,
		Name: "",
	})
	require.ErrorIs(t, err, domain.ErrInvalidCategory)
}
