package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/transport"
)

type fakeUseCase struct {
	cart    domain.Cart
	addErr  error
	lastQty int
}

func (f *fakeUseCase) Get(_ context.Context, _ domain.Owner) (domain.Cart, error) { return f.cart, nil }
func (f *fakeUseCase) AddItem(_ context.Context, _ domain.Owner, _ uuid.UUID, qty int) (domain.Cart, error) {
	f.lastQty = qty
	if f.addErr != nil {
		return domain.Cart{}, f.addErr
	}
	f.cart.Items = append(f.cart.Items, domain.CartItem{ID: uuid.New(), Quantity: qty, UnitPriceCents: 1000})
	return f.cart, nil
}
func (f *fakeUseCase) UpdateItem(_ context.Context, _ domain.Owner, _ uuid.UUID, _ int) (domain.Cart, error) {
	return f.cart, nil
}
func (f *fakeUseCase) RemoveItem(_ context.Context, _ domain.Owner, _ uuid.UUID) (domain.Cart, error) {
	return f.cart, nil
}
func (f *fakeUseCase) Clear(_ context.Context, _ domain.Owner) error { return nil }

func router(uc transport.CartUseCase) chi.Router {
	h := transport.NewCartHandlers(uc, "cart_anon")
	r := chi.NewRouter()
	h.RegisterCartRoutes(r)
	return r
}

func TestAddItem_NoIdentity_MintsAnonCookie(t *testing.T) {
	uc := &fakeUseCase{}
	srv := httptest.NewServer(router(uc))
	defer srv.Close()

	body := strings.NewReader(`{"variant_id":"` + uuid.NewString() + `","quantity":2}`)
	resp, err := http.Post(srv.URL+"/cart/items", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "cart_anon" && c.Value != "" {
			found = true
		}
	}
	assert.True(t, found, "expected cart_anon cookie to be set")
	assert.Equal(t, 2, uc.lastQty)
}

func TestAddItem_OverCap_422(t *testing.T) {
	uc := &fakeUseCase{addErr: domain.ErrInvalidQuantity}
	srv := httptest.NewServer(router(uc))
	defer srv.Close()

	body := strings.NewReader(`{"variant_id":"` + uuid.NewString() + `","quantity":200}`)
	resp, err := http.Post(srv.URL+"/cart/items", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestGetCart_NoIdentity_EmptyCart(t *testing.T) {
	srv := httptest.NewServer(router(&fakeUseCase{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/cart")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var out transport.CartResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Empty(t, out.Items)
	assert.Equal(t, int64(0), out.SubtotalCents)
}
