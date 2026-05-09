// Package catalog wires the catalog bounded context.
package catalog

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/core/adminauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/transport"
)

// Module wires the catalog bounded context onto an HTTP router.
type Module struct {
	publicHandler *transport.PublicHandler
	adminHandler  *transport.AdminHandler
	adminToken    string
}

// New builds the catalog module from its raw dependencies.
func New(pool *pgxpool.Pool, adminToken string) *Module {
	repo := infrastructure.New(pool)
	publicSvc := application.NewPublicService(repo, repo)
	adminSvc := application.NewAdminService(repo)

	return &Module{
		publicHandler: transport.NewPublicHandler(publicSvc),
		adminHandler:  transport.NewAdminHandler(adminSvc),
		adminToken:    adminToken,
	}
}

// Mount registers public and admin routes on the given router.
func (m *Module) Mount(r chi.Router) {
	r.Group(func(public chi.Router) {
		m.publicHandler.RegisterPublicRoutes(public)
	})
	r.Group(func(admin chi.Router) {
		admin.Use(adminauth.RequireToken(m.adminToken))
		m.adminHandler.RegisterAdminRoutes(admin)
	})
}
