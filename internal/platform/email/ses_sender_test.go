package email_test

import (
	"context"
	"errors"
	"testing"
	"time"

	awssesv2 "github.com/aws/aws-sdk-go-v2/service/sesv2"
	awssesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSESAPI struct {
	called bool
	in     *awssesv2.SendEmailInput
	out    *awssesv2.SendEmailOutput
	err    error
}

func (f *fakeSESAPI) SendEmail(ctx context.Context, in *awssesv2.SendEmailInput, _ ...func(*awssesv2.Options)) (*awssesv2.SendEmailOutput, error) {
	f.called = true
	f.in = in
	if f.err != nil {
		return nil, f.err
	}
	if f.out != nil {
		return f.out, nil
	}
	return &awssesv2.SendEmailOutput{}, nil
}

func TestSESSender_Send_BuildsExpectedRequest(t *testing.T) {
	api := &fakeSESAPI{}
	sender := email.NewSESSenderWithAPI(api, "Loja <no-reply@example.com>", "marketing")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := sender.Send(ctx, email.Message{
		To:       []string{"ana@example.com"},
		Subject:  "olá",
		TextBody: "texto",
		HTMLBody: "<p>html</p>",
		Tags:     map[string]string{"category": "verify_email"},
	})
	require.NoError(t, err)
	require.True(t, api.called)

	require.NotNil(t, api.in.FromEmailAddress)
	assert.Equal(t, "Loja <no-reply@example.com>", *api.in.FromEmailAddress)
	require.NotNil(t, api.in.Destination)
	assert.Equal(t, []string{"ana@example.com"}, api.in.Destination.ToAddresses)

	require.NotNil(t, api.in.Content)
	require.NotNil(t, api.in.Content.Simple)
	require.NotNil(t, api.in.Content.Simple.Subject)
	assert.Equal(t, "olá", *api.in.Content.Simple.Subject.Data)
	require.NotNil(t, api.in.Content.Simple.Body.Text)
	assert.Equal(t, "texto", *api.in.Content.Simple.Body.Text.Data)
	require.NotNil(t, api.in.Content.Simple.Body.Html)
	assert.Equal(t, "<p>html</p>", *api.in.Content.Simple.Body.Html.Data)

	require.NotNil(t, api.in.ConfigurationSetName)
	assert.Equal(t, "marketing", *api.in.ConfigurationSetName)

	require.Len(t, api.in.EmailTags, 1)
	assert.Equal(t, awssesv2types.MessageTag{Name: ptr("category"), Value: ptr("verify_email")}, api.in.EmailTags[0])
}

func TestSESSender_Send_PropagatesError(t *testing.T) {
	api := &fakeSESAPI{err: errors.New("ses unavailable")}
	sender := email.NewSESSenderWithAPI(api, "Loja <no-reply@example.com>", "")

	err := sender.Send(context.Background(), email.Message{
		To:       []string{"ana@example.com"},
		Subject:  "x",
		TextBody: "y",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ses unavailable")
}

func ptr[T any](v T) *T { return &v }
