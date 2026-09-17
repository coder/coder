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
	"github.com/coder/coder/v2/coderd/database/dbgen"
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
	// A second file the agent could not fit into the response is reported
	// as excluded and pinned without a body.
	excludedSource := discoveryNestedDir + "/CLAUDE.md"
	probe := instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules")
	probe.Files = append(probe.Files, workspacesdk.ContextInstructionFile{
		Directory:   discoveryNestedDir,
		Source:      excludedSource,
		ContentHash: probe.Files[0].ContentHash,
		SizeBytes:   1 << 16,
		Status:      "excluded",
		Error:       "response content cap exceeded",
	})
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir, discoveryNestedDir + "/src"},
	}).Return(probe, nil)

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
	require.Contains(t, prompt, excludedSource+" (excluded)", "the model is told about the file it could not receive")
	require.NotContains(t, prompt, "Source: "+excludedSource)

	//nolint:gocritic // Reading chat-owned rows as the chatd subject.
	rows, err := db.ListChatContextResourcesByChatID(dbauthz.AsChatd(ctx), chat.ID)
	require.NoError(t, err)
	bySource := make(map[string]database.ChatContextResource, len(rows))
	for _, row := range rows {
		bySource[row.Source] = row
	}
	require.Len(t, bySource, 3)
	require.False(t, bySource[discoveryRootSource].Discovered)
	require.True(t, bySource[discoveryNestedSource].Discovered, "the nested file is pinned as a discovered row")
	require.True(t, bySource[excludedSource].Discovered)
	require.Equal(t, database.WorkspaceAgentContextResourceStatusExcluded, bySource[excludedSource].Status, "an excluded file is part of the inventory")
}

// TestLazyInstructionDiscoveryReconcilesRemovedFiles checks that a
// directory the step wrote an instruction file into is probed again and
// that a discovered row the probe no longer returns is dropped, so a
// deleted or renamed nested file does not keep steering the chat.
func TestLazyInstructionDiscoveryReconcilesRemovedFiles(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	replacementSource := discoveryNestedDir + "/CLAUDE.md"
	var modelCalls atomic.Int32
	thirdPrompt := make(chan string, 1)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch modelCalls.Add(1) {
		case 1:
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("read_file", `{"path":"`+discoveryTouchedFile+`"}`))
		case 2:
			require.Contains(t, systemPromptContaining(req), "Source: "+discoveryNestedSource)
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("write_file", `{"path":"`+replacementSource+`","content":"new rules"}`))
		case 3:
			thirdPrompt <- systemPromptContaining(req)
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
	mockConn.EXPECT().WriteFile(gomock.Any(), replacementSource, gomock.Any()).Return(nil)
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir, discoveryNestedDir + "/src"},
	}).Return(instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules"), nil)
	// Writing CLAUDE.md makes site stale, so it is probed again despite its
	// pinned row; the answer no longer lists AGENTS.md.
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir},
	}).Return(instructionFileResponse(discoveryNestedDir, replacementSource, "new rules"), nil)

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
		Title:          "lazy-instruction-discovery-removed",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("swap the rules file"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	prompt := testutil.RequireReceive(ctx, t, thirdPrompt)
	require.Contains(t, prompt, "Source: "+replacementSource)
	require.NotContains(t, prompt, discoveryNestedSource, "the removed file is gone from the next model request")

	//nolint:gocritic // Reading chat-owned rows as the chatd subject.
	rows, err := db.ListChatContextResourcesByChatID(dbauthz.AsChatd(ctx), chat.ID)
	require.NoError(t, err)
	sources := make([]string, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, row.Source)
	}
	require.ElementsMatch(t, []string{discoveryRootSource, replacementSource}, sources)
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
	vanishingSource := discoveryNestedDir + "/CLAUDE.md"
	firstProbe := instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules v1")
	firstProbe.Files = append(firstProbe.Files, instructionFileResponse(discoveryNestedDir, vanishingSource, "claude rules").Files...)
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir, discoveryNestedDir + "/src"},
	}).Return(firstProbe, nil)
	// Refresh re-resolves only the directories that held discovered rows;
	// a file the directory no longer holds is dropped.
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
	require.Len(t, before, 3)
	require.True(t, before[discoveryNestedSource].Discovered)
	require.True(t, before[vanishingSource].Discovered)

	current, err := db.GetChatByID(chatdCtx, chat.ID)
	require.NoError(t, err)
	_, err = server.RefreshChatContext(ctx, current)
	require.NoError(t, err)

	after := pinned()
	require.Len(t, after, 2, "refresh keeps the discovered file alongside the snapshot copy and drops the vanished one")
	require.False(t, after[discoveryRootSource].Discovered)
	require.True(t, after[discoveryNestedSource].Discovered)
	require.NotContains(t, after, vanishingSource)
	v2 := sha256.Sum256([]byte("site rules v2"))
	require.Equal(t, v2[:], after[discoveryNestedSource].ContentHash, "refresh re-reads the nested file")
}

// TestRefreshKeepsDiscoveredRowsWhenReReadFails checks that a refresh whose
// agent re-read fails leaves the discovered rows as they were instead of
// dropping nested files until the next tool touch.
func TestRefreshKeepsDiscoveredRowsWhenReReadFails(t *testing.T) {
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
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir},
	}).Return(workspacesdk.ResolveContextInstructionsResponse{}, xerrors.New("agent gone"))

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
		Title:          "lazy-instruction-discovery-refresh-failure",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the app"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	//nolint:gocritic // Reading chat-owned rows as the chatd subject.
	chatdCtx := dbauthz.AsChatd(ctx)
	current, err := db.GetChatByID(chatdCtx, chat.ID)
	require.NoError(t, err)
	refreshed, err := server.RefreshChatContext(ctx, current)
	require.NoError(t, err)
	require.False(t, refreshed.ContextDirtySince.Valid)

	rows, err := db.ListChatContextResourcesByChatID(chatdCtx, chat.ID)
	require.NoError(t, err)
	bySource := make(map[string]database.ChatContextResource, len(rows))
	for _, row := range rows {
		bySource[row.Source] = row
	}
	require.Len(t, bySource, 2)
	require.True(t, bySource[discoveryNestedSource].Discovered, "the discovered row survives a failed re-read")
	v1 := sha256.Sum256([]byte("site rules v1"))
	require.Equal(t, v1[:], bySource[discoveryNestedSource].ContentHash)
}

// startChatWithDiscoveredFile drives one read_file step on the nested path,
// answering the probe of its directory chain with firstProbe, and returns
// the chat once it is waiting with the discovered rows pinned. Probes the
// test triggers afterwards are its own to expect.
func startChatWithDiscoveredFile(ctx context.Context, t *testing.T, firstProbe workspacesdk.ResolveContextInstructionsResponse) (database.Store, *chatd.Server, database.Chat, *agentconnmock.MockAgentConn) {
	t.Helper()
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
	}).Return(firstProbe, nil)

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
	//nolint:gocritic // Reading the chat as the chatd subject.
	current, err := db.GetChatByID(dbauthz.AsChatd(ctx), chat.ID)
	require.NoError(t, err)
	return db, server, current, mockConn
}

func pinnedBySource(ctx context.Context, t *testing.T, db database.Store, chatID uuid.UUID) map[string]database.ChatContextResource {
	t.Helper()
	//nolint:gocritic // Reading chat-owned rows as the chatd subject.
	rows, err := db.ListChatContextResourcesByChatID(dbauthz.AsChatd(ctx), chatID)
	require.NoError(t, err)
	out := make(map[string]database.ChatContextResource, len(rows))
	for _, row := range rows {
		out[row.Source] = row
	}
	return out
}

// TestRefreshKeepsNewerStepDiscovery races a refresh against a step: while
// the refresh probe is in flight, a step pins newer bytes for the same file.
// The probe's older read must not replace them.
func TestRefreshKeepsNewerStepDiscovery(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, server, chat, mockConn := startChatWithDiscoveredFile(ctx, t, instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules v1"))
	v3 := sha256.Sum256([]byte("site rules v3"))
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir},
	}).DoAndReturn(func(context.Context, workspacesdk.ResolveContextInstructionsRequest) (workspacesdk.ResolveContextInstructionsResponse, error) {
		//nolint:gocritic // A concurrent step pins as the chatd subject.
		require.NoError(t, db.UpsertChatContextDiscoveredResource(dbauthz.AsChatd(ctx), database.UpsertChatContextDiscoveredResourceParams{
			ChatID:      chat.ID,
			Source:      discoveryNestedSource,
			BodyKind:    database.WorkspaceAgentContextBodyKindInstructionFile,
			Body:        pinnedBySource(ctx, t, db, chat.ID)[discoveryNestedSource].Body,
			ContentHash: v3[:],
			SizeBytes:   13,
			Status:      database.WorkspaceAgentContextResourceStatusOk,
		}))
		return instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules v2"), nil
	})
	_, err := server.RefreshChatContext(ctx, chat)
	require.NoError(t, err)

	after := pinnedBySource(ctx, t, db, chat.ID)
	require.Equal(t, v3[:], after[discoveryNestedSource].ContentHash, "the step's newer read survives the refresh")
	require.True(t, after[discoveryNestedSource].Discovered)
}

// TestRefreshDiscardsRediscoveryAfterRebind races a refresh against a rebind:
// while the refresh probe is in flight, the chat moves to another agent and
// its rows are cleared. The old agent's answer, including a file the refresh
// never captured, must not be pinned onto the rebound chat.
func TestRefreshDiscardsRediscoveryAfterRebind(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, server, chat, mockConn := startChatWithDiscoveredFile(ctx, t, instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules v1"))
	//nolint:gocritic // Rebinding the chat as the chatd subject.
	chatdCtx := dbauthz.AsChatd(ctx)
	newSource := discoveryNestedDir + "/CLAUDE.md"
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir},
	}).DoAndReturn(func(context.Context, workspacesdk.ResolveContextInstructionsRequest) (workspacesdk.ResolveContextInstructionsResponse, error) {
		oldAgent, err := db.GetWorkspaceAgentByID(chatdCtx, chat.AgentID.UUID)
		require.NoError(t, err)
		newAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: oldAgent.ResourceID, Directory: oldAgent.Directory, OperatingSystem: "linux"})
		_, err = db.UpdateChatBuildAgentBinding(chatdCtx, database.UpdateChatBuildAgentBindingParams{
			ID:      chat.ID,
			BuildID: chat.BuildID,
			AgentID: uuid.NullUUID{UUID: newAgent.ID, Valid: true},
		})
		require.NoError(t, err)
		require.NoError(t, db.DeleteChatContextResourcesByChatID(chatdCtx, chat.ID))
		resp := instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules v2")
		resp.Files = append(resp.Files, instructionFileResponse(discoveryNestedDir, newSource, "claude rules").Files...)
		return resp, nil
	})
	_, err := server.RefreshChatContext(ctx, chat)
	require.NoError(t, err)

	require.Empty(t, pinnedBySource(ctx, t, db, chat.ID), "the previous agent's files are not pinned onto the rebound chat")
}

// TestLazyInstructionDiscoveryReprobesStaleDirectory checks that a
// directory a command or rule-file write made stale is not remembered as
// empty: the command may still be creating the file, so a later touch of
// the same tree asks the agent again.
func TestLazyInstructionDiscoveryReprobesStaleDirectory(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	writtenSource := discoveryNestedDir + "/CLAUDE.md"
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch modelCalls.Add(1) {
		case 1:
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("write_file", `{"path":"`+writtenSource+`","content":"draft"}`))
		case 2:
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("read_file", `{"path":"`+discoveryTouchedFile+`"}`))
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupDiscoveryAgentConn(mockConn)
	mockConn.EXPECT().WriteFile(gomock.Any(), writtenSource, gomock.Any()).Return(nil)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), discoveryTouchedFile, int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data"}, nil)
	// The write makes site stale and the probe finds nothing yet; the read
	// on the next step probes site again instead of trusting that answer.
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir},
	}).Return(workspacesdk.ResolveContextInstructionsResponse{}, nil)
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir, discoveryNestedDir + "/src"},
	}).Return(instructionFileResponse(discoveryNestedDir, writtenSource, "final rules"), nil)

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
		Title:          "lazy-instruction-discovery-reprobe",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("write then read"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	//nolint:gocritic // Reading chat-owned rows as the chatd subject.
	rows, err := db.ListChatContextResourcesByChatID(dbauthz.AsChatd(ctx), chat.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, writtenSource, rows[0].Source)
	require.True(t, rows[0].Discovered)
}

// TestLazyInstructionDiscoveryReprobesAfterBackgroundCommand checks that a
// directory a command ran in is read again on a later touch even though it
// already contributed a pinned file: the command may still be writing when
// its result returns, so the file it creates afterwards is picked up by the
// next read in that tree rather than waiting for Refresh context.
func TestLazyInstructionDiscoveryReprobesAfterBackgroundCommand(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	generatedSource := discoveryNestedDir + "/CLAUDE.md"
	var modelCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch modelCalls.Add(1) {
		case 1:
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("execute", `{"command":"./gen-rules.sh","workdir":"`+discoveryNestedDir+`","run_in_background":true}`))
		case 2:
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("read_file", `{"path":"`+discoveryTouchedFile+`"}`))
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupDiscoveryAgentConn(mockConn)
	mockConn.EXPECT().StartProcess(gomock.Any(), gomock.Cond(func(req workspacesdk.StartProcessRequest) bool {
		return req.Background && req.WorkDir == discoveryNestedDir
	})).Return(workspacesdk.StartProcessResponse{ID: "gen", Started: true}, nil)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), discoveryTouchedFile, int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data"}, nil)
	// The command's directory is probed right away and already holds a file,
	// which pins it. The read on the next step would normally skip a pinned
	// directory; the recent command makes it probe site again and find the
	// file the command has created since.
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir},
	}).Return(instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules"), nil)
	both := instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules")
	both.Files = append(both.Files, instructionFileResponse(discoveryNestedDir, generatedSource, "generated rules").Files...)
	mockConn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir, discoveryNestedDir + "/src"},
	}).Return(both, nil)

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
		Title:          "lazy-instruction-discovery-background-command",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("generate then read"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	pinned := pinnedBySource(ctx, t, db, chat.ID)
	require.Len(t, pinned, 2)
	require.True(t, pinned[discoveryNestedSource].Discovered)
	require.True(t, pinned[generatedSource].Discovered, "the file the command created after its result is pinned by the next read")
}
