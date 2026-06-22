package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// Repository is the Postgres-backed address store.
type Repository struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

// New builds a Repository from a pgx pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: queries.New(pool)}
}

// Create persists a new address; when IsDefault it clears the prior default in one tx.
func (r *Repository) Create(ctx context.Context, a domain.Address) (domain.Address, error) {
	if !a.IsDefault {
		row, err := r.q.CreateAddress(ctx, createParams(a))
		if err != nil {
			return domain.Address{}, fmt.Errorf("address repo: create: %w", err)
		}
		return mapAddress(row), nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if err := q.ClearDefaultAddress(ctx, a.UserID); err != nil {
		return domain.Address{}, fmt.Errorf("address repo: clear default: %w", err)
	}
	row, err := q.CreateAddress(ctx, createParams(a))
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: create default: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Address{}, fmt.Errorf("address repo: commit: %w", err)
	}
	return mapAddress(row), nil
}

// GetByID returns an address scoped to the user.
func (r *Repository) GetByID(ctx context.Context, id, userID uuid.UUID) (domain.Address, error) {
	row, err := r.q.GetAddressByID(ctx, queries.GetAddressByIDParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Address{}, domain.ErrAddressNotFound
	}
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: get: %w", err)
	}
	return mapAddress(row), nil
}

// List returns the user's addresses.
func (r *Repository) List(ctx context.Context, userID uuid.UUID) ([]domain.Address, error) {
	rows, err := r.q.ListAddressesByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("address repo: list: %w", err)
	}
	out := make([]domain.Address, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapAddress(row))
	}
	return out, nil
}

// Update persists mutable fields scoped to the user.
func (r *Repository) Update(ctx context.Context, a domain.Address) (domain.Address, error) {
	row, err := r.q.UpdateAddress(ctx, queries.UpdateAddressParams{
		ID:            a.ID,
		UserID:        a.UserID,
		RecipientName: a.RecipientName,
		PostalCode:    a.PostalCode,
		Street:        a.Street,
		Number:        a.Number,
		Complement:    a.Complement,
		Neighborhood:  a.Neighborhood,
		City:          a.City,
		State:         a.State,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Address{}, domain.ErrAddressNotFound
	}
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: update: %w", err)
	}
	return mapAddress(row), nil
}

// Delete removes an address scoped to the user.
func (r *Repository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	n, err := r.q.DeleteAddress(ctx, queries.DeleteAddressParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("address repo: delete: %w", err)
	}
	if n == 0 {
		return domain.ErrAddressNotFound
	}
	return nil
}

// SetDefault clears the current default and marks id default, in one tx.
func (r *Repository) SetDefault(ctx context.Context, id, userID uuid.UUID) (domain.Address, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if err := q.ClearDefaultAddress(ctx, userID); err != nil {
		return domain.Address{}, fmt.Errorf("address repo: clear default: %w", err)
	}
	row, err := q.SetDefaultAddress(ctx, queries.SetDefaultAddressParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Address{}, domain.ErrAddressNotFound
	}
	if err != nil {
		return domain.Address{}, fmt.Errorf("address repo: set default: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Address{}, fmt.Errorf("address repo: commit: %w", err)
	}
	return mapAddress(row), nil
}

func createParams(a domain.Address) queries.CreateAddressParams {
	return queries.CreateAddressParams{
		ID:            a.ID,
		UserID:        a.UserID,
		RecipientName: a.RecipientName,
		PostalCode:    a.PostalCode,
		Street:        a.Street,
		Number:        a.Number,
		Complement:    a.Complement,
		Neighborhood:  a.Neighborhood,
		City:          a.City,
		State:         a.State,
		IsDefault:     a.IsDefault,
	}
}
