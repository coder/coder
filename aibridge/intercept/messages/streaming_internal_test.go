package messages

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
)

// TestClientContentBlockIndex verifies that relayed content blocks are
// re-indexed into a single contiguous, monotonic client sequence across the
// stitched upstream messages of an agentic tool loop. Each upstream message
// restarts its own indices at 0, and injected tool blocks are dropped from the
// relayed stream, so the client must never see a gap or a repeated index.
func TestClientContentBlockIndex(t *testing.T) {
	t.Parallel()

	var i StreamingInterception

	start := func(idx int64) anthropic.MessageStreamEventUnion {
		return anthropic.MessageStreamEventUnion{Type: "content_block_start", Index: idx}
	}
	delta := func(idx int64) anthropic.MessageStreamEventUnion {
		return anthropic.MessageStreamEventUnion{Type: "content_block_delta", Index: idx}
	}
	stop := func(idx int64) anthropic.MessageStreamEventUnion {
		return anthropic.MessageStreamEventUnion{Type: "content_block_stop", Index: idx}
	}

	var relayedBlocks int64

	// First upstream message: a single relayed text block at upstream index 0.
	// (Its injected tool_use block at index 1 is suppressed before relay, so it
	// never reaches the remapper.)
	first := make(map[int64]int64)
	require.Equal(t, int64(0), derefBlockIndex(t, i.clientContentBlockIndex(start(0), first, &relayedBlocks)))
	require.Equal(t, int64(0), derefBlockIndex(t, i.clientContentBlockIndex(delta(0), first, &relayedBlocks)))
	require.Equal(t, int64(0), derefBlockIndex(t, i.clientContentBlockIndex(stop(0), first, &relayedBlocks)))

	// Second upstream message: its text block restarts at upstream index 0 but
	// must be relayed as the next contiguous client index (1).
	second := make(map[int64]int64)
	require.Equal(t, int64(1), derefBlockIndex(t, i.clientContentBlockIndex(start(0), second, &relayedBlocks)))
	require.Equal(t, int64(1), derefBlockIndex(t, i.clientContentBlockIndex(delta(0), second, &relayedBlocks)))
	require.Equal(t, int64(1), derefBlockIndex(t, i.clientContentBlockIndex(stop(0), second, &relayedBlocks)))

	require.Equal(t, int64(2), relayedBlocks)
}

// TestClientContentBlockIndexNonContentEvents verifies that events without a
// content block index (message_start, message_delta, message_stop) are not
// remapped and do not consume a client index.
func TestClientContentBlockIndexNonContentEvents(t *testing.T) {
	t.Parallel()

	var i StreamingInterception
	var relayedBlocks int64
	m := make(map[int64]int64)

	for _, typ := range []string{"message_start", "message_delta", "message_stop", "ping"} {
		require.Nil(t, i.clientContentBlockIndex(anthropic.MessageStreamEventUnion{Type: typ}, m, &relayedBlocks))
	}
	require.Equal(t, int64(0), relayedBlocks)

	// A multi-block message relays contiguous indices unchanged.
	require.Equal(t, int64(0), derefBlockIndex(t, i.clientContentBlockIndex(anthropic.MessageStreamEventUnion{Type: "content_block_start", Index: 0}, m, &relayedBlocks)))
	require.Equal(t, int64(1), derefBlockIndex(t, i.clientContentBlockIndex(anthropic.MessageStreamEventUnion{Type: "content_block_start", Index: 1}, m, &relayedBlocks)))
}

// TestClientContentBlockIndexUnmappedContinuation verifies that a delta or stop
// arriving for an upstream index that never had a start is left unmapped rather
// than being assigned a spurious client index.
func TestClientContentBlockIndexUnmappedContinuation(t *testing.T) {
	t.Parallel()

	var i StreamingInterception
	var relayedBlocks int64
	m := make(map[int64]int64)

	require.Nil(t, i.clientContentBlockIndex(anthropic.MessageStreamEventUnion{Type: "content_block_delta", Index: 7}, m, &relayedBlocks))
	require.Nil(t, i.clientContentBlockIndex(anthropic.MessageStreamEventUnion{Type: "content_block_stop", Index: 7}, m, &relayedBlocks))
	require.Equal(t, int64(0), relayedBlocks)
}

func derefBlockIndex(t *testing.T, p *int64) int64 {
	t.Helper()
	require.NotNil(t, p)
	return *p
}
