package chatstate

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

type recordingPublisher struct {
	calls  []recordedCall
	errOn  map[string]error
	failed map[string]int
}

type recordedCall struct {
	Channel string
	Payload []byte
}

func newRecordingPublisher() *recordingPublisher {
	return &recordingPublisher{
		errOn:  map[string]error{},
		failed: map[string]int{},
	}
}

func (r *recordingPublisher) Publish(channel string, payload []byte) error {
	r.calls = append(r.calls, recordedCall{Channel: channel, Payload: append([]byte(nil), payload...)})
	if err, ok := r.errOn[channel]; ok {
		r.failed[channel]++
		return err
	}
	return nil
}

func TestPublishBuffer_DefersPublishUntilFlush(t *testing.T) {
	t.Parallel()
	inner := newRecordingPublisher()
	buf := NewPublishBuffer(inner)

	require.NoError(t, buf.Publish("a", []byte("1")))
	require.NoError(t, buf.Publish("b", []byte("2")))

	require.Empty(t, inner.calls, "inner publisher should not be called before flush")
	require.Equal(t, []string{"a", "b"}, buf.BufferedChannels())
}

func TestPublishBuffer_FlushPublishesInOrder(t *testing.T) {
	t.Parallel()
	inner := newRecordingPublisher()
	buf := NewPublishBuffer(inner)

	require.NoError(t, buf.Publish("a", []byte("1")))
	require.NoError(t, buf.Publish("b", []byte("2")))
	require.NoError(t, buf.Publish("c", []byte("3")))

	require.NoError(t, buf.Flush())
	require.Len(t, inner.calls, 3)
	require.Equal(t, "a", inner.calls[0].Channel)
	require.Equal(t, "b", inner.calls[1].Channel)
	require.Equal(t, "c", inner.calls[2].Channel)
	require.Equal(t, []byte("1"), inner.calls[0].Payload)
}

func TestPublishBuffer_FlushReturnsJoinedErrors(t *testing.T) {
	t.Parallel()
	inner := newRecordingPublisher()
	errB := xerrors.New("broken b")
	errC := xerrors.New("broken c")
	inner.errOn["b"] = errB
	inner.errOn["c"] = errC
	buf := NewPublishBuffer(inner)

	require.NoError(t, buf.Publish("a", []byte("1")))
	require.NoError(t, buf.Publish("b", []byte("2")))
	require.NoError(t, buf.Publish("c", []byte("3")))
	require.NoError(t, buf.Publish("d", []byte("4")))

	err := buf.Flush()
	require.Error(t, err)
	require.ErrorIs(t, err, errB)
	require.ErrorIs(t, err, errC)
	require.Contains(t, err.Error(), "publish b:")
	require.Contains(t, err.Error(), "publish c:")
	// Even after broken channels, later messages should still be
	// attempted so the inner publisher sees them.
	require.Len(t, inner.calls, 4)
}

func TestPublishBuffer_PublishAfterFlushFails(t *testing.T) {
	t.Parallel()
	inner := newRecordingPublisher()
	buf := NewPublishBuffer(inner)
	require.NoError(t, buf.Flush())
	require.Error(t, buf.Publish("x", []byte("y")))
}

func TestPublishBuffer_DiscardSuppressesPending(t *testing.T) {
	t.Parallel()
	inner := newRecordingPublisher()
	buf := NewPublishBuffer(inner)
	require.NoError(t, buf.Publish("a", []byte("1")))
	buf.Discard()
	require.NoError(t, buf.Flush())
	require.Empty(t, inner.calls)
}

func TestPublishBuffer_DiscardBlocksLaterPublishes(t *testing.T) {
	t.Parallel()
	inner := newRecordingPublisher()
	buf := NewPublishBuffer(inner)
	buf.Discard()
	// Discard sets disabled; subsequent Publish is a no-op (not an
	// error) so callers using Discard before/around rollback paths
	// do not have to special-case unwind.
	require.NoError(t, buf.Publish("a", []byte("1")))
	require.NoError(t, buf.Flush())
	require.Empty(t, inner.calls)
}
