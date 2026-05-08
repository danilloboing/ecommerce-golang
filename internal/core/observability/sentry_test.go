package observability_test

import (
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupSentry_NoDSNIsNoop(t *testing.T) {
	flush, err := observability.SetupSentry(observability.SentryOptions{
		Service: "x",
		Env:     "test",
	})
	require.NoError(t, err)
	require.NotNil(t, flush)

	flush()
}

func TestSetupSentry_InvalidDSNReturnsError(t *testing.T) {
	_, err := observability.SetupSentry(observability.SentryOptions{
		DSN:     "not-a-dsn",
		Service: "x",
		Env:     "test",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sentry")
}
