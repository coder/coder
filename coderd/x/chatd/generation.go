package chatd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

// generationPrepareInput contains the committed state used to prepare one
// generation action.
type generationPrepareInput struct {
	Chat     database.Chat
	Messages []database.ChatMessage
}

// generationPrepared contains the side-effect inputs for a generation task.
type generationPrepared struct {
	Chat     database.Chat
	Messages []database.ChatMessage

	Model              chatprovider.Model
	Prompt             []fantasy.Message
	Tools              []fantasy.AgentTool
	ActiveTools        []string
	AllowInactiveTools map[string]bool
	ProviderTools      []chatloop.ProviderTool
	ModelRoute         aiGatewayModelRoute
	ModelBuildOptions  modelBuildOptions

	// ResolvedProvider is the configured provider identity used to label
	// user-facing errors. See chatloop.GenerateAssistantOptions.ErrorProvider.
	ResolvedProvider string

	ModelConfigID uuid.UUID
	// CallTemplate is the resolver-built assistant call envelope; see
	// chatloop.GenerateAssistantOptions.CallTemplate.
	CallTemplate         fantasy.Call
	ContextLimitFallback int64

	DynamicToolNames   map[string]bool
	StopAfterTools     map[string]struct{}
	ExclusiveToolNames map[string]bool
	BuiltinToolNames   map[string]bool
	ToolNameToConfigID map[string]uuid.UUID

	MaxSteps   int
	Compaction *generationCompaction
	// Cleanup is always non-nil when prepareGeneration succeeds.
	Cleanup func()

	Debug *generationDebug
}

// generationCompaction contains compaction inputs prepared for generation.
type generationCompaction struct {
	// Override, when non-nil, is the compaction model override resolved at
	// prepare time. Its model client is built in the compact action path,
	// so construction failures cannot fail turns that never compact.
	Override *resolvedCompactionOverride
	// ChatModelConfig is the chat model's config, used to detect provider
	// changes when sanitizing the compaction prompt.
	ChatModelConfig database.ChatModelConfig

	Required bool
	Options  chatloop.GenerateCompactionOptions
}

type generationDebug struct {
	Enabled             bool
	Service             *chatdebug.Service
	Provider            string
	Model               string
	TriggerMessageID    int64
	HistoryTipMessageID int64
	TriggerLabel        string
	ModelConfig         database.ChatModelConfig
}

// generationOutcome describes a completed generation outcome.
type generationOutcome struct {
	Chat              database.Chat
	Kind              runnerActionKind
	WatchEventKind    codersdk.ChatWatchEventKind
	LastError         string
	PromotedMessageID int64
	InsertedMessages  []runnerActionMessage
}

type generationActionKind string

const (
	generationActionExecuteLocalTools   generationActionKind = "execute_local_tools"
	generationActionEnterRequiresAction generationActionKind = "enter_requires_action"
	generationActionFinishTurn          generationActionKind = "finish_turn"
	generationActionCompact             generationActionKind = "compact"
	generationActionGenerateAssistant   generationActionKind = "generate_assistant"
)

type generationFinishReason string

const (
	generationFinishReasonStopAfterTool generationFinishReason = "stop_after_tool"
	generationFinishReasonComplete      generationFinishReason = "complete"
	generationFinishReasonMaxSteps      generationFinishReason = "max_steps"
)

var errCompactionStillOverLimit = chaterror.WithClassification(
	xerrors.New("compaction left the chat above the compaction limit"),
	chaterror.ClassifiedError{
		Message: "Conversation compaction could not reduce the history below the configured limit. Raise the compaction limit in settings, or start a new conversation.",
		Kind:    codersdk.ChatErrorKindConfig,
	},
)

type generationDecision struct {
	kind              generationActionKind
	localToolCalls    []fantasy.ToolCallContent
	finishReason      generationFinishReason
	promotedMessageID int64
	// forced marks a compact action triggered by a manual
	// compaction request rather than the usage threshold.
	forced bool
}

type generationRetryDecision struct {
	retry             bool
	generationAttempt int64
	delay             time.Duration
}

var errRetryStateDecisionOnly = xerrors.New("retry state decision only")

// errTerminalGeneration marks a prepare or decide failure as terminal: a
// deterministic error where retrying cannot help. The generation loop
// finishes the turn with an error instead of retrying when an error
// unwraps to this sentinel.
var errTerminalGeneration = xerrors.New("terminal generation error")

type terminalGenerationError struct{ err error }

func (e terminalGenerationError) Error() string { return e.err.Error() }

func (e terminalGenerationError) Unwrap() error { return errors.Join(errTerminalGeneration, e.err) }

// terminalGeneration wraps err so the prepare/decide retry loop stops
// immediately and finishes the turn with an error.
func terminalGeneration(err error) error {
	if err == nil {
		return nil
	}
	return terminalGenerationError{err: err}
}

func isTerminalGeneration(err error) bool {
	return errors.Is(err, errTerminalGeneration)
}

type generationDecisionInput struct {
	chat                       database.Chat
	messages                   []database.ChatMessage
	dynamicToolNames           map[string]bool
	exclusiveToolNames         map[string]bool
	stopAfterTools             map[string]struct{}
	maxSteps                   int
	compactionEnabled          bool
	compactionNeeded           bool
	compactionThresholdPercent int32
	compactionContextLimit     int64
}

func decideGenerationAction(input generationDecisionInput) (generationDecision, error) {
	localCalls, dynamicCalls, err := unresolvedToolCallsFromHistory(input.messages, input.dynamicToolNames)
	if err != nil {
		return generationDecision{}, err
	}
	if len(localCalls) > 0 {
		if len(dynamicCalls) > 0 && hasExclusiveToolCall(localCalls, input.exclusiveToolNames) {
			for _, dynamicCall := range dynamicCalls {
				localCalls = append(localCalls, fantasy.ToolCallContent{
					ToolCallID: dynamicCall.ToolCallID,
					ToolName:   dynamicCall.ToolName,
					Input:      dynamicCall.Args,
				})
			}
		}
		return generationDecision{kind: generationActionExecuteLocalTools, localToolCalls: localCalls}, nil
	}
	if len(dynamicCalls) > 0 {
		return generationDecision{kind: generationActionEnterRequiresAction}, nil
	}

	// A manual compaction request wins over every non-tool decision:
	// idle chats would otherwise finish the turn via the
	// history-complete check before ever compacting. The request is
	// ignored when nothing after the latest boundary is compactable
	// (for example the history was edited between request and
	// execution); the stale marker is then cleared by the terminal
	// transition of this turn.
	if input.chat.CompactionRequestedAt.Valid {
		boundary := latestCompactionBoundaryIndex(input.messages)
		if _, ok := firstUncompressedAssistantAfter(input.messages, boundary); ok {
			return generationDecision{kind: generationActionCompact, forced: true}, nil
		}
	}

	stopAfter, err := historyHasStopAfterToolResult(input.messages, input.stopAfterTools)
	if err != nil {
		return generationDecision{}, err
	}
	if stopAfter {
		return generationDecision{kind: generationActionFinishTurn, finishReason: generationFinishReasonStopAfterTool}, nil
	}
	complete, err := currentHistoryComplete(input.messages)
	if err != nil {
		return generationDecision{}, err
	}
	if complete {
		return generationDecision{kind: generationActionFinishTurn, finishReason: generationFinishReasonComplete}, nil
	}
	if input.maxSteps > 0 && currentTurnStepCount(input.messages) >= input.maxSteps {
		return generationDecision{kind: generationActionFinishTurn, finishReason: generationFinishReasonMaxSteps}, nil
	}
	compactionRequirement := compactionRequirementNotNeeded
	if input.compactionEnabled && input.compactionNeeded {
		compactionRequirement = compactionRequirementNeeded
	}
	switch compactionStatusFromHistory(input.messages, compactionRequirement, input.compactionThresholdPercent, input.compactionContextLimit) {
	case compactionStatusNeeded:
		return generationDecision{kind: generationActionCompact}, nil
	case compactionStatusAfterCompaction:
		return generationDecision{kind: generationActionGenerateAssistant}, nil
	case compactionStatusStillOverLimit:
		return generationDecision{}, terminalGeneration(errCompactionStillOverLimit)
	case compactionStatusNotNeeded:
		return generationDecision{kind: generationActionGenerateAssistant}, nil
	default:
		return generationDecision{}, terminalGeneration(xerrors.New("unknown compaction status"))
	}
}

func generationCompactionThreshold(compaction *generationCompaction) int32 {
	if compaction == nil {
		return 0
	}
	return compaction.Options.ThresholdPercent
}

// generationCompactionContextLimit returns the context limit the compaction
// trigger was evaluated against at prepare time (the stricter of the chat and
// override models' limits). The still-over-limit check must compare against
// the same limit, otherwise a stricter override loops through repeated
// compactions instead of surfacing errCompactionStillOverLimit.
func generationCompactionContextLimit(compaction *generationCompaction) int64 {
	if compaction == nil {
		return 0
	}
	return compaction.Options.ContextLimit
}

func unresolvedToolCallsFromHistory(
	messages []database.ChatMessage,
	dynamicToolNames map[string]bool,
) ([]fantasy.ToolCallContent, []pendingDynamicToolCall, error) {
	assistantIndex := lastMessageIndex(messages, func(msg database.ChatMessage) bool {
		return msg.Role == database.ChatMessageRoleAssistant
	})
	if assistantIndex == -1 {
		return nil, nil, nil
	}
	assistantParts, err := chatprompt.ParseContent(messages[assistantIndex])
	if err != nil {
		return nil, nil, xerrors.Errorf("parse assistant message: %w", err)
	}
	handled, err := handledToolCallIDs(messages[assistantIndex+1:])
	if err != nil {
		return nil, nil, err
	}
	localCalls := make([]fantasy.ToolCallContent, 0)
	dynamicCalls := make([]pendingDynamicToolCall, 0)
	for _, part := range assistantParts {
		if part.Type != codersdk.ChatMessagePartTypeToolCall || part.ProviderExecuted || handled[part.ToolCallID] {
			continue
		}
		if dynamicToolNames[part.ToolName] {
			dynamicCalls = append(dynamicCalls, pendingDynamicToolCall{
				ToolCallID: part.ToolCallID,
				ToolName:   part.ToolName,
				Args:       string(part.Args),
			})
			continue
		}
		localCalls = append(localCalls, fantasy.ToolCallContent{
			ToolCallID:       part.ToolCallID,
			ToolName:         part.ToolName,
			Input:            string(part.Args),
			ProviderExecuted: part.ProviderExecuted,
		})
	}
	return localCalls, dynamicCalls, nil
}

// exclusiveBatchRejected reports whether the exclusive-tool policy will
// reject the whole batch, which mirrors the condition chatloop applies
// when it decides that nothing in the batch may execute. Callers must ask
// before filtering the batch, because dropping calls from it can leave the
// exclusive call alone and admissible.
func exclusiveBatchRejected(toolCalls []fantasy.ToolCallContent, exclusiveToolNames map[string]bool) bool {
	return len(toolCalls) > 1 && hasExclusiveToolCall(toolCalls, exclusiveToolNames)
}

func hasExclusiveToolCall(toolCalls []fantasy.ToolCallContent, exclusiveToolNames map[string]bool) bool {
	if len(exclusiveToolNames) == 0 {
		return false
	}
	for _, toolCall := range toolCalls {
		if exclusiveToolNames[toolCall.ToolName] {
			return true
		}
	}
	return false
}

type sessionStartResult struct {
	Chat database.Chat
}

func applySessionStartResponse(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	chat database.Chat,
	result *chathooks.Result,
) (sessionStartResult, error) {
	if result.GetModelContext() == "" && result.GetUserMessage() == "" {
		return sessionStartResult{Chat: chat}, nil
	}

	eventMessages, err := chathooks.EventMessages(result, chat.LastModelConfigID)
	if err != nil {
		return sessionStartResult{}, err
	}

	var applied sessionStartResult
	err = machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := loadChatForGeneration(ctx, store, input, generationAttemptNotRequired); err != nil {
			return xerrors.Errorf("load chat for session_start response: %w", err)
		}
		if len(eventMessages) > 0 {
			if _, err := tx.CommitStep(chatstate.CommitStepInput{Messages: eventMessages}); err != nil {
				return xerrors.Errorf("insert session_start response messages: %w", err)
			}
		}
		applied.Chat, err = store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("reload chat after session_start response: %w", err)
		}
		return nil
	})
	if err != nil {
		return sessionStartResult{}, normalizeTaskTransitionError(err, "apply session_start response")
	}
	return applied, nil
}

func (s *taskStarter) startGenerationSession(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	chat database.Chat,
	messages []database.ChatMessage,
) (result sessionStartResult, dispatched bool, err error) {
	dispatched, complete, err := input.SessionStart.claim(ctx)
	if err != nil {
		return sessionStartResult{}, false, errors.Join(errTaskExpectedExit, xerrors.Errorf("claim session_start: %w", err))
	}
	if !dispatched {
		return sessionStartResult{Chat: chat}, false, nil
	}

	completed := false
	// Re-arm the claim until its response is applied so a replacement task
	// can replay session_start effects.
	defer func() { complete(completed) }()
	response, err := s.server.hooks.Trigger(ctx, chathooks.ChatFor(chat, input.hookTurnID()), chathooks.Message{Source: chathooks.SessionStartSource(messages)}, agenthooks.EventSessionStart, dispatch.CapacityClassGeneration)
	if err != nil {
		return sessionStartResult{}, true, chathooks.GenerationDispatchError(agenthooks.EventSessionStart, err)
	}
	result, err = applySessionStartResponse(ctx, machine, input, chat, response)
	if err != nil {
		return sessionStartResult{}, true, err
	}
	completed = true
	return result, true, nil
}

func (s *taskStarter) StartGeneration(ctx context.Context, input chatWorkerTaskStartInput) error {
	if input.StopNudges == nil {
		input.StopNudges = &stopNudgeTracker{}
	}
	if input.TurnID == uuid.Nil {
		input.TurnID = uuid.New()
	}
	machine := chatstate.NewChatMachine(s.opts.Store, s.opts.Pubsub, input.ChatID)
	for {
		chat, messages, err := loadGenerationState(ctx, machine, input)
		if err != nil {
			return xerrors.Errorf("load generation state: %w", err)
		}
		if s.server.hooks.Enabled() {
			result, dispatched, err := s.startGenerationSession(ctx, machine, input, chat, messages)
			if err != nil {
				if errors.Is(err, errTaskExpectedExit) {
					return err
				}
				return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
			}
			if dispatched {
				input.HistoryVersion = result.Chat.HistoryVersion
				continue
			}
		}
		prepareInput := generationPrepareInput{
			Chat:     chat,
			Messages: messages,
		}
		prepared, err := retryGenerationPhase(ctx, s, "prepare", func() (generationPrepared, error) {
			return s.server.prepareGeneration(ctx, prepareInput)
		})
		if err != nil {
			if errors.Is(err, errTaskExpectedExit) || errors.Is(err, errTaskRetryable) {
				return xerrors.Errorf("prepare generation: %w", err)
			}
			return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
		}
		cleanup := prepared.Cleanup
		var decision generationDecision
		if input.StopNudges.consume(stopNudgeKey(prepared.Messages)) {
			decision = generationDecision{kind: generationActionGenerateAssistant}
		} else {
			decision, err = retryGenerationPhase(ctx, s, "decide", func() (generationDecision, error) {
				return decideGenerationAction(generationDecisionInput{
					chat:                       prepared.Chat,
					messages:                   prepared.Messages,
					dynamicToolNames:           prepared.DynamicToolNames,
					exclusiveToolNames:         prepared.ExclusiveToolNames,
					stopAfterTools:             prepared.StopAfterTools,
					maxSteps:                   prepared.MaxSteps,
					compactionEnabled:          prepared.Compaction != nil,
					compactionNeeded:           prepared.Compaction != nil && prepared.Compaction.Required,
					compactionThresholdPercent: generationCompactionThreshold(prepared.Compaction),
					compactionContextLimit:     generationCompactionContextLimit(prepared.Compaction),
				})
			})
		}
		if err != nil {
			cleanup()
			if errors.Is(err, errTaskExpectedExit) || errors.Is(err, errTaskRetryable) {
				return xerrors.Errorf("decide generation: %w", err)
			}
			if errors.Is(err, errCompactionStillOverLimit) && prepared.Compaction != nil {
				metricProvider, metricModel := compactionMetricIdentity(prepared.Compaction)
				s.server.metrics.RecordCompaction(
					metricProvider,
					metricModel,
					false,
					errCompactionStillOverLimit,
				)
			}
			return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
		}

		var actionErr error
		switch decision.kind {
		case generationActionEnterRequiresAction:
			cleanup()
			return s.enterRequiresAction(ctx, machine, input)
		case generationActionFinishTurn:
			cleanup()
			return s.finishGenerationTurn(ctx, machine, input, decision, generationAttemptNotRequired)
		case generationActionGenerateAssistant:
			actionErr = s.generateAssistant(ctx, machine, input, prepared)
		case generationActionExecuteLocalTools:
			actionErr = s.executeLocalTools(ctx, machine, input, prepared, decision)
		case generationActionCompact:
			actionErr = s.generateCompaction(ctx, machine, input, prepared, compactionSourceForDecision(decision))
		default:
			return s.finishGenerationError(ctx, machine, input, xerrors.Errorf("unknown generation action %q", decision.kind), generationAttemptNotRequired)
		}
		cleanup()
		if actionErr == nil {
			return nil
		}
		// Task cancellation is handled by the runner, not here.
		if ctx.Err() != nil && errors.Is(actionErr, context.Canceled) {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("generation action: %w", actionErr), ctx.Err())
		}
		if errors.Is(actionErr, chatloop.ErrInterrupted) {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("generation action: %w", actionErr))
		}
		if errors.Is(actionErr, errTaskExpectedExit) {
			return xerrors.Errorf("generation action: %w", actionErr)
		}
		classified := chaterror.Classify(actionErr)
		if classified.Retryable {
			action := decision.kind
			decision, err := s.recordGenerationRetry(ctx, machine, input, classified)
			if err != nil {
				return xerrors.Errorf("record generation retry: %w", err)
			}
			if decision.retry {
				s.opts.Logger.Warn(ctx, "chat generation retrying",
					slog.F("chat_id", input.ChatID),
					slog.F("worker_id", input.WorkerID),
					slog.F("action", action),
					slog.F("generation_attempt", decision.generationAttempt),
					slog.F("delay", decision.delay),
					slog.F("error_kind", classified.Kind),
					slog.F("provider", classified.Provider),
					slog.F("status_code", classified.StatusCode),
					slogError(actionErr),
				)
				if err := s.waitGenerationRetry(ctx, decision.delay); err != nil {
					return xerrors.Errorf("wait generation retry: %w", err)
				}
				continue
			}
			return s.finishGenerationError(ctx, machine, input, actionErr, requireGenerationAttempt(decision.generationAttempt))
		}
		return s.finishGenerationError(ctx, machine, input, actionErr, generationAttemptNotRequired)
	}
}

func loadGenerationState(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
) (database.Chat, []database.ChatMessage, error) {
	var chat database.Chat
	var messages []database.ChatMessage
	err := machine.ReadLock(ctx, func(store database.Store) error {
		loadedChat, err := loadChatForTask(ctx, store, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true})
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
		return database.Chat{}, nil, normalizeTaskInfrastructureError(err, "lock chat for generation")
	}
	return chat, messages, nil
}

func (*taskStarter) recordGenerationRetry(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	classified chaterror.ClassifiedError,
) (generationRetryDecision, error) {
	var decision generationRetryDecision
	var payload *codersdk.ChatStreamRetry
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		chat, err := loadChatForTask(ctx, store, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true})
		if err != nil {
			return xerrors.Errorf("load chat for task: %w", err)
		}
		decision.generationAttempt = chat.GenerationAttempt
		if chat.GenerationAttempt <= 0 || chat.GenerationAttempt >= int64(chatretry.MaxAttempts) {
			decision.retry = false
			return errRetryStateDecisionOnly
		}

		attempt := int(chat.GenerationAttempt)
		delay := chatretry.Delay(attempt - 1)
		if classified.RetryAfter > delay {
			delay = classified.RetryAfter
		}
		decision.retry = true
		decision.delay = delay

		payload = chaterror.StreamRetryPayload(attempt, delay, classified)
		if payload == nil {
			return errRetryStateDecisionOnly
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return xerrors.Errorf("marshal retry state: %w", err)
		}
		_, err = tx.RecordRetryState(chatstate.RecordRetryStateInput{
			RetryState: pqtype.NullRawMessage{RawMessage: encoded, Valid: true},
		})
		if err != nil {
			return xerrors.Errorf("record retry state: %w", err)
		}
		return nil
	})
	if errors.Is(err, errRetryStateDecisionOnly) {
		return decision, nil
	}
	if err != nil {
		return generationRetryDecision{}, normalizeTaskTransitionError(err, "record retry state")
	}
	return decision, nil
}

func (s *taskStarter) waitGenerationRetry(ctx context.Context, delay time.Duration) error {
	timer := s.opts.Clock.NewTimer(delay, "chatworker", "generation-retry")
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("wait generation retry: %w", ctx.Err()))
	}
}

const (
	// generationPhaseMaxAttempts bounds how many times prepareGeneration
	// and decideGenerationAction run before the turn finishes with an
	// error. Both phases are retried because prepareGeneration performs
	// I/O (DB reads, MCP connects, workspace dials) that can fail
	// transiently.
	generationPhaseMaxAttempts = 3
	// generationPhaseBaseBackoff is the delay before the first retry. It
	// doubles on each subsequent attempt.
	generationPhaseBaseBackoff = 200 * time.Millisecond
)

func generationPhaseBackoff(attempt int) time.Duration {
	d := generationPhaseBaseBackoff
	for range attempt {
		d *= 2
	}
	return d
}

// retryGenerationPhase runs fn up to generationPhaseMaxAttempts times. It
// returns early on success or on a terminal error (see terminalGeneration).
// Non-terminal errors are retried with exponential backoff. Context
// cancellation returns errTaskExpectedExit so shutdown does not write an
// error state. Task timeouts return a retryable task error so the runner can
// start a fresh attempt. When every attempt fails, the last error is returned.
func retryGenerationPhase[T any](ctx context.Context, starter *taskStarter, phase string, fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < generationPhaseMaxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		if isTerminalGeneration(err) {
			return zero, xerrors.Errorf("retryGenerationPhase terminal error: %w", err)
		}
		if ctx.Err() != nil {
			return zero, errors.Join(errTaskExpectedExit, xerrors.Errorf("retryGenerationPhase %s: %w", phase, ctx.Err()))
		}
		lastErr = err
		if attempt < generationPhaseMaxAttempts-1 {
			delay := generationPhaseBackoff(attempt)
			starter.opts.Logger.Warn(ctx, "chat generation phase retrying",
				slog.F("phase", phase),
				slog.F("attempt", attempt+1),
				slog.F("max_attempts", generationPhaseMaxAttempts),
				slog.F("delay", delay),
				slogError(err),
			)
			if waitErr := starter.waitGenerationPhaseBackoff(ctx, delay); waitErr != nil {
				return zero, waitErr
			}
		}
	}
	return zero, lastErr
}

func (s *taskStarter) waitGenerationPhaseBackoff(ctx context.Context, delay time.Duration) error {
	timer := s.opts.Clock.NewTimer(delay, "chatworker", "generation-phase-retry")
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("wait generation phase backoff: %w", ctx.Err()))
	}
}

func (s *taskStarter) generateAssistant(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	prepared generationPrepared,
) error {
	attempt, err := s.beginGenerationAttempt(ctx, machine, input)
	if err != nil {
		return xerrors.Errorf("begin generation attempt: %w", err)
	}
	defer attempt.closeEpisode()
	runCtx := input.DebugTurn.Ensure(ctx, prepared.Chat, prepared.Debug)
	outcome, err := chatloop.GenerateAssistant(runCtx, chatloop.GenerateAssistantOptions{
		Model:                prepared.Model.LanguageModel(),
		ErrorProvider:        prepared.ResolvedProvider,
		Messages:             prepared.Prompt,
		Tools:                prepared.Tools,
		ActiveTools:          prepared.ActiveTools,
		ProviderTools:        prepared.ProviderTools,
		ContextLimitFallback: prepared.ContextLimitFallback,
		CallTemplate:         prepared.CallTemplate,
		PublishMessagePart:   attempt.publish,
		OnModelStreamStart:   attempt.startModelInvocation,
		Logger:               s.opts.Logger,
		Clock:                s.opts.Clock,
		Metrics:              s.server.metrics,
	})
	if err != nil {
		return xerrors.Errorf("generate assistant: %w", err)
	}
	if len(outcome.Step.Content) == 0 {
		return s.finishGenerationTurn(ctx, machine, input, generationDecision{kind: generationActionFinishTurn, finishReason: generationFinishReasonComplete}, requireGenerationAttempt(attempt.number))
	}
	preflight, err := s.admitStepToolCalls(ctx, input, prepared, outcome.Step.Content)
	if err != nil {
		return err
	}
	outcome.Step.Content = chathooks.ApplyAdmittedToolCalls(outcome.Step.Content, preflight)
	messages, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		modelConfigID:          prepared.ModelConfigID,
		step:                   stepDataFromPersisted(outcome.Step),
		toolNameToConfigID:     prepared.ToolNameToConfigID,
		logger:                 s.opts.Logger,
		contentVersion:         chatprompt.CurrentContentVersion,
		hookRewrittenToolCalls: preflight.Overrides,
	})
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	messages, err = appendHookResultMessages(messages, preflight.Results, prepared.ModelConfigID)
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	return s.commitGenerationStep(ctx, machine, input, attempt.number, generationActionGenerateAssistant, messages, generationCommitHooks{})
}

func (s *taskStarter) admitStepToolCalls(
	ctx context.Context,
	input chatWorkerTaskStartInput,
	prepared generationPrepared,
	content []fantasy.Content,
) (chathooks.PreToolUseExecutionResult, error) {
	if !s.server.hooks.Enabled() {
		return chathooks.PreToolUseExecutionResult{}, nil
	}
	toolCalls := chathooks.PendingToolCalls(content)
	if len(toolCalls) == 0 || exclusiveBatchRejected(toolCalls, prepared.ExclusiveToolNames) {
		return chathooks.PreToolUseExecutionResult{}, nil
	}
	// An admission error discards the whole batch before it can be
	// committed, so its find_tools calls would otherwise never reach
	// the executeLocalTools counter; count them at each error exit.
	countBatch := func() {
		if !prepared.BuiltinToolNames[chattool.FindToolsName] {
			return
		}
		for _, toolCall := range toolCalls {
			if toolCall.ToolName == chattool.FindToolsName {
				s.server.metrics.FindToolsCallsTotal.Inc()
			}
		}
	}
	// Check the full batch first: a call removed below still occupies its ID
	// in the step, so filtering before this would hide the collision.
	if err := chathooks.RejectDuplicateToolUseIDs(toolCalls); err != nil {
		countBatch()
		return chathooks.PreToolUseExecutionResult{}, chathooks.GenerationDispatchError(agenthooks.EventPreToolUse, err)
	}
	unambiguous, ambiguous := partitionAmbiguousToolCalls(prepared, toolCalls)
	preflight, err := s.server.hooks.PreflightPendingToolCalls(ctx, chathooks.ChatFor(prepared.Chat, input.hookTurnID()), unambiguous)
	if err != nil {
		countBatch()
		return chathooks.PreToolUseExecutionResult{}, chathooks.GenerationDispatchError(agenthooks.EventPreToolUse, err)
	}
	if err := validateOverriddenToolInputs(prepared, preflight); err != nil {
		countBatch()
		return chathooks.PreToolUseExecutionResult{}, chathooks.GenerationDispatchError(agenthooks.EventPreToolUse, err)
	}
	preflight.Denied = append(preflight.Denied, ambiguous...)
	// Calls denied at admission persist synthetic results with the
	// assistant step, so they never surface as unresolved calls where
	// executeLocalTools counts find_tools invocations; count them here.
	if prepared.BuiltinToolNames[chattool.FindToolsName] {
		for _, result := range preflight.Denied {
			if result.ToolName == chattool.FindToolsName {
				s.server.metrics.FindToolsCallsTotal.Inc()
			}
		}
	}
	return preflight, nil
}

func (s *taskStarter) executeLocalTools(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	prepared generationPrepared,
	decision generationDecision,
) error {
	allowed := decision.localToolCalls
	var denied []fantasy.ToolResultContent
	if !exclusiveBatchRejected(decision.localToolCalls, prepared.ExclusiveToolNames) {
		allowed, denied = partitionAmbiguousToolCalls(prepared, decision.localToolCalls)
	}
	// find_tools calls are counted here, at the single point every
	// model-emitted call passes through, because rejections upstream of
	// the tool (partition denials, hook denials, exclusive-policy
	// batches) never reach its handler or OnCall.
	if prepared.BuiltinToolNames[chattool.FindToolsName] {
		for _, toolCall := range decision.localToolCalls {
			if toolCall.ToolName == chattool.FindToolsName {
				s.server.metrics.FindToolsCallsTotal.Inc()
			}
		}
	}
	attempt, err := s.beginGenerationAttempt(ctx, machine, input)
	if err != nil {
		return xerrors.Errorf("beginGenerationAttempt: %w", err)
	}
	defer attempt.closeEpisode()
	provider := ""
	modelName := ""
	if prepared.Model.Valid() {
		provider = prepared.Model.Provider()
		modelName = prepared.Model.ModelID()
	}
	var outcome chatloop.ToolExecutionOutcome
	var spawnDispatchErr error
	if len(allowed) > 0 {
		outcome, err = chatloop.ExecuteLocalTools(ctx, chatloop.ExecuteLocalToolsOptions{
			Tools:              prepared.Tools,
			ActiveTools:        prepared.ActiveTools,
			AllowInactiveTools: prepared.AllowInactiveTools,
			ProviderTools:      prepared.ProviderTools,
			ToolCalls:          allowed,
			ObservedToolCalls:  decision.localToolCalls,
			ExclusiveToolNames: prepared.ExclusiveToolNames,
			BuiltinToolNames:   prepared.BuiltinToolNames,
			ModelProvider:      provider,
			ModelName:          modelName,
			ContextLimit:       prepared.ContextLimitFallback,
			ToolNameAliases:    subagentToolNameAliases,
			PublishMessagePart: attempt.publish,
			Logger:             s.opts.Logger,
			Metrics:            s.server.metrics,
			Clock:              s.opts.Clock,
		})
		if err != nil {
			return xerrors.Errorf("execute local tools: %w", err)
		}
		// Subagent spawn admission dispatches user_prompt_submit inside
		// the tool run; its failure surfaces as a tool result error. The
		// step still commits so a sibling tool that already ran keeps its
		// result and is not re-executed, and the turn fails afterwards.
		if hookErr := chathooks.DispatchFailureFromResults(outcome.Step.Content); hookErr != nil {
			spawnDispatchErr = chathooks.GenerationDispatchError(agenthooks.EventUserPromptSubmit, hookErr)
		}
	}
	postResults, postDispatchErr := s.server.hooks.PostToolUseResults(ctx, chathooks.ChatFor(prepared.Chat, input.hookTurnID()), outcome.Step.Content)
	for _, result := range denied {
		outcome.Step.Content = append(outcome.Step.Content, result)
	}
	chathooks.RestoreToolCallOrder(outcome.Step.Content, decision.localToolCalls)
	messages, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		modelConfigID:      prepared.ModelConfigID,
		step:               stepDataFromPersisted(outcome.Step),
		toolNameToConfigID: prepared.ToolNameToConfigID,
		logger:             s.opts.Logger,
		contentVersion:     chatprompt.CurrentContentVersion,
	})
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	messages, err = appendHookResultMessages(messages, postResults, prepared.ModelConfigID)
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	var postCommitErr error
	switch {
	case spawnDispatchErr != nil:
		// Spawn admission is the causal root: post_tool_use only ran
		// because the batch reached execution at all.
		postCommitErr = spawnDispatchErr
		if postDispatchErr != nil {
			s.opts.Logger.Warn(ctx, "post_tool_use hook dispatch failed alongside spawn admission",
				slog.F("chat_id", input.ChatID),
				slog.F("worker_id", input.WorkerID),
				slog.Error(postDispatchErr),
			)
		}
	case postDispatchErr != nil:
		postCommitErr = chathooks.GenerationDispatchError(agenthooks.EventPostToolUse, postDispatchErr)
	}
	return s.commitGenerationStep(ctx, machine, input, attempt.number, generationActionExecuteLocalTools, messages, generationCommitHooks{
		PostCommitError: postCommitErr,
	})
}

// compactionSourceForDecision maps a compact decision to the
// compaction source recorded in the summary messages. Manual
// requests also force the compaction past the usage-threshold gates.
func compactionSourceForDecision(decision generationDecision) chatloop.CompactionSource {
	if decision.forced {
		return chatloop.CompactionSourceManual
	}
	return chatloop.CompactionSourceAutomatic
}

func (s *taskStarter) generateCompaction(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	prepared generationPrepared,
	source chatloop.CompactionSource,
) error {
	attempt, err := s.beginGenerationAttempt(ctx, machine, input)
	if err != nil {
		return xerrors.Errorf("beginGenerationAttempt: %w", err)
	}
	defer attempt.closeEpisode()
	if prepared.Compaction == nil {
		return s.finishGenerationError(ctx, machine, input, xerrors.New("compaction action missing options"), requireGenerationAttempt(attempt.number))
	}
	compactionOpts := prepared.Compaction.Options
	metricProvider, metricModel := compactionMetricIdentity(prepared.Compaction)
	if override := prepared.Compaction.Override; override != nil {
		// Errors are hard failures: a usable override that cannot be
		// constructed must fail the generation visibly instead of silently
		// compacting with the chat model.
		overrideModel, err := s.server.resolveModelCall(ctx, compactionOverrideSpec(prepared.Chat, override.Config, prepared.ModelBuildOptions))
		if err != nil {
			return xerrors.Errorf("build compaction model override: %w", err)
		}
		logger := s.server.logger.With(
			slog.F("chat_id", prepared.Chat.ID),
			slog.F("owner_id", prepared.Chat.OwnerID),
		)
		compactionOpts.Model = overrideModel.model.LanguageModel()
		compactionOpts.ResolvedProvider = overrideModel.resolvedProvider
		compactionOpts.ResolvedModel = overrideModel.resolvedModel
		compactionOpts.ModelConfigID = overrideModel.dbConfig.ID
		compactionOpts.SummaryCall = overrideModel.newCall(compactionSummaryOverrides(false))
		compactionOpts.Messages = sanitizeCompactionPrompt(
			ctx,
			logger,
			compactionOpts.Messages,
			overrideModel.model,
			prepared.Compaction.ChatModelConfig,
			overrideModel.dbConfig,
		)
	}
	preResult, err := s.server.hooks.Trigger(ctx, chathooks.ChatFor(prepared.Chat, input.hookTurnID()), chathooks.Message{}, agenthooks.EventPreCompact, dispatch.CapacityClassGeneration)
	if err != nil {
		return chathooks.GenerationDispatchError(agenthooks.EventPreCompact, err)
	}
	compactionOpts.SummaryHint = preResult.GetModelContext()
	compactionOpts.PublishMessagePart = attempt.publish
	compactionOpts.OnModelStreamStart = attempt.startModelInvocation
	compactionOpts.Source = source
	compactionOpts.Force = source == chatloop.CompactionSourceManual
	compactionOpts.Clock = s.opts.Clock
	// Attach the turn debug run so the compaction call records a child
	// debug run; without it startCompactionDebugRun finds no parent and
	// skips debug instrumentation entirely.
	runCtx := input.DebugTurn.Ensure(ctx, prepared.Chat, prepared.Debug)
	outcome, err := chatloop.GenerateCompaction(runCtx, compactionOpts)
	if err != nil {
		s.server.metrics.RecordCompaction(metricProvider, metricModel, false, err)
		return xerrors.Errorf("generate compaction: %w", err)
	}
	if strings.TrimSpace(outcome.SystemSummary) == "" || strings.TrimSpace(outcome.SummaryReport) == "" {
		err := xerrors.New("compaction produced no summary")
		s.server.metrics.RecordCompaction(metricProvider, metricModel, false, err)
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	messages, err := buildCompactionMessages(buildCompactionMessagesInput{
		modelConfigID:  prepared.ModelConfigID,
		toolCallID:     compactionOpts.ToolCallID,
		toolName:       compactionOpts.ToolName,
		compaction:     compactionOutcome(outcome),
		contentVersion: chatprompt.CurrentContentVersion,
	})
	if err != nil {
		s.server.metrics.RecordCompaction(metricProvider, metricModel, false, err)
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	// The summary hint already consumed the pre_compact model context.
	persistedPreResult := &chathooks.Result{UserMessage: preResult.GetUserMessage()}
	commitMessages, err := applyHookResultMessages(stepMessagesForCommit{
		Messages:                 messages.Messages,
		VisibleIndexes:           visibleMessageIndexes(messages.Messages),
		ConsumeCompactionRequest: true,
	}, []*chathooks.Result{persistedPreResult}, prepared.ModelConfigID)
	if err != nil {
		s.server.metrics.RecordCompaction(metricProvider, metricModel, false, err)
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	// Hook effects and fail-closed errors must commit atomically with
	// compaction; a separate commit races the runner and can be dropped
	// on crash.
	postResult, postDispatchErr := s.server.hooks.Trigger(ctx, chathooks.ChatFor(prepared.Chat, input.hookTurnID()), chathooks.Message{}, agenthooks.EventPostCompact, dispatch.CapacityClassGeneration)
	var postCommitErr error
	if postDispatchErr != nil {
		postCommitErr = chathooks.GenerationDispatchError(agenthooks.EventPostCompact, postDispatchErr)
	} else {
		commitMessages, err = appendHookResultMessages(commitMessages, []*chathooks.Result{postResult}, prepared.ModelConfigID)
		if err != nil {
			s.server.metrics.RecordCompaction(metricProvider, metricModel, false, err)
			return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
		}
	}
	err = s.commitGenerationStep(ctx, machine, input, attempt.number, generationActionCompact, commitMessages, generationCommitHooks{
		PostCommitError: postCommitErr,
	})
	s.server.metrics.RecordCompaction(metricProvider, metricModel, err == nil, err)
	if err != nil {
		return xerrors.Errorf("commit compaction step: %w", err)
	}
	return nil
}

// compactionMetricIdentity returns the provider/model labels for compaction
// metrics. Override labels come from prepare-time resolution so events
// recorded before the override client is built (still-over-limit) match
// the compact action's own events.
func compactionMetricIdentity(compaction *generationCompaction) (provider, model string) {
	if compaction.Override != nil {
		return compaction.Override.ResolvedProvider, compaction.Override.ResolvedModel
	}
	return compactionProvider(compaction.Options), compactionModel(compaction.Options)
}

func compactionProvider(opts chatloop.GenerateCompactionOptions) string {
	if opts.Model == nil {
		return ""
	}
	return opts.Model.Provider()
}

func compactionModel(opts chatloop.GenerateCompactionOptions) string {
	if opts.Model == nil {
		return ""
	}
	return opts.Model.Model()
}

// generationAttempt groups the state a generation action needs after
// recording a new attempt.
type generationAttempt struct {
	number int64
	// publish streams a message part into the attempt's buffer episode.
	publish func(codersdk.ChatMessageRole, codersdk.ChatMessagePart)
	// startModelInvocation marks the start of the attempt's billable
	// model invocation window on the buffer episode, so an interrupt
	// can bill the window the step would have reported. It is always
	// non-nil when beginGenerationAttempt succeeds.
	startModelInvocation func()
	// closeEpisode closes the attempt's buffer episode. It is always
	// non-nil when beginGenerationAttempt succeeds.
	closeEpisode func()
}

func (s *taskStarter) beginGenerationAttempt(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
) (generationAttempt, error) {
	var attempt int64
	var committed database.Chat
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := loadChatForTask(ctx, store, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true}); err != nil {
			return xerrors.Errorf("load chat for task: %w", err)
		}
		result, err := tx.RecordGenerationAttempt(chatstate.RecordGenerationAttemptInput{})
		if err != nil {
			return xerrors.Errorf("tx.RecordGenerationAttempt: %w", err)
		}
		attempt = result.GenerationAttempt
		committed, err = store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		return nil
	})
	if err != nil {
		return generationAttempt{}, normalizeTaskTransitionError(err, "record generation attempt")
	}
	key := messagepartbuffer.Key{
		ChatID:            input.ChatID,
		HistoryVersion:    committed.HistoryVersion,
		GenerationAttempt: attempt,
	}
	if err := s.opts.MessagePartBuffer.CreateEpisode(key); err != nil && ctx.Err() == nil {
		return generationAttempt{}, taskRetryableError{err: xerrors.Errorf("create message part episode: %w", err)}
	}
	return generationAttempt{
		number: attempt,
		publish: func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
			_ = s.opts.MessagePartBuffer.AddPart(key, role, part)
		},
		startModelInvocation: func() {
			_ = s.opts.MessagePartBuffer.StartModelInvocation(key)
		},
		closeEpisode: func() {
			_ = s.opts.MessagePartBuffer.CloseEpisode(key)
		},
	}, nil
}

type generationCommitHooks struct {
	PostCommitError error
}

func (s *taskStarter) commitGenerationStep(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	attempt int64,
	kind generationActionKind,
	messages stepMessagesForCommit,
	commitHooks generationCommitHooks,
) error {
	if len(messages.Messages) == 0 {
		if commitHooks.PostCommitError != nil {
			return s.finishGenerationError(ctx, machine, input, commitHooks.PostCommitError, requireGenerationAttempt(attempt))
		}
		return s.finishGenerationTurn(ctx, machine, input, generationDecision{kind: generationActionFinishTurn, finishReason: generationFinishReasonComplete}, requireGenerationAttempt(attempt))
	}
	failClosed := commitHooks.PostCommitError != nil
	var postCommitLastError pqtype.NullRawMessage
	var postCommitMessage string
	if commitHooks.PostCommitError != nil {
		classified := chaterror.Classify(commitHooks.PostCommitError)
		s.opts.Logger.Warn(ctx, "chat generation failed",
			slog.F("chat_id", input.ChatID),
			slog.F("worker_id", input.WorkerID),
			slog.F("generation_attempt", input.GenerationAttempt),
			slog.F("error_kind", classified.Kind),
			slog.F("provider", classified.Provider),
			slog.F("status_code", classified.StatusCode),
			slog.F("retryable", classified.Retryable),
			slog.Error(commitHooks.PostCommitError),
		)
		postCommitLastError, postCommitMessage = generationLastError(commitHooks.PostCommitError)
	}
	var committed database.Chat
	insertedMessages := []runnerActionMessage{}
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := loadChatForGeneration(ctx, store, input, requireGenerationAttempt(attempt)); err != nil {
			return xerrors.Errorf("load chat for generation: %w", err)
		}
		commitResult, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages:                 messages.Messages,
			ConsumeCompactionRequest: messages.ConsumeCompactionRequest,
		})
		if err != nil {
			return xerrors.Errorf("tx.CommitStep: %w", err)
		}
		inserted := commitResult.InsertedMessages
		// The fail-closed hook error must commit atomically with the
		// step; a separate commit races the runner and can be dropped
		// on crash.
		if failClosed {
			if _, err := tx.FinishError(chatstate.FinishErrorInput{LastError: postCommitLastError}); err != nil {
				return xerrors.Errorf("tx.FinishError: %w", err)
			}
		}
		insertedMessages = make([]runnerActionMessage, 0, len(inserted))
		for _, msg := range inserted {
			insertedMessages = append(insertedMessages, runnerActionMessage{ID: msg.ID, Role: codersdk.ChatMessageRole(msg.Role)})
		}
		loadedChat, err := store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		committed = loadedChat
		return nil
	})
	if err != nil {
		return normalizeTaskTransitionError(err, "commit generation step")
	}
	if failClosed {
		input.DebugTurn.RecordOutcome(chatdebug.StatusError)
		postCommitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), postCommitWatchPublishTimeout)
		defer cancel()
		if err := s.publishWatchAndRoute(postCommitCtx, committed, codersdk.ChatWatchEventKindStatusChange); err != nil {
			return xerrors.Errorf("publish watch and route: %w", err)
		}
		return s.afterGenerationOutcome(postCommitCtx, generationOutcome{
			Chat:           committed,
			Kind:           runnerActionKindFinishError,
			WatchEventKind: codersdk.ChatWatchEventKindStatusChange,
			LastError:      postCommitMessage,
		})
	}
	s.routeStateHint(ctx, stateUpdateFromChat(committed))
	return s.afterGenerationOutcome(ctx, generationOutcome{
		Chat:             committed,
		Kind:             runnerActionKind(kind),
		InsertedMessages: insertedMessages,
	})
}

func (s *taskStarter) enterRequiresAction(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
) error {
	var committed database.Chat
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := loadChatForTask(ctx, store, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true}); err != nil {
			return xerrors.Errorf("load chat for task: %w", err)
		}
		if _, err := tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{}); err != nil {
			return xerrors.Errorf("tx.EnterRequiresAction: %w", err)
		}
		chat, err := store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		committed = chat
		return nil
	})
	if err != nil {
		return normalizeTaskTransitionError(err, "enter requires action")
	}
	if err := s.publishWatchAndRoute(ctx, committed, codersdk.ChatWatchEventKindActionRequired); err != nil {
		return xerrors.Errorf("publish watch and route: %w", err)
	}
	return s.afterGenerationOutcome(ctx, generationOutcome{
		Chat:           committed,
		Kind:           runnerActionKindEnterRequiresAction,
		WatchEventKind: codersdk.ChatWatchEventKindActionRequired,
	})
}

// generationAttemptFence controls whether a terminal generation
// transition also verifies that the chat's GenerationAttempt counter
// matches an expected value.
type generationAttemptFence struct {
	required bool
	attempt  int64
}

// generationAttemptNotRequired skips the generation attempt fence; only the
// running-task fence is verified.
var generationAttemptNotRequired = generationAttemptFence{}

// requireGenerationAttempt returns a fence that also verifies the chat's
// generation attempt matches the given value.
func requireGenerationAttempt(attempt int64) generationAttemptFence {
	return generationAttemptFence{required: true, attempt: attempt}
}

// loadChatForGeneration loads the chat and verifies the running-task fence,
// additionally verifying the generation attempt fence when required.
func loadChatForGeneration(
	ctx context.Context,
	store database.Store,
	input chatWorkerTaskStartInput,
	fence generationAttemptFence,
) (database.Chat, error) {
	chat, err := loadChatForTask(ctx, store, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true})
	if err != nil {
		return database.Chat{}, err
	}
	if fence.required && chat.GenerationAttempt != fence.attempt {
		return database.Chat{}, errors.Join(errTaskExpectedExit, xerrors.Errorf("generation fence mismatch: %d != %d", chat.GenerationAttempt, fence.attempt))
	}
	return chat, nil
}

// recordGenerationFinishFailure records an error outcome on the debug turn
// when a terminal generation transition fails, so the debug run is not
// finalized as interrupted when work was actually done. It skips expected
// exits (fence lost, chat deleted) where another task owns the turn, and
// retryable errors where a task retry will record the real outcome.
func recordGenerationFinishFailure(turn *runnerDebugTurn, err error) {
	if errors.Is(err, errTaskExpectedExit) || errors.Is(err, errTaskRetryable) {
		return
	}
	turn.RecordOutcome(chatdebug.StatusError)
}

func (s *taskStarter) completeGenerationTurn(
	ctx context.Context,
	input chatWorkerTaskStartInput,
	committed database.Chat,
	promotedMessageID int64,
) error {
	input.StopNudges.reset()
	input.DebugTurn.RecordOutcome(chatdebug.StatusCompleted)
	watchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), postCommitWatchPublishTimeout)
	defer cancel()
	if err := s.publishWatchWithRetry(watchCtx, committed, codersdk.ChatWatchEventKindStatusChange); err != nil {
		return xerrors.Errorf("publish watch and route: %w", err)
	}
	if err := s.afterGenerationOutcome(ctx, generationOutcome{
		Chat:              committed,
		Kind:              runnerActionKindFinishTurn,
		WatchEventKind:    codersdk.ChatWatchEventKindStatusChange,
		PromotedMessageID: promotedMessageID,
	}); err != nil {
		return xerrors.Errorf("after generation outcome: %w", err)
	}
	s.routeStateHint(ctx, stateUpdateFromChat(committed))
	return nil
}

func (s *taskStarter) finishGenerationTurnWithoutHook(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	decision generationDecision,
	fence generationAttemptFence,
) error {
	var committed database.Chat
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := loadChatForGeneration(ctx, store, input, fence); err != nil {
			return xerrors.Errorf("load chat for generation: %w", err)
		}
		finishResult, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		if err != nil {
			return xerrors.Errorf("tx.FinishTurn: %w", err)
		}
		if finishResult.PromotedMessage != nil {
			decision.promotedMessageID = finishResult.PromotedMessage.ID
		}
		committed = finishResult.Chat
		return nil
	})
	if err != nil {
		err := normalizeTaskTransitionError(err, "finish generation turn")
		recordGenerationFinishFailure(input.DebugTurn, err)
		return err
	}
	return s.completeGenerationTurn(ctx, input, committed, decision.promotedMessageID)
}

func (s *taskStarter) finishGenerationTurn(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	decision generationDecision,
	fence generationAttemptFence,
) error {
	if !s.server.hooks.Enabled() {
		return s.finishGenerationTurnWithoutHook(ctx, machine, input, decision, fence)
	}
	var chat database.Chat
	var messages []database.ChatMessage
	err := machine.ReadLock(ctx, func(store database.Store) error {
		loadedChat, err := loadChatForGeneration(ctx, store, input, fence)
		if err != nil {
			return xerrors.Errorf("load chat for stop hook: %w", err)
		}
		loadedMessages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  input.ChatID,
			AfterID: 0,
		})
		if err != nil {
			return xerrors.Errorf("load messages for stop hook: %w", err)
		}
		chat = loadedChat
		messages = loadedMessages
		return nil
	})
	if err != nil {
		return normalizeTaskTransitionError(err, "load stop hook state")
	}
	response, err := s.server.hooks.Trigger(ctx, chathooks.ChatFor(chat, input.hookTurnID()), chathooks.Message{}, agenthooks.EventStop, dispatch.CapacityClassGeneration)
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, chathooks.GenerationDispatchError(agenthooks.EventStop, err), fence)
	}
	stopMessages, err := chathooks.EventMessages(response, chat.LastModelConfigID)
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, fence)
	}
	nudgeKey := stopNudgeKey(messages)
	// Prompt conversion drops whitespace-only text parts, so a blank
	// model context would buy a continuation that nudges nothing.
	continueTurn := strings.TrimSpace(response.GetModelContext()) != "" && input.StopNudges.claim(nudgeKey)

	var committed database.Chat
	err = machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := loadChatForGeneration(ctx, store, input, fence); err != nil {
			return xerrors.Errorf("load chat for generation: %w", err)
		}
		if len(stopMessages) > 0 {
			if _, err := tx.CommitStep(chatstate.CommitStepInput{Messages: stopMessages}); err != nil {
				return xerrors.Errorf("commit stop hook messages: %w", err)
			}
		}
		if !continueTurn {
			finishResult, err := tx.FinishTurn(chatstate.FinishTurnInput{})
			if err != nil {
				return xerrors.Errorf("tx.FinishTurn: %w", err)
			}
			if finishResult.PromotedMessage != nil {
				decision.promotedMessageID = finishResult.PromotedMessage.ID
			}
			committed = finishResult.Chat
			return nil
		}
		loadedChat, err := store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		committed = loadedChat
		return nil
	})
	if err != nil {
		if continueTurn {
			input.StopNudges.cancel(nudgeKey)
		}
		err := normalizeTaskTransitionError(err, "finish generation turn")
		recordGenerationFinishFailure(input.DebugTurn, err)
		return err
	}
	if continueTurn {
		s.routeStateHint(ctx, stateUpdateFromChat(committed))
		return s.afterGenerationOutcome(ctx, generationOutcome{
			Chat: committed,
			Kind: runnerActionKind(generationActionGenerateAssistant),
		})
	}
	return s.completeGenerationTurn(ctx, input, committed, decision.promotedMessageID)
}

func (s *taskStarter) finishGenerationError(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	cause error,
	fence generationAttemptFence,
) error {
	classified := chaterror.Classify(cause)
	// Log the unsanitized cause before persisting so administrators can
	// diagnose the failure even when the classified user-facing message
	// omits the underlying reason, and even if the persist below fails.
	s.opts.Logger.Warn(ctx, "chat generation failed",
		slog.F("chat_id", input.ChatID),
		slog.F("worker_id", input.WorkerID),
		slog.F("generation_attempt", input.GenerationAttempt),
		slog.F("error_kind", classified.Kind),
		slog.F("provider", classified.Provider),
		slog.F("status_code", classified.StatusCode),
		slog.F("retryable", classified.Retryable),
		slog.Error(cause),
	)
	lastError, message := generationLastError(cause)
	var committed database.Chat
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := loadChatForGeneration(ctx, store, input, fence); err != nil {
			return xerrors.Errorf("load chat for generation: %w", err)
		}
		if _, err := tx.FinishError(chatstate.FinishErrorInput{LastError: lastError}); err != nil {
			return xerrors.Errorf("tx.FinishError: %w", err)
		}
		chat, err := store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		committed = chat
		return nil
	})
	if err != nil {
		err := normalizeTaskTransitionError(err, "finish generation error")
		recordGenerationFinishFailure(input.DebugTurn, err)
		return err
	}
	input.DebugTurn.RecordOutcome(chatdebug.StatusError)
	if err := s.publishWatchAndRoute(ctx, committed, codersdk.ChatWatchEventKindStatusChange); err != nil {
		return xerrors.Errorf("publish watch and route: %w", err)
	}
	return s.afterGenerationOutcome(ctx, generationOutcome{
		Chat:           committed,
		Kind:           runnerActionKindFinishError,
		WatchEventKind: codersdk.ChatWatchEventKindStatusChange,
		LastError:      message,
	})
}

func generationLastError(err error) (pqtype.NullRawMessage, string) {
	if err == nil {
		return pqtype.NullRawMessage{}, ""
	}
	classified := chaterror.Classify(err)
	payload := chaterror.TerminalErrorPayload(classified)
	if payload == nil {
		payload = &codersdk.ChatError{Message: err.Error()}
	}
	encoded, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return pqtype.NullRawMessage{}, payload.Message
	}
	return pqtype.NullRawMessage{RawMessage: encoded, Valid: true}, payload.Message
}

func (s *taskStarter) afterGenerationOutcome(ctx context.Context, outcome generationOutcome) error {
	if err := s.server.afterGenerationOutcome(ctx, outcome); err != nil {
		return taskRetryableError{err: xerrors.Errorf("generation post-outcome side effects: %w", err)}
	}
	return nil
}

func stepDataFromPersisted(step chatloop.PersistedStep) stepData {
	return stepData{
		Content:              step.Content,
		Usage:                step.Usage,
		ContextLimit:         step.ContextLimit,
		Runtime:              step.Runtime,
		ToolCallCreatedAt:    step.ToolCallCreatedAt,
		ToolResultCreatedAt:  step.ToolResultCreatedAt,
		ReasoningStartedAt:   step.ReasoningStartedAt,
		ReasoningCompletedAt: step.ReasoningCompletedAt,
	}
}
