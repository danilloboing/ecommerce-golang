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

// UserRepository is the Postgres implementation of application.UserRepository.
type UserRepository struct {
	q *queries.Queries
}

var _ application.UserRepository = (*UserRepository)(nil)

// NewUserRepository builds a UserRepository over the given pool.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{q: queries.New(pool)}
}

// Insert creates a user and returns the row.
// Returns domain.ErrEmailAlreadyTaken on unique violation.
func (r *UserRepository) Insert(ctx context.Context, email, name string) (domain.User, error) {
	row, err := r.q.InsertUser(ctx, queries.InsertUserParams{Email: email, Name: name})
	if err != nil {
		if isUniqueViolation(err) {
			return domain.User{}, domain.ErrEmailAlreadyTaken
		}
		return domain.User{}, fmt.Errorf("user repo: insert: %w", err)
	}
	return mapUser(row), nil
}

// FindByID returns ErrUserNotFound if missing.
func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	row, err := r.q.FindUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("user repo: find by id: %w", err)
	}
	return mapUser(row), nil
}

// FindByEmail returns ErrUserNotFound if missing.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := r.q.FindUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("user repo: find by email: %w", err)
	}
	return mapUser(row), nil
}

// MarkEmailVerified is idempotent — repeated calls do not error.
func (r *UserRepository) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	if err := r.q.MarkUserEmailVerified(ctx, id); err != nil {
		return fmt.Errorf("user repo: mark email verified: %w", err)
	}
	return nil
}

// UpdateName updates the name and returns the new row.
func (r *UserRepository) UpdateName(ctx context.Context, id uuid.UUID, name string) (domain.User, error) {
	row, err := r.q.UpdateUserName(ctx, queries.UpdateUserNameParams{ID: id, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("user repo: update name: %w", err)
	}
	return mapUser(row), nil
}
