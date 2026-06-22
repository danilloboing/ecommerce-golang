package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

func TestCart_SubtotalCents(t *testing.T) {
	c := domain.Cart{Items: []domain.CartItem{
		{ID: uuid.New(), VariantID: uuid.New(), Quantity: 2, UnitPriceCents: 1500},
		{ID: uuid.New(), VariantID: uuid.New(), Quantity: 3, UnitPriceCents: 1000},
	}}
	assert.Equal(t, int64(6000), c.SubtotalCents())
}

func TestCart_SubtotalCents_Empty(t *testing.T) {
	assert.Equal(t, int64(0), domain.Cart{}.SubtotalCents())
}

func TestValidateQuantity(t *testing.T) {
	require.NoError(t, domain.ValidateQuantity(1))
	require.NoError(t, domain.ValidateQuantity(99))
	require.ErrorIs(t, domain.ValidateQuantity(0), domain.ErrInvalidQuantity)
	require.ErrorIs(t, domain.ValidateQuantity(-1), domain.ErrInvalidQuantity)
	require.ErrorIs(t, domain.ValidateQuantity(100), domain.ErrInvalidQuantity)
}
