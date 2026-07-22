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
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
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

	Model             fantasy.LanguageModel
	Prompt            []fantasy.Message
	Tools             []fantasy.AgentTool
	ActiveTools       []string
	ProviderTools     []chatloop.ProviderTool
	ModelRoute        aiGatewayModelRoute
	ModelBuildOptions modelBuildOptions

	// ResolvedProvider is the configured provider identity used to label
	// user-facing errors. See chatloop.GenerateAssistantOptions.ErrorProvider.
	ResolvedProvider string

	ModelConfigID        uuid.UUID
	ModelConfig          codersdk.ChatModelCallConfig
	ProviderOptions      fantasy.ProviderOptions
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
	kind                    generationActionKind
	localToolCalls          []fantasy.ToolCallContent
	pendingDynamicToolCalls []pendingDynamicToolCall
	finishReason            generationFinishReason
	promotedMessageID       int64
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
			dynamicCalls = nil
		}
		return generationDecision{kind: generationActionExecuteLocalTools, localToolCalls: localCalls, pendingDynamicToolCalls: dynamicCalls}, nil
	}
	if len(dynamicCalls) > 0 {
		return generationDecision{kind: generationActionEnterRequiresAction, pendingDynamicToolCalls: dynamicCalls}, nil
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

func (s *taskStarter) StartGeneration(ctx context.Context, input chatWorkerTaskStartInput) error {
	machine := chatstate.NewChatMachine(s.opts.Store, s.opts.Pubsub, input.ChatID)
	for {
		chat, messages, err := loadGenerationState(ctx, machine, input)
		if err != nil {
			return xerrors.Errorf("load generation state: %w", err)
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
		decision, err := retryGenerationPhase(ctx, s, "decide", func() (generationDecision, error) {
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
		Model:                prepared.Model,
		ErrorProvider:        prepared.ResolvedProvider,
		Messages:             prepared.Prompt,
		Tools:                prepared.Tools,
		ActiveTools:          prepared.ActiveTools,
		ProviderTools:        prepared.ProviderTools,
		ContextLimitFallback: prepared.ContextLimitFallback,
		ModelConfig:          prepared.ModelConfig,
		ProviderOptions:      prepared.ProviderOptions,
		PublishMessagePart:   attempt.publish,
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
	messages, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		modelConfigID:      prepared.ModelConfigID,
		modelCallConfig:    prepared.ModelConfig,
		step:               stepDataFromPersisted(outcome.Step),
		toolNameToConfigID: prepared.ToolNameToConfigID,
		logger:             s.opts.Logger,
		contentVersion:     chatprompt.CurrentContentVersion,
	})
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	return s.commitGenerationStep(ctx, machine, input, attempt.number, generationActionGenerateAssistant, messages)
}

func (s *taskStarter) executeLocalTools(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	prepared generationPrepared,
	decision generationDecision,
) error {
	attempt, err := s.beginGenerationAttempt(ctx, machine, input)
	if err != nil {
		return xerrors.Errorf("beginGenerationAttempt: %w", err)
	}
	defer attempt.closeEpisode()
	provider := ""
	modelName := ""
	if prepared.Model != nil {
		provider = prepared.Model.Provider()
		modelName = prepared.Model.Model()
	}
	outcome, err := chatloop.ExecuteLocalTools(ctx, chatloop.ExecuteLocalToolsOptions{
		Tools:              prepared.Tools,
		ActiveTools:        prepared.ActiveTools,
		ProviderTools:      prepared.ProviderTools,
		ToolCalls:          decision.localToolCalls,
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
	messages, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		modelConfigID:      prepared.ModelConfigID,
		modelCallConfig:    prepared.ModelConfig,
		step:               stepDataFromPersisted(outcome.Step),
		toolNameToConfigID: prepared.ToolNameToConfigID,
		logger:             s.opts.Logger,
		contentVersion:     chatprompt.CurrentContentVersion,
	})
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	return s.commitGenerationStep(ctx, machine, input, attempt.number, generationActionExecuteLocalTools, messages)
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
		overrideModel, err := s.server.buildCompactionOverrideModel(ctx, prepared.Chat, override.Config, prepared.ModelBuildOptions)
		if err != nil {
			return xerrors.Errorf("build compaction model override: %w", err)
		}
		logger := s.server.logger.With(
			slog.F("chat_id", prepared.Chat.ID),
			slog.F("owner_id", prepared.Chat.OwnerID),
		)
		compactionOpts.Model = overrideModel.model
		compactionOpts.ResolvedProvider = overrideModel.resolvedProvider
		compactionOpts.ResolvedModel = overrideModel.resolvedModel
		compactionOpts.ModelConfigID = overrideModel.modelConfig.ID
		compactionOpts.ProviderOptions = overrideModel.providerOptions
		compactionOpts.Messages = sanitizeCompactionPrompt(
			ctx,
			logger,
			compactionOpts.Messages,
			overrideModel.model,
			prepared.Compaction.ChatModelConfig,
			overrideModel.modelConfig,
		)
	}
	compactionOpts.PublishMessagePart = attempt.publish
	compactionOpts.Source = source
	compactionOpts.Force = source == chatloop.CompactionSourceManual
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
	err = s.commitGenerationStep(ctx, machine, input, attempt.number, generationActionCompact, stepMessagesForCommit{
		Messages:                 messages.Messages,
		VisibleIndexes:           visibleMessageIndexes(messages.Messages),
		ConsumeCompactionRequest: true,
	})
	s.server.metrics.RecordCompaction(metricProvider, metricModel, err == nil, err)
	if err != nil {
		return xerrors.Errorf("commit generation step: %w", err)
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
		closeEpisode: func() {
			_ = s.opts.MessagePartBuffer.CloseEpisode(key)
		},
	}, nil
}

func (s *taskStarter) commitGenerationStep(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	attempt int64,
	kind generationActionKind,
	messages stepMessagesForCommit,
) error {
	if len(messages.Messages) == 0 {
		return s.finishGenerationTurn(ctx, machine, input, generationDecision{kind: generationActionFinishTurn, finishReason: generationFinishReasonComplete}, requireGenerationAttempt(attempt))
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
		insertedMessages = make([]runnerActionMessage, 0, len(commitResult.InsertedMessages))
		for _, msg := range commitResult.InsertedMessages {
			insertedMessages = append(insertedMessages, runnerActionMessage{ID: msg.ID, Role: codersdk.ChatMessageRole(msg.Role)})
		}
		committed, err = store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		return nil
	})
	if err != nil {
		return normalizeTaskTransitionError(err, "commit generation step")
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

func (s *taskStarter) finishGenerationTurn(
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
		PromotedMessageID: decision.promotedMessageID,
	}); err != nil {
		return xerrors.Errorf("after generation outcome: %w", err)
	}
	s.routeStateHint(ctx, stateUpdateFromChat(committed))
	return nil
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
