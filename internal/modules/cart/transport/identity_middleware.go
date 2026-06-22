// Package transport adapts cart use cases to HTTP.
package transport

import (
	"context"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
)

type ownerCtxKey struct{}

// ContextWithOwner injects a resolved cart owner.
func ContextWithOwner(ctx context.Context, owner domain.Owner) context.Context {
	return context.WithValue(ctx, ownerCtxKey{}, owner)
}

// OwnerFromContext returns the resolved owner, or false if none was resolved.
func OwnerFromContext(ctx context.Context) (domain.Owner, bool) {
	o, ok := ctx.Value(ownerCtxKey{}).(domain.Owner)
	return o, ok
}

// ResolveCartIdentity resolves the cart owner without requiring authentication.
// Preference: a valid user session, else an existing cart_anon cookie. When
// neither is present no owner is injected (handlers decide whether to mint one).
func ResolveCartIdentity(sessions sessionauth.Manager, sessionCookieName, anonCookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
				if sess, err := sessions.Get(r.Context(), c.Value); err == nil {
					uid := sess.UserID
					ctx := ContextWithOwner(r.Context(), domain.Owner{UserID: &uid})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			if c, err := r.Cookie(anonCookieName); err == nil && c.Value != "" {
				id := c.Value
				ctx := ContextWithOwner(r.Context(), domain.Owner{AnonID: &id})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
