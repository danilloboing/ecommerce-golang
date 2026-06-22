// Package inventory wires the inventory bounded context.
package inventory

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/core/adminauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/transport"
)

// Module wires the inventory bounded context onto an HTTP router.
type Module struct {
	svc        *application.InventoryService
	handler    *transport.StockHandler
	adminToken string
}

// Deps groups raw dependencies the inventory module needs.
type Deps struct {
	Pool       *pgxpool.Pool
	AdminToken string
}

// New builds the inventory Module from its raw dependencies.
func New(d Deps) *Module {
	svc := application.NewInventoryService(infrastructure.New(d.Pool))
	return &Module{
		svc:        svc,
		handler:    transport.NewStockHandler(svc),
		adminToken: d.AdminToken,
	}
}

// Service exposes the InventoryService so other modules (e.g. checkout) can consume it.
func (m *Module) Service() *application.InventoryService { return m.svc }

// Mount registers admin inventory routes on the given router.
func (m *Module) Mount(r chi.Router) {
	r.Group(func(admin chi.Router) {
		admin.Use(adminauth.RequireToken(m.adminToken))
		m.handler.RegisterStockRoutes(admin)
	})
}
