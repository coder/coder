package chatloop

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/schema"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	interruptedToolResultErrorMessage = "tool call was interrupted before it produced a result"
	// maxCompactionRetries limits how many times the post-run
	// compaction safety net can re-enter the step loop. This
	// prevents infinite compaction loops when the model keeps
	// hitting the context limit after summarization.
	maxCompactionRetries = 3
	// defaultStartupTimeout bounds how long an individual
	// model attempt may spend starting to respond before
	// the attempt is canceled and retried.
	defaultStartupTimeout = 60 * time.Second
)

var (
	ErrInterrupted     = xerrors.New("chat interrupted")
	ErrDynamicToolCall = xerrors.New("dynamic tool call")
	// ErrStopAfterTool is returned when a tool listed in
	// StopAfterTools produces a successful result, indicating
	// the run should terminate cleanly after persistence.
	ErrStopAfterTool = xerrors.New("stop after tool")

	errStartupTimeout = xerrors.New(
		"chat response did not start before the startup timeout",
	)
)

// PendingToolCall describes a tool call that targets a dynamic
// tool. These calls are not executed by the chatloop; instead
// they are persisted so the caller can fulfill them externally.
type PendingToolCall struct {
	ToolCallID string
	ToolName   string
	Args       string
}

// PersistedStep contains the full content of a completed or
// interrupted agent step. Content includes both assistant blocks
// (text, reasoning, tool calls) and tool result blocks. The
// persistence layer is responsible for splitting these into
// separate database messages by role.
type PersistedStep struct {
	Content            []fantasy.Content
	Usage              fantasy.Usage
	ContextLimit       sql.NullInt64
	ProviderResponseID string
	// Runtime is the wall-clock duration of this step,
	// covering LLM streaming, tool execution, and retries.
	// Zero indicates the duration was not measured (e.g.
	// interrupted steps).
	Runtime time.Duration
	// PendingDynamicToolCalls lists tool calls that target
	// dynamic tools. When non-empty the chatloop exits with
	// ErrDynamicToolCall so the caller can execute them
	// externally and resume the loop.
	PendingDynamicToolCalls []PendingToolCall
	// ToolCallCreatedAt maps tool-call IDs to the time
	// the model emitted each tool call. Applied by the
	// persistence layer to set CreatedAt on persisted
	// tool-call ChatMessageParts.
	ToolCallCreatedAt map[string]time.Time
	// ToolResultCreatedAt maps tool-call IDs to the time
	// each tool result was produced (or interrupted).
	// Applied by the persistence layer to set CreatedAt
	// on persisted tool-result ChatMessageParts.
	ToolResultCreatedAt map[string]time.Time
}

// RunOptions configures a single streaming chat loop run.
type RunOptions struct {
	Model    fantasy.LanguageModel
	Messages []fantasy.Message
	Tools    []fantasy.AgentTool
	MaxSteps int
	// StartupTimeout bounds how long each model attempt may
	// spend opening the provider stream and waiting for its
	// first stream part before the attempt is canceled and
	// retried. Zero uses the production default.
	StartupTimeout time.Duration
	// Clock creates startup guard timers. In production use a
	// real clock; tests can inject quartz.NewMock(t) to make
	// startup timeout behavior deterministic.
	Clock quartz.Clock

	ActiveTools          []string
	ContextLimitFallback int64

	// DynamicToolNames lists tool names that are handled
	// externally. When the model invokes one of these tools
	// the chatloop persists partial results and exits with
	// ErrDynamicToolCall instead of executing the tool.
	DynamicToolNames map[string]bool
	// StopAfterTools lists tool names that, when they produce a
	// successful result, cause the run to stop after persisting
	// the current step. This is used for plan turns where
	// propose_plan should terminate the run on success.
	StopAfterTools map[string]struct{}
	// ExclusiveToolNames lists tool names that must be called
	// alone in a batch. When any exclusive tool appears
	// alongside other locally-executed tools, every tool in the
	// batch receives a policy error and nothing executes.
	ExclusiveToolNames map[string]bool

	// ModelConfig holds per-call LLM parameters (temperature,
	// max tokens, etc.) read from the chat model configuration.
	ModelConfig codersdk.ChatModelCallConfig
	// ProviderOptions are provider-specific call options
	// converted from ModelConfig.ProviderOptions. This is a
	// separate field because the conversion requires knowledge
	// of the provider, which lives in chatd, not chatloop.
	ProviderOptions fantasy.ProviderOptions

	// ProviderTools are provider-native tools (like web search
	// and computer use) whose definitions are passed directly
	// to the provider API. When a ProviderTool has a non-nil
	// Runner, tool calls are executed locally; otherwise the
	// provider handles execution (e.g. web search).
	ProviderTools []ProviderTool

	PersistStep        func(context.Context, PersistedStep) error
	PublishMessagePart func(
		role codersdk.ChatMessageRole,
		part codersdk.ChatMessagePart,
	)
	// Callers should attach correlation fields (chat_id, owner_id, etc.)
	// using Logger.With before passing the logger in.
	Logger           slog.Logger
	Compaction       *CompactionOptions
	ReloadMessages   func(context.Context) ([]fantasy.Message, error)
	DisableChainMode func()
	// PrepareMessages is called before each LLM step with the
	// current message history. If it returns non-nil, the returned
	// slice replaces messages for this and all subsequent steps.
	// Used to inject system context that becomes available mid-loop
	// (e.g. AGENTS.md after create_workspace).
	PrepareMessages func([]fantasy.Message) []fantasy.Message

	// OnRetry is called before each retry attempt when the LLM
	// stream fails with a retryable error. It provides the attempt
	// number, raw error, normalized classification, and backoff
	// delay so callers can publish status events to connected
	// clients. Callers should also clear any buffered stream state
	// from the failed attempt in this callback to avoid sending
	// duplicated content.
	OnRetry chatretry.OnRetryFn

	OnInterruptedPersistError func(error)

	// Metrics records Prometheus metrics for the chatd subsystem.
	// When nil, no metrics are recorded.
	Metrics *Metrics

	// BuiltinToolNames lists tool names that are built into chatd.
	BuiltinToolNames map[string]bool
}

// ProviderTool pairs a provider-native tool definition with an
// optional local executor. When Runner is nil the tool is fully
// provider-executed (e.g. web search). When Runner is non-nil
// the definition is sent to the API but execution is handled
// locally (e.g. computer use).
type ProviderTool struct {
	Definition fantasy.Tool
	Runner     fantasy.AgentTool
}

// stepResult holds the accumulated output of a single streaming
// step. Since we own the stream consumer, all content is tracked
// directly here, no shadow draft state needed.
type stepResult struct {
	content             []fantasy.Content
	usage               fantasy.Usage
	providerMetadata    fantasy.ProviderMetadata
	finishReason        fantasy.FinishReason
	toolCalls           []fantasy.ToolCallContent
	shouldContinue      bool
	toolCallCreatedAt   map[string]time.Time
	toolResultCreatedAt map[string]time.Time
}

// toResponseMessages converts step content into messages suitable
// for appending to the conversation. Mirrors fantasy's
// toResponseMessages logic.
func (r stepResult) toResponseMessages() []fantasy.Message {
	var assistantParts []fantasy.MessagePart
	var toolParts []fantasy.MessagePart

	for _, c := range r.content {
		switch c.GetType() {
		case fantasy.ContentTypeText:
			text, ok := fantasy.AsContentType[fantasy.TextContent](c)
			if !ok || strings.TrimSpace(text.Text) == "" {
				continue
			}
			assistantParts = append(assistantParts, fantasy.TextPart{
				Text:            text.Text,
				ProviderOptions: fantasy.ProviderOptions(text.ProviderMetadata),
			})
		case fantasy.ContentTypeReasoning:
			reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](c)
			if !ok || strings.TrimSpace(reasoning.Text) == "" {
				continue
			}
			assistantParts = append(assistantParts, fantasy.ReasoningPart{
				Text:            reasoning.Text,
				ProviderOptions: fantasy.ProviderOptions(reasoning.ProviderMetadata),
			})
		case fantasy.ContentTypeToolCall:
			toolCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](c)
			if !ok {
				continue
			}
			assistantParts = append(assistantParts, fantasy.ToolCallPart{
				ToolCallID:       toolCall.ToolCallID,
				ToolName:         toolCall.ToolName,
				Input:            toolCall.Input,
				ProviderExecuted: toolCall.ProviderExecuted,
				ProviderOptions:  fantasy.ProviderOptions(toolCall.ProviderMetadata),
			})
		case fantasy.ContentTypeFile:
			file, ok := fantasy.AsContentType[fantasy.FileContent](c)
			if !ok {
				continue
			}
			assistantParts = append(assistantParts, fantasy.FilePart{
				Data:            file.Data,
				MediaType:       file.MediaType,
				ProviderOptions: fantasy.ProviderOptions(file.ProviderMetadata),
			})
		case fantasy.ContentTypeSource:
			// Sources are metadata about references; they don't
			// need to be included in conversation messages.
			continue
		case fantasy.ContentTypeToolResult:
			result, ok := fantasy.AsContentType[fantasy.ToolResultContent](c)
			if !ok {
				continue
			}
			part := fantasy.ToolResultPart{
				ToolCallID:       result.ToolCallID,
				Output:           result.Result,
				ProviderExecuted: result.ProviderExecuted,
				ProviderOptions:  fantasy.ProviderOptions(result.ProviderMetadata),
			}
			// Provider-executed tool results (e.g. web_search)
			// must stay in the assistant message so the result
			// block appears inline after the corresponding
			// server_tool_use block. This matches the persistence
			// layer in chatd.go which keeps them in
			// assistantBlocks.
			if result.ProviderExecuted {
				assistantParts = append(assistantParts, part)
			} else {
				toolParts = append(toolParts, part)
			}
		default:
			continue
		}
	}

	var messages []fantasy.Message
	if len(assistantParts) > 0 {
		messages = append(messages, fantasy.Message{
			Role:    fantasy.MessageRoleAssistant,
			Content: assistantParts,
		})
	}
	if len(toolParts) > 0 {
		messages = append(messages, fantasy.Message{
			Role:    fantasy.MessageRoleTool,
			Content: toolParts,
		})
	}
	return messages
}

// reasoningState accumulates reasoning content and provider
// metadata while the stream is in flight.
type reasoningState struct {
	text    string
	options fantasy.ProviderMetadata
}

// Run executes the chat step-stream loop and delegates
// persistence/publishing to callbacks.
func Run(ctx context.Context, opts RunOptions) error {
	if opts.Model == nil {
		return xerrors.New("chat model is required")
	}
	if opts.PersistStep == nil {
		return xerrors.New("persist step callback is required")
	}
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 1
	}
	if opts.StartupTimeout <= 0 {
		opts.StartupTimeout = defaultStartupTimeout
	}
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.Metrics == nil {
		opts.Metrics = NopMetrics()
	}

	publishMessagePart := func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
		if opts.PublishMessagePart == nil {
			return
		}
		opts.PublishMessagePart(role, part)
	}

	tools := buildToolDefinitions(opts.Tools, opts.ActiveTools, opts.ProviderTools)
	applyAnthropicCaching := shouldApplyAnthropicPromptCaching(opts.Model)

	messages := opts.Messages
	var lastUsage fantasy.Usage
	var lastProviderMetadata fantasy.ProviderMetadata
	needsFullHistoryReload := false
	reloadFullHistory := func(stage string) error {
		if opts.ReloadMessages == nil {
			return nil
		}
		reloaded, err := opts.ReloadMessages(ctx)
		if err != nil {
			return xerrors.Errorf("reload messages %s: %w", stage, err)
		}
		messages = reloaded
		return nil
	}

	totalSteps := 0
	// When totalSteps reaches MaxSteps the inner loop exits immediately
	// (its condition is false), stoppedByModel stays false, and the
	// post-loop guard breaks the outer compaction loop.
	for compactionAttempt := 0; ; compactionAttempt++ {
		alreadyCompacted := false
		// stoppedByModel is true when the inner step loop
		// exited because the model produced no tool calls
		// (shouldContinue was false). This distinguishes a
		// natural stop from hitting MaxSteps.
		stoppedByModel := false
		// compactedOnFinalStep tracks whether compaction
		// occurred on the very step where the model stopped.
		// Only in that case should we re-enter, because the
		// agent never had a chance to use the compacted context.
		compactedOnFinalStep := false

		for step := 0; totalSteps < opts.MaxSteps; step++ {
			totalSteps++
			provider := opts.Model.Provider()
			modelName := opts.Model.Model()
			opts.Metrics.StepsTotal.WithLabelValues(provider, modelName).Inc()
			stepStart := time.Now()
			// Copy messages so that provider-specific caching
			// mutations don't leak back to the caller's slice.
			// copy copies Message structs by value, so field
			// reassignments in addAnthropicPromptCaching only
			// affect the prepared slice.
			if opts.PrepareMessages != nil {
				if updated := opts.PrepareMessages(messages); updated != nil {
					messages = updated
				}
			}
			prepared := make([]fantasy.Message, len(messages))
			copy(prepared, messages)
			prepared, sanitizeStats := chatsanitize.SanitizeAnthropicProviderToolHistory(provider, prepared)
			chatsanitize.LogAnthropicProviderToolSanitization(
				ctx, opts.Logger, "pre_request", provider, modelName, sanitizeStats,
				slog.F("step_index", step),
				slog.F("total_steps", totalSteps),
			)
			prepared = chatsanitize.ApplyAnthropicProviderToolGuard(
				ctx, opts.Logger, provider, modelName, prepared,
			)
			if applyAnthropicCaching {
				addAnthropicPromptCaching(prepared)
			}
			opts.Metrics.MessageCount.WithLabelValues(provider, modelName).Observe(float64(len(prepared)))
			opts.Metrics.PromptSizeBytes.WithLabelValues(provider, modelName).Observe(float64(EstimatePromptSize(prepared)))

			call := fantasy.Call{
				Prompt:           prepared,
				Tools:            tools,
				MaxOutputTokens:  opts.ModelConfig.MaxOutputTokens,
				Temperature:      opts.ModelConfig.Temperature,
				TopP:             opts.ModelConfig.TopP,
				TopK:             opts.ModelConfig.TopK,
				PresencePenalty:  opts.ModelConfig.PresencePenalty,
				FrequencyPenalty: opts.ModelConfig.FrequencyPenalty,
				ProviderOptions:  opts.ProviderOptions,
			}

			var result stepResult
			stepCtx := chatdebug.ReuseStep(ctx)
			err := chatretry.Retry(stepCtx, func(retryCtx context.Context) error {
				attempt, streamErr := guardedStream(
					retryCtx,
					provider,
					modelName,
					opts.Clock,
					opts.StartupTimeout,
					func(attemptCtx context.Context) (fantasy.StreamResponse, error) {
						return opts.Model.Stream(attemptCtx, call)
					},
					opts.Metrics,
				)
				if streamErr != nil {
					return streamErr
				}
				defer attempt.release()
				var processErr error
				result, processErr = processStepStream(
					attempt.ctx,
					attempt.stream,
					publishMessagePart,
				)
				return attempt.finish(processErr)
			}, func(
				attempt int,
				retryErr error,
				classified chatretry.ClassifiedError,
				delay time.Duration,
			) {
				// Reset result from the failed attempt so the next
				// attempt starts clean.
				result = stepResult{}
				// Record before OnRetry so a panicking callback can't
				// drop the sample. The metric's provider label comes
				// from the outer local; WithProvider only affects the
				// classified payload handed to OnRetry.
				classified = classified.WithProvider(provider)
				opts.Metrics.RecordStreamRetry(provider, modelName, classified)
				if opts.OnRetry != nil {
					opts.OnRetry(attempt, retryErr, classified, delay)
				}
			})
			if err != nil {
				if errors.Is(err, ErrInterrupted) {
					persistInterruptedStep(ctx, opts, &result)
					return ErrInterrupted
				}
				return xerrors.Errorf("stream response: %w", err)
			}

			// Execute tools before persisting so that tool results
			// are included in the persisted step content. The
			// persistence layer splits assistant and tool-result
			// blocks into separate database messages by role.
			var toolResults []fantasy.ToolResultContent
			if result.shouldContinue {
				var err error
				toolResults, err = executeToolsForStep(ctx, opts, &result, provider, modelName, step, stepStart, publishMessagePart)
				if err != nil {
					return err
				}
			}
			// Extract context limit from provider metadata.
			contextLimit := extractContextLimitWithFallback(
				result.providerMetadata,
				opts.ContextLimitFallback,
			)
			result.content = chatsanitize.SanitizeAnthropicProviderToolStepContent(
				ctx, opts.Logger, provider, modelName,
				"normal_persist", step, result.finishReason, result.content,
			)
			if len(result.content) == 0 {
				lastUsage = result.usage
				lastProviderMetadata = result.providerMetadata
				stoppedByModel = true
				break
			}

			// Persist the step. If persistence fails because
			// the chat was interrupted between the previous
			// check and here, fall back to the interrupt-safe
			// path so partial content is not lost.
			if err := opts.PersistStep(ctx, PersistedStep{
				Content:             result.content,
				Usage:               result.usage,
				ContextLimit:        contextLimit,
				ProviderResponseID:  chatopenai.ExtractResponseIDIfStored(opts.ProviderOptions, result.providerMetadata),
				Runtime:             time.Since(stepStart),
				ToolCallCreatedAt:   result.toolCallCreatedAt,
				ToolResultCreatedAt: result.toolResultCreatedAt,
			}); err != nil {
				if errors.Is(err, ErrInterrupted) {
					persistInterruptedStep(ctx, opts, &result)
					return ErrInterrupted
				}
				return xerrors.Errorf("persist step: %w", err)
			}
			lastUsage = result.usage
			lastProviderMetadata = result.providerMetadata

			// Check if any executed tool triggers an early stop.
			if shouldStopAfterTools(opts.StopAfterTools, toolResults) {
				tryCompactOnExit(ctx, opts, result.usage, result.providerMetadata)
				return ErrStopAfterTool
			}

			// When chain mode is active (PreviousResponseID set), exit
			// it after persisting the first chained step. Continuation
			// steps include tool-result messages, which fantasy rejects
			// when previous_response_id is set, so we must leave chain
			// mode and reload the full history before the next call.
			stepMessages := result.toResponseMessages()
			if chatopenai.HasPreviousResponseID(opts.ProviderOptions) {
				opts.ProviderOptions = chatopenai.ClearPreviousResponseID(opts.ProviderOptions)
				if opts.DisableChainMode != nil {
					opts.DisableChainMode()
				}
				switch {
				case opts.ReloadMessages != nil:
					if err := reloadFullHistory("after chain mode exit"); err != nil {
						return err
					}
					needsFullHistoryReload = false
				default:
					messages = append(messages, stepMessages...)
					needsFullHistoryReload = false
				}
			} else {
				messages = append(messages, stepMessages...)
			}

			if needsFullHistoryReload && !result.shouldContinue &&
				opts.ReloadMessages != nil {
				if err := reloadFullHistory("before final compaction after chain mode exit"); err != nil {
					return err
				}
				needsFullHistoryReload = false
			}

			// Inline compaction.
			if !needsFullHistoryReload && opts.Compaction != nil && opts.ReloadMessages != nil {
				did, compactErr := tryCompact(
					ctx,
					opts.Model,
					opts.Compaction,
					opts.ContextLimitFallback,
					result.usage,
					result.providerMetadata,
					messages,
				)
				opts.Metrics.RecordCompaction(provider, modelName, did, compactErr)
				if compactErr != nil && opts.Compaction.OnError != nil {
					opts.Compaction.OnError(compactErr)
				}

				if did {
					alreadyCompacted = true
					compactedOnFinalStep = true
					if err := reloadFullHistory("after compaction"); err != nil {
						return err
					}
				}
			}
			if !result.shouldContinue {
				stoppedByModel = true
				break
			}

			// The agent is continuing with tool calls, so any
			// prior compaction has already been consumed.
			compactedOnFinalStep = false
		}

		if needsFullHistoryReload && stoppedByModel && opts.ReloadMessages != nil {
			if err := reloadFullHistory("before post-run compaction after chain mode exit"); err != nil {
				return err
			}
			needsFullHistoryReload = false
		}

		// Post-run compaction safety net: if we never compacted
		// during the loop, try once at the end.
		if !needsFullHistoryReload && !alreadyCompacted && opts.Compaction != nil && opts.ReloadMessages != nil {
			did, err := tryCompact(
				ctx,
				opts.Model,
				opts.Compaction,
				opts.ContextLimitFallback,
				lastUsage,
				lastProviderMetadata,
				messages,
			)
			opts.Metrics.RecordCompaction(opts.Model.Provider(), opts.Model.Model(), did, err)
			if err != nil {
				if opts.Compaction.OnError != nil {
					opts.Compaction.OnError(err)
				}
			}
			if did {
				compactedOnFinalStep = true
			}
		}
		// Re-enter the step loop when compaction fired on the
		// model's final step. This lets the agent continue
		// working with fresh summarized context instead of
		// stopping. When the inner loop continued after inline
		// compaction (tool-call steps kept going), the agent
		// already used the compacted context, so no re-entry
		// is needed. Limit retries to prevent infinite loops.
		if compactedOnFinalStep && stoppedByModel &&
			opts.ReloadMessages != nil &&
			compactionAttempt < maxCompactionRetries {
			reloaded, reloadErr := opts.ReloadMessages(ctx)
			if reloadErr != nil {
				return xerrors.Errorf("reload messages after compaction: %w", reloadErr)
			}
			messages = reloaded
			continue
		}
		break
	}

	return nil
}

// guardedAttempt owns an attempt-scoped context and startup guard
// around a provider stream. release is idempotent and frees the
// attempt-scoped timer/context. finish canonicalizes startup timeout
// errors before the retry loop classifies them.
type guardedAttempt struct {
	ctx     context.Context
	stream  fantasy.StreamResponse
	release func()
	finish  func(error) error
}

// startupGuard arbitrates whether an attempt times out during
// stream startup. Exactly one outcome wins: the timer cancels
// the attempt, or the first-part path disarms the timer.
type startupGuard struct {
	timer  *quartz.Timer
	cancel context.CancelCauseFunc
	once   sync.Once
}

func newStartupGuard(
	clock quartz.Clock,
	timeout time.Duration,
	cancel context.CancelCauseFunc,
) *startupGuard {
	guard := &startupGuard{cancel: cancel}
	guard.timer = clock.AfterFunc(timeout, guard.onTimeout, "startupGuard")
	return guard
}

func (g *startupGuard) onTimeout() {
	g.once.Do(func() {
		g.cancel(errStartupTimeout)
	})
}

func (g *startupGuard) Disarm() {
	g.once.Do(func() {
		g.timer.Stop()
	})
}

func classifyStartupTimeout(
	attemptCtx context.Context,
	provider string,
	err error,
) error {
	if !errors.Is(context.Cause(attemptCtx), errStartupTimeout) {
		return err
	}
	if err == nil {
		err = errStartupTimeout
	}
	return chaterror.WithClassification(err, chaterror.ClassifiedError{
		Kind:      chaterror.KindStartupTimeout,
		Provider:  provider,
		Retryable: true,
	})
}

func guardedStream(
	parent context.Context,
	provider, model string,
	clock quartz.Clock,
	timeout time.Duration,
	openStream func(context.Context) (fantasy.StreamResponse, error),
	metrics *Metrics,
) (guardedAttempt, error) {
	attemptCtx, cancelAttempt := context.WithCancelCause(parent)
	guard := newStartupGuard(clock, timeout, cancelAttempt)
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			guard.Disarm()
			cancelAttempt(nil)
		})
	}

	streamStart := clock.Now()
	stream, err := openStream(attemptCtx)
	if err != nil {
		err = classifyStartupTimeout(attemptCtx, provider, err)
		release()
		return guardedAttempt{}, err
	}

	recordTTFT := sync.OnceFunc(func() {
		metrics.TTFTSeconds.WithLabelValues(provider, model).Observe(
			clock.Since(streamStart).Seconds(),
		)
	})
	return guardedAttempt{
		ctx: attemptCtx,
		stream: fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
			for part := range stream {
				guard.Disarm()
				recordTTFT()
				if !yield(part) {
					return
				}
			}
		}),
		release: release,
		finish: func(err error) error {
			return classifyStartupTimeout(attemptCtx, provider, err)
		},
	}, nil
}

// processStepStream consumes a fantasy StreamResponse and
// accumulates all content into a stepResult. Callbacks fire
// inline and their errors propagate directly.
func processStepStream(
	ctx context.Context,
	stream fantasy.StreamResponse,
	publishMessagePart func(codersdk.ChatMessageRole, codersdk.ChatMessagePart),
) (stepResult, error) {
	var result stepResult

	activeToolCalls := make(map[string]*fantasy.ToolCallContent)
	activeTextContent := make(map[string]string)
	activeReasoningContent := make(map[string]reasoningState)
	// Track tool names by ID for input delta publishing.
	toolNames := make(map[string]string)

	for part := range stream {
		switch part.Type {
		case fantasy.StreamPartTypeTextStart:
			activeTextContent[part.ID] = ""

		case fantasy.StreamPartTypeTextDelta:
			if _, exists := activeTextContent[part.ID]; exists {
				activeTextContent[part.ID] += part.Delta
			}
			publishMessagePart(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(part.Delta))

		case fantasy.StreamPartTypeTextEnd:
			if text, exists := activeTextContent[part.ID]; exists {
				result.content = append(result.content, fantasy.TextContent{
					Text:             text,
					ProviderMetadata: part.ProviderMetadata,
				})
				delete(activeTextContent, part.ID)
			}

		case fantasy.StreamPartTypeReasoningStart:
			activeReasoningContent[part.ID] = reasoningState{
				text:    part.Delta,
				options: part.ProviderMetadata,
			}

		case fantasy.StreamPartTypeReasoningDelta:
			if active, exists := activeReasoningContent[part.ID]; exists {
				active.text += part.Delta
				active.options = part.ProviderMetadata
				activeReasoningContent[part.ID] = active
			}
			publishMessagePart(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageReasoning(part.Delta))

		case fantasy.StreamPartTypeReasoningEnd:
			if active, exists := activeReasoningContent[part.ID]; exists {
				if part.ProviderMetadata != nil {
					active.options = part.ProviderMetadata
				}
				content := fantasy.ReasoningContent{
					Text:             active.text,
					ProviderMetadata: active.options,
				}
				result.content = append(result.content, content)
				delete(activeReasoningContent, part.ID)
			}
		case fantasy.StreamPartTypeToolInputStart:
			activeToolCalls[part.ID] = &fantasy.ToolCallContent{
				ToolCallID:       part.ID,
				ToolName:         part.ToolCallName,
				Input:            "",
				ProviderExecuted: part.ProviderExecuted,
			}
			if strings.TrimSpace(part.ToolCallName) != "" {
				toolNames[part.ID] = part.ToolCallName
			}

		case fantasy.StreamPartTypeToolInputDelta:
			var providerExecuted bool
			if toolCall, exists := activeToolCalls[part.ID]; exists {
				toolCall.Input += part.Delta
				providerExecuted = toolCall.ProviderExecuted
			}
			toolName := toolNames[part.ID]
			publishMessagePart(codersdk.ChatMessageRoleAssistant, codersdk.ChatMessagePart{
				Type:             codersdk.ChatMessagePartTypeToolCall,
				ToolCallID:       part.ID,
				ToolName:         toolName,
				ArgsDelta:        part.Delta,
				ProviderExecuted: providerExecuted,
			})
		case fantasy.StreamPartTypeToolInputEnd:
			// No callback needed; the full tool call arrives in
			// StreamPartTypeToolCall.

		case fantasy.StreamPartTypeToolCall:
			tc := fantasy.ToolCallContent{
				ToolCallID:       part.ID,
				ToolName:         part.ToolCallName,
				Input:            part.ToolCallInput,
				ProviderExecuted: part.ProviderExecuted,
				ProviderMetadata: part.ProviderMetadata,
			}
			result.toolCalls = append(result.toolCalls, tc)
			result.content = append(result.content, tc)
			if strings.TrimSpace(part.ToolCallName) != "" {
				toolNames[part.ID] = part.ToolCallName
			}
			// Clean up active tool call tracking.
			delete(activeToolCalls, part.ID)

			// Record when the model emitted this tool call
			// so the persisted part carries an accurate
			// timestamp for duration computation.
			now := dbtime.Now()
			if result.toolCallCreatedAt == nil {
				result.toolCallCreatedAt = make(map[string]time.Time)
			}
			result.toolCallCreatedAt[part.ID] = now

			ssePart := chatprompt.PartFromContent(tc)
			ssePart.CreatedAt = &now
			publishMessagePart(
				codersdk.ChatMessageRoleAssistant,
				ssePart,
			)

		case fantasy.StreamPartTypeSource:
			sourceContent := fantasy.SourceContent{
				SourceType:       part.SourceType,
				ID:               part.ID,
				URL:              part.URL,
				Title:            part.Title,
				ProviderMetadata: part.ProviderMetadata,
			}
			result.content = append(result.content, sourceContent)
			publishMessagePart(
				codersdk.ChatMessageRoleAssistant,
				chatprompt.PartFromContent(sourceContent),
			)

		case fantasy.StreamPartTypeToolResult:
			// Provider-executed tool results (e.g. web search)
			// are emitted by the provider and added directly
			// to the step content for multi-turn round-tripping.
			// This mirrors fantasy's agent.go accumulation logic.
			if part.ProviderExecuted {
				tr := fantasy.ToolResultContent{
					ToolCallID:       part.ID,
					ToolName:         part.ToolCallName,
					ProviderExecuted: part.ProviderExecuted,
					ProviderMetadata: part.ProviderMetadata,
				}
				result.content = append(result.content, tr)

				now := dbtime.Now()
				if result.toolResultCreatedAt == nil {
					result.toolResultCreatedAt = make(map[string]time.Time)
				}
				result.toolResultCreatedAt[part.ID] = now

				ssePart := chatprompt.PartFromContent(tr)
				ssePart.CreatedAt = &now
				publishMessagePart(
					codersdk.ChatMessageRoleTool,
					ssePart,
				)
			}
		case fantasy.StreamPartTypeFinish:
			result.usage = part.Usage
			result.finishReason = part.FinishReason
			result.providerMetadata = part.ProviderMetadata

		case fantasy.StreamPartTypeError:
			// Detect interruption: the stream may surface the
			// cancel as context.Canceled or propagate the
			// ErrInterrupted cause directly, depending on
			// the provider implementation.
			if errors.Is(context.Cause(ctx), ErrInterrupted) &&
				(errors.Is(part.Error, context.Canceled) || errors.Is(part.Error, ErrInterrupted)) {
				// Flush in-progress content so that
				// persistInterruptedStep has access to partial
				// text, reasoning, and tool calls that were
				// still streaming when the interrupt arrived.
				flushActiveState(
					&result,
					activeTextContent,
					activeReasoningContent,
					activeToolCalls,
					toolNames,
				)
				return result, ErrInterrupted
			}
			return result, part.Error
		}
	}

	// The stream iterator may stop yielding parts without
	// producing a StreamPartTypeError when the context is
	// canceled (e.g. some providers close the response body
	// silently). Detect this case and flush partial content
	// so that persistInterruptedStep can save it.
	if ctx.Err() != nil &&
		errors.Is(context.Cause(ctx), ErrInterrupted) {
		flushActiveState(
			&result,
			activeTextContent,
			activeReasoningContent,
			activeToolCalls,
			toolNames,
		)
		return result, ErrInterrupted
	}
	hasLocalToolCalls := false
	for _, tc := range result.toolCalls {
		if !tc.ProviderExecuted {
			hasLocalToolCalls = true
			break
		}
	}
	result.shouldContinue = hasLocalToolCalls &&
		result.finishReason == fantasy.FinishReasonToolCalls
	return result, nil
}

// executeTools runs all tool calls concurrently after the stream
// completes. Results are published via onResult in the original
// tool-call order after all tools finish, preserving deterministic
// event ordering for SSE subscribers.
func executeTools(
	ctx context.Context,
	allTools []fantasy.AgentTool,
	activeTools []string,
	providerTools []ProviderTool,
	toolCalls []fantasy.ToolCallContent,
	metrics *Metrics,
	logger slog.Logger,
	provider, model string,
	builtinToolNames map[string]bool,
	onResult func(fantasy.ToolResultContent, time.Time),
) []fantasy.ToolResultContent {
	if len(toolCalls) == 0 {
		return nil
	}

	// Filter out provider-executed tool calls. These were
	// handled server-side by the LLM provider (e.g., web
	// search) and their results are already in the stream
	// content.
	localToolCalls := make([]fantasy.ToolCallContent, 0, len(toolCalls))
	for _, tc := range toolCalls {
		if !tc.ProviderExecuted {
			localToolCalls = append(localToolCalls, tc)
		}
	}
	if len(localToolCalls) == 0 {
		return nil
	}

	toolMap := make(map[string]fantasy.AgentTool, len(allTools))
	for _, t := range allTools {
		toolMap[t.Info().Name] = t
	}
	providerRunnerNames := make(map[string]struct{}, len(providerTools))
	// Include runners from provider tools so locally-executed
	// provider tools (e.g. computer use) can be dispatched.
	for _, pt := range providerTools {
		if pt.Runner != nil {
			name := pt.Runner.Info().Name
			toolMap[name] = pt.Runner
			providerRunnerNames[name] = struct{}{}
		}
	}

	results := make([]fantasy.ToolResultContent, len(localToolCalls))
	completedAt := make([]time.Time, len(localToolCalls))
	var wg sync.WaitGroup
	wg.Add(len(localToolCalls))
	for i, tc := range localToolCalls {
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[i] = fantasy.ToolResultContent{
						ToolCallID: tc.ToolCallID,
						ToolName:   tc.ToolName,
						Result: fantasy.ToolResultOutputContentError{
							Error: xerrors.Errorf("tool panicked: %v", r),
						},
					}
				}
				// Record when this tool completed (or panicked).
				// Captured per-goroutine so parallel tools get
				// accurate individual completion times.
				completedAt[i] = dbtime.Now()
			}()
			results[i] = executeSingleTool(ctx, toolMap, tc, metrics, logger, provider, model, builtinToolNames, activeTools, providerRunnerNames)
		}()
	}
	wg.Wait()

	// Publish results in the original tool-call order so SSE
	// subscribers see a deterministic event sequence.
	if onResult != nil {
		for i, tr := range results {
			onResult(tr, completedAt[i])
		}
	}
	return results
}

// executeToolsForStep runs the tool-execution phase of a single
// chatloop step. It enforces the exclusive-tool policy, partitions
// built-in versus dynamic tool calls, dispatches built-in tools, and
// when dynamic tool calls are present persists the step and returns
// ErrDynamicToolCall so the caller can execute them externally.
// Returns the tool results to append to the step, or an error that the
// caller must propagate (ErrInterrupted, ErrDynamicToolCall, ctx.Err(),
// or a persistence failure).
func executeToolsForStep(
	ctx context.Context,
	opts RunOptions,
	result *stepResult,
	provider, modelName string,
	step int,
	stepStart time.Time,
	publishMessagePart func(codersdk.ChatMessageRole, codersdk.ChatMessagePart),
) ([]fantasy.ToolResultContent, error) {
	// Check for context cancellation before starting tool
	// execution. If the chat was interrupted between stream
	// completion and here, persist what we have and bail out.
	if ctx.Err() != nil {
		if errors.Is(context.Cause(ctx), ErrInterrupted) {
			persistInterruptedStep(ctx, opts, result)
			return nil, ErrInterrupted
		}
		return nil, ctx.Err()
	}

	// Enforce exclusivity across ALL locally-executable tool
	// calls (both built-in and dynamic) before partitioning.
	// Checking only the built-in partition would let the model
	// bypass the policy by mixing an exclusive tool with a
	// dynamic tool: the exclusive tool would still run and the
	// dynamic call would still be handed to the caller for
	// external execution, breaking the planning-only contract.
	localCandidates := make([]fantasy.ToolCallContent, 0, len(result.toolCalls))
	for _, tc := range result.toolCalls {
		if !tc.ProviderExecuted {
			localCandidates = append(localCandidates, tc)
		}
	}
	policyResults, exclusiveViolation := applyExclusiveToolPolicy(
		localCandidates,
		opts.ExclusiveToolNames,
		opts.Metrics,
		provider,
		modelName,
	)
	if exclusiveViolation {
		now := dbtime.Now()
		for _, tr := range policyResults {
			recordToolResultTimestamp(result, tr.ToolCallID, now)
			publishToolAttachments(ctx, opts.Logger, tr, now, publishMessagePart)
			ssePart := chatprompt.PartFromContentWithLogger(ctx, opts.Logger, tr)
			ssePart.CreatedAt = &now
			publishMessagePart(codersdk.ChatMessageRoleTool, ssePart)
		}
		for _, tr := range policyResults {
			result.content = append(result.content, tr)
		}
		// Mirror the post-execution interruption check used by the
		// non-policy path: if the chat was interrupted while we
		// synthesized policy errors, route through
		// persistInterruptedStep so the synthesized results are not
		// dropped when the regular PersistStep path fails on a
		// canceled context.
		if ctx.Err() != nil {
			if errors.Is(context.Cause(ctx), ErrInterrupted) {
				persistInterruptedStep(ctx, opts, result)
				return nil, ErrInterrupted
			}
			return nil, ctx.Err()
		}
		// Fall through to the normal persistence path so the loop
		// continues with error results that the model can observe
		// and retry. Skip partitioning, execution, and
		// pending-dynamic persistence.
		return policyResults, nil
	}

	// Partition tool calls into built-in and dynamic.
	var builtinCalls, dynamicCalls []fantasy.ToolCallContent
	if len(opts.DynamicToolNames) > 0 {
		for _, tc := range result.toolCalls {
			if opts.DynamicToolNames[tc.ToolName] {
				dynamicCalls = append(dynamicCalls, tc)
			} else {
				builtinCalls = append(builtinCalls, tc)
			}
		}
	} else {
		builtinCalls = result.toolCalls
	}

	// Execute only built-in tools.
	toolResults := executeTools(ctx, opts.Tools, opts.ActiveTools, opts.ProviderTools, builtinCalls, opts.Metrics, opts.Logger, provider, modelName, opts.BuiltinToolNames, func(tr fantasy.ToolResultContent, completedAt time.Time) {
		recordToolResultTimestamp(result, tr.ToolCallID, completedAt)
		publishToolAttachments(ctx, opts.Logger, tr, completedAt, publishMessagePart)
		ssePart := chatprompt.PartFromContentWithLogger(ctx, opts.Logger, tr)
		ssePart.CreatedAt = &completedAt
		publishMessagePart(codersdk.ChatMessageRoleTool, ssePart)
	})
	for _, tr := range toolResults {
		result.content = append(result.content, tr)
	}

	// If dynamic tools were called, persist what we have
	// (assistant + built-in results) and exit so the caller can
	// execute them externally.
	if len(dynamicCalls) > 0 {
		// Strip Anthropic provider-executed tool calls without
		// matching results before persisting so the action-required
		// step does not carry a malformed tool-call history into
		// downstream provider requests.
		result.content = chatsanitize.SanitizeAnthropicProviderToolStepContent(
			ctx, opts.Logger, provider, modelName,
			"dynamic_tool_persist", step, result.finishReason, result.content,
		)
		if err := persistPendingDynamicStep(ctx, opts, result, stepStart, dynamicCalls); err != nil {
			return nil, err
		}
		tryCompactOnExit(ctx, opts, result.usage, result.providerMetadata)
		return nil, ErrDynamicToolCall
	}

	// Check for interruption after tool execution. Tools that
	// were canceled mid-flight produce error results via ctx
	// cancellation. Persist the full step (assistant blocks +
	// tool results) through the interrupt-safe path so nothing
	// is lost.
	if ctx.Err() != nil {
		if errors.Is(context.Cause(ctx), ErrInterrupted) {
			persistInterruptedStep(ctx, opts, result)
			return nil, ErrInterrupted
		}
		return nil, ctx.Err()
	}

	return toolResults, nil
}

// persistPendingDynamicStep persists a step that has pending dynamic
// tool calls awaiting external execution. Returns ErrInterrupted when
// persistence fails because the chat was interrupted.
func persistPendingDynamicStep(
	ctx context.Context,
	opts RunOptions,
	result *stepResult,
	stepStart time.Time,
	dynamicCalls []fantasy.ToolCallContent,
) error {
	pending := make([]PendingToolCall, 0, len(dynamicCalls))
	for _, dc := range dynamicCalls {
		pending = append(pending, PendingToolCall{
			ToolCallID: dc.ToolCallID,
			ToolName:   dc.ToolName,
			Args:       dc.Input,
		})
	}

	contextLimit := extractContextLimitWithFallback(result.providerMetadata, opts.ContextLimitFallback)

	if err := opts.PersistStep(ctx, PersistedStep{
		Content:                 result.content,
		Usage:                   result.usage,
		ContextLimit:            contextLimit,
		ProviderResponseID:      chatopenai.ExtractResponseIDIfStored(opts.ProviderOptions, result.providerMetadata),
		Runtime:                 time.Since(stepStart),
		PendingDynamicToolCalls: pending,
	}); err != nil {
		if errors.Is(err, ErrInterrupted) {
			persistInterruptedStep(ctx, opts, result)
			return ErrInterrupted
		}
		return xerrors.Errorf("persist step: %w", err)
	}
	return nil
}

// applyExclusiveToolPolicy checks whether toolCalls violate the
// exclusive-tool policy declared by exclusiveToolNames. When a
// violation is detected it synthesizes deterministic policy-error
// results for every tool call and records size/error metrics so the
// exclusivity failure mode is visible to operators. Returns
// (results, true) on violation; (nil, false) otherwise.
func applyExclusiveToolPolicy(
	toolCalls []fantasy.ToolCallContent,
	exclusiveToolNames map[string]bool,
	metrics *Metrics,
	provider, model string,
) ([]fantasy.ToolResultContent, bool) {
	blockingToolName, ok := firstExclusiveToolName(toolCalls, exclusiveToolNames)
	if !ok {
		return nil, false
	}
	results := exclusiveToolPolicyResults(toolCalls, exclusiveToolNames, blockingToolName)
	for _, tr := range results {
		recordToolResultMetrics(metrics, provider, model, tr)
	}
	return results, true
}

// recordToolResultMetrics observes tool result size and increments
// tool_errors_total when the result carries an error output. Mirrors
// the metric-recording defer in executeSingleTool so that synthetic
// results (e.g. exclusive-tool policy errors) contribute to operator
// visibility.
func recordToolResultMetrics(metrics *Metrics, provider, model string, tr fantasy.ToolResultContent) {
	if metrics == nil {
		return
	}
	label := tr.ToolName
	if label == "" {
		label = "unknown"
	}
	metrics.ToolResultSizeBytes.WithLabelValues(provider, model, label).Observe(
		float64(ToolResultSize(tr)),
	)
	if _, ok := tr.Result.(fantasy.ToolResultOutputContentError); ok {
		metrics.RecordToolError(provider, model, label)
	}
}

func firstExclusiveToolName(
	toolCalls []fantasy.ToolCallContent,
	exclusiveToolNames map[string]bool,
) (string, bool) {
	if len(toolCalls) <= 1 || len(exclusiveToolNames) == 0 {
		return "", false
	}

	for _, tc := range toolCalls {
		if exclusiveToolNames[tc.ToolName] {
			return tc.ToolName, true
		}
	}

	return "", false
}

func exclusiveToolPolicyResults(
	toolCalls []fantasy.ToolCallContent,
	exclusiveToolNames map[string]bool,
	blockingToolName string,
) []fantasy.ToolResultContent {
	results := make([]fantasy.ToolResultContent, len(toolCalls))
	for i, tc := range toolCalls {
		message := exclusiveToolSkippedErrorMessage(blockingToolName)
		if exclusiveToolNames[tc.ToolName] {
			message = exclusiveToolMustRunAloneErrorMessage(tc.ToolName)
		}
		results[i] = fantasy.ToolResultContent{
			ToolCallID: tc.ToolCallID,
			ToolName:   tc.ToolName,
			Result: fantasy.ToolResultOutputContentError{
				Error: xerrors.New(message),
			},
		}
	}
	return results
}

func exclusiveToolMustRunAloneErrorMessage(toolName string) string {
	return toolName + " must be called alone, without other tools in the same batch. Retry with only the " + toolName + " call."
}

func exclusiveToolSkippedErrorMessage(toolName string) string {
	return "this tool was skipped because " + toolName + " must run alone in its batch. Retry your tool calls without " + toolName + ", or call " + toolName + " separately first."
}

// executeSingleTool executes one tool call and converts the
// response into a ToolResultContent.
func executeSingleTool(
	ctx context.Context,
	toolMap map[string]fantasy.AgentTool,
	tc fantasy.ToolCallContent,
	metrics *Metrics,
	logger slog.Logger,
	provider, model string,
	builtinToolNames map[string]bool,
	activeTools []string,
	providerRunnerNames map[string]struct{},
) fantasy.ToolResultContent {
	result := fantasy.ToolResultContent{
		ToolCallID:       tc.ToolCallID,
		ToolName:         tc.ToolName,
		ProviderExecuted: false,
	}
	defer func() {
		metricLabel := tc.ToolName
		if metricLabel == "" {
			metricLabel = "unknown"
		}
		metrics.ToolResultSizeBytes.WithLabelValues(provider, model, metricLabel).Observe(
			float64(ToolResultSize(result)),
		)
		if _, ok := result.Result.(fantasy.ToolResultOutputContentError); ok {
			metrics.RecordToolError(provider, model, metricLabel)
		}
	}()

	_, isProviderRunner := providerRunnerNames[tc.ToolName]
	if !isProviderRunner && !isToolActive(tc.ToolName, activeTools) {
		result.Result = fantasy.ToolResultOutputContentError{
			Error: xerrors.New("Tool not active in this turn: " + tc.ToolName),
		}
		return result
	}

	tool, exists := toolMap[tc.ToolName]
	if !exists {
		result.Result = fantasy.ToolResultOutputContentError{
			Error: xerrors.New("Tool not found: " + tc.ToolName),
		}
		return result
	}

	logger.Debug(ctx, "tool execution",
		slog.F("tool_name", tc.ToolName),
		slog.F("tool_call_id", tc.ToolCallID),
		slog.F("builtin", builtinToolNames[tc.ToolName]),
		slog.F("is_provider_runner", isProviderRunner),
	)
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    tc.ToolCallID,
		Name:  tc.ToolName,
		Input: tc.Input,
	})
	if err != nil {
		result.Result = fantasy.ToolResultOutputContentError{
			Error: err,
		}
		result.ClientMetadata = resp.Metadata
		logger.Error(ctx, "tool execution failed",
			slog.F("tool_name", tc.ToolName),
			slog.F("tool_call_id", tc.ToolCallID),
			slog.Error(err),
		)
		return result
	}

	result.ClientMetadata = resp.Metadata
	switch {
	case resp.IsError:
		result.Result = fantasy.ToolResultOutputContentError{
			Error: xerrors.New(resp.Content),
		}
		logger.Info(ctx, "tool returned error result",
			slog.F("tool_name", tc.ToolName),
			slog.F("tool_call_id", tc.ToolCallID),
			slog.F("tool_error", resp.Content),
		)
	case resp.Type == "image" || resp.Type == "media":
		result.Result = fantasy.ToolResultOutputContentMedia{
			Data:      base64.StdEncoding.EncodeToString(resp.Data),
			MediaType: resp.MediaType,
			Text:      strings.ToValidUTF8(resp.Content, "\uFFFD"),
		}
	default:
		result.Result = fantasy.ToolResultOutputContentText{
			Text: strings.ToValidUTF8(resp.Content, "\uFFFD"),
		}
	}
	return result
}

// flushActiveState moves any in-progress text, reasoning, and
// tool calls from the active tracking maps into result.content
// and result.toolCalls. This is called on interruption so that
// partial content from an incomplete stream is available for
// persistence.
func flushActiveState(
	result *stepResult,
	activeText map[string]string,
	activeReasoning map[string]reasoningState,
	activeToolCalls map[string]*fantasy.ToolCallContent,
	toolNames map[string]string,
) {
	// Flush partial text content.
	for _, text := range activeText {
		if text != "" {
			result.content = append(result.content, fantasy.TextContent{Text: text})
		}
	}

	// Flush partial reasoning content.
	for _, rs := range activeReasoning {
		if rs.text != "" {
			result.content = append(result.content, fantasy.ReasoningContent{
				Text:             rs.text,
				ProviderMetadata: rs.options,
			})
		}
	}

	// Flush in-progress tool calls. These haven't received a
	// StreamPartTypeToolCall yet, so they only exist in
	// activeToolCalls. We add them to both content and toolCalls
	// so persistInterruptedStep can generate synthetic error
	// results for them.
	for id, tc := range activeToolCalls {
		if tc == nil {
			continue
		}
		// Prefer the tool name from the toolNames map since
		// ToolInputStart may provide a cleaner name.
		toolName := tc.ToolName
		if name, ok := toolNames[id]; ok && strings.TrimSpace(name) != "" {
			toolName = name
		}
		flushed := fantasy.ToolCallContent{
			ToolCallID:       tc.ToolCallID,
			ToolName:         toolName,
			Input:            tc.Input,
			ProviderExecuted: tc.ProviderExecuted,
		}
		result.content = append(result.content, flushed)
		result.toolCalls = append(result.toolCalls, flushed)
	}
}

// persistInterruptedStep saves durable content from a partial stream.
// Provider-executed calls without results are removed because their
// result metadata cannot be synthesized safely.
func persistInterruptedStep(
	ctx context.Context,
	opts RunOptions,
	result *stepResult,
) {
	if result == nil || (len(result.content) == 0 && len(result.toolCalls) == 0) {
		return
	}

	provider := ""
	modelName := ""
	if opts.Model != nil {
		provider = opts.Model.Provider()
		modelName = opts.Model.Model()
	}
	var sanitizeStats chatsanitize.AnthropicProviderToolSanitizationStats
	result.content, sanitizeStats = chatsanitize.SanitizeAnthropicProviderToolContent(provider, result.content)
	chatsanitize.LogAnthropicProviderToolSanitization(
		ctx, opts.Logger, "interrupted_persist", provider, modelName, sanitizeStats,
	)

	// Track which tool calls already have results in the content.
	answeredToolCalls := make(map[string]struct{})
	for _, c := range result.content {
		tr, ok := fantasy.AsContentType[fantasy.ToolResultContent](c)
		if ok && tr.ToolCallID != "" {
			answeredToolCalls[tr.ToolCallID] = struct{}{}
		}
	}

	// Copy existing timestamps and add result timestamps for
	// interrupted tool calls so the frontend can show partial
	// duration.
	toolCallCreatedAt := maps.Clone(result.toolCallCreatedAt)
	if toolCallCreatedAt == nil {
		toolCallCreatedAt = make(map[string]time.Time)
	}
	toolResultCreatedAt := maps.Clone(result.toolResultCreatedAt)
	if toolResultCreatedAt == nil {
		toolResultCreatedAt = make(map[string]time.Time)
	}

	// Build combined content: all accumulated content + synthetic
	// interrupted results for any unanswered tool calls.
	content := make([]fantasy.Content, 0, len(result.content))
	content = append(content, result.content...)

	interruptedAt := dbtime.Now()
	for _, tc := range result.toolCalls {
		if tc.ToolCallID == "" {
			continue
		}
		if _, exists := answeredToolCalls[tc.ToolCallID]; exists {
			continue
		}
		if chatsanitize.IsAnthropicProviderExecutedToolCall(provider, tc) {
			continue
		}
		content = append(content, fantasy.ToolResultContent{
			ToolCallID:       tc.ToolCallID,
			ToolName:         tc.ToolName,
			ProviderExecuted: tc.ProviderExecuted,
			Result: fantasy.ToolResultOutputContentError{
				Error: xerrors.New(interruptedToolResultErrorMessage),
			},
		})
		// Only stamp synthetic results; don't clobber
		// timestamps from tools that completed before
		// the interruption arrived.
		if _, exists := toolResultCreatedAt[tc.ToolCallID]; !exists {
			toolResultCreatedAt[tc.ToolCallID] = interruptedAt
		}
		answeredToolCalls[tc.ToolCallID] = struct{}{}
	}

	if len(content) == 0 {
		return
	}

	persistCtx := context.WithoutCancel(ctx)
	if err := opts.PersistStep(persistCtx, PersistedStep{
		Content:             content,
		ToolCallCreatedAt:   toolCallCreatedAt,
		ToolResultCreatedAt: toolResultCreatedAt,
	}); err != nil {
		if opts.OnInterruptedPersistError != nil {
			opts.OnInterruptedPersistError(err)
		}
	}
}

// tryCompactOnExit runs compaction when the chatloop is about
// to exit early (e.g. via ErrDynamicToolCall). The normal
// inline and post-run compaction paths are unreachable in
// early-exit scenarios, so this ensures the context window
// doesn't grow unbounded.
func tryCompactOnExit(
	ctx context.Context,
	opts RunOptions,
	usage fantasy.Usage,
	metadata fantasy.ProviderMetadata,
) {
	if opts.Compaction == nil || opts.ReloadMessages == nil {
		return
	}
	reloaded, err := opts.ReloadMessages(ctx)
	if err != nil {
		return
	}
	did, compactErr := tryCompact(
		ctx,
		opts.Model,
		opts.Compaction,
		opts.ContextLimitFallback,
		usage,
		metadata,
		reloaded,
	)
	opts.Metrics.RecordCompaction(opts.Model.Provider(), opts.Model.Model(), did, compactErr)
	if compactErr != nil && opts.Compaction.OnError != nil {
		opts.Compaction.OnError(compactErr)
	}
}

func isToolActive(name string, activeTools []string) bool {
	return len(activeTools) == 0 || slices.Contains(activeTools, name)
}

// buildToolDefinitions converts AgentTool definitions into the
// fantasy.Tool slice expected by fantasy.Call. When activeTools
// is non-empty, only function tools whose name appears in the
// list are included. Provider tool definitions are always
// appended unconditionally.
func buildToolDefinitions(tools []fantasy.AgentTool, activeTools []string, providerTools []ProviderTool) []fantasy.Tool {
	prepared := make([]fantasy.Tool, 0, len(tools)+len(providerTools))
	for _, tool := range tools {
		info := tool.Info()
		if !isToolActive(info.Name, activeTools) {
			continue
		}

		inputSchema := map[string]any{
			"type":       "object",
			"properties": info.Parameters,
		}
		// Only include "required" when non-empty so that a nil slice
		// never serializes to null, which OpenAI rejects.
		if len(info.Required) > 0 {
			inputSchema["required"] = info.Required
		}
		schema.Normalize(inputSchema)
		prepared = append(prepared, fantasy.FunctionTool{
			Name:            info.Name,
			Description:     info.Description,
			InputSchema:     inputSchema,
			ProviderOptions: tool.ProviderOptions(),
		})
	}
	for _, pt := range providerTools {
		prepared = append(prepared, pt.Definition)
	}
	return prepared
}

// shouldStopAfterTools returns true if any tool result in the
// slice matches a name in stopTools and produced a successful
// (non-error) result.
func shouldStopAfterTools(stopTools map[string]struct{}, results []fantasy.ToolResultContent) bool {
	if len(stopTools) == 0 {
		return false
	}
	for _, tr := range results {
		if _, ok := stopTools[tr.ToolName]; !ok {
			continue
		}
		if _, isErr := tr.Result.(fantasy.ToolResultOutputContentError); !isErr {
			return true
		}
	}
	return false
}

func shouldApplyAnthropicPromptCaching(model fantasy.LanguageModel) bool {
	if model == nil {
		return false
	}
	return model.Provider() == fantasyanthropic.Name
}

// addAnthropicPromptCaching mutates messages in-place, setting
// ProviderOptions for Anthropic prompt caching on the last system
// message and the final two messages.
func addAnthropicPromptCaching(messages []fantasy.Message) {
	for i := range messages {
		messages[i].ProviderOptions = nil
	}

	providerOption := fantasy.ProviderOptions{
		fantasyanthropic.Name: &fantasyanthropic.ProviderCacheControlOptions{
			CacheControl: fantasyanthropic.CacheControl{Type: "ephemeral"},
		},
	}

	lastSystemRoleIdx := -1
	systemMessageUpdated := false
	for i, msg := range messages {
		if msg.Role == fantasy.MessageRoleSystem {
			lastSystemRoleIdx = i
		} else if !systemMessageUpdated && lastSystemRoleIdx >= 0 {
			messages[lastSystemRoleIdx].ProviderOptions = providerOption
			systemMessageUpdated = true
		}
		if i > len(messages)-3 {
			messages[i].ProviderOptions = providerOption
		}
	}
}

// recordToolResultTimestamp lazily initializes the
// toolResultCreatedAt map on the stepResult and records
// the completion timestamp for the given tool-call ID.
func recordToolResultTimestamp(result *stepResult, toolCallID string, ts time.Time) {
	if result.toolResultCreatedAt == nil {
		result.toolResultCreatedAt = make(map[string]time.Time)
	}
	result.toolResultCreatedAt[toolCallID] = ts
}

func publishToolAttachments(
	ctx context.Context,
	logger slog.Logger,
	tr fantasy.ToolResultContent,
	createdAt time.Time,
	publishMessagePart func(codersdk.ChatMessageRole, codersdk.ChatMessagePart),
) {
	attachments, err := chattool.AttachmentsFromMetadata(tr.ClientMetadata)
	if err != nil {
		logger.Warn(ctx, "skipping malformed tool attachment metadata",
			slog.F("tool_name", tr.ToolName),
			slog.F("tool_call_id", tr.ToolCallID),
			slog.Error(err),
		)
		return
	}
	for _, attachment := range attachments {
		filePart := codersdk.ChatMessageFile(
			attachment.FileID,
			attachment.MediaType,
			attachment.Name,
		)
		filePart.CreatedAt = &createdAt
		publishMessagePart(codersdk.ChatMessageRoleAssistant, filePart)
	}
}

func extractContextLimit(metadata fantasy.ProviderMetadata) sql.NullInt64 {
	if len(metadata) == 0 {
		return sql.NullInt64{}
	}

	encoded, err := json.Marshal(metadata)
	if err != nil || len(encoded) == 0 {
		return sql.NullInt64{}
	}

	var payload any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return sql.NullInt64{}
	}

	limit, ok := findContextLimitValue(payload)
	if !ok {
		return sql.NullInt64{}
	}

	return sql.NullInt64{
		Int64: limit,
		Valid: true,
	}
}

func extractContextLimitWithFallback(metadata fantasy.ProviderMetadata, fallback int64) sql.NullInt64 {
	contextLimit := extractContextLimit(metadata)
	if contextLimit.Valid || fallback <= 0 {
		return contextLimit
	}
	return sql.NullInt64{
		Int64: fallback,
		Valid: true,
	}
}

func findContextLimitValue(value any) (int64, bool) {
	var (
		limit int64
		found bool
	)

	collectContextLimitValues(value, func(candidate int64) {
		if !found || candidate > limit {
			limit = candidate
			found = true
		}
	})

	return limit, found
}

func collectContextLimitValues(value any, onValue func(int64)) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isContextLimitKey(key) {
				if numeric, ok := numericContextLimitValue(child); ok {
					onValue(numeric)
				}
			}
			collectContextLimitValues(child, onValue)
		}
	case []any:
		for _, child := range typed {
			collectContextLimitValues(child, onValue)
		}
	}
}

func isContextLimitKey(key string) bool {
	normalized := normalizeMetadataKey(key)
	if normalized == "" {
		return false
	}

	switch normalized {
	case
		"contextlimit",
		"contextwindow",
		"contextlength",
		"maxcontext",
		"maxcontexttokens",
		"maxinputtokens",
		"maxinputtoken",
		"inputtokenlimit":
		return true
	}

	words := metadataKeyWords(key)
	if !slices.Contains(words, "context") {
		return false
	}

	if slices.Contains(words, "limit") {
		return true
	}

	if slices.Contains(words, "window") {
		return slices.Contains(words, "size") || slices.Contains(words, "max")
	}

	if slices.Contains(words, "length") {
		return slices.Contains(words, "max")
	}

	return (slices.Contains(words, "token") || slices.Contains(words, "tokens")) &&
		(slices.Contains(words, "max") || slices.Contains(words, "limit"))
}

func normalizeMetadataKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))

	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
			_, _ = b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			_, _ = b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			_, _ = b.WriteRune(r)
		}
	}

	return b.String()
}

func metadataKeyWords(key string) []string {
	words := make([]string, 0, 4)
	var current strings.Builder

	flush := func() {
		if current.Len() == 0 {
			return
		}
		words = append(words, current.String())
		current.Reset()
	}

	var prev rune
	var hasPrev bool
	for _, r := range key {
		if !unicode.IsLetter(r) {
			flush()
			hasPrev = false
			continue
		}

		if hasPrev && unicode.IsUpper(r) && unicode.IsLower(prev) {
			flush()
		}

		_, _ = current.WriteRune(unicode.ToLower(r))
		prev = r
		hasPrev = true
	}

	flush()
	return words
}

func numericContextLimitValue(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return positiveInt64(typed)
	case int32:
		return positiveInt64(int64(typed))
	case int:
		return positiveInt64(int64(typed))
	case float64:
		casted := int64(typed)
		if typed > 0 && float64(casted) == typed {
			return casted, true
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return positiveInt64(parsed)
		}
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return positiveInt64(parsed)
		}
	}

	return 0, false
}

func positiveInt64(value int64) (int64, bool) {
	if value <= 0 {
		return 0, false
	}
	return value, true
}
