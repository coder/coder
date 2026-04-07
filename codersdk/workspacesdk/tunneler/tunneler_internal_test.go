package tunneler

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

// TestHandleBuildUpdate_Coverage ensures that we handle all possible initial states in combination with build updates.
func TestHandleBuildUpdate_Coverage(t *testing.T) {
	t.Parallel()
	workspaceID := uuid.UUID{1}

	for s := range maxState {
		for _, trans := range codersdk.WorkspaceTransitionEnums() {
			for _, jobStatus := range codersdk.ProvisionerJobStatusEnums() {
				for _, noAutostart := range []bool{true, false} {
					for _, noWaitForScripts := range []bool{true, false} {
						t.Run(fmt.Sprintf("%d_%s_%s_%t_%t", s, trans, jobStatus, noAutostart, noWaitForScripts), func(t *testing.T) {
							t.Parallel()
							coverUpdate(t, workspaceID, noAutostart, noWaitForScripts, s, func(uut *Tunneler) {
								uut.handleBuildUpdate(&buildUpdate{transition: trans, jobStatus: jobStatus})
							})
						})
					}
				}
			}
		}
	}
}

func coverUpdate(t *testing.T, workspaceID uuid.UUID, noAutostart bool, noWaitForScripts bool, s state, update func(uut *Tunneler)) {
	ctrl := gomock.NewController(t)
	mAgentConn := agentconnmock.NewMockAgentConn(ctrl)
	logger := testutil.Logger(t)
	fClient := &fakeClient{conn: mAgentConn}

	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	uut := &Tunneler{
		client: fClient,
		config: Config{
			WorkspaceID:      workspaceID,
			App:              &fakeApp{},
			WorkspaceStarter: &fakeWorkspaceStarter{},
			AgentName:        "test",
			NoAutostart:      noAutostart,
			NoWaitForScripts: noWaitForScripts,
			DebugLogger:      logger.Named("tunneler"),
		},
		events:    make(chan tunnelerEvent),
		ctx:       ctx,
		cancel:    cancel,
		state:     s,
		agentConn: mAgentConn,
	}

	mAgentConn.EXPECT().Close().Return(nil).AnyTimes()

	update(uut)
	done := make(chan struct{})
	go func() {
		defer close(done)
		uut.wg.Wait()
	}()
	cancel() // cancel in case the update triggers a go routine that writes another event
	// ensure we don't leak a go routine
	_ = testutil.TryReceive(testCtx, t, done)

	// We're not asserting the resulting state, as there are just too many to directly enumerate
	// due to the combinations. Unhandled cases will hit a critical log in the handler and fail
	// the test.
	require.Less(t, uut.state, maxState)
	require.GreaterOrEqual(t, uut.state, 0)
}

func TestBuildUpdatesStoppedWorkspace(t *testing.T) {
	t.Parallel()
	workspaceID := uuid.UUID{1}
	logger := testutil.Logger(t)
	fWorkspaceStarter := fakeWorkspaceStarter{}

	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	uut := &Tunneler{
		config: Config{
			WorkspaceID:      workspaceID,
			App:              &fakeApp{},
			WorkspaceStarter: &fWorkspaceStarter,
			AgentName:        "test",
			DebugLogger:      logger.Named("tunneler"),
		},
		events: make(chan tunnelerEvent),
		ctx:    ctx,
		cancel: cancel,
		state:  stateInit,
	}

	uut.handleBuildUpdate(&buildUpdate{transition: codersdk.WorkspaceTransitionStop, jobStatus: codersdk.ProvisionerJobPending})
	require.Equal(t, waitToStart, uut.state)
	waitForGoroutines(testCtx, t, uut)
	require.False(t, fWorkspaceStarter.started)

	uut.handleBuildUpdate(&buildUpdate{transition: codersdk.WorkspaceTransitionStop, jobStatus: codersdk.ProvisionerJobRunning})
	require.Equal(t, waitToStart, uut.state)
	waitForGoroutines(testCtx, t, uut)
	require.False(t, fWorkspaceStarter.started)

	// when stop job succeeds, we start the workspace
	uut.handleBuildUpdate(&buildUpdate{transition: codersdk.WorkspaceTransitionStop, jobStatus: codersdk.ProvisionerJobSucceeded})
	require.Equal(t, waitForWorkspaceStarted, uut.state)
	waitForGoroutines(testCtx, t, uut)
	require.True(t, fWorkspaceStarter.started)

	uut.handleBuildUpdate(&buildUpdate{transition: codersdk.WorkspaceTransitionStart, jobStatus: codersdk.ProvisionerJobPending})
	require.Equal(t, waitForWorkspaceStarted, uut.state)
	waitForGoroutines(testCtx, t, uut)

	uut.handleBuildUpdate(&buildUpdate{transition: codersdk.WorkspaceTransitionStart, jobStatus: codersdk.ProvisionerJobRunning})
	require.Equal(t, waitForWorkspaceStarted, uut.state)
	waitForGoroutines(testCtx, t, uut)

	uut.handleBuildUpdate(&buildUpdate{transition: codersdk.WorkspaceTransitionStart, jobStatus: codersdk.ProvisionerJobSucceeded})
	require.Equal(t, waitForAgent, uut.state)
	waitForGoroutines(testCtx, t, uut)
}

func TestBuildUpdatesNewBuildWhileWaiting(t *testing.T) {
	t.Parallel()
	workspaceID := uuid.UUID{1}
	logger := testutil.Logger(t)
	fWorkspaceStarter := fakeWorkspaceStarter{}

	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	uut := &Tunneler{
		config: Config{
			WorkspaceID:      workspaceID,
			App:              &fakeApp{},
			WorkspaceStarter: &fWorkspaceStarter,
			AgentName:        "test",
			DebugLogger:      logger.Named("tunneler"),
		},
		events: make(chan tunnelerEvent),
		ctx:    ctx,
		cancel: cancel,
		state:  waitForAgent,
	}

	// New build comes in while we are waiting for the agent to start. We roll back to waiting for the workspace to start.
	uut.handleBuildUpdate(&buildUpdate{transition: codersdk.WorkspaceTransitionStart, jobStatus: codersdk.ProvisionerJobRunning})
	require.Equal(t, waitForWorkspaceStarted, uut.state)
	waitForGoroutines(testCtx, t, uut)
	require.False(t, fWorkspaceStarter.started)
}

func TestBuildUpdatesBadJobs(t *testing.T) {
	t.Parallel()
	for _, jobStatus := range []codersdk.ProvisionerJobStatus{
		codersdk.ProvisionerJobFailed,
		codersdk.ProvisionerJobCanceling,
		codersdk.ProvisionerJobCanceled,
		codersdk.ProvisionerJobUnknown,
	} {
		t.Run(string(jobStatus), func(t *testing.T) {
			t.Parallel()
			workspaceID := uuid.UUID{1}
			logger := testutil.Logger(t)
			fWorkspaceStarter := fakeWorkspaceStarter{}

			testCtx := testutil.Context(t, testutil.WaitShort)
			ctx, cancel := context.WithCancel(testCtx)
			uut := &Tunneler{
				config: Config{
					WorkspaceID:      workspaceID,
					App:              &fakeApp{},
					WorkspaceStarter: &fWorkspaceStarter,
					AgentName:        "test",
					DebugLogger:      logger.Named("tunneler"),
				},
				events: make(chan tunnelerEvent),
				ctx:    ctx,
				cancel: cancel,
				state:  stateInit,
			}

			uut.handleBuildUpdate(&buildUpdate{transition: codersdk.WorkspaceTransitionStart, jobStatus: codersdk.ProvisionerJobRunning})
			require.Equal(t, waitForWorkspaceStarted, uut.state)
			waitForGoroutines(testCtx, t, uut)
			require.False(t, fWorkspaceStarter.started)

			uut.handleBuildUpdate(&buildUpdate{transition: codersdk.WorkspaceTransitionStop, jobStatus: jobStatus})
			require.Equal(t, exit, uut.state)
			waitForGoroutines(testCtx, t, uut)
			require.False(t, fWorkspaceStarter.started)

			// should cancel
			require.Error(t, ctx.Err())
		})
	}
}

func TestBuildUpdatesNoAutostart(t *testing.T) {
	t.Parallel()
	workspaceID := uuid.UUID{1}
	logger := testutil.Logger(t)
	fWorkspaceStarter := fakeWorkspaceStarter{}

	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	uut := &Tunneler{
		config: Config{
			WorkspaceID:      workspaceID,
			App:              &fakeApp{},
			WorkspaceStarter: &fWorkspaceStarter,
			AgentName:        "test",
			NoAutostart:      true,
			DebugLogger:      logger.Named("tunneler"),
		},
		events: make(chan tunnelerEvent),
		ctx:    ctx,
		cancel: cancel,
		state:  stateInit,
	}

	// when stop job succeeds, we exit because autostart is disabled
	uut.handleBuildUpdate(&buildUpdate{transition: codersdk.WorkspaceTransitionStop, jobStatus: codersdk.ProvisionerJobSucceeded})
	require.Equal(t, exit, uut.state)
	waitForGoroutines(testCtx, t, uut)
	require.False(t, fWorkspaceStarter.started)

	// should cancel
	require.Error(t, ctx.Err())
}

func TestAgentUpdate_Coverage(t *testing.T) {
	t.Parallel()
	workspaceID := uuid.UUID{1}
	agentID := uuid.UUID{2}

	for s := range maxState {
		for _, lifecycle := range codersdk.WorkspaceAgentLifecycleOrder {
			for _, noAutostart := range []bool{true, false} {
				for _, noWaitForScripts := range []bool{true, false} {
					t.Run(fmt.Sprintf("%d_%s_%t_%t", s, lifecycle, noAutostart, noWaitForScripts), func(t *testing.T) {
						t.Parallel()
						coverUpdate(t, workspaceID, noAutostart, noWaitForScripts, s, func(uut *Tunneler) {
							uut.handleAgentUpdate(&agentUpdate{lifecycle: lifecycle, id: agentID})
						})
					})
				}
			}
		}
	}
}

func TestAgentUpdateReady(t *testing.T) {
	t.Parallel()
	workspaceID := uuid.UUID{1}
	agentID := uuid.UUID{2}
	logger := testutil.Logger(t)

	ctrl := gomock.NewController(t)
	mAgentConn := agentconnmock.NewMockAgentConn(ctrl)
	fClient := &fakeClient{conn: mAgentConn}

	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	uut := &Tunneler{
		config: Config{
			WorkspaceID: workspaceID,
			AgentName:   "test",
			DebugLogger: logger.Named("tunneler"),
		},
		events: make(chan tunnelerEvent),
		ctx:    ctx,
		cancel: cancel,
		state:  waitForAgent,
		client: fClient,
	}

	uut.handleAgentUpdate(&agentUpdate{lifecycle: codersdk.WorkspaceAgentLifecycleReady, id: agentID})
	require.Equal(t, establishTailnet, uut.state)
	event := testutil.RequireReceive(testCtx, t, uut.events)
	require.NotNil(t, event.tailnetUpdate)
	require.True(t, fClient.dialed)
	require.Equal(t, mAgentConn, event.tailnetUpdate.conn)
	require.True(t, event.tailnetUpdate.up)
}

func TestAgentUpdateNoWait(t *testing.T) {
	t.Parallel()
	workspaceID := uuid.UUID{1}
	agentID := uuid.UUID{2}
	logger := testutil.Logger(t)

	ctrl := gomock.NewController(t)
	mAgentConn := agentconnmock.NewMockAgentConn(ctrl)
	fClient := &fakeClient{conn: mAgentConn}

	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	uut := &Tunneler{
		config: Config{
			WorkspaceID:      workspaceID,
			AgentName:        "test",
			DebugLogger:      logger.Named("tunneler"),
			NoWaitForScripts: true,
		},
		events: make(chan tunnelerEvent),
		ctx:    ctx,
		cancel: cancel,
		state:  waitForAgent,
		client: fClient,
	}

	uut.handleAgentUpdate(&agentUpdate{lifecycle: codersdk.WorkspaceAgentLifecycleStarting, id: agentID})
	require.Equal(t, establishTailnet, uut.state)
	event := testutil.RequireReceive(testCtx, t, uut.events)
	require.NotNil(t, event.tailnetUpdate)
	require.True(t, fClient.dialed)
	require.Equal(t, mAgentConn, event.tailnetUpdate.conn)
	require.True(t, event.tailnetUpdate.up)
}

func TestAppUpdate(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                                  string
		up                                    bool
		initState, expected                   state
		expectCloseApp, expectShutdownTailnet bool
	}{
		{
			name:      "mainline_up",
			up:        true,
			initState: tailnetUp,
			expected:  applicationUp,
		},
		{
			name:                  "mainline_down",
			up:                    false,
			initState:             applicationUp,
			expected:              shutdownTailnet,
			expectShutdownTailnet: true,
		},
		{
			name:                  "failed_app_start",
			up:                    false,
			initState:             tailnetUp,
			expected:              shutdownTailnet,
			expectShutdownTailnet: true,
		},
		{
			name:           "graceful_shutdown_while_starting",
			up:             true,
			initState:      shutdownApplication,
			expected:       shutdownApplication,
			expectCloseApp: true,
		},
		{
			name:                  "graceful_shutdown_of_app",
			up:                    false,
			initState:             shutdownApplication,
			expected:              shutdownTailnet,
			expectShutdownTailnet: true,
		},
		// note that we don't expect initState: applicationUp with an up update, so only five valid cases
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			workspaceID := uuid.UUID{1}
			logger := testutil.Logger(t)

			ctrl := gomock.NewController(t)
			mAgentConn := agentconnmock.NewMockAgentConn(ctrl)
			fApp := &fakeApp{}

			testCtx := testutil.Context(t, testutil.WaitShort)
			ctx, cancel := context.WithCancel(testCtx)
			uut := &Tunneler{
				config: Config{
					WorkspaceID: workspaceID,
					AgentName:   "test",
					DebugLogger: logger.Named("tunneler"),
					App:         fApp,
				},
				events:    make(chan tunnelerEvent),
				ctx:       ctx,
				cancel:    cancel,
				state:     tc.initState,
				agentConn: mAgentConn,
			}
			if tc.expectShutdownTailnet {
				mAgentConn.EXPECT().Close().Return(nil).Times(1)
			}

			uut.handleAppUpdate(&networkedApplicationUpdate{up: tc.up})
			require.Equal(t, tc.expected, uut.state)
			cancel() // so that any goroutines can complete without an event loop
			waitForGoroutines(testCtx, t, uut)
			require.Equal(t, tc.expectCloseApp, fApp.closed)
		})
	}
}

func TestTailnetUpdate(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                                  string
		up                                    bool
		initState, expected                   state
		expectStartApp, expectShutdownTailnet bool
	}{
		{
			name:           "mainline_up",
			up:             true,
			initState:      establishTailnet,
			expected:       tailnetUp,
			expectStartApp: true,
		},
		{
			name:      "mainline_down",
			up:        false,
			initState: shutdownTailnet,
			expected:  exit,
		},
		{
			name:      "failed_tailnet_start",
			up:        false,
			initState: establishTailnet,
			expected:  exit,
		},
		{
			name:                  "graceful_shutdown_while_starting",
			up:                    true,
			initState:             shutdownTailnet,
			expected:              shutdownTailnet,
			expectShutdownTailnet: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			workspaceID := uuid.UUID{1}
			logger := testutil.Logger(t)

			ctrl := gomock.NewController(t)
			mAgentConn := agentconnmock.NewMockAgentConn(ctrl)
			fApp := &fakeApp{}

			testCtx := testutil.Context(t, testutil.WaitShort)
			ctx, cancel := context.WithCancel(testCtx)
			uut := &Tunneler{
				config: Config{
					WorkspaceID: workspaceID,
					AgentName:   "test",
					DebugLogger: logger.Named("tunneler"),
					App:         fApp,
				},
				events: make(chan tunnelerEvent),
				ctx:    ctx,
				cancel: cancel,
				state:  tc.initState,
			}
			if tc.expectShutdownTailnet {
				mAgentConn.EXPECT().Close().Return(nil).Times(1)
			}

			update := &tailnetUpdate{up: tc.up}
			if tc.up {
				update.conn = mAgentConn
			}
			uut.handleTailnetUpdate(update)
			require.Equal(t, tc.expected, uut.state)
			cancel() // so that any goroutines can complete without an event loop
			waitForGoroutines(testCtx, t, uut)
			require.Equal(t, tc.expectStartApp, fApp.started)
		})
	}
}

func TestTunneler_EventLoop_Signal(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.UUID{1}
	agentID := uuid.UUID{2}
	logger := testutil.Logger(t)

	ctrl := gomock.NewController(t)
	mAgentConn := agentconnmock.NewMockAgentConn(ctrl)
	fApp := &fakeApp{
		starts: make(chan appStartRequest),
		closes: make(chan errorResult),
	}
	fClient := &fakeClient{
		dials: make(chan dialRequest),
	}

	testCtx := testutil.Context(t, testutil.WaitShort)
	ctx, cancel := context.WithCancel(testCtx)
	uut := &Tunneler{
		client: fClient,
		config: Config{
			WorkspaceID: workspaceID,
			AgentName:   "test",
			DebugLogger: logger.Named("tunneler"),
			App:         fApp,
		},
		events: make(chan tunnelerEvent),
		ctx:    ctx,
		cancel: cancel,
		state:  stateInit,
	}
	uut.wg.Add(1)
	go uut.eventLoop()

	testutil.RequireSend(testCtx, t, uut.events, tunnelerEvent{
		buildUpdate: &buildUpdate{
			transition: codersdk.WorkspaceTransitionStart,
			jobStatus:  codersdk.ProvisionerJobPending,
		},
	})
	testutil.RequireSend(testCtx, t, uut.events, tunnelerEvent{
		buildUpdate: &buildUpdate{
			transition: codersdk.WorkspaceTransitionStart,
			jobStatus:  codersdk.ProvisionerJobRunning,
		},
	})
	testutil.RequireSend(testCtx, t, uut.events, tunnelerEvent{
		buildUpdate: &buildUpdate{
			transition: codersdk.WorkspaceTransitionStart,
			jobStatus:  codersdk.ProvisionerJobSucceeded,
		},
	})
	testutil.RequireSend(testCtx, t, uut.events, tunnelerEvent{
		agentUpdate: &agentUpdate{
			lifecycle: codersdk.WorkspaceAgentLifecycleReady,
			id:        agentID,
		},
	})

	// Workspace started, agent ready. Should connect the tailnet.
	tailnetDial := testutil.RequireReceive(testCtx, t, fClient.dials)
	testutil.RequireSend(testCtx, t, tailnetDial.result, dialResult{conn: mAgentConn})

	// Tailnet up, should start App
	appStart := testutil.RequireReceive(testCtx, t, fApp.starts)
	require.Equal(t, mAgentConn, appStart.conn)
	testutil.RequireSend(testCtx, t, appStart.result, nil)

	connClosed := make(chan struct{})
	mAgentConn.EXPECT().Close().Times(1).Do(func() {
		close(connClosed)
	}).Return(nil)

	testutil.RequireSend(testCtx, t, uut.events, tunnelerEvent{
		shutdownSignal: &shutdownSignal{},
	})

	closeReq := testutil.RequireReceive(testCtx, t, fApp.closes)
	testutil.RequireSend(testCtx, t, closeReq.result, nil)

	// next tailnet closes
	_ = testutil.TryReceive(testCtx, t, connClosed)

	// should cancel the loop and be at exit
	waitForGoroutines(testCtx, t, uut)
	require.Equal(t, exit, uut.state)
}

func waitForGoroutines(ctx context.Context, t *testing.T, tunneler *Tunneler) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		tunneler.wg.Wait()
	}()
	_ = testutil.TryReceive(ctx, t, done)
}

type errorResult struct {
	result chan error
}

type fakeWorkspaceStarter struct {
	starts  chan errorResult
	started bool
}

func (f *fakeWorkspaceStarter) StartWorkspace() error {
	if f.starts == nil {
		f.started = true
		return nil
	}
	result := make(chan error)
	f.starts <- errorResult{result: result}
	return <-result
}

type appStartRequest struct {
	conn   workspacesdk.AgentConn
	result chan error
}

type fakeApp struct {
	starts  chan appStartRequest
	closes  chan errorResult
	closed  bool
	started bool
}

func (f *fakeApp) Close() error {
	if f.closes == nil {
		f.closed = true
		return nil
	}
	result := make(chan error)
	f.closes <- errorResult{result: result}
	return <-result
}

func (f *fakeApp) Start(conn workspacesdk.AgentConn) error {
	if f.starts == nil {
		f.started = true
		return nil
	}
	result := make(chan error)
	f.starts <- appStartRequest{result: result, conn: conn}
	return <-result
}

type dialRequest struct {
	id     uuid.UUID
	result chan dialResult
}

type dialResult struct {
	conn workspacesdk.AgentConn
	err  error
}

type fakeClient struct {
	// async:
	dials chan dialRequest

	// sync:
	conn   workspacesdk.AgentConn
	dialed bool
}

func (f *fakeClient) DialAgent(
	_ context.Context, id uuid.UUID, _ *workspacesdk.DialAgentOptions,
) (
	workspacesdk.AgentConn, error,
) {
	if f.dials == nil {
		f.dialed = true
		return f.conn, nil
	}
	results := make(chan dialResult)
	f.dials <- dialRequest{id: id, result: results}
	result := <-results
	return result.conn, result.err
}
