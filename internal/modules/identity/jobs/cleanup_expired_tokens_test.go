//go:build integration

package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/jobs"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

func TestCleanupExpiredTokens_DeletesOldTokens(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := newTestPool(t)
	users := infrastructure.NewUserRepository(pool)

	u, err := users.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)

	q := queries.New(pool)

	// One stale (expired 8 days ago), one fresh (expires tomorrow).
	stale := []byte("00112233445566778899aabbccddeeff")
	fresh := []byte("ffeeddccbbaa99887766554433221100")
	require.NoError(t, q.InsertEmailVerifyToken(ctx, queries.InsertEmailVerifyTokenParams{
		TokenHash: stale, UserID: u.ID, Email: "ana@example.com",
		ExpiresAt: time.Now().Add(-8 * 24 * time.Hour),
	}))
	require.NoError(t, q.InsertEmailVerifyToken(ctx, queries.InsertEmailVerifyTokenParams{
		TokenHash: fresh, UserID: u.ID, Email: "ana@example.com",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}))

	deleted, err := jobs.RunCleanupExpiredTokensOnce(ctx, pool)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, int64(1))

	_, err = q.FindEmailVerifyToken(ctx, stale)
	require.Error(t, err)
	got, err := q.FindEmailVerifyToken(ctx, fresh)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.UserID)

	_ = uuid.Nil
}
