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

// TaskKind identifies the type of background task a runner is executing.
type TaskKind string

const (
	TaskKindGeneration            TaskKind = "generation"
	TaskKindInterrupt             TaskKind = "interrupt"
	TaskKindRequiresActionTimeout TaskKind = "requires_action_timeout"
	TaskKindAbandon               TaskKind = "abandon"
)

type taskInstanceID uuid.UUID

type localWorkKey struct {
	historyVersion int64
	status         database.ChatStatus
}

type taskIndexKey struct {
	kind TaskKind
	key  localWorkKey
}

type taskRecord struct {
	id       taskInstanceID
	kind     TaskKind
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
	// Pubsub can deliver chat:update messages that were already queued by
	// Postgres before this runner subscribed. Hold those hints until the
	// runner initializes its local state with the current database snapshot.
	// Otherwise a stale chat:update that shows the chat has no owner could
	// cause the runner to exit before it starts work.
	bootstrapReady := make(chan struct{})
	bootstrapReadyClosed := false
	defer func() {
		if !bootstrapReadyClosed {
			close(bootstrapReady)
		}
	}()

	channel := coderdpubsub.ChatStateUpdateChannel(r.rec.key.ChatID)
	unsubscribe, err := r.opts.Pubsub.SubscribeWithErr(channel, coderdpubsub.HandleChatStateUpdate(
		func(ctx context.Context, payload coderdpubsub.ChatStateUpdateMessage, err error) {
			if err != nil {
				r.opts.Logger.Warn(ctx, "chatworker state update decode failed", slogError(err))
				return
			}
			<-bootstrapReady
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
	r.mgr.RouteStateHint(r.ctx, stateUpdateFromChat(chat))
	close(bootstrapReady)
	bootstrapReadyClosed = true
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

func (r *runner) processState(state runnerStateUpdate) {
	if state.SnapshotVersion <= r.lastSnapshotVersion {
		return
	}

	r.removeFinishedTasks()

	if !uuidPtrEqual(state.WorkerID, r.rec.workerID) || !uuidPtrEqual(state.RunnerID, r.rec.key.RunnerID) {
		r.acceptState(state)
		r.mgr.requestCleanup(r.ctx, r.rec.key)
		return
	}

	changed := !r.hasAcceptedState ||
		r.latestState.HistoryVersion != state.HistoryVersion ||
		r.latestState.Status != state.Status ||
		r.latestState.Archived != state.Archived
	if !changed {
		r.acceptState(state)
		return
	}
	if r.hasAcceptedState && r.activeTaskSet {
		r.cancelActiveTask()
	}

	r.spawnForState(state)
	r.acceptState(state)
}

func (r *runner) acceptState(state runnerStateUpdate) {
	r.hasAcceptedState = true
	r.latestState = state
	r.lastSnapshotVersion = state.SnapshotVersion
}

func (r *runner) spawnForState(state runnerStateUpdate) {
	if state.Archived {
		r.spawnTaskIfNeeded(TaskKindAbandon, state)
		return
	}
	switch state.Status {
	case database.ChatStatusRunning:
		r.spawnTaskIfNeeded(TaskKindGeneration, state)
	case database.ChatStatusInterrupting:
		r.spawnTaskIfNeeded(TaskKindInterrupt, state)
	case database.ChatStatusRequiresAction:
		r.spawnTaskIfNeeded(TaskKindRequiresActionTimeout, state)
	case database.ChatStatusWaiting, database.ChatStatusError:
		r.spawnTaskIfNeeded(TaskKindAbandon, state)
	default:
		r.spawnTaskIfNeeded(TaskKindAbandon, state)
	}
}

func (r *runner) spawnTaskIfNeeded(kind TaskKind, state runnerStateUpdate) {
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
	r.rec.setActiveTaskKind(&kind)

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
	}
	go r.runTask(taskCtx, kind, key, input, done)
}

func (r *runner) runTask(
	ctx context.Context,
	kind TaskKind,
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
		case TaskKindGeneration:
			return r.opts.TaskStarter.StartGeneration(ctx, input)
		case TaskKindInterrupt:
			return r.opts.TaskStarter.StartInterrupt(ctx, input)
		case TaskKindRequiresActionTimeout:
			return r.opts.TaskStarter.StartRequiresActionTimeout(ctx, input)
		case TaskKindAbandon:
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
	r.rec.setActiveTaskKind(nil)

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
				r.rec.setActiveTaskKind(nil)
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
