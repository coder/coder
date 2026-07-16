package chatd //nolint:testpackage // Asserts unexported status tables and lineage hashing invariants.

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// TestExecutionStatusMirrorsDatabaseEnum pins the hand-mirrored
// chattool.ExecutionStatus values to the generated database enum so
// a new enum value or a typo fails here instead of at a live
// Postgres commit.
func TestExecutionStatusMirrorsDatabaseEnum(t *testing.T) {
	t.Parallel()

	toolStatuses := []chattool.ExecutionStatus{
		chattool.ExecutionStatusReserved,
		chattool.ExecutionStatusStarting,
		chattool.ExecutionStatusRunning,
		chattool.ExecutionStatusExited,
		chattool.ExecutionStatusDetached,
		chattool.ExecutionStatusCancelRequested,
		chattool.ExecutionStatusCanceled,
		chattool.ExecutionStatusUnknown,
		chattool.ExecutionStatusNoEffect,
	}
	dbValues := database.AllChatToolCallExecutionStatusValues()
	require.Len(t, toolStatuses, len(dbValues), "chattool.ExecutionStatus and the database enum must declare the same values")

	dbSet := make(map[string]struct{}, len(dbValues))
	for _, v := range dbValues {
		dbSet[string(v)] = struct{}{}
	}
	for _, v := range toolStatuses {
		require.Contains(t, dbSet, string(v))
	}

	for status, sources := range terminalObservationSources {
		require.Contains(t, dbSet, string(status))
		for _, source := range sources {
			require.Contains(t, dbSet, string(source))
		}
	}
}

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
