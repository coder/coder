package subagentexec

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

const testAuthToken = "child-auth-token"

// fakeController stands in for the v2.11 Agent API. Every call is
// recorded and published on a channel so tests can wait for the
// manager's asynchronous reconciliation instead of polling.
type fakeController struct {
	mu           sync.Mutex
	acquisitions []*proto.AcquireSubagentExecutionRequest
	reports      []*proto.ReportSubagentExecutionStatusRequest

	acquireErr        error
	acquireResponseFn func(*proto.AcquireSubagentExecutionRequest) *proto.AcquireSubagentExecutionResponse
	// acquireErrs are returned one per call, in order, before falling back
	// to acquireErr. A nil entry lets that call succeed.
	acquireErrs []error
	// acquireGate, when non-nil, blocks every Acquire until it is closed.
	acquireGate chan struct{}
	// inFlight and maxInFlight track overlapping Acquire calls, which is
	// how the tests prove acquisitions are never concurrent.
	inFlight    int
	maxInFlight int

	acquireCh chan *proto.AcquireSubagentExecutionRequest
	reportCh  chan *proto.ReportSubagentExecutionStatusRequest
}

func newFakeController() *fakeController {
	return &fakeController{
		acquireCh: make(chan *proto.AcquireSubagentExecutionRequest, 16),
		reportCh:  make(chan *proto.ReportSubagentExecutionStatusRequest, 16),
	}
}

func (f *fakeController) AcquireSubagentExecution(_ context.Context, in *proto.AcquireSubagentExecutionRequest) (*proto.AcquireSubagentExecutionResponse, error) {
	f.mu.Lock()
	f.acquisitions = append(f.acquisitions, in)
	f.inFlight++
	f.maxInFlight = max(f.maxInFlight, f.inFlight)
	err := f.acquireErr
	if len(f.acquireErrs) > 0 {
		err, f.acquireErrs = f.acquireErrs[0], f.acquireErrs[1:]
	}
	responseFn := f.acquireResponseFn
	gate := f.acquireGate
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()

	f.acquireCh <- in
	if gate != nil {
		<-gate
	}
	if err != nil {
		return nil, err
	}
	if responseFn != nil {
		return responseFn(in), nil
	}
	childAgentID := uuid.New()
	return &proto.AcquireSubagentExecutionResponse{
		ChildAgentId:       childAgentID[:],
		AuthToken:          testAuthToken,
		AcquisitionVersion: 7,
	}, nil
}

func (f *fakeController) ReportSubagentExecutionStatus(_ context.Context, in *proto.ReportSubagentExecutionStatusRequest) (*proto.ReportSubagentExecutionStatusResponse, error) {
	f.mu.Lock()
	f.reports = append(f.reports, in)
	f.mu.Unlock()

	f.reportCh <- in
	return &proto.ReportSubagentExecutionStatusResponse{}, nil
}

func (f *fakeController) acquisitionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.acquisitions)
}

// acquisitionCountFor counts the acquisitions for one execution, which is
// what pins the number of retry attempts a declaration produced.
func (f *fakeController) acquisitionCountFor(executionID uuid.UUID) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	var count int
	for _, acquisition := range f.acquisitions {
		if uuid.Must(uuid.FromBytes(acquisition.GetExecutionId())) == executionID {
			count++
		}
	}
	return count
}

func (f *fakeController) maxConcurrentAcquisitions() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInFlight
}

func (f *fakeController) reportCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reports)
}

func (f *fakeController) countStatus(status proto.ReportSubagentExecutionStatusRequest_Status) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	var count int
	for _, report := range f.reports {
		if report.GetStatus() == status {
			count++
		}
	}
	return count
}

func (f *fakeController) setAcquireErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquireErr = err
}

func (f *fakeController) setAcquireResponseFn(fn func(*proto.AcquireSubagentExecutionRequest) *proto.AcquireSubagentExecutionResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquireResponseFn = fn
}

// fakeProcess is a launched execution the test drives by hand.
type fakeProcess struct {
	// exit unblocks Wait. Close it, or send one error, to end the run.
	exit chan error
	// stopped receives once per Stop call.
	stopped chan struct{}
	// stopExits makes Stop end the run, which is what a real driver
	// does. Tests that want a hung process leave it false.
	stopExits bool

	stopOnce sync.Once
}

func newFakeProcess() *fakeProcess {
	return &fakeProcess{
		exit:      make(chan error, 1),
		stopped:   make(chan struct{}, 16),
		stopExits: true,
	}
}

func (p *fakeProcess) Wait() error {
	err, ok := <-p.exit
	if !ok {
		return nil
	}
	return err
}

func (p *fakeProcess) Stop(context.Context) error {
	p.stopped <- struct{}{}
	if p.stopExits {
		p.stopOnce.Do(func() { close(p.exit) })
	}
	return nil
}

// fakeDriver records the launches it is asked to start. It is the only
// seam that may read the private auth token on a Launch.
type fakeDriver struct {
	mu       sync.Mutex
	launches []Launch
	tokens   []string
	startErr error
	// gate, when non-nil, blocks Start until it is closed. It proves the
	// manager holds no lock across a driver call.
	gate chan struct{}
	// processes are handed out in order; a nil entry, or an exhausted
	// list, yields a fresh process.
	processes []*fakeProcess

	startCh chan Launch
}

func newFakeDriver() *fakeDriver {
	return &fakeDriver{startCh: make(chan Launch, 16)}
}

func (d *fakeDriver) Start(_ context.Context, launch Launch) (Process, error) {
	d.mu.Lock()
	d.launches = append(d.launches, launch)
	// The launch's auth token is unexported: only in-package code such as
	// this fake can observe it, which is what keeps it out of driver
	// implementations for now.
	d.tokens = append(d.tokens, launch.authToken)
	err := d.startErr
	gate := d.gate
	var proc *fakeProcess
	if len(d.processes) > 0 {
		proc, d.processes = d.processes[0], d.processes[1:]
	}
	d.mu.Unlock()

	d.startCh <- launch
	if gate != nil {
		<-gate
	}
	if err != nil {
		return nil, err
	}
	if proc == nil {
		proc = newFakeProcess()
	}
	return proc, nil
}

func (d *fakeDriver) setStartErr(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.startErr = err
}

func (d *fakeDriver) startCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.launches)
}

func (d *fakeDriver) observedTokens() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.tokens...)
}

// testLogger tolerates error logs: several cases below deliberately fail
// a launch or a process, and the manager is expected to log that.
func testLogger(t *testing.T) slog.Logger {
	return slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
}

func newManager(t *testing.T, driver Driver) *Manager {
	t.Helper()
	m := New(Options{
		Logger:       testLogger(t),
		Driver:       driver,
		CloseTimeout: testutil.WaitShort,
	})
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func testDeclaration() agentsdk.SubagentExecution {
	return agentsdk.SubagentExecution{
		ExecutionID:     uuid.New(),
		Generation:      uuid.New(),
		Name:            "sandbox",
		Driver:          "bubblewrap",
		DriverProtocol:  1,
		SharedHostPath:  "/home/coder/project",
		SharedChildPath: "/workspace/project",
		StartupTimeout:  time.Minute,
		RestartPolicy:   "never",
	}
}

func TestManager_LaunchesDeclaredExecution(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	childAgentID := uuid.New()
	controller.setAcquireResponseFn(func(*proto.AcquireSubagentExecutionRequest) *proto.AcquireSubagentExecutionResponse {
		return &proto.AcquireSubagentExecutionResponse{
			ChildAgentId:       childAgentID[:],
			AuthToken:          testAuthToken,
			AcquisitionVersion: 42,
		}
	})
	driver := newFakeDriver()
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})

	acquire := testutil.RequireReceive(ctx, t, controller.acquireCh)
	require.Equal(t, decl.ExecutionID[:], acquire.GetExecutionId())
	require.Equal(t, decl.Generation[:], acquire.GetGeneration())

	launch := testutil.RequireReceive(ctx, t, driver.startCh)
	require.Equal(t, decl, launch.Declaration)
	require.Equal(t, childAgentID, launch.ChildAgentID)
	require.EqualValues(t, 42, launch.AcquisitionVersion)
	require.Equal(t, []string{testAuthToken}, driver.observedTokens())

	report := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_RUNNING, report.GetStatus())
	require.Equal(t, decl.ExecutionID[:], report.GetExecutionId())
	require.Equal(t, decl.Generation[:], report.GetGeneration())
	require.EqualValues(t, 42, report.GetAcquisitionVersion())
	require.Empty(t, report.GetError())

	statuses := m.Statuses()
	require.Len(t, statuses, 1)
	require.Equal(t, StateRunning, statuses[0].State)
	require.Equal(t, childAgentID, statuses[0].ChildAgentID)
	require.EqualValues(t, 42, statuses[0].AcquisitionVersion)

	require.Equal(t, 1, controller.acquisitionCount())
	require.Equal(t, 1, driver.startCount())
}

func TestManager_StartFailureReportsFailed(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	driver := newFakeDriver()
	driver.startErr = xerrors.New("no sandbox for you")
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})

	report := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, report.GetStatus())
	require.EqualValues(t, 7, report.GetAcquisitionVersion())
	require.Contains(t, report.GetError(), "no sandbox for you")
	require.LessOrEqual(t, len(report.GetError()), MaxReportErrorBytes)

	statuses := m.Statuses()
	require.Len(t, statuses, 1)
	require.Equal(t, StateFailed, statuses[0].State)
	require.Contains(t, statuses[0].LastError, "no sandbox for you")
}

func TestManager_AcquireFailureDoesNotReport(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	controller.setAcquireErr(xerrors.New("subagent execution is unavailable"))
	driver := newFakeDriver()
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})
	testutil.RequireReceive(ctx, t, controller.acquireCh)

	require.Eventually(t, func() bool {
		statuses := m.Statuses()
		return len(statuses) == 1 && statuses[0].State == StateFailed
	}, testutil.WaitShort, testutil.IntervalFast)

	require.Zero(t, driver.startCount())
	// A launcher with no acquisition version has no identity to report
	// under, so it must not send a report at all.
	require.Zero(t, controller.reportCount())
}

// A transient acquisition failure, such as the one a stale controller
// returns while the DRPC connection is being replaced, must not strand the
// declared child. The reconcile that follows the reconnect has to acquire
// again, through the fresh controller.
func TestManager_AcquireFailureRetriesWithFreshController(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	stale := newFakeController()
	stale.setAcquireErr(xerrors.New("connection reset by peer"))
	driver := newFakeDriver()
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(stale, []agentsdk.SubagentExecution{decl})
	testutil.RequireReceive(ctx, t, stale.acquireCh)

	require.Eventually(t, func() bool {
		statuses := m.Statuses()
		return len(statuses) == 1 && statuses[0].State == StateFailed
	}, testutil.WaitShort, testutil.IntervalFast)

	// The failure is visible locally, redacted, and carries no token.
	failedStatuses := m.Statuses()
	require.Contains(t, failedStatuses[0].LastError, "connection reset by peer")
	require.LessOrEqual(t, len(failedStatuses[0].LastError), MaxReportErrorBytes)
	encoded, err := json.Marshal(failedStatuses)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), testAuthToken)
	require.Zero(t, driver.startCount())

	// The reconnect supplies a fresh controller with the same declaration.
	fresh := newFakeController()
	childAgentID := uuid.New()
	fresh.setAcquireResponseFn(func(*proto.AcquireSubagentExecutionRequest) *proto.AcquireSubagentExecutionResponse {
		return &proto.AcquireSubagentExecutionResponse{
			ChildAgentId:       childAgentID[:],
			AuthToken:          testAuthToken,
			AcquisitionVersion: 11,
		}
	})
	m.Reconcile(fresh, []agentsdk.SubagentExecution{decl})

	acquire := testutil.RequireReceive(ctx, t, fresh.acquireCh)
	require.Equal(t, decl.ExecutionID[:], acquire.GetExecutionId())
	require.Equal(t, decl.Generation[:], acquire.GetGeneration())

	launch := testutil.RequireReceive(ctx, t, driver.startCh)
	require.Equal(t, decl, launch.Declaration)
	require.Equal(t, childAgentID, launch.ChildAgentID)

	report := testutil.RequireReceive(ctx, t, fresh.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_RUNNING, report.GetStatus())
	require.EqualValues(t, 11, report.GetAcquisitionVersion())

	statuses := m.Statuses()
	require.Len(t, statuses, 1)
	require.Equal(t, StateRunning, statuses[0].State)
	require.Equal(t, childAgentID, statuses[0].ChildAgentID)
	require.EqualValues(t, 11, statuses[0].AcquisitionVersion)
	require.Empty(t, statuses[0].LastError)

	// The stale controller acquired once and, having no acquisition
	// version, reported nothing.
	require.Equal(t, 1, stale.acquisitionCount())
	require.Zero(t, stale.reportCount())
	require.Equal(t, 1, driver.startCount())

	// Now that it is running, an identical declaration is a no-op: a
	// reconcile that adds a second declaration is the ordering point that
	// proves the identical one ahead of it was drained.
	m.Reconcile(fresh, []agentsdk.SubagentExecution{decl})
	next := testDeclaration()
	m.Reconcile(fresh, []agentsdk.SubagentExecution{decl, next})
	testutil.RequireReceive(ctx, t, driver.startCh)

	require.Equal(t, 1, fresh.acquisitionCountFor(decl.ExecutionID))
	require.Equal(t, 1, fresh.acquisitionCountFor(next.ExecutionID))
	require.Equal(t, 2, driver.startCount())
}

// Reconciles that queue up behind a blocked acquisition must not race it.
// Reconciliation is single-goroutine, so the queued requests only drive a
// further attempt after the first one has finished, and only while the
// declaration has not launched.
func TestManager_QueuedReconcilesAcquireSequentially(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	gate := make(chan struct{})
	controller.acquireGate = gate
	// The first attempt fails; the retry the queued reconciles drive
	// succeeds.
	controller.acquireErrs = []error{xerrors.New("acquisition is temporarily unavailable")}
	driver := newFakeDriver()
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})
	testutil.RequireReceive(ctx, t, controller.acquireCh)

	// The loop is inside the first acquisition, so these queue behind it.
	for range 3 {
		m.Reconcile(controller, []agentsdk.SubagentExecution{decl})
	}
	require.Zero(t, driver.startCount())

	close(gate)

	launch := testutil.RequireReceive(ctx, t, driver.startCh)
	require.Equal(t, decl, launch.Declaration)
	report := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_RUNNING, report.GetStatus())

	next := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl, next})
	testutil.RequireReceive(ctx, t, driver.startCh)

	// Exactly one retry: the first queued reconcile launched the
	// declaration and the rest found it already launched.
	require.Equal(t, 1, controller.maxConcurrentAcquisitions())
	require.Equal(t, 2, controller.acquisitionCountFor(decl.ExecutionID))
	require.Equal(t, 2, driver.startCount())

	statuses := m.Statuses()
	require.Len(t, statuses, 2)
	for _, status := range statuses {
		require.Equal(t, StateRunning, status.State)
	}
}

// A driver that could not start the process leaves no launcher behind, so
// the next reconcile of the same declaration retries it. The retry acquires
// a fresh version, which fences the launch that never happened.
func TestManager_StartFailureRetriesOnNextReconcile(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	driver := newFakeDriver()
	driver.startErr = xerrors.New("sandbox setup failed")
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})

	failed := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, failed.GetStatus())
	require.Contains(t, failed.GetError(), "sandbox setup failed")

	driver.setStartErr(nil)
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})

	running := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_RUNNING, running.GetStatus())
	require.Equal(t, decl.Generation[:], running.GetGeneration())

	require.Equal(t, 2, controller.acquisitionCountFor(decl.ExecutionID))
	require.Equal(t, 1, controller.maxConcurrentAcquisitions())
	require.Equal(t, 2, driver.startCount())

	statuses := m.Statuses()
	require.Len(t, statuses, 1)
	require.Equal(t, StateRunning, statuses[0].State)
	require.Empty(t, statuses[0].LastError)
}

func TestManager_UnexpectedExitReportsFailed(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	proc := newFakeProcess()
	driver := newFakeDriver()
	driver.processes = []*fakeProcess{proc}
	m := newManager(t, driver)

	m.Reconcile(controller, []agentsdk.SubagentExecution{testDeclaration()})
	running := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_RUNNING, running.GetStatus())

	proc.exit <- xerrors.New("driver crashed")

	failed := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, failed.GetStatus())
	require.Contains(t, failed.GetError(), "driver crashed")

	require.Eventually(t, func() bool {
		statuses := m.Statuses()
		return len(statuses) == 1 && statuses[0].State == StateFailed
	}, testutil.WaitShort, testutil.IntervalFast)
	require.Zero(t, len(proc.stopped))
}

func TestManager_CleanExitIsUnexpected(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	proc := newFakeProcess()
	driver := newFakeDriver()
	driver.processes = []*fakeProcess{proc}
	m := newManager(t, driver)

	m.Reconcile(controller, []agentsdk.SubagentExecution{testDeclaration()})
	testutil.RequireReceive(ctx, t, controller.reportCh)

	// This slice has no restart policy, so an exit nobody asked for is a
	// failure even when the process exited cleanly.
	close(proc.exit)

	failed := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, failed.GetStatus())
	require.Contains(t, failed.GetError(), "exited unexpectedly")
}

func TestManager_IdenticalReconcileIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	driver := newFakeDriver()
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})
	testutil.RequireReceive(ctx, t, driver.startCh)
	testutil.RequireReceive(ctx, t, controller.reportCh)

	for range 3 {
		m.Reconcile(controller, []agentsdk.SubagentExecution{decl})
	}
	// A reconcile that changes something is the only way to observe the
	// loop draining the identical ones ahead of it.
	next := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl, next})
	testutil.RequireReceive(ctx, t, driver.startCh)

	require.Equal(t, 2, driver.startCount())
	require.Equal(t, 2, controller.acquisitionCount())
	require.Len(t, m.Statuses(), 2)
}

func TestManager_ReplacedControllerReceivesLaterReport(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	first := newFakeController()
	proc := newFakeProcess()
	driver := newFakeDriver()
	driver.processes = []*fakeProcess{proc}
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(first, []agentsdk.SubagentExecution{decl})
	testutil.RequireReceive(ctx, t, first.reportCh)

	// A DRPC reconnect replaces the controller without re-acquiring the
	// execution, and the next report must use the live connection.
	second := newFakeController()
	m.Reconcile(second, []agentsdk.SubagentExecution{decl})

	require.Eventually(t, func() bool {
		return m.currentController() == Controller(second)
	}, testutil.WaitShort, testutil.IntervalFast)

	proc.exit <- xerrors.New("driver crashed after reconnect")
	failed := testutil.RequireReceive(ctx, t, second.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_FAILED, failed.GetStatus())

	require.Equal(t, 1, first.reportCount())
	require.Zero(t, second.acquisitionCount())
}

func TestManager_NewGenerationStopsOldAndReacquires(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	old := newFakeProcess()
	driver := newFakeDriver()
	driver.processes = []*fakeProcess{old, newFakeProcess()}
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})
	testutil.RequireReceive(ctx, t, controller.reportCh)

	next := decl
	next.Generation = uuid.New()
	m.Reconcile(controller, []agentsdk.SubagentExecution{next})

	stopped := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_STOPPED, stopped.GetStatus())
	require.Equal(t, decl.Generation[:], stopped.GetGeneration())
	testutil.RequireReceive(ctx, t, old.stopped)

	running := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_RUNNING, running.GetStatus())
	require.Equal(t, next.Generation[:], running.GetGeneration())

	require.Equal(t, 2, controller.acquisitionCount())
	require.Equal(t, 2, driver.startCount())
	statuses := m.Statuses()
	require.Len(t, statuses, 1)
	require.Equal(t, next.Generation, statuses[0].Generation)
	require.Equal(t, StateRunning, statuses[0].State)
}

func TestManager_RemovedDeclarationStops(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	proc := newFakeProcess()
	driver := newFakeDriver()
	driver.processes = []*fakeProcess{proc}
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})
	testutil.RequireReceive(ctx, t, controller.reportCh)

	m.Reconcile(controller, nil)

	stopped := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_STOPPED, stopped.GetStatus())
	require.EqualValues(t, 7, stopped.GetAcquisitionVersion())
	testutil.RequireReceive(ctx, t, proc.stopped)
	require.Empty(t, m.Statuses())
}

func TestManager_CloseStopsActiveExecutions(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	first, second := newFakeProcess(), newFakeProcess()
	driver := newFakeDriver()
	driver.processes = []*fakeProcess{first, second}
	m := New(Options{
		Logger:       testLogger(t),
		Driver:       driver,
		CloseTimeout: testutil.WaitShort,
	})

	declA, declB := testDeclaration(), testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{declA, declB})
	testutil.RequireReceive(ctx, t, controller.reportCh)
	testutil.RequireReceive(ctx, t, controller.reportCh)

	require.NoError(t, m.Close())

	testutil.RequireReceive(ctx, t, first.stopped)
	testutil.RequireReceive(ctx, t, second.stopped)
	require.Empty(t, m.Statuses())

	require.Equal(t, 2, controller.countStatus(proto.ReportSubagentExecutionStatusRequest_STOPPED))

	// Close is idempotent, and a reconcile afterwards launches nothing.
	require.NoError(t, m.Close())
	m.Reconcile(controller, []agentsdk.SubagentExecution{testDeclaration()})
	require.Equal(t, 2, driver.startCount())
}

func TestManager_StatusNeverExposesToken(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	driver := newFakeDriver()
	m := newManager(t, driver)

	m.Reconcile(controller, []agentsdk.SubagentExecution{testDeclaration()})
	testutil.RequireReceive(ctx, t, controller.reportCh)

	// The driver did receive the token, so the redaction below is not
	// vacuous.
	require.Equal(t, []string{testAuthToken}, driver.observedTokens())

	statusType := reflect.TypeOf(Status{})
	for i := range statusType.NumField() {
		name := strings.ToLower(statusType.Field(i).Name)
		require.NotContains(t, name, "token")
		require.NotContains(t, name, "secret")
		require.NotContains(t, name, "credential")
	}

	statuses := m.Statuses()
	require.Len(t, statuses, 1)
	encoded, err := json.Marshal(statuses)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), testAuthToken)
	require.NotContains(t, fmt.Sprintf("%v", statuses), testAuthToken)
}

func TestManager_NoControllerNeverLaunches(t *testing.T) {
	t.Parallel()

	driver := newFakeDriver()
	m := newManager(t, driver)

	m.Reconcile(nil, []agentsdk.SubagentExecution{testDeclaration()})
	// A reconcile with a controller is the only ordering point available,
	// so use one that returns declarations we can wait on.
	controller := newFakeController()
	decl := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})
	testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, driver.startCh)

	require.Equal(t, 1, driver.startCount())
	require.Equal(t, 1, controller.acquisitionCount())
}

func TestManager_IgnoresInvalidDeclarations(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	driver := newFakeDriver()
	m := newManager(t, driver)

	valid := testDeclaration()
	noID := testDeclaration()
	noID.ExecutionID = uuid.Nil
	noGeneration := testDeclaration()
	noGeneration.Generation = uuid.Nil
	duplicate := valid
	duplicate.Generation = uuid.New()

	m.Reconcile(controller, []agentsdk.SubagentExecution{noID, noGeneration, valid, duplicate})
	testutil.RequireReceive(ctx, t, driver.startCh)

	require.Equal(t, 1, driver.startCount())
	statuses := m.Statuses()
	require.Len(t, statuses, 1)
	require.Equal(t, valid.Generation, statuses[0].Generation)
}

func TestManager_BlockingDriverDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	controller := newFakeController()
	driver := newFakeDriver()
	gate := make(chan struct{})
	driver.gate = gate
	m := newManager(t, driver)

	decl := testDeclaration()
	m.Reconcile(controller, []agentsdk.SubagentExecution{decl})
	testutil.RequireReceive(ctx, t, driver.startCh)

	// Start is blocked. Every caller-facing method must still return
	// promptly, which is only true if no lock spans the driver call.
	done := make(chan []Status, 1)
	go func() {
		m.Reconcile(controller, []agentsdk.SubagentExecution{decl})
		done <- m.Statuses()
	}()
	statuses := testutil.RequireReceive(ctx, t, done)
	require.Len(t, statuses, 1)
	require.Equal(t, StateStarting, statuses[0].State)

	close(gate)
	report := testutil.RequireReceive(ctx, t, controller.reportCh)
	require.Equal(t, proto.ReportSubagentExecutionStatusRequest_RUNNING, report.GetStatus())
}

func TestBoundError(t *testing.T) {
	t.Parallel()

	require.Equal(t, "short", BoundError("short"))

	long := strings.Repeat("a", MaxReportErrorBytes+10)
	require.Len(t, BoundError(long), MaxReportErrorBytes)

	// A multi-byte rune straddling the limit is dropped whole rather than
	// split, so the result is always valid UTF-8.
	runes := strings.Repeat("é", MaxReportErrorBytes)
	bounded := BoundError(runes)
	require.True(t, utf8.ValidString(bounded))
	require.LessOrEqual(t, len(bounded), MaxReportErrorBytes)
	require.Greater(t, len(bounded), MaxReportErrorBytes-4)
}

func TestLaunchStringRedactsToken(t *testing.T) {
	t.Parallel()

	launch := Launch{
		Declaration:        testDeclaration(),
		ChildAgentID:       uuid.New(),
		AcquisitionVersion: 3,
		authToken:          testAuthToken,
	}
	require.NotContains(t, launch.String(), testAuthToken)
	require.NotContains(t, fmt.Sprintf("%v", launch), testAuthToken)
}
