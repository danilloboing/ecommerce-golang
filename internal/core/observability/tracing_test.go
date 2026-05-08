package observability_test

import (
	"context"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestSetupTracing_NoEndpointReturnsNoop(t *testing.T) {
	shutdown, err := observability.SetupTracing(context.Background(), observability.TracingOptions{
		ServiceName: "x",
		Env:         "test",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "noop-span")
	defer span.End()

	assert.NotNil(t, span)
}

func TestSetupTracing_AcceptsValidEndpoint(t *testing.T) {
	shutdown, err := observability.SetupTracing(context.Background(), observability.TracingOptions{
		ServiceName:  "x",
		Env:          "test",
		Endpoint:     "localhost:4318",
		SamplerRatio: 0.5,
		Insecure:     true,
	})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() { _ = shutdown(context.Background()) })
}
