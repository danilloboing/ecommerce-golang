// Package responsex centralises HTTP error responses across modules.
package responsex

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danilloboing/marketplace-golang/internal/core/observability"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
)

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// WriteError maps domain or application errors to JSON HTTP responses.
// Unknown errors are rendered as 500 to avoid leaking internals.
func WriteError(w http.ResponseWriter, err error) {
	status, code := classify(err)

	body := errorBody{}
	body.Error.Code = code
	body.Error.Message = err.Error()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Error writes a JSON error body with explicit status, code, and user-facing message.
// Caller is responsible for choosing status/code; this helper does not classify.
// Internal err details (when present) are logged via slog at warn (4xx) or error (5xx).
func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	body := errorBody{}
	body.Error.Code = code
	body.Error.Message = message

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ErrorWithCause is Error plus structured logging of an internal error chain.
// Use when transport mapped a domain error and wants the original cause logged
// without leaking it in the response body.
func ErrorWithCause(w http.ResponseWriter, r *http.Request, status int, code, message string, cause error) {
	logger := observability.FromContext(r.Context())
	attrs := []any{
		slog.Int("status", status),
		slog.String("code", code),
		slog.String("path", r.URL.Path),
		slog.String("method", r.Method),
	}
	if cause != nil {
		attrs = append(attrs, slog.String("error", cause.Error()))
	}
	switch {
	case status >= 500:
		logger.Error("request_failed", attrs...)
	case status >= 400:
		logger.Warn("request_rejected", attrs...)
	}

	Error(w, r, status, code, message)
}

// JSON writes status + arbitrary payload as JSON.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func classify(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrInvalidProduct),
		errors.Is(err, domain.ErrInvalidCategory),
		errors.Is(err, domain.ErrInvalidCurrency),
		errors.Is(err, domain.ErrNegativeAmount),
		errors.Is(err, domain.ErrInvalidSlug),
		errors.Is(err, domain.ErrDuplicateSKU),
		errors.Is(err, domain.ErrCurrencyMismatch),
		errors.Is(err, application.ErrBlankSearchQuery):
		return http.StatusBadRequest, "invalid_request"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}
