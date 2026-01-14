package metadatabatcher_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/agentapi/metadatabatcher"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		agentIDs []uuid.UUID
	}{
		{
			name:     "Empty",
			agentIDs: []uuid.UUID{},
		},
		{
			name:     "Single",
			agentIDs: []uuid.UUID{uuid.New()},
		},
		{
			name: "Multiple",
			agentIDs: []uuid.UUID{
				uuid.New(),
				uuid.New(),
				uuid.New(),
			},
		},
		{
			name: "Exactly 363 (one chunk)",
			agentIDs: func() []uuid.UUID {
				ids := make([]uuid.UUID, 363)
				for i := range ids {
					ids[i] = uuid.New()
				}
				return ids
			}(),
		},
		{
			name: "364 (two chunks)",
			agentIDs: func() []uuid.UUID {
				ids := make([]uuid.UUID, 364)
				for i := range ids {
					ids[i] = uuid.New()
				}
				return ids
			}(),
		},
		{
			name: "600 (multiple chunks)",
			agentIDs: func() []uuid.UUID {
				ids := make([]uuid.UUID, 600)
				for i := range ids {
					ids[i] = uuid.New()
				}
				return ids
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Encode the agent IDs into chunks.
			chunks := metadatabatcher.EncodeAgentIDChunks(tt.agentIDs)

			// Decode all chunks and collect the agent IDs.
			var decoded []uuid.UUID
			for _, chunk := range chunks {
				iter, err := metadatabatcher.NewAgentIDIterator(chunk)
				require.NoError(t, err)

				for {
					agentID, ok := iter.Next()
					if !ok {
						require.NoError(t, iter.Err())
						break
					}
					decoded = append(decoded, agentID)
				}
			}

			// Verify we got the same agent IDs back.
			if len(tt.agentIDs) == 0 {
				require.Empty(t, decoded)
			} else {
				require.Equal(t, tt.agentIDs, decoded)
			}
		})
	}
}

func TestEncodeAgentIDChunks(t *testing.T) {
	t.Parallel()

	t.Run("ChunkSize", func(t *testing.T) {
		t.Parallel()

		// Create 600 agents (should split into 2 chunks: 363 + 237).
		agentIDs := make([]uuid.UUID, 600)
		for i := range agentIDs {
			agentIDs[i] = uuid.New()
		}

		chunks := metadatabatcher.EncodeAgentIDChunks(agentIDs)
		require.Len(t, chunks, 2)

		// First chunk should have 363 IDs (363 * 22 = 7986 bytes).
		require.Equal(t, 363*22, len(chunks[0]))

		// Second chunk should have 237 IDs (237 * 22 = 5214 bytes).
		require.Equal(t, 237*22, len(chunks[1]))
	})

	t.Run("PayloadSizeLimit", func(t *testing.T) {
		t.Parallel()

		// Create enough agents to test the 8KB limit.
		agentIDs := make([]uuid.UUID, 1000)
		for i := range agentIDs {
			agentIDs[i] = uuid.New()
		}

		chunks := metadatabatcher.EncodeAgentIDChunks(agentIDs)

		// Each chunk should be under 8KB.
		for i, chunk := range chunks {
			require.LessOrEqual(t, len(chunk), 8000, "chunk %d exceeds 8KB limit", i)
		}
	})
}

func TestAgentIDIterator(t *testing.T) {
	t.Parallel()

	t.Run("InvalidSize", func(t *testing.T) {
		t.Parallel()

		// Data that's not a multiple of 22 bytes should fail.
		_, err := metadatabatcher.NewAgentIDIterator([]byte("invalid"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "must be multiple of 22")
	})

	t.Run("Count", func(t *testing.T) {
		t.Parallel()

		agentIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
		chunks := metadatabatcher.EncodeAgentIDChunks(agentIDs)
		require.Len(t, chunks, 1)

		iter, err := metadatabatcher.NewAgentIDIterator(chunks[0])
		require.NoError(t, err)
		require.Equal(t, 3, iter.Count())
	})

	t.Run("LazyDecoding", func(t *testing.T) {
		t.Parallel()

		// Create a batch with multiple agent IDs.
		targetID := uuid.New()
		agentIDs := []uuid.UUID{
			uuid.New(),
			uuid.New(),
			targetID, // Our target is in the middle.
			uuid.New(),
			uuid.New(),
		}

		chunks := metadatabatcher.EncodeAgentIDChunks(agentIDs)
		require.Len(t, chunks, 1)

		iter, err := metadatabatcher.NewAgentIDIterator(chunks[0])
		require.NoError(t, err)

		// Iterate until we find our target, then stop.
		found := false
		iterCount := 0
		for {
			agentID, ok := iter.Next()
			if !ok {
				break
			}
			iterCount++

			if agentID == targetID {
				found = true
				break
			}
		}

		require.True(t, found)
		require.Equal(t, 3, iterCount, "should stop after finding target")
	})

	t.Run("EmptyData", func(t *testing.T) {
		t.Parallel()

		iter, err := metadatabatcher.NewAgentIDIterator([]byte{})
		require.NoError(t, err)
		require.Equal(t, 0, iter.Count())

		_, ok := iter.Next()
		require.False(t, ok)
		require.NoError(t, iter.Err())
	})
}
