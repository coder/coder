package chatd //nolint:testpackage // Exercises unexported generation helpers.

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCompactionMetricIdentity(t *testing.T) {
	t.Parallel()

	compaction := &generationCompaction{
		Options: chatloop.GenerateCompactionOptions{
			Model: &chattest.FakeModel{ProviderName: "anthropic", ModelName: "claude-sonnet-4-5"},
		},
	}

	provider, model := compactionMetricIdentity(compaction)
	require.Equal(t, "anthropic", provider)
	require.Equal(t, "claude-sonnet-4-5", model)

	// With an override, metrics use the prepare-time identity, not the
	// chat model carried by the options.
	compaction.Override = &resolvedCompactionOverride{
		ResolvedProvider: "openai",
		ResolvedModel:    "gpt-4.1-mini",
	}
	provider, model = compactionMetricIdentity(compaction)
	require.Equal(t, "openai", provider)
	require.Equal(t, "gpt-4.1-mini", model)
}

func TestGenerationCompactionContextLimit(t *testing.T) {
	t.Parallel()

	require.EqualValues(t, 0, generationCompactionContextLimit(nil))

	// The decision path must see the prepare-time compaction limit (the
	// stricter of the chat and override models' limits), not the chat
	// model's limit.
	compaction := &generationCompaction{
		Options: chatloop.GenerateCompactionOptions{ContextLimit: 50_000},
	}
	require.EqualValues(t, 50_000, generationCompactionContextLimit(compaction))
}

// TestUnresolvedToolCallsPartialResults asserts that a partially
// committed tool step (an aborted batch persisted its completed
// sibling results) leaves only the result-less calls unresolved, so
// the retry never re-runs a sibling whose side effect already
// happened.
func TestUnresolvedToolCallsPartialResults(t *testing.T) {
	t.Parallel()

	messageFromParts := func(id int64, role database.ChatMessageRole, parts []codersdk.ChatMessagePart) database.ChatMessage {
		content, err := chatprompt.MarshalParts(parts)
		require.NoError(t, err)
		return database.ChatMessage{
			ID:             id,
			Role:           role,
			Content:        content,
			ContentVersion: chatprompt.CurrentContentVersion,
		}
	}

	messages := []database.ChatMessage{
		messageFromParts(1, database.ChatMessageRoleAssistant, []codersdk.ChatMessagePart{
			{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "call-exec", ToolName: "execute", Args: json.RawMessage(`{"command":"echo hi"}`)},
			{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "call-edit", ToolName: "edit_files", Args: json.RawMessage(`{}`)},
		}),
		messageFromParts(2, database.ChatMessageRoleTool, []codersdk.ChatMessagePart{
			{Type: codersdk.ChatMessagePartTypeToolResult, ToolCallID: "call-edit", ToolName: "edit_files", Result: json.RawMessage(`"ok"`)},
		}),
	}

	msgID, localCalls, dynamicCalls, err := unresolvedToolCallsFromHistory(messages, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), msgID)
	require.Empty(t, dynamicCalls)
	require.Len(t, localCalls, 1)
	require.Equal(t, "call-exec", localCalls[0].ToolCallID)
}

func TestRecordGenerationFinishFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		wantRecorded bool
	}{
		{
			name:         "TerminalFailureRecordsError",
			err:          normalizeTaskTransitionError(chatstate.ErrTransitionNotAllowed, "finish generation error"),
			wantRecorded: true,
		},
		{
			name:         "ExpectedExitSkips",
			err:          normalizeTaskTransitionError(errTaskExpectedExit, "finish generation error"),
			wantRecorded: false,
		},
		{
			name:         "RetryableSkips",
			err:          normalizeTaskTransitionError(xerrors.New("transient infrastructure failure"), "finish generation error"),
			wantRecorded: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			turn := newRunnerDebugTurn(testutil.Context(t, testutil.WaitShort), testutil.Logger(t))
			recordGenerationFinishFailure(turn, tt.err)
			require.Equal(t, tt.wantRecorded, turn.statusSet)
			if tt.wantRecorded {
				require.Equal(t, chatdebug.StatusError, turn.status)
			}
		})
	}
}
