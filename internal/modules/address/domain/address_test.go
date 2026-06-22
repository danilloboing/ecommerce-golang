package domain_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

func valid() domain.Address {
	return domain.Address{
		ID: uuid.New(), UserID: uuid.New(),
		RecipientName: "Ana", PostalCode: "01001000", Street: "Praça da Sé",
		Number: "100", Neighborhood: "Sé", City: "São Paulo", State: "SP",
	}
}

func TestValidate_OK(t *testing.T) {
	require.NoError(t, domain.Validate(valid()))
}

func TestValidate_BadPostalCode(t *testing.T) {
	a := valid()
	a.PostalCode = "1234"
	require.ErrorIs(t, domain.Validate(a), domain.ErrInvalidAddress)
}

func TestValidate_BadState(t *testing.T) {
	a := valid()
	a.State = "SAO"
	require.ErrorIs(t, domain.Validate(a), domain.ErrInvalidAddress)
}

func TestValidate_MissingRequired(t *testing.T) {
	a := valid()
	a.City = "  "
	require.ErrorIs(t, domain.Validate(a), domain.ErrInvalidAddress)
}
