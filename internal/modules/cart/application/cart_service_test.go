package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

// fakeRepo is an in-memory CartRepository for service unit tests.
type fakeRepo struct {
	cart        domain.Cart
	hasCart     bool
	prices      map[uuid.UUID]int64
	priceErr    error
	mergeCalled bool
}

func (f *fakeRepo) FindActive(_ context.Context, _ domain.Owner) (domain.Cart, error) {
	if !f.hasCart {
		return domain.Cart{}, domain.ErrCartNotFound
	}
	return f.cart, nil
}

func (f *fakeRepo) EnsureActive(_ context.Context, owner domain.Owner) (domain.Cart, error) {
	if !f.hasCart {
		f.cart = domain.Cart{ID: uuid.New(), UserID: owner.UserID, AnonSessionID: owner.AnonID, Status: domain.StatusActive}
		f.hasCart = true
	}
	return f.cart, nil
}

func (f *fakeRepo) VariantUnitPrice(_ context.Context, id uuid.UUID) (int64, error) {
	if f.priceErr != nil {
		return 0, f.priceErr
	}
	p, ok := f.prices[id]
	if !ok {
		return 0, domain.ErrVariantNotFound
	}
	return p, nil
}

func (f *fakeRepo) UpsertItem(_ context.Context, cartID, variantID uuid.UUID, qty int, unitPrice int64) error {
	f.cart.Items = append(f.cart.Items, domain.CartItem{ID: uuid.New(), VariantID: variantID, Quantity: qty, UnitPriceCents: unitPrice})
	return nil
}

func (f *fakeRepo) UpdateItemQuantity(_ context.Context, cartID, itemID uuid.UUID, qty int) error {
	for i := range f.cart.Items {
		if f.cart.Items[i].ID == itemID {
			f.cart.Items[i].Quantity = qty
			return nil
		}
	}
	return domain.ErrItemNotFound
}

func (f *fakeRepo) DeleteItem(_ context.Context, cartID, itemID uuid.UUID) error {
	for i := range f.cart.Items {
		if f.cart.Items[i].ID == itemID {
			f.cart.Items = append(f.cart.Items[:i], f.cart.Items[i+1:]...)
			return nil
		}
	}
	return domain.ErrItemNotFound
}

func (f *fakeRepo) ClearItems(_ context.Context, _ uuid.UUID) error { f.cart.Items = nil; return nil }
func (f *fakeRepo) Merge(_ context.Context, _ string, _ uuid.UUID) error {
	f.mergeCalled = true
	return nil
}

func anonOwner() domain.Owner { id := "anon123"; return domain.Owner{AnonID: &id} }

func TestCartService_AddItem_Success(t *testing.T) {
	variant := uuid.New()
	repo := &fakeRepo{prices: map[uuid.UUID]int64{variant: 2500}}
	svc := application.NewCartService(repo)

	cart, err := svc.AddItem(context.Background(), anonOwner(), variant, 2)
	require.NoError(t, err)
	require.Len(t, cart.Items, 1)
	assert.Equal(t, int64(2500), cart.Items[0].UnitPriceCents)
	assert.Equal(t, int64(5000), cart.SubtotalCents())
}

func TestCartService_AddItem_QuantityOverCap(t *testing.T) {
	repo := &fakeRepo{prices: map[uuid.UUID]int64{}}
	svc := application.NewCartService(repo)
	_, err := svc.AddItem(context.Background(), anonOwner(), uuid.New(), 200)
	require.ErrorIs(t, err, domain.ErrInvalidQuantity)
}

func TestCartService_AddItem_UnknownVariant(t *testing.T) {
	repo := &fakeRepo{prices: map[uuid.UUID]int64{}}
	svc := application.NewCartService(repo)
	_, err := svc.AddItem(context.Background(), anonOwner(), uuid.New(), 1)
	require.ErrorIs(t, err, domain.ErrVariantNotFound)
}

func TestCartService_Get_NoCartReturnsEmpty(t *testing.T) {
	svc := application.NewCartService(&fakeRepo{})
	cart, err := svc.Get(context.Background(), anonOwner())
	require.NoError(t, err)
	assert.Empty(t, cart.Items)
}

func TestCartService_UpdateItem_NotFound(t *testing.T) {
	repo := &fakeRepo{hasCart: true, cart: domain.Cart{ID: uuid.New(), Status: domain.StatusActive}}
	svc := application.NewCartService(repo)
	_, err := svc.UpdateItem(context.Background(), anonOwner(), uuid.New(), 3)
	require.ErrorIs(t, err, domain.ErrItemNotFound)
}

func TestCartService_Merge_Delegates(t *testing.T) {
	repo := &fakeRepo{}
	svc := application.NewCartService(repo)
	require.NoError(t, svc.Merge(context.Background(), "anon123", uuid.New()))
	assert.True(t, repo.mergeCalled)
}

func TestCartService_Clear_NoOpWhenNoCart(t *testing.T) {
	svc := application.NewCartService(&fakeRepo{})
	require.NoError(t, svc.Clear(context.Background(), anonOwner()))
}

func TestCartService_RemoveItem_NoCart(t *testing.T) {
	svc := application.NewCartService(&fakeRepo{})
	_, err := svc.RemoveItem(context.Background(), anonOwner(), uuid.New())
	require.ErrorIs(t, err, domain.ErrCartNotFound)
}
