package agentchat

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentchat/chatexec"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/quartz"
)

const (
	defaultPollInterval      = 2 * time.Second
	defaultHeartbeatInterval = 15 * time.Second
	defaultMaxConcurrent     = 5
	detachedRPCTimeout       = 5 * time.Second
	maxPollChatRequest       = int32(^uint32(0) >> 1)
)

var (
	errRunnerAlreadyStarted = xerrors.New("runner already started")
	errRunnerClosed         = xerrors.New("runner is closed")
	errClientRequired       = xerrors.New("client is required")
	errLoggerRequired       = xerrors.New("logger is required")
	errContextRequired      = xerrors.New("context is required")
	errPositiveValue        = xerrors.New("must be positive")
)

// CoordinationClient is the narrow RPC interface the runner depends on.
type CoordinationClient interface {
	ReportChatRunnerStatus(ctx context.Context, in *proto.ReportChatRunnerStatusRequest) (*proto.ReportChatRunnerStatusResponse, error)
	PollChatWork(ctx context.Context, in *proto.PollChatWorkRequest) (*proto.PollChatWorkResponse, error)
	AcquireChatLease(ctx context.Context, in *proto.AcquireChatLeaseRequest) (*proto.AcquireChatLeaseResponse, error)
	RenewChatLease(ctx context.Context, in *proto.RenewChatLeaseRequest) (*proto.RenewChatLeaseResponse, error)
	ReleaseChatLease(ctx context.Context, in *proto.ReleaseChatLeaseRequest) (*proto.ReleaseChatLeaseResponse, error)
}

// Executor runs a chat to completion.
type Executor interface {
	Execute(ctx context.Context, chatID uuid.UUID) error
}

type executorFunc func(ctx context.Context, chatID uuid.UUID) error

func (fn executorFunc) Execute(ctx context.Context, chatID uuid.UUID) error {
	return fn(ctx, chatID)
}

// Options configures a Runner.
type Options struct {
	Client            CoordinationClient
	Logger            slog.Logger
	Clock             quartz.Clock
	PollInterval      time.Duration
	MaxConcurrent     int
	HeartbeatInterval time.Duration
	Executor          Executor
}

// Runner coordinates chat execution for a single agent connection.
type Runner struct {
	client            CoordinationClient
	logger            slog.Logger
	clock             quartz.Clock
	pollInterval      time.Duration
	maxConcurrent     int
	heartbeatInterval time.Duration
	executor          Executor

	mu sync.Mutex
	// activeChats also guards per-chat mutable state. Keeping the map,
	// cancel function, and lease-loss flag under one lock avoids ordering bugs
	// between polling, heartbeats, and worker cleanup.
	activeChats    map[uuid.UUID]*chatExecution
	ctx            context.Context
	cancel         context.CancelFunc
	readyReported  bool
	workerCloseErr error

	wg        sync.WaitGroup
	started   atomic.Bool
	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

// chatExecution tracks the state needed to execute and release one chat.
type chatExecution struct {
	chatID      uuid.UUID
	leaseEpoch  int64
	cancel      context.CancelFunc
	leaseLost   bool
	releaseOnce sync.Once
}

// New constructs a chat runner from the provided options.
func New(opts Options) (*Runner, error) {
	if opts.Client == nil {
		return nil, xerrors.Errorf("client: %w", errClientRequired)
	}
	if reflect.ValueOf(opts.Logger).IsZero() {
		return nil, xerrors.Errorf("logger: %w", errLoggerRequired)
	}

	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = defaultPollInterval
	}
	if opts.HeartbeatInterval == 0 {
		opts.HeartbeatInterval = defaultHeartbeatInterval
	}
	if opts.MaxConcurrent == 0 {
		opts.MaxConcurrent = defaultMaxConcurrent
	}
	if opts.Executor == nil {
		opts.Executor = executorFunc(func(ctx context.Context, chatID uuid.UUID) error {
			opts.Logger.Info(
				ctx,
				"would run chat (placeholder)",
				slog.F("chat_id", chatID),
			)
			return nil
		})
	}

	if opts.PollInterval <= 0 {
		return nil, xerrors.Errorf("poll interval %s: %w", opts.PollInterval, errPositiveValue)
	}
	if opts.HeartbeatInterval <= 0 {
		return nil, xerrors.Errorf("heartbeat interval %s: %w", opts.HeartbeatInterval, errPositiveValue)
	}
	if opts.MaxConcurrent <= 0 {
		return nil, xerrors.Errorf("max concurrent %d: %w", opts.MaxConcurrent, errPositiveValue)
	}

	return &Runner{
		client:            opts.Client,
		logger:            opts.Logger,
		clock:             opts.Clock,
		pollInterval:      opts.PollInterval,
		maxConcurrent:     opts.MaxConcurrent,
		heartbeatInterval: opts.HeartbeatInterval,
		executor:          opts.Executor,
		activeChats:       make(map[uuid.UUID]*chatExecution),
	}, nil
}

// Start reports readiness, polls once immediately, and starts background loops.
func (r *Runner) Start(ctx context.Context) error {
	if ctx == nil {
		return xerrors.Errorf("context: %w", errContextRequired)
	}
	if r.closed.Load() {
		return xerrors.Errorf("start: %w", errRunnerClosed)
	}
	if !r.started.CompareAndSwap(false, true) {
		return xerrors.Errorf("start: %w", errRunnerAlreadyStarted)
	}

	runnerCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.ctx = runnerCtx
	r.cancel = cancel
	r.mu.Unlock()

	_, err := r.client.ReportChatRunnerStatus(runnerCtx, &proto.ReportChatRunnerStatusRequest{Ready: true})
	if err != nil {
		cancel()
		if drpcerr.Code(err) == drpcerr.Unimplemented {
			r.logger.Info(ctx, "chat runner coordination is unavailable", slog.Error(err))
			return nil
		}
		return xerrors.Errorf("report chat runner ready: %w", err)
	}

	r.mu.Lock()
	r.readyReported = true
	r.mu.Unlock()

	r.pollOnce(runnerCtx)

	r.wg.Add(2)
	go r.pollLoop(runnerCtx)
	go r.heartbeatLoop(runnerCtx)
	return nil
}

func (r *Runner) pollLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := r.clock.NewTicker(r.pollInterval, "agentchat", "poll")
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.pollOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (r *Runner) heartbeatLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := r.clock.NewTicker(r.heartbeatInterval, "agentchat", "heartbeat")
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.heartbeatOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (r *Runner) heartbeatOnce(ctx context.Context) {
	r.mu.Lock()
	active := make([]*chatExecution, 0, len(r.activeChats))
	for _, exec := range r.activeChats {
		// Assert that heartbeat traffic only applies to acquired leases. The
		// poller inserts placeholders before acquisition to block duplicate work.
		if exec.leaseEpoch <= 0 || exec.leaseLost {
			continue
		}
		active = append(active, exec)
	}
	r.mu.Unlock()

	for _, exec := range active {
		resp, err := r.client.RenewChatLease(ctx, &proto.RenewChatLeaseRequest{
			ChatId:     exec.chatID[:],
			LeaseEpoch: exec.leaseEpoch,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			r.loseLease(ctx, exec, xerrors.Errorf("renew chat lease: %w", err))
			continue
		}
		if resp != nil && resp.GetRenewed() {
			continue
		}
		r.loseLease(ctx, exec, nil)
	}
}

func (r *Runner) loseLease(ctx context.Context, exec *chatExecution, err error) {
	r.mu.Lock()
	if exec.leaseLost {
		r.mu.Unlock()
		return
	}
	exec.leaseLost = true
	cancel := exec.cancel
	r.mu.Unlock()

	fields := []slog.Field{
		slog.F("chat_id", exec.chatID),
		slog.F("lease_epoch", exec.leaseEpoch),
	}
	if err != nil {
		fields = append(fields, slog.Error(err))
	}
	r.logger.Warn(ctx, "chat lease was lost", fields...)
	if cancel != nil {
		cancel()
	}
}

func (r *Runner) pollOnce(ctx context.Context) {
	r.mu.Lock()
	freeCapacity := r.maxConcurrent - len(r.activeChats)
	r.mu.Unlock()
	if freeCapacity <= 0 {
		return
	}

	if freeCapacity > int(maxPollChatRequest) {
		freeCapacity = int(maxPollChatRequest)
	}

	// #nosec G115 - freeCapacity is clamped to maxPollChatRequest above.
	resp, err := r.client.PollChatWork(ctx, &proto.PollChatWorkRequest{MaxChats: int32(freeCapacity)})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		r.logger.Warn(ctx, "poll chat work failed", slog.F("max_chats", freeCapacity), slog.Error(err))
		return
	}
	if resp == nil {
		return
	}

	for _, item := range resp.GetWorkItems() {
		if item == nil {
			r.logger.Warn(ctx, "skipping nil chat work item")
			continue
		}

		chatID, err := uuid.FromBytes(item.GetChatId())
		if err != nil {
			r.logger.Warn(ctx, "skipping chat work item with invalid chat id", slog.Error(err))
			continue
		}

		r.mu.Lock()
		if len(r.activeChats) >= r.maxConcurrent {
			r.mu.Unlock()
			return
		}
		if _, exists := r.activeChats[chatID]; exists {
			r.mu.Unlock()
			continue
		}
		exec := &chatExecution{chatID: chatID}
		r.activeChats[chatID] = exec
		r.mu.Unlock()

		acquireResp, err := r.client.AcquireChatLease(ctx, &proto.AcquireChatLeaseRequest{ChatId: item.GetChatId()})
		if err != nil {
			if ctx.Err() == nil {
				r.logger.Warn(
					ctx,
					"acquire chat lease failed",
					slog.F("chat_id", chatID),
					slog.Error(err),
				)
			}
			r.mu.Lock()
			delete(r.activeChats, chatID)
			r.mu.Unlock()
			continue
		}

		leaseEpoch := int64(0)
		leaseStatus := ""
		if acquireResp != nil {
			leaseEpoch = acquireResp.GetLeaseEpoch()
			leaseStatus = acquireResp.GetStatus()
		}
		if leaseEpoch <= 0 {
			r.logger.Warn(
				ctx,
				"chat lease acquisition returned no lease",
				slog.F("chat_id", chatID),
				slog.F("status", leaseStatus),
			)
			r.mu.Lock()
			delete(r.activeChats, chatID)
			r.mu.Unlock()
			continue
		}

		r.mu.Lock()
		exec.leaseEpoch = leaseEpoch
		r.mu.Unlock()

		r.wg.Add(1)
		go func(exec *chatExecution) {
			defer r.wg.Done()
			r.runChat(exec)
		}(exec)
	}
}

func (r *Runner) runChat(exec *chatExecution) {
	r.mu.Lock()
	runnerCtx := r.ctx
	r.mu.Unlock()

	childCtx, cancel := context.WithCancel(runnerCtx)
	defer cancel()

	r.mu.Lock()
	exec.cancel = cancel
	leaseLost := exec.leaseLost
	r.mu.Unlock()
	if leaseLost {
		cancel()
	}

	r.logger.Info(
		childCtx,
		"starting chat execution",
		slog.F("chat_id", exec.chatID),
		slog.F("lease_epoch", exec.leaseEpoch),
	)

	var releaseReq *proto.ReleaseChatLeaseRequest
	defer func() {
		r.mu.Lock()
		delete(r.activeChats, exec.chatID)
		r.mu.Unlock()

		exec.releaseOnce.Do(func() {
			r.mu.Lock()
			leaseLost := exec.leaseLost
			r.mu.Unlock()
			if leaseLost {
				return
			}

			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), detachedRPCTimeout)
			defer releaseCancel()

			_, err := r.client.ReleaseChatLease(releaseCtx, releaseReq)
			if err == nil {
				return
			}

			wrappedErr := xerrors.Errorf("release chat lease for %s: %w", exec.chatID, err)
			r.logger.Error(
				context.Background(),
				"release chat lease failed",
				slog.F("chat_id", exec.chatID),
				slog.F("lease_epoch", exec.leaseEpoch),
				slog.Error(wrappedErr),
			)
			r.recordWorkerCloseError(wrappedErr)
		})
	}()

	err := r.executor.Execute(childCtx, exec.chatID)
	requiresAction := errors.Is(err, chatexec.ErrRequiresAction)
	if err != nil && !requiresAction {
		r.logger.Warn(
			childCtx,
			"chat execution failed",
			slog.F("chat_id", exec.chatID),
			slog.F("lease_epoch", exec.leaseEpoch),
			slog.Error(err),
		)
	}

	finalStatus := "waiting"
	finalError := ""
	switch {
	case requiresAction:
		finalStatus = "requires_action"
	case err != nil:
		finalStatus = "error"
		finalError = err.Error()
	case childCtx.Err() != nil:
		finalStatus = "error"
		finalError = childCtx.Err().Error()
	}

	releaseReq = &proto.ReleaseChatLeaseRequest{
		ChatId:      exec.chatID[:],
		LeaseEpoch:  exec.leaseEpoch,
		FinalStatus: finalStatus,
		Error:       finalError,
	}
}

func (r *Runner) recordWorkerCloseError(err error) {
	if err == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.workerCloseErr = errors.Join(r.workerCloseErr, err)
}

// Close stops background work, waits for in-flight chats, and reports that the
// runner is no longer ready.
func (r *Runner) Close() error {
	r.closeOnce.Do(func() {
		r.closed.Store(true)

		r.mu.Lock()
		cancel := r.cancel
		r.mu.Unlock()
		if cancel != nil {
			cancel()
		}

		r.wg.Wait()

		var readyErr error
		r.mu.Lock()
		readyReported := r.readyReported
		workerCloseErr := r.workerCloseErr
		r.mu.Unlock()
		if readyReported {
			reportCtx, reportCancel := context.WithTimeout(context.Background(), detachedRPCTimeout)
			defer reportCancel()

			_, err := r.client.ReportChatRunnerStatus(reportCtx, &proto.ReportChatRunnerStatusRequest{Ready: false})
			if err != nil {
				readyErr = xerrors.Errorf("report chat runner not ready: %w", err)
			}
		}

		r.closeErr = errors.Join(workerCloseErr, readyErr)
	})
	return r.closeErr
}
