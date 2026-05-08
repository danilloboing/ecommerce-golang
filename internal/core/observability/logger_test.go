package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger_EmitsJSON(t *testing.T) {
	var buf bytes.Buffer

	logger, err := observability.NewLogger(observability.LoggerOptions{
		Level:     "info",
		Output:    &buf,
		Service:   "test-svc",
		Env:       "test",
		AddSource: false,
	})
	require.NoError(t, err)

	logger.Info("hello world", slog.String("orderID", "abc"))

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))

	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "hello world", entry["msg"])
	assert.Equal(t, "abc", entry["orderID"])
	assert.Equal(t, "test-svc", entry["service"])
	assert.Equal(t, "test", entry["env"])
}

func TestNewLogger_RejectsInvalidLevel(t *testing.T) {
	_, err := observability.NewLogger(observability.LoggerOptions{
		Level:   "shout",
		Output:  &bytes.Buffer{},
		Service: "x",
		Env:     "x",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestNewLogger_RespectsLevel(t *testing.T) {
	var buf bytes.Buffer

	logger, err := observability.NewLogger(observability.LoggerOptions{
		Level:   "warn",
		Output:  &buf,
		Service: "test",
		Env:     "test",
	})
	require.NoError(t, err)

	logger.Info("filtered out")
	assert.Empty(t, buf.String())

	logger.Warn("kept")
	assert.Contains(t, buf.String(), "kept")
}

func TestFromContext_UsesContextLogger(t *testing.T) {
	var buf bytes.Buffer
	logger, err := observability.NewLogger(observability.LoggerOptions{
		Level:   "info",
		Output:  &buf,
		Service: "x",
		Env:     "x",
	})
	require.NoError(t, err)

	ctx := observability.WithLogger(context.Background(), logger)
	got := observability.FromContext(ctx)

	got.Info("from ctx")
	assert.Contains(t, buf.String(), "from ctx")
}
