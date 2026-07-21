package aibridged_test

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
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

func TestSubscribeProviderReloadReloadsOnEventError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)

	logger := slogtest.Make(t, nil)
	ps := &errInjectingPubsub{}

	calls := &recordingReloader{}
	unsub, err := aibridged.SubscribeProviderReload(ctx, ps, calls, logger)
	require.NoError(t, err)
	t.Cleanup(unsub)

	require.Equal(t, 1, calls.count())

	// A dropped-message delivery error may have masked a change, so it must
	// still trigger a reload to reconverge.
	ps.listener(ctx, nil, errPubsubDelivery)
	require.Equal(t, 2, calls.count())

	ps.listener(ctx, nil, nil)
	require.Equal(t, 3, calls.count())
}

func TestWatchProviderReload(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	logger := slogtest.Make(t, nil)

	ctrl := gomock.NewController(t)
	mockClient := aibridgedmock.NewMockDRPCClient(ctrl)

	// A single stream delivers two change signals, then blocks on its context
	// until the watch is canceled.
	events := make(chan error, 2)
	events <- nil
	events <- nil
	mockClient.EXPECT().WatchAIProviders(gomock.Any(), gomock.Any()).DoAndReturn(
		func(rpcCtx context.Context, _ *proto.WatchAIProvidersRequest) (proto.DRPCProviderConfigurator_WatchAIProvidersClient, error) {
			return &fakeWatchClientStream{ctx: rpcCtx, events: events}, nil
		}).AnyTimes()

	calls := &recordingReloader{}
	clientFunc := func(context.Context) (aibridged.DRPCClient, error) { return mockClient, nil }

	watchCtx, watchCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- aibridged.WatchProviderReload(watchCtx, clientFunc, calls, logger) }()

	require.Eventually(t, func() bool { return calls.count() >= 2 }, testutil.WaitShort, testutil.IntervalFast,
		"each change signal must trigger a reload")

	watchCancel()
	require.ErrorIs(t, testutil.TryReceive(ctx, t, done), context.Canceled)
}

func TestWatchProviderReloadReconnects(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	logger := slogtest.Make(t, nil)

	ctrl := gomock.NewController(t)
	mockClient := aibridgedmock.NewMockDRPCClient(ctrl)

	// The first stream delivers one signal then drops; subsequent streams
	// deliver one signal then block. WatchProviderReload must reconnect after
	// the drop and keep reloading.
	var attempt atomic.Int32
	mockClient.EXPECT().WatchAIProviders(gomock.Any(), gomock.Any()).DoAndReturn(
		func(rpcCtx context.Context, _ *proto.WatchAIProvidersRequest) (proto.DRPCProviderConfigurator_WatchAIProvidersClient, error) {
			ev := make(chan error, 2)
			if attempt.Add(1) == 1 {
				ev <- nil
				ev <- io.EOF
			} else {
				ev <- nil
			}
			return &fakeWatchClientStream{ctx: rpcCtx, events: ev}, nil
		}).AnyTimes()

	calls := &recordingReloader{}
	clientFunc := func(context.Context) (aibridged.DRPCClient, error) { return mockClient, nil }

	watchCtx, watchCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- aibridged.WatchProviderReload(watchCtx, clientFunc, calls, logger) }()

	require.Eventually(t, func() bool { return calls.count() >= 2 }, testutil.WaitShort, testutil.IntervalFast,
		"reload must continue after the stream drops and reconnects")

	watchCancel()
	require.ErrorIs(t, testutil.TryReceive(ctx, t, done), context.Canceled)
}

func TestWatchProviderReloadCancelUnblocksClient(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	logger := slogtest.Make(t, nil)

	// clientFn blocks until its context is canceled, modeling
	// Server.ClientContext waiting for the daemon to connect to coderd. Only
	// watchCancel is exercised (no stream activity, no daemon close), so the
	// loop can return only if clientFn honors the context it receives.
	var once sync.Once
	entered := make(chan struct{})
	clientFunc := func(clientCtx context.Context) (aibridged.DRPCClient, error) {
		once.Do(func() { close(entered) })
		<-clientCtx.Done()
		return nil, clientCtx.Err()
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- aibridged.WatchProviderReload(watchCtx, clientFunc, &recordingReloader{}, logger) }()

	testutil.TryReceive(ctx, t, entered)
	watchCancel()
	require.ErrorIs(t, testutil.TryReceive(ctx, t, done), context.Canceled)
}

func TestWatchProviderReloadRetriesDialFailure(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	logger := slogtest.Make(t, nil)

	ctrl := gomock.NewController(t)
	mockClient := aibridgedmock.NewMockDRPCClient(ctrl)

	// Once dialed, the stream delivers one signal then blocks on its context.
	mockClient.EXPECT().WatchAIProviders(gomock.Any(), gomock.Any()).DoAndReturn(
		func(rpcCtx context.Context, _ *proto.WatchAIProvidersRequest) (proto.DRPCProviderConfigurator_WatchAIProvidersClient, error) {
			ev := make(chan error, 1)
			ev <- nil
			return &fakeWatchClientStream{ctx: rpcCtx, events: ev}, nil
		}).AnyTimes()

	calls := &recordingReloader{}

	// The first dial fails; the second succeeds, and the loop must keep
	// retrying until the dial succeeds and a reload fires.
	var attempt atomic.Int32
	clientFunc := func(context.Context) (aibridged.DRPCClient, error) {
		if attempt.Add(1) == 1 {
			return nil, xerrors.New("dial failed")
		}
		return mockClient, nil
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- aibridged.WatchProviderReload(watchCtx, clientFunc, calls, logger) }()

	require.Eventually(t, func() bool { return calls.count() >= 1 }, testutil.WaitShort, testutil.IntervalFast,
		"reload must fire only after a failed dial is retried successfully")
	require.GreaterOrEqual(t, int(attempt.Load()), 2, "the first dial must have failed and been retried")

	watchCancel()
	require.ErrorIs(t, testutil.TryReceive(ctx, t, done), context.Canceled)
}

func TestWatchProviderReloadContinuesAfterReloadError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	logger := slogtest.Make(t, nil)

	ctrl := gomock.NewController(t)
	mockClient := aibridgedmock.NewMockDRPCClient(ctrl)

	events := make(chan error, 3)
	for range 3 {
		events <- nil
	}
	mockClient.EXPECT().WatchAIProviders(gomock.Any(), gomock.Any()).DoAndReturn(
		func(rpcCtx context.Context, _ *proto.WatchAIProvidersRequest) (proto.DRPCProviderConfigurator_WatchAIProvidersClient, error) {
			return &fakeWatchClientStream{ctx: rpcCtx, events: events}, nil
		}).AnyTimes()

	// Fails its first two reloads, then succeeds.
	reloader := &failNReloader{n: 2}
	clientFunc := func(context.Context) (aibridged.DRPCClient, error) { return mockClient, nil }

	watchCtx, watchCancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- aibridged.WatchProviderReload(watchCtx, clientFunc, reloader, logger) }()

	require.Eventually(t, func() bool { return reloader.count() >= 3 }, testutil.WaitShort, testutil.IntervalFast,
		"a failed reload must not stop the watch loop")

	watchCancel()
	require.ErrorIs(t, testutil.TryReceive(ctx, t, done), context.Canceled)
}

// fakeWatchClientStream is a minimal
// proto.DRPCProviderConfigurator_WatchAIProvidersClient. Each value popped from
// events either yields a change signal (nil) or returns the given error; when
// events is empty Recv blocks until the stream context is canceled.
type fakeWatchClientStream struct {
	ctx    context.Context
	events chan error
}

func (s *fakeWatchClientStream) Recv() (*proto.WatchAIProvidersResponse, error) {
	select {
	case err := <-s.events:
		if err != nil {
			return nil, err
		}
		return &proto.WatchAIProvidersResponse{}, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *fakeWatchClientStream) Context() context.Context                { return s.ctx }
func (*fakeWatchClientStream) MsgSend(drpc.Message, drpc.Encoding) error { return nil }
func (*fakeWatchClientStream) MsgRecv(drpc.Message, drpc.Encoding) error { return nil }
func (*fakeWatchClientStream) CloseSend() error                          { return nil }
func (*fakeWatchClientStream) Close() error                              { return nil }

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

// failNReloader fails its first n Reload calls, then succeeds, counting all
// calls.
type failNReloader struct {
	n     int32
	calls atomic.Int32
}

func (r *failNReloader) Reload(_ context.Context) error {
	if r.calls.Add(1) <= r.n {
		return errReloadFailed
	}
	return nil
}

func (r *failNReloader) count() int {
	return int(r.calls.Load())
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
