package chattool_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

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
	"github.com/coder/quartz"
)

type chatHistoryItem struct {
	ItemID          string `json:"itemId"`
	WindowID        string `json:"windowId"`
	Role            string `json:"role"`
	StartCharOffset int    `json:"startCharOffset"`
	Text            string `json:"text"`
	NextCharOffset  *int   `json:"nextCharOffset"`
}

type chatHistoryWindow struct {
	WindowID     string `json:"windowId"`
	BoundaryKind string `json:"boundaryKind"`
}

type chatHistoryResult struct {
	Success   bool                `json:"success"`
	Error     string              `json:"error"`
	HasMore   bool                `json:"has_more"`
	Truncated bool                `json:"truncated"`
	Items     []chatHistoryItem   `json:"items"`
	Windows   []chatHistoryWindow `json:"windows"`
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
	return chattool.ChatHistory(chattool.ChatHistoryOptions{DB: f.db, ChatID: f.chat.ID, ParseContent: chatprompt.ParseContent})
}

func itemTexts(items []chatHistoryItem) []string {
	texts := make([]string, 0, len(items))
	for _, item := range items {
		texts = append(texts, item.Text)
	}
	return texts
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

		result, _ = runChatHistory(t, tool, map[string]any{"action": "list_items", "window_id": summaryWindow, "role": "user"})
		require.Equal(t, []string{"second request"}, itemTexts(result.Items))
		require.Equal(t, summaryWindow, result.Items[0].WindowID)
	})

	t.Run("PrivacyFloor", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		secret := f.user(t, "secret before clear")
		f.boundary(t, "chat_summarized", `{"summary":"secret summary"}`)
		clearWindow := f.boundary(t, "chat_cleared", `{"source":"manual"}`)
		f.user(t, "after clear")
		tool := f.tool()

		result, _ := runChatHistory(t, tool, map[string]any{"action": "search", "query": "secret"})
		require.True(t, result.Success)
		require.Empty(t, result.Items)

		result, _ = runChatHistory(t, tool, map[string]any{"action": "list_windows"})
		require.Equal(t, []chatHistoryWindow{{WindowID: clearWindow, BoundaryKind: "clear"}}, result.Windows)

		result, _ = runChatHistory(t, tool, map[string]any{"action": "read_item", "item_id": fmt.Sprint(secret)})
		require.False(t, result.Success)
		require.Equal(t, "item_not_found", result.Error)

		result, _ = runChatHistory(t, tool, map[string]any{"action": "list_items", "window_id": "w:0"})
		require.False(t, result.Success)
		require.Equal(t, "window_not_found", result.Error)

		result, _ = runChatHistory(t, tool, map[string]any{"action": "list_items", "role": "user"})
		require.Equal(t, []string{"after clear"}, itemTexts(result.Items))
	})

	t.Run("HasMore", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		for _, text := range []string{"one", "two", "three"} {
			f.user(t, text)
		}
		tool := f.tool()

		result, _ := runChatHistory(t, tool, map[string]any{"action": "list_items", "limit": 2})
		require.Equal(t, []string{"one", "two"}, itemTexts(result.Items))
		require.True(t, result.HasMore)

		result, _ = runChatHistory(t, tool, map[string]any{"action": "list_items", "limit": 3})
		require.Len(t, result.Items, 3)
		require.False(t, result.HasMore)

		result, _ = runChatHistory(t, tool, map[string]any{"action": "list_items", "limit": 1, "recent_first": true})
		require.Equal(t, []string{"three"}, itemTexts(result.Items))
		require.True(t, result.HasMore)
	})

	t.Run("ReadItemPaging", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		text := "abcd\U0001F600efgh"
		id := f.user(t, text)
		tool := f.tool()

		var pages []string
		offset := 0
		for {
			result, _ := runChatHistory(t, tool, map[string]any{"action": "read_item", "item_id": fmt.Sprint(id), "offset_chars": offset, "limit_chars": 5})
			require.True(t, result.Success)
			require.Len(t, result.Items, 1)
			item := result.Items[0]
			require.Equal(t, offset, item.StartCharOffset)
			pages = append(pages, item.Text)
			if item.NextCharOffset == nil {
				break
			}
			offset = *item.NextCharOffset
		}
		// The first page stops before the surrogate pair instead of splitting it.
		require.Equal(t, []string{"abcd", "\U0001F600efg", "h"}, pages)

		// A manual offset inside the pair rounds back to include it.
		result, _ := runChatHistory(t, tool, map[string]any{"action": "read_item", "item_id": fmt.Sprint(id), "offset_chars": 5, "limit_chars": 3})
		require.Equal(t, 4, result.Items[0].StartCharOffset)
		require.Equal(t, "\U0001F600e", result.Items[0].Text)
	})

	t.Run("LiteralCaseInsensitiveSearch", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.user(t, "The PRICE is $5.00 (approx)")
		f.user(t, "the price is 5500 approx")
		long := strings.Repeat("x", 2000) + "needle" + strings.Repeat("y", 2000)
		f.user(t, long)
		tool := f.tool()

		result, _ := runChatHistory(t, tool, map[string]any{"action": "search", "query": "price IS $5.00 ("})
		require.Equal(t, []string{"The PRICE is $5.00 (approx)"}, itemTexts(result.Items))

		result, _ = runChatHistory(t, tool, map[string]any{"action": "search", "query": "NEEDLE"})
		require.Len(t, result.Items, 1)
		item := result.Items[0]
		require.Len(t, item.Text, 500)
		require.Equal(t, strings.Index(long, "needle")-120, item.StartCharOffset)
		require.Contains(t, item.Text, "needle")
	})

	t.Run("ProjectionOmitsReasoningAndNestedHistory", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		assistant := f.insert(t, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, false,
			codersdk.ChatMessageReasoning("private reasoning"),
			codersdk.ChatMessageText("visible answer"),
			codersdk.ChatMessageFile(uuid.New(), "image/png", "diagram.png"),
			codersdk.ChatMessageToolCall("call", "execute", json.RawMessage(`{"command":"`+strings.Repeat("a", 3000)+`"}`)),
		)
		results := f.insert(t, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, false,
			codersdk.ChatMessageToolResult("call", "execute", json.RawMessage(`{"output":"done"}`), false, false),
			codersdk.ChatMessageToolResult("history", chattool.ChatHistoryToolName, json.RawMessage(`{"items":[{"text":"recursive"}]}`), false, false),
			codersdk.ChatMessageToolResult("shot", "computer", json.RawMessage(`{"data":"`+strings.Repeat("QUJD", 1000)+`","mime_type":"image/png","text":"Screenshot taken"}`), false, true),
		)
		tool := f.tool()

		result, _ := runChatHistory(t, tool, map[string]any{"action": "read_item", "item_id": fmt.Sprint(assistant), "limit_chars": 16000})
		require.Len(t, result.Items, 1)
		text := result.Items[0].Text
		require.Contains(t, text, "visible answer")
		require.Contains(t, text, "execute")
		require.Contains(t, text, "[file: diagram.png]")
		require.NotContains(t, text, "private reasoning")
		require.Less(t, len(text), 3000, "tool call arguments are truncated")

		result, _ = runChatHistory(t, tool, map[string]any{"action": "read_item", "item_id": fmt.Sprint(results)})
		text = result.Items[0].Text
		require.Contains(t, text, `{"output":"done"}`)
		require.NotContains(t, text, "recursive")
		require.Contains(t, text, "[tool result: computer] [media: image/png] Screenshot taken")
		require.NotContains(t, text, "QUJD", "media results keep only their annotation")

		result, _ = runChatHistory(t, tool, map[string]any{"action": "search", "query": "recursive"})
		require.Empty(t, result.Items)
	})

	t.Run("ToolNameFilter", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.user(t, "run the build")
		call := f.insert(t, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, false,
			codersdk.ChatMessageToolCall("build", "execute", json.RawMessage(`{"command":"make"}`)))
		result := f.insert(t, database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, false,
			codersdk.ChatMessageToolResult("build", "execute", json.RawMessage(`{"output":"ok"}`), false, false))
		f.insert(t, database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, false,
			codersdk.ChatMessageToolCall("read", "read_file", json.RawMessage(`{"path":"Makefile"}`)))
		tool := f.tool()

		itemIDs := func(items []chatHistoryItem) []string {
			ids := make([]string, 0, len(items))
			for _, item := range items {
				ids = append(ids, item.ItemID)
			}
			return ids
		}
		got, _ := runChatHistory(t, tool, map[string]any{"action": "list_items", "tool_name": "execute"})
		require.Equal(t, []string{fmt.Sprint(call), fmt.Sprint(result)}, itemIDs(got.Items))

		got, _ = runChatHistory(t, tool, map[string]any{"action": "list_items", "tool_name": "execute", "role": "assistant"})
		require.Equal(t, []string{fmt.Sprint(call)}, itemIDs(got.Items), "role and tool_name combine with AND")
	})

	t.Run("RejectsUnsupportedArguments", func(t *testing.T) {
		t.Parallel()
		tool := newFixture(t).tool()
		for _, tc := range []struct {
			args map[string]any
			want string
		}{
			{map[string]any{"action": "delete"}, "invalid_action"},
			{map[string]any{"action": "search", "query": "  "}, "query_required"},
			{map[string]any{"action": "read_item"}, "item_id_required"},
			{map[string]any{"action": "read_item", "item_id": "1", "role": "user"}, "filters_unsupported"},
			{map[string]any{"action": "list_windows", "tool_name": "execute"}, "filters_unsupported"},
			{map[string]any{"action": "list_items", "limit": -1}, "invalid_arguments"},
		} {
			got, _ := runChatHistory(t, tool, tc.args)
			require.False(t, got.Success, tc.args)
			require.Equal(t, tc.want, got.Error, tc.args)
		}

		// The query limit counts characters, not UTF-8 bytes.
		got, _ := runChatHistory(t, tool, map[string]any{"action": "search", "query": strings.Repeat("界", 600)})
		require.True(t, got.Success, got.Error)
		got, _ = runChatHistory(t, tool, map[string]any{"action": "search", "query": strings.Repeat("界", 1025)})
		require.Equal(t, "invalid_arguments", got.Error)
	})

	t.Run("ResultCap", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		// JSON escapes '<' as six bytes, so the cap must count encoded bytes.
		big := strings.Repeat("<", 40_000)
		for range 5 {
			f.user(t, big)
		}
		tool := f.tool()

		result, raw := runChatHistory(t, tool, map[string]any{"action": "list_items", "max_chars_per_item": 16000, "limit": 25})
		require.LessOrEqual(t, len(raw), 16*1024)
		require.True(t, result.HasMore)

		result, raw = runChatHistory(t, tool, map[string]any{"action": "read_item", "item_id": result.Items[0].ItemID, "limit_chars": 16000})
		require.LessOrEqual(t, len(raw), 16*1024)
		require.True(t, result.Truncated)
		item := result.Items[0]
		require.NotEmpty(t, item.Text)
		require.NotNil(t, item.NextCharOffset)
		require.Equal(t, item.StartCharOffset+len(utf16.Encode([]rune(item.Text))), *item.NextCharOffset)
	})

	t.Run("ScanByteBudget", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		// One row over the 32 MiB scan budget is never parsed.
		huge := &hugeRowStore{Store: db, size: 32*1024*1024 + 1}
		var parsed int
		tool := chattool.ChatHistory(chattool.ChatHistoryOptions{DB: huge, ChatID: f.chat.ID, ParseContent: func(database.ChatMessage) ([]codersdk.ChatMessagePart, error) {
			parsed++
			return []codersdk.ChatMessagePart{codersdk.ChatMessageText("huge row")}, nil
		}})

		result, _ := runChatHistory(t, tool, map[string]any{"action": "search", "query": "huge"})
		require.True(t, result.Success)
		require.Empty(t, result.Items)
		require.True(t, result.HasMore)
		require.Zero(t, parsed)
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.user(t, "a row")
		clock := quartz.NewMock(t)
		slow := &advancingStore{Store: db, clock: clock}
		tool := chattool.ChatHistory(chattool.ChatHistoryOptions{DB: slow, ChatID: f.chat.ID, ParseContent: chatprompt.ParseContent, Clock: clock})

		result, _ := runChatHistory(t, tool, map[string]any{"action": "search", "query": "missing"})
		require.False(t, result.Success)
		require.Equal(t, "history_timeout", result.Error)
	})
}

// advancingStore makes every boundary read take longer than the
// chat_history deadline.
type advancingStore struct {
	database.Store
	clock *quartz.Mock
}

func (s *advancingStore) GetChatContextBoundaryMessages(ctx context.Context, chatID uuid.UUID) ([]database.GetChatContextBoundaryMessagesRow, error) {
	s.clock.Advance(11 * time.Second)
	return s.Store.GetChatContextBoundaryMessages(ctx, chatID)
}

// hugeRowStore returns one user row of the given content size.
type hugeRowStore struct {
	database.Store
	size int
}

func (s *hugeRowStore) GetChatMessagesByChatIDAscPaginated(_ context.Context, arg database.GetChatMessagesByChatIDAscPaginatedParams) ([]database.ChatMessage, error) {
	if arg.AfterID > 0 {
		return nil, nil
	}
	return []database.ChatMessage{{
		ID:         1,
		ChatID:     arg.ChatID,
		Role:       database.ChatMessageRoleUser,
		Visibility: database.ChatMessageVisibilityBoth,
		Content:    pqtype.NullRawMessage{RawMessage: make([]byte, s.size), Valid: true},
	}}, nil
}
