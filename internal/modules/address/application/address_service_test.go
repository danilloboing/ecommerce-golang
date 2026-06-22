package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

type fakeRepo struct {
	store map[uuid.UUID]domain.Address
}

func newFake() *fakeRepo { return &fakeRepo{store: map[uuid.UUID]domain.Address{}} }

func (f *fakeRepo) Create(_ context.Context, a domain.Address) (domain.Address, error) {
	f.store[a.ID] = a
	return a, nil
}
func (f *fakeRepo) GetByID(_ context.Context, id, userID uuid.UUID) (domain.Address, error) {
	a, ok := f.store[id]
	if !ok || a.UserID != userID {
		return domain.Address{}, domain.ErrAddressNotFound
	}
	return a, nil
}
func (f *fakeRepo) List(_ context.Context, userID uuid.UUID) ([]domain.Address, error) {
	var out []domain.Address
	for _, a := range f.store {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeRepo) Update(_ context.Context, a domain.Address) (domain.Address, error) {
	f.store[a.ID] = a
	return a, nil
}
func (f *fakeRepo) Delete(_ context.Context, id, userID uuid.UUID) error {
	a, ok := f.store[id]
	if !ok || a.UserID != userID {
		return domain.ErrAddressNotFound
	}
	delete(f.store, id)
	return nil
}
func (f *fakeRepo) SetDefault(_ context.Context, id, userID uuid.UUID) (domain.Address, error) {
	return f.GetByID(context.Background(), id, userID)
}

func TestAddressService_Create_Valid(t *testing.T) {
	svc := application.NewAddressService(newFake())
	user := uuid.New()
	a, err := svc.Create(context.Background(), application.CreateInput{
		UserID: user, RecipientName: "Ana", PostalCode: "01001000",
		Street: "Sé", Number: "1", Neighborhood: "Sé", City: "SP", State: "SP",
	})
	require.NoError(t, err)
	assert.Equal(t, user, a.UserID)
	assert.NotEqual(t, uuid.Nil, a.ID)
}

func TestAddressService_Create_Invalid(t *testing.T) {
	svc := application.NewAddressService(newFake())
	_, err := svc.Create(context.Background(), application.CreateInput{
		UserID: uuid.New(), RecipientName: "Ana", PostalCode: "bad",
		Street: "Sé", Number: "1", Neighborhood: "Sé", City: "SP", State: "SP",
	})
	require.ErrorIs(t, err, domain.ErrInvalidAddress)
}

func TestAddressService_Update_PartialAndCrossUser(t *testing.T) {
	repo := newFake()
	svc := application.NewAddressService(repo)
	owner := uuid.New()
	created, err := svc.Create(context.Background(), application.CreateInput{
		UserID: owner, RecipientName: "Ana", PostalCode: "01001000",
		Street: "Sé", Number: "1", Neighborhood: "Sé", City: "SP", State: "SP",
	})
	require.NoError(t, err)

	newName := "Ana Maria"
	updated, err := svc.Update(context.Background(), application.UpdateInput{
		UserID: owner, ID: created.ID, RecipientName: &newName,
	})
	require.NoError(t, err)
	assert.Equal(t, "Ana Maria", updated.RecipientName)
	assert.Equal(t, "Sé", updated.Street) // unchanged

	_, err = svc.Update(context.Background(), application.UpdateInput{
		UserID: uuid.New(), ID: created.ID, RecipientName: &newName,
	})
	require.ErrorIs(t, err, domain.ErrAddressNotFound)
}
