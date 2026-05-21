//nolint:testpackage // Inspects internal coalescing state.
package nats

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func TestCoalescing_SameSubjectSharesSubscription(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })

	cancelA, err := ps.Subscribe("coalesce_evt", func(context.Context, []byte) {})
	require.NoError(t, err)
	t.Cleanup(cancelA)
	cancelB, err := ps.Subscribe("coalesce_evt", func(context.Context, []byte) {})
	require.NoError(t, err)
	t.Cleanup(cancelB)

	ps.mu.Lock()
	defer ps.mu.Unlock()
	require.Len(t, ps.sharedBySubject, 1)
}
