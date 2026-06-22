package domain

import (
	"time"

	"github.com/google/uuid"
)

// AuthProvider enumerates supported login methods.
type AuthProvider string

const (
	AuthProviderPassword AuthProvider = "password"
	AuthProviderGoogle   AuthProvider = "google" // implementation deferred to Phase 2.5
)

// AuthMethod is one credential bound to a user.
type AuthMethod struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Provider        AuthProvider
	PasswordHash    *string // present only when Provider == AuthProviderPassword
	ProviderSubject *string // present only for OAuth providers
	CreatedAt       time.Time
	LastUsedAt      *time.Time
}
