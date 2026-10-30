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

// Redacted replaces secrets that RedactURLPath and RedactRouteParam hide.
const Redacted = "REDACTED"

// chatTriggerWebhookPrefix starts the route whose callers authenticate
// with a secret in the path: /chat-triggers/{trigger}/webhook/{secret}.
const chatTriggerWebhookPrefix = "/api/experimental/chat-triggers/"

// RedactURLPath hides secrets that some routes carry in the URL path, so
// logs and traces never record them. Other paths are returned unchanged.
func RedactURLPath(path string) string {
	rest, ok := strings.CutPrefix(path, chatTriggerWebhookPrefix)
	if !ok {
		return path
	}
	trigger, afterTrigger, ok := strings.Cut(rest, "/")
	if !ok {
		return path
	}
	secretAndTail, ok := strings.CutPrefix(afterTrigger, "webhook/")
	if !ok || secretAndTail == "" {
		return path
	}
	redacted := chatTriggerWebhookPrefix + trigger + "/webhook/" + Redacted
	if _, tail, hasTail := strings.Cut(secretAndTail, "/"); hasTail {
		redacted += "/" + tail
	}
	return redacted
}

// RedactRouteParam hides the value of route parameters that hold secrets.
func RedactRouteParam(key, value string) string {
	if key == "secret" {
		return Redacted
	}
	return value
}
