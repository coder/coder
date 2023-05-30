package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/testutil"
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
	cycles := (PubsubBufferSize / 5) * 2 // almost twice around the ring
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
	cycles := (PubsubBufferSize / 5) * 2 // almost twice around the ring
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
	for i := 0; i < PubsubBufferSize+2; i++ {
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
	require.Equal(t, PubsubBufferSize, n)
}
