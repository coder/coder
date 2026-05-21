//nolint:testpackage // Internal test: exercises sharedSub readiness internals.
package nats

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func newReadinessPubsub(t *testing.T) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

func TestReadiness_PublishAfterJoinerNotLost(t *testing.T) {
	t.Parallel()
	ps := newReadinessPubsub(t)

	const event = "ready_publish_evt"
	release := make(chan struct{})
	creatorAtHook := make(chan struct{})
	var hookFired atomic.Bool
	ps.testHookBeforeFlush = func(string) {
		if !hookFired.CompareAndSwap(false, true) {
			return
		}
		close(creatorAtHook)
		<-release
	}

	gotCreator := make(chan []byte, 1)
	creatorReady := make(chan struct{})
	creatorErr := make(chan error, 1)
	go func() {
		c, err := ps.Subscribe(event, func(_ context.Context, msg []byte) {
			select {
			case gotCreator <- msg:
			default:
			}
		})
		if err == nil {
			t.Cleanup(c)
		}
		creatorErr <- err
		close(creatorReady)
	}()
	select {
	case <-creatorAtHook:
	case <-time.After(testutil.WaitShort):
		t.Fatal("creator did not reach hook")
	}

	gotJoiner := make(chan []byte, 1)
	joinerReady := make(chan struct{})
	joinerErr := make(chan error, 1)
	go func() {
		c, err := ps.Subscribe(event, func(_ context.Context, msg []byte) {
			select {
			case gotJoiner <- msg:
			default:
			}
		})
		if err == nil {
			t.Cleanup(c)
		}
		joinerErr <- err
		close(joinerReady)
	}()

	close(release)
	select {
	case <-creatorReady:
	case <-time.After(testutil.WaitShort):
		t.Fatal("creator never returned from Subscribe")
	}
	require.NoError(t, <-creatorErr)
	select {
	case <-joinerReady:
	case <-time.After(testutil.WaitShort):
		t.Fatal("joiner never returned from Subscribe")
	}
	require.NoError(t, <-joinerErr)

	require.NoError(t, ps.Publish(event, []byte("after-joiner")))
	require.NoError(t, ps.Flush())

	ctx := testutil.Context(t, testutil.WaitShort)
	require.Equal(t, "after-joiner", string(testutil.TryReceive(ctx, t, gotCreator)))
	require.Equal(t, "after-joiner", string(testutil.TryReceive(ctx, t, gotJoiner)))
}

func TestClose_DuringInitDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)

	const event = "close_during_init_evt"
	creatorAtHook := make(chan struct{})
	releaseCreator := make(chan struct{})
	var hookFired atomic.Bool
	ps.testHookBeforeFlush = func(string) {
		if !hookFired.CompareAndSwap(false, true) {
			return
		}
		close(creatorAtHook)
		<-releaseCreator
	}

	type result struct {
		cancel func()
		err    error
	}
	creatorResult := make(chan result, 1)
	go func() {
		c, err := ps.Subscribe(event, func(context.Context, []byte) {})
		creatorResult <- result{cancel: c, err: err}
	}()
	select {
	case <-creatorAtHook:
	case <-time.After(testutil.WaitShort):
		t.Fatal("creator did not reach pre-Flush hook")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- ps.Close() }()
	require.Eventually(t, func() bool { return ps.ctx.Err() != nil },
		testutil.WaitShort, testutil.IntervalFast,
		"Close must cancel p.ctx promptly")
	close(releaseCreator)

	select {
	case err := <-closeDone:
		require.NoError(t, err)
	case <-time.After(testutil.WaitMedium):
		t.Fatal("Close deadlocked during init window")
	}

	select {
	case r := <-creatorResult:
		require.Error(t, r.err)
		require.Nil(t, r.cancel)
	case <-time.After(testutil.WaitShort):
		t.Fatal("SubscribeWithErr never returned after Close")
	}

	require.Equal(t, 0, sharedCount(ps))
	require.Equal(t, 0, listenerCount(ps))
}

func TestClose_RejectsNewSubscribes(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)

	require.NoError(t, ps.Close())
	_, err = ps.Subscribe("post_close_evt", func(context.Context, []byte) {})
	require.Error(t, err)
	require.Equal(t, 0, listenerCount(ps))
	require.Equal(t, 0, sharedCount(ps))
}
