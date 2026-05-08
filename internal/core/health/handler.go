// Package health provides liveness and readiness HTTP handlers.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Checker reports component health.
type Checker interface {
	Check(ctx context.Context) error
}

// CheckerFunc adapts a function to Checker.
type CheckerFunc func(ctx context.Context) error

// Check implements Checker.
func (f CheckerFunc) Check(ctx context.Context) error { return f(ctx) }

// Handler exposes /health (liveness) and /ready (readiness).
type Handler struct {
	checkers map[string]Checker
	timeout  time.Duration
}

const defaultReadinessTimeout = 2 * time.Second

// NewHandler builds a handler with the default readiness timeout.
func NewHandler(checkers map[string]Checker) *Handler {
	return NewHandlerWithTimeout(checkers, defaultReadinessTimeout)
}

// NewHandlerWithTimeout overrides the per-checker timeout.
func NewHandlerWithTimeout(checkers map[string]Checker, timeout time.Duration) *Handler {
	return &Handler{checkers: checkers, timeout: timeout}
}

// Liveness always returns 200; the process is up.
func (h *Handler) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readiness verifies all registered components and returns 503 on any failure.
func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	results := make(map[string]string, len(h.checkers))
	healthy := true

	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, checker := range h.checkers {
		wg.Add(1)
		go func(name string, checker Checker) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
			defer cancel()

			if err := checker.Check(ctx); err != nil {
				mu.Lock()
				results[name] = err.Error()
				healthy = false
				mu.Unlock()
				return
			}

			mu.Lock()
			results[name] = "ok"
			mu.Unlock()
		}(name, checker)
	}
	wg.Wait()

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, results)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
