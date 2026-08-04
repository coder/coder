package subagentexec

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// defaultCloseTimeout bounds how long Close waits for the reconciliation
// loop to yield, for each process to exit, and for the STOPPED reports
// that follow.
const defaultCloseTimeout = 10 * time.Second

// stop reasons, used for diagnostics only.
const (
	reasonRemoved    = "declaration removed"
	reasonSuperseded = "declaration superseded by a new generation"
	reasonClosing    = "manager closing"
)

// Options configures a Manager.
type Options struct {
	Logger slog.Logger
	// Driver launches acquired executions. A nil Driver installs one that
	// fails every launch with ErrDriverNotConfigured. An agent whose
	// manifest declares no executions never invokes it.
	Driver Driver
	// CloseTimeout bounds the work Close does. Zero uses
	// defaultCloseTimeout.
	CloseTimeout time.Duration
}

// execution is one declared execution the manager tracks. Every mutable
// field is guarded by Manager.mu; the immutable fields are safe to read
// from the supervision goroutine without it.
//
// A tracked execution is not necessarily a running one: a declaration
// whose acquisition or driver handoff failed is retained so its failure is
// visible through Statuses, with launched left false so the next
// reconcile of the same declaration tries again.
type execution struct {
	// immutable after construction.
	declaration        agentsdk.SubagentExecution
	childAgentID       uuid.UUID
	acquisitionVersion int64
	// done is closed by the supervision goroutine when the process exits.
	// It is nil when no process was ever started.
	done chan struct{}
	// proc is written before the supervision goroutine starts and never
	// mutated afterwards.
	proc Process

	// guarded by Manager.mu.
	state     State
	lastError string
	// launched records that the driver handed back a process for this
	// execution. Only a launched execution satisfies its declaration, so a
	// record that never launched is retried by the next reconcile of the
	// same generation. It stays true after the process exits: this slice
	// has no restart policy, so an execution that ran and then failed is
	// terminal rather than retryable.
	launched bool
	// stopping records that the manager, not the process, ended this run,
	// so the supervision goroutine leaves the report to stop.
	stopping bool
}

// Manager reconciles the executions a manifest declares against the
// executions it has launched. It is constructed once per agent and
// survives DRPC reconnects: only the Controller reference is replaced.
//
// All reconciliation runs on a single goroutine, so declarations are
// applied in the order they arrive. No lock is ever held across an RPC, a
// driver call, or a process wait.
type Manager struct {
	logger       slog.Logger
	driver       Driver
	closeTimeout time.Duration

	// runCtx bounds the reconciliation path: acquisitions, driver starts,
	// and the reports that follow. It is cancelled at the very end of
	// Close, after the STOPPED reports have been made.
	runCtx    context.Context
	cancelRun context.CancelFunc
	// stopReconcile tells the reconciliation loop to yield.
	stopReconcile chan struct{}
	closeOnce     sync.Once
	loopDone      chan struct{}
	// wake signals queued work to the reconciliation loop.
	wake chan struct{}
	// wg tracks the per-execution supervision goroutines.
	wg sync.WaitGroup

	mu         sync.Mutex
	closed     bool
	controller Controller
	pending    []request
	executions map[uuid.UUID]*execution
}

// request is one queued manifest. The controller travels with it so that
// declarations are acquired through the connection that carried them,
// while reports always use whichever controller is current.
type request struct {
	controller   Controller
	declarations []agentsdk.SubagentExecution
}

// New constructs a Manager and starts its reconciliation loop. The caller
// owns it for the lifetime of the agent and must Close it.
func New(opts Options) *Manager {
	if opts.Driver == nil {
		opts.Driver = unsupportedDriver{}
	}
	if opts.CloseTimeout <= 0 {
		opts.CloseTimeout = defaultCloseTimeout
	}
	runCtx, cancelRun := context.WithCancel(context.Background())
	m := &Manager{
		logger:        opts.Logger,
		driver:        opts.Driver,
		closeTimeout:  opts.CloseTimeout,
		runCtx:        runCtx,
		cancelRun:     cancelRun,
		stopReconcile: make(chan struct{}),
		loopDone:      make(chan struct{}),
		wake:          make(chan struct{}, 1),
		executions:    make(map[uuid.UUID]*execution),
	}
	go m.reconcileLoop()
	return m
}

// Reconcile queues the manifest's declarations and returns immediately.
// The controller replaces the one used for later acquisitions and
// reports, which is how the manager survives a DRPC reconnect. A nil
// controller means this coderd does not implement the subagent execution
// API, so nothing is launched.
//
// Child agents never launch nested executions; their caller passes no
// declarations, which also tears down anything an earlier manifest
// declared.
func (m *Manager) Reconcile(controller Controller, declarations []agentsdk.SubagentExecution) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		m.logger.Debug(context.Background(), "ignoring reconcile on closed subagent execution manager")
		return
	}
	m.controller = controller
	m.pending = append(m.pending, request{
		controller:   controller,
		declarations: slices.Clone(declarations),
	})
	m.mu.Unlock()

	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Statuses returns the redacted status of every execution the manager
// currently tracks, ordered by execution ID.
func (m *Manager) Statuses() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	statuses := make([]Status, 0, len(m.executions))
	for _, ex := range m.executions {
		statuses = append(statuses, Status{
			ExecutionID:        ex.declaration.ExecutionID,
			Generation:         ex.declaration.Generation,
			Name:               ex.declaration.Name,
			Driver:             ex.declaration.Driver,
			ChildAgentID:       ex.childAgentID,
			AcquisitionVersion: ex.acquisitionVersion,
			State:              ex.state,
			LastError:          ex.lastError,
		})
	}
	slices.SortFunc(statuses, func(a, b Status) int {
		return strings.Compare(a.ExecutionID.String(), b.ExecutionID.String())
	})
	return statuses
}

// Close stops every active execution and reports STOPPED while the
// controller can still reach coderd, then waits for the supervision
// goroutines. It is safe to call more than once.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.pending = nil
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), m.closeTimeout)
	defer cancel()

	// Yield the reconciliation loop first so it cannot start a new
	// execution while we are tearing the existing ones down.
	m.closeOnce.Do(func() { close(m.stopReconcile) })
	select {
	case <-m.loopDone:
	case <-ctx.Done():
		m.logger.Warn(ctx, "timed out waiting for subagent execution reconciliation to yield")
	}

	for _, ex := range m.trackedExecutions() {
		m.stop(ctx, ex, reasonClosing)
	}

	waited := make(chan struct{})
	go func() {
		defer close(waited)
		m.wg.Wait()
	}()
	select {
	case <-waited:
	case <-ctx.Done():
		m.logger.Warn(ctx, "timed out waiting for subagent execution processes to exit")
	}

	// Only now is it safe to cancel the run context: the STOPPED reports
	// above needed a live controller call.
	m.cancelRun()
	return nil
}

func (m *Manager) reconcileLoop() {
	defer close(m.loopDone)
	for {
		for _, req := range m.takePending() {
			if m.stopping() {
				return
			}
			m.reconcile(req)
		}
		select {
		case <-m.stopReconcile:
			return
		case <-m.wake:
		}
	}
}

func (m *Manager) stopping() bool {
	select {
	case <-m.stopReconcile:
		return true
	default:
		return false
	}
}

func (m *Manager) takePending() []request {
	m.mu.Lock()
	defer m.mu.Unlock()
	pending := m.pending
	m.pending = nil
	return pending
}

// reconcile applies one manifest's declarations. It runs on the
// reconciliation loop goroutine and never holds m.mu across a blocking
// call.
func (m *Manager) reconcile(req request) {
	desired := make(map[uuid.UUID]agentsdk.SubagentExecution, len(req.declarations))
	ordered := make([]agentsdk.SubagentExecution, 0, len(req.declarations))
	for _, decl := range req.declarations {
		if decl.ExecutionID == uuid.Nil || decl.Generation == uuid.Nil {
			m.logger.Warn(m.runCtx, "ignoring subagent execution declaration with an empty identity",
				slog.F("execution_id", decl.ExecutionID), slog.F("generation", decl.Generation))
			continue
		}
		if _, dup := desired[decl.ExecutionID]; dup {
			m.logger.Warn(m.runCtx, "ignoring duplicate subagent execution declaration",
				slog.F("execution_id", decl.ExecutionID))
			continue
		}
		desired[decl.ExecutionID] = decl
		ordered = append(ordered, decl)
	}

	// Tear down executions the manifest no longer declares, and
	// executions whose declaration moved to a new generation, before
	// launching anything: a superseded generation must not run alongside
	// its replacement.
	for _, ex := range m.trackedExecutions() {
		decl, declared := desired[ex.declaration.ExecutionID]
		if declared && decl.Generation == ex.declaration.Generation {
			continue
		}
		reason := reasonRemoved
		if declared {
			reason = reasonSuperseded
		}
		m.stop(m.runCtx, ex, reason)
	}

	for _, decl := range ordered {
		if m.launchedAtGeneration(decl.ExecutionID, decl.Generation) {
			// Already launched at this generation. Re-acquiring would
			// fence the running launcher against itself, and restarting
			// would duplicate the child.
			continue
		}
		// A declaration whose previous attempt never launched is retried
		// here, with this request's controller. Because every reconcile
		// runs on this one goroutine, the retry cannot overlap the attempt
		// that failed: at most one acquisition per execution is ever in
		// flight, and each queued reconcile drives at most one further
		// attempt.
		m.start(m.runCtx, req.controller, decl)
	}
}

// start acquires the child's credentials and launches the execution. A
// failure before the driver returns a process leaves the execution tracked
// but not launched, so the next reconcile of the same declaration retries
// it. There is no background retry: nothing happens until the parent agent
// reconciles again, which it does on every manifest refresh and reconnect.
func (m *Manager) start(ctx context.Context, controller Controller, decl agentsdk.SubagentExecution) {
	logger := m.declLogger(decl)

	if controller == nil {
		logger.Warn(ctx, "cannot launch declared subagent execution: this deployment does not support the subagent execution API")
		return
	}

	resp, err := controller.AcquireSubagentExecution(ctx, &proto.AcquireSubagentExecutionRequest{
		ExecutionId: decl.ExecutionID[:],
		Generation:  decl.Generation[:],
	})
	if err != nil {
		// Without an acquisition version there is no launcher identity to
		// report under, so the failure is recorded locally only. A stale
		// controller or a dropped connection lands here, so the record must
		// stay retryable: the reconcile that follows the reconnect acquires
		// again through the fresh controller.
		logger.Warn(ctx, "acquire subagent execution", slog.Error(err))
		m.record(decl, uuid.Nil, 0, StateFailed, xerrors.Errorf("acquire subagent execution: %w", err))
		return
	}

	childAgentID, parseErr := uuid.FromBytes(resp.GetChildAgentId())
	switch {
	case parseErr != nil:
		err = xerrors.Errorf("parse child agent id: %w", parseErr)
	case childAgentID == uuid.Nil:
		err = xerrors.New("acquisition returned a nil child agent id")
	case resp.GetAuthToken() == "":
		err = xerrors.New("acquisition returned an empty child auth token")
	case resp.GetAcquisitionVersion() <= 0:
		err = xerrors.New("acquisition returned a non-positive acquisition version")
	}
	if err != nil {
		// The acquisition is unusable, so there is no launcher identity we
		// can trust for a report either.
		logger.Error(ctx, "subagent execution acquisition is unusable", slog.Error(err))
		m.record(decl, uuid.Nil, 0, StateFailed, err)
		return
	}

	ex := &execution{
		declaration:        decl,
		childAgentID:       childAgentID,
		acquisitionVersion: resp.GetAcquisitionVersion(),
		done:               make(chan struct{}),
		state:              StateStarting,
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		logger.Debug(ctx, "not launching subagent execution: manager is closing")
		return
	}
	m.executions[decl.ExecutionID] = ex
	m.mu.Unlock()

	// The driver gets the manager's run context so the process outlives
	// this reconciliation.
	proc, err := m.startDriver(ctx, ex, resp.GetAuthToken())
	if err != nil {
		// No process exists, so this acquisition is spent. The record stays
		// unlaunched: the next reconcile of the same declaration acquires a
		// higher version, which fences this dead launch.
		logger.Error(ctx, "start subagent execution", slog.Error(err))
		m.mu.Lock()
		ex.state = StateFailed
		ex.lastError = BoundError(err.Error())
		m.mu.Unlock()
		m.report(ctx, ex, proto.ReportSubagentExecutionStatusRequest_FAILED, err)
		return
	}

	m.mu.Lock()
	ex.proc = proc
	ex.state = StateRunning
	ex.launched = true
	m.mu.Unlock()

	logger.Info(ctx, "launched subagent execution",
		slog.F("child_agent_id", childAgentID),
		slog.F("acquisition_version", ex.acquisitionVersion))
	m.report(ctx, ex, proto.ReportSubagentExecutionStatusRequest_RUNNING, nil)

	m.wg.Add(1)
	go m.supervise(ex)
}

// startDriver hands the child's auth token to the driver just long enough
// for the driver to materialize it as a private 0600 token file, then drops
// the launcher's reference to it. Nothing the manager retains afterwards
// carries the token: the execution record does not hold it, and the
// sandboxed child reads it from the file the driver created.
//
// Go cannot guarantee the string's bytes are wiped from memory, so this is
// a lifetime boundary rather than a scrubbing guarantee.
func (m *Manager) startDriver(ctx context.Context, ex *execution, authToken string) (Process, error) {
	launch := Launch{
		Declaration:        ex.declaration,
		ChildAgentID:       ex.childAgentID,
		AcquisitionVersion: ex.acquisitionVersion,
		authToken:          authToken,
	}
	defer func() { launch.authToken = "" }()
	return m.driver.Start(ctx, launch)
}

// supervise waits for a launched process and reports the outcome. An exit
// the manager did not ask for is a failure: this slice has no restart
// policy, so nothing else would notice.
func (m *Manager) supervise(ex *execution) {
	defer m.wg.Done()

	waitErr := ex.proc.Wait()
	close(ex.done)

	exitErr := waitErr
	if exitErr == nil {
		exitErr = xerrors.New("subagent execution process exited unexpectedly")
	}

	m.mu.Lock()
	stopping := ex.stopping
	if !stopping {
		ex.state = StateFailed
		ex.lastError = BoundError(exitErr.Error())
	}
	m.mu.Unlock()
	if stopping {
		// stop observed the exit first and owns the STOPPED report.
		return
	}

	logger := m.declLogger(ex.declaration)
	logger.Error(m.runCtx, "subagent execution exited unexpectedly", slog.Error(exitErr))
	m.report(m.runCtx, ex, proto.ReportSubagentExecutionStatusRequest_FAILED, exitErr)
}

// stop ends an execution the manager launched and reports STOPPED. It
// must not be called with m.mu held.
func (m *Manager) stop(ctx context.Context, ex *execution, reason string) {
	logger := m.declLogger(ex.declaration).With(slog.F("reason", reason))

	m.mu.Lock()
	if current, ok := m.executions[ex.declaration.ExecutionID]; ok && current == ex {
		delete(m.executions, ex.declaration.ExecutionID)
	}
	if ex.stopping {
		m.mu.Unlock()
		return
	}
	ex.stopping = true
	// A run that never started, or that already ended on its own, has no
	// stop to perform and no STOPPED transition to report.
	inactive := ex.proc == nil || ex.state == StateFailed || ex.state == StateStopped
	if !inactive {
		ex.state = StateStopping
	}
	proc := ex.proc
	m.mu.Unlock()

	if inactive {
		logger.Debug(ctx, "dropping subagent execution that is not running")
		return
	}

	if err := proc.Stop(ctx); err != nil {
		logger.Warn(ctx, "stop subagent execution process", slog.Error(err))
	}
	select {
	case <-ex.done:
	case <-ctx.Done():
		logger.Warn(ctx, "timed out waiting for subagent execution process to exit")
	}

	m.mu.Lock()
	ex.state = StateStopped
	m.mu.Unlock()

	logger.Info(ctx, "stopped subagent execution")
	m.report(ctx, ex, proto.ReportSubagentExecutionStatusRequest_STOPPED, nil)
}

// report sends one status report using whichever controller is current,
// so a report made after a reconnect uses the live connection.
func (m *Manager) report(ctx context.Context, ex *execution, status proto.ReportSubagentExecutionStatusRequest_Status, reportErr error) {
	controller := m.currentController()
	logger := m.declLogger(ex.declaration).With(slog.F("status", status.String()))
	if controller == nil {
		logger.Warn(ctx, "cannot report subagent execution status: no controller")
		return
	}

	req := &proto.ReportSubagentExecutionStatusRequest{
		ExecutionId:        ex.declaration.ExecutionID[:],
		Generation:         ex.declaration.Generation[:],
		AcquisitionVersion: ex.acquisitionVersion,
		Status:             status,
	}
	if reportErr != nil {
		req.Error = BoundError(reportErr.Error())
	}
	if _, err := controller.ReportSubagentExecutionStatus(ctx, req); err != nil {
		logger.Warn(ctx, "report subagent execution status", slog.Error(err))
	}
}

// record retains a redacted status for a declaration the manager could not
// acquire, so the failure is visible through Statuses. The record is
// deliberately unlaunched, so it does not suppress the retry a later
// identical manifest performs.
func (m *Manager) record(decl agentsdk.SubagentExecution, childAgentID uuid.UUID, acquisitionVersion int64, state State, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	ex := &execution{
		declaration:        decl,
		childAgentID:       childAgentID,
		acquisitionVersion: acquisitionVersion,
		state:              state,
	}
	if err != nil {
		ex.lastError = BoundError(err.Error())
	}
	m.executions[decl.ExecutionID] = ex
}

func (m *Manager) currentController() Controller {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.controller
}

// trackedExecutions snapshots every tracked execution, launched or not, in
// a stable order so teardown is deterministic.
func (m *Manager) trackedExecutions() []*execution {
	m.mu.Lock()
	defer m.mu.Unlock()
	executions := make([]*execution, 0, len(m.executions))
	for _, ex := range m.executions {
		executions = append(executions, ex)
	}
	slices.SortFunc(executions, func(a, b *execution) int {
		return strings.Compare(a.declaration.ExecutionID.String(), b.declaration.ExecutionID.String())
	})
	return executions
}

// launchedAtGeneration reports whether this exact declaration already
// reached a started process. A tracked but unlaunched record, such as one
// left by a failed acquisition, does not count: suppressing the retry there
// would strand the declared child forever.
func (m *Manager) launchedAtGeneration(executionID, generation uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	ex, ok := m.executions[executionID]
	return ok && ex.launched && ex.declaration.Generation == generation
}

func (m *Manager) declLogger(decl agentsdk.SubagentExecution) slog.Logger {
	return m.logger.With(
		slog.F("execution_id", decl.ExecutionID),
		slog.F("generation", decl.Generation),
		slog.F("name", decl.Name),
		slog.F("driver", decl.Driver),
	)
}
