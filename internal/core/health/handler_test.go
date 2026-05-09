package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeChecker struct{ err error }

func (f fakeChecker) Check(_ context.Context) error { return f.err }

func TestLiveness_AlwaysReturns200(t *testing.T) {
	h := health.NewHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.Liveness(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadiness_AllHealthyReturns200(t *testing.T) {
	checks := map[string]health.Checker{
		"postgres": fakeChecker{},
		"redis":    fakeChecker{},
	}
	h := health.NewHandler(checks)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	h.Readiness(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"postgres":"ok"`)
	assert.Contains(t, rec.Body.String(), `"redis":"ok"`)
}

func TestReadiness_AnyUnhealthyReturns503(t *testing.T) {
	checks := map[string]health.Checker{
		"postgres": fakeChecker{},
		"redis":    fakeChecker{err: errors.New("conn refused")},
	}
	h := health.NewHandler(checks)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	h.Readiness(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), `"redis":"conn refused"`)
}

func TestReadiness_RespectsTimeout(t *testing.T) {
	slow := slowChecker{}
	h := health.NewHandlerWithTimeout(map[string]health.Checker{"slow": slow}, 1)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	h.Readiness(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

type slowChecker struct{}

func (slowChecker) Check(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestReadiness_NoCheckersReturns200(t *testing.T) {
	h := health.NewHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	h.Readiness(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}
