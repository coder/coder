package chatd

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

type taskKind string

const (
	taskKindGeneration            taskKind = "generation"
	taskKindInterrupt             taskKind = "interrupt"
	taskKindRequiresActionTimeout taskKind = "requires_action_timeout"
)

type localWorkKey struct {
	historyVersion int64
	status         database.ChatStatus
}

type runner struct {
	ctx  context.Context
	mgr  *runnerManager
	rec  *runnerRecord
	opts chatWorkerOptions

	debugTurn    *runnerDebugTurn
	sessionStart sessionStartTracker
	stopNudges   stopNudgeTracker
}

func newRunner(ctx context.Context, mgr *runnerManager, rec *runnerRecord, opts chatWorkerOptions) *runner {
	return &runner{
		ctx:       ctx,
		mgr:       mgr,
		rec:       rec,
		opts:      opts,
		debugTurn: newRunnerDebugTurn(ctx, opts.Logger),
	}
}

type runnerActivity string

const (
	runnerRead      runnerActivity = "read"
	runnerOperation runnerActivity = "operation"
	runnerRetry     runnerActivity = "retry"
	runnerRelease   runnerActivity = "release"
)

func (r *runner) run() {
	executor := runnerExecutor{ctx: r.ctx, locked: make(map[localWorkKey]chan struct{})}
	defer r.closeDebugTurn()
	defer executor.close()
	// Stop heartbeats before joining operations that may still be draining.
	defer r.mgr.requestCleanup(context.WithoutCancel(r.ctx), r.rec.key)
	if !r.subscribe() {
		return
	}

	activity := runnerRead
	var state runnerStateUpdate
	var snapshotVersion, historyVersion int64
	var hasState, checkProgress bool
	var kind taskKind
	delay := r.opts.TaskRetryInitialBackoff
	for r.ctx.Err() == nil {
		var resultCh <-chan runnerActivityResult
		switch activity {
		case runnerRead:
			resultCh = executor.start(nil, func(ctx context.Context) runnerActivityResult {
				chat, err := r.opts.Store.GetChatByID(ctx, r.rec.key.ChatID)
				return runnerActivityResult{chat: chat, err: err}
			})
		case runnerOperation:
			input := chatWorkerTaskStartInput{
				TaskID:                   uuid.New(),
				ChatID:                   r.rec.key.ChatID,
				WorkerID:                 r.rec.workerID,
				RunnerID:                 r.rec.key.RunnerID,
				HistoryVersion:           state.HistoryVersion,
				GenerationAttempt:        state.GenerationAttempt,
				Status:                   state.Status,
				RequiresActionDeadlineAt: state.RequiresActionDeadlineAt,
				DebugTurn:                r.debugTurn,
				SessionStart:             &r.sessionStart,
				StopNudges:               &r.stopNudges,
			}
			operationKind := kind
			key := localWorkKey{historyVersion: state.HistoryVersion, status: state.Status}
			resultCh = executor.start(&key, func(ctx context.Context) runnerActivityResult {
				ctx, cancel := taskAttemptContext(ctx, r.opts.Clock, operationKind)
				defer cancel()
				err := executeTaskSafely(ctx, func(ctx context.Context) error {
					if err := ctx.Err(); err != nil {
						return err
					}
					switch operationKind {
					case taskKindGeneration:
						return r.opts.TaskStarter.StartGeneration(ctx, input)
					case taskKindInterrupt:
						return r.opts.TaskStarter.StartInterrupt(ctx, input)
					default:
						return r.opts.TaskStarter.StartRequiresActionTimeout(ctx, input)
					}
				})
				if err != nil && errors.Is(context.Cause(ctx), errTaskTimeout) &&
					(!errors.Is(err, errTaskExpectedExit) || errors.Is(err, context.Canceled) ||
						errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errTaskTimeout)) {
					err = errors.Join(errTaskTimeout, errTaskRetryable, err)
				}
				return runnerActivityResult{err: err}
			})
		case runnerRetry:
			retryDelay, retryKind := delay, kind
			resultCh = executor.start(nil, func(ctx context.Context) runnerActivityResult {
				timer := r.opts.Clock.NewTimer(retryDelay, "chatworker", "task-retry-"+string(retryKind))
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-ctx.Done():
				}
				return runnerActivityResult{}
			})
			delay = min(delay*2, r.opts.TaskRetryMaxBackoff)
		case runnerRelease:
			releaseState := state
			resultCh = executor.start(nil, func(ctx context.Context) runnerActivityResult {
				released, err := r.mgr.release(ctx, r.rec, releaseState)
				return runnerActivityResult{released: released, err: err}
			})
		}

	wait:
		for {
			select {
			case <-r.ctx.Done():
				return
			case hint := <-r.rec.stateCh:
				if !hasState || hint.SnapshotVersion <= snapshotVersion || hint.HistoryVersion < historyVersion {
					continue
				}
				snapshotVersion = hint.SnapshotVersion
				historyVersion = hint.HistoryVersion
				// Attempts and queue changes do not obsolete healthy work.
				// Hints invalidate work; only a read selects its replacement.
				if activity != runnerRead && (!r.owns(hint) || !sameRunnerWork(state, hint)) {
					activity = runnerRead
					checkProgress = false
					break wait
				}
			case result := <-resultCh:
				if activity == runnerRead && errors.Is(result.err, sql.ErrNoRows) {
					return
				}
				if result.err != nil && (activity != runnerOperation || !errors.Is(result.err, errTaskExpectedExit) || errors.Is(result.err, errTaskRetryable)) {
					r.opts.Logger.Warn(r.ctx, "chatworker runner retrying",
						slog.F("activity", activity), slog.F("task_kind", kind),
						slog.F("chat_id", r.rec.key.ChatID), slog.F("delay", delay), slogError(result.err))
					activity = runnerRetry
					checkProgress = false
					break wait
				}
				switch activity {
				case runnerRead:
					next := stateUpdateFromChat(result.chat)
					// A read may advance history at an equal snapshot, but
					// cannot regress behind state already observed locally.
					if next.SnapshotVersion < snapshotVersion || next.HistoryVersion < historyVersion {
						activity = runnerRetry
						checkProgress = false
						break
					}
					if !r.owns(next) {
						return
					}
					unchanged := hasState && sameRunnerWork(state, next)
					state, hasState = next, true
					snapshotVersion, historyVersion = next.SnapshotVersion, next.HistoryVersion
					if !unchanged {
						delay = r.opts.TaskRetryInitialBackoff
					}
					if checkProgress && unchanged {
						activity = runnerRetry
					} else {
						activity = runnerOperation
						switch {
						case state.Archived:
							activity = runnerRelease
						case state.Status == database.ChatStatusRunning:
							kind = taskKindGeneration
						case state.Status == database.ChatStatusInterrupting:
							kind = taskKindInterrupt
						case state.Status == database.ChatStatusRequiresAction:
							kind = taskKindRequiresActionTimeout
						default:
							activity = runnerRelease
						}
					}
					checkProgress = false
				case runnerOperation, runnerRelease:
					if result.released {
						return
					}
					checkProgress = true
					activity = runnerRead
				case runnerRetry:
					activity = runnerRead
				}
				break wait
			}
		}
	}
}

func (r *runner) owns(state runnerStateUpdate) bool {
	return uuidPtrEqual(state.WorkerID, r.rec.workerID) && uuidPtrEqual(state.RunnerID, r.rec.key.RunnerID)
}

func sameRunnerWork(a, b runnerStateUpdate) bool {
	return a.HistoryVersion == b.HistoryVersion && a.Status == b.Status && a.Archived == b.Archived
}

func (r *runner) subscribe() bool {
	channel := coderdpubsub.ChatStateUpdateChannel(r.rec.key.ChatID)
	unsubscribe, err := r.opts.Pubsub.SubscribeWithErr(channel, coderdpubsub.HandleChatStateUpdate(
		func(ctx context.Context, payload coderdpubsub.ChatStateUpdateMessage, err error) {
			if err != nil {
				r.opts.Logger.Warn(ctx, "chatworker state update decode failed", slogError(err))
				return
			}
			r.mgr.RouteStateHint(ctx, stateUpdateFromPubsub(r.rec.key.ChatID, payload))
		},
	))
	if err != nil {
		r.opts.Logger.Warn(r.ctx, "chatworker runner subscribe failed", slogError(err))
		return false
	}
	return r.rec.setUnsubscribe(unsubscribe)
}

func stateUpdateFromPubsub(chatID uuid.UUID, payload coderdpubsub.ChatStateUpdateMessage) runnerStateUpdate {
	return runnerStateUpdate{
		ChatID:            chatID,
		WorkerID:          payload.WorkerID,
		RunnerID:          payload.RunnerID,
		SnapshotVersion:   payload.SnapshotVersion,
		HistoryVersion:    payload.HistoryVersion,
		QueueVersion:      payload.QueueVersion,
		GenerationAttempt: payload.GenerationAttempt,
		Status:            database.ChatStatus(payload.Status),
		Archived:          payload.Archived,
	}
}

func (r *runner) closeDebugTurn() {
	if r.debugTurn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.ctx), debugFinalizeTimeout)
	defer cancel()
	r.debugTurn.Finalize(ctx)
}

func uuidPtrEqual(got *uuid.UUID, want uuid.UUID) bool {
	return got != nil && *got == want
}

type runnerActivityResult struct {
	chat     database.Chat
	released bool
	err      error
}

// runnerExecutor owns cancellation and draining, not scheduling. Each result
// channel belongs to one invocation, so obsolete completions cannot advance
// the runner. Operations with the same history and status drain serially.
type runnerExecutor struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	locked map[localWorkKey]chan struct{}
}

func (e *runnerExecutor) start(key *localWorkKey, fn func(context.Context) runnerActivityResult) <-chan runnerActivityResult {
	if e.cancel != nil {
		e.cancel()
	}
	ctx, cancel := context.WithCancel(e.ctx)
	e.cancel = cancel
	resultCh := make(chan runnerActivityResult, 1)
	e.wg.Go(func() {
		defer cancel()
		if key != nil {
			unlock, ok := e.acquire(ctx, *key)
			if !ok {
				resultCh <- runnerActivityResult{err: ctx.Err()}
				return
			}
			defer unlock()
		}
		var result runnerActivityResult
		result.err = executeTaskSafely(ctx, func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			result = fn(ctx)
			return result.err
		})
		resultCh <- result
	})
	return resultCh
}

func (e *runnerExecutor) close() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}

func (e *runnerExecutor) acquire(ctx context.Context, key localWorkKey) (func(), bool) {
	for {
		e.mu.Lock()
		wait, ok := e.locked[key]
		if !ok {
			released := make(chan struct{})
			e.locked[key] = released
			e.mu.Unlock()
			return func() {
				e.mu.Lock()
				delete(e.locked, key)
				close(released)
				e.mu.Unlock()
			}, true
		}
		e.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, false
		}
	}
}
