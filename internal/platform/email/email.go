// Package email provides a transport-agnostic abstraction for sending
// transactional emails (account verification, password reset). It exposes
// a Sender interface plus two implementations: LogSender for local
// development and tests, and SESSender for production via AWS SES v2.
package email

import (
	"context"
	"errors"
	"log/slog"
)

// Message is a single outbound email built from one of the rendered templates.
// HTMLBody may be empty for text-only messages; TextBody is required.
type Message struct {
	To       []string
	Subject  string
	HTMLBody string
	TextBody string
	Tags     map[string]string
}

// Sender delivers a Message via some transport (log, AWS SES, etc.).
// Implementations must be safe for concurrent use.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Config selects and configures the email backend at startup.
// Provider values: "" or "log" use LogSender; "ses" uses SESSender.
type Config struct {
	Provider            string
	FromAddress         string
	FromName            string
	SESRegion           string
	SESConfigurationSet string
}

// ErrUnknownProvider indicates Config.Provider did not match a known backend.
var ErrUnknownProvider = errors.New("email: unknown provider")

// NewSenderFromConfig builds the Sender selected by cfg.Provider.
// "" and "log" return a LogSender; "ses" returns a SESSender. Any other
// value yields ErrUnknownProvider.
func NewSenderFromConfig(cfg Config, logger *slog.Logger) (Sender, error) {
	switch cfg.Provider {
	case "", "log":
		return NewLogSender(logger), nil
	case "ses":
		return NewSESSender(SESConfig{
			Region:           cfg.SESRegion,
			FromAddress:      cfg.FromAddress,
			FromName:         cfg.FromName,
			ConfigurationSet: cfg.SESConfigurationSet,
		})
	default:
		return nil, ErrUnknownProvider
	}
}
