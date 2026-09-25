package chatd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

type searchChatMessagesResult struct {
	ChatID       string `json:"chat_id"`
	Query        string `json:"query"`
	Returned     int    `json:"returned"`
	HasMore      bool   `json:"has_more"`
	NextBeforeID int64  `json:"next_before_id"`
	Matches      []struct {
		ID        int64  `json:"id"`
		CreatedAt string `json:"created_at"`
		Role      string `json:"role"`
		Excerpt   string `json:"excerpt"`
		ToolName  string `json:"tool_name"`
	} `json:"matches"`
}

func runSearchChatMessagesTool(
	ctx context.Context,
	t *testing.T,
	server *Server,
	caller database.Chat,
	args any,
) fantasy.ToolResponse {
	t.Helper()

	tool := server.searchChatMessagesTool(func() database.Chat { return caller })
	require.Equal(t, searchChatMessagesToolName, tool.Info().Name)

	input, err := json.Marshal(args)
	require.NoError(t, err)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    uuid.NewString(),
		Name:  searchChatMessagesToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func decodeSearchChatMessagesResult(t *testing.T, resp fantasy.ToolResponse) searchChatMessagesResult {
	t.Helper()
	require.False(t, resp.IsError, "unexpected tool error: %s", resp.Content)
	var result searchChatMessagesResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result
}

func insertSearchTestMessage(
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	role database.ChatMessageRole,
	text string,
) database.ChatMessage {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{{"type": "text", "text": text}})
	require.NoError(t, err)
	return dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:  chatID,
		Role:    role,
		Content: pqtype.NullRawMessage{RawMessage: raw, Valid: true},
	})
}

type searchToolFixture struct {
	server *Server
	db     database.Store
	ctx    context.Context
	parent database.Chat
	child  database.Chat
	other  database.Chat
}

func newSearchToolFixture(t *testing.T) searchToolFixture {
	t.Helper()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	parent := createInternalParentChat(ctx, t, server, db, org.ID, user.ID, model.ID, "search-parent-"+uuid.NewString())
	child, err := server.createChildSubagentChatWithOptions(ctx, parent, "child prompt", "", childSubagentChatOptions{})
	require.NoError(t, err)
	other := createInternalParentChat(ctx, t, server, db, org.ID, user.ID, model.ID, "search-other-"+uuid.NewString())

	return searchToolFixture{server: server, db: db, ctx: ctx, parent: parent, child: child, other: other}
}

func TestSearchChatMessagesTool_OwnChatDefault(t *testing.T) {
	t.Parallel()

	f := newSearchToolFixture(t)
	marker := "marker-" + uuid.NewString()
	hit := insertSearchTestMessage(t, f.db, f.parent.ID, database.ChatMessageRoleAssistant,
		"Prefix text before the "+marker+" and a long tail "+strings.Repeat("z", 300))
	insertSearchTestMessage(t, f.db, f.parent.ID, database.ChatMessageRoleAssistant, "no hit here")
	insertSearchTestMessage(t, f.db, f.other.ID, database.ChatMessageRoleAssistant, "unrelated chat "+marker)

	resp := runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, map[string]any{"query": marker})
	result := decodeSearchChatMessagesResult(t, resp)

	require.Equal(t, f.parent.ID.String(), result.ChatID)
	require.Equal(t, marker, result.Query)
	require.Equal(t, 1, result.Returned)
	require.False(t, result.HasMore)
	require.Zero(t, result.NextBeforeID)
	require.Len(t, result.Matches, 1)
	match := result.Matches[0]
	require.Equal(t, hit.ID, match.ID)
	require.Equal(t, "assistant", match.Role)
	require.Empty(t, match.ToolName)
	require.NotEmpty(t, match.CreatedAt)
	require.Contains(t, match.Excerpt, marker)
	require.True(t, strings.HasPrefix(match.Excerpt, "Prefix text"), "hit near the start keeps the original beginning: %q", match.Excerpt)
	require.True(t, strings.HasSuffix(match.Excerpt, "..."), "long tail must be elided: %q", match.Excerpt)
	require.LessOrEqual(t, len([]rune(match.Excerpt)), searchChatMessagesExcerptMaxRunes+3)
}

func TestSearchChatMessagesTool_ParentAndChildCanSearchEachOther(t *testing.T) {
	t.Parallel()

	f := newSearchToolFixture(t)
	marker := "marker-" + uuid.NewString()
	childHit := insertSearchTestMessage(t, f.db, f.child.ID, database.ChatMessageRoleAssistant, "child said "+marker)
	parentHit := insertSearchTestMessage(t, f.db, f.parent.ID, database.ChatMessageRoleUser, "parent said "+marker)

	fromParent := decodeSearchChatMessagesResult(t, runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, map[string]any{
		"query":   marker,
		"chat_id": f.child.ID.String(),
	}))
	require.Equal(t, f.child.ID.String(), fromParent.ChatID)
	require.Len(t, fromParent.Matches, 1)
	require.Equal(t, childHit.ID, fromParent.Matches[0].ID)

	fromChild := decodeSearchChatMessagesResult(t, runSearchChatMessagesTool(f.ctx, t, f.server, f.child, map[string]any{
		"query":   marker,
		"chat_id": f.parent.ID.String(),
	}))
	require.Equal(t, f.parent.ID.String(), fromChild.ChatID)
	require.Len(t, fromChild.Matches, 1)
	require.Equal(t, parentHit.ID, fromChild.Matches[0].ID)
}

func TestSearchChatMessagesTool_DeniesUnrelatedAndMissingChats(t *testing.T) {
	t.Parallel()

	f := newSearchToolFixture(t)
	insertSearchTestMessage(t, f.db, f.other.ID, database.ChatMessageRoleAssistant, "secret")

	unrelated := runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, map[string]any{
		"query":   "secret",
		"chat_id": f.other.ID.String(),
	})
	require.True(t, unrelated.IsError)
	require.Equal(t, errSearchChatNotAccessible.Error(), unrelated.Content)

	missing := runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, map[string]any{
		"query":   "secret",
		"chat_id": uuid.NewString(),
	})
	require.True(t, missing.IsError)
	require.Equal(t, errSearchChatNotAccessible.Error(), missing.Content)

	malformed := runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, map[string]any{
		"query":   "secret",
		"chat_id": "not-a-uuid",
	})
	require.True(t, malformed.IsError)
}

func TestSearchChatMessagesTool_LineageAuthorization(t *testing.T) {
	t.Parallel()

	f := newSearchToolFixture(t)
	marker := "marker-" + uuid.NewString()

	// Delegated chats cannot spawn children through the server, so the
	// grandchild row is inserted directly to exercise the multi-hop walk.
	grandchild := dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.parent.OrganizationID,
		OwnerID:           f.parent.OwnerID,
		ParentChatID:      uuid.NullUUID{UUID: f.child.ID, Valid: true},
		LastModelConfigID: f.parent.LastModelConfigID,
	})
	sibling, err := f.server.createChildSubagentChatWithOptions(f.ctx, f.parent, "sibling prompt", "", childSubagentChatOptions{})
	require.NoError(t, err)

	// A chat linked into the lineage but owned by someone else.
	stranger := dbgen.User(t, f.db, database.User{})
	foreignChild := dbgen.Chat(t, f.db, database.Chat{
		OrganizationID:    f.parent.OrganizationID,
		OwnerID:           stranger.ID,
		ParentChatID:      uuid.NullUUID{UUID: f.parent.ID, Valid: true},
		LastModelConfigID: f.parent.LastModelConfigID,
	})

	for _, chatID := range []uuid.UUID{f.parent.ID, grandchild.ID, sibling.ID, foreignChild.ID} {
		insertSearchTestMessage(t, f.db, chatID, database.ChatMessageRoleAssistant, marker)
	}

	search := func(caller database.Chat, target uuid.UUID) fantasy.ToolResponse {
		return runSearchChatMessagesTool(f.ctx, t, f.server, caller, map[string]any{
			"query":   marker,
			"chat_id": target.String(),
		})
	}

	// Grandparent and grandchild reach each other through the intermediate chat.
	require.Len(t, decodeSearchChatMessagesResult(t, search(f.parent, grandchild.ID)).Matches, 1)
	require.Len(t, decodeSearchChatMessagesResult(t, search(grandchild, f.parent.ID)).Matches, 1)

	// Siblings share a parent but neither is an ancestor of the other.
	siblingResp := search(f.child, sibling.ID)
	require.True(t, siblingResp.IsError)
	require.Equal(t, errSearchChatNotAccessible.Error(), siblingResp.Content)

	// A different owner is denied in both directions even with a parent link.
	foreignResp := search(f.parent, foreignChild.ID)
	require.True(t, foreignResp.IsError)
	require.Equal(t, errSearchChatNotAccessible.Error(), foreignResp.Content)
	foreignUp := search(foreignChild, f.parent.ID)
	require.True(t, foreignUp.IsError)
	require.Equal(t, errSearchChatNotAccessible.Error(), foreignUp.Content)
}

func TestSearchChatMessagesTool_ExploreSubagentSearchesOwnChatOnly(t *testing.T) {
	t.Parallel()

	f := newSearchToolFixture(t)
	marker := "marker-" + uuid.NewString()

	explore, err := f.server.createChildSubagentChatWithOptions(f.ctx, f.parent, "explore prompt", "", childSubagentChatOptions{
		chatMode: database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true},
	})
	require.NoError(t, err)
	require.True(t, isExploreSubagentMode(explore.Mode))

	insertSearchTestMessage(t, f.db, f.parent.ID, database.ChatMessageRoleAssistant, "parent "+marker)
	ownHit := insertSearchTestMessage(t, f.db, explore.ID, database.ChatMessageRoleAssistant, "explore "+marker)

	own := decodeSearchChatMessagesResult(t, runSearchChatMessagesTool(f.ctx, t, f.server, explore, map[string]any{
		"query": marker,
	}))
	require.Equal(t, explore.ID.String(), own.ChatID)
	require.Len(t, own.Matches, 1)
	require.Equal(t, ownHit.ID, own.Matches[0].ID)

	explicitOwn := decodeSearchChatMessagesResult(t, runSearchChatMessagesTool(f.ctx, t, f.server, explore, map[string]any{
		"query":   marker,
		"chat_id": explore.ID.String(),
	}))
	require.Len(t, explicitOwn.Matches, 1)

	parentResp := runSearchChatMessagesTool(f.ctx, t, f.server, explore, map[string]any{
		"query":   marker,
		"chat_id": f.parent.ID.String(),
	})
	require.True(t, parentResp.IsError)
	require.Equal(t, errSearchChatNotAccessible.Error(), parentResp.Content)

	// The parent still reaches the explore child's transcript.
	fromParent := decodeSearchChatMessagesResult(t, runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, map[string]any{
		"query":   marker,
		"chat_id": explore.ID.String(),
	}))
	require.Len(t, fromParent.Matches, 1)
	require.Equal(t, ownHit.ID, fromParent.Matches[0].ID)
}

func TestSearchChatMessagesTool_LimitClampAndPaging(t *testing.T) {
	t.Parallel()

	f := newSearchToolFixture(t)
	marker := "marker-" + uuid.NewString()
	const total = 12
	ids := make([]int64, 0, total)
	for i := 0; i < total; i++ {
		msg := insertSearchTestMessage(t, f.db, f.parent.ID, database.ChatMessageRoleAssistant, marker)
		ids = append(ids, msg.ID)
	}

	// limit 0 falls back to the default of 10 and reports more.
	first := decodeSearchChatMessagesResult(t, runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, map[string]any{
		"query": marker,
		"limit": 0,
	}))
	require.Equal(t, defaultSearchChatMessagesLimit, first.Returned)
	require.True(t, first.HasMore)
	require.Equal(t, ids[total-defaultSearchChatMessagesLimit], first.NextBeforeID)
	require.Equal(t, ids[total-1], first.Matches[0].ID, "newest first")

	second := decodeSearchChatMessagesResult(t, runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, map[string]any{
		"query":     marker,
		"before_id": first.NextBeforeID,
	}))
	require.Equal(t, total-defaultSearchChatMessagesLimit, second.Returned)
	require.False(t, second.HasMore)
	require.Zero(t, second.NextBeforeID)
	require.Equal(t, ids[1], second.Matches[0].ID)
	require.Equal(t, ids[0], second.Matches[1].ID)

	// limit above the maximum is clamped to 50 and returns everything.
	all := decodeSearchChatMessagesResult(t, runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, map[string]any{
		"query": marker,
		"limit": 500,
	}))
	require.Equal(t, total, all.Returned)
	require.False(t, all.HasMore)
}

func TestSearchChatMessagesTool_RoleFilterAndToolName(t *testing.T) {
	t.Parallel()

	f := newSearchToolFixture(t)
	marker := "marker-" + uuid.NewString()
	insertSearchTestMessage(t, f.db, f.parent.ID, database.ChatMessageRoleAssistant, "assistant "+marker)
	toolRow := dbgen.ChatMessage(t, f.db, database.ChatMessage{
		ChatID: f.parent.ID,
		Role:   database.ChatMessageRoleTool,
		Content: pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`[{"type":"tool-result","tool_call_id":"c1","tool_name":"execute","result":{"output":"` + marker + `"}}]`),
			Valid:      true,
		},
	})

	result := decodeSearchChatMessagesResult(t, runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, map[string]any{
		"query": marker,
		"role":  "Tool",
	}))
	require.Len(t, result.Matches, 1)
	require.Equal(t, toolRow.ID, result.Matches[0].ID)
	require.Equal(t, "tool", result.Matches[0].Role)
	require.Equal(t, "execute", result.Matches[0].ToolName)
	createdAt, err := time.Parse(time.RFC3339Nano, result.Matches[0].CreatedAt)
	require.NoError(t, err)
	require.True(t, createdAt.Equal(toolRow.CreatedAt), "created_at %s must round-trip %s", result.Matches[0].CreatedAt, toolRow.CreatedAt)
}

func TestSearchChatMessagesTool_RejectsInvalidArgs(t *testing.T) {
	t.Parallel()

	f := newSearchToolFixture(t)

	for name, args := range map[string]map[string]any{
		"empty query":        {"query": "   "},
		"long query":         {"query": strings.Repeat("q", maxSearchChatMessagesQueryRunes+1)},
		"bad role":           {"query": "x", "role": "robot"},
		"system role":        {"query": "x", "role": "system"},
		"bad since":          {"query": "x", "since": "yesterday"},
		"bad until":          {"query": "x", "until": "2026-13-01T00:00:00Z"},
		"until before since": {"query": "x", "since": "2026-02-01T00:00:00Z", "until": "2026-01-01T00:00:00Z"},
		"zero before_id":     {"query": "x", "before_id": 0},
	} {
		resp := runSearchChatMessagesTool(f.ctx, t, f.server, f.parent, args)
		require.True(t, resp.IsError, "%s should be rejected, got %s", name, resp.Content)
	}
}

func TestShapeSearchExcerpt(t *testing.T) {
	t.Parallel()

	t.Run("HitAtStartShortText", func(t *testing.T) {
		t.Parallel()
		got := shapeSearchExcerpt(database.SearchChatMessagesRow{
			Excerpt: "hello world", HitPos: 1, TextLength: 11,
		})
		require.Equal(t, "hello world", got)
	})

	t.Run("WindowCutBothSides", func(t *testing.T) {
		t.Parallel()
		got := shapeSearchExcerpt(database.SearchChatMessagesRow{
			Excerpt:    "middle",
			HitPos:     searchChatMessagesContextChars + 100,
			TextLength: 10000,
		})
		require.Equal(t, "...middle...", got)
	})

	t.Run("RuneCapAppliesEllipsis", func(t *testing.T) {
		t.Parallel()
		got := shapeSearchExcerpt(database.SearchChatMessagesRow{
			Excerpt:    strings.Repeat("é", searchChatMessagesExcerptMaxRunes+50),
			HitPos:     1,
			TextLength: int32(searchChatMessagesExcerptMaxRunes + 50),
		})
		require.Equal(t, searchChatMessagesExcerptMaxRunes+3, len([]rune(got)))
		require.True(t, strings.HasSuffix(got, "..."))
	})
}
