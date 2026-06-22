package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/cart/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/cart/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// Repository is the Postgres-backed cart store.
type Repository struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

var _ application.CartRepository = (*Repository)(nil)

// New builds a Repository from a pgx pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: queries.New(pool)}
}

// FindActive returns the owner's active cart with items loaded.
func (r *Repository) FindActive(ctx context.Context, owner domain.Owner) (domain.Cart, error) {
	row, err := r.activeCartRow(ctx, r.q, owner)
	if err != nil {
		return domain.Cart{}, err
	}
	items, err := r.q.ListCartItems(ctx, row.ID)
	if err != nil {
		return domain.Cart{}, fmt.Errorf("cart repo: list items: %w", err)
	}
	return mapCart(row, items), nil
}

// EnsureActive returns the owner's active cart, creating an empty one if absent.
func (r *Repository) EnsureActive(ctx context.Context, owner domain.Owner) (domain.Cart, error) {
	cart, err := r.FindActive(ctx, owner)
	if err == nil {
		return cart, nil
	}
	if !errors.Is(err, domain.ErrCartNotFound) {
		return domain.Cart{}, err
	}
	var row queries.Cart
	switch {
	case owner.UserID != nil:
		row, err = r.q.CreateUserCart(ctx, owner.UserID)
	case owner.AnonID != nil:
		row, err = r.q.CreateAnonCart(ctx, owner.AnonID)
	default:
		return domain.Cart{}, fmt.Errorf("cart repo: invalid owner")
	}
	if err != nil {
		return domain.Cart{}, fmt.Errorf("cart repo: create cart: %w", err)
	}
	return mapCart(row, nil), nil
}

// VariantUnitPrice returns the effective unit price for a variant.
func (r *Repository) VariantUnitPrice(ctx context.Context, variantID uuid.UUID) (int64, error) {
	price, err := r.q.GetVariantUnitPrice(ctx, variantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrVariantNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("cart repo: variant price: %w", err)
	}
	return price, nil
}

// UpsertItem inserts or sums a cart line (clamped to 99 by the query).
func (r *Repository) UpsertItem(ctx context.Context, cartID, variantID uuid.UUID, qty int, unitPrice int64) error {
	_, err := r.q.UpsertCartItem(ctx, queries.UpsertCartItemParams{
		CartID:         cartID,
		VariantID:      variantID,
		Quantity:       int32(qty),
		UnitPriceCents: unitPrice,
	})
	if err != nil {
		return fmt.Errorf("cart repo: upsert item: %w", err)
	}
	return nil
}

// UpdateItemQuantity sets a line quantity scoped to the cart.
func (r *Repository) UpdateItemQuantity(ctx context.Context, cartID, itemID uuid.UUID, qty int) error {
	_, err := r.q.UpdateCartItemQuantity(ctx, queries.UpdateCartItemQuantityParams{
		ID:       itemID,
		CartID:   cartID,
		Quantity: int32(qty),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrItemNotFound
	}
	if err != nil {
		return fmt.Errorf("cart repo: update item: %w", err)
	}
	return nil
}

// DeleteItem removes a line scoped to the cart.
func (r *Repository) DeleteItem(ctx context.Context, cartID, itemID uuid.UUID) error {
	n, err := r.q.DeleteCartItem(ctx, queries.DeleteCartItemParams{ID: itemID, CartID: cartID})
	if err != nil {
		return fmt.Errorf("cart repo: delete item: %w", err)
	}
	if n == 0 {
		return domain.ErrItemNotFound
	}
	return nil
}

// ClearItems removes all lines from a cart.
func (r *Repository) ClearItems(ctx context.Context, cartID uuid.UUID) error {
	if err := r.q.DeleteCartItemsByCart(ctx, cartID); err != nil {
		return fmt.Errorf("cart repo: clear items: %w", err)
	}
	return nil
}

// Merge folds the anon cart into the user's active cart. It retries once if a
// concurrent user-cart create trips carts_user_active_uniq (23505), since a
// poisoned tx must be restarted, not continued.
func (r *Repository) Merge(ctx context.Context, anonID string, userID uuid.UUID) error {
	const maxAttempts = 2
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = r.mergeOnce(ctx, anonID, userID)
		if err == nil || !isUniqueViolation(err) {
			return err
		}
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *Repository) mergeOnce(ctx context.Context, anonID string, userID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cart repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	anonCart, err := q.GetActiveCartByAnon(ctx, &anonID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // nothing to merge
	}
	if err != nil {
		return fmt.Errorf("cart repo: merge get anon: %w", err)
	}

	userCart, err := q.GetActiveCartByUser(ctx, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		userCart, err = q.CreateUserCart(ctx, &userID)
	}
	if err != nil {
		return fmt.Errorf("cart repo: merge ensure user cart: %w", err)
	}

	items, err := q.ListCartItems(ctx, anonCart.ID)
	if err != nil {
		return fmt.Errorf("cart repo: merge list items: %w", err)
	}
	for _, it := range items {
		if _, err := q.UpsertCartItem(ctx, queries.UpsertCartItemParams{
			CartID:         userCart.ID,
			VariantID:      it.VariantID,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
		}); err != nil {
			return fmt.Errorf("cart repo: merge upsert: %w", err)
		}
	}

	if err := q.SetCartStatus(ctx, queries.SetCartStatusParams{ID: anonCart.ID, Status: string(domain.StatusMerged)}); err != nil {
		return fmt.Errorf("cart repo: merge mark status: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *Repository) activeCartRow(ctx context.Context, q *queries.Queries, owner domain.Owner) (queries.Cart, error) {
	var row queries.Cart
	var err error
	switch {
	case owner.UserID != nil:
		row, err = q.GetActiveCartByUser(ctx, owner.UserID)
	case owner.AnonID != nil:
		row, err = q.GetActiveCartByAnon(ctx, owner.AnonID)
	default:
		return queries.Cart{}, fmt.Errorf("cart repo: invalid owner")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return queries.Cart{}, domain.ErrCartNotFound
	}
	if err != nil {
		return queries.Cart{}, fmt.Errorf("cart repo: get active cart: %w", err)
	}
	return row, nil
}
