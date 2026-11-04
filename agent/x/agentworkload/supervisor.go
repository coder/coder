package agentworkload

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"maps"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const operationTimeout = 30 * time.Second

type record struct {
	Request         codersdk.StartHostWorkloadRequest `json:"request"`
	Workload        codersdk.Workload                 `json:"workload"`
	Reserved        bool                              `json:"reserved"`
	CancelRequested bool                              `json:"cancel_requested"`
	output          retainedOutput
	outputBytes     int
	outputOnDisk    bool
	cancel          chan struct{}
	changed         chan struct{}
	input           inputAttachment
	inputCtx        context.Context
	inputCancel     context.CancelFunc
	inputController bool
	inputClosed     bool
}

type durableState struct {
	Version  int                   `json:"version"`
	Draining bool                  `json:"draining"`
	Runs     map[uuid.UUID]*record `json:"runs"`
}

// Supervisor admits and observes workloads through its authenticated HTTP API.
// Its mutex is the authority for all outstanding resource reservations.
type Supervisor struct {
	mu         sync.Mutex
	config     Config
	generation codersdk.WorkloadGeneration
	tokenHash  [32]byte
	state      durableState
	store      *store
	runtime    workloadRuntime
	clock      quartz.Clock
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	closed     bool
	failed     bool
	closeOnce  sync.Once
	closeErr   error
}

func newSupervisor(ctx context.Context, cfg Config, rt workloadRuntime) (_ *Supervisor, err error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	st, err := openStore(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = st.close()
		}
	}()
	s := &Supervisor{
		config:    cfg,
		tokenHash: sha256.Sum256([]byte(cfg.Token)),
		store:     st,
		runtime:   rt,
		clock:     quartz.NewReal(),
		state:     durableState{Version: 1, Runs: make(map[uuid.UUID]*record)},
	}
	s.config.Repositories = maps.Clone(cfg.Repositories)
	var loaded durableState
	if err := st.read("state.json", 1<<30, &loaded); err == nil {
		s.state = loaded
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, xerrors.New("read durable workload state; restore valid private state before enrollment")
	}
	if s.state.Version != 1 || s.state.Runs == nil || len(s.state.Runs) > maxHistory {
		return nil, xerrors.New("invalid durable workload state")
	}
	for id, run := range s.state.Runs {
		if run == nil || id != run.Request.ID || id != run.Workload.ID || run.Request.Validate() != nil {
			return nil, xerrors.New("invalid durable workload record")
		}
		if run.Reserved || !run.Workload.Terminal() || run.Workload.State == "unknown" || run.Workload.CleanupPending {
			return nil, xerrors.New("outstanding workload state: reconcile coder-workload-<UUID> containers and private state with an operator; automatic replay is disabled; do not delete state or resubmit commands")
		}
		var output retainedOutput
		if err := st.read(outputName(id), 8<<20, &output); err != nil || !output.valid() {
			return nil, xerrors.New("read retained workload output; restore private state before enrollment")
		}
		run.outputOnDisk = true
	}
	validateCtx, cancelValidation := context.WithTimeout(ctx, operationTimeout)
	defer cancelValidation()
	imageID, err := rt.Validate(validateCtx)
	if err != nil {
		return nil, err
	}
	s.generation = codersdk.WorkloadGeneration{ID: imageID, Repositories: maps.Clone(cfg.Repositories), Workdir: cfg.Workdir}
	// An explicit restart may reopen a drained host only after every retained
	// run is terminal and the dedicated daemon has no surviving containers.
	s.state.Draining = false
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s, nil
}

// Close detaches all Docker observation connections without killing commands.
// Active records retain reservations and cause subsequent startup to fail closed.
func (s *Supervisor) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.cancel()
		s.mu.Unlock()
		s.wg.Wait()
		s.closeErr = errors.Join(s.runtime.Close(), s.store.close())
	})
	return s.closeErr
}

func (s *Supervisor) persistLocked() error {
	if s.closed {
		return xerrors.New("workload supervisor is detached")
	}
	if err := s.store.write("state.json", s.state); err != nil {
		s.failed = true
		return xerrors.New("persist workload state; supervisor is unavailable until operator reconciliation")
	}
	return nil
}

func (s *Supervisor) infoLocked() codersdk.WorkloadHostInfo {
	info := codersdk.WorkloadHostInfo{
		Generation: s.generation, Runtime: "docker-overlay2-cgroupv2",
		CPU: s.config.CPU, MemoryBytes: s.config.MemoryBytes,
		AvailableCPU: s.config.CPU, AvailableMemoryBytes: s.config.MemoryBytes,
		Draining: s.state.Draining || s.failed || s.closed,
	}
	for _, run := range s.state.Runs {
		if run.Reserved {
			info.AvailableCPU -= run.Request.CPU
			info.AvailableMemoryBytes -= run.Request.MemoryBytes + runtimeOverhead
		}
	}
	return info
}

func containerName(id uuid.UUID) string { return "coder-workload-" + id.String() }

func (s *Supervisor) detached() bool { return s.ctx.Err() != nil }

func (*Supervisor) canceled(run *record) bool {
	select {
	case <-run.cancel:
		return true
	default:
		return false
	}
}

func (s *Supervisor) operation(fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(s.ctx, operationTimeout)
	defer cancel()
	return fn(ctx)
}

func (s *Supervisor) unknown(run *record, failure string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unknownLocked(run, failure)
}

func (s *Supervisor) unknownLocked(run *record, failure string) {
	if s.detached() {
		return
	}
	run.Workload.State = "unknown"
	run.Workload.Failure = failure
	run.Workload.CleanupPending = true
	s.state.Draining = true
	s.notifyLocked(run)
	//nolint:errcheck // Admission is closed; persistLocked also marks failures.
	_ = s.persistLocked()
}

func (s *Supervisor) execute(run *record) {
	defer s.wg.Done()
	defer func() {
		run.inputCancel()
		s.mu.Lock()
		run.input = nil
		s.mu.Unlock()
	}()
	if s.detached() {
		return
	}
	if s.canceled(run) {
		s.finish(run, "canceled", nil, false)
		return
	}
	name := containerName(run.Request.ID)
	if err := s.operation(func(ctx context.Context) error {
		started := s.clock.Now()
		err := s.runtime.Create(ctx, name, run.Request)
		duration := s.clock.Since(started).Nanoseconds()
		s.mu.Lock()
		run.Workload.Timings.ProvisionNS = &duration
		s.notifyLocked(run)
		s.mu.Unlock()
		return err
	}); err != nil {
		s.unknown(run, "container creation outcome is unknown; operator reconciliation required")
		return
	}
	if s.detached() {
		return
	}
	if s.canceled(run) {
		s.cancelContainer(run, name)
		return
	}

	var attach io.ReadCloser
	err := s.operation(func(ctx context.Context) error {
		var err error
		if rt, ok := s.runtime.(inputRuntime); run.Request.Stdin && ok {
			attach, err = rt.AttachInput(ctx, name)
		} else {
			attach, err = s.runtime.Attach(ctx, name)
		}
		return err
	})
	if err != nil {
		s.unknown(run, "container attachment failed; operator reconciliation required")
		return
	}
	// A hijacked Docker connection is not necessarily canceled with its request.
	// Explicitly close it on detachment, without signaling the container.
	stopClose := context.AfterFunc(s.ctx, func() { _ = attach.Close() })
	defer stopClose()
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		//nolint:errcheck // collect marks incomplete output as truncated.
		_ = s.collect(run, attach)
	}()
	stopCollection := func() { _ = attach.Close(); <-outputDone }
	defer stopCollection()

	if s.detached() {
		return
	}
	if s.canceled(run) {
		stopCollection()
		s.cancelContainer(run, name)
		return
	}
	if err := s.operation(func(ctx context.Context) error { return s.runtime.Start(ctx, name) }); err != nil {
		stopCollection()
		s.unknown(run, "container start outcome is unknown; command will not be replayed")
		return
	}
	s.mu.Lock()
	if s.detached() {
		s.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	run.Workload.State = "running"
	run.Workload.StartedAt = &now
	err = s.persistLocked()
	if err != nil {
		run.output.Truncated = true
	} else {
		if run.Request.Stdin {
			run.input, _ = attach.(inputAttachment)
		}
		s.notifyLocked(run)
	}
	s.mu.Unlock()
	if err != nil {
		// Stop output before publishing a terminal state, so its empty batch
		// cannot race trailing output. Never cancel or remove the container.
		stopCollection()
		s.unknown(run, "running workload state could not be persisted; operator reconciliation required; command will not be replayed")
		return
	}
	if s.detached() {
		return
	}

	waitCtx, stopWait := context.WithCancel(s.ctx)
	defer stopWait()
	var exitCode int
	var waitErr error
	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		exitCode, waitErr = s.runtime.Wait(waitCtx, name)
	}()
	defer func() { stopWait(); <-waitDone }()
	select {
	case <-s.ctx.Done():
		return
	case <-run.cancel:
		stopCollection()
		s.cancelContainer(run, name)
	case <-waitDone:
		// Stop input immediately, even while trailing output is draining.
		run.inputCancel()
		if waitErr != nil {
			stopCollection()
			s.unknown(run, "container exit outcome is unknown; command will not be replayed")
			return
		}
		// Docker timestamps exclude attachment drain and state persistence. A
		// missing measurement must not change the observed command outcome.
		_ = s.operation(func(ctx context.Context) error {
			duration, err := s.runtime.ExecutionDuration(ctx, name)
			if err == nil {
				s.mu.Lock()
				run.Workload.Timings.ExecNS = new(duration.Nanoseconds())
				s.notifyLocked(run)
				s.mu.Unlock()
			}
			return err
		})
		// Wait for trailing output, but never let a broken attach stream prevent
		// cleanup forever. Closing only this connection preserves execution.
		outputCtx, stopOutput := context.WithTimeout(s.ctx, operationTimeout)
		defer stopOutput()
		select {
		case <-outputDone:
		case <-outputCtx.Done():
			s.truncate(run)
			stopCollection()
		}
		if s.detached() {
			return
		}
		state := "succeeded"
		if exitCode != 0 {
			state = "failed"
		}
		s.finish(run, state, &exitCode, true)
	}
}

func (s *Supervisor) cancelContainer(run *record, name string) {
	if s.detached() {
		return
	}
	if err := s.operation(func(ctx context.Context) error { return s.runtime.Remove(ctx, name) }); err != nil {
		s.unknown(run, "container cancellation outcome is unknown; operator reconciliation required")
		return
	}
	// Cancellation may race trailing output. Do not promise a complete stream.
	s.truncate(run)
	s.finish(run, "canceled", nil, false)
}

//nolint:revive // remove distinguishes observed exit from confirmed cancellation.
func (s *Supervisor) finish(run *record, state string, code *int, remove bool) {
	s.mu.Lock()
	if s.detached() {
		s.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	run.Workload.State = state
	run.Workload.ExitCode = code
	run.Workload.FinishedAt = &now
	s.notifyLocked(run)
	if err := s.store.write(outputName(run.Request.ID), run.output); err != nil {
		s.failed = true
		s.mu.Unlock()
		return
	}
	if err := s.persistLocked(); err != nil {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	if remove {
		if err := s.operation(func(ctx context.Context) error { return s.runtime.Remove(ctx, containerName(run.Request.ID)) }); err != nil {
			s.mu.Lock()
			run.Workload.Failure = "container cleanup is unconfirmed; reservation retained"
			s.notifyLocked(run)
			s.state.Draining = true
			//nolint:errcheck // Keep the reservation; persistence fails closed.
			_ = s.persistLocked()
			s.mu.Unlock()
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run.Reserved = false
	run.Workload.CleanupPending = false
	s.notifyLocked(run)
	if err := s.persistLocked(); err != nil {
		run.Reserved = true
		run.Workload.CleanupPending = true
		return
	}
	// Completed history must not retain up to 4 MiB per key in agent memory.
	run.outputOnDisk = true
	run.output = retainedOutput{}
	run.outputBytes = 0
}
