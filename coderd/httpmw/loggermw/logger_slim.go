//go:build slim

package loggermw

import (
	"context"
	"time"

	"cdr.dev/slog"
)

type RequestLogger interface {
	WithFields(fields ...slog.Field)
	WriteLog(ctx context.Context, status int)
}

var _ RequestLogger = &SlogRequestLogger{}

func NewRequestLogger(log slog.Logger, message string, start time.Time) RequestLogger {
	return &SlogRequestLogger{
		log:     log,
		written: false,
		message: message,
		start:   start,
	}
}
