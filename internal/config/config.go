// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config aggregates all runtime configuration sections.
type Config struct {
	App           App
	Database      Database
	Redis         Redis
	Admin         Admin
	CORS          CORS
	Observability Observability
	Storage       Storage
}

// App holds general application settings.
type App struct {
	Env             string        `env:"APP_ENV" envDefault:"development"`
	Port            int           `env:"APP_PORT" envDefault:"8080"`
	LogLevel        string        `env:"APP_LOG_LEVEL" envDefault:"info"`
	ShutdownTimeout time.Duration `env:"APP_SHUTDOWN_TIMEOUT" envDefault:"30s"`
}

// Database holds Postgres connection settings.
type Database struct {
	URL             string        `env:"DATABASE_URL,required,notEmpty"`
	MaxOpenConns    int           `env:"DATABASE_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"DATABASE_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"DATABASE_CONN_MAX_LIFETIME" envDefault:"30m"`
}

// Redis holds Redis connection settings.
type Redis struct {
	Addr     string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_DB" envDefault:"0"`
}

// Admin holds bootstrap admin auth (Phase 1 only).
type Admin struct {
	APIToken string `env:"ADMIN_API_TOKEN,required,notEmpty"`
}

// CORS holds allowed origins.
type CORS struct {
	AllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`
}

// Observability holds tracing and error reporting endpoints.
type Observability struct {
	SentryDSN              string  `env:"SENTRY_DSN"`
	OTELExporterEndpoint   string  `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTELServiceName        string  `env:"OTEL_SERVICE_NAME" envDefault:"marketplace-golang-api"`
	OTELTracesSamplerRatio float64 `env:"OTEL_TRACES_SAMPLER_RATIO" envDefault:"0.1"`
}

// Storage holds object storage settings (R2 / S3 compatible).
type Storage struct {
	Endpoint        string `env:"STORAGE_ENDPOINT,required,notEmpty"`
	AccessKeyID     string `env:"STORAGE_ACCESS_KEY_ID,required,notEmpty"`
	SecretAccessKey string `env:"STORAGE_SECRET_ACCESS_KEY,required,notEmpty"`
	Bucket          string `env:"STORAGE_BUCKET,required,notEmpty"`
	Region          string `env:"STORAGE_REGION" envDefault:"auto"`
	PublicBaseURL   string `env:"STORAGE_PUBLIC_BASE_URL,required,notEmpty"`
	UsePathStyle    bool   `env:"STORAGE_USE_PATH_STYLE" envDefault:"true"`
}

// Load parses configuration from environment variables.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse env: %w", err)
	}
	return cfg, nil
}
