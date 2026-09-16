package chatd_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

const (
	discoveryRootSource   = "/home/coder/project/AGENTS.md"
	discoveryNestedDir    = "/home/coder/project/site"
	discoveryNestedSource = "/home/coder/project/site/AGENTS.md"
	discoveryTouchedFile  = "/home/coder/project/site/src/App.tsx"
)

func instructionFileResponse(dir, source, content string) workspacesdk.ResolveContextInstructionsResponse {
	sum := sha256.Sum256([]byte(content))
	return workspacesdk.ResolveContextInstructionsResponse{Files: []workspacesdk.ContextInstructionFile{{
		Directory:   dir,
		Source:      source,
		Content:     content,
		ContentHash: hex.EncodeToString(sum[:]),
		SizeBytes:   uint64(len(content)),
		Status:      "ok",
	}}}
}

// setupDiscoveryAgentConn mirrors setupToolExecutionAgentConn without the
// permissive instructions probe, so each test pins down the exact probe it
// expects.
func setupDiscoveryAgentConn(mockConn *agentconnmock.MockAgentConn) {
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/coder"}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()
}

// systemPromptContaining returns the concatenated system content of a model
// request so tests can check what <workspace-context> the step carried.
func systemPromptContaining(req *chattest.OpenAIRequest) string {
	var b strings.Builder
	for _, message := range req.Messages {
		if message.Role == "system" {
			_, _ = b.WriteString(message.Content)
			_, _ = b.WriteString("\n")
		}
	}
	return b.String()
}

// TestLazyInstructionDiscoveryInjectsBeforeNextStep drives a read_file step
// on a nested path and checks that the instruction file the agent resolves
// for that directory is pinned for the chat and rendered in the very next
// model request, after the root file.
func TestLazyInstructionDiscoveryInjectsBeforeNextStep(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	secondPrompt := make(chan string, 1)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch modelCalls.Add(1) {
		case 1:
			require.NotContains(t, systemPromptContaining(req), discoveryNestedSource, "nothing is pinned before the tool touches the directory")
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("read_file", `{"path":"`+discoveryTouchedFile+`"}`))
		case 2:
			secondPrompt <- systemPromptContaining(req)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	//nolint:gocritic // Seeding agent context as the chatd subject.
	seedAgentInstructionContext(dbauthz.AsChatd(ctx), t, db, dbAgent.ID, discoveryRootSource, "root rules")

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupDiscoveryAgentConn(mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), discoveryTouchedFile, int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data"}, nil)
	// Only the nested directory is probed: the working directory itself is a
	// scan root, and the file's own directory (site/src) is probed alongside.
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir, discoveryNestedDir + "/src"},
	}).Return(instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules"), nil)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		Title:          "lazy-instruction-discovery",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the app"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	prompt := testutil.RequireReceive(ctx, t, secondPrompt)
	rootIdx := strings.Index(prompt, "Source: "+discoveryRootSource)
	nestedIdx := strings.Index(prompt, "Source: "+discoveryNestedSource)
	require.NotEqual(t, -1, rootIdx, "root file stays pinned")
	require.NotEqual(t, -1, nestedIdx, "the discovered file is in the next model request")
	require.Less(t, rootIdx, nestedIdx, "root files precede nested ones")
	require.Contains(t, prompt, "site rules")

	//nolint:gocritic // Reading chat-owned rows as the chatd subject.
	rows, err := db.ListChatContextResourcesByChatID(dbauthz.AsChatd(ctx), chat.ID)
	require.NoError(t, err)
	bySource := make(map[string]database.ChatContextResource, len(rows))
	for _, row := range rows {
		bySource[row.Source] = row
	}
	require.Len(t, bySource, 2)
	require.False(t, bySource[discoveryRootSource].Discovered)
	require.True(t, bySource[discoveryNestedSource].Discovered, "the nested file is pinned as a discovered row")
}

// TestLazyInstructionDiscoveryNeverFailsStep checks that an agent that
// cannot answer the instructions probe (an older agent, a timeout) leaves
// the tool step intact.
func TestLazyInstructionDiscoveryNeverFailsStep(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("read_file", `{"path":"`+discoveryTouchedFile+`"}`))
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupDiscoveryAgentConn(mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), discoveryTouchedFile, int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data"}, nil)
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), gomock.Any()).
		Return(workspacesdk.ResolveContextInstructionsResponse{}, xerrors.New("404 not found"))

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		Title:          "lazy-instruction-discovery-error",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the app"),
		},
	})
	require.NoError(t, err)
	final := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.Empty(t, final.LastError)
	require.EqualValues(t, 2, modelCalls.Load(), "the step committed and the turn finished")

	var toolResults int
	for _, message := range chatMessages(ctx, t, db, chat.ID) {
		if message.Role == database.ChatMessageRoleTool {
			toolResults++
		}
	}
	require.Equal(t, 1, toolResults)
	//nolint:gocritic // Reading chat-owned rows as the chatd subject.
	rows, err := db.ListChatContextResourcesByChatID(dbauthz.AsChatd(ctx), chat.ID)
	require.NoError(t, err)
	require.Empty(t, rows, "a failed probe pins nothing")
}

// TestRefreshReResolvesDiscoveredRows checks that Refresh context, which
// rewrites the pinned snapshot copy, re-reads discovered nested files through
// the agent instead of dropping them until the next tool touch.
func TestRefreshReResolvesDiscoveredRows(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("read_file", `{"path":"`+discoveryTouchedFile+`"}`))
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	//nolint:gocritic // Seeding agent context as the chatd subject.
	seedAgentInstructionContext(dbauthz.AsChatd(ctx), t, db, dbAgent.ID, discoveryRootSource, "root rules")

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupDiscoveryAgentConn(mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), discoveryTouchedFile, int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data"}, nil)
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir, discoveryNestedDir + "/src"},
	}).Return(instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules v1"), nil)
	// Refresh re-resolves only the directories that held discovered rows.
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir},
	}).Return(instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules v2"), nil)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		Title:          "lazy-instruction-discovery-refresh",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the app"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	//nolint:gocritic // Reading chat-owned rows as the chatd subject.
	chatdCtx := dbauthz.AsChatd(ctx)
	pinned := func() map[string]database.ChatContextResource {
		t.Helper()
		rows, err := db.ListChatContextResourcesByChatID(chatdCtx, chat.ID)
		require.NoError(t, err)
		out := make(map[string]database.ChatContextResource, len(rows))
		for _, row := range rows {
			out[row.Source] = row
		}
		return out
	}
	before := pinned()
	require.Len(t, before, 2)
	require.True(t, before[discoveryNestedSource].Discovered)

	current, err := db.GetChatByID(chatdCtx, chat.ID)
	require.NoError(t, err)
	_, err = server.RefreshChatContext(ctx, current)
	require.NoError(t, err)

	after := pinned()
	require.Len(t, after, 2, "refresh keeps the discovered file alongside the snapshot copy")
	require.False(t, after[discoveryRootSource].Discovered)
	require.True(t, after[discoveryNestedSource].Discovered)
	v2 := sha256.Sum256([]byte("site rules v2"))
	require.Equal(t, v2[:], after[discoveryNestedSource].ContentHash, "refresh re-reads the nested file")
}
