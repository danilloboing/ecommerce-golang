// Package address wires the address bounded context.
package address

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/address/transport"
	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
)

// Compile-time assertion: *application.AddressService must satisfy transport.AddressWriter.
var _ transport.AddressWriter = (*application.AddressService)(nil)

// Module wires the address bounded context onto a chi router.
type Module struct {
	addresses     *transport.AddressHandlers
	cep           *transport.CEPHandler
	sessions      sessionauth.Manager
	sessionCookie string
	csrfCfg       csrf.Config
}

// Deps groups raw dependencies the address module needs.
type Deps struct {
	Pool          *pgxpool.Pool
	Sessions      sessionauth.Manager
	SessionCookie string
	CSRFCfg       csrf.Config
	ViaCEP        viacep.Lookuper
}

// New builds the address Module.
func New(d Deps) *Module {
	repo := infrastructure.New(d.Pool)
	svc := application.NewAddressService(repo)

	return &Module{
		addresses:     transport.NewAddressHandlers(svc),
		cep:           transport.NewCEPHandler(d.ViaCEP),
		sessions:      d.Sessions,
		sessionCookie: d.SessionCookie,
		csrfCfg:       d.CSRFCfg,
	}
}

// Mount registers the public CEP route and the authenticated address routes.
func (m *Module) Mount(r chi.Router) {
	r.Group(func(public chi.Router) {
		m.cep.RegisterCEPRoutes(public)
	})
	r.Group(func(auth chi.Router) {
		auth.Use(sessionauth.Middleware(m.sessions, m.sessionCookie))
		auth.Use(csrf.Middleware(m.csrfCfg))
		m.addresses.RegisterAddressRoutes(auth)
	})
}
