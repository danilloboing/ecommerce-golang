//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPI_Smoke_HealthReadyMetrics(t *testing.T) {
	dsn := testutil.NewTestPostgresURL(t)
	addr := testutil.NewTestRedisAddr(t)
	minio := testutil.NewTestMinio(t)

	port := "18081"

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bin := testutil.BuildAPIBinary(t)
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(),
		"APP_PORT="+port,
		"APP_ENV=test",
		"APP_LOG_LEVEL=warn",
		"DATABASE_URL="+dsn,
		"REDIS_ADDR="+addr,
		"ADMIN_API_TOKEN=test-token",
		"CORS_ALLOWED_ORIGINS=http://test.local",
		"STORAGE_ENDPOINT="+minio.Endpoint,
		"STORAGE_ACCESS_KEY_ID="+minio.AccessKeyID,
		"STORAGE_SECRET_ACCESS_KEY="+minio.SecretAccessKey,
		"STORAGE_BUCKET=marketplace-test",
		"STORAGE_REGION=us-east-1",
		"STORAGE_PUBLIC_BASE_URL="+minio.Endpoint+"/marketplace-test",
		"STORAGE_USE_PATH_STYLE=true",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	base := "http://127.0.0.1:" + port
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/health")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 15*time.Second, 200*time.Millisecond, "server did not start")

	t.Run("health", func(t *testing.T) {
		resp, err := http.Get(base + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var got map[string]string
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Equal(t, "ok", got["status"])
	})

	t.Run("ready", func(t *testing.T) {
		resp, err := http.Get(base + "/ready")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		s := string(body)
		assert.Contains(t, s, `"postgres":"ok"`)
		assert.Contains(t, s, `"redis":"ok"`)
	})

	t.Run("metrics", func(t *testing.T) {
		resp, err := http.Get(base + "/metrics")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		s := string(body)
		assert.True(t, strings.Contains(s, "marketplace_http_requests_total") || strings.Contains(s, "go_goroutines"),
			"expected Prometheus metrics in body, got: %s", s[:min(200, len(s))])
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
