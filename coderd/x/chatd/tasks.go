package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

const (
	postCommitWatchPublishTimeout = 10 * time.Second
	defaultTaskTimeout            = 15 * time.Minute
	taskTimeoutMargin             = 5 * time.Minute
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
	tag := "task-timeout-" + string(kind)
	timer := clock.AfterFunc(defaultTaskTimeout, func() {
		cancelCause(errTaskTimeout)
	}, "chatworker", tag)
	// A silent stream must fail through the silence guard rather than the
	// watchdog, and a healthy long stream must not be cut off.
	attemptCtx = chatloop.WithStreamWatchdog(attemptCtx, func(silence time.Duration) {
		if silence < 0 {
			timer.Stop()
			return
		}
		timer.Reset(max(defaultTaskTimeout, silence+taskTimeoutMargin), "chatworker", tag)
	})
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
	Chat database.Chat
	Kind runnerActionKind
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

func (s *taskStarter) StartInterrupt(ctx context.Context, input chatWorkerTaskStartInput) error {
	machine := chatstate.NewChatMachine(s.opts.Store, s.opts.Pubsub, input.ChatID)
	var chat database.Chat
	var messages []database.ChatMessage
	err := machine.ReadLock(ctx, func(store database.Store) error {
		loadedChat, err := loadChatForTask(ctx, store, input, database.ChatStatusInterrupting, taskFenceOptions{requireHistory: true})
		if err != nil {
			return xerrors.Errorf("load chat for task: %w", err)
		}
		loadedMessages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  input.ChatID,
			AfterID: 0,
		})
		if err != nil {
			return xerrors.Errorf("load chat messages: %w", err)
		}
		chat = loadedChat
		messages = loadedMessages
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
	modelInvokedAt := s.opts.MessagePartBuffer.ModelInvokedAt(key)
	toolCompletions := s.opts.MessagePartBuffer.ToolCompletions(key)
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
	interruptedAt := s.opts.Clock.Now("chatworker", "interrupt")
	var attemptRuntime time.Duration
	if !modelInvokedAt.IsZero() {
		attemptRuntime = interruptedAt.Sub(modelInvokedAt)
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
	// The update below fences on the history version, so it sees the
	// unresolved tool calls of messages.
	toolCallMessageID, interruptResults, err := s.server.runInterruptedToolCalls(ctx, chat, messages, chattool.ToolCallCauseInterrupt)
	if err != nil {
		if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("run interrupted tool calls: %w", err), ctx.Err())
		}
		return xerrors.Errorf("run interrupted tool calls: %w", err)
	}

	var committed database.Chat
	err = machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		chat, err := loadChatForTask(ctx, store, input, database.ChatStatusInterrupting, taskFenceOptions{requireHistory: true})
		if err != nil {
			return xerrors.Errorf("load chat for task: %w", err)
		}
		messages := partialMessages
		// Reuse the captured interrupt instant so database delay does not
		// inflate billing.
		committedCancels, err := committedPendingLocalToolCancellationMessages(ctx, store, chat, interruptedAt, toolCompletions, toolCallMessageID, interruptResults)
		if err != nil {
			return xerrors.Errorf("committed pending local tool cancellation messages: %w", err)
		}
		if len(committedCancels) > 0 {
			messages = append(append([]chatstate.Message{}, partialMessages...), committedCancels...)
		}
		if _, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{
			PartialMessages: messages,
		}); err != nil {
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
		Chat: committed,
		Kind: runnerActionKindFinishInterruption,
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
	if s.server.disableCallerSuppliedTools {
		return s.cancelRequiresAction(ctx, machine, input, "Tool execution canceled because caller-supplied tools are disabled")
	}
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
		if !s.server.disableCallerSuppliedTools && chat.RequiresActionDeadlineAt.Valid {
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

// interruptResult is the result the interrupt run built for an
// unresolved tool call.
type interruptResult struct {
	part codersdk.ChatMessagePart
}

// runInterruptedToolCalls runs the interrupt run over the unresolved
// tool calls in messages, the history of chat: every call of a tool that
// sends tool call headers is dispatched with cause, which makes the tool
// cancel it on the workspace agent before sending its request again. It
// returns the ID of the assistant message with the unresolved calls and
// the results by provider tool call ID. A call without a result keeps
// the generic interrupted result, which is every call when the agent is
// not capable. When ctx ends, it returns an error: answers to requests
// that ended with ctx are not the agent's.
func (server *Server) runInterruptedToolCalls(
	ctx context.Context,
	chat database.Chat,
	messages []database.ChatMessage,
	cause chattool.ToolCallCause,
) (toolCallMessageID int64, results map[string]interruptResult, err error) {
	toolCallMsg, localCalls, _, err := unresolvedToolCallsFromHistory(messages, dynamicToolNamesFromChat(chat))
	if err != nil {
		return 0, nil, err
	}
	calls := interruptedToolCalls(localCalls)
	if len(calls) == 0 {
		return toolCallMsg.id, nil, nil
	}

	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:      server,
		chatStateMu: &sync.Mutex{},
		currentChat: &currentChat,
		loadChatSnapshot: func(ctx context.Context, chatID uuid.UUID) (database.Chat, error) {
			return server.db.GetChatByID(ctx, chatID)
		},
	}
	defer workspaceCtx.close()
	if !workspaceCtx.isCapableAgent(ctx) {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		return toolCallMsg.id, nil, nil
	}
	// Dial once for every call, so an unreachable agent costs one dial
	// timeout and each tool reports the same error.
	conn, connErr := workspaceCtx.getWorkspaceConn(ctx)
	getConn := func(context.Context) (workspacesdk.AgentConn, error) { return conn, connErr }

	var contextLimit int64
	if modelConfig, err := server.db.GetChatModelConfigByID(ctx, chat.LastModelConfigID); err == nil {
		contextLimit = modelConfig.ContextLimit
	} else if ctx.Err() == nil {
		// The default result budget applies.
		server.logger.Warn(ctx, "load model config for interrupted tool calls",
			slog.F("chat_id", chat.ID), slog.Error(err))
	}

	tools := chattool.NewToolCallTools(chattool.ToolCallToolsOptions{
		GetWorkspaceConn:    getConn,
		AgentBrowserSession: chat.ID.String(),
		Clock:               server.clock,
	})
	step, err := chatloop.ExecuteLocalTools(ctx, chatloop.ExecuteLocalToolsOptions{
		Tools:     []fantasy.AgentTool{tools.Execute, tools.EditFiles, tools.WriteFile},
		ToolCalls: calls,
		ToolCallIdentity: chattool.ToolCallIdentity{
			ChatID:    chat.ID,
			MessageID: toolCallMsg.id,
			Cause:     cause,
		},
		ContextLimit: contextLimit,
		Clock:        server.clock,
		Logger:       server.logger,
	})
	if err != nil {
		return 0, nil, xerrors.Errorf("execute interrupted tool calls: %w", err)
	}
	results = make(map[string]interruptResult, len(step.Content))
	for _, content := range step.Content {
		tr, ok := content.(fantasy.ToolResultContent)
		if !ok {
			continue
		}
		part := chatprompt.PartFromContent(tr)
		if createdAt, ok := step.ToolResultCreatedAt[tr.ToolCallID]; ok {
			part.CreatedAt = &createdAt
		}
		results[tr.ToolCallID] = interruptResult{part: part}
	}
	return toolCallMsg.id, results, nil
}

// interruptedToolCalls returns the calls in localCalls that the
// interrupt run dispatches: those of tools that send tool call headers.
// Results are committed by provider tool call ID alone, so calls sharing
// an ID are dispatched once, and only when every call with that ID is
// for the same such tool with the same input.
func interruptedToolCalls(localCalls []fantasy.ToolCallContent) []fantasy.ToolCallContent {
	first := make(map[string]fantasy.ToolCallContent, len(localCalls))
	skip := make(map[string]bool)
	var ids []string
	for _, call := range localCalls {
		prev, seen := first[call.ToolCallID]
		if !chattool.SendsToolCallIdentity(call.ToolName) ||
			(seen && (prev.ToolName != call.ToolName || prev.Input != call.Input)) {
			skip[call.ToolCallID] = true
		}
		if !seen {
			first[call.ToolCallID] = call
			ids = append(ids, call.ToolCallID)
		}
	}
	var calls []fantasy.ToolCallContent
	for _, id := range ids {
		if !skip[id] {
			calls = append(calls, first[id])
		}
	}
	return calls
}

// committedPendingLocalToolCancellationMessages returns a result for
// every unresolved local tool call: its entry in interruptResults, which
// runInterruptedToolCalls built for the tool call message
// toolCallMessageID, or the generic interrupted result.
func committedPendingLocalToolCancellationMessages(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	interruptedAt time.Time,
	toolCompletions map[int]messagepartbuffer.ToolCompletion,
	toolCallMessageID int64,
	interruptResults map[string]interruptResult,
) ([]chatstate.Message, error) {
	messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	if err != nil {
		return nil, xerrors.Errorf("load committed messages for interruption: %w", err)
	}
	toolCallMsg, localCalls, _, err := unresolvedToolCallsFromHistory(messages, dynamicToolNamesFromChat(chat))
	if err != nil {
		return nil, err
	}
	if len(localCalls) == 0 {
		return nil, nil
	}
	if len(interruptResults) > 0 && toolCallMsg.id != toolCallMessageID {
		return nil, xerrors.Errorf("interrupted tool calls ran for message %d, but the latest tool call message is %d", toolCallMessageID, toolCallMsg.id)
	}
	var intervals []chatloop.BilledInterval
	result := make([]chatstate.Message, 0, len(localCalls))
	for i, call := range localCalls {
		res, ok := interruptResults[call.ToolCallID]
		if !ok {
			payload, err := json.Marshal(map[string]string{"error": interruptedToolResultErrorMessage})
			if err != nil {
				return nil, xerrors.Errorf("marshal interrupted tool result: %w", err)
			}
			res.part = codersdk.ChatMessageToolResult(call.ToolCallID, call.ToolName, payload, true, false)
		}
		part := res.part
		if part.CreatedAt == nil && !interruptedAt.IsZero() {
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
		if unbilledSubagentToolNames[call.ToolName] {
			continue
		}
		// Bill only started calls. Completed calls end at completion and
		// running calls end at the interrupt.
		occurrence, ok := toolCompletions[i]
		if !ok {
			continue
		}
		start := occurrence.StartedAt
		if start.IsZero() {
			continue
		}
		end := occurrence.CompletedAt
		if end.IsZero() {
			end = interruptedAt
		}
		intervals = append(intervals, chatloop.BilledInterval{Start: start, End: end})
	}
	// Bill the interval union once on a dedicated usage record so
	// cancellation rows stay free of batch-level runtime.
	stamp, ok, err := batchUsageMessage(
		chat.LastModelConfigID,
		chatprompt.CurrentContentVersion,
		chatloop.BilledIntervalsDuration(intervals),
		len(intervals),
	)
	if err != nil {
		return nil, err
	}
	if ok {
		result = append(result, stamp)
	}
	return result, nil
}
