package audit_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
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

func TestResourceTarget_ChatTitleNotLeaked(t *testing.T) {
	t.Parallel()

	chat := database.Chat{
		ID:    uuid.UUID{1},
		Title: "sensitive-project-name",
	}
	target := audit.ResourceTarget(chat)
	require.NotContains(t, target, chat.Title,
		"ResourceTarget for Chat must not contain the title; it should use a UUID prefix")
}
