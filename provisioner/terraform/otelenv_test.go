package terraform // nolint:testpackage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type testIDGenerator struct{}

var _ sdktrace.IDGenerator = (*testIDGenerator)(nil)

func (testIDGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	traceID, _ := trace.TraceIDFromHex("60d19e9e9abf2197c1d6d8f93e28ee2a")
	spanID, _ := trace.SpanIDFromHex("a028bd951229a46f")
	return traceID, spanID
}

func (testIDGenerator) NewSpanID(ctx context.Context, traceID trace.TraceID) trace.SpanID {
	spanID, _ := trace.SpanIDFromHex("a028bd951229a46f")
	return spanID
}

func TestOtelEnvInject(t *testing.T) {
	t.Parallel()
	testTraceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithIDGenerator(testIDGenerator{}),
	)

	tracer := testTraceProvider.Tracer("example")
	ctx, span := tracer.Start(context.Background(), "testing")
	defer span.End()

	input := []string{"PATH=/usr/bin:/bin"}

	otel.SetTextMapPropagator(propagation.TraceContext{})
	got := otelEnvInject(ctx, input)
	require.Equal(t, []string{
		"PATH=/usr/bin:/bin",
		"TRACEPARENT=00-60d19e9e9abf2197c1d6d8f93e28ee2a-a028bd951229a46f-01",
	}, got)

	// verify we update rather than append
	input = []string{
		"PATH=/usr/bin:/bin",
		"TRACEPARENT=origTraceParent",
		"TERM=xterm",
	}

	otel.SetTextMapPropagator(propagation.TraceContext{})
	got = otelEnvInject(ctx, input)
	require.Equal(t, []string{
		"PATH=/usr/bin:/bin",
		"TRACEPARENT=00-60d19e9e9abf2197c1d6d8f93e28ee2a-a028bd951229a46f-01",
		"TERM=xterm",
	}, got)
}
