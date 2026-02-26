package chatloop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

const (
	interruptedToolResultErrorMessage = "tool call was interrupted before it produced a result"
)

var ErrInterrupted = xerrors.New("chat interrupted")

// PersistedStep contains the full content of a completed or
// interrupted agent step. Content includes both assistant blocks
// (text, reasoning, tool calls) and tool result blocks, mirroring
// what fantasy provides in StepResult.Content. The persistence
// layer is responsible for splitting these into separate database
// messages by role.
type PersistedStep struct {
	Content      []fantasy.Content
	Usage        fantasy.Usage
	ContextLimit sql.NullInt64
}

// RunOptions configures a single streaming chat loop run.
type RunOptions struct {
	Model      fantasy.LanguageModel
	Messages   []fantasy.Message
	Tools      []fantasy.AgentTool
	StreamCall fantasy.AgentStreamCall
	MaxSteps   int

	ActiveTools          []string
	ContextLimitFallback int64

	PersistStep        func(context.Context, PersistedStep) error
	PublishMessagePart func(
		role fantasy.MessageRole,
		part codersdk.ChatMessagePart,
	)
	Compaction *CompactionOptions

	OnInterruptedPersistError func(error)
}

// Run executes the chat step-stream loop and delegates persistence/publishing to callbacks.
func Run(ctx context.Context, opts RunOptions) (*fantasy.AgentResult, error) {
	if opts.Model == nil {
		return nil, xerrors.New("chat model is required")
	}
	if opts.PersistStep == nil {
		return nil, xerrors.New("persist step callback is required")
	}
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 1
	}

	publishMessagePart := func(role fantasy.MessageRole, part codersdk.ChatMessagePart) {
		if opts.PublishMessagePart == nil {
			return
		}
		opts.PublishMessagePart(role, part)
	}

	var (
		stepStateMu           sync.Mutex
		streamToolNames       map[string]string
		streamReasoningTitles map[string]string
		streamReasoningText   map[string]string
		// stepToolResultContents tracks tool results received during
		// streaming. These are needed for the interrupted-step path
		// where OnStepFinish never fires.
		stepToolResultContents []fantasy.ToolResultContent
		stepAssistantDraft     []fantasy.Content
		stepToolCallIndexByID  map[string]int
	)

	resetStepState := func() {
		stepStateMu.Lock()
		streamToolNames = make(map[string]string)
		streamReasoningTitles = make(map[string]string)
		streamReasoningText = make(map[string]string)
		stepToolResultContents = nil
		stepAssistantDraft = nil
		stepToolCallIndexByID = make(map[string]int)
		stepStateMu.Unlock()
	}

	setReasoningTitleFromText := func(id string, text string) {
		if id == "" || strings.TrimSpace(text) == "" {
			return
		}

		stepStateMu.Lock()
		defer stepStateMu.Unlock()

		if streamReasoningTitles[id] != "" {
			return
		}

		streamReasoningText[id] += text
		if !strings.ContainsAny(streamReasoningText[id], "\r\n") {
			return
		}
		title := chatprompt.ReasoningTitleFromFirstLine(streamReasoningText[id])
		if title == "" {
			return
		}

		streamReasoningTitles[id] = title
	}

	appendDraftText := func(text string) {
		if text == "" {
			return
		}

		stepStateMu.Lock()
		defer stepStateMu.Unlock()

		if len(stepAssistantDraft) > 0 {
			lastIndex := len(stepAssistantDraft) - 1
			switch last := stepAssistantDraft[lastIndex].(type) {
			case fantasy.TextContent:
				last.Text += text
				stepAssistantDraft[lastIndex] = last
				return
			case *fantasy.TextContent:
				last.Text += text
				stepAssistantDraft[lastIndex] = fantasy.TextContent{Text: last.Text}
				return
			}
		}
		stepAssistantDraft = append(stepAssistantDraft, fantasy.TextContent{Text: text})
	}

	appendDraftReasoning := func(text string) {
		if text == "" {
			return
		}

		stepStateMu.Lock()
		defer stepStateMu.Unlock()

		if len(stepAssistantDraft) > 0 {
			lastIndex := len(stepAssistantDraft) - 1
			switch last := stepAssistantDraft[lastIndex].(type) {
			case fantasy.ReasoningContent:
				last.Text += text
				stepAssistantDraft[lastIndex] = last
				return
			case *fantasy.ReasoningContent:
				last.Text += text
				stepAssistantDraft[lastIndex] = fantasy.ReasoningContent{Text: last.Text}
				return
			}
		}
		stepAssistantDraft = append(stepAssistantDraft, fantasy.ReasoningContent{Text: text})
	}

	upsertDraftToolCall := func(toolCallID, toolName, input string, appendInput bool) {
		if toolCallID == "" {
			return
		}

		stepStateMu.Lock()
		defer stepStateMu.Unlock()

		if strings.TrimSpace(toolName) != "" {
			streamToolNames[toolCallID] = toolName
		}

		index, exists := stepToolCallIndexByID[toolCallID]
		if !exists {
			stepToolCallIndexByID[toolCallID] = len(stepAssistantDraft)
			stepAssistantDraft = append(stepAssistantDraft, fantasy.ToolCallContent{
				ToolCallID: toolCallID,
				ToolName:   toolName,
				Input:      input,
			})
			return
		}

		if index < 0 || index >= len(stepAssistantDraft) {
			stepToolCallIndexByID[toolCallID] = len(stepAssistantDraft)
			stepAssistantDraft = append(stepAssistantDraft, fantasy.ToolCallContent{
				ToolCallID: toolCallID,
				ToolName:   toolName,
				Input:      input,
			})
			return
		}

		existingCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](stepAssistantDraft[index])
		if !ok {
			if ptrCall, ptrOK := fantasy.AsContentType[*fantasy.ToolCallContent](stepAssistantDraft[index]); ptrOK && ptrCall != nil {
				existingCall = *ptrCall
				ok = true
			}
		}
		if !ok {
			stepToolCallIndexByID[toolCallID] = len(stepAssistantDraft)
			stepAssistantDraft = append(stepAssistantDraft, fantasy.ToolCallContent{
				ToolCallID: toolCallID,
				ToolName:   toolName,
				Input:      input,
			})
			return
		}

		if strings.TrimSpace(toolName) != "" {
			existingCall.ToolName = toolName
		}
		if appendInput {
			existingCall.Input += input
		} else if input != "" || existingCall.Input == "" {
			existingCall.Input = input
		}
		stepAssistantDraft[index] = existingCall
	}

	appendDraftSource := func(source fantasy.SourceContent) {
		stepStateMu.Lock()
		stepAssistantDraft = append(stepAssistantDraft, source)
		stepStateMu.Unlock()
	}

	persistInterruptedStep := func() error {
		stepStateMu.Lock()
		draft := append([]fantasy.Content(nil), stepAssistantDraft...)
		toolResults := append([]fantasy.ToolResultContent(nil), stepToolResultContents...)
		toolNameByCallID := make(map[string]string, len(streamToolNames))
		for id, name := range streamToolNames {
			toolNameByCallID[id] = name
		}
		stepStateMu.Unlock()

		if len(draft) == 0 && len(toolResults) == 0 {
			return nil
		}

		// Track which tool calls already have results.
		answeredToolCalls := make(map[string]struct{}, len(toolResults))
		for _, tr := range toolResults {
			if tr.ToolCallID != "" {
				answeredToolCalls[tr.ToolCallID] = struct{}{}
			}
		}

		// Build the combined content: draft + received tool results
		// + synthetic interrupted results for unanswered tool calls.
		content := make([]fantasy.Content, 0, len(draft)+len(toolResults))
		content = append(content, draft...)
		for _, tr := range toolResults {
			content = append(content, tr)
		}

		for _, block := range draft {
			toolCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](block)
			if !ok {
				if ptrCall, ptrOK := fantasy.AsContentType[*fantasy.ToolCallContent](block); ptrOK && ptrCall != nil {
					toolCall = *ptrCall
					ok = true
				}
			}
			if !ok || toolCall.ToolCallID == "" {
				continue
			}
			if _, exists := answeredToolCalls[toolCall.ToolCallID]; exists {
				continue
			}

			toolName := strings.TrimSpace(toolCall.ToolName)
			if toolName == "" {
				toolName = strings.TrimSpace(toolNameByCallID[toolCall.ToolCallID])
			}

			content = append(content, fantasy.ToolResultContent{
				ToolCallID: toolCall.ToolCallID,
				ToolName:   toolName,
				Result: fantasy.ToolResultOutputContentError{
					Error: xerrors.New(interruptedToolResultErrorMessage),
				},
			})
			answeredToolCalls[toolCall.ToolCallID] = struct{}{}
		}

		persistCtx := context.WithoutCancel(ctx)
		return opts.PersistStep(persistCtx, PersistedStep{
			Content: content,
		})
	}

	resetStepState()

	agent := fantasy.NewAgent(
		opts.Model,
		fantasy.WithTools(opts.Tools...),
		fantasy.WithStopConditions(fantasy.StepCountIs(opts.MaxSteps)),
	)
	applyAnthropicCaching := shouldApplyAnthropicPromptCaching(opts.Model)
	// Fantasy's AgentStreamCall currently requires a non-empty Prompt and always
	// appends it as a user message. chatd already supplies the full history in
	// Messages, so we pass and then strip a sentinel user message in PrepareStep.
	sentinelPrompt := "__chatd_agent_prompt_sentinel_" + uuid.NewString()

	streamCall := opts.StreamCall
	streamCall.Prompt = sentinelPrompt
	streamCall.Messages = opts.Messages
	streamCall.PrepareStep = func(
		stepCtx context.Context,
		options fantasy.PrepareStepFunctionOptions,
	) (context.Context, fantasy.PrepareStepResult, error) {
		return stepCtx, prepareStepResult(
			options.Messages,
			sentinelPrompt,
			opts.ActiveTools,
			applyAnthropicCaching,
		), nil
	}
	streamCall.OnStepStart = func(_ int) error {
		resetStepState()
		return nil
	}
	streamCall.OnTextDelta = func(_ string, text string) error {
		appendDraftText(text)
		publishMessagePart(fantasy.MessageRoleAssistant, codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeText,
			Text: text,
		})
		return nil
	}
	streamCall.OnReasoningDelta = func(id string, text string) error {
		appendDraftReasoning(text)
		setReasoningTitleFromText(id, text)
		stepStateMu.Lock()
		title := streamReasoningTitles[id]
		stepStateMu.Unlock()
		publishMessagePart(fantasy.MessageRoleAssistant, codersdk.ChatMessagePart{
			Type:  codersdk.ChatMessagePartTypeReasoning,
			Text:  text,
			Title: title,
		})
		return nil
	}
	streamCall.OnReasoningEnd = func(id string, _ fantasy.ReasoningContent) error {
		stepStateMu.Lock()
		if streamReasoningTitles[id] == "" {
			// At the end of reasoning we have the full text, so we can
			// safely evaluate first-line title format even if no newline
			// ever arrived in deltas.
			streamReasoningTitles[id] = chatprompt.ReasoningTitleFromFirstLine(
				streamReasoningText[id],
			)
		}
		title := streamReasoningTitles[id]
		stepStateMu.Unlock()
		if title != "" {
			// Publish a title-only reasoning part so clients can update the
			// reasoning header when metadata arrives at the end of streaming.
			publishMessagePart(fantasy.MessageRoleAssistant, codersdk.ChatMessagePart{
				Type:  codersdk.ChatMessagePartTypeReasoning,
				Title: title,
			})
		}
		return nil
	}
	streamCall.OnToolInputStart = func(id, toolName string) error {
		upsertDraftToolCall(id, toolName, "", false)
		return nil
	}
	streamCall.OnToolInputDelta = func(id, delta string) error {
		stepStateMu.Lock()
		toolName := streamToolNames[id]
		stepStateMu.Unlock()
		upsertDraftToolCall(id, toolName, delta, true)
		publishMessagePart(fantasy.MessageRoleAssistant, codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: id,
			ToolName:   toolName,
			ArgsDelta:  delta,
		})
		return nil
	}
	streamCall.OnToolCall = func(toolCall fantasy.ToolCallContent) error {
		upsertDraftToolCall(toolCall.ToolCallID, toolCall.ToolName, toolCall.Input, false)
		publishMessagePart(
			fantasy.MessageRoleAssistant,
			chatprompt.PartFromContent(toolCall),
		)
		return nil
	}
	streamCall.OnSource = func(source fantasy.SourceContent) error {
		appendDraftSource(source)
		publishMessagePart(
			fantasy.MessageRoleAssistant,
			chatprompt.PartFromContent(source),
		)
		return nil
	}
	streamCall.OnToolResult = func(result fantasy.ToolResultContent) error {
		publishMessagePart(
			fantasy.MessageRoleTool,
			chatprompt.PartFromContent(result),
		)

		stepStateMu.Lock()
		if result.ToolCallID != "" && strings.TrimSpace(result.ToolName) != "" {
			streamToolNames[result.ToolCallID] = result.ToolName
		}
		stepToolResultContents = append(stepToolResultContents, result)
		stepStateMu.Unlock()

		return nil
	}
	streamCall.OnStepFinish = func(stepResult fantasy.StepResult) error {
		contextLimit := extractContextLimit(stepResult.ProviderMetadata)
		if !contextLimit.Valid && opts.ContextLimitFallback > 0 {
			contextLimit = sql.NullInt64{
				Int64: opts.ContextLimitFallback,
				Valid: true,
			}
		}

		return opts.PersistStep(ctx, PersistedStep{
			Content:      stepResult.Content,
			Usage:        stepResult.Usage,
			ContextLimit: contextLimit,
		})
	}

	result, err := agent.Stream(ctx, streamCall)
	if err != nil {
		if errors.Is(err, context.Canceled) &&
			errors.Is(context.Cause(ctx), ErrInterrupted) {
			if persistErr := persistInterruptedStep(); persistErr != nil {
				if opts.OnInterruptedPersistError != nil {
					opts.OnInterruptedPersistError(persistErr)
				}
			}
			return nil, ErrInterrupted
		}
		return nil, xerrors.Errorf("stream response: %w", err)
	}
	if opts.Compaction != nil {
		if err := maybeCompact(ctx, opts, result); err != nil {
			if opts.Compaction.OnError != nil {
				opts.Compaction.OnError(err)
			}
		}
	}

	return result, nil
}

//nolint:revive // Boolean controls Anthropic-specific caching behavior.
func prepareStepResult(
	messages []fantasy.Message,
	sentinel string,
	activeTools []string,
	anthropicCaching bool,
) fantasy.PrepareStepResult {
	filtered := make([]fantasy.Message, 0, len(messages))
	removed := false
	for _, message := range messages {
		if !removed &&
			message.Role == fantasy.MessageRoleUser &&
			len(message.Content) == 1 {
			textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](message.Content[0])
			if ok && textPart.Text == sentinel {
				removed = true
				continue
			}
		}
		filtered = append(filtered, message)
	}

	result := fantasy.PrepareStepResult{
		Messages: filtered,
	}
	if anthropicCaching {
		result.Messages = addAnthropicPromptCaching(result.Messages)
	}
	if len(activeTools) > 0 {
		result.ActiveTools = append([]string(nil), activeTools...)
	}
	return result
}

func shouldApplyAnthropicPromptCaching(model fantasy.LanguageModel) bool {
	if model == nil {
		return false
	}
	return model.Provider() == fantasyanthropic.Name
}

func addAnthropicPromptCaching(messages []fantasy.Message) []fantasy.Message {
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

	return messages
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
