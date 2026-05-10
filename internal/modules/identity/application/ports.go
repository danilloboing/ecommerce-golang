// Package application contains the IdentityService and its repository ports.
package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
)

// UserRepository persists the user aggregate.
type UserRepository interface {
	Insert(ctx context.Context, email, name string) (domain.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (domain.User, error)
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	MarkEmailVerified(ctx context.Context, id uuid.UUID) error
	UpdateName(ctx context.Context, id uuid.UUID, name string) (domain.User, error)
}

// AuthMethodRepository persists per-user credentials.
type AuthMethodRepository interface {
	InsertPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (domain.AuthMethod, error)
	FindForUser(ctx context.Context, userID uuid.UUID, provider domain.AuthProvider) (domain.AuthMethod, error)
	UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
	TouchLastUsed(ctx context.Context, id uuid.UUID) error
}

// EmailVerifyTokenRepository persists email-verification tokens.
type EmailVerifyTokenRepository interface {
	Insert(ctx context.Context, hash []byte, userID uuid.UUID, email string, expiresAt time.Time) error
	Find(ctx context.Context, hash []byte) (domain.EmailVerifyToken, error)
	Consume(ctx context.Context, hash []byte) error
}

// PasswordResetTokenRepository persists password-reset tokens.
type PasswordResetTokenRepository interface {
	Insert(ctx context.Context, hash []byte, userID uuid.UUID, expiresAt time.Time) error
	Find(ctx context.Context, hash []byte) (domain.PasswordResetToken, error)
	Consume(ctx context.Context, hash []byte) error
}
