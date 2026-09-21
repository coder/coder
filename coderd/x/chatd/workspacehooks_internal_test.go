package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// TestWorkspaceHookResults proves the split the prototype relies on:
// model_context from a workspace hook becomes a model-only transcript
// row and user_message a user-visible notice, while the tool result
// text stays untouched. PROTOTYPE (CODAGT-1083).
func TestWorkspaceHookResults(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil)
	metadata, err := json.Marshal(map[string]any{
		"workspace_hooks": []workspacesdk.HookDecision{
			{Hook: "silent", Decision: "allow"},
			{Hook: "reminder", Decision: "allow", ModelContext: "run make gen", UserMessage: "reminded about make gen"},
		},
	})
	require.NoError(t, err)
	content := []fantasy.Content{
		fantasy.TextContent{Text: "not a tool result"},
		fantasy.ToolResultContent{
			ToolCallID:     "toolu_1",
			ToolName:       "execute",
			Result:         fantasy.ToolResultOutputContentText{Text: `{"success":true}`},
			ClientMetadata: string(metadata),
		},
		fantasy.ToolResultContent{
			ToolCallID:     "toolu_2",
			ToolName:       "read_file",
			Result:         fantasy.ToolResultOutputContentText{Text: "no hooks"},
			ClientMetadata: `{"attachments":[]}`,
		},
		fantasy.ToolResultContent{
			ToolCallID:     "toolu_3",
			ToolName:       "write_file",
			ClientMetadata: `not json`,
		},
	}

	results := workspaceHookResults(context.Background(), logger, content)
	require.Equal(t, []*chathooks.Result{{ModelContext: "run make gen", UserMessage: "reminded about make gen"}}, results)

	modelConfigID := uuid.New()
	rows, err := chathooks.EventMessagesForResults(results, modelConfigID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, database.ChatMessageRoleUser, rows[0].Role)
	require.Equal(t, database.ChatMessageVisibilityModel, rows[0].Visibility)
	require.Contains(t, string(rows[0].Content.RawMessage), "run make gen")
	require.Equal(t, database.ChatMessageRoleSystem, rows[1].Role)
	require.Equal(t, database.ChatMessageVisibilityUser, rows[1].Visibility)
	require.Contains(t, string(rows[1].Content.RawMessage), "reminded about make gen")
}
