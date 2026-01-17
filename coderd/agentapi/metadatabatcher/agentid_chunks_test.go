package metadatabatcher_test

import (
	"encoding/base64"
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
			chunks, err := metadatabatcher.EncodeAgentIDChunks(tt.agentIDs)
			require.NoError(t, err)

			// Decode all chunks and collect the agent IDs.
			var decoded []uuid.UUID
			for _, chunk := range chunks {
				for i := 0; i < len(chunk); i += metadatabatcher.UUIDBase64Size {
					var u uuid.UUID
					_, err := base64.RawStdEncoding.Decode(u[:], chunk[i:i+metadatabatcher.UUIDBase64Size])
					require.NoError(t, err)
					decoded = append(decoded, u)
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

	// Create 600 agents (should split into 2 chunks: 363 + 237).
	agentIDs := make([]uuid.UUID, 600)
	for i := range agentIDs {
		agentIDs[i] = uuid.New()
	}

	chunks, err := metadatabatcher.EncodeAgentIDChunks(agentIDs)
	require.NoError(t, err)
	require.Len(t, chunks, 2)

	// First chunk should have 363 IDs (363 * 22 = 7986 bytes).
	require.Equal(t, 363*22, len(chunks[0]))

	// Second chunk should have 237 IDs (237 * 22 = 5214 bytes).
	require.Equal(t, 237*22, len(chunks[1]))

	// Each chunk should be under 8KB.
	for i, chunk := range chunks {
		require.LessOrEqual(t, len(chunk), 8000, "chunk %d exceeds 8KB limit", i)
	}
}
