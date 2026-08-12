package chatloop

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolResultByteBudget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		contextLimit int64
		want         int
	}{
		{name: "Unknown", contextLimit: 0, want: defaultToolResultBytes},
		{name: "Negative", contextLimit: -1, want: defaultToolResultBytes},
		{name: "BelowFloor", contextLimit: 1000, want: minToolResultBytes},
		{
			name:         "LargeWindow",
			contextLimit: 200_000,
			want:         200_000 / toolResultContextDivisor * bytesPerTokenEstimate,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, toolResultByteBudget(tt.contextLimit))
		})
	}

	t.Run("NeverBelowFloor", func(t *testing.T) {
		t.Parallel()
		for limit := int64(1); limit <= 200_000; limit += 137 {
			assert.GreaterOrEqual(t, toolResultByteBudget(limit), minToolResultBytes)
		}
	})
}

func TestTruncateToolResultText(t *testing.T) {
	t.Parallel()

	t.Run("UnderLimitUnchanged", func(t *testing.T) {
		t.Parallel()
		in := "small output"
		out, truncated := truncateToolResultText(in, 1024)
		assert.False(t, truncated)
		assert.Equal(t, in, out)
	})

	t.Run("ExactlyAtLimitUnchanged", func(t *testing.T) {
		t.Parallel()
		in := strings.Repeat("a", 1024)
		out, truncated := truncateToolResultText(in, 1024)
		assert.False(t, truncated)
		assert.Equal(t, in, out)
	})

	t.Run("ZeroBudgetUnchanged", func(t *testing.T) {
		t.Parallel()
		in := strings.Repeat("a", 1024)
		out, truncated := truncateToolResultText(in, 0)
		assert.False(t, truncated)
		assert.Equal(t, in, out)
	})

	t.Run("PreservesHeadAndTail", func(t *testing.T) {
		t.Parallel()
		in := strings.Repeat("A", 1000) + "MIDDLE" + strings.Repeat("B", 1000)
		const maxBytes = 600
		out, truncated := truncateToolResultText(in, maxBytes)
		require.True(t, truncated)
		assert.LessOrEqual(t, len(out), maxBytes)
		assert.True(t, utf8.ValidString(out))
		assert.True(t, strings.HasPrefix(out, strings.Repeat("A", 100)))
		assert.True(t, strings.HasSuffix(out, strings.Repeat("B", 100)))
		assert.NotContains(t, out, "MIDDLE")
		assert.Contains(t, out, "truncated")
	})

	t.Run("MultibyteStaysValid", func(t *testing.T) {
		t.Parallel()
		// Each rune is 3 bytes, so cuts routinely land mid-rune.
		in := strings.Repeat("界", 1000)
		const maxBytes = 600
		out, truncated := truncateToolResultText(in, maxBytes)
		require.True(t, truncated)
		assert.LessOrEqual(t, len(out), maxBytes)
		assert.True(t, utf8.ValidString(out), "truncated output must be valid UTF-8")
	})

	t.Run("TinyBudgetHardCut", func(t *testing.T) {
		t.Parallel()
		in := strings.Repeat("界", 10) // 30 bytes
		const maxBytes = 10
		out, truncated := truncateToolResultText(in, maxBytes)
		require.True(t, truncated)
		assert.LessOrEqual(t, len(out), maxBytes)
		assert.True(t, utf8.ValidString(out))
	})
}
