package chatd_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
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

// setupDiscoveryAgentConn is setupToolExecutionAgentConn without the
// permissive instructions probe.
func setupDiscoveryAgentConn(mockConn *agentconnmock.MockAgentConn) {
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/coder"}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()
}

func systemPrompt(req *chattest.OpenAIRequest) string {
	var b strings.Builder
	for _, message := range req.Messages {
		if message.Role == "system" {
			_, _ = b.WriteString(message.Content)
			_, _ = b.WriteString("\n")
		}
	}
	return b.String()
}

// discoveryTurn seeds a workspace whose agent pins a root instruction file
// and a strict mock connection for one chat turn.
type discoveryTurn struct {
	ctx       context.Context
	db        database.Store
	ps        pubsub.Pubsub
	openAIURL string
	user      database.User
	org       database.Organization
	model     database.ChatModelConfig
	ws        database.WorkspaceTable
	agent     database.WorkspaceAgent
	conn      *agentconnmock.MockAgentConn
}

func newDiscoveryTurn(ctx context.Context, t *testing.T, model func(*chattest.OpenAIRequest) chattest.OpenAIResponse) *discoveryTurn {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	openAIURL := chattest.NewOpenAI(t, model)
	user, org, modelConfig := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, agent := seedWorkspaceWithAgent(t, db, user.ID)
	//nolint:gocritic // Seeding agent context as the chatd subject.
	seedAgentInstructionContext(dbauthz.AsChatd(ctx), t, db, agent.ID, discoveryRootSource, "root rules")
	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	setupDiscoveryAgentConn(conn)
	return &discoveryTurn{ctx: ctx, db: db, ps: ps, openAIURL: openAIURL, user: user, org: org, model: modelConfig, ws: ws, agent: agent, conn: conn}
}

// start runs the first turn on a server that dials agentConn (the mock
// connection when nil) and returns the chat once it is waiting.
func (d *discoveryTurn) start(t *testing.T, agentConn chatd.AgentConnFunc) database.Chat {
	t.Helper()
	if agentConn == nil {
		agentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, d.agent.ID, agentID)
			return d.conn, func() {}, nil
		}
	}
	server := newActiveTestServer(t, d.db, d.ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, d.openAIURL))
		cfg.AgentConn = agentConn
	})
	chat, err := server.CreateChat(d.ctx, chatd.CreateOptions{
		OrganizationID: d.org.ID,
		OwnerID:        d.user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: d.ws.ID, Valid: true},
		AgentID:        uuid.NullUUID{UUID: d.agent.ID, Valid: true},
		Title:          "lazy-instruction-discovery",
		ModelConfigID:  d.model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the app"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(d.ctx, t, d.db, chat.ID, database.ChatStatusWaiting)
	return chat
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

func TestLazyInstructionDiscoveryInjectsBeforeNextStep(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	var modelCalls atomic.Int32
	secondPrompt := make(chan string, 1)
	turn := newDiscoveryTurn(ctx, t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch modelCalls.Add(1) {
		case 1:
			require.NotContains(t, systemPrompt(req), discoveryNestedSource, "nothing is pinned before the tool touches the directory")
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("read_file", `{"path":"`+discoveryTouchedFile+`"}`))
		case 2:
			secondPrompt <- systemPrompt(req)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	turn.conn.EXPECT().ReadFileLines(gomock.Any(), discoveryTouchedFile, int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data"}, nil)
	// The working directory is a scan root, so only the nested chain is probed.
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
	turn.conn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{discoveryNestedDir, discoveryNestedDir + "/src"},
	}).Return(probe, nil)

	chat := turn.start(t, nil)

	prompt := testutil.RequireReceive(ctx, t, secondPrompt)
	rootIdx := strings.Index(prompt, "Source: "+discoveryRootSource)
	nestedIdx := strings.Index(prompt, "Source: "+discoveryNestedSource)
	require.NotEqual(t, -1, rootIdx, "root file stays pinned")
	require.NotEqual(t, -1, nestedIdx, "the discovered file is in the next model request")
	require.Less(t, rootIdx, nestedIdx, "root files precede nested ones")
	require.Contains(t, prompt, "site rules")
	require.Contains(t, prompt, excludedSource+" (excluded)", "the model is told about the file it could not receive")
	require.NotContains(t, prompt, "Source: "+excludedSource)

	pinned := pinnedBySource(ctx, t, turn.db, chat.ID)
	require.Len(t, pinned, 3)
	require.False(t, pinned[discoveryRootSource].Discovered)
	require.True(t, pinned[discoveryNestedSource].Discovered, "the nested file is pinned as a discovered row")
	require.Equal(t, database.WorkspaceAgentContextResourceStatusExcluded, pinned[excludedSource].Status, "an excluded file is part of the inventory")
}

func TestLazyInstructionDiscoveryNeverFailsStep(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	var modelCalls atomic.Int32
	turn := newDiscoveryTurn(ctx, t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("read_file", `{"path":"`+discoveryTouchedFile+`"}`))
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	turn.conn.EXPECT().ReadFileLines(gomock.Any(), discoveryTouchedFile, int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data"}, nil)
	turn.conn.EXPECT().ResolveContextInstructions(gomock.Any(), gomock.Any()).
		Return(workspacesdk.ResolveContextInstructionsResponse{}, xerrors.New("404 not found"))

	chat := turn.start(t, nil)

	final, err := turn.db.GetChatByID(dbauthz.AsChatd(ctx), chat.ID) //nolint:gocritic // Reading the chat as the chatd subject.
	require.NoError(t, err)
	require.Empty(t, final.LastError)
	require.EqualValues(t, 2, modelCalls.Load(), "the step committed and the turn finished")
	var toolResults int
	for _, message := range chatMessages(ctx, t, turn.db, chat.ID) {
		if message.Role == database.ChatMessageRoleTool {
			toolResults++
		}
	}
	require.Equal(t, 1, toolResults)
	require.Len(t, pinnedBySource(ctx, t, turn.db, chat.ID), 1, "a failed probe pins nothing")
}

// The agent drops between the step's preparation and its tools, and the
// tools' dial rebinds the turn to the rebuilt workspace's agent.
func TestLazyInstructionDiscoveryFollowsSwitchedAgent(t *testing.T) {
	t.Parallel()

	// Rebuilding while the model answers switches the turn through the tools'
	// connection; rebuilding during the read switches it through discovery's own.
	for _, tc := range []struct {
		name string
		at   rebuildPoint
	}{{name: "BeforeTools", at: rebuildWhileModelAnswers}, {name: "DuringTool", at: rebuildWhileToolRuns}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runSwitchedAgentDiscovery(t, tc.at)
		})
	}
}

type rebuildPoint int

const (
	rebuildWhileModelAnswers rebuildPoint = iota
	rebuildWhileToolRuns
)

func runSwitchedAgentDiscovery(t *testing.T, at rebuildPoint) {
	ctx := testutil.Context(t, testutil.WaitLong)
	//nolint:gocritic // Seeding and reading workspace rows as the chatd subject.
	chatdCtx := dbauthz.AsChatd(ctx)

	// The new agent works one directory up, so the touched file has one more
	// nested scope below the working directory.
	var (
		turn         *discoveryTurn
		build        database.WorkspaceBuild
		currentAgent atomic.Pointer[database.WorkspaceAgent]
		modelCalls   atomic.Int32
	)
	rebuild := func() {
		now := dbtime.Now()
		require.NoError(t, turn.db.UpdateWorkspaceAgentConnectionByID(chatdCtx, database.UpdateWorkspaceAgentConnectionByIDParams{
			ID:                     turn.agent.ID,
			FirstConnectedAt:       sql.NullTime{Time: now.Add(-2 * time.Hour), Valid: true},
			LastConnectedAt:        sql.NullTime{Time: now.Add(-2 * time.Hour), Valid: true},
			DisconnectedAt:         sql.NullTime{Time: now.Add(-time.Hour), Valid: true},
			UpdatedAt:              now,
			LastConnectedReplicaID: uuid.NullUUID{},
		}))
		job := dbgen.ProvisionerJob(t, turn.db, nil, database.ProvisionerJob{InitiatorID: turn.user.ID, OrganizationID: turn.org.ID})
		_ = dbgen.WorkspaceBuild(t, turn.db, database.WorkspaceBuild{
			TemplateVersionID: build.TemplateVersionID,
			WorkspaceID:       turn.ws.ID,
			JobID:             job.ID,
			BuildNumber:       build.BuildNumber + 1,
		})
		resource := dbgen.WorkspaceResource(t, turn.db, database.WorkspaceResource{Transition: database.WorkspaceTransitionStart, JobID: job.ID})
		agent := dbgen.WorkspaceAgent(t, turn.db, database.WorkspaceAgent{ResourceID: resource.ID, Directory: "/home/coder", OperatingSystem: "linux"})
		currentAgent.Store(&agent)
	}
	turn = newDiscoveryTurn(ctx, t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if modelCalls.Add(1) == 1 {
			if at == rebuildWhileModelAnswers {
				rebuild()
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("read_file", `{"path":"`+discoveryTouchedFile+`"}`))
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})
	var err error
	build, err = turn.db.GetLatestWorkspaceBuildByWorkspaceID(chatdCtx, turn.ws.ID)
	require.NoError(t, err)
	turn.conn.EXPECT().ReadFileLines(gomock.Any(), discoveryTouchedFile, int64(1), int64(0), gomock.Any()).
		DoAndReturn(func(context.Context, string, int64, int64, workspacesdk.ReadFileLinesLimits) (workspacesdk.ReadFileLinesResponse, error) {
			if at == rebuildWhileToolRuns {
				rebuild()
			}
			return workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 4, TotalLines: 1, LinesRead: 1, Content: "data"}, nil
		})
	turn.conn.EXPECT().ResolveContextInstructions(gomock.Any(), workspacesdk.ResolveContextInstructionsRequest{
		Directories: []string{"/home/coder/project", discoveryNestedDir, discoveryNestedDir + "/src"},
	}).Return(instructionFileResponse(discoveryNestedDir, discoveryNestedSource, "site rules"), nil)

	chat := turn.start(t, func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		current := currentAgent.Load()
		switch {
		case current == nil:
			require.Equal(t, turn.agent.ID, agentID)
		case agentID == turn.agent.ID:
			return nil, nil, xerrors.New("agent gone")
		default:
			require.Equal(t, current.ID, agentID)
		}
		return turn.conn, func() {}, nil
	})

	current, err := turn.db.GetChatByID(chatdCtx, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, currentAgent.Load(), "the model was called")
	require.Equal(t, currentAgent.Load().ID, current.AgentID.UUID, "the turn rebound the chat to the current agent")
	require.True(t, pinnedBySource(ctx, t, turn.db, chat.ID)[discoveryNestedSource].Discovered)
}
