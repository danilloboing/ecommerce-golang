package infrastructure_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/payment/infrastructure"
)

func TestMockProvider_CreateCharge_Pending(t *testing.T) {
	p := infrastructure.NewMockProvider("secret")
	ch, err := p.CreateCharge(context.Background(), application.ChargeRequest{OrderID: uuid.New(), AmountCents: 19800, Method: "pix"})
	require.NoError(t, err)
	assert.Equal(t, domain.ChargePending, ch.Status)
	assert.NotEmpty(t, ch.ProviderChargeID)
	assert.Equal(t, int64(19800), ch.AmountCents)
}

func TestMockProvider_VerifyWebhook_GoodAndBadSignature(t *testing.T) {
	p := infrastructure.NewMockProvider("secret")
	body := []byte(`{"id":"evt_1","type":"paid","provider_charge_id":"mock_x","amount_cents":19800}`)
	sig := infrastructure.Sign("secret", body)

	ev, err := p.VerifyWebhook(body, sig)
	require.NoError(t, err)
	assert.Equal(t, "paid", ev.Type)
	assert.Equal(t, int64(19800), ev.AmountCents)

	_, err = p.VerifyWebhook(body, "deadbeef")
	require.ErrorIs(t, err, domain.ErrInvalidSignature)
}
