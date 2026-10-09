package chatd

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRunnerAgentConn_ReusesConnectionAcrossSteps(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerAgentConnFixture(t)
	w, a := f.workspace(), f.agent(0)
	f.conns[a].EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/a"}, nil).
		Times(1)
	var runnerAgentConn runnerAgentConn

	for range 2 {
		step := f.step(f.chat(w, a), &runnerAgentConn)
		home, err := step.workspaceHome(ctx)
		require.NoError(t, err)
		require.Equal(t, "/home/a", home)
		conn, err := step.getWorkspaceConn(ctx)
		require.NoError(t, err)
		require.Same(t, f.conns[a], conn)
		step.close()
	}
	require.Equal(t, []uuid.UUID{a}, f.dialedAgents())
	require.Zero(t, f.releases(a), "steps do not release the runner's connection")
	runnerAgentConn.close()
	require.Equal(t, 1, f.releases(a))
}

func TestRunnerAgentConn_DialsWhenWorkspaceAgentChanges(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerAgentConnFixture(t)
	w1, w2 := f.workspace(), f.workspace()
	a1, a2, a3 := f.agent(0), f.agent(0), f.agent(0)
	for agentID, home := range map[uuid.UUID]string{a1: "/home/a1", a2: "/home/a2", a3: "/home/a3"} {
		f.conns[agentID].EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(workspacesdk.LSResponse{AbsolutePathString: home}, nil).
			Times(1)
	}
	var runnerAgentConn runnerAgentConn

	// Each step's chat snapshot names the workspace agent: first a1, then
	// a2 in the same workspace, as after a rebuild, then a3 in another
	// workspace.
	for _, tc := range []struct {
		workspaceID, agentID uuid.UUID
		home                 string
	}{
		{workspaceID: w1, agentID: a1, home: "/home/a1"},
		{workspaceID: w1, agentID: a2, home: "/home/a2"},
		{workspaceID: w2, agentID: a3, home: "/home/a3"},
	} {
		step := f.step(f.chat(tc.workspaceID, tc.agentID), &runnerAgentConn)
		home, err := step.workspaceHome(ctx)
		require.NoError(t, err)
		require.Equal(t, tc.home, home)
		conn, err := step.getWorkspaceConn(ctx)
		require.NoError(t, err)
		require.Same(t, f.conns[tc.agentID], conn)
		step.close()
	}
	require.Equal(t, []uuid.UUID{a1, a2, a3}, f.dialedAgents())
	require.Zero(t, f.releases(a1)+f.releases(a2), "a step may still use a replaced connection")
	runnerAgentConn.close()
	require.Equal(t, 1, f.releases(a1))
	require.Equal(t, 1, f.releases(a2))
	require.Equal(t, 1, f.releases(a3))
}

func TestRunnerAgentConn_DialsWhenWorkspaceAgentDisconnected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerAgentConnFixture(t)
	w, a := f.workspace(), f.agent(10*time.Minute)
	held := agentconnmock.NewMockAgentConn(f.ctrl)
	var heldReleases int
	runnerAgentConn := runnerAgentConn{
		agentID:  a,
		conn:     held,
		home:     "/home/a",
		releases: []func(){func() { heldReleases++ }},
	}

	step := f.step(f.chat(w, a), &runnerAgentConn)
	conn, err := step.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, f.conns[a], conn)
	require.Equal(t, []uuid.UUID{a}, f.dialedAgents())
	_, ok := runnerAgentConn.homeOf(a)
	require.False(t, ok, "the home directory belongs to the replaced connection")
	step.close()
	runnerAgentConn.close()
	require.Equal(t, 1, heldReleases)
	require.Equal(t, 1, f.releases(a))
}

func TestRunnerAgentConn_ForgetsConnectionOfStaleBinding(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerAgentConnFixture(t)
	w, stale, latest := f.workspace(), f.agent(0), f.agent(0)
	f.db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), w).
		Return([]database.WorkspaceAgent{{ID: latest}}, nil).
		AnyTimes()
	var runnerAgentConn runnerAgentConn

	step := f.step(f.chat(w, stale), &runnerAgentConn)
	_, err := step.getWorkspaceConn(ctx)
	require.NoError(t, err)
	_, latestAgentID, err := step.workspaceAgentIDForConn(ctx)
	require.NoError(t, err)
	require.Equal(t, latest, latestAgentID)
	step.close()

	step = f.step(f.chat(w, stale), &runnerAgentConn)
	_, err = step.getWorkspaceConn(ctx)
	require.NoError(t, err)
	step.close()
	require.Equal(t, []uuid.UUID{stale, stale}, f.dialedAgents(), "the next step dials again")
	runnerAgentConn.close()
	require.Equal(t, 2, f.releases(stale))
}

func TestRunnerAgentConn_CachedHomeBumpsWorkspaceUsage(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerAgentConnFixture(t)
	w, a := f.workspace(), f.agent(0)
	f.conns[a].EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/a"}, nil).
		Times(1)
	tracker := workspacestats.NewTracker(f.db,
		workspacestats.TrackerWithTickFlush(make(chan time.Time), make(chan int, 1)),
		workspacestats.TrackerWithLogger(slogtest.Make(t, nil)),
	)
	t.Cleanup(func() { _ = tracker.Close() })
	f.server.usageTracker = tracker
	var bumps atomic.Int32
	f.db.EXPECT().ActivityBumpWorkspace(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, database.ActivityBumpWorkspaceParams) error {
			bumps.Add(1)
			return nil
		}).AnyTimes()
	var runnerAgentConn runnerAgentConn
	defer runnerAgentConn.close()

	for i := range 2 {
		step := f.step(f.chat(w, a), &runnerAgentConn)
		before := bumps.Load()
		_, err := step.workspaceHome(ctx)
		require.NoError(t, err)
		require.Greaterf(t, bumps.Load(), before, "step %d should bump workspace usage", i+1)
		step.close()
	}
}

func TestRunnerAgentConn_Close(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerAgentConnFixture(t)
	a1, a2 := f.agent(0), f.agent(0)
	var runnerAgentConn runnerAgentConn

	conn, release, err := runnerAgentConn.dial(ctx, f.server, a1)
	require.NoError(t, err)
	release()
	runnerAgentConn.setHome(conn, "/home/a1")
	other := agentconnmock.NewMockAgentConn(f.ctrl)
	runnerAgentConn.setHome(other, "/home/other")
	home, ok := runnerAgentConn.homeOf(a1)
	require.True(t, ok)
	require.Equal(t, "/home/a1", home, "only the held connection sets the home directory")
	_, _, err = runnerAgentConn.dial(ctx, f.server, a2)
	require.NoError(t, err)
	require.Zero(t, f.releases(a1))

	runnerAgentConn.close()
	runnerAgentConn.close()
	require.Equal(t, 1, f.releases(a1))
	require.Equal(t, 1, f.releases(a2))
	_, ok = runnerAgentConn.homeOf(a2)
	require.False(t, ok)
}

// runnerAgentConnFixture serves workspaces and workspace agents from a mock
// database and dials one mock connection per workspace agent.
type runnerAgentConnFixture struct {
	ctrl   *gomock.Controller
	db     *dbmock.MockStore
	clock  *quartz.Mock
	server *Server
	conns  map[uuid.UUID]*agentconnmock.MockAgentConn

	mu       sync.Mutex
	dialed   []uuid.UUID
	released map[uuid.UUID]int
}

func newRunnerAgentConnFixture(t *testing.T) *runnerAgentConnFixture {
	ctrl := gomock.NewController(t)
	f := &runnerAgentConnFixture{
		ctrl:     ctrl,
		db:       dbmock.NewMockStore(ctrl),
		clock:    quartz.NewMock(t),
		conns:    make(map[uuid.UUID]*agentconnmock.MockAgentConn),
		released: make(map[uuid.UUID]int),
	}
	f.server = &Server{
		db:                             f.db,
		logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		clock:                          f.clock,
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    defaultDialTimeout,
		agentConnFn:                    f.dial,
	}
	return f
}

func (f *runnerAgentConnFixture) workspace() uuid.UUID {
	workspaceID := uuid.New()
	expectLiveWorkspace(f.db, workspaceID)
	return workspaceID
}

// agent adds a workspace agent that last connected lastSeen ago, and the
// connection that dialing it returns.
func (f *runnerAgentConnFixture) agent(lastSeen time.Duration) uuid.UUID {
	now := f.clock.Now()
	agent := database.WorkspaceAgent{
		ID:               uuid.New(),
		FirstConnectedAt: sql.NullTime{Time: now.Add(-time.Hour), Valid: true},
		LastConnectedAt:  sql.NullTime{Time: now.Add(-lastSeen), Valid: true},
	}
	f.db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agent.ID).Return(agent, nil).AnyTimes()
	conn := agentconnmock.NewMockAgentConn(f.ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	f.conns[agent.ID] = conn
	return agent.ID
}

func (*runnerAgentConnFixture) chat(workspaceID, agentID uuid.UUID) database.Chat {
	return database.Chat{
		ID:          uuid.New(),
		WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		AgentID:     uuid.NullUUID{UUID: agentID, Valid: true},
	}
}

// step returns the workspace context of one generation step.
func (f *runnerAgentConnFixture) step(chat database.Chat, runnerAgentConn *runnerAgentConn) *turnWorkspaceContext {
	currentChat := chat
	return &turnWorkspaceContext{
		server:      f.server,
		chatStateMu: &sync.Mutex{},
		currentChat: &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) {
			return currentChat, nil
		},
		runnerAgentConn: runnerAgentConn,
	}
}

func (f *runnerAgentConnFixture) dial(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	conn, ok := f.conns[agentID]
	if !ok {
		return nil, nil, xerrors.Errorf("unknown workspace agent %s", agentID)
	}
	f.dialed = append(f.dialed, agentID)
	return conn, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.released[agentID]++
	}, nil
}

func (f *runnerAgentConnFixture) dialedAgents() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uuid.UUID(nil), f.dialed...)
}

func (f *runnerAgentConnFixture) releases(agentID uuid.UUID) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.released[agentID]
}
