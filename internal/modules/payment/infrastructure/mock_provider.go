// Package infrastructure contains payment provider adapters.
package infrastructure

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
)

// Sign returns hex(HMAC-SHA256(secret, body)). Shared by the mock and tests.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// MockProvider is an in-process PaymentProvider for tests and local development.
type MockProvider struct{ secret string }

var _ application.PaymentProvider = (*MockProvider)(nil)

// NewMockProvider constructs a MockProvider that signs webhooks with secret.
func NewMockProvider(secret string) *MockProvider { return &MockProvider{secret: secret} }

// CreateCharge returns an immediate pending charge without any external call.
func (m *MockProvider) CreateCharge(_ context.Context, req application.ChargeRequest) (domain.Charge, error) {
	return domain.Charge{
		OrderID:          req.OrderID,
		Provider:         "mock",
		ProviderChargeID: "mock_" + req.OrderID.String(),
		Method:           req.Method,
		Status:           domain.ChargePending,
		AmountCents:      req.AmountCents,
	}, nil
}

type mockEvent struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	ProviderChargeID string `json:"provider_charge_id"`
	AmountCents      int64  `json:"amount_cents"`
}

// VerifyWebhook validates the HMAC over the raw body in constant time (C1), then decodes.
func (m *MockProvider) VerifyWebhook(payload []byte, signature string) (domain.Event, error) {
	expected := Sign(m.secret, payload)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return domain.Event{}, domain.ErrInvalidSignature
	}
	var e mockEvent
	if err := json.Unmarshal(payload, &e); err != nil {
		return domain.Event{}, domain.ErrInvalidSignature
	}
	return domain.Event{ID: e.ID, Type: e.Type, ProviderChargeID: e.ProviderChargeID, AmountCents: e.AmountCents}, nil
}
