package tracing

import (
	"context"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var NoopSpan trace.Span

func init() {
	tracer := trace.NewNoopTracerProvider().Tracer("")
	_, NoopSpan = tracer.Start(context.Background(), "")
}

const TracerName = "coderd"

func FuncName() string {
	return FuncNameSkip(1)
}

func FuncNameSkip(skip int) string {
	fnpc, _, _, ok := runtime.Caller(1 + skip)
	if !ok {
		return ""
	}
	fn := runtime.FuncForPC(fnpc)
	name := fn.Name()
	if i := strings.LastIndex(name, "/"); i > 0 {
		name = name[i+1:]
	}
	return name
}

// RunWithoutSpan runs the given function with the span stripped from the
// context and replaced with a no-op span. This is useful for avoiding logs
// being added to span (to save money).
func RunWithoutSpan(ctx context.Context, fn func(ctx context.Context)) {
	ctx = trace.ContextWithSpan(ctx, NoopSpan)
	fn(ctx)
}

func MetadataFromContext(ctx context.Context) map[string]string {
	metadata := make(map[string]string)
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(metadata))
	return metadata
}

func MetadataToContext(ctx context.Context, metadata map[string]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(metadata))
}
