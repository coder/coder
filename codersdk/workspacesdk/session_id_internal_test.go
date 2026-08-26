package workspacesdk

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/websocket"
)

func TestSetClientSessionIDBaggage(t *testing.T) {
	t.Parallel()

	const sessionID = "0123456789abcdef0123456789abcdef"

	t.Run("Set", func(t *testing.T) {
		t.Parallel()

		opts := &websocket.DialOptions{}
		setClientSessionIDBaggage(opts, sessionID)

		// The value must be readable the same way coderd's tracing middleware
		// reads it, so log/trace correlation lines up end to end.
		ctx := propagation.Baggage{}.Extract(context.Background(), propagation.HeaderCarrier(opts.HTTPHeader))
		got := baggage.FromContext(ctx).Member(tracing.SessionIDBaggageKey).Value()
		require.Equal(t, sessionID, got)

		// Exactly one baggage header, so we do not emit duplicates.
		require.Len(t, opts.HTTPHeader.Values("baggage"), 1)
	})

	t.Run("EmptyIsNoop", func(t *testing.T) {
		t.Parallel()

		opts := &websocket.DialOptions{}
		setClientSessionIDBaggage(opts, "")
		require.Nil(t, opts.HTTPHeader)
	})

	t.Run("PreservesExistingHeaders", func(t *testing.T) {
		t.Parallel()

		opts := &websocket.DialOptions{HTTPHeader: http.Header{}}
		opts.HTTPHeader.Set("Coder-Session-Token", "token")
		setClientSessionIDBaggage(opts, sessionID)

		require.Equal(t, "token", opts.HTTPHeader.Get("Coder-Session-Token"))
		ctx := propagation.Baggage{}.Extract(context.Background(), propagation.HeaderCarrier(opts.HTTPHeader))
		require.Equal(t, sessionID, baggage.FromContext(ctx).Member(tracing.SessionIDBaggageKey).Value())
	})
}
