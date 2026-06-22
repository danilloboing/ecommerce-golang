package transport_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/infrastructure"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/transport"
)

type fakeApplier struct{ called bool; ev domain.Event }

func (f *fakeApplier) Apply(_ context.Context, ev domain.Event) error { f.called = true; f.ev = ev; return nil }

func TestWebhook_SignatureGate(t *testing.T) {
	provider := infrastructure.NewMockProvider("secret")
	applier := &fakeApplier{}
	h := transport.NewWebhookHandler(provider, applier)
	r := chi.NewRouter()
	h.RegisterWebhookRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := []byte(`{"id":"evt_1","type":"paid","provider_charge_id":"mock_x","amount_cents":100}`)
	good := infrastructure.Sign("secret", body)

	// bad signature → 401, applier not called
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/payments/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webhook-Signature", "bad")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
	assert.False(t, applier.called)

	// good signature → 200, applier called
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/payments/webhook", bytes.NewReader(body))
	req.Header.Set("X-Webhook-Signature", good)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	assert.True(t, applier.called)
	assert.Equal(t, "paid", applier.ev.Type)

	// missing signature → 401, applier not called
	applier.called = false
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/payments/webhook", bytes.NewReader(body))
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
	assert.False(t, applier.called)
}
