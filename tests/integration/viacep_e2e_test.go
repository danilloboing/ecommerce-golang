//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViaCEPE2E_LookupAndCache(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)

	for i := 0; i < 2; i++ {
		resp, err := http.Get(env.srv.URL + "/address/cep/01001000")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	}

	assert.Equal(t, int64(1), env.viacepHit(), "second lookup must be served from Redis cache")
}
