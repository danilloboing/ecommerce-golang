package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// AuthMethodRepository is the Postgres implementation of application.AuthMethodRepository.
type AuthMethodRepository struct {
	q *queries.Queries
}

var _ application.AuthMethodRepository = (*AuthMethodRepository)(nil)

// NewAuthMethodRepository builds an AuthMethodRepository over the given pool.
func NewAuthMethodRepository(pool *pgxpool.Pool) *AuthMethodRepository {
	return &AuthMethodRepository{q: queries.New(pool)}
}

// InsertPassword creates a password auth method bound to the given user.
func (r *AuthMethodRepository) InsertPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (domain.AuthMethod, error) {
	row, err := r.q.InsertAuthMethodPassword(ctx, queries.InsertAuthMethodPasswordParams{
		UserID:       userID,
		PasswordHash: stringPtr(passwordHash),
	})
	if err != nil {
		return domain.AuthMethod{}, fmt.Errorf("auth_method repo: insert password: %w", err)
	}
	return mapAuthMethod(row), nil
}

// FindForUser locates the auth method for (userID, provider).
// Returns domain.ErrUserNotFound if no row exists for the pair.
func (r *AuthMethodRepository) FindForUser(ctx context.Context, userID uuid.UUID, provider domain.AuthProvider) (domain.AuthMethod, error) {
	row, err := r.q.FindAuthMethodByUserAndProvider(ctx, queries.FindAuthMethodByUserAndProviderParams{
		UserID:   userID,
		Provider: string(provider),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AuthMethod{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.AuthMethod{}, fmt.Errorf("auth_method repo: find: %w", err)
	}
	return mapAuthMethod(row), nil
}

// UpdatePassword rotates the password hash for the user's password method.
func (r *AuthMethodRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	if err := r.q.UpdateAuthMethodPassword(ctx, queries.UpdateAuthMethodPasswordParams{
		UserID:       userID,
		PasswordHash: stringPtr(passwordHash),
	}); err != nil {
		return fmt.Errorf("auth_method repo: update password: %w", err)
	}
	return nil
}

// TouchLastUsed bumps last_used_at on a successful authentication.
func (r *AuthMethodRepository) TouchLastUsed(ctx context.Context, id uuid.UUID) error {
	if err := r.q.TouchAuthMethodLastUsed(ctx, id); err != nil {
		return fmt.Errorf("auth_method repo: touch last used: %w", err)
	}
	return nil
}

func stringPtr(s string) *string { return &s }
