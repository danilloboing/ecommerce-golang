//go:build integration

package ratelimit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/ratelimit"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := testutil.NewTestRedisAddr(t)
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Ping(context.Background()).Err())
	return client
}

func TestMiddleware_AllowsUpToLimitThenBlocks(t *testing.T) {
	rdb := newRedisClient(t)

	mw := ratelimit.Middleware(ratelimit.Options{
		Client: rdb,
		Rules: []ratelimit.Rule{
			{Key: "test:ip", Source: ratelimit.ByIP, Limit: 3, Window: time.Minute},
		},
	})

	called := 0
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))

	for i := 0; i < 3; i++ {
		r := httptest.NewRequest("POST", "/x", nil)
		r.RemoteAddr = "1.2.3.4:80"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		require.Equal(t, http.StatusNoContent, rec.Code, "request %d should pass", i+1)
	}

	r := httptest.NewRequest("POST", "/x", nil)
	r.RemoteAddr = "1.2.3.4:80"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	assert.Equal(t, 3, called)
}
