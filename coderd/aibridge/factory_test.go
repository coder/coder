package aibridge_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge"
)

func TestAttributionContext(t *testing.T) {
	t.Parallel()

	wsID := uuid.New()

	t.Run("WithWorkspace", func(t *testing.T) {
		t.Parallel()

		attr := aibridge.Attribution{WorkspaceID: wsID}
		ctx := aibridge.WithAttribution(context.Background(), attr)
		got, ok := aibridge.AttributionFromContext(ctx)
		require.True(t, ok)
		require.Equal(t, attr, got)
	})

	t.Run("NoAttribution", func(t *testing.T) {
		t.Parallel()

		_, ok := aibridge.AttributionFromContext(context.Background())
		require.False(t, ok)
	})

	t.Run("ZeroWorkspaceID", func(t *testing.T) {
		t.Parallel()

		// An Attribution with a zero WorkspaceID is not considered set.
		attr := aibridge.Attribution{WorkspaceID: uuid.Nil}
		ctx := aibridge.WithAttribution(context.Background(), attr)
		_, ok := aibridge.AttributionFromContext(ctx)
		require.False(t, ok, "Attribution with zero WorkspaceID must not be considered set")
	})

	t.Run("OverwrittenByInnerContext", func(t *testing.T) {
		t.Parallel()

		// A child context can shadow the parent's Attribution without
		// mutating it, ensuring per-request immutability.
		parent := aibridge.Attribution{WorkspaceID: uuid.New()}
		child := aibridge.Attribution{WorkspaceID: wsID}

		pCtx := aibridge.WithAttribution(context.Background(), parent)
		cCtx := aibridge.WithAttribution(pCtx, child)

		gotParent, ok := aibridge.AttributionFromContext(pCtx)
		require.True(t, ok)
		require.Equal(t, parent, gotParent)

		gotChild, ok := aibridge.AttributionFromContext(cCtx)
		require.True(t, ok)
		require.Equal(t, child, gotChild)
	})
}
