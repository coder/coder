package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	postCommitWatchPublishTimeout = 10 * time.Second
	// defaultTaskTimeout must exceed chatloop's stream-silence guard so
	// silent provider streams fail through chat-specific retry handling
	// before the runner retries the whole task.
	defaultTaskTimeout = 15 * time.Minute
)

var (
	errTaskExpectedExit = xerrors.New("chatworker task expected exit")
	errTaskRetryable    = xerrors.New("chatworker task retryable error")
	errTaskTimeout      = xerrors.New("chatworker task timeout")
)

type taskRetryableError struct {
	err error
}

func (e taskRetryableError) Error() string {
	if e.err == nil {
		return errTaskRetryable.Error()
	}
	return e.err.Error()
}

func (e taskRetryableError) Unwrap() error {
	if e.err == nil {
		return errTaskRetryable
	}
	return errors.Join(errTaskRetryable, e.err)
}

type retryWrapperOptions struct {
	clock        quartz.Clock
	logger       slog.Logger
	initialDelay time.Duration
	maxDelay     time.Duration
}

type retryWrapperTaskInfo struct {
	ChatID   uuid.UUID
	WorkerID uuid.UUID
	RunnerID uuid.UUID
}

// runTaskWithRetry ensures that a task doesn't exit until it completes
// successfully or gets canceled. It retries the task in case of any ephemeral errors.
// It's critical for the correct operation of the chat runner:
// this function is THE place that ensures task liveness within the runner.
func runTaskWithRetry(
	ctx context.Context,
	opts retryWrapperOptions,
	kind taskKind,
	info retryWrapperTaskInfo,
	fn func(context.Context) error,
) error {
	if opts.clock == nil {
		opts.clock = quartz.NewReal()
	}
	if opts.initialDelay <= 0 {
		opts.initialDelay = defaultTaskRetryInitialBackoff
	}
	if opts.maxDelay <= 0 {
		opts.maxDelay = defaultTaskRetryMaxBackoff
	}
	if opts.maxDelay < opts.initialDelay {
		opts.maxDelay = opts.initialDelay
	}
	delay := opts.initialDelay
	for {
		attemptCtx, cancelAttempt := taskAttemptContext(ctx, opts.clock, kind)
		err := executeTaskSafely(attemptCtx, fn)
		timedOut := errors.Is(context.Cause(attemptCtx), errTaskTimeout)
		cancelAttempt()
		if timedOut && err != nil {
			if !errors.Is(err, errTaskExpectedExit) ||
				errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) ||
				errors.Is(err, errTaskTimeout) {
				err = taskRetryableError{err: errors.Join(errTaskTimeout, err)}
			}
		}
		if err == nil {
			// no log on success to avoid noise
			return nil
		}

		exitReason := ""
		switch {
		case ctx.Err() != nil:
			exitReason = "context_canceled"
		case errors.Is(err, errTaskExpectedExit) && !errors.Is(err, errTaskRetryable):
			exitReason = "expected_non_retryable_exit"
		}
		if exitReason != "" {
			opts.logger.Debug(ctx, "chatworker task exited",
				slog.F("task_kind", kind),
				slog.F("reason", exitReason),
				slog.F("chat_id", info.ChatID),
				slog.F("worker_id", info.WorkerID),
				slog.F("runner_id", info.RunnerID),
				slogError(err),
			)
			return nil
		}

		opts.logger.Warn(ctx, "chatworker task retrying",
			slog.F("task_kind", kind),
			slog.F("delay", delay),
			slog.F("chat_id", info.ChatID),
			slog.F("worker_id", info.WorkerID),
			slog.F("runner_id", info.RunnerID),
			slogError(err),
		)
		timer := opts.clock.NewTimer(delay, "chatworker", "task-retry-"+string(kind))
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil
		}
		timer.Stop()
		if delay < opts.maxDelay {
			delay *= 2
			if delay > opts.maxDelay {
				delay = opts.maxDelay
			}
		}
	}
}

func taskAttemptContext(ctx context.Context, clock quartz.Clock, kind taskKind) (context.Context, func()) {
	attemptCtx, cancelCause := context.WithCancelCause(ctx)
	timer := clock.AfterFunc(defaultTaskTimeout, func() {
		cancelCause(errTaskTimeout)
	}, "chatworker", "task-timeout-"+string(kind))
	return attemptCtx, func() {
		timer.Stop()
		cancelCause(nil)
	}
}

func executeTaskSafely(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = xerrors.Errorf("chatworker task panic: %v", recovered)
		}
	}()
	return fn(ctx)
}

type interruptionOutcome struct {
	Chat           database.Chat
	Kind           runnerActionKind
	WatchEventKind codersdk.ChatWatchEventKind
}

type taskStarter struct {
	server                   *Server
	opts                     chatWorkerOptions
	routeStateHint           func(context.Context, runnerStateUpdate)
	requestCleanup           func(context.Context, runnerKey)
	afterInterruptionOutcome func(context.Context, interruptionOutcome) error
}

func newTaskStarter(
	server *Server,
	opts chatWorkerOptions,
	routeStateHint func(context.Context, runnerStateUpdate),
	requestCleanup func(context.Context, runnerKey),
) (*taskStarter, error) {
	if server == nil {
		return nil, xerrors.New("chatworker: server is required")
	}
	if opts.Store == nil {
		return nil, xerrors.New("chatworker: task store is required")
	}
	if opts.Pubsub == nil {
		return nil, xerrors.New("chatworker: task pubsub is required")
	}
	if opts.MessagePartBuffer == nil {
		return nil, xerrors.New("chatworker: message part buffer is required")
	}
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.TaskRetryInitialBackoff <= 0 {
		opts.TaskRetryInitialBackoff = defaultTaskRetryInitialBackoff
	}
	if opts.TaskRetryMaxBackoff <= 0 {
		opts.TaskRetryMaxBackoff = defaultTaskRetryMaxBackoff
	}
	if opts.TaskRetryMaxBackoff < opts.TaskRetryInitialBackoff {
		opts.TaskRetryMaxBackoff = opts.TaskRetryInitialBackoff
	}
	if routeStateHint == nil {
		return nil, xerrors.New("chatworker: route state hint callback is required")
	}
	if requestCleanup == nil {
		return nil, xerrors.New("chatworker: cleanup callback is required")
	}
	return &taskStarter{
		server:         server,
		opts:           opts,
		routeStateHint: routeStateHint,
		requestCleanup: requestCleanup,
	}, nil
}

func (o chatWorkerOptions) retryOptions() retryWrapperOptions {
	return retryWrapperOptions{
		clock:        o.Clock,
		logger:       o.Logger,
		initialDelay: o.TaskRetryInitialBackoff,
		maxDelay:     o.TaskRetryMaxBackoff,
	}
}

// interruptEpisodeSnapshot carries the interrupt task's one-time episode
// snapshot across retry attempts of the same task instance. It is
// captured before the task's first database read: one stalled read (a
// database outage holds an attempt up to the task timeout) can outlive
// the buffer's closed-episode retention, after which the buffer only
// offers a blank recreated episode that would underbill the interrupted
// work and drop the partial messages. Attempts of one task run
// sequentially, so no locking is needed.
type interruptEpisodeSnapshot struct {
	loaded  bool
	key     messagepartbuffer.Key
	billing messagepartbuffer.EpisodeBilling
	parts   []messagepartbuffer.Part
}

// closeInterruptEpisode closes the interrupted attempt's buffer episode
// and returns its billing snapshot and buffered parts. Closing and
// snapshotting billing state must be one atomic step: the generation
// goroutine records batch starts and tool completions concurrently, so
// a read-then-close would let stamps land in the gap and go missing
// from the snapshot. Unknown episodes close blank, so interruption
// converges even when the worker exited before publishing parts.
func (s *taskStarter) closeInterruptEpisode(ctx context.Context, key messagepartbuffer.Key) (messagepartbuffer.EpisodeBilling, []messagepartbuffer.Part, error) {
	billing, err := s.opts.MessagePartBuffer.CloseEpisodeForBilling(key)
	if err != nil {
		if ctx.Err() != nil {
			return messagepartbuffer.EpisodeBilling{}, nil, errors.Join(errTaskExpectedExit, xerrors.Errorf("close message part episode: %w", err), ctx.Err())
		}
		return messagepartbuffer.EpisodeBilling{}, nil, taskRetryableError{err: xerrors.Errorf("close message part episode: %w", err)}
	}
	parts, err := s.opts.MessagePartBuffer.GetParts(key)
	if errors.Is(err, messagepartbuffer.ErrEpisodeNotFound) {
		parts = nil
		err = nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return messagepartbuffer.EpisodeBilling{}, nil, errors.Join(errTaskExpectedExit, xerrors.Errorf("get message part episode: %w", err), ctx.Err())
		}
		return messagepartbuffer.EpisodeBilling{}, nil, taskRetryableError{err: xerrors.Errorf("get message part episode: %w", err)}
	}
	return billing, parts, nil
}

func (s *taskStarter) StartInterrupt(ctx context.Context, input chatWorkerTaskStartInput) error {
	snapshot := input.InterruptSnapshot
	// Capture the episode before the chat read below, which can stall
	// on a database outage past the buffer's retention and let the
	// cleanup loop evict the episode before a post-read capture. The
	// input carries the interrupted attempt number, and generation
	// attempts only advance while the chat is running, so the pre-read
	// key matches the row while it stays interrupting; if the read
	// below disagrees, the snapshot is retaken with the row's key.
	if snapshot != nil && !snapshot.loaded {
		earlyKey := messagepartbuffer.Key{
			ChatID:            input.ChatID,
			HistoryVersion:    input.HistoryVersion,
			GenerationAttempt: input.GenerationAttempt,
		}
		billing, parts, err := s.closeInterruptEpisode(ctx, earlyKey)
		if err != nil {
			return err
		}
		snapshot.loaded = true
		snapshot.key = earlyKey
		snapshot.billing = billing
		snapshot.parts = parts
	}
	machine := chatstate.NewChatMachine(s.opts.Store, s.opts.Pubsub, input.ChatID)
	var chat database.Chat
	err := machine.ReadLock(ctx, func(store database.Store) error {
		loadedChat, err := loadChatForTask(ctx, store, input, database.ChatStatusInterrupting, taskFenceOptions{requireHistory: true})
		if err != nil {
			return xerrors.Errorf("load chat for task: %w", err)
		}
		chat = loadedChat
		return nil
	})
	if err != nil {
		return normalizeTaskInfrastructureError(err, "lock chat for interrupt")
	}

	key := messagepartbuffer.Key{
		ChatID:            input.ChatID,
		HistoryVersion:    input.HistoryVersion,
		GenerationAttempt: chat.GenerationAttempt,
	}
	var episodeBilling messagepartbuffer.EpisodeBilling
	var parts []messagepartbuffer.Part
	if snapshot != nil && snapshot.loaded && snapshot.key == key {
		// This task already snapshotted the episode, before its first
		// chat read or on a previous attempt. Reuse it: the episode
		// may have been evicted from the buffer while an attempt
		// stalled, and re-reading would find a blank recreated
		// episode.
		episodeBilling = snapshot.billing
		parts = snapshot.parts
	} else {
		// No usable snapshot: either the caller passed none (tests) or
		// the row's attempt number disagrees with the pre-read key, so
		// the pre-read snapshot describes the wrong episode.
		episodeBilling, parts, err = s.closeInterruptEpisode(ctx, key)
		if err != nil {
			return err
		}
		if snapshot != nil {
			snapshot.loaded = true
			snapshot.key = key
			snapshot.billing = episodeBilling
			snapshot.parts = parts
		}
	}
	// The interrupt instant is the episode's first close, which is
	// stable across repeat closes: a retried interrupt task (transient
	// database errors) recomputes identical partial messages and
	// billing windows instead of billing still-running tools through
	// each retry's later clock reading.
	interruptedAt := episodeBilling.ClosedAt
	var attemptRuntime time.Duration
	if !episodeBilling.ModelInvokedAt.IsZero() {
		attemptRuntime = interruptedAt.Sub(episodeBilling.ModelInvokedAt)
	}
	partialMessages, err := bufferedPartsToPartialMessages(bufferedPartsToPartialMessagesInput{
		parts:          parts,
		modelConfigID:  chat.LastModelConfigID,
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         s.opts.Logger,
		interruptedAt:  interruptedAt,
		attemptRuntime: attemptRuntime,
	})
	if err != nil {
		return xerrors.Errorf("convert buffered parts: %w", err)
	}

	var committed database.Chat
	err = machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		chat, err := loadChatForTask(ctx, store, input, database.ChatStatusInterrupting, taskFenceOptions{requireHistory: true})
		if err != nil {
			return xerrors.Errorf("load chat for task: %w", err)
		}
		messages := partialMessages
		// Reuse the interrupt instant captured when the episode closed:
		// a fresh clock read here would run while this transaction
		// waits on the database (and again on retries), inflating the
		// billed window of a still-running call past the actual
		// interrupt.
		committedCancels, err := committedPendingLocalToolCancellationMessages(ctx, store, chat, interruptedAt, interruptedToolBatchBilling{
			batchStartedAt:  episodeBilling.ToolBatchStartedAt,
			toolCompletions: episodeBilling.ToolCompletions,
		})
		if err != nil {
			return xerrors.Errorf("committed pending local tool cancellation messages: %w", err)
		}
		if len(committedCancels) > 0 {
			messages = append(append([]chatstate.Message{}, partialMessages...), committedCancels...)
		}
		if _, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{PartialMessages: messages}); err != nil {
			return xerrors.Errorf("finish interruption: %w", err)
		}
		committed, err = store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		return nil
	})
	if err != nil {
		if current, ok := s.committedStateAfterUpdateError(ctx, committed); ok {
			return s.publishWatchAndRoute(ctx, current, codersdk.ChatWatchEventKindStatusChange)
		}
		return normalizeTaskTransitionError(err, "finish interruption")
	}
	input.DebugTurn.RecordOutcome(chatdebug.StatusInterrupted)
	if err := s.publishWatchAndRoute(ctx, committed, codersdk.ChatWatchEventKindStatusChange); err != nil {
		return xerrors.Errorf("publish watch and route: %w", err)
	}
	return s.runAfterInterruptionOutcome(ctx, interruptionOutcome{
		Chat:           committed,
		Kind:           runnerActionKindFinishInterruption,
		WatchEventKind: codersdk.ChatWatchEventKindStatusChange,
	})
}

func (s *taskStarter) runAfterInterruptionOutcome(ctx context.Context, outcome interruptionOutcome) error {
	afterOutcome := s.afterInterruptionOutcome
	if afterOutcome == nil {
		afterOutcome = s.server.afterInterruptionOutcome
	}
	if afterOutcome == nil {
		return nil
	}
	if err := afterOutcome(ctx, outcome); err != nil {
		return taskRetryableError{err: xerrors.Errorf("interruption post-outcome side effects: %w", err)}
	}
	return nil
}

func (s *taskStarter) StartRequiresActionTimeout(ctx context.Context, input chatWorkerTaskStartInput) error {
	machine := chatstate.NewChatMachine(s.opts.Store, s.opts.Pubsub, input.ChatID)
	for {
		decision, err := decideRequiresActionTimeout(ctx, machine, input)
		if err != nil {
			return xerrors.Errorf("decide requires action timeout: %w", err)
		}
		if decision.cancel {
			return s.cancelRequiresAction(ctx, machine, input, decision.reason)
		}
		if !decision.waitUntil.Valid {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("requires action deadline is missing"))
		}
		if err := s.waitUntil(ctx, decision.waitUntil.Time); err != nil {
			return xerrors.Errorf("wait until: %w", err)
		}
	}
}

type requiresActionTimeoutDecision struct {
	cancel    bool
	reason    string
	waitUntil sql.NullTime
}

func decideRequiresActionTimeout(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
) (requiresActionTimeoutDecision, error) {
	var decision requiresActionTimeoutDecision
	err := machine.ReadLock(ctx, func(store database.Store) error {
		chat, err := loadChatForTask(ctx, store, input, database.ChatStatusRequiresAction, taskFenceOptions{requireHistory: true})
		if err != nil {
			return xerrors.Errorf("load chat for task: %w", err)
		}
		if !chat.RequiresActionDeadlineAt.Valid {
			decision.cancel = true
			decision.reason = "Tool execution canceled because the action deadline was missing"
			return nil
		}
		now, err := store.GetDatabaseNow(ctx)
		if err != nil {
			return xerrors.Errorf("get database time: %w", err)
		}
		if now.Before(chat.RequiresActionDeadlineAt.Time) {
			decision.waitUntil = chat.RequiresActionDeadlineAt
			return nil
		}
		decision.cancel = true
		decision.reason = "Tool execution timed out"
		return nil
	})
	if err != nil {
		return requiresActionTimeoutDecision{}, normalizeTaskInfrastructureError(err, "lock chat for requires action timeout")
	}
	return decision, nil
}

func (s *taskStarter) waitUntil(ctx context.Context, deadline time.Time) error {
	now := s.opts.Clock.Now("chatworker", "requires-action-timeout")
	if !now.Before(deadline) {
		return nil
	}
	timer := s.opts.Clock.NewTimer(deadline.Sub(now), "chatworker", "requires-action-timeout")
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("wait until: %w", ctx.Err()))
	}
}

func (s *taskStarter) cancelRequiresAction(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	reason string,
) error {
	var committed database.Chat
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		chat, err := loadChatForTask(ctx, store, input, database.ChatStatusRequiresAction, taskFenceOptions{requireHistory: true})
		if err != nil {
			return xerrors.Errorf("load chat for task: %w", err)
		}
		if chat.RequiresActionDeadlineAt.Valid {
			now, err := store.GetDatabaseNow(ctx)
			if err != nil {
				return xerrors.Errorf("get database time: %w", err)
			}
			if now.Before(chat.RequiresActionDeadlineAt.Time) {
				return errors.Join(errTaskExpectedExit, xerrors.Errorf("requires action deadline is in the future"))
			}
		}
		if _, err := tx.CancelRequiresAction(chatstate.CancelRequiresActionInput{Reason: reason}); err != nil {
			return xerrors.Errorf("cancel requires action: %w", err)
		}
		committed, err = store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		return nil
	})
	if err != nil {
		if current, ok := s.committedStateAfterUpdateError(ctx, committed); ok {
			return s.publishWatchAndRoute(ctx, current, codersdk.ChatWatchEventKindStatusChange)
		}
		return normalizeTaskTransitionError(err, "cancel requires action")
	}
	return s.publishWatchAndRoute(ctx, committed, codersdk.ChatWatchEventKindStatusChange)
}

func (s *taskStarter) StartAbandon(ctx context.Context, input chatWorkerTaskStartInput) error {
	machine := chatstate.NewChatMachine(s.opts.Store, s.opts.Pubsub, input.ChatID)
	mismatch := false
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		chat, err := store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				mismatch = true
				return errors.Join(errTaskExpectedExit, xerrors.Errorf("load chat: %w", err))
			}
			return xerrors.Errorf("load chat: %w", err)
		}
		if !ownedByTask(chat, input) {
			mismatch = true
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("chat not owned by task"))
		}
		if err := verifyTaskFence(chat, input, input.Status, taskFenceOptions{requireHistory: true, allowArchived: true}); err != nil {
			return xerrors.Errorf("task fence mismatch: %w", err)
		}
		if _, err := tx.Abandon(chatstate.AbandonInput{}); err != nil {
			return xerrors.Errorf("abandon chat: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errTaskExpectedExit) && mismatch {
			s.requestCleanup(ctx, runnerKey{ChatID: input.ChatID, RunnerID: input.RunnerID})
			return nil
		}
		return normalizeTaskTransitionError(err, "abandon chat")
	}
	s.requestCleanup(ctx, runnerKey{ChatID: input.ChatID, RunnerID: input.RunnerID})
	return nil
}

func (s *taskStarter) committedStateAfterUpdateError(ctx context.Context, committed database.Chat) (database.Chat, bool) {
	if committed.ID == uuid.Nil {
		return database.Chat{}, false
	}
	current, err := s.opts.Store.GetChatByID(ctx, committed.ID)
	if err != nil {
		return database.Chat{}, false
	}
	if current.SnapshotVersion != committed.SnapshotVersion ||
		current.HistoryVersion != committed.HistoryVersion ||
		current.QueueVersion != committed.QueueVersion ||
		current.GenerationAttempt != committed.GenerationAttempt ||
		current.Status != committed.Status ||
		current.Archived != committed.Archived ||
		current.WorkerID != committed.WorkerID ||
		current.RunnerID != committed.RunnerID {
		return database.Chat{}, false
	}
	return current, true
}

func (s *taskStarter) publishWatchAndRoute(
	ctx context.Context,
	chat database.Chat,
	kind codersdk.ChatWatchEventKind,
) error {
	watchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), postCommitWatchPublishTimeout)
	defer cancel()
	if err := s.publishWatchWithRetry(watchCtx, chat, kind); err != nil {
		return xerrors.Errorf("publish watch with retry: %w", err)
	}
	s.routeStateHint(ctx, stateUpdateFromChat(chat))
	return nil
}

func (s *taskStarter) publishWatchWithRetry(
	ctx context.Context,
	chat database.Chat,
	kind codersdk.ChatWatchEventKind,
) error {
	delay := s.opts.TaskRetryInitialBackoff
	for {
		if err := publishChatWatchEvent(s.opts.Pubsub, chat, kind); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("publishChatWatchEvent: %w", ctx.Err()))
		}
		timer := s.opts.Clock.NewTimer(delay, "chatworker", "watch-publish-retry")
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("watch publish retry context done: %w", ctx.Err()))
		}
		timer.Stop()
		if delay < s.opts.TaskRetryMaxBackoff {
			delay *= 2
			if delay > s.opts.TaskRetryMaxBackoff {
				delay = s.opts.TaskRetryMaxBackoff
			}
		}
	}
}

func publishChatWatchEvent(pubsub chatWorkerPubsub, chat database.Chat, kind codersdk.ChatWatchEventKind) error {
	event := codersdk.ChatWatchEvent{
		Kind: kind,
		Chat: chatWatchEventSDKChat(chat, nil),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return xerrors.Errorf("marshal chat watch event: %w", err)
	}
	if err := pubsub.Publish(coderdpubsub.ChatWatchEventChannel(chat.OwnerID), payload); err != nil {
		return xerrors.Errorf("publish chat watch event: %w", err)
	}
	return nil
}

type taskFenceOptions struct {
	requireHistory bool
	allowArchived  bool
}

// loadChatForTask loads the chat row and verifies the task fence in one
// step so call sites cannot skip the fence check. It returns an error
// wrapping errTaskExpectedExit when the chat no longer exists or the fence
// no longer matches.
func loadChatForTask(
	ctx context.Context,
	store database.Store,
	input chatWorkerTaskStartInput,
	status database.ChatStatus,
	opts taskFenceOptions,
) (database.Chat, error) {
	chat, err := store.GetChatByID(ctx, input.ChatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return database.Chat{}, errors.Join(errTaskExpectedExit, xerrors.Errorf("load chat: %w", err))
		}
		return database.Chat{}, xerrors.Errorf("load chat: %w", err)
	}
	if err := verifyTaskFence(chat, input, status, opts); err != nil {
		return database.Chat{}, xerrors.Errorf("verifyTaskFence: %w", err)
	}
	return chat, nil
}

func verifyTaskFence(
	chat database.Chat,
	input chatWorkerTaskStartInput,
	status database.ChatStatus,
	opts taskFenceOptions,
) error {
	if !ownedByTask(chat, input) {
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("chat not owned by task"))
	}
	if chat.Status != status {
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("chat status mismatch: %s != %s", chat.Status, status))
	}
	if !opts.allowArchived && chat.Archived {
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("chat archived"))
	}
	if opts.requireHistory && chat.HistoryVersion != input.HistoryVersion {
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("chat history version mismatch: %d != %d", chat.HistoryVersion, input.HistoryVersion))
	}
	return nil
}

func ownedByTask(chat database.Chat, input chatWorkerTaskStartInput) bool {
	return chat.WorkerID.Valid && chat.WorkerID.UUID == input.WorkerID &&
		chat.RunnerID.Valid && chat.RunnerID.UUID == input.RunnerID
}

func normalizeTaskInfrastructureError(err error, action string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errTaskExpectedExit) {
		return err
	}
	if errors.Is(err, chatstate.ErrChatNotFound) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, context.Canceled) {
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("%s: %w", action, err))
	}
	return taskRetryableError{err: xerrors.Errorf("%s: %w", action, err)}
}

func normalizeTaskTransitionError(err error, action string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errTaskExpectedExit) {
		return err
	}
	if errors.Is(err, chatstate.ErrChatNotFound) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, context.Canceled) {
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("%s: %w", action, err))
	}
	if errors.Is(err, chatstate.ErrTransitionNotAllowed) || errors.Is(err, chatstate.ErrInvalidState) {
		return xerrors.Errorf("%s: %w", action, err)
	}
	return taskRetryableError{err: xerrors.Errorf("%s: %w", action, err)}
}

func dynamicToolNamesFromChat(chat database.Chat) map[string]bool {
	if !chat.DynamicTools.Valid || len(chat.DynamicTools.RawMessage) == 0 {
		return nil
	}
	var tools []codersdk.DynamicTool
	if err := json.Unmarshal(chat.DynamicTools.RawMessage, &tools); err != nil {
		return nil
	}
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name != "" {
			names[name] = true
		}
	}
	return names
}

// interruptedToolBatchBilling carries what the interrupt task knows about
// the live tool batch it is canceling, so the synthesized cancellation
// rows can bill the partial window the batch would have reported.
type interruptedToolBatchBilling struct {
	// batchStartedAt is the StartToolBatch stamp from the interrupted
	// attempt's buffer episode. Zero when no batch was live at the
	// interrupt (crash recovery, state promotion), in which case the
	// cancellation rows carry no runtime.
	batchStartedAt time.Time
	// toolCompletions holds the live batch's dispatched tool call
	// occurrences, each keyed by its position in the step's unresolved
	// call order: seeded when the batch started, marked as each call
	// began executing, and stamped as each tool finished. A completed
	// occurrence bills the interval from its start to its completion,
	// so a batch whose billed tools all finished early does not bill
	// the longer window of a still-running unbilled tool such as
	// wait_agent. A started but uncompleted occurrence was still
	// running when the interrupt landed. A seeded occurrence that
	// never started is a serial call that was still waiting behind
	// its siblings, and bills nothing. A call with no occurrence at
	// its position was never dispatched, such as a call rejected as
	// malformed or ambiguous before execution, and bills nothing even
	// though it too receives a cancellation row.
	toolCompletions []messagepartbuffer.ToolCompletion
}

func committedPendingLocalToolCancellationMessages(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	interruptedAt time.Time,
	billing interruptedToolBatchBilling,
) ([]chatstate.Message, error) {
	messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	if err != nil {
		return nil, xerrors.Errorf("load committed messages for interruption: %w", err)
	}
	localCalls, _, err := unresolvedToolCallsFromHistory(messages, dynamicToolNamesFromChat(chat))
	if err != nil {
		return nil, err
	}
	if len(localCalls) == 0 {
		return nil, nil
	}
	// Dispatched occurrences keyed by their position in the unresolved
	// call order, the same order this loop walks. Positional matching
	// keeps a call rejected before execution from consuming a same-ID
	// dispatched occurrence and keeps duplicate tool call IDs from
	// sharing one completion state.
	dispatched := make(map[int]messagepartbuffer.ToolCompletion, len(billing.toolCompletions))
	for _, completion := range billing.toolCompletions {
		if completion.CallIndex < 0 {
			continue
		}
		dispatched[completion.CallIndex] = completion
	}
	var (
		windowEnd    time.Time
		windowRowIdx = -1
		intervals    []chatloop.BilledInterval
	)
	result := make([]chatstate.Message, 0, len(localCalls))
	for i, call := range localCalls {
		payload, err := json.Marshal(map[string]string{"error": interruptedToolResultErrorMessage})
		if err != nil {
			return nil, xerrors.Errorf("marshal interrupted tool result: %w", err)
		}
		part := codersdk.ChatMessageToolResult(call.ToolCallID, call.ToolName, payload, true, false)
		if !interruptedAt.IsZero() {
			part.CreatedAt = &interruptedAt
		}
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{part})
		if err != nil {
			return nil, xerrors.Errorf("marshal interrupted tool result part: %w", err)
		}
		result = append(result, chatstate.Message{
			Role:           database.ChatMessageRoleTool,
			Content:        content,
			Visibility:     database.ChatMessageVisibilityBoth,
			ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: chat.LastModelConfigID != uuid.Nil},
			ContentVersion: chatprompt.CurrentContentVersion,
		})
		if billing.batchStartedAt.IsZero() || unbilledSubagentToolNames[call.ToolName] {
			continue
		}
		// Only dispatched calls bill: a call with no occurrence at its
		// position was rejected before execution, and an ID mismatch
		// means the seed does not describe this call. A completed
		// occurrence bills its execution interval; a started but
		// uncompleted one was still running, so its window ends at the
		// interrupt. A dispatched occurrence that never started is a
		// serial call still waiting behind its siblings when the
		// interrupt landed, and bills nothing. Strictly-after keeps
		// the earliest call on ties, matching billableBatchWindow.
		occurrence, ok := dispatched[i]
		if !ok || occurrence.ToolCallID != call.ToolCallID {
			continue
		}
		start := occurrence.StartedAt
		end := occurrence.CompletedAt
		if end.IsZero() {
			if start.IsZero() {
				continue
			}
			end = interruptedAt
		}
		if start.IsZero() {
			// Live execution always marks a start before a
			// completion; fall back to the batch start rather than
			// dropping a completed call's billed work.
			start = billing.batchStartedAt
		}
		intervals = append(intervals, chatloop.BilledInterval{Start: start, End: end})
		if end.After(windowEnd) {
			windowEnd = end
			windowRowIdx = len(result) - 1
		}
	}
	// Mirror the committed-batch policy: bill the union of the billed
	// calls' execution intervals once, on the cancellation row of the
	// billed tool call whose window ends last. The union never charges
	// a span where only unbilled tools were running, such as a
	// sub-agent wait that delayed a serial call's launch.
	if windowRowIdx >= 0 {
		result[windowRowIdx].RuntimeMs = nullInt64IfNonZero(chatloop.BilledIntervalsDuration(intervals).Milliseconds())
	}
	return result, nil
}
