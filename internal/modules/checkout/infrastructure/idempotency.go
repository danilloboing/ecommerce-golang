package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/application"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// IdempotencyRepo resolves idempotency keys before the confirm transaction. The
// matching insert happens atomically inside ConfirmTx; this read-side only
// classifies an existing key as a safe replay or a conflicting reuse.
type IdempotencyRepo struct {
	q *queries.Queries
}

var _ application.Idempotency = (*IdempotencyRepo)(nil)

// NewIdempotencyRepo builds an IdempotencyRepo from a pgx pool.
func NewIdempotencyRepo(pool *pgxpool.Pool) *IdempotencyRepo {
	return &IdempotencyRepo{q: queries.New(pool)}
}

// Lookup classifies an idempotency key for (userID, key):
//   - absent            → IdemHit{} (zero value)
//   - same requestHash  → IdemHit{Replay: true} with the decoded stored result
//   - different hash    → IdemHit{Conflict: true} (key reused for another request)
func (r *IdempotencyRepo) Lookup(ctx context.Context, userID uuid.UUID, key, requestHash string) (application.IdemHit, error) {
	row, err := r.q.GetIdempotencyKey(ctx, queries.GetIdempotencyKeyParams{UserID: userID, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.IdemHit{}, nil
	}
	if err != nil {
		return application.IdemHit{}, fmt.Errorf("checkout idempotency: lookup: %w", err)
	}

	if row.RequestHash != requestHash {
		return application.IdemHit{Conflict: true}, nil
	}

	var stored application.ConfirmResult
	if err := json.Unmarshal(row.Response, &stored); err != nil {
		return application.IdemHit{}, fmt.Errorf("checkout idempotency: decode stored result: %w", err)
	}
	return application.IdemHit{Replay: true, StoredResult: &stored}, nil
}
