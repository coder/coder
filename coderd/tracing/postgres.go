package tracing
import (
	"errors"
	"context"
	"fmt"
	"strings"
	"go.nhat.io/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/trace"
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
		otelsql.WithSpanNameFormatter(formatPostgresSpan),
	)
	if err != nil {
		return "", fmt.Errorf("registering postgres tracing driver: %w", err)
	}
	return driverName, nil
}
func formatPostgresSpan(ctx context.Context, op string) string {
	const qPrefix = "-- name: "
	q := otelsql.QueryFromContext(ctx)
	if q == "" || !strings.HasPrefix(q, qPrefix) {
		return strings.ToUpper(op)
	}
	// Remove the qPrefix and then grab the method name.
	// We expect the first line of the query to be in
	// the format "-- name: GetAPIKeyByID :one".
	s := strings.SplitN(strings.TrimPrefix(q, qPrefix), " ", 2)[0]
	return fmt.Sprintf("%s %s", strings.ToUpper(op), s)
}
