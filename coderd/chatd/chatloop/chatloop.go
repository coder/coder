package chatloop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/schema"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/chatd/chatretry"
	"github.com/coder/coder/v2/codersdk"
)

const (
	interruptedToolResultErrorMessage = "tool call was interrupted before it produced a result"

	// maxCompactionRetries limits how many times the post-run
	// compaction safety net can re-enter the step loop. This
	// prevents infinite compaction loops when the model keeps
	// hitting the context limit after summarization.
	maxCompactionRetries = 3
)

var ErrInterrupted = xerrors.New("chat interrupted")

// PersistedStep contains the full content of a completed or
// interrupted agent step. Content includes both assistant blocks
// (text, reasoning, tool calls) and tool result blocks. The
// persistence layer is responsible for splitting these into
// separate database messages by role.
type PersistedStep struct {
	Content      []fantasy.Content
	Usage        fantasy.Usage
	ContextLimit sql.NullInt64
}

// RunOptions configures a single streaming chat loop run.
type RunOptions struct {
	Model    fantasy.LanguageModel
	Messages []fantasy.Message
	Tools    []fantasy.AgentTool
	MaxSteps int

	ActiveTools          []string
	ContextLimitFallback int64

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
	Compaction     *CompactionOptions
	ReloadMessages func(context.Context) ([]fantasy.Message, error)

	// OnRetry is called before each retry attempt when the LLM
	// stream fails with a retryable error. It provides the attempt
	// number, error, and backoff delay so callers can publish status
	// events to connected clients. Callers should also clear any
	// buffered stream state from the failed attempt in this callback
	// to avoid sending duplicated content.
	OnRetry chatretry.OnRetryFn

	OnInterruptedPersistError func(error)
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
// directly here — no shadow draft state needed.
type stepResult struct {
	content          []fantasy.Content
	usage            fantasy.Usage
	providerMetadata fantasy.ProviderMetadata
	finishReason     fantasy.FinishReason
	toolCalls        []fantasy.ToolCallContent
	shouldContinue   bool
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
			if !ok {
				continue
			}
			assistantParts = append(assistantParts, fantasy.TextPart{
				Text:            text.Text,
				ProviderOptions: fantasy.ProviderOptions(text.ProviderMetadata),
			})
		case fantasy.ContentTypeReasoning:
			reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](c)
			if !ok {
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
			// Copy messages so that provider-specific caching
			// mutations don't leak back to the caller's slice.
			// copy copies Message structs by value, so field
			// reassignments in addAnthropicPromptCaching only
			// affect the prepared slice.
			prepared := make([]fantasy.Message, len(messages))
			copy(prepared, messages)
			if applyAnthropicCaching {
				addAnthropicPromptCaching(prepared)
			}

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
			err := chatretry.Retry(ctx, func(retryCtx context.Context) error {
				stream, streamErr := opts.Model.Stream(retryCtx, call)
				if streamErr != nil {
					return streamErr
				}
				var processErr error
				result, processErr = processStepStream(retryCtx, stream, publishMessagePart)
				return processErr
			}, func(attempt int, retryErr error, delay time.Duration) {
				// Reset result from the failed attempt so the next
				// attempt starts clean.
				result = stepResult{}
				if opts.OnRetry != nil {
					opts.OnRetry(attempt, retryErr, delay)
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
				// Check for context cancellation before starting
				// tool execution. If the chat was interrupted
				// between stream completion and here, persist
				// what we have and bail out.
				if ctx.Err() != nil {
					if errors.Is(context.Cause(ctx), ErrInterrupted) {
						persistInterruptedStep(ctx, opts, &result)
						return ErrInterrupted
					}
					return ctx.Err()
				}

				toolResults = executeTools(ctx, opts.Tools, opts.ProviderTools, result.toolCalls, func(tr fantasy.ToolResultContent) {
					publishMessagePart(
						codersdk.ChatMessageRoleTool,
						chatprompt.PartFromContent(tr),
					)
				})
				for _, tr := range toolResults {
					result.content = append(result.content, tr)
				}

				// Check for interruption after tool execution.
				// Tools that were canceled mid-flight produce error
				// results via ctx cancellation. Persist the full
				// step (assistant blocks + tool results) through
				// the interrupt-safe path so nothing is lost.
				if ctx.Err() != nil {
					if errors.Is(context.Cause(ctx), ErrInterrupted) {
						persistInterruptedStep(ctx, opts, &result)
						return ErrInterrupted
					}
					return ctx.Err()
				}
			}
			// Extract context limit from provider metadata.
			contextLimit := extractContextLimit(result.providerMetadata)
			if !contextLimit.Valid && opts.ContextLimitFallback > 0 {
				contextLimit = sql.NullInt64{
					Int64: opts.ContextLimitFallback,
					Valid: true,
				}
			}
			// Persist the step. If persistence fails because
			// the chat was interrupted between the previous
			// check and here, fall back to the interrupt-safe
			// path so partial content is not lost.
			if err := opts.PersistStep(ctx, PersistedStep{
				Content:      result.content,
				Usage:        result.usage,
				ContextLimit: contextLimit,
			}); err != nil {
				if errors.Is(err, ErrInterrupted) {
					persistInterruptedStep(ctx, opts, &result)
					return ErrInterrupted
				}
				return xerrors.Errorf("persist step: %w", err)
			}
			lastUsage = result.usage
			lastProviderMetadata = result.providerMetadata

			// Append the step's response messages so that both
			// inline and post-loop compaction see the full
			// conversation including the latest assistant reply.
			stepMessages := result.toResponseMessages()
			messages = append(messages, stepMessages...)

			// Inline compaction.
			if opts.Compaction != nil && opts.ReloadMessages != nil {
				did, compactErr := tryCompact(
					ctx,
					opts.Model,
					opts.Compaction,
					opts.ContextLimitFallback,
					result.usage,
					result.providerMetadata,
					messages,
				)
				if compactErr != nil && opts.Compaction.OnError != nil {
					opts.Compaction.OnError(compactErr)
				}
				if did {
					alreadyCompacted = true
					compactedOnFinalStep = true
					reloaded, reloadErr := opts.ReloadMessages(ctx)
					if reloadErr != nil {
						return xerrors.Errorf("reload messages after compaction: %w", reloadErr)
					}
					messages = reloaded
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

		// Post-run compaction safety net: if we never compacted
		// during the loop, try once at the end.
		if !alreadyCompacted && opts.Compaction != nil && opts.ReloadMessages != nil {
			did, err := tryCompact(
				ctx,
				opts.Model,
				opts.Compaction,
				opts.ContextLimitFallback,
				lastUsage,
				lastProviderMetadata,
				messages,
			)
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

			publishMessagePart(
				codersdk.ChatMessageRoleAssistant,
				chatprompt.PartFromContent(tc),
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
				publishMessagePart(
					codersdk.ChatMessageRoleTool,
					chatprompt.PartFromContent(tr),
				)
			}
		case fantasy.StreamPartTypeFinish:
			result.usage = part.Usage
			result.finishReason = part.FinishReason
			result.providerMetadata = part.ProviderMetadata

		case fantasy.StreamPartTypeError:
			// Detect interruption: context canceled with
			// ErrInterrupted as the cause.
			if errors.Is(part.Error, context.Canceled) &&
				errors.Is(context.Cause(ctx), ErrInterrupted) {
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
	providerTools []ProviderTool,
	toolCalls []fantasy.ToolCallContent,
	onResult func(fantasy.ToolResultContent),
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
	// Include runners from provider tools so locally-executed
	// provider tools (e.g. computer use) can be dispatched.
	for _, pt := range providerTools {
		if pt.Runner != nil {
			toolMap[pt.Runner.Info().Name] = pt.Runner
		}
	}

	results := make([]fantasy.ToolResultContent, len(localToolCalls))
	var wg sync.WaitGroup
	wg.Add(len(localToolCalls))
	for i, tc := range localToolCalls {
		go func(i int, tc fantasy.ToolCallContent) {
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
			}()
			results[i] = executeSingleTool(ctx, toolMap, tc)
		}(i, tc)
	}
	wg.Wait()

	// Publish results in the original tool-call order so SSE
	// subscribers see a deterministic event sequence.
	if onResult != nil {
		for _, tr := range results {
			onResult(tr)
		}
	}
	return results
}

// executeSingleTool executes one tool call and converts the
// response into a ToolResultContent.
func executeSingleTool(
	ctx context.Context,
	toolMap map[string]fantasy.AgentTool,
	tc fantasy.ToolCallContent,
) fantasy.ToolResultContent {
	result := fantasy.ToolResultContent{
		ToolCallID:       tc.ToolCallID,
		ToolName:         tc.ToolName,
		ProviderExecuted: false,
	}

	tool, exists := toolMap[tc.ToolName]
	if !exists {
		result.Result = fantasy.ToolResultOutputContentError{
			Error: xerrors.New("Tool not found: " + tc.ToolName),
		}
		return result
	}

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
		return result
	}

	result.ClientMetadata = resp.Metadata
	switch {
	case resp.IsError:
		result.Result = fantasy.ToolResultOutputContentError{
			Error: xerrors.New(resp.Content),
		}
	case resp.Type == "image" || resp.Type == "media":
		result.Result = fantasy.ToolResultOutputContentMedia{
			Data:      string(resp.Data),
			MediaType: resp.MediaType,
			Text:      resp.Content,
		}
	default:
		result.Result = fantasy.ToolResultOutputContentText{
			Text: resp.Content,
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

// persistInterruptedStep saves all accumulated content from a
// partial stream. Since we own the stepResult directly, no shadow
// state is needed.
func persistInterruptedStep(
	ctx context.Context,
	opts RunOptions,
	result *stepResult,
) {
	if result == nil || (len(result.content) == 0 && len(result.toolCalls) == 0) {
		return
	}

	// Track which tool calls already have results in the content.
	answeredToolCalls := make(map[string]struct{})
	for _, c := range result.content {
		tr, ok := fantasy.AsContentType[fantasy.ToolResultContent](c)
		if ok && tr.ToolCallID != "" {
			answeredToolCalls[tr.ToolCallID] = struct{}{}
		}
	}

	// Build combined content: all accumulated content + synthetic
	// interrupted results for any unanswered tool calls.
	content := make([]fantasy.Content, 0, len(result.content))
	content = append(content, result.content...)

	for _, tc := range result.toolCalls {
		if tc.ToolCallID == "" {
			continue
		}
		if _, exists := answeredToolCalls[tc.ToolCallID]; exists {
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
		answeredToolCalls[tc.ToolCallID] = struct{}{}
	}

	persistCtx := context.WithoutCancel(ctx)
	if err := opts.PersistStep(persistCtx, PersistedStep{
		Content: content,
	}); err != nil {
		if opts.OnInterruptedPersistError != nil {
			opts.OnInterruptedPersistError(err)
		}
	}
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
		if len(activeTools) > 0 && !slices.Contains(activeTools, info.Name) {
			continue
		}

		inputSchema := map[string]any{
			"type":       "object",
			"properties": info.Parameters,
			"required":   info.Required,
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

	return strings.Contains(normalized, "context") &&
		(strings.Contains(normalized, "limit") ||
			strings.Contains(normalized, "window") ||
			strings.Contains(normalized, "length") ||
			strings.HasPrefix(normalized, "max"))
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
