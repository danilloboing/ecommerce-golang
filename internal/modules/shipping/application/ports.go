// Package application contains the shipping service and its port interfaces.
package application

import (
	"context"

	"github.com/danilloboing/marketplace-golang/internal/modules/shipping/domain"
)

// ShippingProvider is the port that concrete shipping carriers must implement.
type ShippingProvider interface {
	Quote(ctx context.Context, req domain.QuoteRequest) ([]domain.Quote, error)
}

// ShippingService delegates quote requests to the injected provider.
type ShippingService struct{ provider ShippingProvider }

// NewShippingService builds a ShippingService backed by the given provider.
func NewShippingService(p ShippingProvider) *ShippingService {
	return &ShippingService{provider: p}
}

// Quote fetches shipping options from the underlying provider.
func (s *ShippingService) Quote(ctx context.Context, req domain.QuoteRequest) ([]domain.Quote, error) {
	return s.provider.Quote(ctx, req)
}
