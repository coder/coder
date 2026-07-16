package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
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
	server *Server
	opts   chatWorkerOptions
	// shutdownCtx is the worker's root context, canceled at the
	// start of chatWorker.Close before the runner drain waits on
	// in-flight tasks. Deferred best-effort work that must ignore
	// normal task cancellation aborts on this context instead, so
	// graceful shutdown is not blocked by it.
	shutdownCtx              context.Context
	routeStateHint           func(context.Context, runnerStateUpdate)
	requestCleanup           func(context.Context, runnerKey)
	afterInterruptionOutcome func(context.Context, interruptionOutcome) error
}

func newTaskStarter(
	shutdownCtx context.Context,
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
		shutdownCtx:    shutdownCtx,
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

func (s *taskStarter) StartInterrupt(ctx context.Context, input chatWorkerTaskStartInput) error {
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
	if err := s.opts.MessagePartBuffer.CloseEpisode(key); err != nil {
		if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("close message part episode: %w", err), ctx.Err())
		}
		return taskRetryableError{err: xerrors.Errorf("close message part episode: %w", err)}
	}
	parts, err := s.opts.MessagePartBuffer.GetParts(key)
	if errors.Is(err, messagepartbuffer.ErrEpisodeNotFound) {
		parts = nil
		err = nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("get message part episode: %w", err), ctx.Err())
		}
		return taskRetryableError{err: xerrors.Errorf("get message part episode: %w", err)}
	}
	partialMessages, err := bufferedPartsToPartialMessages(bufferedPartsToPartialMessagesInput{
		parts:          parts,
		modelConfigID:  chat.LastModelConfigID,
		contentVersion: chatprompt.CurrentContentVersion,
		logger:         s.opts.Logger,
		interruptedAt:  s.opts.Clock.Now("chatworker", "interrupt"),
	})
	if err != nil {
		return xerrors.Errorf("convert buffered parts: %w", err)
	}

	var committed database.Chat
	var interruptedExecutions []database.ChatToolCallExecution
	err = machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		chat, err := loadChatForTask(ctx, store, input, database.ChatStatusInterrupting, taskFenceOptions{requireHistory: true})
		if err != nil {
			return xerrors.Errorf("load chat for task: %w", err)
		}
		messages := partialMessages
		interruptedExecutions = nil
		interruptedAt := s.opts.Clock.Now("chatworker", "interrupt")
		// The ledger's interrupt mapping runs inside the builder so
		// each synthetic result derives from its mapped row; the
		// synthetic results and the mapping commit atomically in
		// this transaction.
		committedCancels, canceledIDs, assistantMessageID, mapped, err := committedPendingLocalToolCancellationMessages(ctx, store, chat, interruptedAt)
		if err != nil {
			return xerrors.Errorf("committed pending local tool cancellation messages: %w", err)
		}
		interruptedExecutions = mapped
		if len(committedCancels) > 0 {
			messages = append(append([]chatstate.Message{}, partialMessages...), committedCancels...)
		}
		if _, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{PartialMessages: messages}); err != nil {
			return xerrors.Errorf("finish interruption: %w", err)
		}
		if len(canceledIDs) > 0 {
			err = store.MarkChatToolCallExecutionsResultCommitted(ctx, database.MarkChatToolCallExecutionsResultCommittedParams{
				ChatID:             input.ChatID,
				AssistantMessageID: assistantMessageID,
				ToolCallIds:        canceledIDs,
				ResultCommittedAt:  interruptedAt,
			})
			if err != nil {
				return xerrors.Errorf("mark tool call executions result committed: %w", err)
			}
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
	// Deferred so clients and queued follow-up work observe the
	// committed interruption before this best-effort pass, which
	// can wait on identity grace windows and slow agent dials.
	defer s.reconcileInterruptedExecutions(ctx, interruptedExecutions)
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

// committedPendingLocalToolCancellationMessages builds the synthetic
// cancellation results for the unresolved local tool calls at
// interrupt and maps their ledger rows in the same transaction. The
// ledger mapping runs first so each synthetic payload derives from
// the mapping's RETURNING row: a background execute the mapping
// spares as detached carries its process handle in the committed
// result, and the handle-in-result and spare-as-detached decisions
// are one observation instead of two racy reads. Everything else
// gets the generic cancellation error. Returns the mapped ledger
// rows for the post-commit reconcile pass.
func committedPendingLocalToolCancellationMessages(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	interruptedAt time.Time,
) ([]chatstate.Message, []string, int64, []database.ChatToolCallExecution, error) {
	messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	if err != nil {
		return nil, nil, 0, nil, xerrors.Errorf("load committed messages for interruption: %w", err)
	}
	assistantMessageID, localCalls, _, err := unresolvedToolCallsFromHistory(messages, dynamicToolNamesFromChat(chat))
	if err != nil {
		return nil, nil, 0, nil, err
	}
	if len(localCalls) == 0 {
		return nil, nil, 0, nil, nil
	}
	toolCallIDs := make([]string, 0, len(localCalls))
	for _, call := range localCalls {
		toolCallIDs = append(toolCallIDs, call.ToolCallID)
	}
	mapped, err := store.MarkChatToolCallExecutionsInterrupted(ctx, database.MarkChatToolCallExecutionsInterruptedParams{
		ChatID:             chat.ID,
		AssistantMessageID: assistantMessageID,
		ToolCallIds:        toolCallIDs,
		SpareBackground:    true,
		UpdatedAt:          interruptedAt,
	})
	if err != nil {
		return nil, nil, 0, nil, xerrors.Errorf("mark tool call executions interrupted: %w", err)
	}
	mappedByCallID := make(map[string]database.ChatToolCallExecution, len(mapped))
	for _, row := range mapped {
		mappedByCallID[row.ToolCallID] = row
	}
	result := make([]chatstate.Message, 0, len(localCalls))
	for _, call := range localCalls {
		payload, isError, err := interruptedToolResultPayload(mappedByCallID[call.ToolCallID])
		if err != nil {
			return nil, nil, 0, nil, err
		}
		part := codersdk.ChatMessageToolResult(call.ToolCallID, call.ToolName, payload, isError, false)
		if !interruptedAt.IsZero() {
			part.CreatedAt = &interruptedAt
		}
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{part})
		if err != nil {
			return nil, nil, 0, nil, xerrors.Errorf("marshal interrupted tool result part: %w", err)
		}
		result = append(result, chatstate.Message{
			Role:           database.ChatMessageRoleTool,
			Content:        content,
			Visibility:     database.ChatMessageVisibilityBoth,
			ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: chat.LastModelConfigID != uuid.Nil},
			ContentVersion: chatprompt.CurrentContentVersion,
		})
	}
	return result, toolCallIDs, assistantMessageID, mapped, nil
}

// interruptedToolResultPayload builds the synthetic result committed
// for an unresolved tool call at interrupt, from the ledger row the
// interrupt mapping returned. A background execute the mapping
// spared as detached carries its process handle; everything else
// (including calls with no ledger row) gets the generic cancellation
// error.
func interruptedToolResultPayload(row database.ChatToolCallExecution) (json.RawMessage, bool, error) {
	payload, spared, err := chatstate.SparedBackgroundResult(row)
	if err != nil {
		return nil, false, err
	}
	if spared {
		return payload, false, nil
	}
	payload, err = json.Marshal(map[string]string{"error": interruptedToolResultErrorMessage})
	if err != nil {
		return nil, false, xerrors.Errorf("marshal interrupted tool result: %w", err)
	}
	return payload, true, nil
}

// interruptKillDialTimeout bounds the agent dial and signal round
// trip when killing processes on interrupt. Interrupts must stay
// fast even when the agent is slow or unreachable.
const interruptKillDialTimeout = 5 * time.Second

// interruptRecordGrace is how long after a claim an interrupted
// row without recorded process identity is left unresolved,
// giving an in-flight StartProcess time to land its handle on the
// cancel_requested row. Matches the execute tool's claim staleness
// bound.
const interruptRecordGrace = time.Minute

// interruptReconcileTimeout bounds the immediate post-interrupt
// reconcile pass: the per-row identity wait plus slack for the
// serial kill round-trips.
const interruptReconcileTimeout = interruptRecordGrace + time.Minute

// executionCancelGiveUpAfter is how long after its claim a
// cancel_requested row keeps being retried before the sweep
// records unknown and stops. Without this bound a row whose agent
// is stopped but never deleted would be retention-exempt and
// re-swept forever.
const executionCancelGiveUpAfter = 24 * time.Hour

// reconcileInterruptedExecutions resolves the cancel_requested
// ledger rows left by an interrupt commit. Everything is
// best-effort: the interrupt already committed and must not fail
// because an agent is slow or gone. A row whose kill cannot be
// confirmed keeps cancel_requested with its full process identity;
// the periodic execution sweep retries it until a terminal outcome
// is recorded. Background executes with a recorded handle were
// marked detached in the commit and are left running; the model
// chose to detach them.
func (s *taskStarter) reconcileInterruptedExecutions(ctx context.Context, records []database.ChatToolCallExecution) {
	if len(records) == 0 {
		return
	}
	// The runner may cancel the task context as soon as the
	// interrupt's state transition lands, so that cancellation
	// must be ignored. Worker shutdown must still abort this pass
	// promptly: it is awaited by the runner's task drain, and the
	// periodic sweep converges anything it abandons. The abort
	// hooks onto the worker context, which cancels at the start of
	// Close, not the server context: that one cancels only after
	// the drain this pass would otherwise block.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), interruptReconcileTimeout)
	defer cancel()
	if s.shutdownCtx != nil {
		stop := context.AfterFunc(s.shutdownCtx, cancel)
		defer stop()
	}
	r := s.executionReconciler()
	for _, record := range records {
		r.reconcile(ctx, record)
	}
}

// reconcile resolves one cancel_requested ledger row. It is the
// single dispatch shared by the immediate post-interrupt pass and
// the periodic sweep, so give-up and agent-gone handling cannot
// drift between them.
func (r executionReconciler) reconcile(ctx context.Context, record database.ChatToolCallExecution) {
	if record.Status != database.ChatToolCallExecutionStatusCancelRequested {
		return
	}
	if record.ProcessID.Valid && !record.WorkspaceAgentID.Valid {
		// The owning agent row was deleted out from under the
		// process (the FK nulls on delete): its workspace is gone
		// and the process died with it. Neither the kill flow nor
		// the missing-process guard can ever resolve this shape.
		// Checked before the give-up bound so an old row with a
		// provably dead process resolves canceled, not unknown.
		r.logger.Info(ctx, "interrupted process's agent row is gone; resolving execution canceled",
			slog.F("chat_id", record.ChatID),
			slog.F("tool_call_id", record.ToolCallID),
			slog.F("process_id", record.ProcessID.String),
		)
		r.resolveCancelOutcome(ctx, record, database.ChatToolCallExecutionStatusCanceled, record.CancelSignalSentAt)
		return
	}
	if r.resolveExpiredCancel(ctx, record) {
		return
	}
	if !record.ProcessID.Valid {
		// The claim may have dispatched a process whose identity
		// is not recorded yet: recordProcessStart runs on an
		// uncanceled context, so an interrupted in-flight
		// StartProcess can still land its handle on this
		// cancel_requested row moments after the commit. Wait out
		// that window instead of resolving early.
		r.resolveUnknownAfterIdentityWait(ctx, record)
		return
	}
	r.reconcileCancelRequested(ctx, record)
}

// resolveExpiredCancel gives up on a cancel_requested row whose
// kill has been unconfirmable for longer than
// executionCancelGiveUpAfter, recording unknown. The bound is
// anchored on the cancellation commit (result_committed_at is
// stamped in the same transaction that maps the row
// cancel_requested and is never bumped afterwards), not the claim:
// a long-lived call interrupted a day after its dispatch still gets
// its full kill-retry budget. It reports whether the row was
// resolved.
func (r executionReconciler) resolveExpiredCancel(ctx context.Context, record database.ChatToolCallExecution) bool {
	anchor := record.CreatedAt
	if record.ClaimedAt.Valid {
		anchor = record.ClaimedAt.Time
	}
	if record.ResultCommittedAt.Valid {
		anchor = record.ResultCommittedAt.Time
	}
	if dbtime.Now().Before(anchor.Add(executionCancelGiveUpAfter)) {
		return false
	}
	r.logger.Warn(ctx, "giving up on interrupted execution whose kill was never confirmed; resolving unknown",
		slog.F("chat_id", record.ChatID),
		slog.F("tool_call_id", record.ToolCallID),
		slog.F("workspace_agent_id", record.WorkspaceAgentID.UUID),
		slog.F("process_id", record.ProcessID.String),
		slog.F("cancel_requested_at", anchor),
	)
	r.resolveCancelOutcome(ctx, record, database.ChatToolCallExecutionStatusUnknown, record.CancelSignalSentAt)
	return true
}

// resolveUnknownAfterIdentityWait handles a cancel_requested row
// without recorded process identity: wait out the late-handle
// window, kill a handle that lands, and only then resolve unknown.
func (r executionReconciler) resolveUnknownAfterIdentityWait(ctx context.Context, record database.ChatToolCallExecution) {
	record = r.awaitInterruptedIdentity(ctx, record)
	if record.Status != database.ChatToolCallExecutionStatusCancelRequested {
		return
	}
	if record.ProcessID.Valid && record.WorkspaceAgentID.Valid {
		r.reconcileCancelRequested(ctx, record)
		return
	}
	r.resolveCancelOutcomeWithoutProcess(ctx, record, database.ChatToolCallExecutionStatusUnknown)
}

// executionReconciler resolves cancel_requested execution rows. It
// is shared between the immediate post-interrupt pass and the
// periodic sweep that retries rows whose first pass failed.
type executionReconciler struct {
	store     database.Store
	logger    slog.Logger
	agentConn AgentConnFunc
}

func newExecutionReconciler(store database.Store, logger slog.Logger, agentConn AgentConnFunc) executionReconciler {
	return executionReconciler{
		store:     store,
		logger:    logger,
		agentConn: agentConn,
	}
}

func (s *taskStarter) executionReconciler() executionReconciler {
	return newExecutionReconciler(s.opts.Store, s.opts.Logger, s.server.agentConnFn)
}

// awaitInterruptedIdentity polls a cancel_requested row without
// recorded process identity while the dead dispatch's uncanceled
// recordProcessStart write can still land (recordWriteTimeout plus
// slack from the claim). It returns the freshest row observed.
func (r executionReconciler) awaitInterruptedIdentity(ctx context.Context, record database.ChatToolCallExecution) database.ChatToolCallExecution {
	if !record.ClaimedAt.Valid {
		return record
	}
	deadline := record.ClaimedAt.Time.Add(interruptRecordGrace)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return record
		case <-time.After(2 * time.Second):
		}
		latest, err := r.store.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
			ChatID:             record.ChatID,
			AssistantMessageID: record.AssistantMessageID,
			ToolCallID:         record.ToolCallID,
		})
		if err != nil {
			continue
		}
		record = latest
		if record.ProcessID.Valid && record.WorkspaceAgentID.Valid {
			return record
		}
		if record.Status != database.ChatToolCallExecutionStatusCancelRequested {
			return record
		}
	}
	return record
}

// reconcileCancelRequested resolves one cancel_requested row with
// recorded process identity by killing the process. Background rows
// only reach cancel_requested when no committed result carries their
// handle (the handle had not landed at commit time, or a history
// delete committed no result at all): sparing such a process would
// leave it running with no addressable handle, so it is killed like
// foreground work. The kill records how far confirmation got:
// canceled when the agent definitively has no such process or a
// post-kill snapshot shows it exited, otherwise cancel_requested
// stays (with cancel_signal_sent_at set when the signal was
// delivered) for the periodic sweep to retry.
func (r executionReconciler) reconcileCancelRequested(ctx context.Context, record database.ChatToolCallExecution) {
	if _, err := r.store.GetWorkspaceAgentByID(ctx, record.WorkspaceAgentID.UUID); errors.Is(err, sql.ErrNoRows) {
		// The agent row is deleted (or soft-deleted): its
		// workspace pod is gone and the process died with it.
		// Dialing would fail every sweep forever.
		r.logger.Info(ctx, "interrupted process's agent is deleted; resolving execution canceled",
			slog.F("chat_id", record.ChatID),
			slog.F("tool_call_id", record.ToolCallID),
			slog.F("process_id", record.ProcessID.String),
		)
		r.resolveCancelOutcome(ctx, record, database.ChatToolCallExecutionStatusCanceled, record.CancelSignalSentAt)
		return
	}
	dialCtx, cancel := context.WithTimeout(ctx, interruptKillDialTimeout)
	defer cancel()
	conn, release, err := r.agentConn(dialCtx, record.WorkspaceAgentID.UUID)
	if err != nil {
		r.logger.Warn(ctx, "failed to dial agent to kill interrupted process",
			slog.F("chat_id", record.ChatID),
			slog.F("tool_call_id", record.ToolCallID),
			slog.F("workspace_agent_id", record.WorkspaceAgentID.UUID),
			slog.F("process_id", record.ProcessID.String),
			slog.Error(err),
		)
		return
	}
	if release != nil {
		defer release()
	}
	if err := conn.SignalProcess(dialCtx, record.ProcessID.String, "kill"); err != nil {
		if sdkErr, ok := errors.AsType[*codersdk.Error](err); ok && sdkErr.StatusCode() == http.StatusNotFound {
			// The agent was reached and has no such process:
			// termination is confirmed.
			r.resolveCancelOutcome(ctx, record, database.ChatToolCallExecutionStatusCanceled, sql.NullTime{})
			return
		}
		r.logger.Warn(ctx, "failed to kill interrupted process",
			slog.F("chat_id", record.ChatID),
			slog.F("tool_call_id", record.ToolCallID),
			slog.F("workspace_agent_id", record.WorkspaceAgentID.UUID),
			slog.F("process_id", record.ProcessID.String),
			slog.Error(err),
		)
		return
	}
	signalSentAt := sql.NullTime{Time: dbtime.Now(), Valid: true}
	r.resolveCancelOutcome(ctx, record, database.ChatToolCallExecutionStatusCancelRequested, signalSentAt)
	snapCtx, cancelSnap := context.WithTimeout(ctx, interruptKillDialTimeout)
	defer cancelSnap()
	resp, err := conn.ProcessOutput(snapCtx, record.ProcessID.String, nil)
	if err != nil {
		if sdkErr, ok := errors.AsType[*codersdk.Error](err); ok && sdkErr.StatusCode() == http.StatusNotFound {
			r.resolveCancelOutcome(ctx, record, database.ChatToolCallExecutionStatusCanceled, signalSentAt)
		}
		return
	}
	if !resp.Running {
		r.resolveCancelOutcome(ctx, record, database.ChatToolCallExecutionStatusCanceled, signalSentAt)
	}
}

// resolveCancelOutcomeWithoutProcess resolves a cancel_requested
// row whose outcome was decided from the absence of a process
// handle. The dead dispatch's recordProcessStart runs on an
// uncanceled context and can land identity on the row after the
// identity wait gave up; the guarded update loses to that write and
// the freshly identified process gets the kill flow instead of
// being orphaned behind a terminal status the sweep never retries.
func (r executionReconciler) resolveCancelOutcomeWithoutProcess(ctx context.Context, record database.ChatToolCallExecution, status database.ChatToolCallExecutionStatus) {
	_, err := r.store.UpdateChatToolCallExecutionCancelOutcome(ctx, database.UpdateChatToolCallExecutionCancelOutcomeParams{
		ChatID:                record.ChatID,
		AssistantMessageID:    record.AssistantMessageID,
		ToolCallID:            record.ToolCallID,
		Status:                status,
		CancelSignalSentAt:    sql.NullTime{},
		RequireMissingProcess: true,
		UpdatedAt:             dbtime.Now(),
	})
	if err == nil {
		if status == database.ChatToolCallExecutionStatusUnknown {
			// The idempotency safety net fired: the claim may
			// have dispatched a process whose handle was never
			// recorded. Surface it to operators.
			r.logger.Warn(ctx, "interrupted execution resolved with unknown process state",
				slog.F("chat_id", record.ChatID),
				slog.F("tool_call_id", record.ToolCallID),
			)
		}
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		r.logger.Warn(ctx, "failed to record interrupt cancel outcome",
			slog.F("chat_id", record.ChatID),
			slog.F("tool_call_id", record.ToolCallID),
			slog.F("status", string(status)),
			slog.Error(err),
		)
		return
	}
	latest, getErr := r.store.GetChatToolCallExecution(ctx, database.GetChatToolCallExecutionParams{
		ChatID:             record.ChatID,
		AssistantMessageID: record.AssistantMessageID,
		ToolCallID:         record.ToolCallID,
	})
	if getErr != nil {
		r.logger.Warn(ctx, "failed to re-read interrupted execution after guarded cancel outcome",
			slog.F("chat_id", record.ChatID),
			slog.F("tool_call_id", record.ToolCallID),
			slog.Error(getErr),
		)
		return
	}
	if latest.Status == database.ChatToolCallExecutionStatusCancelRequested && latest.ProcessID.Valid && latest.WorkspaceAgentID.Valid {
		r.reconcileCancelRequested(ctx, latest)
	}
}

func (r executionReconciler) resolveCancelOutcome(ctx context.Context, record database.ChatToolCallExecution, status database.ChatToolCallExecutionStatus, signalSentAt sql.NullTime) {
	_, err := r.store.UpdateChatToolCallExecutionCancelOutcome(ctx, database.UpdateChatToolCallExecutionCancelOutcomeParams{
		ChatID:                record.ChatID,
		AssistantMessageID:    record.AssistantMessageID,
		ToolCallID:            record.ToolCallID,
		Status:                status,
		CancelSignalSentAt:    signalSentAt,
		RequireMissingProcess: false,
		UpdatedAt:             dbtime.Now(),
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		r.logger.Warn(ctx, "failed to record interrupt cancel outcome",
			slog.F("chat_id", record.ChatID),
			slog.F("tool_call_id", record.ToolCallID),
			slog.F("status", string(status)),
			slog.Error(err),
		)
	}
}

// executionSweepRetryAge is how long a cancel_requested row must sit
// unchanged before the periodic sweep retries it. Sized past the
// immediate post-interrupt pass's bound so the sweep never races a
// pass that is still working the row.
const executionSweepRetryAge = 3 * time.Minute

// executionSweepBatchSize bounds one sweep pass.
const executionSweepBatchSize = 100

// executionSweepPassTimeout bounds one sweep pass so a batch of
// unreachable agents (each burning serial dial timeouts) cannot
// collapse the retry cadence. Rows the deadline abandons keep
// their lease and retry after executionSweepRetryAge.
const executionSweepPassTimeout = time.Minute

// executionSweepLoop periodically retries stalled cancel_requested
// executions. The immediate post-interrupt pass is best-effort, so
// this loop is what guarantees an interrupted process is eventually
// killed even when its agent was unreachable at interrupt time or
// the server died mid-reconciliation.
func (w *chatWorker) executionSweepLoop(ctx context.Context) {
	r := newExecutionReconciler(w.opts.Store, w.opts.Logger, w.server.agentConnFn)
	ticker := w.opts.Clock.NewTicker(w.opts.ExecutionSweepInterval, "chatworker", "execution-sweep")
	defer ticker.Stop("chatworker", "execution-sweep")
	for {
		select {
		case tick := <-ticker.C:
			ticker.Stop("chatworker", "execution-sweep")
			r.sweepOnce(ctx, dbtime.Time(tick).UTC())
			ticker.Reset(w.opts.ExecutionSweepInterval, "chatworker", "execution-sweep")
		case <-ctx.Done():
			return
		}
	}
}

// sweepOnce retries cancel_requested rows with recorded process
// identity whose reconciliation stalled: the post-interrupt pass is
// best-effort, so an unreachable agent or a dying server leaves the
// row unresolved. The claim query bumps updated_at as a lease, so
// unresolved rows retry on the next sweep without hammering the
// same agent from every replica. Rows without recorded process
// identity (a crash between the interrupt commit and
// reconciliation) get the same late-handle wait as the immediate
// pass, then resolve unknown; the sweep retry age exceeds the
// record grace window, so the wait normally returns immediately.
func (r executionReconciler) sweepOnce(ctx context.Context, now time.Time) {
	ctx, cancel := context.WithTimeout(ctx, executionSweepPassTimeout)
	defer cancel()
	records, err := r.store.ClaimStaleChatToolCallExecutionCancels(ctx, database.ClaimStaleChatToolCallExecutionCancelsParams{
		UpdatedBefore: now.Add(-executionSweepRetryAge),
		LimitCount:    executionSweepBatchSize,
		Now:           now,
	})
	if err != nil {
		r.logger.Warn(ctx, "failed to claim stale interrupted executions", slog.Error(err))
		return
	}
	for _, record := range records {
		select {
		case <-ctx.Done():
			return
		default:
		}
		r.reconcile(ctx, record)
	}
}
