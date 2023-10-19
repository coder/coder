package audit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"

	"github.com/coder/coder/v2/coderd/audit"
)

func TestBaggage(t *testing.T) {
	t.Parallel()
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	expected := audit.WorkspaceBuildBaggage{
		IP: "127.0.0.1",
	}

	ctx, err := audit.BaggageToContext(context.Background(), expected)
	require.NoError(t, err)

	carrier := propagation.MapCarrier{}
	prop.Inject(ctx, carrier)
	bCtx := prop.Extract(ctx, carrier)
	got := audit.BaggageFromContext(bCtx)

	require.Equal(t, expected, got)
}
