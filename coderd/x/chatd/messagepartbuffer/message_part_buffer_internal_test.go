package messagepartbuffer

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestBuffer_SubscribeCanceledWithoutCreateReclaimsEpisode(t *testing.T) {
	t.Parallel()

	buffer := New(Options{})
	defer buffer.Close()
	key := Key{ChatID: uuid.New(), HistoryVersion: 1, GenerationAttempt: 1}
	ctx := testutil.Context(t, testutil.WaitLong)
	ch, cancelSub, err := buffer.SubscribeToEpisode(ctx, key)
	require.NoError(t, err)
	cancelSub()
	// The subscriber goroutine removes itself from the episode before closing
	// the output channel, so cleanup is complete once the channel is closed.
drain:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break drain
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for channel close")
		}
	}

	_, err = buffer.GetParts(key)
	require.ErrorIs(t, err, ErrEpisodeNotFound)
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	require.Empty(t, buffer.episodes)
}
