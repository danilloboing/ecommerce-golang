// Package ordering wires the ordering bounded context.
package ordering

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/transport"
)

// Module wires the ordering bounded context onto a chi router.
type Module struct {
	handlers      *transport.OrderHandlers
	svc           *application.OrderService
	sessions      sessionauth.Manager
	sessionCookie string
}

// Deps groups raw dependencies the ordering module needs.
type Deps struct {
	Pool          *pgxpool.Pool
	Sessions      sessionauth.Manager
	SessionCookie string
}

// New builds the ordering Module.
func New(d Deps) *Module {
	repo := infrastructure.New(d.Pool)
	svc := application.NewOrderService(repo)
	return &Module{
		handlers:      transport.NewOrderHandlers(svc),
		svc:           svc,
		sessions:      d.Sessions,
		sessionCookie: d.SessionCookie,
	}
}

// Mount registers authenticated order routes behind sessionauth.Middleware.
func (m *Module) Mount(r chi.Router) {
	r.Group(func(auth chi.Router) {
		auth.Use(sessionauth.Middleware(m.sessions, m.sessionCookie))
		m.handlers.RegisterOrderRoutes(auth)
	})
}

// Service exposes the underlying OrderService for cross-module use.
func (m *Module) Service() *application.OrderService {
	return m.svc
}
