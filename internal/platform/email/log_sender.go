package email

import (
	"context"
	"log/slog"
)

// previewLen caps how many characters of the text body are echoed into the
// log line so debug output stays readable without dumping full message bodies.
const previewLen = 200

// LogSender writes outbound messages to a structured logger instead of
// delivering them. It is the default backend in development and tests so
// engineers can inspect verification/reset URLs without external services.
type LogSender struct {
	logger *slog.Logger
}

var _ Sender = (*LogSender)(nil)

// NewLogSender returns a LogSender that writes to the provided logger.
// A nil logger falls back to slog.Default() so callers do not need to
// short-circuit construction in tests.
func NewLogSender(logger *slog.Logger) *LogSender {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogSender{logger: logger}
}

// Send logs the message metadata and a truncated text-body preview at INFO.
// It never returns an error; the signature exists to satisfy Sender.
func (s *LogSender) Send(ctx context.Context, msg Message) error {
	s.logger.LogAttrs(ctx, slog.LevelInfo, "email_send",
		slog.Any("to", msg.To),
		slog.String("subject", msg.Subject),
		slog.String("preview", preview(msg.TextBody, previewLen)),
	)
	return nil
}

func preview(body string, n int) string {
	if len(body) <= n {
		return body
	}
	return body[:n] + "..."
}
