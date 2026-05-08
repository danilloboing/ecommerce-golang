// Package testutil provides shared helpers for integration tests.
package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NewTestPostgresURL spins up a fresh Postgres container and returns its DSN.
// The container is automatically terminated at test cleanup.
func NewTestPostgresURL(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("marketplace_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	return dsn
}

// ApplyMigrations runs Atlas against the given DSN to bring the schema up.
// Requires the atlas binary on PATH (or at $GOPATH/bin/atlas).
func ApplyMigrations(t *testing.T, dsn string) {
	t.Helper()
	atlas := atlasBinary()
	root := projectRoot(t)
	cmd := exec.Command(atlas, "migrate", "apply",
		"--dir", "file://db/migrations",
		"--url", dsn,
	)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "atlas migrate apply: %s", out)
}

func atlasBinary() string {
	if path, err := exec.LookPath("atlas"); err == nil {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, "go", "bin", "atlas")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "atlas"
}

func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd || parent == "" {
			t.Fatal("could not locate go.mod root")
		}
		wd = parent
	}
}

// _ keeps strings import used if helpers shrink.
var _ = strings.TrimSpace
