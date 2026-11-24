//go:build slim

package loggermw

import (
	"context"
	"time"

	"cdr.dev/slog/v3"
)

type RequestLogger interface {
	WithFields(fields ...slog.Field)
	WriteLog(ctx context.Context, status int)
}

var _ RequestLogger = &SlogRequestLogger{}

// NewRequestLogger returns a logger for a request. The path is logged by WriteLog,
// not before, so that secrets in route params can be masked once they are known.
func NewRequestLogger(log slog.Logger, message, path string, start time.Time) RequestLogger {
	return &SlogRequestLogger{
		log:     log,
		written: false,
		message: message,
		path:    path,
		start:   start,
	}
}
