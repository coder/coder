package aibridged_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridged"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestSubscribeProviderReload(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)

	logger := slogtest.Make(t, nil)
	ps := dbpubsub.NewInMemory()
	t.Cleanup(func() { _ = ps.Close() })

	calls := &recordingReloader{}

	unsub, err := aibridged.SubscribeProviderReload(ctx, ps, calls, logger)
	require.NoError(t, err)
	t.Cleanup(unsub)

	require.Equal(t, 1, calls.count())

	require.NoError(t, ps.Publish(pubsub.AIProvidersChangedChannel, nil))

	require.Eventually(t, func() bool { return calls.count() >= 2 }, testutil.WaitShort, testutil.IntervalFast,
		"Reload must fire again after a pubsub notification")
}

func TestSubscribeProviderReloadSurfacesReloadError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)

	logger := slogtest.Make(t, nil)
	ps := dbpubsub.NewInMemory()
	t.Cleanup(func() { _ = ps.Close() })

	calls := &recordingReloader{returnErr: true}

	unsub, err := aibridged.SubscribeProviderReload(ctx, ps, calls, logger)
	require.NoError(t, err)
	t.Cleanup(unsub)

	require.Equal(t, 1, calls.count())
	require.NoError(t, ps.Publish(pubsub.AIProvidersChangedChannel, nil))
	require.Eventually(t, func() bool { return calls.count() >= 2 }, testutil.WaitShort, testutil.IntervalFast,
		"Reload must keep firing even after a previous Reload returned an error")
}

func TestSubscribeProviderReloadFailsWhenSubscribeFails(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)

	logger := slogtest.Make(t, nil)
	ps := &subscribeErrPubsub{}

	calls := &recordingReloader{}
	unsub, err := aibridged.SubscribeProviderReload(ctx, ps, calls, logger)
	require.Error(t, err, "a subscription failure must be surfaced to the caller")
	require.Nil(t, unsub)

	// Without a subscription the snapshot can never track changes, so the
	// caller must fail; no reload is attempted.
	require.Equal(t, 0, calls.count())
}

func TestSubscribeProviderReloadIgnoresEventError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)

	logger := slogtest.Make(t, nil)
	ps := &errInjectingPubsub{}

	calls := &recordingReloader{}
	unsub, err := aibridged.SubscribeProviderReload(ctx, ps, calls, logger)
	require.NoError(t, err)
	t.Cleanup(unsub)

	require.Equal(t, 1, calls.count())

	ps.listener(ctx, nil, errPubsubDelivery)
	require.Equal(t, 1, calls.count())

	ps.listener(ctx, nil, nil)
	require.Equal(t, 2, calls.count())
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

var (
	errReloadFailed   = stubError("reload failed")
	errPubsubDelivery = stubError("pubsub delivery failed")
)

type stubError string

func (s stubError) Error() string { return string(s) }

var _ dbpubsub.Pubsub = &errInjectingPubsub{}

type errInjectingPubsub struct {
	listener dbpubsub.ListenerWithErr
}

func (*errInjectingPubsub) Subscribe(string, dbpubsub.Listener) (func(), error) {
	return nil, xerrors.New("Subscribe not implemented")
}

func (p *errInjectingPubsub) SubscribeWithErr(_ string, listener dbpubsub.ListenerWithErr) (func(), error) {
	p.listener = listener
	return func() {}, nil
}

func (*errInjectingPubsub) Publish(string, []byte) error {
	return xerrors.New("Publish not implemented")
}

func (*errInjectingPubsub) Close() error {
	return nil
}

var _ dbpubsub.Pubsub = &subscribeErrPubsub{}

// subscribeErrPubsub fails every subscription attempt, exercising the path
// where SubscribeProviderReload cannot establish a subscription.
type subscribeErrPubsub struct{}

func (*subscribeErrPubsub) Subscribe(string, dbpubsub.Listener) (func(), error) {
	return nil, xerrors.New("Subscribe not implemented")
}

func (*subscribeErrPubsub) SubscribeWithErr(string, dbpubsub.ListenerWithErr) (func(), error) {
	return nil, xerrors.New("subscribe failed")
}

func (*subscribeErrPubsub) Publish(string, []byte) error {
	return xerrors.New("Publish not implemented")
}

func (*subscribeErrPubsub) Close() error {
	return nil
}
