//go:build integration

package redis_test

import (
	"context"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/config"
	internalredis "github.com/danilloboing/marketplace-golang/internal/platform/redis"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_ConnectsAndPings(t *testing.T) {
	addr := testutil.NewTestRedisAddr(t)

	cfg := config.Redis{Addr: addr}

	client, err := internalredis.NewClient(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Ping(context.Background()).Err())

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())
	got, err := client.Get(context.Background(), "k").Result()
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}
