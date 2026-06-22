// Package infrastructure holds concrete implementations of shipping ports.
package infrastructure

import (
	"context"

	"github.com/danilloboing/marketplace-golang/internal/modules/shipping/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/shipping/domain"
)

// Compile-time assertion: MockShipping must satisfy ShippingProvider.
var _ application.ShippingProvider = (*MockShipping)(nil)

// MockShipping is a deterministic in-memory ShippingProvider for tests and
// local development. It returns two services — PAC and SEDEX — with fixed
// prices and ETAs, optionally varying by the first digit of the postal code.
type MockShipping struct{}

// Quote returns two deterministic shipping options for any valid request.
func (m *MockShipping) Quote(_ context.Context, req domain.QuoteRequest) ([]domain.Quote, error) {
	if req.PostalCode == "" {
		return nil, domain.ErrQuoteUnavailable
	}

	// Vary prices slightly by the first digit of the CEP so different regions
	// produce distinct values (useful for integration scenarios).
	var regionBonus int64
	if len(req.PostalCode) > 0 && req.PostalCode[0] >= '0' && req.PostalCode[0] <= '9' {
		regionBonus = int64(req.PostalCode[0]-'0') * 10
	}

	return []domain.Quote{
		{
			ServiceID:  "pac",
			Name:       "PAC",
			PriceCents: 1990 + regionBonus,
			ETADays:    8,
		},
		{
			ServiceID:  "sedex",
			Name:       "SEDEX",
			PriceCents: 3490 + regionBonus,
			ETADays:    3,
		},
	}, nil
}
