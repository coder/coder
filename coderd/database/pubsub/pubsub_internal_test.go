package pubsub

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestPubSub_DoesntBlockNotify(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	uut := newWithoutListener(logger, nil)
	fListener := newFakePqListener()
	uut.pgListener = fListener
	go uut.listen()

	cancels := make(chan func())
	go func() {
		subCancel, err := uut.Subscribe("bagels", func(ctx context.Context, message []byte) {
			t.Logf("got message: %s", string(message))
		})
		assert.NoError(t, err)
		cancels <- subCancel
	}()
	subCancel := testutil.TryReceive(ctx, t, cancels)
	cancelDone := make(chan struct{})
	go func() {
		defer close(cancelDone)
		subCancel()
	}()
	testutil.TryReceive(ctx, t, cancelDone)

	closeErrs := make(chan error)
	go func() {
		closeErrs <- uut.Close()
	}()
	err := testutil.TryReceive(ctx, t, closeErrs)
	require.NoError(t, err)
}

// TestPubSub_DoesntRaceListenUnlisten tests for regressions of
// https://github.com/coder/coder/issues/15312
func TestPubSub_DoesntRaceListenUnlisten(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)

	uut := newWithoutListener(logger, nil)
	fListener := newFakePqListener()
	uut.pgListener = fListener
	go uut.listen()

	noopListener := func(_ context.Context, _ []byte) {}

	const numEvents = 500
	events := make([]string, numEvents)
	cancels := make([]func(), numEvents)
	for i := range events {
		var err error
		events[i] = fmt.Sprintf("event-%d", i)
		cancels[i], err = uut.Subscribe(events[i], noopListener)
		require.NoError(t, err)
	}
	start := make(chan struct{})
	done := make(chan struct{})
	finalCancels := make([]func(), numEvents)
	for i := range events {
		event := events[i]
		cancel := cancels[i]
		go func() {
			<-start
			var err error
			// subscribe again
			finalCancels[i], err = uut.Subscribe(event, noopListener)
			assert.NoError(t, err)
			done <- struct{}{}
		}()
		go func() {
			<-start
			cancel()
			done <- struct{}{}
		}()
	}
	close(start)
	for range numEvents * 2 {
		_ = testutil.TryReceive(ctx, t, done)
	}
	for i := range events {
		fListener.requireIsListening(t, events[i])
		finalCancels[i]()
	}
	require.Len(t, uut.queues, 0)
}

const (
	numNotifications = 5
	testMessage      = "birds of a feather"
)

// fakePqListener is a fake version of pq.Listener.  This test code tests for regressions of
// https://github.com/coder/coder/issues/11950 where pq.Listener deadlocked because we blocked the
// PGPubsub.listen() goroutine while calling other pq.Listener functions.  So, all function calls
// into the fakePqListener will send 5 notifications before returning to ensure the listen()
// goroutine is unblocked.
type fakePqListener struct {
	mu       sync.Mutex
	channels map[string]struct{}
	notify   chan *pq.Notification
}

func (f *fakePqListener) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := f.getTestChanLocked()
	for i := 0; i < numNotifications; i++ {
		f.notify <- &pq.Notification{Channel: ch, Extra: testMessage}
	}
	// note that the realPqListener must only be closed once, so go ahead and
	// close the notify unprotected here.  If it panics, we have a bug.
	close(f.notify)
	return nil
}

func (f *fakePqListener) Listen(s string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := f.getTestChanLocked()
	for i := 0; i < numNotifications; i++ {
		f.notify <- &pq.Notification{Channel: ch, Extra: testMessage}
	}
	if _, ok := f.channels[s]; ok {
		return pq.ErrChannelAlreadyOpen
	}
	f.channels[s] = struct{}{}
	return nil
}

func (f *fakePqListener) Unlisten(s string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := f.getTestChanLocked()
	for i := 0; i < numNotifications; i++ {
		f.notify <- &pq.Notification{Channel: ch, Extra: testMessage}
	}
	if _, ok := f.channels[s]; ok {
		delete(f.channels, s)
		return nil
	}
	return pq.ErrChannelNotOpen
}

func (f *fakePqListener) NotifyChan() <-chan *pq.Notification {
	return f.notify
}

// getTestChanLocked returns the name of a channel we are currently listening for, if there is one.
// Otherwise, it just returns "test".  We prefer to send test notifications for channels that appear
// in the tests, but if there are none, just return anything.
func (f *fakePqListener) getTestChanLocked() string {
	for c := range f.channels {
		return c
	}
	return "test"
}

func newFakePqListener() *fakePqListener {
	return &fakePqListener{
		channels: make(map[string]struct{}),
		notify:   make(chan *pq.Notification),
	}
}

func (f *fakePqListener) requireIsListening(t testing.TB, s string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.channels[s]
	require.True(t, ok, "should be listening for '%s', but isn't", s)
}
