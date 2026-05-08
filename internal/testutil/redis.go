package testutil

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// NewTestRedisAddr spins up a fresh Redis container and returns its host:port.
func NewTestRedisAddr(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	return strings.TrimPrefix(uri, "redis://")
}
