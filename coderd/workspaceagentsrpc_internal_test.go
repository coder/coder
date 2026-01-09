package coderd

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestAgentConnectionMonitor_ContextCancel(t *testing.T) {
	t.Parallel()
	now := dbtime.Now()
	agentID := uuid.UUID{1}
	replicaID := uuid.UUID{2}
	testCases := []struct {
		name           string
		agent          database.WorkspaceAgent
		initialMatcher connectionUpdateMatcher
	}{
		{
			name: "no disconnected at",
			agent: database.WorkspaceAgent{
				ID: agentID,
				FirstConnectedAt: sql.NullTime{
					Time:  now.Add(-time.Minute),
					Valid: true,
				},
			},
			initialMatcher: connectionUpdate(agentID, replicaID),
		},
		{
			name: "disconnected at",
			agent: database.WorkspaceAgent{
				ID: agentID,
				FirstConnectedAt: sql.NullTime{
					Time:  now.Add(-time.Minute),
					Valid: true,
				},
				DisconnectedAt: sql.NullTime{
					Time:  now.Add(-2 * time.Minute),
					Valid: true,
				},
			},
			initialMatcher: connectionUpdate(agentID, replicaID, withDisconnectedAt(now.Add(-2*time.Minute))),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			fConn := &fakePingerCloser{}
			ctrl := gomock.NewController(t)
			mDB := dbmock.NewMockStore(ctrl)
			fUpdater := &fakeUpdater{}
			logger := testutil.Logger(t)
			build := database.WorkspaceBuild{
				ID:          uuid.New(),
				WorkspaceID: uuid.New(),
			}

			uut := &agentConnectionMonitor{
				apiCtx:            ctx,
				workspaceAgent:    tc.agent,
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
				tc.initialMatcher,
			).
				AnyTimes().
				Return(nil)
			mDB.EXPECT().UpdateWorkspaceAgentConnectionByID(
				gomock.Any(),
				connectionUpdate(agentID, replicaID, withDisconnectedNotBefore(now)),
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
			_ = testutil.TryReceive(ctx, t, done)
			m := fUpdater.getUpdates()
			require.Greater(t, m, n)
		})
	}
}

func TestAgentConnectionMonitor_PingTimeout(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	now := dbtime.Now()
	fConn := &fakePingerCloser{}
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	fUpdater := &fakeUpdater{}
	logger := testutil.Logger(t)
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

	uut := &agentConnectionMonitor{
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
		connectionUpdate(agent.ID, replicaID, withDisconnectedNotBefore(now)),
	).
		After(connected).
		Times(1).
		Return(nil)
	mDB.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), build.WorkspaceID).
		AnyTimes().
		Return(database.WorkspaceBuild{ID: build.ID}, nil)

	done := make(chan struct{})
	go func() {
		uut.monitor(ctx)
		close(done)
	}()
	fConn.requireEventuallyClosed(t, websocket.StatusGoingAway, "ping timeout")
	fUpdater.requireEventuallySomeUpdates(t, build.WorkspaceID)
	_ = testutil.TryReceive(ctx, t, done) // ensure monitor() exits before mDB assertions are checked.
}

func TestAgentConnectionMonitor_BuildOutdated(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	now := dbtime.Now()
	fConn := &fakePingerCloser{}
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	fUpdater := &fakeUpdater{}
	logger := testutil.Logger(t)
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

	uut := &agentConnectionMonitor{
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
		connectionUpdate(agent.ID, replicaID, withDisconnectedNotBefore(now)),
	).
		After(connected).
		Times(1).
		Return(nil)

	// return a new buildID each time, meaning the connection is outdated
	mDB.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), build.WorkspaceID).
		AnyTimes().
		Return(database.WorkspaceBuild{ID: uuid.New()}, nil)

	done := make(chan struct{})
	go func() {
		uut.monitor(ctx)
		close(done)
	}()
	fConn.requireEventuallyClosed(t, websocket.StatusGoingAway, "build is outdated")
	fUpdater.requireEventuallySomeUpdates(t, build.WorkspaceID)
	_ = testutil.TryReceive(ctx, t, done) // ensure monitor() exits before mDB assertions are checked.
}

func TestAgentConnectionMonitor_SendPings(t *testing.T) {
	t.Parallel()
	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	t.Cleanup(cancel)
	fConn := &fakePingerCloser{}
	uut := &agentConnectionMonitor{
		pingPeriod: testutil.IntervalFast,
		conn:       fConn,
	}
	done := make(chan struct{})
	go func() {
		uut.sendPings(ctx)
		close(done)
	}()
	fConn.requireEventuallyHasPing(t)
	cancel()
	_ = testutil.TryReceive(testCtx, t, done)
	lastPing := uut.lastPing.Load()
	require.NotNil(t, lastPing)
}

func TestAgentConnectionMonitor_StartClose(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	fConn := &fakePingerCloser{}
	now := dbtime.Now()
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	fUpdater := &fakeUpdater{}
	logger := testutil.Logger(t)
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
	uut := &agentConnectionMonitor{
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
		connectionUpdate(agent.ID, replicaID, withDisconnectedNotBefore(now)),
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
	_ = testutil.TryReceive(ctx, t, closed)
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

func (f *fakeUpdater) publishWorkspaceUpdate(_ context.Context, _ uuid.UUID, event wspubsub.WorkspaceEvent) {
	f.Lock()
	defer f.Unlock()
	f.updates = append(f.updates, event.WorkspaceID)
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
	agentID               uuid.UUID
	replicaID             uuid.UUID
	disconnectedAt        sql.NullTime
	disconnectedNotBefore sql.NullTime
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

func withDisconnectedNotBefore(t time.Time) connectionUpdateMatcherOption {
	return func(m connectionUpdateMatcher) connectionUpdateMatcher {
		m.disconnectedNotBefore = sql.NullTime{
			Valid: true,
			Time:  t,
		}
		return m
	}
}

func withDisconnectedAt(t time.Time) connectionUpdateMatcherOption {
	return func(m connectionUpdateMatcher) connectionUpdateMatcher {
		m.disconnectedAt = sql.NullTime{
			Valid: true,
			Time:  t,
		}
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
	if m.disconnectedNotBefore.Valid {
		if !args.DisconnectedAt.Valid {
			return false
		}
		if args.DisconnectedAt.Time.Before(m.disconnectedNotBefore.Time) {
			return false
		}
		// disconnectedNotBefore takes precedence over disconnectedAt
	} else if args.DisconnectedAt != m.disconnectedAt {
		return false
	}
	return true
}

func (m connectionUpdateMatcher) String() string {
	return fmt.Sprintf("{agent=%s, replica=%s, disconnectedAt=%v, disconnectedNotBefore=%v}",
		m.agentID.String(), m.replicaID.String(), m.disconnectedAt, m.disconnectedNotBefore)
}

func (connectionUpdateMatcher) Got(x interface{}) string {
	args, ok := x.(database.UpdateWorkspaceAgentConnectionByIDParams)
	if !ok {
		return fmt.Sprintf("type=%T", x)
	}
	return fmt.Sprintf("{agent=%s, replica=%s, disconnectedAt=%v}",
		args.ID, args.LastConnectedReplicaID.UUID, args.DisconnectedAt)
}
