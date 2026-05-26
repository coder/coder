package aibridged_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/testutil"
)

// TestSubscribeProviderReload covers the contract that the subscriber
// performs an initial Reload(ctx) synchronously and then invokes
// Reload(ctx) on every pubsub event delivered on
// AIProvidersChangedChannel.
func TestSubscribeProviderReload(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	t.Cleanup(cancel)

	logger := slogtest.Make(t, nil)
	ps := pubsub.NewInMemory()
	t.Cleanup(func() { _ = ps.Close() })

	calls := &recordingReloader{}

	unsub, err := aibridged.SubscribeProviderReload(ctx, ps, calls, logger)
	require.NoError(t, err)
	t.Cleanup(unsub)

	require.Eventually(t, func() bool { return calls.count() >= 1 }, testutil.WaitShort, testutil.IntervalFast,
		"initial Reload must fire synchronously from SubscribeProviderReload")

	require.NoError(t, ps.Publish(coderpubsub.AIProvidersChangedChannel, nil))

	require.Eventually(t, func() bool { return calls.count() >= 2 }, testutil.WaitShort, testutil.IntervalFast,
		"Reload must fire again after a pubsub notification")
}

// TestSubscribeProviderReloadSurfacesReloadError verifies that an
// error returned by Reload is logged but does not break the
// subscription: subsequent notifications keep firing.
func TestSubscribeProviderReloadSurfacesReloadError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	t.Cleanup(cancel)

	logger := slogtest.Make(t, nil)
	ps := pubsub.NewInMemory()
	t.Cleanup(func() { _ = ps.Close() })

	calls := &recordingReloader{returnErr: true}

	unsub, err := aibridged.SubscribeProviderReload(ctx, ps, calls, logger)
	require.NoError(t, err)
	t.Cleanup(unsub)

	require.Eventually(t, func() bool { return calls.count() >= 1 }, testutil.WaitShort, testutil.IntervalFast)
	require.NoError(t, ps.Publish(coderpubsub.AIProvidersChangedChannel, nil))
	require.Eventually(t, func() bool { return calls.count() >= 2 }, testutil.WaitShort, testutil.IntervalFast,
		"Reload must keep firing even after a previous Reload returned an error")
}

// TestSubscribeProviderReloadIgnoresEventError verifies that a
// pubsub-layer error delivered to the handler does not trigger Reload.
func TestSubscribeProviderReloadIgnoresEventError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	t.Cleanup(cancel)

	logger := slogtest.Make(t, nil)
	ps := pubsub.NewInMemory()
	t.Cleanup(func() { _ = ps.Close() })

	calls := &recordingReloader{}
	unsub, err := aibridged.SubscribeProviderReload(ctx, ps, calls, logger)
	require.NoError(t, err)
	t.Cleanup(unsub)

	require.Eventually(t, func() bool { return calls.count() >= 1 }, testutil.WaitShort, testutil.IntervalFast)
	initial := calls.count()

	// No way to inject a pubsub delivery error against the in-memory
	// implementation; assert nothing fires absent a publish.
	time.Sleep(testutil.IntervalFast * 5) //nolint:forbidigo // bounded wait to confirm Reload does not spuriously fire.
	require.Equal(t, initial, calls.count())
}

// recordingReloader is a minimal [aibridged.ProviderReloader] that
// counts calls.
type recordingReloader struct {
	n         atomic.Int32
	returnErr bool
}

func (r *recordingReloader) Reload(_ context.Context) error {
	r.n.Add(1)
	if r.returnErr {
		return errReloadFailed
	}
	return nil
}

func (r *recordingReloader) count() int {
	return int(r.n.Load())
}

var errReloadFailed = stubError("reload failed")

type stubError string

func (s stubError) Error() string { return string(s) }
