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

func TestAgentConnCache(t *testing.T) {
	t.Parallel()

	t.Run("ReleaseWaitsForLastBorrower", func(t *testing.T) {
		t.Parallel()

		agentID := uuid.New()
		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		var releases int
		var cache agentConnCache

		got, ret1 := cache.adopt(agentID, conn, func() { releases++ })
		require.Same(t, conn, got)
		got, ret2, ok := cache.borrow(agentID)
		require.True(t, ok)
		require.Same(t, conn, got)

		cache.drop()
		_, _, ok = cache.borrow(agentID)
		require.False(t, ok, "a dropped connection is not lent again")
		ret1()
		ret1()
		require.Zero(t, releases, "a borrower still holds the connection")
		ret2()
		require.Equal(t, 1, releases)
		ret2()
		require.Equal(t, 1, releases)
	})

	t.Run("AdoptSameAgentKeepsCachedConn", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		agentID := uuid.New()
		cached := agentconnmock.NewMockAgentConn(ctrl)
		dialed := agentconnmock.NewMockAgentConn(ctrl)
		var cachedReleases, dialedReleases int
		var cache agentConnCache

		_, ret1 := cache.adopt(agentID, cached, func() { cachedReleases++ })
		got, ret2 := cache.adopt(agentID, dialed, func() { dialedReleases++ })
		require.Same(t, cached, got)
		require.Equal(t, 1, dialedReleases)

		ret1()
		ret2()
		require.Zero(t, cachedReleases, "the cache keeps the connection when no step uses it")
		cache.drop()
		require.Equal(t, 1, cachedReleases)
	})

	t.Run("AdoptOtherAgentDropsCachedConn", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		oldAgentID, newAgentID := uuid.New(), uuid.New()
		var oldReleases int
		var cache agentConnCache

		_, ret := cache.adopt(oldAgentID, agentconnmock.NewMockAgentConn(ctrl), func() { oldReleases++ })
		ret()
		cache.setHome(oldAgentID, "/home/old")
		_, _ = cache.adopt(newAgentID, agentconnmock.NewMockAgentConn(ctrl), func() {})
		require.Equal(t, 1, oldReleases)

		_, _, ok := cache.borrow(oldAgentID)
		require.False(t, ok)
		_, ok = cache.home(newAgentID)
		require.False(t, ok, "the home directory belongs to the old workspace agent")
		cache.setHome(oldAgentID, "/home/old")
		_, ok = cache.home(oldAgentID)
		require.False(t, ok, "a home directory is kept only for the cached workspace agent")
		cache.setHome(newAgentID, "/home/new")
		home, ok := cache.home(newAgentID)
		require.True(t, ok)
		require.Equal(t, "/home/new", home)
	})

	t.Run("NilCache", func(t *testing.T) {
		t.Parallel()

		conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
		var releases int
		var cache *agentConnCache

		got, ret := cache.adopt(uuid.New(), conn, func() { releases++ })
		require.Same(t, conn, got)
		ret()
		require.Equal(t, 1, releases)
		_, _, ok := cache.borrow(uuid.New())
		require.False(t, ok)
		cache.drop()
	})
}

func TestTurnWorkspaceContext_RunnerConnFollowsStepWorkspace(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerConnFixture(t)
	w1, a1 := f.workspace(), f.agent(0)
	w2, a2 := f.workspace(), f.agent(0)
	var conns agentConnCache

	step1 := f.step(f.chat(w1, a1), &conns)
	conn, err := step1.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, f.conns[a1], conn)
	step1.close()
	require.Zero(t, f.releases(a1), "the chat runner keeps the connection between steps")

	// The next step's chat snapshot names another workspace.
	step2 := f.step(f.chat(w2, a2), &conns)
	conn, err = step2.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, f.conns[a2], conn)
	require.Equal(t, 1, f.releases(a1))
	step2.close()
	conns.drop()
	require.Equal(t, 1, f.releases(a2))
	require.Equal(t, []uuid.UUID{a1, a2}, f.dialedAgents())
}

func TestTurnWorkspaceContext_RunnerConnReusedAcrossSteps(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerConnFixture(t)
	w, a := f.workspace(), f.agent(0)
	f.conns[a].EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/a"}, nil).
		Times(1)
	var conns agentConnCache

	for range 2 {
		step := f.step(f.chat(w, a), &conns)
		home, err := step.workspaceHome(ctx)
		require.NoError(t, err)
		require.Equal(t, "/home/a", home)
		conn, err := step.getWorkspaceConn(ctx)
		require.NoError(t, err)
		require.Same(t, f.conns[a], conn)
		step.close()
	}
	require.Equal(t, []uuid.UUID{a}, f.dialedAgents())
	require.Zero(t, f.releases(a))
	conns.drop()
	require.Equal(t, 1, f.releases(a))
}

func TestTurnWorkspaceContext_RunnerHomeFollowsWorkspaceAgent(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerConnFixture(t)
	w, a1, a2 := f.workspace(), f.agent(0), f.agent(0)
	f.conns[a1].EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/a1"}, nil).
		Times(1)
	f.conns[a2].EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/a2"}, nil).
		Times(1)
	var conns agentConnCache

	step1 := f.step(f.chat(w, a1), &conns)
	home, err := step1.workspaceHome(ctx)
	require.NoError(t, err)
	require.Equal(t, "/home/a1", home)
	step1.close()

	// The chat was rebound to another workspace agent of the same
	// workspace, as after a rebuild.
	step2 := f.step(f.chat(w, a2), &conns)
	home, err = step2.workspaceHome(ctx)
	require.NoError(t, err)
	require.Equal(t, "/home/a2", home)
	step2.close()
	require.Equal(t, 1, f.releases(a1))
}

func TestTurnWorkspaceContext_RunnerHomeBumpsWorkspaceUsage(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerConnFixture(t)
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
	var conns agentConnCache

	for i := range 2 {
		step := f.step(f.chat(w, a), &conns)
		before := bumps.Load()
		_, err := step.workspaceHome(ctx)
		require.NoError(t, err)
		require.Greaterf(t, bumps.Load(), before, "step %d should bump workspace usage", i+1)
		step.close()
	}
}

func TestTurnWorkspaceContext_RunnerConnReleasedAfterEveryStep(t *testing.T) {
	t.Parallel()

	// A step whose task was canceled can still be running when the next
	// step starts. Dropping the connection in one step must not close it
	// under the other.
	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerConnFixture(t)
	w, a := f.workspace(), f.agent(0)
	var conns agentConnCache

	step1 := f.step(f.chat(w, a), &conns)
	_, err := step1.getWorkspaceConn(ctx)
	require.NoError(t, err)
	step2 := f.step(f.chat(w, a), &conns)
	_, err = step2.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{a}, f.dialedAgents())

	step1.clearCachedWorkspaceState()
	step1.close()
	require.Zero(t, f.releases(a))
	conn, err := step2.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, f.conns[a], conn)
	step2.close()
	require.Equal(t, 1, f.releases(a))
}

func TestTurnWorkspaceContext_RunnerConnDroppedWhenAgentDisconnected(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	f := newRunnerConnFixture(t)
	w, a := f.workspace(), f.agent(10*time.Minute)
	staleConn := agentconnmock.NewMockAgentConn(f.ctrl)
	var staleReleases int
	var conns agentConnCache
	_, ret := conns.adopt(a, staleConn, func() { staleReleases++ })
	ret()

	step := f.step(f.chat(w, a), &conns)
	conn, err := step.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, f.conns[a], conn)
	require.Equal(t, 1, staleReleases)
	require.Equal(t, []uuid.UUID{a}, f.dialedAgents())
	step.close()
}

func TestTurnWorkspaceContext_DropStaleRunnerConn(t *testing.T) {
	t.Parallel()

	f := newRunnerConnFixture(t)
	w, a := f.workspace(), f.agent(0)
	var releases int
	var conns agentConnCache
	_, ret := conns.adopt(a, f.conns[a], func() { releases++ })
	ret()

	step := f.step(f.chat(w, a), &conns)
	step.dropStaleRunnerConn(a)
	require.Zero(t, releases, "the chat is bound to the latest workspace agent")
	step.dropStaleRunnerConn(uuid.New())
	require.Equal(t, 1, releases)
	step.close()
}

// runnerConnFixture serves workspaces and workspace agents from a mock
// database and dials one mock connection per workspace agent.
type runnerConnFixture struct {
	ctrl   *gomock.Controller
	db     *dbmock.MockStore
	clock  *quartz.Mock
	server *Server
	conns  map[uuid.UUID]*agentconnmock.MockAgentConn

	mu       sync.Mutex
	dialed   []uuid.UUID
	released map[uuid.UUID]int
}

func newRunnerConnFixture(t *testing.T) *runnerConnFixture {
	ctrl := gomock.NewController(t)
	f := &runnerConnFixture{
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

func (f *runnerConnFixture) workspace() uuid.UUID {
	workspaceID := uuid.New()
	expectLiveWorkspace(f.db, workspaceID)
	return workspaceID
}

// agent adds a workspace agent that last connected lastSeen ago, and the
// connection that dialing it returns.
func (f *runnerConnFixture) agent(lastSeen time.Duration) uuid.UUID {
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

func (*runnerConnFixture) chat(workspaceID, agentID uuid.UUID) database.Chat {
	return database.Chat{
		ID:          uuid.New(),
		WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
		AgentID:     uuid.NullUUID{UUID: agentID, Valid: true},
	}
}

// step returns the workspace context of one generation step.
func (f *runnerConnFixture) step(chat database.Chat, conns *agentConnCache) *turnWorkspaceContext {
	currentChat := chat
	return &turnWorkspaceContext{
		server:      f.server,
		chatStateMu: &sync.Mutex{},
		currentChat: &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) {
			return currentChat, nil
		},
		conns: conns,
	}
}

func (f *runnerConnFixture) dial(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
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

func (f *runnerConnFixture) dialedAgents() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uuid.UUID(nil), f.dialed...)
}

func (f *runnerConnFixture) releases(agentID uuid.UUID) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.released[agentID]
}
