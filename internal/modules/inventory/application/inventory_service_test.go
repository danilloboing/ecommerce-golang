package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

type fakeStock struct {
	reserveErr error
	committed  uuid.UUID
	released   uuid.UUID
}

func (f *fakeStock) Reserve(_ context.Context, _ []application.ReserveItem, _ uuid.UUID, _ time.Time) error {
	return f.reserveErr
}
func (f *fakeStock) CommitForOrder(_ context.Context, o uuid.UUID) error  { f.committed = o; return nil }
func (f *fakeStock) ReleaseForOrder(_ context.Context, o uuid.UUID) error { f.released = o; return nil }
func (f *fakeStock) SetStock(_ context.Context, v uuid.UUID, a, _ int) (domain.Stock, error) {
	return domain.Stock{VariantID: v, Available: a}, nil
}
func (f *fakeStock) Get(_ context.Context, v uuid.UUID) (domain.Stock, error) {
	return domain.Stock{VariantID: v}, nil
}

func TestInventoryService_Reserve_Insufficient(t *testing.T) {
	svc := application.NewInventoryService(&fakeStock{reserveErr: domain.ErrInsufficientStock})
	err := svc.Reserve(context.Background(), []application.ReserveItem{{VariantID: uuid.New(), Quantity: 2}}, uuid.New(), time.Now())
	require.ErrorIs(t, err, domain.ErrInsufficientStock)
}

func TestInventoryService_CommitRelease(t *testing.T) {
	f := &fakeStock{}
	svc := application.NewInventoryService(f)
	order := uuid.New()
	require.NoError(t, svc.Commit(context.Background(), order))
	assert.Equal(t, order, f.committed)
	require.NoError(t, svc.Release(context.Background(), order))
	assert.Equal(t, order, f.released)
}

func TestInventoryService_Reserve_Delegates(t *testing.T) {
	f := &fakeStock{}
	svc := application.NewInventoryService(f)
	items := []application.ReserveItem{{VariantID: uuid.New(), Quantity: 3}}
	orderID := uuid.New()
	expiresAt := time.Now().Add(15 * time.Minute)
	err := svc.Reserve(context.Background(), items, orderID, expiresAt)
	require.NoError(t, err)
}

func TestInventoryService_SetStock_Delegates(t *testing.T) {
	variantID := uuid.New()
	f := &fakeStock{}
	svc := application.NewInventoryService(f)
	stock, err := svc.SetStock(context.Background(), variantID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, variantID, stock.VariantID)
	assert.Equal(t, 10, stock.Available)
}

func TestInventoryService_SetStock_PropagatesConflict(t *testing.T) {
	svc := application.NewInventoryService(&fakeStockConflict{})
	_, err := svc.SetStock(context.Background(), uuid.New(), 5, 1)
	require.ErrorIs(t, err, domain.ErrStockConflict)
}

type fakeStockConflict struct{ fakeStock }

func (f *fakeStockConflict) SetStock(_ context.Context, _ uuid.UUID, _, _ int) (domain.Stock, error) {
	return domain.Stock{}, domain.ErrStockConflict
}
