package domain_test

import (
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMoney_AcceptsValidValues(t *testing.T) {
	m, err := domain.NewMoney(9990, "BRL")
	require.NoError(t, err)
	assert.Equal(t, int64(9990), m.AmountCents())
	assert.Equal(t, "BRL", m.Currency())
}

func TestNewMoney_RejectsEmptyCurrency(t *testing.T) {
	_, err := domain.NewMoney(100, "")
	require.ErrorIs(t, err, domain.ErrInvalidCurrency)
}

func TestNewMoney_RejectsNonISOCurrency(t *testing.T) {
	_, err := domain.NewMoney(100, "BR")
	require.ErrorIs(t, err, domain.ErrInvalidCurrency)
}

func TestNewMoney_RejectsNegativeAmount(t *testing.T) {
	_, err := domain.NewMoney(-1, "BRL")
	require.ErrorIs(t, err, domain.ErrNegativeAmount)
}

func TestMoney_Add_SameCurrency(t *testing.T) {
	a, _ := domain.NewMoney(1000, "BRL")
	b, _ := domain.NewMoney(2500, "BRL")

	got, err := a.Add(b)
	require.NoError(t, err)
	assert.Equal(t, int64(3500), got.AmountCents())
}

func TestMoney_Add_DifferentCurrencyFails(t *testing.T) {
	a, _ := domain.NewMoney(1000, "BRL")
	b, _ := domain.NewMoney(2500, "USD")

	_, err := a.Add(b)
	require.ErrorIs(t, err, domain.ErrCurrencyMismatch)
}

func TestMoney_String_FormatsBRL(t *testing.T) {
	m, _ := domain.NewMoney(9990, "BRL")
	assert.Equal(t, "BRL 99.90", m.String())
}
