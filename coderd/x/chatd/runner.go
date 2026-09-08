package chatd

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

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

type taskIndexKey struct {
	kind taskKind
	key  localWorkKey
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

	activeTaskID  taskInstanceID
	activeTaskSet bool
	tasks         map[taskInstanceID]*taskRecord
	tasksByIndex  map[taskIndexKey]taskInstanceID
	localLocks    *localLockSet
	debugTurn     *runnerDebugTurn
	sessionStart  sessionStartTracker
	stopNudges    stopNudgeTracker
}

func newRunner(ctx context.Context, mgr *runnerManager, rec *runnerRecord, opts chatWorkerOptions) *runner {
	return &runner{
		ctx:          ctx,
		mgr:          mgr,
		rec:          rec,
		opts:         opts,
		tasks:        make(map[taskInstanceID]*taskRecord),
		tasksByIndex: make(map[taskIndexKey]taskInstanceID),
		localLocks:   newLocalLockSet(),
		debugTurn:    newRunnerDebugTurn(ctx, opts.Logger),
	}
}

func (r *runner) run() {
	if !r.bootstrap() {
		return
	}
	for {
		select {
		case state := <-r.rec.stateCh:
			r.processState(state)
		case <-r.ctx.Done():
			r.cancelActiveTask()
			r.waitForTasks()
			r.closeDebugTurn()
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
	r.processState(stateUpdateFromChat(chat))
	return true
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

// processState decides which task should be running for the chat and starts
// it if it is not. It runs for every state the runner receives: pubsub hints,
// the periodic database sync, and the bootstrap read.
//
// It must handle a state it has already seen. A task can stop on its own,
// for example when the chat history changes under it, and nothing tells the
// runner. The periodic sync then delivers the same row again, and that is
// how the runner notices the task is gone and starts it again.
func (r *runner) processState(state runnerStateUpdate) {
	r.removeFinishedTasks()

	if r.isNewer(state) {
		if !r.owns(state) {
			r.acceptState(state)
			r.mgr.requestCleanup(r.ctx, r.rec.key)
			return
		}
		if r.requiredWorkChanged(state) {
			r.cancelActiveTask()
		}
		r.acceptState(state)
	}

	// Once another runner owns the chat, this runner is only waiting to be
	// canceled. Events that arrive in the meantime must not start work.
	if !r.activeTaskSet && r.owns(r.latestState) {
		r.spawnForState(r.latestState)
	}
}

func (r *runner) owns(state runnerStateUpdate) bool {
	return uuidPtrEqual(state.WorkerID, r.rec.workerID) && uuidPtrEqual(state.RunnerID, r.rec.key.RunnerID)
}

// isNewer reports whether the runner has not seen this state before.
//
// Comparing snapshot versions alone is not enough. Every write through the
// state machine increments snapshot_version, but editing a chat_messages row
// directly fires a trigger that increments history_version only. Two states
// with the same snapshot version can therefore require different work, and
// the second one is new.
func (r *runner) isNewer(state runnerStateUpdate) bool {
	if state.SnapshotVersion != r.lastSnapshotVersion {
		return state.SnapshotVersion > r.lastSnapshotVersion
	}
	return r.requiredWorkChanged(state)
}

func (r *runner) requiredWorkChanged(state runnerStateUpdate) bool {
	return !r.hasAcceptedState ||
		r.latestState.HistoryVersion != state.HistoryVersion ||
		r.latestState.Status != state.Status ||
		r.latestState.Archived != state.Archived
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
	key := localWorkKey{historyVersion: state.HistoryVersion, status: state.Status}
	idx := taskIndexKey{kind: kind, key: key}
	if r.activeTaskSet && r.tasksByIndex[idx] == r.activeTaskID {
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
	r.tasksByIndex[idx] = id
	r.activeTaskID = id
	r.activeTaskSet = true

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
	if !r.activeTaskSet {
		return
	}
	id := r.activeTaskID
	r.activeTaskSet = false
	if record := r.tasks[id]; record != nil {
		record.cancel()
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
		select {
		case <-record.done:
			delete(r.tasks, id)
			idx := taskIndexKey{kind: record.kind, key: record.localKey}
			if r.tasksByIndex[idx] == id {
				delete(r.tasksByIndex, idx)
			}
			if r.activeTaskSet && r.activeTaskID == id {
				r.activeTaskSet = false
			}
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
