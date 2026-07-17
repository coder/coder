package chatd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// TestExecutionIntentHashRoundTrip pins the byte-preservation
// property the input_sha256 lineage guard depends on: the persisted
// tool-call args must come back from ParseContent byte-identical,
// or every replayed claim would fail closed with
// ErrExecutionInputMismatch.
func TestExecutionIntentHashRoundTrip(t *testing.T) {
	t.Parallel()

	// Non-alphabetical key order must survive the round trip.
	input := `{"workdir":"/tmp","command":"echo hi"}`
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: "call-1",
		ToolName:   chattool.ExecuteToolName,
		Args:       json.RawMessage(input),
	}})
	require.NoError(t, err)

	parts, err := chatprompt.ParseContent(database.ChatMessage{
		Role:           database.ChatMessageRoleAssistant,
		Content:        content,
		ContentVersion: chatprompt.CurrentContentVersion,
	})
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, input, string(parts[0].Args))
	require.Equal(t, chattool.HashToolInput(input), chattool.HashToolInput(string(parts[0].Args)))
}
