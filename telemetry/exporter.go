package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"golang.org/x/xerrors"
)

// Exporter creates a grpc otlp exporter and sets it as the global trace provider.
// Caller is responsible for closing exporter to ensure all data is flushed.
func Exporter(ctx context.Context, service string) (func(), error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String(service),
		),
	)

	otlptracegrpc.NewClient()
	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient())
	if err != nil {
		return nil, xerrors.Errorf("creating otlp exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)

	return func() {
		_ = tracerProvider.Shutdown(ctx)
	}, nil
}
