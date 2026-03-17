package chatd

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

func TestRefreshChatWorkspaceSnapshot_NoReloadWhenWorkspacePresent(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}

	calls := 0
	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(context.Context, uuid.UUID) (database.Chat, error) {
			calls++
			return database.Chat{}, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, chat, refreshed)
	require.Equal(t, 0, calls)
}

func TestRefreshChatWorkspaceSnapshot_ReloadsWhenWorkspaceMissing(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	workspaceID := uuid.New()
	chat := database.Chat{ID: chatID}
	reloaded := database.Chat{
		ID: chatID,
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}

	calls := 0
	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(_ context.Context, id uuid.UUID) (database.Chat, error) {
			calls++
			require.Equal(t, chatID, id)
			return reloaded, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, reloaded, refreshed)
	require.Equal(t, 1, calls)
}

func TestRefreshChatWorkspaceSnapshot_ReturnsReloadError(t *testing.T) {
	t.Parallel()

	chat := database.Chat{ID: uuid.New()}
	loadErr := xerrors.New("boom")

	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(context.Context, uuid.UUID) (database.Chat, error) {
			return database.Chat{}, loadErr
		},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "reload chat workspace state")
	require.ErrorContains(t, err, loadErr.Error())
	require.Equal(t, chat, refreshed)
}

func TestResolveInstructionsReusesTurnLocalWorkspaceAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}
	workspaceAgent := database.WorkspaceAgent{
		ID:                uuid.New(),
		OperatingSystem:   "linux",
		Directory:         "/home/coder/project",
		ExpandedDirectory: "/home/coder/project",
	}

	db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(
		gomock.Any(),
		workspaceID,
	).Return([]database.WorkspaceAgent{workspaceAgent}, nil).Times(1)

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)
	conn.EXPECT().LS(gomock.Any(), "", gomock.Any()).Return(
		workspacesdk.LSResponse{},
		codersdk.NewTestError(404, "POST", "/api/v0/list-directory"),
	).Times(1)
	conn.EXPECT().ReadFile(
		gomock.Any(),
		"/home/coder/project/AGENTS.md",
		int64(0),
		int64(maxInstructionFileBytes+1),
	).Return(
		nil,
		"",
		codersdk.NewTestError(404, "GET", "/api/v0/read-file"),
	).Times(1)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := &Server{
		db:               db,
		logger:           logger,
		instructionCache: make(map[uuid.UUID]cachedInstruction),
		agentConnFn: func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return conn, func() {}, nil
		},
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	instruction := server.resolveInstructions(
		ctx,
		chat,
		workspaceCtx.getWorkspaceAgent,
		workspaceCtx.getWorkspaceConn,
	)
	require.Contains(t, instruction, "Operating System: linux")
	require.Contains(t, instruction, "Working Directory: /home/coder/project")
}

func TestTurnWorkspaceContextGetWorkspaceConnRefreshesWorkspaceAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}
	initialAgent := database.WorkspaceAgent{ID: uuid.New()}
	refreshedAgent := database.WorkspaceAgent{ID: uuid.New()}

	gomock.InOrder(
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(
			gomock.Any(),
			workspaceID,
		).Return([]database.WorkspaceAgent{initialAgent}, nil),
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(
			gomock.Any(),
			workspaceID,
		).Return([]database.WorkspaceAgent{refreshedAgent}, nil),
	)

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)

	var dialed []uuid.UUID
	server := &Server{db: db}
	server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		dialed = append(dialed, agentID)
		if agentID == initialAgent.ID {
			return nil, nil, xerrors.New("dial failed")
		}
		return conn, func() {}, nil
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, conn, gotConn)
	require.Equal(t, []uuid.UUID{initialAgent.ID, refreshedAgent.ID}, dialed)
}
