package coderd_test

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestOrchestratorChat covers the orchestrator lifecycle end to end: lazy
// creation through the dedicated endpoint, singleton enforcement, exclusion
// from the regular chat list, the restricted tool set, and the spawn/list
// tools operating on the owner's chats.
func TestOrchestratorChat(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	type recordedCall struct {
		tools    []string
		messages []chattest.OpenAIMessage
	}
	var (
		callsMu sync.Mutex
		calls   []recordedCall
	)
	var streamCount atomic.Int32
	baseURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title": "Generated Title"}`)
		}
		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
		callsMu.Lock()
		calls = append(calls, recordedCall{tools: names, messages: append([]chattest.OpenAIMessage(nil), req.Messages...)})
		callsMu.Unlock()

		// Only the orchestrator receives spawn_chat. Spawned chats and
		// later orchestrator turns get a plain text reply.
		hasSpawnChat := false
		for _, name := range names {
			if name == "spawn_chat" {
				hasSpawnChat = true
				break
			}
		}
		if hasSpawnChat {
			switch streamCount.Add(1) {
			case 1:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("spawn_chat", `{"prompt":"Investigate the flaky build","title":"Flaky build"}`),
				)
			case 2:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("list_chats", `{}`),
				)
			}
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})

	client, db, api := newChatClientWithoutAIBridge(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModelWithBaseURL(t, client, baseURL)
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)

	// No orchestrator exists until the first message creates it.
	_, err := client.OrchestratorChat(ctx)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

	orchestrator, err := client.CreateOrchestratorChat(ctx, codersdk.CreateOrchestratorChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "Start a chat to investigate the flaky build, then list my chats.",
		}},
		ClientType: codersdk.ChatClientTypeUI,
	})
	require.NoError(t, err)
	require.Equal(t, chatd.OrchestratorChatID(firstUser.UserID), orchestrator.ID)
	require.Equal(t, codersdk.ChatModeOrchestrator, orchestrator.Mode)
	require.Equal(t, "Orchestrator", orchestrator.Title)
	require.Nil(t, orchestrator.WorkspaceID)

	fetched, err := client.OrchestratorChat(ctx)
	require.NoError(t, err)
	require.Equal(t, orchestrator.ID, fetched.ID)

	// A second create is rejected instead of producing a duplicate.
	_, err = client.CreateOrchestratorChat(ctx, codersdk.CreateOrchestratorChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "again",
		}},
	})
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusConflict, sdkErr.StatusCode())

	// Orchestrators never attach a workspace or enter plan mode.
	planMode := codersdk.ChatPlanModePlan
	err = client.UpdateChat(ctx, orchestrator.ID, codersdk.UpdateChatRequest{PlanMode: &planMode})
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	workspaceID := uuid.New()
	err = client.UpdateChat(ctx, orchestrator.ID, codersdk.UpdateChatRequest{WorkspaceID: &workspaceID})
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

	settled := coderdtest.WaitForChatSettled(ctx, t, api, orchestrator.ID)
	require.Equal(t, database.ChatStatusWaiting, settled.Status, "last_error=%s", string(settled.LastError.RawMessage))

	// The spawned chat is an independent root chat owned by the user and
	// the orchestrator itself is hidden from the regular list.
	chats, err := client.ListChats(ctx, nil)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	spawned := chats[0]
	require.Equal(t, "Flaky build", spawned.Title)
	require.Equal(t, firstUser.UserID, spawned.OwnerID)
	require.Nil(t, spawned.ParentChatID)
	require.Empty(t, spawned.Mode)
	require.NotEqual(t, orchestrator.ID, spawned.ID)
	coderdtest.WaitForChatSettled(ctx, t, api, spawned.ID)

	callsMu.Lock()
	recorded := append([]recordedCall(nil), calls...)
	callsMu.Unlock()

	var orchestratorCalls, spawnedCalls []recordedCall
	for _, call := range recorded {
		if containsString(call.tools, "spawn_chat") {
			orchestratorCalls = append(orchestratorCalls, call)
			continue
		}
		spawnedCalls = append(spawnedCalls, call)
	}
	require.Len(t, orchestratorCalls, 3, "spawn, list, then final text")
	require.NotEmpty(t, spawnedCalls)

	orchestratorTools := orchestratorCalls[0].tools
	for _, name := range []string{"spawn_chat", "list_chats", "read_chat"} {
		require.Contains(t, orchestratorTools, name)
	}
	for _, name := range []string{
		"read_file", "write_file", "edit_files", "execute", "process_output",
		"list_templates", "create_workspace", "start_workspace", "stop_workspace",
		"spawn_agent", "wait_agent", "list_agents",
	} {
		require.NotContains(t, orchestratorTools, name)
	}
	require.True(t, hasSystemSubstring(orchestratorCalls[0].messages, "You are the Coder orchestrator"))
	require.False(t, hasSystemSubstring(orchestratorCalls[0].messages, "You are the Coder agent"))

	// The spawned chat is a regular root chat with the full tool set.
	require.Contains(t, spawnedCalls[0].tools, "execute")
	require.Contains(t, spawnedCalls[0].tools, "create_workspace")
	require.NotContains(t, spawnedCalls[0].tools, "spawn_chat")

	// The list_chats result fed back to the model includes the spawned
	// chat and not the orchestrator itself.
	var listResult string
	for _, msg := range orchestratorCalls[2].messages {
		if msg.Role == "tool" && strings.Contains(msg.Content, `"chats"`) {
			listResult = msg.Content
		}
	}
	require.NotEmpty(t, listResult, "expected a list_chats tool result in the final prompt")
	require.Contains(t, listResult, spawned.ID.String())
	require.NotContains(t, listResult, orchestrator.ID.String())

	// The orchestrator row is still readable directly.
	dbOrchestrator, err := db.GetOrchestratorChatByOwnerID(dbauthz.AsChatd(ctx), firstUser.UserID)
	require.NoError(t, err)
	require.Equal(t, orchestrator.ID, dbOrchestrator.ID)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasSystemSubstring(messages []chattest.OpenAIMessage, want string) bool {
	for _, msg := range messages {
		if msg.Role == "system" && strings.Contains(msg.Content, want) {
			return true
		}
	}
	return false
}
