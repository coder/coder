package chatloop

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/schema"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	// defaultStreamSilenceTimeout bounds how long an individual
	// model attempt may go without receiving a stream part before
	// the attempt is canceled and retried.
	defaultStreamSilenceTimeout = 10 * time.Minute
	streamSilenceGuardTimerTag  = "streamSilenceGuard"
)

var (
	ErrInterrupted     = xerrors.New("chat interrupted")
	ErrDynamicToolCall = xerrors.New("dynamic tool call")
	// ErrStopAfterTool is returned when a tool listed in
	// StopAfterTools produces a successful result, indicating
	// the run should terminate cleanly after persistence.
	ErrStopAfterTool = xerrors.New("stop after tool")

	errStreamSilenceTimeout = xerrors.New(
		"chat stream was silent for longer than the configured timeout",
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

// PersistedStep is the unit the persistence layer splits into role-separated
// database messages. Content mixes assistant blocks (text, reasoning, tool
// calls) and tool result blocks from one completed or interrupted agent step.
type PersistedStep struct {
	Content      []fantasy.Content
	Usage        fantasy.Usage
	ContextLimit sql.NullInt64
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
	// ReasoningStartedAt and ReasoningCompletedAt are parallel
	// slices indexed by the occurrence order of reasoning
	// content in Content. The persistence layer walks reasoning
	// parts in order and applies these timestamps to the
	// corresponding ChatMessageParts so the frontend can render
	// reasoning duration. Reasoning parts have no provider-side
	// stable ID, so order is the only correlation we have.
	ReasoningStartedAt   []time.Time
	ReasoningCompletedAt []time.Time
}

// RunOptions configures a single streaming chat loop run.
type RunOptions struct {
	Model    fantasy.LanguageModel
	Messages []fantasy.Message
	Tools    []fantasy.AgentTool
	MaxSteps int
	// StreamSilenceTimeout bounds how long each model attempt
	// may go without receiving a stream part before the
	// attempt is canceled and retried. Zero uses the
	// production default.
	StreamSilenceTimeout time.Duration
	// Clock creates stream silence guard timers. In production
	// use a real clock; tests can inject quartz.NewMock(t) to
	// make timeout behavior deterministic.
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
	Logger     slog.Logger
	Compaction *CompactionOptions

	// PrepareTools is called once before each LLM step with the
	// current tool list. If it returns non-nil, the returned slice
	// replaces opts.Tools for this and all subsequent steps, and any
	// new tool names are appended to opts.ActiveTools so they become
	// callable immediately. Used to inject tools that become available
	// mid-turn (e.g. workspace MCP tools discovered after
	// create_workspace).
	//
	// The chatloop tracks whether tools have already been replaced so
	// PrepareTools is not retried on subsequent steps once it has
	// returned a non-nil slice. Callbacks may still be invoked on later
	// steps when they previously returned nil.
	PrepareTools func([]fantasy.AgentTool) []fantasy.AgentTool

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

// GenerateAssistantOptions configures one assistant model call.
type GenerateAssistantOptions struct {
	Model fantasy.LanguageModel
	// ErrorProvider labels user-facing errors with the configured provider
	// identity (e.g. "bedrock"). It differs from Model.Provider(), which
	// reflects the fantasy transport client and is "anthropic" for Bedrock
	// routed through aibridge. Metrics and prompt preparation keep using
	// Model.Provider(). When empty, Model.Provider() is used.
	ErrorProvider        string
	Messages             []fantasy.Message
	Tools                []fantasy.AgentTool
	ActiveTools          []string
	ProviderTools        []ProviderTool
	StreamSilenceTimeout time.Duration
	Clock                quartz.Clock

	ContextLimitFallback int64
	ModelConfig          codersdk.ChatModelCallConfig
	ProviderOptions      fantasy.ProviderOptions

	PublishMessagePart func(codersdk.ChatMessageRole, codersdk.ChatMessagePart)
	Logger             slog.Logger
	Metrics            *Metrics
}

// AssistantOutcome is the durable assistant-side result from one model call.
type AssistantOutcome struct {
	Step         PersistedStep
	ToolCalls    []fantasy.ToolCallContent
	FinishReason fantasy.FinishReason
	ModelStopped bool
}

// ExecuteLocalToolsOptions configures one local tool execution batch.
type ExecuteLocalToolsOptions struct {
	Tools         []fantasy.AgentTool
	ActiveTools   []string
	ProviderTools []ProviderTool
	ToolCalls     []fantasy.ToolCallContent

	ExclusiveToolNames map[string]bool
	BuiltinToolNames   map[string]bool
	ModelProvider      string
	ModelName          string

	// ContextLimit is the model's context window in tokens. It is used
	// to derive a per-result byte budget so a single oversized tool
	// result cannot overflow the prompt. Zero means unknown, in which
	// case a default budget applies.
	ContextLimit int64

	// ToolNameAliases maps a non-advertised tool name to the canonical
	// tool it dispatches to. Used for backward compatibility when a tool
	// is renamed but old chat histories still reference the old name.
	ToolNameAliases map[string]string

	PublishMessagePart func(codersdk.ChatMessageRole, codersdk.ChatMessagePart)
	Logger             slog.Logger
	Metrics            *Metrics
	Clock              quartz.Clock
}

// ToolExecutionOutcome is the durable tool-result content from one batch.
type ToolExecutionOutcome struct {
	Step PersistedStep
}

// GenerateCompactionOptions configures one context compaction call.
type GenerateCompactionOptions struct {
	Model    fantasy.LanguageModel
	Messages []fantasy.Message

	ThresholdPercent     int32
	ContextLimit         int64
	ContextLimitFallback int64
	SummaryPrompt        string
	SystemSummaryPrefix  string
	Timeout              time.Duration
	StepUsage            fantasy.Usage
	StepMetadata         fantasy.ProviderMetadata

	DebugSvc            *chatdebug.Service
	ChatID              uuid.UUID
	HistoryTipMessageID int64
	ToolCallID          string
	ToolName            string

	PublishMessagePart func(codersdk.ChatMessageRole, codersdk.ChatMessagePart)
}

// ProviderTool pairs a provider-native tool definition with an
// optional local executor. When Runner is nil the tool is fully
// provider-executed (e.g. web search). When Runner is non-nil
// the definition is sent to the API but execution is handled
// locally (e.g. computer use).
type ProviderTool struct {
	Definition fantasy.Tool
	Runner     fantasy.AgentTool
	// ResultProviderMetadata extracts provider-specific metadata from successful
	// local runner responses. The chat loop attaches returned metadata to the tool
	// result sent back to the model. OpenAI computer-use uses this to request
	// original screenshot detail for image results.
	ResultProviderMetadata func(response fantasy.ToolResponse) fantasy.ProviderMetadata
}

// stepResult holds the accumulated output of a single streaming
// step. Since we own the stream consumer, all content is tracked
// directly here, no shadow draft state needed.
type stepResult struct {
	content              []fantasy.Content
	usage                fantasy.Usage
	providerMetadata     fantasy.ProviderMetadata
	finishReason         fantasy.FinishReason
	toolCalls            []fantasy.ToolCallContent
	shouldContinue       bool
	toolCallCreatedAt    map[string]time.Time
	toolResultCreatedAt  map[string]time.Time
	reasoningStartedAt   []time.Time
	reasoningCompletedAt []time.Time
}

// reasoningState accumulates reasoning content and provider
// metadata while the stream is in flight.
type reasoningState struct {
	text      string
	options   fantasy.ProviderMetadata
	startedAt time.Time
}

// GenerateAssistant performs one assistant model stream and returns the
// durable assistant-side content. It does not execute tools, retry, or persist.
func GenerateAssistant(ctx context.Context, opts GenerateAssistantOptions) (AssistantOutcome, error) {
	if opts.Model == nil {
		return AssistantOutcome{}, xerrors.New("chat model is required")
	}
	if opts.StreamSilenceTimeout <= 0 {
		opts.StreamSilenceTimeout = defaultStreamSilenceTimeout
	}
	if opts.Clock == nil {
		opts.Clock = quartz.NewReal()
	}
	if opts.Metrics == nil {
		opts.Metrics = NopMetrics()
	}

	publishMessagePart := func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
		if opts.PublishMessagePart != nil {
			opts.PublishMessagePart(role, part)
		}
	}

	provider := opts.Model.Provider()
	modelName := opts.Model.Model()
	// errorProvider labels user-facing errors with the configured provider;
	// see GenerateAssistantOptions.ErrorProvider. The transport provider is
	// kept for prompt preparation, Anthropic history sanitization, and the
	// metric labels below.
	errorProvider := cmp.Or(opts.ErrorProvider, provider)
	runOpts := RunOptions{
		Model:  opts.Model,
		Logger: opts.Logger,
	}
	_, prepared, err := prepareMessagesForRequest(ctx, runOpts, opts.Messages, provider, modelName, 0, 1)
	if err != nil {
		return AssistantOutcome{}, xerrors.Errorf("prepare prompt: %w", err)
	}
	opts.Metrics.MessageCount.WithLabelValues(provider, modelName).Observe(float64(len(prepared)))
	opts.Metrics.PromptSizeBytes.WithLabelValues(provider, modelName).Observe(float64(EstimatePromptSize(prepared)))
	opts.Metrics.StepsTotal.WithLabelValues(provider, modelName).Inc()

	call := fantasy.Call{
		Prompt:           prepared,
		Tools:            buildToolDefinitions(opts.Tools, opts.ActiveTools, opts.ProviderTools),
		MaxOutputTokens:  opts.ModelConfig.MaxOutputTokens,
		Temperature:      opts.ModelConfig.Temperature,
		TopP:             opts.ModelConfig.TopP,
		TopK:             opts.ModelConfig.TopK,
		PresencePenalty:  opts.ModelConfig.PresencePenalty,
		FrequencyPenalty: opts.ModelConfig.FrequencyPenalty,
		ProviderOptions:  opts.ProviderOptions,
	}

	stepStart := opts.Clock.Now()
	stepCtx := chatdebug.ReuseStep(ctx)
	attempt, streamErr := guardedStream(
		stepCtx,
		provider,
		modelName,
		opts.Clock,
		opts.StreamSilenceTimeout,
		func(attemptCtx context.Context) (fantasy.StreamResponse, error) {
			return opts.Model.Stream(attemptCtx, call)
		},
		opts.Metrics,
	)
	if streamErr != nil {
		wrappedErr := wrapProviderStreamError(errorProvider, streamErr)
		classified := chaterror.Classify(wrappedErr).WithProvider(errorProvider)
		if classified.Retryable {
			opts.Metrics.RecordStreamRetry(provider, modelName, classified)
		}
		return AssistantOutcome{}, wrappedErr
	}
	defer attempt.release()

	result, processErr := processStepStream(attempt.ctx, attempt.stream, opts.Clock, publishMessagePart)
	if err := attempt.finish(processErr); err != nil {
		if errors.Is(err, ErrInterrupted) {
			return AssistantOutcome{}, ErrInterrupted
		}
		wrappedErr := wrapProviderStreamError(errorProvider, err)
		classified := chaterror.Classify(wrappedErr).WithProvider(errorProvider)
		if classified.Retryable {
			opts.Metrics.RecordStreamRetry(provider, modelName, classified)
		}
		return AssistantOutcome{}, wrappedErr
	}

	contextLimit := extractContextLimitWithFallback(result.providerMetadata, opts.ContextLimitFallback)
	result.content = chatsanitize.SanitizeAnthropicProviderToolStepContent(
		ctx, opts.Logger, provider, modelName,
		"assistant_helper", 0, result.finishReason, result.content,
	)
	step := PersistedStep{
		Content:              result.content,
		Usage:                result.usage,
		ContextLimit:         contextLimit,
		Runtime:              opts.Clock.Since(stepStart),
		ToolCallCreatedAt:    result.toolCallCreatedAt,
		ToolResultCreatedAt:  result.toolResultCreatedAt,
		ReasoningStartedAt:   result.reasoningStartedAt,
		ReasoningCompletedAt: result.reasoningCompletedAt,
	}
	return AssistantOutcome{
		Step:         step,
		ToolCalls:    append([]fantasy.ToolCallContent(nil), result.toolCalls...),
		FinishReason: result.finishReason,
		ModelStopped: len(result.content) == 0,
	}, nil
}

func wrapProviderStreamError(provider string, err error) error {
	if err == nil {
		return nil
	}
	classified := chaterror.Classify(err).WithProvider(provider)
	if !classified.Retryable && classified.StatusCode == 0 && errors.Is(err, context.Canceled) {
		wrapped := errors.Join(chaterror.ErrProviderTransportReset, err)
		reclassified := chaterror.Classify(wrapped).WithProvider(provider)
		if reclassified.Retryable {
			classified = reclassified
			err = wrapped
		}
	}
	return xerrors.Errorf("stream response: %w", chaterror.WithClassification(err, classified))
}

// ExecuteLocalTools runs local tool calls and returns durable tool results. It
// does not retry or persist.
func ExecuteLocalTools(ctx context.Context, opts ExecuteLocalToolsOptions) (ToolExecutionOutcome, error) {
	if opts.Metrics == nil {
		opts.Metrics = NopMetrics()
	}
	provider := opts.ModelProvider
	if provider == "" {
		provider = "unknown"
	}
	modelName := opts.ModelName
	if modelName == "" {
		modelName = "unknown"
	}
	publishMessagePart := func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
		if opts.PublishMessagePart != nil {
			opts.PublishMessagePart(role, part)
		}
	}
	// Expose the publisher on the execution context so tools that stream
	// intermediate output (e.g. the advisor tool) can publish parts
	// without capturing the publisher at construction time.
	ctx = WithMessagePartPublisher(ctx, opts.PublishMessagePart)
	if ctx.Err() != nil {
		return ToolExecutionOutcome{}, ctx.Err()
	}

	localCalls := make([]fantasy.ToolCallContent, 0, len(opts.ToolCalls))
	for _, tc := range opts.ToolCalls {
		if !tc.ProviderExecuted {
			localCalls = append(localCalls, tc)
		}
	}
	if len(localCalls) == 0 {
		return ToolExecutionOutcome{}, nil
	}

	var result stepResult
	policyResults, exclusiveViolation := applyExclusiveToolPolicy(
		localCalls,
		opts.ExclusiveToolNames,
		opts.Metrics,
		provider,
		modelName,
	)
	if exclusiveViolation {
		now := clockNow(opts.Clock)
		for _, tr := range policyResults {
			recordToolResultTimestamp(&result, tr.ToolCallID, now)
			publishToolAttachments(ctx, opts.Logger, tr, now, publishMessagePart)
			ssePart := chatprompt.PartFromContentWithLogger(ctx, opts.Logger, tr)
			ssePart.CreatedAt = &now
			publishMessagePart(codersdk.ChatMessageRoleTool, ssePart)
			result.content = append(result.content, tr)
		}
		if ctx.Err() != nil {
			return ToolExecutionOutcome{}, ctx.Err()
		}
		return ToolExecutionOutcome{Step: PersistedStep{
			Content:             result.content,
			ToolResultCreatedAt: result.toolResultCreatedAt,
		}}, nil
	}

	maxResultBytes := toolResultByteBudget(opts.ContextLimit)
	toolResults := executeTools(
		ctx,
		opts.Clock,
		opts.Tools,
		opts.ActiveTools,
		opts.ProviderTools,
		localCalls,
		opts.Metrics,
		opts.Logger,
		provider,
		modelName,
		opts.BuiltinToolNames,
		maxResultBytes,
		opts.ToolNameAliases,
		func(tr fantasy.ToolResultContent, completedAt time.Time) {
			recordToolResultTimestamp(&result, tr.ToolCallID, completedAt)
			publishToolAttachments(ctx, opts.Logger, tr, completedAt, publishMessagePart)
			ssePart := chatprompt.PartFromContentWithLogger(ctx, opts.Logger, tr)
			ssePart.CreatedAt = &completedAt
			publishMessagePart(codersdk.ChatMessageRoleTool, ssePart)
		},
	)
	if ctx.Err() != nil {
		return ToolExecutionOutcome{}, ctx.Err()
	}
	for _, tr := range toolResults {
		result.content = append(result.content, tr)
	}
	return ToolExecutionOutcome{Step: PersistedStep{
		Content:             result.content,
		ToolResultCreatedAt: result.toolResultCreatedAt,
	}}, nil
}

// prepareMessagesForRequest applies the prompt preparation pipeline used
// immediately before sending messages to a provider. It returns the
// possibly updated canonical messages and an independent provider-ready
// prompt. When preparation fails, the prompt result is nil and err is the
// terminal prompt-preparation failure.
func prepareMessagesForRequest(
	ctx context.Context,
	opts RunOptions,
	messages []fantasy.Message,
	provider string,
	modelName string,
	step int,
	totalSteps int,
) (canonical []fantasy.Message, prompt []fantasy.Message, err error) {
	canonical = messages
	// Copy messages so provider-specific caching mutations don't leak
	// back to the canonical message slice.
	prompt = slices.Clone(canonical)
	prompt, sanitizeStats := chatsanitize.SanitizeAnthropicProviderToolHistory(provider, prompt)
	chatsanitize.LogAnthropicProviderToolSanitization(
		ctx, opts.Logger, "pre_request", provider, modelName, sanitizeStats,
		slog.F("step_index", step),
		slog.F("total_steps", totalSteps),
	)
	prompt, err = chatsanitize.ApplyAnthropicProviderToolGuard(
		ctx, opts.Logger, provider, modelName, prompt,
	)
	if err != nil {
		err = chaterror.WithClassification(
			xerrors.Errorf("apply anthropic provider tool guard: %w", err),
			chaterror.ClassifiedError{
				Message:   "The chat continuation failed due to an internal state mismatch. This is not a configuration or billing issue. Start a new chat to continue.",
				Detail:    "Anthropic replay diagnostic: match=provider_tool_guard_postcondition_failed.",
				Kind:      codersdk.ChatErrorKindGeneric,
				Provider:  provider,
				Retryable: false,
			},
		)
		return canonical, nil, err
	}
	if shouldApplyAnthropicPromptCaching(opts.Model) {
		addAnthropicPromptCaching(prompt)
	}
	return canonical, prompt, nil
}

// guardedAttempt owns an attempt-scoped context and silence guard
// around a provider stream. release is idempotent and frees the
// attempt-scoped timer/context. finish canonicalizes silence timeout
// errors before the retry loop classifies them.
type guardedAttempt struct {
	ctx     context.Context
	stream  fantasy.StreamResponse
	release func()
	finish  func(error) error
}

// streamSilenceGuard arbitrates whether an attempt times out while
// waiting for the next stream part. Exactly one outcome wins: the
// timer cancels the attempt, or release disarms the timer.
type streamSilenceGuard struct {
	mu      sync.Mutex
	timer   *quartz.Timer
	cancel  context.CancelCauseFunc
	timeout time.Duration
	settled bool
}

func newStreamSilenceGuard(
	clock quartz.Clock,
	timeout time.Duration,
	cancel context.CancelCauseFunc,
) *streamSilenceGuard {
	guard := &streamSilenceGuard{
		cancel:  cancel,
		timeout: timeout,
	}
	guard.timer = clock.AfterFunc(
		timeout,
		guard.onTimeout,
		streamSilenceGuardTimerTag,
	)
	return guard
}

func (g *streamSilenceGuard) settle() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.settled {
		return false
	}
	g.settled = true
	return true
}

func (g *streamSilenceGuard) onTimeout() {
	if !g.settle() {
		return
	}
	g.cancel(errStreamSilenceTimeout)
}

func (g *streamSilenceGuard) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.settled {
		return
	}
	g.timer.Reset(g.timeout, streamSilenceGuardTimerTag)
}

func (g *streamSilenceGuard) Disarm() {
	if !g.settle() {
		return
	}
	g.timer.Stop()
}

func classifyStreamSilenceTimeout(
	attemptCtx context.Context,
	provider string,
	err error,
) error {
	if !errors.Is(context.Cause(attemptCtx), errStreamSilenceTimeout) {
		return err
	}
	if err == nil {
		err = errStreamSilenceTimeout
	}
	return chaterror.WithClassification(err, chaterror.ClassifiedError{
		Kind:      codersdk.ChatErrorKindStreamSilenceTimeout,
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
	guard := newStreamSilenceGuard(clock, timeout, cancelAttempt)
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
		err = classifyStreamSilenceTimeout(attemptCtx, provider, err)
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
				guard.Reset()
				recordTTFT()
				if !yield(part) {
					return
				}
			}
		}),
		release: release,
		finish: func(err error) error {
			return classifyStreamSilenceTimeout(attemptCtx, provider, err)
		},
	}, nil
}

// clockNow returns the clock's current time normalized the same
// way as dbtime.Now so persisted timestamps are Postgres-safe.
func clockNow(clock quartz.Clock) time.Time {
	return dbtime.Time(clock.Now().UTC())
}

// processStepStream consumes a fantasy StreamResponse and
// accumulates all content into a stepResult. Callbacks fire
// inline and their errors propagate directly.
func processStepStream(
	ctx context.Context,
	stream fantasy.StreamResponse,
	clock quartz.Clock,
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
				text:      part.Delta,
				options:   part.ProviderMetadata,
				startedAt: clockNow(clock),
			}

		case fantasy.StreamPartTypeReasoningDelta:
			reasoningPart := codersdk.ChatMessageReasoning(part.Delta)
			if active, exists := activeReasoningContent[part.ID]; exists {
				active.text += part.Delta
				if len(part.ProviderMetadata) > 0 {
					active.options = part.ProviderMetadata
				}
				activeReasoningContent[part.ID] = active
				if !active.startedAt.IsZero() {
					startedAt := active.startedAt
					reasoningPart.CreatedAt = &startedAt
				}
			}
			publishMessagePart(codersdk.ChatMessageRoleAssistant, reasoningPart)

		case fantasy.StreamPartTypeReasoningEnd:
			if active, exists := activeReasoningContent[part.ID]; exists {
				if len(part.ProviderMetadata) > 0 {
					active.options = part.ProviderMetadata
				}
				content := fantasy.ReasoningContent{
					Text:             active.text,
					ProviderMetadata: active.options,
				}
				result.content = append(result.content, content)
				result.reasoningStartedAt = append(result.reasoningStartedAt, active.startedAt)
				result.reasoningCompletedAt = append(result.reasoningCompletedAt, clockNow(clock))
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
			now := clockNow(clock)
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

				now := clockNow(clock)
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
					clock,
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
			clock,
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
	clock quartz.Clock,
	allTools []fantasy.AgentTool,
	activeTools []string,
	providerTools []ProviderTool,
	toolCalls []fantasy.ToolCallContent,
	metrics *Metrics,
	logger slog.Logger,
	provider, model string,
	builtinToolNames map[string]bool,
	maxResultBytes int,
	toolNameAliases map[string]string,
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
	resultProviderMetadata := make(
		map[string]func(fantasy.ToolResponse) fantasy.ProviderMetadata,
		len(providerTools),
	)
	// Include runners from provider tools so locally-executed
	// provider tools (e.g. computer use) can be dispatched.
	for _, pt := range providerTools {
		if pt.Runner == nil {
			continue
		}

		name := pt.Runner.Info().Name
		toolMap[name] = pt.Runner
		providerRunnerNames[name] = struct{}{}
		if pt.ResultProviderMetadata != nil {
			resultProviderMetadata[name] = pt.ResultProviderMetadata
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
				completedAt[i] = clockNow(clock)
			}()
			results[i] = executeSingleTool(
				ctx,
				toolMap,
				tc,
				metrics,
				logger,
				provider,
				model,
				builtinToolNames,
				activeTools,
				providerRunnerNames,
				resultProviderMetadata,
				maxResultBytes,
				toolNameAliases,
			)
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
	resultProviderMetadata map[string]func(fantasy.ToolResponse) fantasy.ProviderMetadata,
	maxResultBytes int,
	toolNameAliases map[string]string,
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

	// Resolve backward-compatible tool aliases (for example a renamed
	// tool whose old name still appears in chat history) to the canonical
	// tool before the active-tool and dispatch lookups.
	resolvedName := tc.ToolName
	if alias, ok := toolNameAliases[tc.ToolName]; ok {
		resolvedName = alias
	}

	_, isProviderRunner := providerRunnerNames[resolvedName]
	if !isProviderRunner && !isToolActive(resolvedName, activeTools) {
		result.Result = fantasy.ToolResultOutputContentError{
			Error: xerrors.New("Tool not active in this turn: " + resolvedName),
		}
		return result
	}

	tool, exists := toolMap[resolvedName]
	if !exists {
		result.Result = fantasy.ToolResultOutputContentError{
			Error: xerrors.New("Tool not found: " + resolvedName),
		}
		return result
	}

	logger.Debug(ctx, "tool execution",
		slog.F("tool_name", tc.ToolName),
		slog.F("resolved_tool_name", resolvedName),
		slog.F("tool_call_id", tc.ToolCallID),
		slog.F("builtin", builtinToolNames[resolvedName]),
		slog.F("is_provider_runner", isProviderRunner),
	)
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    tc.ToolCallID,
		Name:  resolvedName,
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

	// Cap tool output so a single oversized result (most often a large
	// MCP response) cannot overflow the model's context window on the
	// next request. Only the text payload is bounded; binary media data
	// is passed through untouched.
	content := resp.Content
	if truncated, didTruncate := truncateToolResultText(content, maxResultBytes); didTruncate {
		metrics.RecordToolResultTruncated(provider, model, tc.ToolName)
		logger.Warn(ctx, "tool result truncated to fit model context",
			slog.F("tool_name", tc.ToolName),
			slog.F("tool_call_id", tc.ToolCallID),
			slog.F("original_bytes", len(content)),
			slog.F("max_bytes", maxResultBytes),
		)
		content = truncated
	}

	switch {
	case resp.IsError:
		result.Result = fantasy.ToolResultOutputContentError{
			Error: xerrors.New(content),
		}
		logger.Info(ctx, "tool returned error result",
			slog.F("tool_name", tc.ToolName),
			slog.F("tool_call_id", tc.ToolCallID),
			slog.F("tool_error", content),
		)
	case resp.Type == "image" || resp.Type == "media":
		result.Result = fantasy.ToolResultOutputContentMedia{
			Data:      base64.StdEncoding.EncodeToString(resp.Data),
			MediaType: resp.MediaType,
			Text:      strings.ToValidUTF8(content, "\uFFFD"),
		}
	default:
		result.Result = fantasy.ToolResultOutputContentText{
			Text: strings.ToValidUTF8(content, "\uFFFD"),
		}
	}

	if _, isError := result.Result.(fantasy.ToolResultOutputContentError); isError {
		return result
	}
	if len(result.ProviderMetadata) == 0 {
		if callback := resultProviderMetadata[tc.ToolName]; callback != nil {
			metadata := callback(resp)
			if len(metadata) > 0 {
				result.ProviderMetadata = metadata
			}
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
	clock quartz.Clock,
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

	// Flush partial reasoning content. The matching
	// completedAt is filled in here with the interruption
	// time so partial reasoning shows the time spent before
	// the interruption.
	flushedAt := clockNow(clock)
	for _, rs := range activeReasoning {
		if rs.text == "" && !chatsanitize.HasAnthropicSignedReasoningOptions(fantasy.ProviderOptions(rs.options)) {
			continue
		}
		result.content = append(result.content, fantasy.ReasoningContent{
			Text:             rs.text,
			ProviderMetadata: rs.options,
		})
		result.reasoningStartedAt = append(result.reasoningStartedAt, rs.startedAt)
		result.reasoningCompletedAt = append(result.reasoningCompletedAt, flushedAt)
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
