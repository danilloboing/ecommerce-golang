// Package identity wires the identity bounded context.
package identity

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/core/csrf"
	"github.com/danilloboing/marketplace-golang/internal/core/ratelimit"
	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/transport"
	"github.com/danilloboing/marketplace-golang/internal/platform/email"
)

// Module wires the identity bounded context onto a chi router.
type Module struct {
	auth     *transport.AuthHandlers
	me       *transport.MeHandlers
	sessions sessionauth.Manager
	cookies  transport.CookieConfig
	csrfCfg  csrf.Config
	rl       func(http.Handler) http.Handler
}

// Deps groups raw dependencies the module needs.
type Deps struct {
	Pool           *pgxpool.Pool
	Redis          *redis.Client
	Email          email.Sender
	Sessions       sessionauth.Manager
	Cookies        transport.CookieConfig
	CSRFCfg        csrf.Config
	RateLimitOpts  ratelimit.Options
	Cfg            config.Config
	CartMerge      func(ctx context.Context, anonID string, userID uuid.UUID) error
	CartCookieName string
}

// New builds the identity Module.
func New(d Deps) *Module {
	users := infrastructure.NewUserRepository(d.Pool)
	auths := infrastructure.NewAuthMethodRepository(d.Pool)
	verify := infrastructure.NewEmailVerifyTokenRepository(d.Pool)
	reset := infrastructure.NewPasswordResetTokenRepository(d.Pool)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users:                   users,
		AuthMethods:             auths,
		VerifyTokens:            verify,
		ResetTokens:             reset,
		Email:                   d.Email,
		VerifyLinkBaseURL:       d.Cfg.Email.VerifyLinkBaseURL,
		ResetLinkBaseURL:        d.Cfg.Email.ResetLinkBaseURL,
		RevokeAllSessions:       d.Sessions.DeleteAllForUser,
		RevokeAllSessionsExcept: d.Sessions.DeleteAllForUserExcept,
	})

	auth := transport.NewAuthHandlers(svc, d.Sessions, d.Cookies)
	auth.SetCartMerge(d.CartMerge, d.CartCookieName)

	return &Module{
		auth:     auth,
		me:       transport.NewMeHandlers(svc, d.Sessions, d.Cookies),
		sessions: d.Sessions,
		cookies:  d.Cookies,
		csrfCfg:  d.CSRFCfg,
	}
}

// AuthHandler exposes the auth handler for per-endpoint route mounting.
func (m *Module) AuthHandler() *transport.AuthHandlers { return m.auth }

// MeHandler exposes the me handler for per-endpoint route mounting.
func (m *Module) MeHandler() *transport.MeHandlers { return m.me }

// Mount registers public + authenticated routes.
//
// Public routes get rate-limit middleware composed via the per-route group.
// Authenticated routes also pick up sessionauth + csrf middleware.
func (m *Module) Mount(r chi.Router) {
	r.Group(func(public chi.Router) {
		m.auth.RegisterAuthRoutes(public)
	})

	r.Group(func(auth chi.Router) {
		auth.Use(sessionauth.Middleware(m.sessions, m.cookies.SessionName))
		auth.Use(csrf.Middleware(m.csrfCfg))
		m.me.RegisterMeRoutes(auth)
	})
}
