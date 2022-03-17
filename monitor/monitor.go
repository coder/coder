package monitor

import (
	"context"
	"os"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"
)

var (
	// DefaultMonitor is returned if no Monitor is set on a context.
	DefaultMonitor = Monitor{
		Logger:         slog.Make(sloghuman.Sink(os.Stderr)),
		TracerProvider: nil,
	}
)

// Monitor handles logging and tracing with context.
type Monitor struct {
	Logger         slog.Logger
	TracerProvider trace.TracerProvider
}

// MonitorOptions are parameters for creating a Monitor.
type MonitorOptions struct {
	Logger         slog.Logger
	TracerProvider trace.TracerProvider
}

type monitorContextKey struct{}

// New creates a Monitor instance.
func New(o MonitorOptions) *Monitor {
	return &Monitor{
		Logger:         o.Logger,
		TracerProvider: o.TracerProvider,
	}
}

// FromContext returns the Monitor saved on the provided context or the DefaultMonitor.
func FromContext(ctx context.Context) *Monitor {
	m, ok := ctx.Value(monitorContextKey{}).(*Monitor)
	if !ok {
		return &DefaultMonitor
	}

	return m
}

// WithMonitor attaches the Monitor to a copy of the provided context.
func WithMonitor(ctx context.Context, m *Monitor) context.Context {
	return context.WithValue(ctx, monitorContextKey{}, m)
}

// LogNamed appends the name to the logger attached to the provided context.
func LogNamed(ctx context.Context, name string) {
	FromContext(ctx).Logger = FromContext(ctx).Logger.Named(name)
}

// Trace starts a new or child tracing span.
//
// Example Usage:
//
// defer monitor.Trace(ctx, "my_operation")()
func Trace(ctx *context.Context, name string) func() {
	newCtx, span := FromContext(*ctx).TracerProvider.Tracer("").Start(*ctx, name)
	*ctx = newCtx
	return func() {
		span.End()
	}
}

// Span returns the current span attached to the given context.
func Span(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// Debug logs the msg and fields at LevelDebug.
func Debug(ctx context.Context, msg string, fields ...slog.Field) {
	FromContext(ctx).Logger.Debug(ctx, msg, fields...)
}

// Info logs the msg and fields at LevelInfo.
func Info(ctx context.Context, msg string, fields ...slog.Field) {
	FromContext(ctx).Logger.Info(ctx, msg, fields...)
}

// Warn logs the msg and fields at LevelWarn.
func Warn(ctx context.Context, msg string, fields ...slog.Field) {
	FromContext(ctx).Logger.Warn(ctx, msg, fields...)
}

// Error logs the msg and fields at LevelError.
func Error(ctx context.Context, msg string, err error) {
	FromContext(ctx).Logger.Error(ctx, msg, slog.Error(err))
	trace.SpanFromContext(ctx).RecordError(xerrors.Errorf("%s: %w", msg, err))
}

// Critical logs the msg and fields at LevelCritical.
func Critical(ctx context.Context, msg string, fields ...slog.Field) {
	FromContext(ctx).Logger.Critical(ctx, msg, fields...)
}
