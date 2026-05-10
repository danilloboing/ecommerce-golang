//go:build integration

package sessionauth_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/core/sessionauth"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

func newRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := testutil.NewTestRedisAddr(t)
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Ping(context.Background()).Err())
	return client
}

func TestRedisManager_CreateGetDeleteRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rdb := newRedisClient(t)

	mgr := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client:        rdb,
		TTLDefault:    14 * 24 * time.Hour,
		TTLRememberMe: 30 * 24 * time.Hour,
		RefreshAfter:  24 * time.Hour,
	})

	uid := uuid.New()
	created, err := mgr.Create(ctx, sessionauth.CreateParams{
		UserID:    uid,
		UserAgent: "go-test",
		IP:        "127.0.0.1",
	})
	require.NoError(t, err)
	assert.Len(t, created.ID, 64)
	assert.Len(t, created.CSRFToken, 64)
	assert.Equal(t, uid, created.UserID)

	got, err := mgr.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, uid, got.UserID)
	assert.Equal(t, created.CSRFToken, got.CSRFToken)

	require.NoError(t, mgr.Delete(ctx, created.ID))

	_, err = mgr.Get(ctx, created.ID)
	require.ErrorIs(t, err, sessionauth.ErrNotFound)
}

func TestRedisManager_DeleteAllForUser_RemovesEverySessionInIndex(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rdb := newRedisClient(t)
	mgr := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client:     rdb,
		TTLDefault: time.Hour,
	})

	uid := uuid.New()
	a, err := mgr.Create(ctx, sessionauth.CreateParams{UserID: uid})
	require.NoError(t, err)
	b, err := mgr.Create(ctx, sessionauth.CreateParams{UserID: uid})
	require.NoError(t, err)

	require.NoError(t, mgr.DeleteAllForUser(ctx, uid))

	_, err = mgr.Get(ctx, a.ID)
	require.ErrorIs(t, err, sessionauth.ErrNotFound)
	_, err = mgr.Get(ctx, b.ID)
	require.ErrorIs(t, err, sessionauth.ErrNotFound)
}

func TestRedisManager_DeleteAllForUserExcept_KeepsOne(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rdb := newRedisClient(t)
	mgr := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client:     rdb,
		TTLDefault: time.Hour,
	})

	uid := uuid.New()
	keep, err := mgr.Create(ctx, sessionauth.CreateParams{UserID: uid})
	require.NoError(t, err)
	other, err := mgr.Create(ctx, sessionauth.CreateParams{UserID: uid})
	require.NoError(t, err)

	require.NoError(t, mgr.DeleteAllForUserExcept(ctx, uid, keep.ID))

	got, err := mgr.Get(ctx, keep.ID)
	require.NoError(t, err)
	assert.Equal(t, uid, got.UserID)

	_, err = mgr.Get(ctx, other.ID)
	require.ErrorIs(t, err, sessionauth.ErrNotFound)
}

func TestRedisManager_Refresh_UpdatesLastActivityAndTTL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rdb := newRedisClient(t)
	mgr := sessionauth.NewRedisManager(sessionauth.RedisOptions{
		Client:       rdb,
		TTLDefault:   time.Hour,
		RefreshAfter: time.Millisecond,
	})

	uid := uuid.New()
	s, err := mgr.Create(ctx, sessionauth.CreateParams{UserID: uid})
	require.NoError(t, err)
	original := s.LastActivityAt

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, mgr.Refresh(ctx, s.ID))

	again, err := mgr.Get(ctx, s.ID)
	require.NoError(t, err)
	assert.True(t, again.LastActivityAt.After(original),
		"expected LastActivityAt to advance, got %v <= %v", again.LastActivityAt, original)
}
