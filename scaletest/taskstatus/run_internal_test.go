package taskstatus

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// fakeClient implements the client interface for testing
type fakeClient struct {
	t      *testing.T
	logger slog.Logger

	// Channels for controlling the behavior
	workspaceUpdatesCh chan codersdk.Workspace
}

func newFakeClient(t *testing.T) *fakeClient {
	return &fakeClient{
		t:                  t,
		workspaceUpdatesCh: make(chan codersdk.Workspace),
	}
}

func (m *fakeClient) initialize(logger slog.Logger) {
	m.logger = logger
}

func (m *fakeClient) watchWorkspace(ctx context.Context, workspaceID uuid.UUID) (<-chan codersdk.Workspace, error) {
	m.logger.Debug(ctx, "called fake WatchWorkspace", slog.F("workspace_id", workspaceID.String()))
	return m.workspaceUpdatesCh, nil
}

const testAgentToken = "test-agent-token"

func (m *fakeClient) createExternalWorkspace(ctx context.Context, req codersdk.CreateWorkspaceRequest) (createExternalWorkspaceResult, error) {
	m.logger.Debug(ctx, "called fake CreateExternalWorkspace", slog.F("req", req))
	// Return a fake workspace ID and token for testing
	return createExternalWorkspaceResult{
		WorkspaceID: uuid.UUID{1, 2, 3, 4}, // Fake workspace ID
		AgentToken:  testAgentToken,
	}, nil
}

// fakeAppStatusPatcher implements the appStatusPatcher interface for testing
type fakeAppStatusPatcher struct {
	t          *testing.T
	logger     slog.Logger
	agentToken string

	// Channels for controlling the behavior
	patchStatusCalls  chan agentsdk.PatchAppStatus
	patchStatusErrors chan error
}

func newFakeAppStatusPatcher(t *testing.T) *fakeAppStatusPatcher {
	return &fakeAppStatusPatcher{
		t:                 t,
		patchStatusCalls:  make(chan agentsdk.PatchAppStatus),
		patchStatusErrors: make(chan error, 1),
	}
}

func (p *fakeAppStatusPatcher) initialize(logger slog.Logger, agentToken string) {
	p.logger = logger
	p.agentToken = agentToken
}

func (p *fakeAppStatusPatcher) patchAppStatus(ctx context.Context, req agentsdk.PatchAppStatus) error {
	assert.NotEmpty(p.t, p.agentToken)
	p.logger.Debug(ctx, "called fake PatchAppStatus", slog.F("req", req))
	// Send the request to the channel so tests can verify it
	select {
	case p.patchStatusCalls <- req:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Check if there's an error to return
	select {
	case err := <-p.patchStatusErrors:
		return err
	default:
		return nil
	}
}

func TestRunner_Run(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	mClock := quartz.NewMock(t)
	fClient := newFakeClient(t)
	fPatcher := newFakeAppStatusPatcher(t)
	templateID := uuid.UUID{5, 6, 7, 8}
	workspaceName := "test-workspace"
	appSlug := "test-app"

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg, "test")

	connectedWaitGroup := &sync.WaitGroup{}
	connectedWaitGroup.Add(1)
	startReporting := make(chan struct{})

	cfg := Config{
		TemplateID:           templateID,
		WorkspaceName:        workspaceName,
		AppSlug:              appSlug,
		ConnectedWaitGroup:   connectedWaitGroup,
		StartReporting:       startReporting,
		ReportStatusPeriod:   10 * time.Second,
		ReportStatusDuration: 35 * time.Second,
		Metrics:              metrics,
		MetricLabelValues:    []string{"test"},
	}
	runner := &Runner{
		client:      fClient,
		patcher:     fPatcher,
		cfg:         cfg,
		clock:       mClock,
		reportTimes: make(map[int]time.Time),
	}

	tickerTrap := mClock.Trap().TickerFunc("reportTaskStatus")
	defer tickerTrap.Close()
	sinceTrap := mClock.Trap().Since("watchWorkspaceUpdates")
	defer sinceTrap.Close()

	// Run the runner in a goroutine
	runErr := make(chan error, 1)
	go func() {
		runErr <- runner.Run(ctx, "test-runner", testutil.NewTestLogWriter(t))
	}()

	// Wait for the runner to connect and watch workspace
	connectedWaitGroup.Wait()

	// Signal to start reporting
	close(startReporting)

	// Wait for the initial TickerFunc call before advancing time, otherwise our ticks will be off.
	tickerTrap.MustWait(ctx).MustRelease(ctx)

	// at this point, the patcher must be initialized
	require.Equal(t, testAgentToken, fPatcher.agentToken)

	updateDelay := time.Duration(0)
	for i := 0; i < 4; i++ {
		tickWaiter := mClock.Advance((10 * time.Second) - updateDelay)

		patchCall := testutil.RequireReceive(ctx, t, fPatcher.patchStatusCalls)
		require.Equal(t, appSlug, patchCall.AppSlug)
		require.Equal(t, fmt.Sprintf("scaletest status update:%d", i), patchCall.Message)
		require.Equal(t, codersdk.WorkspaceAppStatusStateWorking, patchCall.State)
		tickWaiter.MustWait(ctx)

		// Send workspace update 1, 2, 3, or 4 seconds after the report
		updateDelay = time.Duration(i+1) * time.Second
		mClock.Advance(updateDelay)

		workspace := codersdk.Workspace{
			LatestAppStatus: &codersdk.WorkspaceAppStatus{
				Message: fmt.Sprintf("scaletest status update:%d", i),
			},
		}
		testutil.RequireSend(ctx, t, fClient.workspaceUpdatesCh, workspace)
		sinceTrap.MustWait(ctx).MustRelease(ctx)
	}

	// Wait for the runner to complete
	err := testutil.RequireReceive(ctx, t, runErr)
	require.NoError(t, err)

	// Verify metrics were updated correctly
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	var latencyMetricFound bool
	var missingUpdatesFound bool
	for _, mf := range metricFamilies {
		switch mf.GetName() {
		case "coderd_scaletest_task_status_to_workspace_update_latency_seconds":
			latencyMetricFound = true
			require.Len(t, mf.GetMetric(), 1)
			hist := mf.GetMetric()[0].GetHistogram()
			assert.Equal(t, uint64(4), hist.GetSampleCount())
		case "coderd_scaletest_missing_status_updates_total":
			missingUpdatesFound = true
			require.Len(t, mf.GetMetric(), 1)
			counter := mf.GetMetric()[0].GetCounter()
			assert.Equal(t, float64(0), counter.GetValue())
		}
	}
	assert.True(t, latencyMetricFound, "latency metric not found")
	assert.True(t, missingUpdatesFound, "missing updates metric not found")
}

func TestRunner_RunMissedUpdate(t *testing.T) {
	t.Parallel()

	testCtx := testutil.Context(t, testutil.WaitShort)
	runCtx, cancel := context.WithCancel(testCtx)
	defer cancel()

	mClock := quartz.NewMock(t)
	fClient := newFakeClient(t)
	fPatcher := newFakeAppStatusPatcher(t)
	templateID := uuid.UUID{5, 6, 7, 8}
	workspaceName := "test-workspace"
	appSlug := "test-app"

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg, "test")

	connectedWaitGroup := &sync.WaitGroup{}
	connectedWaitGroup.Add(1)
	startReporting := make(chan struct{})

	cfg := Config{
		TemplateID:           templateID,
		WorkspaceName:        workspaceName,
		AppSlug:              appSlug,
		ConnectedWaitGroup:   connectedWaitGroup,
		StartReporting:       startReporting,
		ReportStatusPeriod:   10 * time.Second,
		ReportStatusDuration: 35 * time.Second,
		Metrics:              metrics,
		MetricLabelValues:    []string{"test"},
	}
	runner := &Runner{
		client:      fClient,
		patcher:     fPatcher,
		cfg:         cfg,
		clock:       mClock,
		reportTimes: make(map[int]time.Time),
	}

	tickerTrap := mClock.Trap().TickerFunc("reportTaskStatus")
	defer tickerTrap.Close()
	sinceTrap := mClock.Trap().Since("watchWorkspaceUpdates")
	defer sinceTrap.Close()

	// Run the runner in a goroutine
	runErr := make(chan error, 1)
	go func() {
		runErr <- runner.Run(runCtx, "test-runner", testutil.NewTestLogWriter(t))
	}()

	// Wait for the runner to connect and watch workspace
	connectedWaitGroup.Wait()

	// Signal to start reporting
	close(startReporting)

	// Wait for the initial TickerFunc call before advancing time, otherwise our ticks will be off.
	tickerTrap.MustWait(testCtx).MustRelease(testCtx)

	updateDelay := time.Duration(0)
	for i := 0; i < 4; i++ {
		tickWaiter := mClock.Advance((10 * time.Second) - updateDelay)
		patchCall := testutil.RequireReceive(testCtx, t, fPatcher.patchStatusCalls)
		require.Equal(t, appSlug, patchCall.AppSlug)
		require.Equal(t, fmt.Sprintf("scaletest status update:%d", i), patchCall.Message)
		require.Equal(t, codersdk.WorkspaceAppStatusStateWorking, patchCall.State)
		tickWaiter.MustWait(testCtx)

		// Send workspace update 1, 2, 3, or 4 seconds after the report
		updateDelay = time.Duration(i+1) * time.Second
		mClock.Advance(updateDelay)

		workspace := codersdk.Workspace{
			LatestAppStatus: &codersdk.WorkspaceAppStatus{
				Message: fmt.Sprintf("scaletest status update:%d", i),
			},
		}
		if i != 2 {
			// skip the third update, to test that we report missed updates and still complete.
			testutil.RequireSend(testCtx, t, fClient.workspaceUpdatesCh, workspace)
			sinceTrap.MustWait(testCtx).MustRelease(testCtx)
		}
	}

	// Cancel the run context to simulate the runner being killed.
	cancel()

	// Wait for the runner to complete
	err := testutil.RequireReceive(testCtx, t, runErr)
	require.ErrorIs(t, err, context.Canceled)

	// Verify metrics were updated correctly
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	// Check that metrics were recorded
	var latencyMetricFound bool
	var missingUpdatesFound bool
	for _, mf := range metricFamilies {
		switch mf.GetName() {
		case "coderd_scaletest_task_status_to_workspace_update_latency_seconds":
			latencyMetricFound = true
			require.Len(t, mf.GetMetric(), 1)
			hist := mf.GetMetric()[0].GetHistogram()
			assert.Equal(t, uint64(3), hist.GetSampleCount())
		case "coderd_scaletest_missing_status_updates_total":
			missingUpdatesFound = true
			require.Len(t, mf.GetMetric(), 1)
			counter := mf.GetMetric()[0].GetCounter()
			assert.Equal(t, float64(1), counter.GetValue())
		}
	}
	assert.True(t, latencyMetricFound, "latency metric not found")
	assert.True(t, missingUpdatesFound, "missing updates metric not found")
}

func TestRunner_Run_WithErrors(t *testing.T) {
	t.Parallel()

	testCtx := testutil.Context(t, testutil.WaitShort)
	runCtx, cancel := context.WithCancel(testCtx)
	defer cancel()

	mClock := quartz.NewMock(t)
	fClient := newFakeClient(t)
	fPatcher := newFakeAppStatusPatcher(t)
	templateID := uuid.UUID{5, 6, 7, 8}
	workspaceName := "test-workspace"
	appSlug := "test-app"

	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg, "test")

	connectedWaitGroup := &sync.WaitGroup{}
	connectedWaitGroup.Add(1)
	startReporting := make(chan struct{})

	cfg := Config{
		TemplateID:           templateID,
		WorkspaceName:        workspaceName,
		AppSlug:              appSlug,
		ConnectedWaitGroup:   connectedWaitGroup,
		StartReporting:       startReporting,
		ReportStatusPeriod:   10 * time.Second,
		ReportStatusDuration: 35 * time.Second,
		Metrics:              metrics,
		MetricLabelValues:    []string{"test"},
	}
	runner := &Runner{
		client:      fClient,
		patcher:     fPatcher,
		cfg:         cfg,
		clock:       mClock,
		reportTimes: make(map[int]time.Time),
	}

	tickerTrap := mClock.Trap().TickerFunc("reportTaskStatus")
	defer tickerTrap.Close()

	// Run the runner in a goroutine
	runErr := make(chan error, 1)
	go func() {
		runErr <- runner.Run(runCtx, "test-runner", testutil.NewTestLogWriter(t))
	}()

	connectedWaitGroup.Wait()
	close(startReporting)

	// Wait for the initial TickerFunc call before advancing time, otherwise our ticks will be off.
	tickerTrap.MustWait(testCtx).MustRelease(testCtx)

	for i := 0; i < 4; i++ {
		tickWaiter := mClock.Advance(10 * time.Second)
		testutil.RequireSend(testCtx, t, fPatcher.patchStatusErrors, xerrors.New("a bad thing happened"))
		_ = testutil.RequireReceive(testCtx, t, fPatcher.patchStatusCalls)
		tickWaiter.MustWait(testCtx)
	}

	// Cancel the run context to simulate the runner being killed.
	cancel()

	// Wait for the runner to complete
	err := testutil.RequireReceive(testCtx, t, runErr)
	require.ErrorIs(t, err, context.Canceled)

	// Verify metrics were updated correctly
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	var missingUpdatesFound bool
	var reportTaskStatusErrorsFound bool
	for _, mf := range metricFamilies {
		switch mf.GetName() {
		case "coderd_scaletest_missing_status_updates_total":
			missingUpdatesFound = true
			require.Len(t, mf.GetMetric(), 1)
			counter := mf.GetMetric()[0].GetCounter()
			assert.Equal(t, float64(4), counter.GetValue())
		case "coderd_scaletest_report_task_status_errors_total":
			reportTaskStatusErrorsFound = true
			require.Len(t, mf.GetMetric(), 1)
			counter := mf.GetMetric()[0].GetCounter()
			assert.Equal(t, float64(4), counter.GetValue())
		}
	}

	assert.True(t, missingUpdatesFound, "missing updates metric not found")
	assert.True(t, reportTaskStatusErrorsFound, "report task status errors metric not found")
}

func TestParseStatusMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		wantNum int
		wantOk  bool
	}{
		{
			name:    "valid message",
			message: "scaletest status update:42",
			wantNum: 42,
			wantOk:  true,
		},
		{
			name:    "valid message zero",
			message: "scaletest status update:0",
			wantNum: 0,
			wantOk:  true,
		},
		{
			name:    "invalid prefix",
			message: "wrong prefix:42",
			wantNum: 0,
			wantOk:  false,
		},
		{
			name:    "invalid number",
			message: "scaletest status update:abc",
			wantNum: 0,
			wantOk:  false,
		},
		{
			name:    "empty message",
			message: "",
			wantNum: 0,
			wantOk:  false,
		},
		{
			name:    "missing number",
			message: "scaletest status update:",
			wantNum: 0,
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotNum, gotOk := parseStatusMessage(tt.message)
			assert.Equal(t, tt.wantNum, gotNum)
			assert.Equal(t, tt.wantOk, gotOk)
		})
	}
}
