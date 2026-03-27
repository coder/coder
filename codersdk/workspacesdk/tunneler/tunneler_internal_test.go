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
							ctrl := gomock.NewController(t)
							mAgentConn := agentconnmock.NewMockAgentConn(ctrl)
							logger := testutil.Logger(t)

							testCtx := testutil.Context(t, testutil.WaitShort)
							ctx, cancel := context.WithCancel(testCtx)
							uut := &Tunneler{
								config: Config{
									WorkspaceID:      workspaceID,
									App:              fakeApp{},
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

							uut.handleBuildUpdate(&buildUpdate{transition: trans, jobStatus: jobStatus})
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
						})
					}
				}
			}
		}
	}
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
			App:              fakeApp{},
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
			App:              fakeApp{},
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
					App:              fakeApp{},
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
			App:              fakeApp{},
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

func waitForGoroutines(ctx context.Context, t *testing.T, tunneler *Tunneler) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		tunneler.wg.Wait()
	}()
	_ = testutil.TryReceive(ctx, t, done)
}

type fakeWorkspaceStarter struct {
	started bool
}

func (f *fakeWorkspaceStarter) StartWorkspace() error {
	f.started = true
	return nil
}

type fakeApp struct{}

func (fakeApp) Close() error {
	return nil
}

func (fakeApp) Start(workspacesdk.AgentConn) {}
