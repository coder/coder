package chatsanitize

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicProviderToolViolationLogDetails(t *testing.T) {
	t.Parallel()

	violations := make([]AnthropicProviderToolHistoryViolation, 33)
	for i := range violations {
		violations[i] = AnthropicProviderToolHistoryViolation{
			MessageIndex: i,
			PartIndex:    i + 1,
			ID:           "ws-detail",
			Reason:       "test_reason",
		}
	}
	details, truncated := anthropicProviderToolViolationLogDetails(violations)
	require.True(t, truncated)
	require.Len(t, details, maxAnthropicProviderToolViolationLogDetails)
	require.Len(t, details[0], 4)
	require.Equal(t, 0, details[0]["message_index"])
	require.Equal(t, 1, details[0]["part_index"])
	require.Equal(t, "ws-detail", details[0]["id"])
	require.Equal(t, "test_reason", details[0]["reason"])
}
