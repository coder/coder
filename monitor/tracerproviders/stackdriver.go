package tracerproviders

import (
	"context"
	"net/http"
	"time"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/xerrors"

	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// StackdriverProvider returns a tracer provider that exports to stackdriver.
func StackdriverProvider(ctx context.Context) (*sdktrace.TracerProvider, error) {
	client := &http.Client{
		Timeout: time.Millisecond * 200,
	}

	pid, err := metadata.NewClient(client).ProjectID()
	if err != nil {
		return nil, xerrors.Errorf("get gcp project ID: %q: %w", pid, err)
	}

	exporter, err := texporter.New(texporter.WithProjectID(pid))
	if err != nil {
		return nil, xerrors.Errorf("create new exporter for project %q: %w", pid, err)
	}

	return sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter)), nil
}
