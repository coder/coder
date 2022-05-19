package tracing

import (
	"github.com/nhatthm/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"
)

// Postgres driver will register a new tracing sql driver and return the driver name.
func PostgresDriver(tp trace.TracerProvider, service string) (string, error) {
	// Register the otelsql wrapper for the provided postgres driver.
	driverName, err := otelsql.Register("postgres",
		otelsql.WithDefaultAttributes(
			semconv.ServiceNameKey.String(service),
		),
		otelsql.TraceQueryWithoutArgs(),
		otelsql.WithSystem(semconv.DBSystemPostgreSQL),
		otelsql.WithTracerProvider(tp),
	)
	if err != nil {
		return "", xerrors.Errorf("registering postgres tracing driver: %w", err)
	}

	return driverName, nil
}
