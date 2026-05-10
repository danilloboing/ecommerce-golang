// Package sessionauth provides user session management and HTTP middleware.
//
// Sessions live in Redis only. A successful login creates a fresh session id
// (32 random bytes hex) and a CSRF token (same shape) stored in a Redis hash.
// A secondary set indexes sessions per user for "logout all devices".
package sessionauth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Errors returned by Manager.
var (
	ErrNotFound = errors.New("sessionauth: session not found")
	ErrExpired  = errors.New("sessionauth: session expired")
)

// Session is the authenticated state for a single (user, device) pair.
type Session struct {
	ID             string
	UserID         uuid.UUID
	CSRFToken      string
	CreatedAt      time.Time
	LastActivityAt time.Time
	ExpiresAt      time.Time
	RememberMe     bool
	UserAgent      string
	IP             string
}

// CreateParams describes a new session being created at login.
type CreateParams struct {
	UserID     uuid.UUID
	RememberMe bool
	UserAgent  string
	IP         string
}

// Manager is the session store contract used by transport handlers.
type Manager interface {
	Create(ctx context.Context, p CreateParams) (Session, error)
	Get(ctx context.Context, sessionID string) (Session, error)
	Refresh(ctx context.Context, sessionID string) error
	Delete(ctx context.Context, sessionID string) error
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
	DeleteAllForUserExcept(ctx context.Context, userID uuid.UUID, keepID string) error
}

type ctxKey struct{}

// ContextWithSession injects s into ctx.
func ContextWithSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// SessionFromContext returns the session stored by Middleware, or false.
func SessionFromContext(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(Session)
	return s, ok
}
