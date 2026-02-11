package catalog

import (
	"context"
	"sync/atomic"

	"cdr.dev/slog/v3"
)

type loggerSink struct {
	logger slog.Logger
	done   atomic.Bool
}

func (l *loggerSink) LogEntry(ctx context.Context, e slog.SinkEntry) {
	if l.done.Load() {
		return
	}
	l.logger.Log(ctx, e)
}

func (l *loggerSink) Sync() {
	l.logger.Sync()
}

func (l *loggerSink) Close() {
	l.done.Store(true)
}

func controllableLoggerSink(logger slog.Logger) *loggerSink {
	return &loggerSink{logger: logger}
}
