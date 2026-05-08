package config_test

import (
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ParsesAllFieldsFromEnv(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_PORT", "9090")
	t.Setenv("APP_LOG_LEVEL", "debug")
	t.Setenv("APP_SHUTDOWN_TIMEOUT", "15s")
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/db?sslmode=disable")
	t.Setenv("DATABASE_MAX_OPEN_CONNS", "30")
	t.Setenv("DATABASE_MAX_IDLE_CONNS", "10")
	t.Setenv("DATABASE_CONN_MAX_LIFETIME", "1h")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("ADMIN_API_TOKEN", "abc123")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://a.com,http://b.com")
	t.Setenv("OTEL_SERVICE_NAME", "test-svc")
	t.Setenv("OTEL_TRACES_SAMPLER_RATIO", "0.5")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, "test", cfg.App.Env)
	assert.Equal(t, 9090, cfg.App.Port)
	assert.Equal(t, "debug", cfg.App.LogLevel)
	assert.Equal(t, 15*time.Second, cfg.App.ShutdownTimeout)
	assert.Equal(t, "postgres://u:p@h:5432/db?sslmode=disable", cfg.Database.URL)
	assert.Equal(t, 30, cfg.Database.MaxOpenConns)
	assert.Equal(t, 10, cfg.Database.MaxIdleConns)
	assert.Equal(t, time.Hour, cfg.Database.ConnMaxLifetime)
	assert.Equal(t, "redis:6379", cfg.Redis.Addr)
	assert.Equal(t, "secret", cfg.Redis.Password)
	assert.Equal(t, 2, cfg.Redis.DB)
	assert.Equal(t, "abc123", cfg.Admin.APIToken)
	assert.Equal(t, []string{"http://a.com", "http://b.com"}, cfg.CORS.AllowedOrigins)
	assert.Equal(t, "test-svc", cfg.Observability.OTELServiceName)
	assert.InDelta(t, 0.5, cfg.Observability.OTELTracesSamplerRatio, 0.0001)
}

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_API_TOKEN", "x")

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoad_RequiresAdminAPIToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ADMIN_API_TOKEN", "")

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ADMIN_API_TOKEN")
}

func TestLoad_AppliesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ADMIN_API_TOKEN", "x")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, "development", cfg.App.Env)
	assert.Equal(t, 8080, cfg.App.Port)
	assert.Equal(t, "info", cfg.App.LogLevel)
	assert.Equal(t, 30*time.Second, cfg.App.ShutdownTimeout)
	assert.Equal(t, 25, cfg.Database.MaxOpenConns)
}
