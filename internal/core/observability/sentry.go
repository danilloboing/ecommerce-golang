package observability

import (
	"fmt"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

// SentryOptions configures Sentry initialization.
type SentryOptions struct {
	DSN              string
	Service          string
	Env              string
	Release          string
	TracesSampleRate float64
}

// FlushFunc waits for in-flight events to be delivered.
type FlushFunc func()

// SetupSentry initializes Sentry. With an empty DSN it returns a no-op flusher.
func SetupSentry(opts SentryOptions) (FlushFunc, error) {
	if opts.DSN == "" {
		return func() {}, nil
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              opts.DSN,
		Environment:      opts.Env,
		Release:          opts.Release,
		ServerName:       opts.Service,
		AttachStacktrace: true,
		EnableTracing:    opts.TracesSampleRate > 0,
		TracesSampleRate: opts.TracesSampleRate,
		BeforeSend:       scrubPII,
	})
	if err != nil {
		return nil, fmt.Errorf("sentry: init: %w", err)
	}

	return func() { sentry.Flush(2 * time.Second) }, nil
}

func scrubPII(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event.Request == nil {
		return event
	}

	for _, key := range []string{"Authorization", "Cookie", "Set-Cookie"} {
		delete(event.Request.Headers, key)
	}

	if event.Request.Data != "" && (strings.Contains(strings.ToLower(event.Request.Data), "password") ||
		strings.Contains(strings.ToLower(event.Request.Data), "cpf") ||
		strings.Contains(strings.ToLower(event.Request.Data), "token")) {
		event.Request.Data = "[REDACTED]"
	}

	return event
}
