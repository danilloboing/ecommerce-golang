package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// EmailVerifyTokenRepository persists email-verification tokens.
type EmailVerifyTokenRepository struct {
	q *queries.Queries
}

var _ application.EmailVerifyTokenRepository = (*EmailVerifyTokenRepository)(nil)

// NewEmailVerifyTokenRepository builds an EmailVerifyTokenRepository over the given pool.
func NewEmailVerifyTokenRepository(pool *pgxpool.Pool) *EmailVerifyTokenRepository {
	return &EmailVerifyTokenRepository{q: queries.New(pool)}
}

// Insert persists a new email-verification token.
func (r *EmailVerifyTokenRepository) Insert(ctx context.Context, hash []byte, userID uuid.UUID, email string, expiresAt time.Time) error {
	if err := r.q.InsertEmailVerifyToken(ctx, queries.InsertEmailVerifyTokenParams{
		TokenHash: hash,
		UserID:    userID,
		Email:     email,
		ExpiresAt: expiresAt,
	}); err != nil {
		return fmt.Errorf("verify token repo: insert: %w", err)
	}
	return nil
}

// Find returns the token row for hash. Returns domain.ErrTokenNotFound if missing.
func (r *EmailVerifyTokenRepository) Find(ctx context.Context, hash []byte) (domain.EmailVerifyToken, error) {
	row, err := r.q.FindEmailVerifyToken(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EmailVerifyToken{}, domain.ErrTokenNotFound
	}
	if err != nil {
		return domain.EmailVerifyToken{}, fmt.Errorf("verify token repo: find: %w", err)
	}
	return mapEmailVerifyToken(row), nil
}

// Consume marks the token as consumed. Idempotent: re-consuming is a no-op.
func (r *EmailVerifyTokenRepository) Consume(ctx context.Context, hash []byte) error {
	if err := r.q.ConsumeEmailVerifyToken(ctx, hash); err != nil {
		return fmt.Errorf("verify token repo: consume: %w", err)
	}
	return nil
}

// PasswordResetTokenRepository persists password-reset tokens.
type PasswordResetTokenRepository struct {
	q *queries.Queries
}

var _ application.PasswordResetTokenRepository = (*PasswordResetTokenRepository)(nil)

// NewPasswordResetTokenRepository builds a PasswordResetTokenRepository over the given pool.
func NewPasswordResetTokenRepository(pool *pgxpool.Pool) *PasswordResetTokenRepository {
	return &PasswordResetTokenRepository{q: queries.New(pool)}
}

// Insert persists a new password-reset token.
func (r *PasswordResetTokenRepository) Insert(ctx context.Context, hash []byte, userID uuid.UUID, expiresAt time.Time) error {
	if err := r.q.InsertPasswordResetToken(ctx, queries.InsertPasswordResetTokenParams{
		TokenHash: hash,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}); err != nil {
		return fmt.Errorf("reset token repo: insert: %w", err)
	}
	return nil
}

// Find returns the token row for hash. Returns domain.ErrTokenNotFound if missing.
func (r *PasswordResetTokenRepository) Find(ctx context.Context, hash []byte) (domain.PasswordResetToken, error) {
	row, err := r.q.FindPasswordResetToken(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PasswordResetToken{}, domain.ErrTokenNotFound
	}
	if err != nil {
		return domain.PasswordResetToken{}, fmt.Errorf("reset token repo: find: %w", err)
	}
	return mapPasswordResetToken(row), nil
}

// Consume marks the token as consumed. Idempotent: re-consuming is a no-op.
func (r *PasswordResetTokenRepository) Consume(ctx context.Context, hash []byte) error {
	if err := r.q.ConsumePasswordResetToken(ctx, hash); err != nil {
		return fmt.Errorf("reset token repo: consume: %w", err)
	}
	return nil
}
