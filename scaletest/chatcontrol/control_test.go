package chatcontrol_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scaletest/chatcontrol"
)

func TestPrefixAndParsePrompt(t *testing.T) {
	t.Parallel()

	control := chatcontrol.Control{
		ToolCallsThisTurn: 2,
	}
	prompt, err := chatcontrol.PrefixPrompt("Reply with one short sentence.", control)
	require.NoError(t, err)
	require.Contains(t, prompt, chatcontrol.SentinelStart)

	parsed, stripped, found, err := chatcontrol.ParsePrompt(prompt)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Reply with one short sentence.", stripped)
	require.Equal(t, chatcontrol.SchemaVersion, parsed.Version)
	require.Equal(t, 2, parsed.ToolCallsThisTurn)
	require.Equal(t, chatcontrol.DefaultToolName, parsed.Tool)
	require.Equal(t, chatcontrol.DefaultToolCommand, parsed.Command)
}

func TestParsePromptWithoutSentinel(t *testing.T) {
	t.Parallel()

	parsed, stripped, found, err := chatcontrol.ParsePrompt("Continue.")
	require.NoError(t, err)
	require.False(t, found)
	require.Equal(t, chatcontrol.Control{}, parsed)
	require.Equal(t, "Continue.", stripped)
}

func TestToolCallsByTurn(t *testing.T) {
	t.Parallel()

	counts := chatcontrol.ToolCallsByTurn(4, 7, 99)
	require.Len(t, counts, 4)
	require.ElementsMatch(t, []int{2, 2, 2, 1}, counts)

	countsAgain := chatcontrol.ToolCallsByTurn(4, 7, 99)
	require.Equal(t, counts, countsAgain)

	total := 0
	for _, count := range counts {
		total += count
	}
	require.Equal(t, 7, total)
}

func TestDeriveChatSeed(t *testing.T) {
	t.Parallel()

	seedA := chatcontrol.DeriveChatSeed(11, "run-a", "workspace-0", "chat-0")
	seedB := chatcontrol.DeriveChatSeed(11, "run-a", "workspace-0", "chat-0")
	seedC := chatcontrol.DeriveChatSeed(11, "run-a", "workspace-0", "chat-1")

	require.Equal(t, seedA, seedB)
	require.NotEqual(t, seedA, seedC)
}
