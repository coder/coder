package chatd

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

// taskHandoff keeps the producer responsible until the runner accepts the
// state or cancels it. A nil chat means the row no longer exists.
type taskHandoff struct {
	chat     *database.Chat
	accepted chan bool
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
	cancel  context.CancelFunc
	done    <-chan struct{}
	handoff chan taskHandoff
}

type runner struct {
	ctx  context.Context
	mgr  *runnerManager
	rec  *runnerRecord
	opts chatWorkerOptions

	lastSnapshotVersion int64
	hasAcceptedState    bool
	latestState         runnerStateUpdate

	// The current task only exits after replacement or runner shutdown
	// cancels it. Superseded tasks remain tracked until they drain.
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
	defer func() {
		r.cancelActiveTask()
		r.waitForTasks()
		r.closeDebugTurn()
	}()
	if !r.bootstrap() {
		return
	}

	for {
		if r.ctx.Err() != nil {
			return
		}
		r.removeFinishedTasks()
		select {
		case state := <-r.rec.stateCh:
			if !r.processState(state) {
				return
			}
		case handoff := <-r.currentTask.handoff:
			if handoff.chat == nil {
				r.mgr.requestCleanup(r.ctx, r.rec.key)
				return
			}
			state := stateUpdateFromChat(*handoff.chat)
			if state.SnapshotVersion < r.lastSnapshotVersion || state.HistoryVersion < r.latestState.HistoryVersion {
				handoff.accepted <- false
				continue
			}
			// A direct read may change work at an equal snapshot. Only
			// hints use the strict duplicate guard in processState.
			if !r.applyState(state) {
				return
			}
			handoff.accepted <- true
		case <-r.ctx.Done():
			return
		}
	}
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
	return r.applyState(stateUpdateFromChat(chat))
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

	return r.applyState(state)
}

// applyState assigns responsibility for a state that has passed ordering checks.
func (r *runner) applyState(state runnerStateUpdate) bool {
	if r.ctx.Err() != nil {
		return false
	}
	if !uuidPtrEqual(state.WorkerID, r.rec.workerID) || !uuidPtrEqual(state.RunnerID, r.rec.key.RunnerID) {
		r.acceptState(state)
		r.mgr.requestCleanup(r.ctx, r.rec.key)
		return false
	}

	if r.requiredWorkChanged(state) {
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
		r.replaceTask(taskKindAbandon, state)
		return
	}
	switch state.Status {
	case database.ChatStatusRunning:
		r.replaceTask(taskKindGeneration, state)
	case database.ChatStatusInterrupting:
		r.replaceTask(taskKindInterrupt, state)
	case database.ChatStatusRequiresAction:
		r.replaceTask(taskKindRequiresActionTimeout, state)
	case database.ChatStatusWaiting, database.ChatStatusError:
		r.replaceTask(taskKindAbandon, state)
	default:
		r.replaceTask(taskKindAbandon, state)
	}
}

func (r *runner) replaceTask(kind taskKind, state runnerStateUpdate) {
	if r.ctx.Err() != nil {
		return
	}
	key := localWorkKey{historyVersion: state.HistoryVersion, status: state.Status}
	id := taskInstanceID(uuid.New())
	taskCtx, cancel := context.WithCancel(r.ctx)
	done := make(chan struct{})
	record := &taskRecord{
		cancel:  cancel,
		done:    done,
		handoff: make(chan taskHandoff),
	}
	r.tasks[id] = record
	previous := r.currentTask
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
	go r.runTask(taskCtx, kind, key, input, done, record.handoff)
	if previous != nil {
		previous.cancel()
	}
}

func (r *runner) runTask(
	ctx context.Context,
	kind taskKind,
	key localWorkKey,
	input chatWorkerTaskStartInput,
	done chan<- struct{},
	handoffs chan<- taskHandoff,
) {
	defer close(done)
	taskInfo := retryWrapperTaskInfo{
		ChatID:   input.ChatID,
		WorkerID: input.WorkerID,
		RunnerID: input.RunnerID,
	}
	runTaskWithRetry(ctx, r.opts.retryOptions(), kind, taskInfo, func(ctx context.Context) error {
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
	}, func(ctx context.Context) error {
		chat, err := r.opts.Store.GetChatByID(ctx, input.ChatID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("read chat for task handoff: %w", err)
		}
		handoff := taskHandoff{accepted: make(chan bool, 1)}
		if err == nil {
			handoff.chat = &chat
		}
		select {
		case handoffs <- handoff:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case accepted := <-handoff.accepted:
			if !accepted {
				return xerrors.New("task handoff is older than accepted chat state; reread chat")
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
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
