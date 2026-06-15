package chatd

import (
	"context"
	"database/sql"
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
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
)

// generationPrepareInput contains the committed state used to prepare one
// generation action.
type generationPrepareInput struct {
	Chat              database.Chat
	Messages          []database.ChatMessage
	ChainModeDisabled bool
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
	ProviderKeys      chatprovider.ProviderAPIKeys
	ModelRoute        resolvedModelRoute
	ModelBuildOptions modelBuildOptions

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

	// WorkspaceContextEligible reports whether the current turn is allowed
	// by policy to inject workspace context. The decision helper combines
	// this fact with committed chat metadata and history to decide whether
	// the persist_workspace_context action should run.
	WorkspaceContextEligible bool
}

// generationCompaction contains compaction inputs prepared for generation.
type generationCompaction struct {
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

type workspaceContextBuildInput struct {
	Chat           database.Chat
	Messages       []database.ChatMessage
	ActiveAPIKeyID string
}

type workspaceContextBuildResult struct {
	Messages []chatstate.Message
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
	generationActionExecuteLocalTools       generationActionKind = "execute_local_tools"
	generationActionEnterRequiresAction     generationActionKind = "enter_requires_action"
	generationActionFinishTurn              generationActionKind = "finish_turn"
	generationActionCompact                 generationActionKind = "compact"
	generationActionGenerateAssistant       generationActionKind = "generate_assistant"
	generationActionPersistWorkspaceContext generationActionKind = "persist_workspace_context"
)

type generationFinishReason string

const (
	generationFinishReasonStopAfterTool generationFinishReason = "stop_after_tool"
	generationFinishReasonComplete      generationFinishReason = "complete"
	generationFinishReasonMaxSteps      generationFinishReason = "max_steps"
)

type compactionTrigger string

const (
	compactionTriggerRequired         compactionTrigger = "required"
	compactionTriggerAlreadyCompacted compactionTrigger = "already_compacted"
)

var errCompactionStillOverLimit = xerrors.New("compaction left the chat above the compaction limit")

type generationDecision struct {
	kind                    generationActionKind
	localToolCalls          []fantasy.ToolCallContent
	pendingDynamicToolCalls []pendingDynamicToolCall
	finishReason            generationFinishReason
	compactionTrigger       compactionTrigger
	promotedMessageID       int64
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
	chat                     database.Chat
	messages                 []database.ChatMessage
	dynamicToolNames         map[string]bool
	exclusiveToolNames       map[string]bool
	stopAfterTools           map[string]struct{}
	maxSteps                 int
	compactionEnabled        bool
	compactionNeeded         bool
	workspaceContextEligible bool
}

// shouldPersistWorkspaceContext reports whether the committed chat
// state and history indicate that the persistWorkspaceContext
// generation action should run before the next assistant call. The
// decision uses two facts:
//   - chat metadata says a workspace and selected agent are attached;
//   - committed history either has no context-file marker for the
//     currently selected workspace agent, or the latest non-sentinel
//     marker points to a different agent.
//
// The decision is intentionally pure so generation can choose the
// action without dialing the workspace. Once the action commits a
// context-file marker for the agent (with or without content), this
// helper returns false on the next pass and the loop is broken.
func shouldPersistWorkspaceContext(chat database.Chat, messages []database.ChatMessage) bool {
	if !chat.WorkspaceID.Valid || !chat.AgentID.Valid {
		return false
	}
	if hasPersistedContextFileForAgent(messages, chat.AgentID.UUID) {
		return false
	}
	persistedAgentID, found := contextFileAgentIDFromMessages(messages)
	if !found {
		return true
	}
	return persistedAgentID != chat.AgentID.UUID
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
	if input.workspaceContextEligible && shouldPersistWorkspaceContext(input.chat, input.messages) {
		return generationDecision{kind: generationActionPersistWorkspaceContext}, nil
	}
	compactionRequirement := compactionRequirementNotNeeded
	if input.compactionEnabled && input.compactionNeeded {
		compactionRequirement = compactionRequirementNeeded
	}
	switch compactionStatusFromHistory(input.messages, compactionRequirement) {
	case compactionStatusNeeded:
		return generationDecision{kind: generationActionCompact, compactionTrigger: compactionTriggerRequired}, nil
	case compactionStatusAfterCompaction:
		return generationDecision{kind: generationActionGenerateAssistant, compactionTrigger: compactionTriggerAlreadyCompacted}, nil
	case compactionStatusStillOverLimit:
		return generationDecision{}, terminalGeneration(errCompactionStillOverLimit)
	case compactionStatusNotNeeded:
		return generationDecision{kind: generationActionGenerateAssistant}, nil
	default:
		return generationDecision{}, terminalGeneration(xerrors.New("unknown compaction status"))
	}
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
	if s.server == nil {
		return xerrors.New("chatworker: server is required")
	}
	machine := chatstate.NewChatMachine(s.opts.Store, s.opts.Pubsub, input.ChatID)
	chainModeDisabled := false
	for {
		locked, messages, err := loadGenerationState(ctx, machine, input)
		if err != nil {
			return err
		}
		prepareInput := generationPrepareInput{
			Chat:              locked,
			Messages:          messages,
			ChainModeDisabled: chainModeDisabled,
		}
		prepared, err := retryGenerationPhase(ctx, s.waitGenerationPhaseBackoff, func() (generationPrepared, error) {
			return s.server.prepareGeneration(ctx, prepareInput)
		})
		if err != nil {
			if errors.Is(err, errTaskExpectedExit) {
				return errTaskExpectedExit
			}
			return s.finishGenerationError(ctx, machine, input, 0, err, generationAttemptNotRequired)
		}
		cleanup := prepared.Cleanup
		decision, err := retryGenerationPhase(ctx, s.waitGenerationPhaseBackoff, func() (generationDecision, error) {
			return decideGenerationAction(generationDecisionInput{
				chat:                     prepared.Chat,
				messages:                 prepared.Messages,
				dynamicToolNames:         prepared.DynamicToolNames,
				exclusiveToolNames:       prepared.ExclusiveToolNames,
				stopAfterTools:           prepared.StopAfterTools,
				maxSteps:                 prepared.MaxSteps,
				compactionEnabled:        prepared.Compaction != nil,
				compactionNeeded:         prepared.Compaction != nil && prepared.Compaction.Required,
				workspaceContextEligible: prepared.WorkspaceContextEligible,
			})
		})
		if err != nil {
			cleanup()
			if errors.Is(err, errTaskExpectedExit) {
				return errTaskExpectedExit
			}
			return s.finishGenerationError(ctx, machine, input, 0, err, generationAttemptNotRequired)
		}

		var actionErr error
		switch decision.kind {
		case generationActionEnterRequiresAction:
			cleanup()
			return s.enterRequiresAction(ctx, machine, input)
		case generationActionFinishTurn:
			cleanup()
			return s.finishGenerationTurn(ctx, machine, input, 0, decision, generationAttemptNotRequired)
		case generationActionGenerateAssistant:
			actionErr = s.generateAssistant(ctx, machine, input, prepared, decision)
		case generationActionExecuteLocalTools:
			actionErr = s.executeLocalTools(ctx, machine, input, prepared, decision)
		case generationActionCompact:
			actionErr = s.generateCompaction(ctx, machine, input, prepared)
		case generationActionPersistWorkspaceContext:
			actionErr = s.persistWorkspaceContext(ctx, machine, input, prepared.Chat)
		default:
			return s.finishGenerationError(ctx, machine, input, 0, xerrors.Errorf("unknown generation action %q", decision.kind), generationAttemptNotRequired)
		}
		cleanup()
		if actionErr == nil {
			return nil
		}
		if errors.Is(actionErr, errTaskExpectedExit) || errors.Is(actionErr, chatloop.ErrInterrupted) {
			return nil
		}
		if errors.Is(actionErr, context.Canceled) && ctx.Err() != nil {
			return nil
		}
		classified := chaterror.Classify(actionErr)
		if classified.Retryable {
			decision, err := s.recordGenerationRetry(ctx, machine, input, classified)
			if err != nil {
				return err
			}
			if decision.retry {
				if classified.ChainBroken {
					chainModeDisabled = true
				}
				if err := s.waitGenerationRetry(ctx, decision.delay); err != nil {
					return err
				}
				continue
			}
			return s.finishGenerationError(ctx, machine, input, decision.generationAttempt, actionErr, generationAttemptRequired)
		}
		return s.finishGenerationError(ctx, machine, input, 0, actionErr, generationAttemptNotRequired)
	}
}

func loadGenerationState(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
) (database.Chat, []database.ChatMessage, error) {
	var locked database.Chat
	var messages []database.ChatMessage
	err := machine.ReadLock(ctx, func(store database.Store) error {
		chat, err := store.GetChatByID(ctx, input.ChatID)
		if errors.Is(err, sql.ErrNoRows) {
			return errTaskExpectedExit
		}
		if err != nil {
			return xerrors.Errorf("load locked chat: %w", err)
		}
		if err := verifyTaskFence(chat, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true}); err != nil {
			return err
		}
		loaded, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  input.ChatID,
			AfterID: 0,
		})
		if err != nil {
			return xerrors.Errorf("load chat messages: %w", err)
		}
		locked = chat
		messages = loaded
		return nil
	})
	if err != nil {
		return database.Chat{}, nil, normalizeTaskInfrastructureError(err, "lock chat for generation")
	}
	return locked, messages, nil
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
		locked, err := store.GetChatByID(ctx, input.ChatID)
		if errors.Is(err, sql.ErrNoRows) {
			return errTaskExpectedExit
		}
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if err := verifyTaskFence(locked, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true}); err != nil {
			return err
		}
		decision.generationAttempt = locked.GenerationAttempt
		if locked.GenerationAttempt <= 0 || locked.GenerationAttempt >= int64(chatretry.MaxAttempts) {
			decision.retry = false
			return errRetryStateDecisionOnly
		}

		attempt := int(locked.GenerationAttempt)
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
		return err
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
		return errTaskExpectedExit
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
// error state. When every attempt fails, the last error is returned.
func retryGenerationPhase[T any](
	ctx context.Context,
	wait func(context.Context, time.Duration) error,
	fn func() (T, error),
) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < generationPhaseMaxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		if isTerminalGeneration(err) {
			return zero, err
		}
		if ctx.Err() != nil {
			return zero, errTaskExpectedExit
		}
		lastErr = err
		if attempt < generationPhaseMaxAttempts-1 {
			if waitErr := wait(ctx, generationPhaseBackoff(attempt)); waitErr != nil {
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
		return errTaskExpectedExit
	}
}

func (s *taskStarter) generateAssistant(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	prepared generationPrepared,
	decision generationDecision,
) error {
	attempt, _, publish, closeEpisode, err := s.beginGenerationAttempt(ctx, machine, input)
	if err != nil {
		return err
	}
	defer closeEpisode()
	runCtx := input.DebugTurn.Ensure(ctx, prepared.Chat, prepared.Debug)
	outcome, err := chatloop.GenerateAssistant(runCtx, chatloop.GenerateAssistantOptions{
		Model:                prepared.Model,
		Messages:             prepared.Prompt,
		Tools:                prepared.Tools,
		ActiveTools:          prepared.ActiveTools,
		ProviderTools:        prepared.ProviderTools,
		ContextLimitFallback: prepared.ContextLimitFallback,
		ModelConfig:          prepared.ModelConfig,
		ProviderOptions:      prepared.ProviderOptions,
		PublishMessagePart:   publish,
		Logger:               s.opts.Logger,
		Clock:                s.opts.Clock,
		Metrics:              s.server.metrics,
	})
	if err != nil {
		return err
	}
	if decision.compactionTrigger == compactionTriggerAlreadyCompacted &&
		shouldCompactPromptUsage(outcome.Step.Usage, prepared.ContextLimitFallback, prepared.Compaction.Options.ThresholdPercent) {
		err := errCompactionStillOverLimit
		s.server.metrics.RecordCompaction(compactionProvider(prepared.Compaction.Options), compactionModel(prepared.Compaction.Options), false, err)
		return s.finishGenerationError(ctx, machine, input, attempt, err, generationAttemptRequired)
	}
	if len(outcome.Step.Content) == 0 {
		return s.finishGenerationTurn(ctx, machine, input, attempt, generationDecision{kind: generationActionFinishTurn, finishReason: generationFinishReasonComplete}, generationAttemptRequired)
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
		return s.finishGenerationError(ctx, machine, input, attempt, err, generationAttemptRequired)
	}
	return s.commitGenerationStep(ctx, machine, input, attempt, generationActionGenerateAssistant, messages)
}

func (s *taskStarter) executeLocalTools(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	prepared generationPrepared,
	decision generationDecision,
) error {
	attempt, _, publish, closeEpisode, err := s.beginGenerationAttempt(ctx, machine, input)
	if err != nil {
		return err
	}
	defer closeEpisode()
	provider := ""
	modelName := ""
	if prepared.Model != nil {
		provider = prepared.Model.Provider()
		modelName = prepared.Model.Model()
	}
	// Local tool callbacks (e.g. spawn_agent, message_agent) read the
	// active turn's delegated API key ID from the context to route
	// subagent traffic through the AI Gateway. prepareGeneration sets it
	// only on its own context, so re-derive it here for tool execution.
	toolCtx := withActiveTurnAPIKeyID(ctx, prepared.ModelBuildOptions)
	outcome, err := chatloop.ExecuteLocalTools(toolCtx, chatloop.ExecuteLocalToolsOptions{
		Tools:              prepared.Tools,
		ActiveTools:        prepared.ActiveTools,
		ProviderTools:      prepared.ProviderTools,
		ToolCalls:          decision.localToolCalls,
		ExclusiveToolNames: prepared.ExclusiveToolNames,
		BuiltinToolNames:   prepared.BuiltinToolNames,
		ModelProvider:      provider,
		ModelName:          modelName,
		PublishMessagePart: publish,
		Logger:             s.opts.Logger,
		Metrics:            s.server.metrics,
		Clock:              s.opts.Clock,
	})
	if err != nil {
		return err
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
		return s.finishGenerationError(ctx, machine, input, attempt, err, generationAttemptRequired)
	}
	return s.commitGenerationStep(ctx, machine, input, attempt, generationActionExecuteLocalTools, messages)
}

func (s *taskStarter) generateCompaction(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	prepared generationPrepared,
) error {
	attempt, _, publish, closeEpisode, err := s.beginGenerationAttempt(ctx, machine, input)
	if err != nil {
		return err
	}
	defer closeEpisode()
	if prepared.Compaction == nil {
		return s.finishGenerationError(ctx, machine, input, attempt, xerrors.New("compaction action missing options"), generationAttemptRequired)
	}
	compactionOpts := prepared.Compaction.Options
	compactionOpts.PublishMessagePart = publish
	outcome, err := chatloop.GenerateCompaction(ctx, compactionOpts)
	if err != nil {
		s.server.metrics.RecordCompaction(compactionProvider(compactionOpts), compactionModel(compactionOpts), false, err)
		return err
	}
	if strings.TrimSpace(outcome.SystemSummary) == "" || strings.TrimSpace(outcome.SummaryReport) == "" {
		err := xerrors.New("compaction produced no summary")
		s.server.metrics.RecordCompaction(compactionProvider(compactionOpts), compactionModel(compactionOpts), false, err)
		return s.finishGenerationError(ctx, machine, input, attempt, err, generationAttemptRequired)
	}
	messages, err := buildCompactionMessages(buildCompactionMessagesInput{
		modelConfigID:  prepared.ModelConfigID,
		activeAPIKeyID: prepared.ModelBuildOptions.ActiveAPIKeyID,
		toolCallID:     compactionOpts.ToolCallID,
		toolName:       compactionOpts.ToolName,
		compaction:     compactionOutcome(outcome),
		contentVersion: chatprompt.CurrentContentVersion,
	})
	if err != nil {
		s.server.metrics.RecordCompaction(compactionProvider(compactionOpts), compactionModel(compactionOpts), false, err)
		return s.finishGenerationError(ctx, machine, input, attempt, err, generationAttemptRequired)
	}
	err = s.commitGenerationStep(ctx, machine, input, attempt, generationActionCompact, stepMessagesForCommit{
		Messages:       messages.Messages,
		VisibleIndexes: visibleMessageIndexes(messages.Messages),
	})
	s.server.metrics.RecordCompaction(compactionProvider(compactionOpts), compactionModel(compactionOpts), err == nil, err)
	return err
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

// persistWorkspaceContext is the generation action that commits durable
// workspace context messages (e.g. AGENTS.md, workspace skills) into
// chat history. It records a generation attempt, calls the injected
// workspace context builder without holding the DB lock, then commits
// the returned messages fenced to the attempt. If the builder returns
// no messages, the action exits as expected and the next worker task
// re-reads the chat.
func (s *taskStarter) persistWorkspaceContext(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	locked database.Chat,
) error {
	if s.server == nil {
		return errTaskExpectedExit
	}
	messages, err := s.opts.Store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  input.ChatID,
		AfterID: 0,
	})
	if err != nil {
		return taskRetryableError{err: xerrors.Errorf("load chat messages for workspace context: %w", err)}
	}
	attempt, _, _, closeEpisode, err := s.beginGenerationAttempt(ctx, machine, input)
	if err != nil {
		return err
	}
	defer closeEpisode()
	modelOpts := modelBuildOptionsFromMessages(messages)
	result, err := s.server.buildWorkspaceContext(ctx, workspaceContextBuildInput{
		Chat:           locked,
		Messages:       messages,
		ActiveAPIKeyID: modelOpts.ActiveAPIKeyID,
	})
	if err != nil {
		if errors.Is(err, errWorkspaceContextUnavailable) {
			// Builder reported nothing durable to commit (workspace or
			// agent missing, unreachable, etc.). Exit the action without
			// committing so the next worker task can re-read the chat.
			return errTaskExpectedExit
		}
		return err
	}
	return s.commitGenerationStep(ctx, machine, input, attempt, generationActionPersistWorkspaceContext, stepMessagesForCommit{
		Messages:       result.Messages,
		VisibleIndexes: visibleMessageIndexes(result.Messages),
	})
}

func (s *taskStarter) beginGenerationAttempt(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
) (int64, messagepartbuffer.Key, func(codersdk.ChatMessageRole, codersdk.ChatMessagePart), func(), error) {
	var attempt int64
	var committed database.Chat
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		locked, err := store.GetChatByID(ctx, input.ChatID)
		if errors.Is(err, sql.ErrNoRows) {
			return errTaskExpectedExit
		}
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if err := verifyTaskFence(locked, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true}); err != nil {
			return err
		}
		result, err := tx.RecordGenerationAttempt(chatstate.RecordGenerationAttemptInput{})
		if err != nil {
			return err
		}
		attempt = result.GenerationAttempt
		committed, err = store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, messagepartbuffer.Key{}, nil, nil, normalizeTaskTransitionError(err, "record generation attempt")
	}
	key := messagepartbuffer.Key{
		ChatID:            input.ChatID,
		HistoryVersion:    committed.HistoryVersion,
		GenerationAttempt: attempt,
	}
	if err := s.opts.MessagePartBuffer.CreateEpisode(key); err != nil && ctx.Err() == nil {
		return 0, messagepartbuffer.Key{}, nil, nil, taskRetryableError{err: xerrors.Errorf("create message part episode: %w", err)}
	}
	publish := func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
		_ = s.opts.MessagePartBuffer.AddPart(key, role, part)
	}
	closeEpisode := func() {
		_ = s.opts.MessagePartBuffer.CloseEpisode(key)
	}
	return attempt, key, publish, closeEpisode, nil
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
		return s.finishGenerationTurn(ctx, machine, input, attempt, generationDecision{kind: generationActionFinishTurn, finishReason: generationFinishReasonComplete}, generationAttemptRequired)
	}
	var committed database.Chat
	insertedMessages := []runnerActionMessage{}
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		locked, err := store.GetChatByID(ctx, input.ChatID)
		if errors.Is(err, sql.ErrNoRows) {
			return errTaskExpectedExit
		}
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if err := verifyGenerationFence(locked, input, attempt); err != nil {
			return err
		}
		commitResult, err := tx.CommitStep(chatstate.CommitStepInput{Messages: messages.Messages})
		if err != nil {
			return err
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
		locked, err := store.GetChatByID(ctx, input.ChatID)
		if errors.Is(err, sql.ErrNoRows) {
			return errTaskExpectedExit
		}
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if err := verifyTaskFence(locked, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true}); err != nil {
			return err
		}
		if _, err := tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{}); err != nil {
			return err
		}
		committed, err = store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		return nil
	})
	if err != nil {
		return normalizeTaskTransitionError(err, "enter requires action")
	}
	if err := s.publishWatchAndRoute(ctx, committed, codersdk.ChatWatchEventKindActionRequired); err != nil {
		return err
	}
	return s.afterGenerationOutcome(ctx, generationOutcome{
		Chat:           committed,
		Kind:           runnerActionKindEnterRequiresAction,
		WatchEventKind: codersdk.ChatWatchEventKindActionRequired,
	})
}

type generationAttemptFence int

const (
	generationAttemptNotRequired generationAttemptFence = iota
	generationAttemptRequired
)

func (s *taskStarter) finishGenerationTurn(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	attempt int64,
	decision generationDecision,
	attemptFence generationAttemptFence,
) error {
	var committed database.Chat
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		locked, err := store.GetChatByID(ctx, input.ChatID)
		if errors.Is(err, sql.ErrNoRows) {
			return errTaskExpectedExit
		}
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if attemptFence == generationAttemptRequired {
			if err := verifyGenerationFence(locked, input, attempt); err != nil {
				return err
			}
		} else if err := verifyTaskFence(locked, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true}); err != nil {
			return err
		}
		finishResult, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		if err != nil {
			return err
		}
		if finishResult.PromotedMessage != nil {
			decision.promotedMessageID = finishResult.PromotedMessage.ID
		}
		committed = finishResult.Chat
		return nil
	})
	if err != nil {
		return normalizeTaskTransitionError(err, "finish generation turn")
	}
	input.DebugTurn.RecordOutcome(chatdebug.StatusCompleted)
	watchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), postCommitWatchPublishTimeout)
	defer cancel()
	if err := s.publishWatchWithRetry(watchCtx, committed, codersdk.ChatWatchEventKindStatusChange); err != nil {
		return err
	}
	if err := s.afterGenerationOutcome(ctx, generationOutcome{
		Chat:              committed,
		Kind:              runnerActionKindFinishTurn,
		WatchEventKind:    codersdk.ChatWatchEventKindStatusChange,
		PromotedMessageID: decision.promotedMessageID,
	}); err != nil {
		return err
	}
	s.routeStateHint(ctx, stateUpdateFromChat(committed))
	return nil
}

func (s *taskStarter) finishGenerationError(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	attempt int64,
	cause error,
	attemptFence generationAttemptFence,
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
		locked, err := store.GetChatByID(ctx, input.ChatID)
		if errors.Is(err, sql.ErrNoRows) {
			return errTaskExpectedExit
		}
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if attemptFence == generationAttemptRequired {
			if err := verifyGenerationFence(locked, input, attempt); err != nil {
				return err
			}
		} else if err := verifyTaskFence(locked, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true}); err != nil {
			return err
		}
		if _, err := tx.FinishError(chatstate.FinishErrorInput{LastError: lastError}); err != nil {
			return err
		}
		committed, err = store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("load committed chat: %w", err)
		}
		return nil
	})
	if err != nil {
		return normalizeTaskTransitionError(err, "finish generation error")
	}
	input.DebugTurn.RecordOutcome(chatdebug.StatusError)
	if err := s.publishWatchAndRoute(ctx, committed, codersdk.ChatWatchEventKindStatusChange); err != nil {
		return err
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
	if s.server == nil {
		return nil
	}
	if err := s.server.afterGenerationOutcome(ctx, outcome); err != nil {
		return taskRetryableError{err: xerrors.Errorf("generation post-outcome side effects: %w", err)}
	}
	return nil
}

func verifyGenerationFence(chat database.Chat, input chatWorkerTaskStartInput, attempt int64) error {
	if err := verifyTaskFence(chat, input, database.ChatStatusRunning, taskFenceOptions{requireHistory: true}); err != nil {
		return err
	}
	if chat.GenerationAttempt != attempt {
		return errTaskExpectedExit
	}
	return nil
}

func stepDataFromPersisted(step chatloop.PersistedStep) stepData {
	return stepData{
		Content:              step.Content,
		Usage:                step.Usage,
		ContextLimit:         step.ContextLimit,
		ProviderResponseID:   step.ProviderResponseID,
		Runtime:              step.Runtime,
		ToolCallCreatedAt:    step.ToolCallCreatedAt,
		ToolResultCreatedAt:  step.ToolResultCreatedAt,
		ReasoningStartedAt:   step.ReasoningStartedAt,
		ReasoningCompletedAt: step.ReasoningCompletedAt,
	}
}
