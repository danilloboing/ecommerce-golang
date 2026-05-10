//go:build integration

package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/infrastructure"
)

func TestEmailVerifyTokenRepository_RoundTripAndConsume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(t)
	users := infrastructure.NewUserRepository(pool)
	tokens := infrastructure.NewEmailVerifyTokenRepository(pool)

	u, err := users.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)

	hash := []byte("0123456789abcdef0123456789abcdef")
	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond)

	require.NoError(t, tokens.Insert(ctx, hash, u.ID, "ana@example.com", expires))

	got, err := tokens.Find(ctx, hash)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.UserID)
	assert.False(t, got.IsConsumed())

	require.NoError(t, tokens.Consume(ctx, hash))
	got, err = tokens.Find(ctx, hash)
	require.NoError(t, err)
	assert.True(t, got.IsConsumed())
}

func TestEmailVerifyTokenRepository_FindReturnsNotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(t)
	tokens := infrastructure.NewEmailVerifyTokenRepository(pool)

	_, err := tokens.Find(ctx, []byte("does-not-exist--------------------"))
	require.True(t, errors.Is(err, domain.ErrTokenNotFound))
}

func TestPasswordResetTokenRepository_RoundTripAndConsume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(t)
	users := infrastructure.NewUserRepository(pool)
	tokens := infrastructure.NewPasswordResetTokenRepository(pool)

	u, err := users.Insert(ctx, "ana@example.com", "Ana")
	require.NoError(t, err)

	hash := []byte("aabbccddeeff00112233445566778899")
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)

	require.NoError(t, tokens.Insert(ctx, hash, u.ID, expires))

	got, err := tokens.Find(ctx, hash)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.UserID)
	require.NoError(t, tokens.Consume(ctx, hash))

	got, err = tokens.Find(ctx, hash)
	require.NoError(t, err)
	assert.True(t, got.IsConsumed())
}
