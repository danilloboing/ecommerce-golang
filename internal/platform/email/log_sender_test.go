package email_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogSender_Send_LogsStructured(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	sender := email.NewLogSender(logger)
	err := sender.Send(context.Background(), email.Message{
		To:       []string{"ana@example.com"},
		Subject:  "verifique seu email",
		TextBody: "Hello Ana, click https://app.example/verify?token=abc",
	})
	require.NoError(t, err)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry))
	assert.Equal(t, "email_send", entry["msg"])
	assert.Equal(t, "verifique seu email", entry["subject"])
	to, ok := entry["to"].([]any)
	require.True(t, ok)
	assert.Equal(t, "ana@example.com", to[0])
	assert.Contains(t, entry["preview"], "https://app.example/verify?token=abc")
}
