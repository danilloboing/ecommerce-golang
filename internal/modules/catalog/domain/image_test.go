package domain_test

import (
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageVariants_AllVariantsRequireURL(t *testing.T) {
	_, err := domain.NewImageVariants(domain.ImageVariantURLs{
		Original: "",
		Thumb:    "https://cdn.example/t.jpg",
		Medium:   "https://cdn.example/m.jpg",
		Large:    "https://cdn.example/l.jpg",
	})
	require.ErrorIs(t, err, domain.ErrInvalidImageVariants)
}

func TestImageVariants_HappyPath(t *testing.T) {
	v, err := domain.NewImageVariants(domain.ImageVariantURLs{
		Original: "https://cdn.example/o.jpg",
		Thumb:    "https://cdn.example/t.jpg",
		Medium:   "https://cdn.example/m.jpg",
		Large:    "https://cdn.example/l.jpg",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/t.jpg", v.URLs().Thumb)
}

func TestImageVariants_AttachToProduct(t *testing.T) {
	in := newValidProductInput(t)
	p, err := domain.NewProduct(in)
	require.NoError(t, err)

	urls := domain.ImageVariantURLs{
		Original: "https://cdn.example/o.jpg",
		Thumb:    "https://cdn.example/t.jpg",
		Medium:   "https://cdn.example/m.jpg",
		Large:    "https://cdn.example/l.jpg",
	}
	variants, err := domain.NewImageVariants(urls)
	require.NoError(t, err)

	img := domain.Image{
		ID:         uuid.New(),
		URL:        urls.Original,
		Variants:   &variants,
		Position:   0,
		AltText:    "vestido azul",
		StorageKey: "products/x/y",
	}
	require.NoError(t, p.AddImage(img))

	got := p.Images()
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Variants)
	assert.Equal(t, urls.Thumb, got[0].Variants.URLs().Thumb)
}
