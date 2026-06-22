package shipping_test

import (
	"context"
	"testing"

	shipping "github.com/danilloboing/marketplace-golang/internal/modules/shipping"
	"github.com/danilloboing/marketplace-golang/internal/modules/shipping/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/shipping/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/shipping/infrastructure"
)

func TestMockShipping_QuoteReturnsTwoServices(t *testing.T) {
	mock := &infrastructure.MockShipping{}
	req := domain.QuoteRequest{PostalCode: "01310-100", SubtotalCents: 5000}

	quotes, err := mock.Quote(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) < 2 {
		t.Fatalf("expected ≥2 quotes, got %d", len(quotes))
	}
	for i, q := range quotes {
		if q.PriceCents <= 0 {
			t.Errorf("quote[%d] PriceCents must be positive, got %d", i, q.PriceCents)
		}
		if q.ETADays <= 0 {
			t.Errorf("quote[%d] ETADays must be positive, got %d", i, q.ETADays)
		}
		if q.ServiceID == "" {
			t.Errorf("quote[%d] ServiceID must not be empty", i)
		}
		if q.Name == "" {
			t.Errorf("quote[%d] Name must not be empty", i)
		}
	}
}

func TestShippingService_QuoteDelegatesToProvider(t *testing.T) {
	mock := &infrastructure.MockShipping{}
	svc := application.NewShippingService(mock)

	req := domain.QuoteRequest{PostalCode: "20040-020", SubtotalCents: 12000}
	quotes, err := svc.Quote(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error from ShippingService.Quote: %v", err)
	}
	if len(quotes) == 0 {
		t.Fatal("ShippingService.Quote returned no quotes")
	}
}

func TestModule_New_MockProvider(t *testing.T) {
	m := shipping.New(shipping.Deps{Provider: "mock"})
	if m.Service() == nil {
		t.Fatal("Service() must not be nil")
	}
}

func TestModule_New_UnknownProvider_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unknown provider, got none")
		}
	}()
	shipping.New(shipping.Deps{Provider: "unknown-provider-xyz"})
}
