package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
)

// fakeOrderRepo is an in-memory OrderRepository for service unit tests.
type fakeOrderRepo struct {
	orders map[uuid.UUID]domain.Order
	items  map[uuid.UUID][]domain.OrderItem
}

func (f *fakeOrderRepo) Create(_ context.Context, no application.NewOrder) (domain.Order, error) {
	order := domain.Order{
		ID:               uuid.New(),
		UserID:           no.UserID,
		Status:           domain.PendingPayment,
		Subtotal:         no.Subtotal,
		Shipping:         no.Shipping,
		Discount:         no.Discount,
		Total:            no.Total,
		CouponCode:       no.CouponCode,
		AddressSnapshot:  no.AddressSnapshot,
		ShippingSnapshot: no.ShippingSnapshot,
	}
	f.orders[order.ID] = order
	return order, nil
}

func (f *fakeOrderRepo) GetByID(_ context.Context, id uuid.UUID) (domain.Order, error) {
	order, ok := f.orders[id]
	if !ok {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	return order, nil
}

func (f *fakeOrderRepo) GetUserOrder(_ context.Context, id, userID uuid.UUID) (domain.Order, error) {
	order, ok := f.orders[id]
	if !ok || order.UserID != userID {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	return order, nil
}

func (f *fakeOrderRepo) ListByUser(_ context.Context, userID uuid.UUID, limit int) ([]domain.Order, error) {
	var result []domain.Order
	for _, order := range f.orders {
		if order.UserID == userID {
			result = append(result, order)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (f *fakeOrderRepo) CreateItem(_ context.Context, orderID uuid.UUID, item application.NewOrderItem) (domain.OrderItem, error) {
	out := domain.OrderItem{
		ID:        uuid.New(),
		OrderID:   orderID,
		VariantID: item.VariantID,
		Quantity:  item.Quantity,
		UnitPrice: item.UnitPriceCents,
		Snapshot:  item.ProductSnapshot,
	}
	f.items[orderID] = append(f.items[orderID], out)
	return out, nil
}

func (f *fakeOrderRepo) ListItems(_ context.Context, orderID uuid.UUID) ([]domain.OrderItem, error) {
	items, ok := f.items[orderID]
	if !ok {
		return []domain.OrderItem{}, nil
	}
	return items, nil
}

func TestOrderService_GetForUser_Success(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()
	variantID := uuid.New()

	repo := &fakeOrderRepo{
		orders: map[uuid.UUID]domain.Order{
			orderID: {
				ID:        orderID,
				UserID:    userID,
				Status:    domain.PendingPayment,
				Subtotal:  5000,
				Shipping:  1000,
				Discount:  0,
				Total:     6000,
				CouponCode: nil,
			},
		},
		items: map[uuid.UUID][]domain.OrderItem{
			orderID: {
				{
					ID:        uuid.New(),
					OrderID:   orderID,
					VariantID: variantID,
					Quantity:  2,
					UnitPrice: 2500,
				},
			},
		},
	}

	svc := application.NewOrderService(repo)
	order, items, err := svc.GetForUser(context.Background(), orderID, userID)

	require.NoError(t, err)
	assert.Equal(t, orderID, order.ID)
	assert.Equal(t, userID, order.UserID)
	require.Len(t, items, 1)
	assert.Equal(t, int32(2), items[0].Quantity)
}

func TestOrderService_GetForUser_CrossUserReturnsError(t *testing.T) {
	ownerID := uuid.New()
	otherUserID := uuid.New()
	orderID := uuid.New()

	repo := &fakeOrderRepo{
		orders: map[uuid.UUID]domain.Order{
			orderID: {
				ID:     orderID,
				UserID: ownerID,
				Status: domain.PendingPayment,
			},
		},
		items: map[uuid.UUID][]domain.OrderItem{},
	}

	svc := application.NewOrderService(repo)
	_, _, err := svc.GetForUser(context.Background(), orderID, otherUserID)

	require.ErrorIs(t, err, domain.ErrOrderNotFound)
}

func TestOrderService_GetForUser_NotFoundReturnsError(t *testing.T) {
	userID := uuid.New()
	orderID := uuid.New()

	repo := &fakeOrderRepo{
		orders: map[uuid.UUID]domain.Order{},
		items:  map[uuid.UUID][]domain.OrderItem{},
	}

	svc := application.NewOrderService(repo)
	_, _, err := svc.GetForUser(context.Background(), orderID, userID)

	require.ErrorIs(t, err, domain.ErrOrderNotFound)
}

func TestOrderService_ListForUser_Success(t *testing.T) {
	userID := uuid.New()
	otherUserID := uuid.New()
	order1ID := uuid.New()
	order2ID := uuid.New()
	order3ID := uuid.New()

	repo := &fakeOrderRepo{
		orders: map[uuid.UUID]domain.Order{
			order1ID: {
				ID:     order1ID,
				UserID: userID,
				Status: domain.PendingPayment,
			},
			order2ID: {
				ID:     order2ID,
				UserID: userID,
				Status: domain.Paid,
			},
			order3ID: {
				ID:     order3ID,
				UserID: otherUserID,
				Status: domain.PendingPayment,
			},
		},
		items: map[uuid.UUID][]domain.OrderItem{},
	}

	svc := application.NewOrderService(repo)
	orders, err := svc.ListForUser(context.Background(), userID, 10)

	require.NoError(t, err)
	require.Len(t, orders, 2)
	assert.Equal(t, userID, orders[0].UserID)
	assert.Equal(t, userID, orders[1].UserID)
}

func TestOrderService_ListForUser_WithLimit(t *testing.T) {
	userID := uuid.New()
	order1ID := uuid.New()
	order2ID := uuid.New()
	order3ID := uuid.New()

	repo := &fakeOrderRepo{
		orders: map[uuid.UUID]domain.Order{
			order1ID: {
				ID:     order1ID,
				UserID: userID,
				Status: domain.PendingPayment,
			},
			order2ID: {
				ID:     order2ID,
				UserID: userID,
				Status: domain.Paid,
			},
			order3ID: {
				ID:     order3ID,
				UserID: userID,
				Status: domain.PaymentFailed,
			},
		},
		items: map[uuid.UUID][]domain.OrderItem{},
	}

	svc := application.NewOrderService(repo)
	orders, err := svc.ListForUser(context.Background(), userID, 2)

	require.NoError(t, err)
	assert.Len(t, orders, 2)
}
