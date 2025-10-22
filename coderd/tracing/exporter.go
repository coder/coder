//go:build !slim

package tracing

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	ddprofiler "gopkg.in/DataDog/dd-trace-go.v1/profiler"

	"golang.org/x/xerrors"
	"google.golang.org/grpc/credentials"
)

// TracerOpts specifies which telemetry exporters should be configured.
type TracerOpts struct {
	// Default exports to a backend configured by environment variables. See:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md
	Default bool
	// DataDog exports traces and profiles to the local DataDog daemon.
	DataDog bool
	// Exports traces to Honeycomb.io with the provided API key.
	Honeycomb string
}

// TracerProvider creates a grpc otlp exporter and configures a trace provider.
// Caller is responsible for calling TracerProvider.Shutdown to ensure all data is flushed.
func TracerProvider(ctx context.Context, service string, opts TracerOpts) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		// the service name used to display traces in backends
		semconv.ServiceNameKey.String(service),
	)

	var (
		tracerOpts = []sdktrace.TracerProviderOption{
			sdktrace.WithResource(res),
		}
		closers = []func(context.Context) error{}
	)

	if opts.DataDog {
		// See more:
		// https://docs.datadoghq.com/tracing/metrics/runtime_metrics/go/
		dd := ddotel.NewTracerProvider(ddtracer.WithRuntimeMetrics())
		closers = append(closers, func(_ context.Context) error {
			// For some reason, this doesn't appear to actually wind down
			// the goroutines.
			return dd.Shutdown()
		})

		// See https://docs.datadoghq.com/profiler/enabling/go/
		_ = ddprofiler.Start(
			ddprofiler.WithService("coderd"),
			ddprofiler.WithProfileTypes(
				ddprofiler.CPUProfile,
				ddprofiler.HeapProfile,
				ddprofiler.GoroutineProfile,

				// In the future, we may want to enable:
				// ddprofiler.BlockProfile,
				// ddprofiler.MutexProfile,
			),
		)
		closers = append(closers, func(_ context.Context) error {
			ddprofiler.Stop()
			return nil
		})
	}

	if opts.Default {
		exporter, err := DefaultExporter(ctx)
		if err != nil {
			return nil, nil, xerrors.Errorf("default exporter: %w", err)
		}
		closers = append(closers, exporter.Shutdown)
		tracerOpts = append(tracerOpts, sdktrace.WithBatcher(exporter))
	}
	if opts.Honeycomb != "" {
		exporter, err := HoneycombExporter(ctx, opts.Honeycomb)
		if err != nil {
			return nil, nil, xerrors.Errorf("honeycomb exporter: %w", err)
		}
		closers = append(closers, exporter.Shutdown)
		tracerOpts = append(tracerOpts, sdktrace.WithBatcher(exporter))
	}

	tracerProvider := sdktrace.NewTracerProvider(tracerOpts...)
	otel.SetTracerProvider(tracerProvider)
	// Ignore otel errors!
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(_ error) {}))
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	otel.SetLogger(logr.Discard())

	return tracerProvider, func(ctx context.Context) error {
		var merr error
		err := tracerProvider.ForceFlush(ctx)
		if err != nil {
			merr = multierror.Append(merr, xerrors.Errorf("tracerProvider.ForceFlush(): %w", err))
		}
		for i, closer := range closers {
			err = closer(ctx)
			if err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("closer() %d: %w", i, err))
			}
		}
		err = tracerProvider.Shutdown(ctx)
		if err != nil {
			merr = multierror.Append(merr, xerrors.Errorf("tracerProvider.Shutdown(): %w", err))
		}

		return merr
	}, nil
}

func DefaultExporter(ctx context.Context) (*otlptrace.Exporter, error) {
	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(otlptracegrpc.WithInsecure()))
	if err != nil {
		return nil, xerrors.Errorf("create otlp exporter: %w", err)
	}

	return exporter, nil
}

func HoneycombExporter(ctx context.Context, apiKey string) (*otlptrace.Exporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint("api.honeycomb.io:443"),
		otlptracegrpc.WithHeaders(map[string]string{
			"x-honeycomb-team": apiKey,
		}),
		otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
	}

	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(opts...))
	if err != nil {
		return nil, xerrors.Errorf("create otlp exporter: %w", err)
	}

	return exporter, nil
}
