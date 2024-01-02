package coderd

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/coder/coder/v2/coderd/util/ptr"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentWebsocketMonitor_ContextCancel(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	now := dbtime.Now()
	fConn := &fakePingerCloser{}
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	fUpdater := &fakeUpdater{}
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	agent := database.WorkspaceAgent{
		ID: uuid.New(),
		FirstConnectedAt: sql.NullTime{
			Time:  now.Add(-time.Minute),
			Valid: true,
		},
	}
	build := database.WorkspaceBuild{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
	}
	replicaID := uuid.New()

	uut := &agentWebsocketMonitor{
		apiCtx:            ctx,
		workspaceAgent:    agent,
		workspaceBuild:    build,
		conn:              fConn,
		db:                mDB,
		replicaID:         replicaID,
		updater:           fUpdater,
		logger:            logger,
		pingPeriod:        testutil.IntervalFast,
		disconnectTimeout: testutil.WaitShort,
	}
	uut.init()

	connected := mDB.EXPECT().UpdateWorkspaceAgentConnectionByID(
		gomock.Any(),
		connectionUpdate(agent.ID, replicaID),
	).
		AnyTimes().
		Return(nil)
	mDB.EXPECT().UpdateWorkspaceAgentConnectionByID(
		gomock.Any(),
		connectionUpdate(agent.ID, replicaID, withDisconnected()),
	).
		After(connected).
		Times(1).
		Return(nil)
	mDB.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), build.WorkspaceID).
		AnyTimes().
		Return(database.WorkspaceBuild{ID: build.ID}, nil)

	closeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		uut.monitor(closeCtx)
		close(done)
	}()
	// wait a couple intervals, but not long enough for a disconnect
	time.Sleep(3 * testutil.IntervalFast)
	fConn.requireNotClosed(t)
	fUpdater.requireEventuallySomeUpdates(t, build.WorkspaceID)
	n := fUpdater.getUpdates()
	cancel()
	fConn.requireEventuallyClosed(t, websocket.StatusGoingAway, "canceled")

	// make sure we got at least one additional update on close
	_ = testutil.RequireRecvCtx(ctx, t, done)
	m := fUpdater.getUpdates()
	require.Greater(t, m, n)
}

func TestAgentWebsocketMonitor_PingTimeout(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	now := dbtime.Now()
	fConn := &fakePingerCloser{}
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	fUpdater := &fakeUpdater{}
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	agent := database.WorkspaceAgent{
		ID: uuid.New(),
		FirstConnectedAt: sql.NullTime{
			Time:  now.Add(-time.Minute),
			Valid: true,
		},
	}
	build := database.WorkspaceBuild{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
	}
	replicaID := uuid.New()

	uut := &agentWebsocketMonitor{
		apiCtx:            ctx,
		workspaceAgent:    agent,
		workspaceBuild:    build,
		conn:              fConn,
		db:                mDB,
		replicaID:         replicaID,
		updater:           fUpdater,
		logger:            logger,
		pingPeriod:        testutil.IntervalFast,
		disconnectTimeout: testutil.WaitShort,
	}
	uut.init()
	// set the last ping to the past, so we go thru the timeout
	uut.lastPing.Store(ptr.Ref(now.Add(-time.Hour)))

	connected := mDB.EXPECT().UpdateWorkspaceAgentConnectionByID(
		gomock.Any(),
		connectionUpdate(agent.ID, replicaID),
	).
		AnyTimes().
		Return(nil)
	mDB.EXPECT().UpdateWorkspaceAgentConnectionByID(
		gomock.Any(),
		connectionUpdate(agent.ID, replicaID, withDisconnected()),
	).
		After(connected).
		Times(1).
		Return(nil)
	mDB.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), build.WorkspaceID).
		AnyTimes().
		Return(database.WorkspaceBuild{ID: build.ID}, nil)

	go uut.monitor(ctx)
	fConn.requireEventuallyClosed(t, websocket.StatusGoingAway, "ping timeout")
	fUpdater.requireEventuallySomeUpdates(t, build.WorkspaceID)
}

func TestAgentWebsocketMonitor_BuildOutdated(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	now := dbtime.Now()
	fConn := &fakePingerCloser{}
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	fUpdater := &fakeUpdater{}
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	agent := database.WorkspaceAgent{
		ID: uuid.New(),
		FirstConnectedAt: sql.NullTime{
			Time:  now.Add(-time.Minute),
			Valid: true,
		},
	}
	build := database.WorkspaceBuild{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
	}
	replicaID := uuid.New()

	uut := &agentWebsocketMonitor{
		apiCtx:            ctx,
		workspaceAgent:    agent,
		workspaceBuild:    build,
		conn:              fConn,
		db:                mDB,
		replicaID:         replicaID,
		updater:           fUpdater,
		logger:            logger,
		pingPeriod:        testutil.IntervalFast,
		disconnectTimeout: testutil.WaitShort,
	}
	uut.init()

	connected := mDB.EXPECT().UpdateWorkspaceAgentConnectionByID(
		gomock.Any(),
		connectionUpdate(agent.ID, replicaID),
	).
		AnyTimes().
		Return(nil)
	mDB.EXPECT().UpdateWorkspaceAgentConnectionByID(
		gomock.Any(),
		connectionUpdate(agent.ID, replicaID, withDisconnected()),
	).
		After(connected).
		Times(1).
		Return(nil)

	// return a new buildID each time, meaning the connection is outdated
	mDB.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), build.WorkspaceID).
		AnyTimes().
		Return(database.WorkspaceBuild{ID: uuid.New()}, nil)

	go uut.monitor(ctx)
	fConn.requireEventuallyClosed(t, websocket.StatusGoingAway, "build is outdated")
	fUpdater.requireEventuallySomeUpdates(t, build.WorkspaceID)
}

func TestAgentWebsocketMonitor_SendPings(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	fConn := &fakePingerCloser{}
	uut := &agentWebsocketMonitor{
		pingPeriod: testutil.IntervalFast,
		conn:       fConn,
	}
	go uut.sendPings(ctx)
	fConn.requireEventuallyHasPing(t)
	lastPing := uut.lastPing.Load()
	require.NotNil(t, lastPing)
}

func TestAgentWebsocketMonitor_StartClose(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	fConn := &fakePingerCloser{}
	now := dbtime.Now()
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	fUpdater := &fakeUpdater{}
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	agent := database.WorkspaceAgent{
		ID: uuid.New(),
		FirstConnectedAt: sql.NullTime{
			Time:  now.Add(-time.Minute),
			Valid: true,
		},
	}
	build := database.WorkspaceBuild{
		ID:          uuid.New(),
		WorkspaceID: uuid.New(),
	}
	replicaID := uuid.New()
	uut := &agentWebsocketMonitor{
		apiCtx:            ctx,
		workspaceAgent:    agent,
		workspaceBuild:    build,
		conn:              fConn,
		db:                mDB,
		replicaID:         replicaID,
		updater:           fUpdater,
		logger:            logger,
		pingPeriod:        testutil.IntervalFast,
		disconnectTimeout: testutil.WaitShort,
	}

	connected := mDB.EXPECT().UpdateWorkspaceAgentConnectionByID(
		gomock.Any(),
		connectionUpdate(agent.ID, replicaID),
	).
		AnyTimes().
		Return(nil)
	mDB.EXPECT().UpdateWorkspaceAgentConnectionByID(
		gomock.Any(),
		connectionUpdate(agent.ID, replicaID, withDisconnected()),
	).
		After(connected).
		Times(1).
		Return(nil)
	mDB.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), build.WorkspaceID).
		AnyTimes().
		Return(database.WorkspaceBuild{ID: build.ID}, nil)

	uut.start(ctx)
	closed := make(chan struct{})
	go func() {
		uut.close()
		close(closed)
	}()
	_ = testutil.RequireRecvCtx(ctx, t, closed)
}

type fakePingerCloser struct {
	sync.Mutex
	pings  []time.Time
	code   websocket.StatusCode
	reason string
	closed bool
}

func (f *fakePingerCloser) Ping(context.Context) error {
	f.Lock()
	defer f.Unlock()
	f.pings = append(f.pings, time.Now())
	return nil
}

func (f *fakePingerCloser) Close(code websocket.StatusCode, reason string) error {
	f.Lock()
	defer f.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	f.code = code
	f.reason = reason
	return nil
}

func (f *fakePingerCloser) requireNotClosed(t *testing.T) {
	f.Lock()
	defer f.Unlock()
	require.False(t, f.closed)
}

func (f *fakePingerCloser) requireEventuallyClosed(t *testing.T, code websocket.StatusCode, reason string) {
	require.Eventually(t, func() bool {
		f.Lock()
		defer f.Unlock()
		return f.closed
	}, testutil.WaitShort, testutil.IntervalFast)
	f.Lock()
	defer f.Unlock()
	require.Equal(t, code, f.code)
	require.Equal(t, reason, f.reason)
}

func (f *fakePingerCloser) requireEventuallyHasPing(t *testing.T) {
	require.Eventually(t, func() bool {
		f.Lock()
		defer f.Unlock()
		return len(f.pings) > 0
	}, testutil.WaitShort, testutil.IntervalFast)
}

type fakeUpdater struct {
	sync.Mutex
	updates []uuid.UUID
}

func (f *fakeUpdater) publishWorkspaceUpdate(_ context.Context, workspaceID uuid.UUID) {
	f.Lock()
	defer f.Unlock()
	f.updates = append(f.updates, workspaceID)
}

func (f *fakeUpdater) requireEventuallySomeUpdates(t *testing.T, workspaceID uuid.UUID) {
	require.Eventually(t, func() bool {
		f.Lock()
		defer f.Unlock()
		return len(f.updates) >= 1
	}, testutil.WaitShort, testutil.IntervalFast)

	f.Lock()
	defer f.Unlock()
	for _, u := range f.updates {
		require.Equal(t, workspaceID, u)
	}
}

func (f *fakeUpdater) getUpdates() int {
	f.Lock()
	defer f.Unlock()
	return len(f.updates)
}

type connectionUpdateMatcher struct {
	agentID      uuid.UUID
	replicaID    uuid.UUID
	disconnected bool
}

type connectionUpdateMatcherOption func(m connectionUpdateMatcher) connectionUpdateMatcher

func connectionUpdate(id, replica uuid.UUID, opts ...connectionUpdateMatcherOption) connectionUpdateMatcher {
	m := connectionUpdateMatcher{
		agentID:   id,
		replicaID: replica,
	}
	for _, opt := range opts {
		m = opt(m)
	}
	return m
}

func withDisconnected() connectionUpdateMatcherOption {
	return func(m connectionUpdateMatcher) connectionUpdateMatcher {
		m.disconnected = true
		return m
	}
}

func (m connectionUpdateMatcher) Matches(x interface{}) bool {
	args, ok := x.(database.UpdateWorkspaceAgentConnectionByIDParams)
	if !ok {
		return false
	}
	if args.ID != m.agentID {
		return false
	}
	if !args.LastConnectedReplicaID.Valid {
		return false
	}
	if args.LastConnectedReplicaID.UUID != m.replicaID {
		return false
	}
	if args.DisconnectedAt.Valid != m.disconnected {
		return false
	}
	return true
}

func (m connectionUpdateMatcher) String() string {
	return fmt.Sprintf("{agent=%s, replica=%s, disconnected=%t}",
		m.agentID.String(), m.replicaID.String(), m.disconnected)
}

func (connectionUpdateMatcher) Got(x interface{}) string {
	args, ok := x.(database.UpdateWorkspaceAgentConnectionByIDParams)
	if !ok {
		return fmt.Sprintf("type=%T", x)
	}
	return fmt.Sprintf("{agent=%s, replica=%s, disconnected=%t}",
		args.ID, args.LastConnectedReplicaID.UUID, args.DisconnectedAt.Valid)
}
