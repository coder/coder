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

func Test_msgQueue_ListenerWithError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	m := make(chan string)
	e := make(chan error)
	uut := newMsgQueue(ctx, nil, func(ctx context.Context, msg []byte, err error) {
		m <- string(msg)
		e <- err
	})
	defer uut.close()

	// We're going to enqueue 4 messages and an error in a loop -- that is, a cycle of 5.
	// PubsubBufferSize is 2048, which is a power of 2, so a pattern of 5 will not be aligned
	// when we wrap around the end of the circular buffer.  This tests that we correctly handle
	// the wrapping and aren't dequeueing misaligned data.
	cycles := (BufferSize / 5) * 2 // almost twice around the ring
	for j := 0; j < cycles; j++ {
		for i := 0; i < 4; i++ {
			uut.enqueue([]byte(fmt.Sprintf("%d%d", j, i)))
		}
		uut.dropped()
		for i := 0; i < 4; i++ {
			select {
			case <-ctx.Done():
				t.Fatal("timed out")
			case msg := <-m:
				require.Equal(t, fmt.Sprintf("%d%d", j, i), msg)
			}
			select {
			case <-ctx.Done():
				t.Fatal("timed out")
			case err := <-e:
				require.NoError(t, err)
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("timed out")
		case msg := <-m:
			require.Equal(t, "", msg)
		}
		select {
		case <-ctx.Done():
			t.Fatal("timed out")
		case err := <-e:
			require.ErrorIs(t, err, ErrDroppedMessages)
		}
	}
}

func Test_msgQueue_Listener(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	m := make(chan string)
	uut := newMsgQueue(ctx, func(ctx context.Context, msg []byte) {
		m <- string(msg)
	}, nil)
	defer uut.close()

	// We're going to enqueue 4 messages and an error in a loop -- that is, a cycle of 5.
	// PubsubBufferSize is 2048, which is a power of 2, so a pattern of 5 will not be aligned
	// when we wrap around the end of the circular buffer.  This tests that we correctly handle
	// the wrapping and aren't dequeueing misaligned data.
	cycles := (BufferSize / 5) * 2 // almost twice around the ring
	for j := 0; j < cycles; j++ {
		for i := 0; i < 4; i++ {
			uut.enqueue([]byte(fmt.Sprintf("%d%d", j, i)))
		}
		uut.dropped()
		for i := 0; i < 4; i++ {
			select {
			case <-ctx.Done():
				t.Fatal("timed out")
			case msg := <-m:
				require.Equal(t, fmt.Sprintf("%d%d", j, i), msg)
			}
		}
		// Listener skips over errors, so we only read out the 4 real messages.
	}
}

func Test_msgQueue_Full(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	firstDequeue := make(chan struct{})
	allowRead := make(chan struct{})
	n := 0
	errors := make(chan error)
	uut := newMsgQueue(ctx, nil, func(ctx context.Context, msg []byte, err error) {
		if n == 0 {
			close(firstDequeue)
		}
		<-allowRead
		if err == nil {
			require.Equal(t, fmt.Sprintf("%d", n), string(msg))
			n++
			return
		}
		errors <- err
	})
	defer uut.close()

	// we send 2 more than the capacity.  One extra because the call to the ListenerFunc blocks
	// but only after we've dequeued a message, and then another extra because we want to exceed
	// the capacity, not just reach it.
	for i := 0; i < BufferSize+2; i++ {
		uut.enqueue([]byte(fmt.Sprintf("%d", i)))
		// ensure the first dequeue has happened before proceeding, so that this function isn't racing
		// against the goroutine that dequeues items.
		<-firstDequeue
	}
	close(allowRead)

	select {
	case <-ctx.Done():
		t.Fatal("timed out")
	case err := <-errors:
		require.ErrorIs(t, err, ErrDroppedMessages)
	}
	// Ok, so we sent 2 more than capacity, but we only read the capacity, that's because the last
	// message we send doesn't get queued, AND, it bumps a message out of the queue to make room
	// for the error, so we read 2 less than we sent.
	require.Equal(t, BufferSize, n)
}

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
	subCancel := testutil.RequireRecvCtx(ctx, t, cancels)
	cancelDone := make(chan struct{})
	go func() {
		defer close(cancelDone)
		subCancel()
	}()
	testutil.RequireRecvCtx(ctx, t, cancelDone)

	closeErrs := make(chan error)
	go func() {
		closeErrs <- uut.Close()
	}()
	err := testutil.RequireRecvCtx(ctx, t, closeErrs)
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
		_ = testutil.RequireRecvCtx(ctx, t, done)
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
