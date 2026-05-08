// Package observability provides structured logging, metrics, tracing,
// and error reporting primitives shared across modules.
package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// LoggerOptions configures a slog.Logger.
type LoggerOptions struct {
	Level     string
	Output    io.Writer
	Service   string
	Env       string
	AddSource bool
}

type loggerKey struct{}

// NewLogger builds a JSON slog.Logger with service/env attributes attached.
func NewLogger(opts LoggerOptions) (*slog.Logger, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}

	out := opts.Output
	if out == nil {
		out = os.Stdout
	}

	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level:     level,
		AddSource: opts.AddSource,
	})

	logger := slog.New(handler).With(
		slog.String("service", opts.Service),
		slog.String("env", opts.Env),
	)
	return logger, nil
}

// WithLogger returns a context carrying the given logger.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// FromContext extracts a logger from context, falling back to slog.Default.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("observability: invalid log level %q", s)
	}
}
