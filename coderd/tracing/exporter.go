package tracing

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/credentials"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	ddprofiler "gopkg.in/DataDog/dd-trace-go.v1/profiler"
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

type OtelTracerProvider interface {
	trace.TracerProvider
	Shutdown(context.Context) error
	ForceFlush(context.Context) error
}

// TracerProvider creates a grpc otlp exporter and configures a trace provider.
// Caller is responsible for calling TracerProvider.Shutdown to ensure all data is flushed.
func TracerProvider(ctx context.Context, service string, opts TracerOpts) (OtelTracerProvider, func(context.Context) error, error) {
	var (
		tracerProvider OtelTracerProvider
		closers        = []func(context.Context) error{}
		addCloser      = func(closer func(context.Context) error) {
			closers = append(closers, closer)
		}
	)

	// DataDog is very special :) and cannot be configured as an exporter, only
	// as a provider. This means we can't use DataDog and another exporter at
	// the same time.
	if opts.DataDog {
		if opts.Default {
			return nil, nil, xerrors.New("cannot use DataDog with another trace exporter, please disable the default exporter (CODER_TRACE_ENABLE)")
		}
		if opts.Honeycomb != "" {
			return nil, nil, xerrors.New("cannot use DataDog with another trace exporter, please disable the Honeycomb exporter (CODER_TRACE_HONEYCOMB_API_KEY)")
		}

		tracerProvider = ddogTracerProvider(service, addCloser)
	} else {
		var err error
		tracerProvider, err = defaultTracerProvider(ctx, service, opts, addCloser)
		if err != nil {
			return nil, nil, xerrors.Errorf("default tracer provider: %w", err)
		}
	}

	otel.SetTracerProvider(tracerProvider)
	// Ignore otel errors!
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {}))
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

func defaultTracerProvider(ctx context.Context, service string, opts TracerOpts, addCloser func(func(ctx context.Context) error)) (OtelTracerProvider, error) {
	var (
		res = resource.NewWithAttributes(
			semconv.SchemaURL,
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String(service),
		)
		tracerOpts = []sdktrace.TracerProviderOption{
			sdktrace.WithResource(res),
		}
	)

	if opts.Default {
		exporter, err := defaultExporter(ctx)
		if err != nil {
			return nil, xerrors.Errorf("default exporter: %w", err)
		}
		addCloser(exporter.Shutdown)
		tracerOpts = append(tracerOpts, sdktrace.WithBatcher(exporter))
	}
	if opts.Honeycomb != "" {
		exporter, err := honeycombExporter(ctx, opts.Honeycomb)
		if err != nil {
			return nil, xerrors.Errorf("honeycomb exporter: %w", err)
		}
		addCloser(exporter.Shutdown)
		tracerOpts = append(tracerOpts, sdktrace.WithBatcher(exporter))
	}

	return sdktrace.NewTracerProvider(tracerOpts...), nil
}

func ddogTracerProvider(service string, addCloser func(func(ctx context.Context) error)) OtelTracerProvider {
	// Collect profiling data.
	// See https://docs.datadoghq.com/profiler/enabling/go/
	_ = ddprofiler.Start(
		ddprofiler.WithService(service),
		ddprofiler.WithProfileTypes(
			ddprofiler.CPUProfile,
			ddprofiler.HeapProfile,
			ddprofiler.GoroutineProfile,

			// In the future, we may want to enable:
			// ddprofiler.BlockProfile,
			// ddprofiler.MutexProfile,
		),
	)
	addCloser(func(_ context.Context) error {
		ddprofiler.Stop()
		return nil
	})

	// Collect regular ol' traces.
	//
	// See more:
	// https://docs.datadoghq.com/tracing/metrics/runtime_metrics/go/
	//
	// NOTE: The Shutdown method does not appear to actually wind down the
	// goroutines. We only use this in dogfood at the moment and it's a hidden
	// feature, so we're not going to worry about it for now.
	return ddogOtelTracerProvider{
		TracerProvider: ddotel.NewTracerProvider(
			ddtracer.WithService(service),
			ddtracer.WithRuntimeMetrics(),
		),
	}
}

func defaultExporter(ctx context.Context) (*otlptrace.Exporter, error) {
	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(otlptracegrpc.WithInsecure()))
	if err != nil {
		return nil, xerrors.Errorf("create otlp exporter: %w", err)
	}

	return exporter, nil
}

func honeycombExporter(ctx context.Context, apiKey string) (*otlptrace.Exporter, error) {
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

// ddogOtelTracerProvider is a wrapper around the DataDog tracer provider that
// implements the same methods as sdktrace.TracerProvider. DataDog has methods
// with the same names, but they have different signatures, so we need to wrap
// them.
type ddogOtelTracerProvider struct {
	*ddotel.TracerProvider
}

var _ OtelTracerProvider = ddogOtelTracerProvider{}

func (p ddogOtelTracerProvider) Shutdown(_ context.Context) error {
	return p.TracerProvider.Shutdown()
}

func (p ddogOtelTracerProvider) ForceFlush(ctx context.Context) error {
	errCh := make(chan error, 1)

	timeout := 10 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout < 0 {
			return ctx.Err()
		}
	}
	p.TracerProvider.ForceFlush(timeout, func(ok bool) {
		if ok {
			errCh <- nil
		} else {
			errCh <- xerrors.New("datadog force flush failed")
		}
	})
	return <-errCh
}
