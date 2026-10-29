package chatd

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestSavedWorkspaceAwarenessTextsUnchanged(t *testing.T) {
	t.Parallel()

	// SHA-256 of each awareness text that existing chats store. Changing a
	// saved text would leave it in those chats' prompts.
	want := []string{
		"34ecbd60e31e3827c68d94182e594da9bbd1422ea987fd6c1ca10ad914719b0e",
		"6d213dcff2d3cdd36f681e77f3d818f03288848201412fa19c712d648f59aaad",
		"c6fbbbd83ae9b02594189035aeb5124e006f043b63a3a67d25ad457c235a337b",
		"7604aa42276c05d45f021277f2ec1da725f6a06eb7aaad90016fb2cdf8b54867",
		"0ca2a20d17b32191e430366fdfcd61432f4bc66ce2345325a9489c31c8e48b7c",
		"bbe72d3a48732ecaab09a5d94701a313b3c0547cb8bf99d21a4ffed2ee477d81",
		"33f7e1e009dc0bcefdcb9c257aad6fafb60c60e60813a5a57642fc044a75c102",
	}
	got := make([]string, 0, len(savedWorkspaceAwarenessTexts))
	for text := range savedWorkspaceAwarenessTexts {
		sum := sha256.Sum256([]byte(text))
		got = append(got, hex.EncodeToString(sum[:]))
	}
	require.ElementsMatch(t, want, got)
}

func TestWithoutSavedWorkspaceAwareness(t *testing.T) {
	t.Parallel()

	system := database.ChatMessageRoleSystem
	rows := []database.ChatMessage{textMessage(t, 1, system, "Deployment prompt.")}
	for text := range savedWorkspaceAwarenessTexts {
		rows = append(rows, textMessage(t, int64(len(rows)+1), system, text))
	}
	kept := []database.ChatMessage{
		rows[0],
		textMessage(t, 100, system, savedWorkspaceAttachedAwareness+" Never run sudo."),
		textMessage(t, 101, system, savedWorkspaceAttachedAwareness, "Never run sudo."),
		textMessage(t, 102, database.ChatMessageRoleUser, savedWorkspaceAttachedAwareness),
	}
	rows = append(rows, kept[1:]...)
	input := slices.Clone(rows)

	require.Equal(t, kept, withoutSavedWorkspaceAwareness(rows))
	require.Equal(t, input, rows, "input rows must not change")
}
