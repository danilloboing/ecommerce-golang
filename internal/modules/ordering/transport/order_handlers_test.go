package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/transport"
)

// fakeOrderReader is a test double implementing transport.OrderReader.
type fakeOrderReader struct {
	getOrder  domain.Order
	getItems  []domain.OrderItem
	getErr    error
	listOrders []domain.Order
	listErr   error
}

func (f *fakeOrderReader) GetForUser(_ context.Context, _, _ uuid.UUID) (domain.Order, []domain.OrderItem, error) {
	return f.getOrder, f.getItems, f.getErr
}

func (f *fakeOrderReader) ListForUser(_ context.Context, _ uuid.UUID, _ int) ([]domain.Order, error) {
	return f.listOrders, f.listErr
}

// withSession injects a session into the request context.
func withSession(s sessionauth.Session) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := sessionauth.ContextWithSession(r.Context(), s)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newRouter(svc transport.OrderReader, useSession bool) chi.Router {
	h := transport.NewOrderHandlers(svc)
	r := chi.NewRouter()
	r.Group(func(grp chi.Router) {
		if useSession {
			grp.Use(withSession(sessionauth.Session{
				ID:     "sid",
				UserID: uuid.New(),
			}))
		}
		h.RegisterOrderRoutes(grp)
	})
	return r
}

// TestGetOrder_WithSession checks that GET /me/orders/{id} returns 200 with order JSON.
func TestGetOrder_WithSession(t *testing.T) {
	orderID := uuid.New()
	userID := uuid.New()

	order := domain.Order{
		ID:        orderID,
		UserID:    userID,
		Status:    domain.PendingPayment,
		Subtotal:  5000,
		Shipping:  1500,
		Discount:  0,
		Total:     6500,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	item := domain.OrderItem{
		ID:        uuid.New(),
		OrderID:   orderID,
		ProductID: uuid.New(),
		VariantID: uuid.New(),
		Quantity:  2,
		UnitPrice: 2500,
		CreatedAt: time.Now(),
	}

	svc := &fakeOrderReader{getOrder: order, getItems: []domain.OrderItem{item}}
	h := transport.NewOrderHandlers(svc)
	r := chi.NewRouter()
	r.Group(func(grp chi.Router) {
		grp.Use(withSession(sessionauth.Session{ID: "sid", UserID: userID}))
		h.RegisterOrderRoutes(grp)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me/orders/"+orderID.String(), nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body transport.OrderResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, orderID, body.ID)
	assert.Equal(t, int64(6500), body.Total)
	require.Len(t, body.Items, 1)
	assert.Equal(t, int32(2), body.Items[0].Quantity)
}

// TestGetOrder_WithoutSession checks that GET /me/orders/{id} returns 401 when no session.
func TestGetOrder_WithoutSession(t *testing.T) {
	svc := &fakeOrderReader{}
	h := transport.NewOrderHandlers(svc)
	r := chi.NewRouter()
	h.RegisterOrderRoutes(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me/orders/"+uuid.New().String(), nil))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestGetOrder_NotFound checks that GET /me/orders/{id} returns 404 when order not found.
func TestGetOrder_NotFound(t *testing.T) {
	svc := &fakeOrderReader{getErr: domain.ErrOrderNotFound}
	h := transport.NewOrderHandlers(svc)
	r := chi.NewRouter()
	r.Group(func(grp chi.Router) {
		grp.Use(withSession(sessionauth.Session{ID: "sid", UserID: uuid.New()}))
		h.RegisterOrderRoutes(grp)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me/orders/"+uuid.New().String(), nil))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestListOrders checks that GET /me/orders returns 200 with a JSON array.
func TestListOrders(t *testing.T) {
	userID := uuid.New()
	orders := []domain.Order{
		{
			ID:        uuid.New(),
			UserID:    userID,
			Status:    domain.Paid,
			Subtotal:  3000,
			Shipping:  500,
			Total:     3500,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			UserID:    userID,
			Status:    domain.PendingPayment,
			Subtotal:  7000,
			Shipping:  1000,
			Total:     8000,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	svc := &fakeOrderReader{listOrders: orders}
	h := transport.NewOrderHandlers(svc)
	r := chi.NewRouter()
	r.Group(func(grp chi.Router) {
		grp.Use(withSession(sessionauth.Session{ID: "sid", UserID: userID}))
		h.RegisterOrderRoutes(grp)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me/orders", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body []transport.OrderListItem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Len(t, body, 2)
	assert.Equal(t, int64(3500), body[0].Total)
}
