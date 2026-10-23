package chattool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func historyContent(t *testing.T, parts ...codersdk.ChatMessagePart) pqtype.NullRawMessage {
	t.Helper()
	raw, err := json.Marshal(parts)
	require.NoError(t, err)
	return pqtype.NullRawMessage{RawMessage: raw, Valid: true}
}

func runChatHistory(ctx context.Context, t *testing.T, tool fantasy.AgentTool, args chattool.ChatHistoryArgs) (fantasy.ToolResponse, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	response, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: chattool.ChatHistoryToolName, Input: string(raw)})
	require.NoError(t, err)
	require.LessOrEqual(t, len(response.Content), chattool.MaxChatHistoryResultBytes)
	var result map[string]any
	if !response.IsError {
		require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
	}
	return response, result
}

func TestChatHistory(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	modelCfg := seedModelConfig(t, db)
	chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: modelCfg.ID})
	other := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: modelCfg.ID})

	insert := func(chatID uuid.UUID, role database.ChatMessageRole, visibility database.ChatMessageVisibility, compressed bool, parts ...codersdk.ChatMessagePart) database.ChatMessage {
		return dbgen.ChatMessage(t, db, database.ChatMessage{
			ChatID:     chatID,
			Role:       role,
			Visibility: visibility,
			Compressed: compressed,
			Content:    historyContent(t, parts...),
		})
	}
	boundary := func(toolName string, result string) {
		insert(chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, true, codersdk.ChatMessageText("anchor"))
		insert(chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, true,
			codersdk.ChatMessageToolCall("b-"+toolName, toolName, json.RawMessage(`{}`)))
		insert(chat.ID, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, true,
			codersdk.ChatMessageToolResult("b-"+toolName, toolName, json.RawMessage(result), false, false))
	}

	// w:0 has a secret, then /clear starts w:1.
	secret := insert(chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, codersdk.ChatMessageText("SECRET before clear"))
	boundary("chat_cleared", `{"source":"manual"}`)
	// w:1 ends with a summary.
	request := insert(chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, codersdk.ChatMessageText("Migrate the billing worker"))
	insert(chat.ID, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, false,
		codersdk.ChatMessageText("Reading files"),
		codersdk.ChatMessageToolCall("c1", "read_file", json.RawMessage(`{"path":"main.go"}`)))
	insert(chat.ID, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, false,
		codersdk.ChatMessageToolResult("c1", "read_file", json.RawMessage(`"package main"`), false, false))
	boundary("chat_summarized", `{"source":"automatic","summary":"s"}`)
	// w:2 holds one large item and ends with new_context.
	big := insert(chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, codersdk.ChatMessageText("Keep the cron path behind a flag. "+strings.Repeat("x", 40000)))
	boundary("chat_summarized", `{"source":"new_context","summary":"s"}`)
	// w:3 is current.
	current := insert(chat.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, codersdk.ChatMessageText("Continue"))
	foreign := insert(other.ID, database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, false, codersdk.ChatMessageText("other chat"))

	tool := chattool.ChatHistory(chattool.ChatHistoryOptions{DB: db, ChatID: chat.ID, ParseContent: chatprompt.ParseContent})

	t.Run("ListWindows", func(t *testing.T) {
		t.Parallel()
		resp, result := runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "list_windows"})
		require.False(t, resp.IsError, resp.Content)
		windows, ok := result["windows"].([]any)
		require.True(t, ok)
		require.Len(t, windows, 4)
		var ended []string
		var counts []float64
		for _, w := range windows {
			entry, ok := w.(map[string]any)
			require.True(t, ok)
			ended = append(ended, entry["ended_by"].(string))
			counts = append(counts, entry["items"].(float64))
		}
		require.Equal(t, []string{"clear", "summary", "new_context", "current"}, ended)
		// Nothing before the newest clear; boundary rows are not items.
		require.Equal(t, []float64{0, 3, 1, 1}, counts)
	})

	t.Run("ListItems", func(t *testing.T) {
		t.Parallel()
		resp, result := runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "list_items", WindowID: "w:1"})
		require.False(t, resp.IsError, resp.Content)
		items := result["items"].([]any)
		require.Len(t, items, 3)
		first := items[0].(map[string]any)
		require.EqualValues(t, request.ID, first["item_id"])
		require.Contains(t, items[1].(map[string]any)["snippet"], "[tool call read_file]")

		resp, result = runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "list_items", WindowID: "w:1", Limit: 1})
		require.False(t, resp.IsError, resp.Content)
		require.Len(t, result["items"], 1)
		require.Equal(t, true, result["has_more"])

		resp, result = runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "list_items", WindowID: "w:1", Role: "tool"})
		require.False(t, resp.IsError, resp.Content)
		require.Len(t, result["items"], 1)

		resp, _ = runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "list_items", WindowID: "w:9"})
		require.True(t, resp.IsError)
	})

	t.Run("Search", func(t *testing.T) {
		t.Parallel()
		resp, result := runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "search", Query: "BILLING"})
		require.False(t, resp.IsError, resp.Content)
		items := result["items"].([]any)
		require.Len(t, items, 1)
		require.EqualValues(t, request.ID, items[0].(map[string]any)["item_id"])
		require.Equal(t, "w:1", items[0].(map[string]any)["window_id"])

		// The floor hides rows before the newest clear from search.
		resp, result = runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "search", Query: "secret"})
		require.False(t, resp.IsError, resp.Content)
		require.Empty(t, result["items"])

		// Snippets are short.
		resp, result = runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "search", Query: "cron path"})
		require.False(t, resp.IsError, resp.Content)
		items = result["items"].([]any)
		require.Len(t, items, 1)
		require.LessOrEqual(t, len(items[0].(map[string]any)["snippet"].(string)), 310)
	})

	t.Run("ReadItem", func(t *testing.T) {
		t.Parallel()
		resp, result := runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "read_item", ItemID: current.ID})
		require.False(t, resp.IsError, resp.Content)
		require.Equal(t, "Continue", result["text"])
		require.Equal(t, "w:3", result["window_id"])
		require.Equal(t, false, result["has_more"])

		// A large item pages and the result stays under 16 KiB.
		resp, result = runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "read_item", ItemID: big.ID, LimitChars: 16000})
		require.False(t, resp.IsError, resp.Content)
		require.Equal(t, true, result["has_more"])
		require.True(t, strings.HasPrefix(result["text"].(string), "Keep the cron path"))
		next := int(result["next_offset_chars"].(float64))
		resp, result = runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "read_item", ItemID: big.ID, OffsetChars: next})
		require.False(t, resp.IsError, resp.Content)
		require.Equal(t, strings.Repeat("x", 8000), result["text"])

		// Foreign, pre-floor and boundary IDs are not found.
		for _, id := range []int64{foreign.ID, secret.ID, 999999999} {
			resp, _ = runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "read_item", ItemID: id})
			require.True(t, resp.IsError)
			require.Contains(t, resp.Content, "item_not_found")
		}
	})
}

func TestChatHistoryListCap(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	modelCfg := seedModelConfig(t, db)
	chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: modelCfg.ID})
	// 25 items with multibyte snippets go over 16 KiB once encoded.
	for range 25 {
		dbgen.ChatMessage(t, db, database.ChatMessage{
			ChatID:  chat.ID,
			Role:    database.ChatMessageRoleUser,
			Content: historyContent(t, codersdk.ChatMessageText(strings.Repeat("\u00e9\u4e16", 400))),
		})
	}
	tool := chattool.ChatHistory(chattool.ChatHistoryOptions{DB: db, ChatID: chat.ID, ParseContent: chatprompt.ParseContent})
	resp, result := runChatHistory(ctx, t, tool, chattool.ChatHistoryArgs{Action: "list_items", WindowID: "w:0", Limit: 25})
	require.False(t, resp.IsError, resp.Content)
	require.Equal(t, true, result["has_more"])
	items := result["items"].([]any)
	require.NotEmpty(t, items)
	require.Less(t, len(items), 25)
}
