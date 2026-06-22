// Package transport contains the HTTP transport layer for the payment module.
package transport

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/application"
)

// WebhookHandler handles inbound payment provider webhook notifications.
type WebhookHandler struct {
	provider application.PaymentProvider
	applier  application.EventApplier
}

// NewWebhookHandler constructs a WebhookHandler with the given provider and event applier.
func NewWebhookHandler(provider application.PaymentProvider, applier application.EventApplier) *WebhookHandler {
	return &WebhookHandler{provider: provider, applier: applier}
}

// Provider returns the underlying PaymentProvider (used by module.SetApplier for re-wiring).
func (h *WebhookHandler) Provider() application.PaymentProvider { return h.provider }

// RegisterWebhookRoutes mounts the webhook endpoint onto the router.
func (h *WebhookHandler) RegisterWebhookRoutes(r chi.Router) {
	r.Post("/payments/webhook", h.handleWebhook)
}

func (h *WebhookHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// C1: read raw body BEFORE any parsing — signature is over raw bytes.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"read_error"}`, http.StatusInternalServerError)
		return
	}

	sig := r.Header.Get("X-Webhook-Signature")
	ev, err := h.provider.VerifyWebhook(body, sig)
	if err != nil {
		writeJSON(w, statusForError(err), map[string]string{"error": "invalid_signature"})
		return
	}

	if h.applier == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	if err := h.applier.Apply(r.Context(), ev); err != nil {
		slog.Error("webhook apply failed", "error", err.Error())
		http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
