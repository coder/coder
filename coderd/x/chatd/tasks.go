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

// interruptCancelTimeout bounds the requests an interrupt sends to the
// workspace agent for execute calls. It exists only so an unreachable
// agent yields an unknown result: interrupt latency is not a concern,
// and waiting for the agent's answer reports the real outcome.
const interruptCancelTimeout = time.Minute

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
	// The transaction's history version fence guarantees it sees the same
	// unresolved tool calls as messages.
	interruptResults, err := s.interruptToolCalls(ctx, chat, messages)
	if err != nil {
		return xerrors.Errorf("interrupt tool calls: %w", err)
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
		committedCancels, err := committedPendingLocalToolCancellationMessages(ctx, store, chat, interruptedAt, toolCompletions, interruptResults)
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

// interruptToolCalls ends the unresolved foreground execute, edit_files,
// and write_file calls in messages, looks up the processes of the
// background execute calls, and returns their results by provider tool
// call ID. A call without an entry keeps the generic interrupted result.
// The message part buffer plays no part: the canceled generation
// goroutine records completions and publishes results before interrupt
// handling reads them, and it holds nothing after an ownership change.
func (s *taskStarter) interruptToolCalls(
	ctx context.Context,
	chat database.Chat,
	messages []database.ChatMessage,
) (map[string]interruptResult, error) {
	toolCallMsg, localCalls, _, err := unresolvedToolCallsFromHistory(messages, dynamicToolNamesFromChat(chat))
	if err != nil {
		return nil, err
	}
	calls := interruptCalls(localCalls)
	if len(calls) == 0 {
		return map[string]interruptResult{}, nil
	}
	dbNow, err := s.opts.Store.GetDatabaseNow(ctx)
	if err != nil {
		return nil, normalizeTaskInfrastructureError(err, "get database time")
	}
	age := chattool.NewToolCallAge(s.opts.Clock, dbNow, toolCallMsg.createdAt)

	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := s.opts.Clock.AfterFunc(interruptCancelTimeout, cancel, "chatworker", "interrupt_cancel")
	defer timer.Stop()

	results := make([]interruptResult, len(calls))
	answered := make([]bool, len(calls))
	errs := make([]error, len(calls))
	identities := make([]chattool.ToolCallIdentity, len(calls))
	for i, call := range calls {
		identities[i] = chattool.ToolCallIdentity{
			ChatID:     chat.ID,
			MessageID:  toolCallMsg.id,
			ToolCallID: call.toolCallID,
			Age:        age,
		}
	}
	workspaceCtx := newTurnWorkspaceContext(s.server, chat)
	defer workspaceCtx.close()
	conn, err := workspaceCtx.getWorkspaceConn(agentCtx)
	if err != nil {
		// Each call decides whether anything it did can have survived
		// without a reachable agent.
		for i, call := range calls {
			results[i], answered[i], errs[i] = call.unreachableResult(identities[i], err)
		}
	} else {
		var wg sync.WaitGroup
		for i, call := range calls {
			wg.Go(func() {
				results[i], answered[i], errs[i] = call.interrupt(agentCtx, conn, identities[i])
			})
		}
		wg.Wait()
	}
	// Answers to requests the task's context ended are not the agent's.
	if ctx.Err() != nil {
		return nil, errors.Join(errTaskExpectedExit, xerrors.Errorf("interrupt tool calls: %w", ctx.Err()))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	byID := make(map[string]interruptResult, len(calls))
	for i, call := range calls {
		if answered[i] {
			byID[call.toolCallID] = results[i]
		}
	}
	return byID, nil
}

// interruptResult is the result interrupt handling commits for a tool
// call from the workspace agent's answer.
type interruptResult struct {
	payload json.RawMessage
	isError bool
}

// executeInterruptResult returns the interrupt result of an execute call,
// stored as generation stores the execute tool's response. Execute
// results are never error results, so the model sees every field.
func executeInterruptResult(call interruptCall, result chattool.ExecuteResult) (interruptResult, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return interruptResult{}, xerrors.Errorf("marshal interrupted execute result: %w", err)
	}
	return toolResponseInterruptResult(call, fantasy.NewTextResponse(string(payload))), nil
}

// toolResponseInterruptResult returns the interrupt result of call from
// the response its tool would have returned, stored as generation stores
// a tool response.
func toolResponseInterruptResult(call interruptCall, resp fantasy.ToolResponse) interruptResult {
	// Generation cuts each result to a budget derived from the model's
	// context window. Interrupt handling does not load the chat's model,
	// so it uses the default budget, the one generation uses when the
	// context window is unknown.
	content, _ := chatloop.TruncateToolResult(resp.Content, 0)
	var output fantasy.ToolResultOutputContent = fantasy.ToolResultOutputContentText{Text: content}
	if resp.IsError {
		output = fantasy.ToolResultOutputContentError{Error: xerrors.New(content)}
	}
	part := chatprompt.PartFromContent(fantasy.ToolResultContent{
		ToolCallID: call.toolCallID,
		ToolName:   call.toolName,
		Result:     output,
	})
	return interruptResult{payload: part.Result, isError: part.IsError}
}

// interruptCall is an unresolved tool call that interrupt handling asks
// the workspace agent about: an execute call, or an edit_files or
// write_file call.
type interruptCall struct {
	toolCallID string
	toolName   string
	// background is set for an execute call that runs in the background.
	background bool
	// timeout is the effective timeout of a foreground execute call.
	timeout time.Duration
}

// interrupt asks the workspace agent about call and returns its result.
// ok is false when the call keeps the generic interrupted result.
func (call interruptCall) interrupt(ctx context.Context, conn workspacesdk.AgentConn, id chattool.ToolCallIdentity) (result interruptResult, ok bool, err error) {
	if call.toolName != chattool.ExecuteToolName {
		resp, ok := chattool.InterruptFileToolCall(ctx, conn, call.toolName, id)
		if !ok {
			return interruptResult{}, false, nil
		}
		return toolResponseInterruptResult(call, resp), true, nil
	}
	var execResult chattool.ExecuteResult
	if call.background {
		execResult, ok = chattool.InterruptBackgroundExecute(ctx, conn, id)
	} else {
		execResult, ok = chattool.InterruptExecute(ctx, conn, id, call.timeout)
	}
	if !ok {
		return interruptResult{}, false, nil
	}
	result, err = executeInterruptResult(call, execResult)
	return result, true, err
}

// unreachableResult returns the result of call when no connection to the
// workspace agent could be made. ok is false when the call keeps the
// generic interrupted result: an execute call without a workspace agent,
// whose process cannot be running, and a file tool call whose change
// cannot have survived. A stopped workspace keeps its disk, so a file
// tool call without an agent gets an unknown result.
func (call interruptCall) unreachableResult(id chattool.ToolCallIdentity, connErr error) (result interruptResult, ok bool, err error) {
	if call.toolName != chattool.ExecuteToolName {
		resp, ok := chattool.FileToolCallConnErrorResult(call.toolName, connErr)
		if !ok {
			return interruptResult{}, false, nil
		}
		return toolResponseInterruptResult(call, resp), true, nil
	}
	if chattool.HasNoWorkspaceAgent(connErr) {
		return interruptResult{}, false, nil
	}
	execResult := chattool.AgentUnreachableExecuteResult(id, connErr)
	if call.background {
		execResult = chattool.AgentUnreachableBackgroundExecuteResult(id, connErr)
	}
	result, err = executeInterruptResult(call, execResult)
	return result, true, err
}

// interruptCalls returns the calls in localCalls that interrupt handling
// asks the workspace agent about. Interrupt results are keyed by provider
// tool call ID, so two calls sharing one cannot get different results:
// such an ID is returned once, and only when every call with it is
// handled the same way.
func interruptCalls(localCalls []fantasy.ToolCallContent) []interruptCall {
	byID := make(map[string]interruptCall)
	skip := make(map[string]bool)
	var ids []string
	for _, call := range localCalls {
		classified, ok := classifyInterruptCall(call)
		prev, seen := byID[call.ToolCallID]
		if !ok || (seen && prev != classified) {
			skip[call.ToolCallID] = true
		}
		if !seen {
			byID[call.ToolCallID] = classified
			ids = append(ids, call.ToolCallID)
		}
	}
	var calls []interruptCall
	for _, id := range ids {
		if !skip[id] {
			calls = append(calls, byID[id])
		}
	}
	return calls
}

// classifyInterruptCall returns how interrupt handling acts on call.
// Every edit_files and write_file call qualifies: they have no deadline,
// and cancel waits for an edit in progress, so the answer is the edit's
// outcome. ok is false for other tools, for unparsable execute arguments,
// and for a foreground execute call with an invalid timeout, for which
// the tool starts no process.
func classifyInterruptCall(call fantasy.ToolCallContent) (interruptCall, bool) {
	switch call.ToolName {
	case chattool.EditFilesToolName, chattool.WriteFileToolName:
		return interruptCall{toolCallID: call.ToolCallID, toolName: call.ToolName}, true
	case chattool.ExecuteToolName:
	default:
		return interruptCall{}, false
	}
	var args chattool.ExecuteArgs
	if err := json.Unmarshal([]byte(call.Input), &args); err != nil {
		return interruptCall{}, false
	}
	if args.RunsInBackground() {
		return interruptCall{toolCallID: call.ToolCallID, toolName: call.ToolName, background: true}, true
	}
	timeout, err := args.EffectiveTimeout(chattool.ExecuteDefaultTimeout)
	if err != nil {
		return interruptCall{}, false
	}
	return interruptCall{toolCallID: call.ToolCallID, toolName: call.ToolName, timeout: timeout}, true
}

// committedPendingLocalToolCancellationMessages returns a result for
// every unresolved local tool call: the entry of interruptResults for its
// provider tool call ID, or the generic interrupted result.
func committedPendingLocalToolCancellationMessages(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	interruptedAt time.Time,
	toolCompletions map[int]messagepartbuffer.ToolCompletion,
	interruptResults map[string]interruptResult,
) ([]chatstate.Message, error) {
	messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	if err != nil {
		return nil, xerrors.Errorf("load committed messages for interruption: %w", err)
	}
	_, localCalls, _, err := unresolvedToolCallsFromHistory(messages, dynamicToolNamesFromChat(chat))
	if err != nil {
		return nil, err
	}
	if len(localCalls) == 0 {
		return nil, nil
	}
	var intervals []chatloop.BilledInterval
	result := make([]chatstate.Message, 0, len(localCalls))
	for i, call := range localCalls {
		interrupted, ok := interruptResults[call.ToolCallID]
		payload, isError := interrupted.payload, interrupted.isError
		if !ok {
			payload, err = json.Marshal(map[string]string{"error": interruptedToolResultErrorMessage})
			if err != nil {
				return nil, xerrors.Errorf("marshal interrupted tool result: %w", err)
			}
			isError = true
		}
		part := codersdk.ChatMessageToolResult(call.ToolCallID, call.ToolName, payload, isError, false)
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
