package chatd

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

const runnerRefreshTimeout = 5 * time.Second

// runnerObservation belongs to one completed task. Its result is readable
// after done closes; cancellation does not release the single-flight slot.
type runnerObservation struct {
	task   *taskRecord
	cancel context.CancelFunc
	done   <-chan struct{}
	chat   database.Chat
	err    error
}

type taskKind string

const (
	taskKindGeneration            taskKind = "generation"
	taskKindInterrupt             taskKind = "interrupt"
	taskKindRequiresActionTimeout taskKind = "requires_action_timeout"
	taskKindAbandon               taskKind = "abandon"
)

type taskInstanceID uuid.UUID

type localWorkKey struct {
	historyVersion int64
	status         database.ChatStatus
}

type taskRecord struct {
	id       taskInstanceID
	kind     taskKind
	localKey localWorkKey
	cancel   context.CancelFunc
	done     <-chan struct{}
}

type runner struct {
	ctx  context.Context
	mgr  *runnerManager
	rec  *runnerRecord
	opts chatWorkerOptions

	lastSnapshotVersion int64
	hasAcceptedState    bool
	latestState         runnerStateUpdate

	// The current task remains here after completion until an authoritative
	// observation reconciles it or a state notification supersedes it.
	currentTask  *taskRecord
	tasks        map[taskInstanceID]*taskRecord
	localLocks   *localLockSet
	debugTurn    *runnerDebugTurn
	sessionStart sessionStartTracker
	stopNudges   stopNudgeTracker
}

func newRunner(ctx context.Context, mgr *runnerManager, rec *runnerRecord, opts chatWorkerOptions) *runner {
	return &runner{
		ctx:        ctx,
		mgr:        mgr,
		rec:        rec,
		opts:       opts,
		tasks:      make(map[taskInstanceID]*taskRecord),
		localLocks: newLocalLockSet(),
		debugTurn:  newRunnerDebugTurn(ctx, opts.Logger),
	}
}

func (r *runner) run() {
	var observation *runnerObservation
	defer func() {
		if observation != nil {
			observation.cancel()
		}
		r.cancelActiveTask()
		r.waitForTasks()
		if observation != nil {
			<-observation.done
		}
		r.closeDebugTurn()
	}()
	if !r.bootstrap() {
		return
	}

	var delay time.Duration
	for {
		if r.ctx.Err() != nil {
			return
		}
		r.removeFinishedTasks()
		var taskDone, observed <-chan struct{}
		if observation != nil {
			observed = observation.done
			if observation.task != r.currentTask {
				// Keep the slot until the canceled read actually returns.
				observation.cancel()
			}
		} else if r.currentTask != nil {
			taskDone = r.currentTask.done
		}
		select {
		case state := <-r.rec.stateCh:
			if state.SnapshotVersion > r.lastSnapshotVersion && r.requiredWorkChanged(state) {
				delay = 0
			}
			if !r.processState(state) {
				return
			}
		case <-taskDone:
			observation = r.observeTask(r.currentTask, delay)
			if delay == 0 {
				delay = r.opts.TaskRetryInitialBackoff
			} else {
				// Subtract before adding to avoid overflowing durations.
				delay += min(delay, r.opts.TaskRetryMaxBackoff-delay)
			}
		case <-observed:
			completed := observation
			observation = nil
			if r.ctx.Err() != nil {
				return
			}
			if completed.task != r.currentTask {
				continue
			}
			if errors.Is(completed.err, sql.ErrNoRows) {
				r.mgr.requestCleanup(r.ctx, r.rec.key)
				return
			}
			if completed.err != nil {
				r.opts.Logger.Warn(r.ctx, "chatworker runner refresh failed", slogError(completed.err))
				continue
			}
			state := stateUpdateFromChat(completed.chat)
			if state.SnapshotVersion < r.lastSnapshotVersion || state.HistoryVersion < r.latestState.HistoryVersion {
				continue
			}
			if !uuidPtrEqual(state.WorkerID, r.rec.workerID) || !uuidPtrEqual(state.RunnerID, r.rec.key.RunnerID) {
				r.mgr.requestCleanup(r.ctx, r.rec.key)
				return
			}
			if r.requiredWorkChanged(state) {
				delay = 0
			}
			// Only an authoritative read may restore missing work at an
			// equal snapshot. Keep its fields together, without merging hints.
			r.cancelActiveTask()
			r.acceptState(state)
			r.spawnForState(state)
		case <-r.ctx.Done():
			return
		}
	}
}

// observeTask waits before reading current state without blocking the runner.
// Only the runner applies the result after observing done.
func (r *runner) observeTask(task *taskRecord, delay time.Duration) *runnerObservation {
	ctx, cancel := context.WithCancel(r.ctx)
	done := make(chan struct{})
	observation := &runnerObservation{task: task, cancel: cancel, done: done}
	store, clock, chatID := r.opts.Store, r.opts.Clock, r.rec.key.ChatID
	go func() {
		defer close(done)
		defer cancel()
		if delay > 0 {
			timer := clock.NewTimer(delay, "chatworker", "runner-recovery")
			select {
			case <-timer.C:
			case <-ctx.Done():
			}
			timer.Stop()
		}
		if ctx.Err() != nil {
			observation.err = ctx.Err()
			return
		}
		readCtx, cancelRead := context.WithCancelCause(ctx)
		defer cancelRead(nil)
		timer := clock.AfterFunc(runnerRefreshTimeout, func() {
			cancelRead(context.DeadlineExceeded)
		}, "chatworker", "runner-refresh-timeout")
		defer timer.Stop()
		observation.chat, observation.err = store.GetChatByID(readCtx, chatID)
		if readCtx.Err() != nil {
			observation.err = context.Cause(readCtx)
		}
	}()
	return observation
}

func (r *runner) bootstrap() bool {
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
		r.mgr.requestCleanup(r.ctx, r.rec.key)
		return false
	}
	if !r.rec.setUnsubscribe(unsubscribe) {
		return false
	}
	chat, err := r.opts.Store.GetChatByID(r.ctx, r.rec.key.ChatID)
	if err != nil {
		r.opts.Logger.Warn(r.ctx, "chatworker runner bootstrap failed", slogError(err))
		r.mgr.requestCleanup(r.ctx, r.rec.key)
		return false
	}
	// Apply the database snapshot directly instead of routing it through
	// the manager. Routing fans out through stateCh, where a stale hint
	// (for example the pre-acquisition chat:update relayed by another
	// runner's subscription) could be processed first while
	// lastSnapshotVersion is still zero. A stale unowned hint would make
	// the runner clean itself up without abandoning the chat, leaving the
	// chat owned by a dead runner until its heartbeat goes stale.
	// Processing the snapshot here seeds lastSnapshotVersion before the
	// run loop drains stateCh, so the dedup in processState drops every
	// hint at or below this version regardless of delivery path.
	return r.processState(stateUpdateFromChat(chat))
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

// processState applies an ordered notification and reports whether to continue.
func (r *runner) processState(state runnerStateUpdate) bool {
	if r.ctx.Err() != nil {
		return false
	}
	if state.SnapshotVersion <= r.lastSnapshotVersion {
		return true
	}

	if !uuidPtrEqual(state.WorkerID, r.rec.workerID) || !uuidPtrEqual(state.RunnerID, r.rec.key.RunnerID) {
		r.acceptState(state)
		r.mgr.requestCleanup(r.ctx, r.rec.key)
		return false
	}

	if r.requiredWorkChanged(state) {
		r.cancelActiveTask()
		r.spawnForState(state)
	}
	r.acceptState(state)
	return true
}

func (r *runner) requiredWorkChanged(state runnerStateUpdate) bool {
	return !r.hasAcceptedState || r.latestState.HistoryVersion != state.HistoryVersion ||
		r.latestState.Status != state.Status || r.latestState.Archived != state.Archived
}

func (r *runner) acceptState(state runnerStateUpdate) {
	r.hasAcceptedState = true
	r.latestState = state
	r.lastSnapshotVersion = state.SnapshotVersion
}

func (r *runner) spawnForState(state runnerStateUpdate) {
	if state.Archived {
		r.spawnTaskIfNeeded(taskKindAbandon, state)
		return
	}
	switch state.Status {
	case database.ChatStatusRunning:
		r.spawnTaskIfNeeded(taskKindGeneration, state)
	case database.ChatStatusInterrupting:
		r.spawnTaskIfNeeded(taskKindInterrupt, state)
	case database.ChatStatusRequiresAction:
		r.spawnTaskIfNeeded(taskKindRequiresActionTimeout, state)
	case database.ChatStatusWaiting, database.ChatStatusError:
		r.spawnTaskIfNeeded(taskKindAbandon, state)
	default:
		r.spawnTaskIfNeeded(taskKindAbandon, state)
	}
}

func (r *runner) spawnTaskIfNeeded(kind taskKind, state runnerStateUpdate) {
	if r.ctx.Err() != nil {
		return
	}
	key := localWorkKey{historyVersion: state.HistoryVersion, status: state.Status}
	if r.currentTask != nil && r.currentTask.kind == kind && r.currentTask.localKey == key {
		return
	}

	id := taskInstanceID(uuid.New())
	taskCtx, cancel := context.WithCancel(r.ctx)
	done := make(chan struct{})
	record := &taskRecord{
		id:       id,
		kind:     kind,
		localKey: key,
		cancel:   cancel,
		done:     done,
	}
	r.tasks[id] = record
	r.currentTask = record

	input := chatWorkerTaskStartInput{
		TaskID:                   uuid.UUID(id),
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
	go r.runTask(taskCtx, kind, key, input, done)
}

func (r *runner) runTask(
	ctx context.Context,
	kind taskKind,
	key localWorkKey,
	input chatWorkerTaskStartInput,
	done chan<- struct{},
) {
	defer close(done)
	taskInfo := retryWrapperTaskInfo{
		ChatID:   input.ChatID,
		WorkerID: input.WorkerID,
		RunnerID: input.RunnerID,
	}
	err := runTaskWithRetry(ctx, r.opts.retryOptions(), kind, taskInfo, func(ctx context.Context) error {
		unlock, ok := r.localLocks.acquire(ctx, key)
		if !ok {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("runTask acquire local lock: %w", ctx.Err()))
		}
		defer unlock()
		if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("runTask context canceled: %w", ctx.Err()))
		}

		switch kind {
		case taskKindGeneration:
			return r.opts.TaskStarter.StartGeneration(ctx, input)
		case taskKindInterrupt:
			return r.opts.TaskStarter.StartInterrupt(ctx, input)
		case taskKindRequiresActionTimeout:
			return r.opts.TaskStarter.StartRequiresActionTimeout(ctx, input)
		case taskKindAbandon:
			return r.opts.TaskStarter.StartAbandon(ctx, input)
		default:
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("unknown task kind %q", kind))
		}
	})
	if err != nil && ctx.Err() == nil {
		r.opts.Logger.Warn(ctx, "chatworker task failed", slogError(err))
	}
}

func (r *runner) cancelActiveTask() {
	if r.currentTask != nil {
		r.currentTask.cancel()
		r.currentTask = nil
	}
}

func (r *runner) waitForTasks() {
	for _, record := range r.tasks {
		<-record.done
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

func (r *runner) removeFinishedTasks() {
	for id, record := range r.tasks {
		if record == r.currentTask {
			continue
		}
		select {
		case <-record.done:
			record.cancel()
			delete(r.tasks, id)
		default:
		}
	}
}

func uuidPtrEqual(got *uuid.UUID, want uuid.UUID) bool {
	return got != nil && *got == want
}

type localLockSet struct {
	mu     sync.Mutex
	locked map[localWorkKey]chan struct{}
}

func newLocalLockSet() *localLockSet {
	return &localLockSet{locked: make(map[localWorkKey]chan struct{})}
}

func (l *localLockSet) acquire(ctx context.Context, key localWorkKey) (func(), bool) {
	for {
		l.mu.Lock()
		wait, ok := l.locked[key]
		if !ok {
			released := make(chan struct{})
			l.locked[key] = released
			l.mu.Unlock()
			return func() {
				l.mu.Lock()
				if l.locked[key] == released {
					delete(l.locked, key)
					close(released)
				}
				l.mu.Unlock()
			}, true
		}
		l.mu.Unlock()

		select {
		case <-wait:
		case <-ctx.Done():
			return nil, false
		}
	}
}
