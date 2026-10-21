package chattool_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type chatHistoryWindow struct {
	WindowID     string `json:"windowId"`
	BoundaryKind string `json:"boundaryKind"`
}

type chatHistoryResult struct {
	Success bool                `json:"success"`
	Error   string              `json:"error"`
	HasMore bool                `json:"has_more"`
	Windows []chatHistoryWindow `json:"windows"`
}

type chatHistoryFixture struct {
	db   database.Store
	chat database.Chat
}

// newChatHistoryFixtures returns a constructor of fixtures that each own
// a fresh chat in db.
func newChatHistoryFixtures(t *testing.T, db database.Store) func(t *testing.T) chatHistoryFixture {
	t.Helper()
	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:     "test-model",
		CreatedBy: uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy: uuid.NullUUID{UUID: owner.ID, Valid: true},
		IsDefault: true,
	})
	return func(t *testing.T) chatHistoryFixture {
		chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: owner.ID, LastModelConfigID: modelCfg.ID})
		return chatHistoryFixture{db: db, chat: chat}
	}
}

func (f chatHistoryFixture) insert(t *testing.T, role database.ChatMessageRole, visibility database.ChatMessageVisibility, compressed bool, parts ...codersdk.ChatMessagePart) int64 {
	t.Helper()
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return dbgen.ChatMessage(t, f.db, database.ChatMessage{
		ChatID:     f.chat.ID,
		Role:       role,
		Visibility: visibility,
		Compressed: compressed,
		Content:    content,
	}).ID
}

func (f chatHistoryFixture) user(t *testing.T, text string) int64 {
	t.Helper()
	return f.insert(t, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, codersdk.ChatMessageText(text))
}

// boundary inserts a context boundary triplet in the shape chatd commits
// and returns the ID of its window.
func (f chatHistoryFixture) boundary(t *testing.T, toolName, resultJSON string) string {
	t.Helper()
	anchor := f.insert(t, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, true, codersdk.ChatMessageText("boundary anchor"))
	callID := uuid.NewString()
	f.insert(t, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, true, codersdk.ChatMessageToolCall(callID, toolName, json.RawMessage(`{}`)))
	f.insert(t, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, true, codersdk.ChatMessageToolResult(callID, toolName, json.RawMessage(resultJSON), false, false))
	return fmt.Sprintf("w:%d", anchor)
}

func runChatHistory(t *testing.T, tool fantasy.AgentTool, args map[string]any) (chatHistoryResult, string) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	input, err := json.Marshal(args)
	require.NoError(t, err)
	resp, err := tool.Run(ctx, fantasy.ToolCall{Input: string(input)})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)
	var result chatHistoryResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result, resp.Content
}

func (f chatHistoryFixture) tool() fantasy.AgentTool {
	return chattool.ChatHistory(chattool.ChatHistoryOptions{DB: f.db, ChatID: f.chat.ID})
}

func TestChatHistory(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	newFixture := newChatHistoryFixtures(t, db)

	t.Run("ListWindowsOverRealBoundaries", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.user(t, "first request")
		summaryWindow := f.boundary(t, "chat_summarized", `{"summary":"summary text"}`)
		f.user(t, "second request")
		// A handoff call is user-visible and compressed but starts no window.
		f.insert(t, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, true, codersdk.ChatMessageToolCall("handoff", "chat_context_handoff", json.RawMessage(`{}`)))
		rolloverWindow := f.boundary(t, "chat_context_rollover", `{"reason":"model","checkpoint":"goal"}`)
		f.user(t, "third request")
		tool := f.tool()

		result, _ := runChatHistory(t, tool, map[string]any{"action": "list_windows"})
		require.True(t, result.Success)
		require.Equal(t, []chatHistoryWindow{
			{WindowID: "w:0", BoundaryKind: "root"},
			{WindowID: summaryWindow, BoundaryKind: "summary"},
			{WindowID: rolloverWindow, BoundaryKind: "rollover"},
		}, result.Windows)
		require.False(t, result.HasMore)

		result, _ = runChatHistory(t, tool, map[string]any{"action": "list_windows", "recent_first": true, "limit": 1})
		require.Equal(t, []chatHistoryWindow{{WindowID: rolloverWindow, BoundaryKind: "rollover"}}, result.Windows)
		require.True(t, result.HasMore)
	})

	t.Run("PrivacyFloor", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.user(t, "secret before clear")
		f.boundary(t, "chat_summarized", `{"summary":"secret summary"}`)
		clearWindow := f.boundary(t, "chat_cleared", `{"source":"manual"}`)
		f.user(t, "after clear")
		tool := f.tool()

		result, _ := runChatHistory(t, tool, map[string]any{"action": "list_windows"})
		require.Equal(t, []chatHistoryWindow{{WindowID: clearWindow, BoundaryKind: "clear"}}, result.Windows)

		result, _ = runChatHistory(t, tool, map[string]any{"action": "list_windows", "window_id": "w:0"})
		require.False(t, result.Success)
		require.Equal(t, "window_not_found", result.Error)
	})

	t.Run("RejectsUnsupportedArguments", func(t *testing.T) {
		t.Parallel()
		tool := newFixture(t).tool()
		for _, tc := range []struct {
			args map[string]any
			want string
		}{
			{map[string]any{"action": "delete"}, "invalid_action"},
			{map[string]any{"action": "list_windows", "limit": -1}, "invalid_arguments"},
		} {
			got, _ := runChatHistory(t, tool, tc.args)
			require.False(t, got.Success, tc.args)
			require.Equal(t, tc.want, got.Error, tc.args)
		}
	})
}
