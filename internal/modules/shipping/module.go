// Package shipping wires the shipping bounded context.
package shipping

import (
	"fmt"

	"github.com/danilloboing/marketplace-golang/internal/modules/shipping/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/shipping/infrastructure"
)

// Module holds the wired shipping context. It has no HTTP routes in phase 3a;
// quotes are consumed directly by the checkout module via Service().
type Module struct {
	svc *application.ShippingService
}

// Deps groups the raw configuration the shipping module needs at startup.
type Deps struct {
	// Provider selects the shipping backend. Supported values:
	//   "mock" — deterministic in-memory stub (development / tests)
	// Additional providers (e.g. "melhorenvio") will be added in phase 3b.
	Provider string
}

// New constructs a shipping Module using the provider selected by d.Provider.
// It panics on an unknown provider to surface misconfiguration at startup.
func New(d Deps) *Module {
	var provider application.ShippingProvider

	switch d.Provider {
	case "mock":
		provider = &infrastructure.MockShipping{}
	default:
		panic(fmt.Sprintf("shipping: unknown provider %q", d.Provider))
	}

	return &Module{svc: application.NewShippingService(provider)}
}

// Service returns the ShippingService for use by other modules (e.g. checkout).
func (m *Module) Service() *application.ShippingService { return m.svc }
