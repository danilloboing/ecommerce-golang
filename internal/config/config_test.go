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
	t.Setenv("STORAGE_ENDPOINT", "http://minio:9000")
	t.Setenv("STORAGE_ACCESS_KEY_ID", "ak")
	t.Setenv("STORAGE_SECRET_ACCESS_KEY", "sk")
	t.Setenv("STORAGE_BUCKET", "marketplace")
	t.Setenv("STORAGE_PUBLIC_BASE_URL", "https://cdn.example/marketplace")
	setEmailEnv(t)

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
	assert.Equal(t, "http://minio:9000", cfg.Storage.Endpoint)
	assert.Equal(t, "marketplace", cfg.Storage.Bucket)
	assert.Equal(t, "https://cdn.example/marketplace", cfg.Storage.PublicBaseURL)
	assert.True(t, cfg.Storage.UsePathStyle)
	assert.Equal(t, "auto", cfg.Storage.Region)
}

func setStorageEnv(t *testing.T) {
	t.Helper()
	t.Setenv("STORAGE_ENDPOINT", "http://minio:9000")
	t.Setenv("STORAGE_ACCESS_KEY_ID", "ak")
	t.Setenv("STORAGE_SECRET_ACCESS_KEY", "sk")
	t.Setenv("STORAGE_BUCKET", "marketplace")
	t.Setenv("STORAGE_PUBLIC_BASE_URL", "https://cdn.example/marketplace")
}

func setEmailEnv(t *testing.T) {
	t.Helper()
	t.Setenv("EMAIL_VERIFY_LINK_BASE_URL", "https://app.example/verify")
	t.Setenv("EMAIL_RESET_LINK_BASE_URL", "https://app.example/reset")
}

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ADMIN_API_TOKEN", "x")
	setStorageEnv(t)

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoad_RequiresAdminAPIToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ADMIN_API_TOKEN", "")
	setStorageEnv(t)

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ADMIN_API_TOKEN")
}

func TestLoad_RequiresStorageBucket(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ADMIN_API_TOKEN", "x")
	setStorageEnv(t)
	t.Setenv("STORAGE_BUCKET", "")

	_, err := config.Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "STORAGE_BUCKET")
}

func TestLoad_AppliesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("ADMIN_API_TOKEN", "x")
	setStorageEnv(t)
	setEmailEnv(t)

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, "development", cfg.App.Env)
	assert.Equal(t, 8080, cfg.App.Port)
	assert.Equal(t, "info", cfg.App.LogLevel)
	assert.Equal(t, 30*time.Second, cfg.App.ShutdownTimeout)
	assert.Equal(t, 25, cfg.Database.MaxOpenConns)
}

func TestLoad_PopulatesEmailSessionsCSRFAndRateLimit(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ADMIN_API_TOKEN", "abc")
	t.Setenv("STORAGE_ENDPOINT", "http://localhost:9000")
	t.Setenv("STORAGE_ACCESS_KEY_ID", "k")
	t.Setenv("STORAGE_SECRET_ACCESS_KEY", "s")
	t.Setenv("STORAGE_BUCKET", "b")
	t.Setenv("STORAGE_PUBLIC_BASE_URL", "http://localhost:9000/b")

	t.Setenv("EMAIL_PROVIDER", "log")
	t.Setenv("EMAIL_FROM_ADDRESS", "no-reply@example.com")
	t.Setenv("EMAIL_FROM_NAME", "Loja")
	t.Setenv("EMAIL_VERIFY_LINK_BASE_URL", "https://app.example/verify")
	t.Setenv("EMAIL_RESET_LINK_BASE_URL", "https://app.example/reset")

	t.Setenv("SESSION_TTL_DEFAULT", "336h")
	t.Setenv("SESSION_TTL_REMEMBER_ME", "720h")
	t.Setenv("SESSION_REFRESH_AFTER", "24h")

	t.Setenv("CSRF_ALLOWED_ORIGINS", "http://localhost:3000,https://app.example")
	t.Setenv("RATELIMIT_TRUSTED_PROXIES", "10.0.0.0/8")
	t.Setenv("COOKIES_SECURE_PREFIX", "false")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "log", cfg.Email.Provider)
	assert.Equal(t, "no-reply@example.com", cfg.Email.FromAddress)
	assert.Equal(t, "Loja", cfg.Email.FromName)
	assert.Equal(t, "https://app.example/verify", cfg.Email.VerifyLinkBaseURL)
	assert.Equal(t, "https://app.example/reset", cfg.Email.ResetLinkBaseURL)

	assert.Equal(t, 336*time.Hour, cfg.Session.TTLDefault)
	assert.Equal(t, 720*time.Hour, cfg.Session.TTLRememberMe)
	assert.Equal(t, 24*time.Hour, cfg.Session.RefreshAfter)

	assert.Equal(t, []string{"http://localhost:3000", "https://app.example"}, cfg.CSRF.AllowedOrigins)
	assert.Equal(t, []string{"10.0.0.0/8"}, cfg.RateLimit.TrustedProxies)
	assert.False(t, cfg.Cookies.SecurePrefix)
}
