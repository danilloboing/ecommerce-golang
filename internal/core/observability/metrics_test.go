package observability_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetrics_HandlerReturnsRegisteredCollectors(t *testing.T) {
	m := observability.NewMetrics()

	m.HTTPRequests.WithLabelValues("GET", "/x", "200").Inc()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "marketplace_http_requests_total")
	assert.Contains(t, body, `method="GET"`)
	assert.Contains(t, body, "go_goroutines")
	assert.Contains(t, body, "process_cpu_seconds_total")
}

func TestNewMetrics_DurationHistogramRegistered(t *testing.T) {
	m := observability.NewMetrics()

	m.HTTPDurationSeconds.WithLabelValues("GET", "/x", "200").Observe(0.123)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "marketplace_http_request_duration_seconds")
}
