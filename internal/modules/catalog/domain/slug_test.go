package domain_test

import (
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlug_FromTitle_NormalisesAccents(t *testing.T) {
	got, err := domain.SlugFromTitle("Vestido Açaí Floral")
	require.NoError(t, err)
	assert.Equal(t, "vestido-acai-floral", got.String())
}

func TestSlug_FromTitle_TrimsSpacesAndPunctuation(t *testing.T) {
	got, err := domain.SlugFromTitle("  Conjunto Algodão!! ")
	require.NoError(t, err)
	assert.Equal(t, "conjunto-algodao", got.String())
}

func TestSlug_FromTitle_RejectsEmpty(t *testing.T) {
	_, err := domain.SlugFromTitle("    ")
	require.ErrorIs(t, err, domain.ErrInvalidSlug)
}

func TestParseSlug_AcceptsValid(t *testing.T) {
	got, err := domain.ParseSlug("blusa-cropped-azul")
	require.NoError(t, err)
	assert.Equal(t, "blusa-cropped-azul", got.String())
}

func TestParseSlug_RejectsUppercase(t *testing.T) {
	_, err := domain.ParseSlug("Blusa-Cropped")
	require.ErrorIs(t, err, domain.ErrInvalidSlug)
}

func TestParseSlug_RejectsSpaces(t *testing.T) {
	_, err := domain.ParseSlug("blusa cropped")
	require.ErrorIs(t, err, domain.ErrInvalidSlug)
}
