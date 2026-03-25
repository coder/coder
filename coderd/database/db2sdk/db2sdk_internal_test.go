package db2sdk

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestAggregateTokenMetadata(t *testing.T) {
	t.Parallel()

	t.Run("empty_input", func(t *testing.T) {
		t.Parallel()
		result := aggregateTokenMetadata(nil)
		require.Empty(t, result)
	})

	t.Run("sums_across_rows", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"cache_read_tokens":100,"reasoning_tokens":50}`),
					Valid:      true,
				},
			},
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"cache_read_tokens":200,"reasoning_tokens":75}`),
					Valid:      true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		require.Equal(t, int64(300), result["cache_read_tokens"])
		require.Equal(t, int64(125), result["reasoning_tokens"])
		require.Len(t, result, 2)
	})

	t.Run("skips_null_and_invalid_metadata", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID:       uuid.New(),
				Metadata: pqtype.NullRawMessage{Valid: false},
			},
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: nil,
					Valid:      true,
				},
			},
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"tokens":42}`),
					Valid:      true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		require.Equal(t, int64(42), result["tokens"])
		require.Len(t, result, 1)
	})

	t.Run("skips_non_integer_values", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					// Float values fail json.Number.Int64(), so they
					// are silently dropped.
					RawMessage: json.RawMessage(`{"good":10,"fractional":1.5}`),
					Valid:      true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		require.Equal(t, int64(10), result["good"])
		_, hasFractional := result["fractional"]
		require.False(t, hasFractional)
	})

	t.Run("skips_malformed_json", func(t *testing.T) {
		t.Parallel()
		tokens := []database.AIBridgeTokenUsage{
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`not json`),
					Valid:      true,
				},
			},
			{
				ID: uuid.New(),
				Metadata: pqtype.NullRawMessage{
					RawMessage: json.RawMessage(`{"tokens":5}`),
					Valid:      true,
				},
			},
		}

		result := aggregateTokenMetadata(tokens)
		// The malformed row is skipped, the valid one is counted.
		require.Equal(t, int64(5), result["tokens"])
		require.Len(t, result, 1)
	})
}
