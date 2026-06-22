// Package payment wires the payment bounded context.
package payment

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/transport"
)

// Deps groups raw dependencies the payment module needs.
type Deps struct {
	Pool     *pgxpool.Pool
	Provider application.PaymentProvider
	Applier  application.EventApplier
}

// Module wires the payment bounded context.
type Module struct {
	chargeService  *application.ChargeService
	webhookHandler *transport.WebhookHandler
}

// New builds a payment Module from its dependencies.
func New(d Deps) *Module {
	repo := infrastructure.New(d.Pool)
	svc := application.NewChargeService(d.Provider, repo)
	wh := transport.NewWebhookHandler(d.Provider, d.Applier)
	return &Module{chargeService: svc, webhookHandler: wh}
}

// SetApplier installs or replaces the EventApplier after module construction.
// This is the alternative wiring path for checkout to inject the applier
// without circular imports.
func (m *Module) SetApplier(a application.EventApplier) {
	m.webhookHandler = transport.NewWebhookHandler(m.webhookHandler.Provider(), a)
}

// Mount registers the payment routes onto the router.
// The webhook is reachable at POST /payments/webhook relative to the router.
func (m *Module) Mount(r chi.Router) {
	m.webhookHandler.RegisterWebhookRoutes(r)
}

// ChargeService exposes the charge orchestration use case for checkout wiring.
func (m *Module) ChargeService() *application.ChargeService {
	return m.chargeService
}
