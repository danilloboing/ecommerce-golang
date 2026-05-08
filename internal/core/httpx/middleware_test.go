package httpx_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestID_GeneratesAndPropagates(t *testing.T) {
	captured := ""

	handler := httpx.RequestID()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = httpx.RequestIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.NotEmpty(t, captured)
	assert.Equal(t, captured, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_RespectsExistingHeader(t *testing.T) {
	captured := ""

	handler := httpx.RequestID()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = httpx.RequestIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "abc-123", captured)
	assert.Equal(t, "abc-123", rec.Header().Get("X-Request-ID"))
}

func TestRequestLogger_LogsMethodPathAndStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := httpx.RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "tea")
	}))

	req := httptest.NewRequest(http.MethodGet, "/teapot", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTeapot, rec.Code)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "GET", entry["method"])
	assert.Equal(t, "/teapot", entry["path"])
	assert.EqualValues(t, http.StatusTeapot, entry["status"])
}

func TestRecover_RecoversAndReturns500(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := httpx.Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() { handler.ServeHTTP(rec, req) })

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, buf.String(), "boom")
}

func TestSecurityHeaders_SetsExpected(t *testing.T) {
	handler := httpx.SecurityHeaders()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.TLS = nil
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.NotEmpty(t, rec.Header().Get("Referrer-Policy"))
}

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	handler := httpx.CORS([]string{"http://allowed.test"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/x", strings.NewReader(""))
	req.Header.Set("Origin", "http://allowed.test")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "http://allowed.test", rec.Header().Get("Access-Control-Allow-Origin"))
}
