package email_test

import (
	"strings"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderVerifyEmail_IncludesNameAndLink(t *testing.T) {
	msg, err := email.RenderVerifyEmail(email.VerifyEmailData{
		ToAddress: "ana@example.com",
		Name:      "Ana",
		VerifyURL: "https://app.example/verify?token=abc",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"ana@example.com"}, msg.To)
	assert.Contains(t, msg.Subject, "verifique")
	assert.Contains(t, msg.TextBody, "Ana")
	assert.Contains(t, msg.TextBody, "https://app.example/verify?token=abc")
	assert.Contains(t, msg.HTMLBody, "https://app.example/verify?token=abc")
}

func TestRenderPasswordResetEmail_IncludesLinkAndExpiry(t *testing.T) {
	msg, err := email.RenderPasswordResetEmail(email.PasswordResetEmailData{
		ToAddress: "ana@example.com",
		Name:      "Ana",
		ResetURL:  "https://app.example/reset?token=xyz",
		ExpiryMin: 60,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"ana@example.com"}, msg.To)
	assert.True(t, strings.Contains(msg.Subject, "senha"), "subject should mention password")
	assert.Contains(t, msg.TextBody, "https://app.example/reset?token=xyz")
	assert.Contains(t, msg.TextBody, "60")
}
