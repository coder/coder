package chatd_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestProjectMemoryIndexKeepsPromptPrefixStable drives a project chat through
// a memory write and a second turn, and checks that the memory index reaches
// the model as appended history while tool definitions and earlier messages,
// the provider's cached prefix, stay byte-identical.
func TestProjectMemoryIndexKeepsPromptPrefixStable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)

	var (
		mu       sync.Mutex
		requests []*chattest.OpenAIRequest
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		mu.Lock()
		requests = append(requests, req)
		n := len(requests)
		mu.Unlock()
		if n == 1 {
			chunk := chattest.OpenAIToolCallChunk(chattool.SaveMemoryToolName, `{"name":"deploy-day","description":"Deploys happen on Tuesdays","body":"The team deploys every Tuesday."}`)
			chunk.Choices[0].ToolCalls[0].ID = "call_save"
			return chattest.OpenAIStreamingResponse(chunk)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("ok")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID, Name: "platform"})
	dbgen.ChatProjectMemory(t, db, database.ChatProjectMemory{
		ProjectID: project.ID, OrganizationID: org.ID, CreatedBy: user.ID,
		Name: "release-owner", Description: "Alice owns releases", Body: "Alice owns releases.",
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		ProjectID:          uuid.NullUUID{UUID: project.ID, Valid: true},
		Title:              "memory-index",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("remember deploy day")},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("what do you know?")},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requests, 3, "save step, answer step, second turn")

	snapshot := chattool.FormatMemoryIndexSnapshot([]chattool.MemoryIndexEntry{{Name: "release-owner", Description: "Alice owns releases"}})
	update := chattool.FormatMemoryIndexUpdate([]chattool.MemoryIndexEntry{{Name: "deploy-day", Description: "Deploys happen on Tuesdays"}}, nil)
	count := func(req *chattest.OpenAIRequest, text string) int {
		n := 0
		for _, msg := range req.Messages {
			if strings.Contains(msg.Content, text) {
				n++
			}
		}
		return n
	}

	for i, req := range requests {
		require.Equal(t, 1, count(req, snapshot), "request %d carries the turn-one snapshot once", i)
		// Tool definitions lead the cached prefix and must not change when a
		// memory is written.
		require.Equal(t, requests[0].Tools, req.Tools, "request %d tools", i)
		// Every earlier message is replayed unchanged.
		prev := requests[max(i-1, 0)]
		require.GreaterOrEqual(t, len(req.Messages), len(prev.Messages))
		require.Equal(t, prev.Messages, req.Messages[:len(prev.Messages)], "request %d history prefix", i)
	}
	// The write in turn one reaches the model at the start of turn two, not
	// mid-turn.
	require.Zero(t, count(requests[1], update))
	require.Equal(t, 1, count(requests[2], update))

	promptRows, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var indexRows int
	for _, msg := range promptRows {
		if msg.Visibility == database.ChatMessageVisibilityModel && msg.Role == database.ChatMessageRoleUser {
			indexRows++
		}
	}
	require.Equal(t, 2, indexRows, "one snapshot and one update")
}
