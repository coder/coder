package agentnative

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeEmptyAssistantReceiptCanGrowAfterRestart(t *testing.T) {
	t.Parallel()
	id := func() string { return "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:26] }
	receipts, err := readOpenCodeReceipts(nil)
	require.NoError(t, err)
	var page openCodeMessagePage
	inputID, assistantID := id(), id()
	decode := func(text string) {
		data := `{"data":[{"id":"` + assistantID + `","type":"assistant","content":` + text + `},{"id":"` + inputID + `","type":"synthetic","text":"continue"}],"cursor":{}}`
		require.NoError(t, json.Unmarshal([]byte(data), &page))
	}
	decode(`[]`)
	_, _, err = receipts.project(page, openCodeLiveSnapshot)
	require.NoError(t, err)
	state, err := json.Marshal(receipts)
	require.NoError(t, err)
	restored, err := readOpenCodeReceipts(state)
	require.NoError(t, err)
	decode(`[{"type":"text","text":"new text"}]`)
	events, _, err := restored.project(page, openCodeResumeSnapshot)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "turn.start", events[0].Type)
	require.Equal(t, "", events[0].Data["text"])
	require.Equal(t, "text", events[1].Type)
	require.Equal(t, "new text", events[1].Data["text"])
}

func TestOpenCodeProjectionChunksUnicodeWithinJournalBound(t *testing.T) {
	t.Parallel()
	receipts, err := readOpenCodeReceipts(nil)
	require.NoError(t, err)
	text := strings.Repeat("x", 15999) + strings.Repeat("🌏", 20000) + strings.Repeat("\x00", 20000)
	userID := "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:26]
	assistantID := "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:26]
	encoded, err := json.Marshal(map[string]any{"data": []any{
		map[string]any{"id": assistantID, "type": "assistant", "content": []any{map[string]any{"type": "text", "text": text}}},
		map[string]any{"id": userID, "type": "user", "text": text},
	}, "cursor": map[string]any{}})
	require.NoError(t, err)
	var page openCodeMessagePage
	require.NoError(t, json.Unmarshal(encoded, &page))
	events, _, err := receipts.project(page, openCodeLiveSnapshot)
	require.NoError(t, err)
	var userText, assistantText strings.Builder
	for _, event := range events {
		encoded, err := json.Marshal(event)
		require.NoError(t, err)
		require.LessOrEqual(t, len(encoded)+32, 128<<10)
		switch event.Type {
		case "turn.start":
			_, _ = userText.WriteString(event.Data["text"].(string))
		case "text":
			_, _ = assistantText.WriteString(event.Data["text"].(string))
		}
	}
	require.Equal(t, text, userText.String())
	require.Equal(t, text, assistantText.String())
}
