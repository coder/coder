package chatprompt

import (
	"encoding/json"
	"regexp"
	"strings"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

var toolCallIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// ToolResultBlock is the persisted chat tool result shape.
type ToolResultBlock struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Result     any    `json:"result"`
	IsError    bool   `json:"is_error,omitempty"`
}

func ConvertMessages(
	messages []database.ChatMessage,
	subagentReportToolCallIDPrefix string,
) ([]fantasy.Message, error) {
	prompt := make([]fantasy.Message, 0, len(messages))
	toolNameByCallID := make(map[string]string)
	for _, message := range messages {
		// System messages are always included in the prompt even when
		// hidden, because the system prompt must reach the LLM. Other
		// hidden messages (e.g. internal bookkeeping) are skipped.
		if message.Hidden && message.Role != string(fantasy.MessageRoleSystem) {
			continue
		}

		switch message.Role {
		case string(fantasy.MessageRoleSystem):
			content, err := parseSystemContent(message.Content)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(content) == "" {
				continue
			}
			prompt = append(prompt, fantasy.Message{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: content},
				},
			})
		case string(fantasy.MessageRoleUser):
			content, err := ParseContent(string(fantasy.MessageRoleUser), message.Content)
			if err != nil {
				return nil, err
			}
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleUser,
				Content: ToMessageParts(content),
			})
		case string(fantasy.MessageRoleAssistant):
			content, err := ParseContent(string(fantasy.MessageRoleAssistant), message.Content)
			if err != nil {
				return nil, err
			}
			parts := ToMessageParts(content)
			for _, toolCall := range ExtractToolCalls(parts) {
				if toolCall.ToolCallID == "" || strings.TrimSpace(toolCall.ToolName) == "" {
					continue
				}
				toolNameByCallID[sanitizeToolCallID(toolCall.ToolCallID)] = toolCall.ToolName
			}
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleAssistant,
				Content: parts,
			})
		case string(fantasy.MessageRoleTool):
			results, err := ParseResults(message.Content)
			if err != nil {
				return nil, err
			}
			for _, result := range results {
				if result.ToolCallID == "" || strings.TrimSpace(result.ToolName) == "" {
					continue
				}
				toolNameByCallID[sanitizeToolCallID(result.ToolCallID)] = result.ToolName
			}
			prompt = append(prompt, toolMessageFromResults(results))
		default:
			return nil, xerrors.Errorf("unsupported chat message role %q", message.Role)
		}
	}
	prompt = injectMissingToolResults(prompt)
	prompt = injectMissingToolUses(
		prompt,
		toolNameByCallID,
		subagentReportToolCallIDPrefix,
	)
	return prompt, nil
}

// PrependSystem prepends a system message unless an existing system
// message already mentions create_workspace guidance.
func PrependSystem(prompt []fantasy.Message, instruction string) []fantasy.Message {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return prompt
	}
	for _, message := range prompt {
		if message.Role != fantasy.MessageRoleSystem {
			continue
		}
		for _, part := range message.Content {
			textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](part)
			if !ok {
				continue
			}
			if strings.Contains(strings.ToLower(textPart.Text), "create_workspace") {
				return prompt
			}
		}
	}

	out := make([]fantasy.Message, 0, len(prompt)+1)
	out = append(out, fantasy.Message{
		Role: fantasy.MessageRoleSystem,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: instruction},
		},
	})
	out = append(out, prompt...)
	return out
}

// InsertSystem inserts a system message after the existing system
// block and before the first non-system message.
func InsertSystem(prompt []fantasy.Message, instruction string) []fantasy.Message {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return prompt
	}

	systemMessage := fantasy.Message{
		Role: fantasy.MessageRoleSystem,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: instruction},
		},
	}

	out := make([]fantasy.Message, 0, len(prompt)+1)
	inserted := false
	for _, message := range prompt {
		if !inserted && message.Role != fantasy.MessageRoleSystem {
			out = append(out, systemMessage)
			inserted = true
		}
		out = append(out, message)
	}
	if !inserted {
		out = append(out, systemMessage)
	}
	return out
}

// AppendUser appends an instruction as a user message at the end of
// the prompt.
func AppendUser(prompt []fantasy.Message, instruction string) []fantasy.Message {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return prompt
	}
	out := make([]fantasy.Message, 0, len(prompt)+1)
	out = append(out, prompt...)
	out = append(out, fantasy.Message{
		Role: fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: instruction},
		},
	})
	return out
}

// ParseContent decodes persisted chat message content blocks.
func ParseContent(role string, raw pqtype.NullRawMessage) ([]fantasy.Content, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return nil, nil
	}

	var text string
	if err := json.Unmarshal(raw.RawMessage, &text); err == nil {
		return []fantasy.Content{fantasy.TextContent{Text: text}}, nil
	}

	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(raw.RawMessage, &rawBlocks); err != nil {
		return nil, xerrors.Errorf("parse %s content: %w", role, err)
	}

	content := make([]fantasy.Content, 0, len(rawBlocks))
	for i, rawBlock := range rawBlocks {
		block, err := fantasy.UnmarshalContent(rawBlock)
		if err != nil {
			return nil, xerrors.Errorf("parse %s content block %d: %w", role, i, err)
		}
		content = append(content, block)
	}
	return content, nil
}

// ParseResults decodes persisted tool result blocks.
func ParseResults(raw pqtype.NullRawMessage) ([]ToolResultBlock, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return nil, nil
	}

	var results []ToolResultBlock
	if err := json.Unmarshal(raw.RawMessage, &results); err != nil {
		return nil, xerrors.Errorf("parse tool content: %w", err)
	}
	return results, nil
}

// ToMessageParts converts fantasy content blocks into message parts.
func ToMessageParts(content []fantasy.Content) []fantasy.MessagePart {
	parts := make([]fantasy.MessagePart, 0, len(content))
	for _, block := range content {
		switch value := block.(type) {
		case fantasy.TextContent:
			parts = append(parts, fantasy.TextPart{
				Text:            value.Text,
				ProviderOptions: fantasy.ProviderOptions(value.ProviderMetadata),
			})
		case *fantasy.TextContent:
			parts = append(parts, fantasy.TextPart{
				Text:            value.Text,
				ProviderOptions: fantasy.ProviderOptions(value.ProviderMetadata),
			})
		case fantasy.ReasoningContent:
			parts = append(parts, fantasy.ReasoningPart{
				Text:            value.Text,
				ProviderOptions: fantasy.ProviderOptions(value.ProviderMetadata),
			})
		case *fantasy.ReasoningContent:
			parts = append(parts, fantasy.ReasoningPart{
				Text:            value.Text,
				ProviderOptions: fantasy.ProviderOptions(value.ProviderMetadata),
			})
		case fantasy.ToolCallContent:
			parts = append(parts, fantasy.ToolCallPart{
				ToolCallID:       sanitizeToolCallID(value.ToolCallID),
				ToolName:         value.ToolName,
				Input:            value.Input,
				ProviderExecuted: value.ProviderExecuted,
				ProviderOptions:  fantasy.ProviderOptions(value.ProviderMetadata),
			})
		case *fantasy.ToolCallContent:
			parts = append(parts, fantasy.ToolCallPart{
				ToolCallID:       sanitizeToolCallID(value.ToolCallID),
				ToolName:         value.ToolName,
				Input:            value.Input,
				ProviderExecuted: value.ProviderExecuted,
				ProviderOptions:  fantasy.ProviderOptions(value.ProviderMetadata),
			})
		case fantasy.FileContent:
			parts = append(parts, fantasy.FilePart{
				Data:            value.Data,
				MediaType:       value.MediaType,
				ProviderOptions: fantasy.ProviderOptions(value.ProviderMetadata),
			})
		case *fantasy.FileContent:
			parts = append(parts, fantasy.FilePart{
				Data:            value.Data,
				MediaType:       value.MediaType,
				ProviderOptions: fantasy.ProviderOptions(value.ProviderMetadata),
			})
		case fantasy.ToolResultContent:
			parts = append(parts, fantasy.ToolResultPart{
				ToolCallID:      sanitizeToolCallID(value.ToolCallID),
				Output:          value.Result,
				ProviderOptions: fantasy.ProviderOptions(value.ProviderMetadata),
			})
		case *fantasy.ToolResultContent:
			parts = append(parts, fantasy.ToolResultPart{
				ToolCallID:      sanitizeToolCallID(value.ToolCallID),
				Output:          value.Result,
				ProviderOptions: fantasy.ProviderOptions(value.ProviderMetadata),
			})
		}
	}
	return parts
}

// ExtractToolCalls returns all tool call parts as content blocks.
func ExtractToolCalls(parts []fantasy.MessagePart) []fantasy.ToolCallContent {
	toolCalls := make([]fantasy.ToolCallContent, 0, len(parts))
	for _, part := range parts {
		toolCall, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
		if !ok {
			continue
		}
		toolCalls = append(toolCalls, fantasy.ToolCallContent{
			ToolCallID:       toolCall.ToolCallID,
			ToolName:         toolCall.ToolName,
			Input:            toolCall.Input,
			ProviderExecuted: toolCall.ProviderExecuted,
		})
	}
	return toolCalls
}

// MarshalContent encodes message content blocks for persistence.
func MarshalContent(blocks []fantasy.Content) (pqtype.NullRawMessage, error) {
	if len(blocks) == 0 {
		return pqtype.NullRawMessage{}, nil
	}

	encodedBlocks := make([]json.RawMessage, 0, len(blocks))
	for i, block := range blocks {
		encoded, err := marshalContentBlock(block)
		if err != nil {
			return pqtype.NullRawMessage{}, xerrors.Errorf(
				"encode content block %d: %w",
				i,
				err,
			)
		}
		encodedBlocks = append(encodedBlocks, encoded)
	}

	data, err := json.Marshal(encodedBlocks)
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode content blocks: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

// MarshalResults encodes tool result blocks for persistence.
func MarshalResults(results []ToolResultBlock) (pqtype.NullRawMessage, error) {
	if len(results) == 0 {
		return pqtype.NullRawMessage{}, nil
	}
	data, err := json.Marshal(results)
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode tool results: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

// PartFromContent converts fantasy content into a SDK chat message part.
func PartFromContent(block fantasy.Content) codersdk.ChatMessagePart {
	switch value := block.(type) {
	case fantasy.TextContent:
		return codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeText,
			Text: value.Text,
		}
	case *fantasy.TextContent:
		return codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeText,
			Text: value.Text,
		}
	case fantasy.ReasoningContent:
		return codersdk.ChatMessagePart{
			Type:  codersdk.ChatMessagePartTypeReasoning,
			Text:  value.Text,
			Title: reasoningSummaryTitle(value.ProviderMetadata),
		}
	case *fantasy.ReasoningContent:
		return codersdk.ChatMessagePart{
			Type:  codersdk.ChatMessagePartTypeReasoning,
			Text:  value.Text,
			Title: reasoningSummaryTitle(value.ProviderMetadata),
		}
	case fantasy.ToolCallContent:
		return codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: value.ToolCallID,
			ToolName:   value.ToolName,
			Args:       []byte(value.Input),
		}
	case *fantasy.ToolCallContent:
		return codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: value.ToolCallID,
			ToolName:   value.ToolName,
			Args:       []byte(value.Input),
		}
	case fantasy.SourceContent:
		return codersdk.ChatMessagePart{
			Type:     codersdk.ChatMessagePartTypeSource,
			SourceID: value.ID,
			URL:      value.URL,
			Title:    value.Title,
		}
	case *fantasy.SourceContent:
		return codersdk.ChatMessagePart{
			Type:     codersdk.ChatMessagePartTypeSource,
			SourceID: value.ID,
			URL:      value.URL,
			Title:    value.Title,
		}
	case fantasy.FileContent:
		return codersdk.ChatMessagePart{
			Type:      codersdk.ChatMessagePartTypeFile,
			MediaType: value.MediaType,
			Data:      value.Data,
		}
	case *fantasy.FileContent:
		return codersdk.ChatMessagePart{
			Type:      codersdk.ChatMessagePartTypeFile,
			MediaType: value.MediaType,
			Data:      value.Data,
		}
	case fantasy.ToolResultContent:
		return PartFromResult(ResultFromContent(value))
	case *fantasy.ToolResultContent:
		return PartFromResult(ResultFromContent(*value))
	default:
		return codersdk.ChatMessagePart{}
	}
}

// ReasoningTitleFromFirstLine extracts a compact markdown title.
func ReasoningTitleFromFirstLine(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	firstLine := text
	if idx := strings.IndexAny(firstLine, "\r\n"); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)
	if firstLine == "" || !strings.HasPrefix(firstLine, "**") {
		return ""
	}

	rest := firstLine[2:]
	end := strings.Index(rest, "**")
	if end < 0 {
		return ""
	}

	title := strings.TrimSpace(rest[:end])
	if title == "" {
		return ""
	}

	// Require the first line to be exactly "**title**" (ignoring
	// surrounding whitespace) so providers without this format don't
	// accidentally emit a title.
	if strings.TrimSpace(rest[end+2:]) != "" {
		return ""
	}

	return compactReasoningSummaryTitle(title)
}

// ResultFromContent normalizes fantasy tool result content.
func ResultFromContent(content fantasy.ToolResultContent) ToolResultBlock {
	result := ToolResultBlock{
		ToolCallID: content.ToolCallID,
		ToolName:   content.ToolName,
	}
	switch output := content.Result.(type) {
	case fantasy.ToolResultOutputContentError:
		result.IsError = true
		if output.Error != nil {
			result.Result = map[string]any{"error": output.Error.Error()}
		} else {
			result.Result = map[string]any{"error": ""}
		}
	case fantasy.ToolResultOutputContentText:
		decoded := map[string]any{}
		if err := json.Unmarshal([]byte(output.Text), &decoded); err == nil {
			result.Result = decoded
		} else {
			result.Result = map[string]any{"output": output.Text}
		}
	case fantasy.ToolResultOutputContentMedia:
		result.Result = map[string]any{
			"data":      output.Data,
			"mime_type": output.MediaType,
			"text":      output.Text,
		}
	default:
		result.Result = map[string]any{}
	}
	return result
}

// PartFromResult converts a persisted tool result into SDK part payload.
func PartFromResult(result ToolResultBlock) codersdk.ChatMessagePart {
	return codersdk.ChatMessagePart{
		Type:       codersdk.ChatMessagePartTypeToolResult,
		ToolCallID: result.ToolCallID,
		ToolName:   result.ToolName,
		Result:     toRawJSON(result.Result),
		IsError:    result.IsError,
		ResultMeta: toolResultMetadata(result.Result),
	}
}

func injectMissingToolResults(prompt []fantasy.Message) []fantasy.Message {
	result := make([]fantasy.Message, 0, len(prompt))
	for i := 0; i < len(prompt); i++ {
		msg := prompt[i]
		result = append(result, msg)

		if msg.Role != fantasy.MessageRoleAssistant {
			continue
		}
		toolCalls := ExtractToolCalls(msg.Content)
		if len(toolCalls) == 0 {
			continue
		}

		// Collect the tool call IDs that have results in the
		// following tool message(s).
		answered := make(map[string]struct{})
		j := i + 1
		for ; j < len(prompt); j++ {
			if prompt[j].Role != fantasy.MessageRoleTool {
				break
			}
			for _, part := range prompt[j].Content {
				tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
				if !ok {
					continue
				}
				answered[tr.ToolCallID] = struct{}{}
			}
		}
		if i+1 < j {
			// Preserve persisted tool result ordering and inject any
			// synthetic results after the existing contiguous tool messages.
			result = append(result, prompt[i+1:j]...)
			i = j - 1
		}

		// Build synthetic results for any unanswered tool calls.
		var missing []ToolResultBlock
		for _, tc := range toolCalls {
			if _, ok := answered[tc.ToolCallID]; !ok {
				missing = append(missing, ToolResultBlock{
					ToolCallID: tc.ToolCallID,
					ToolName:   tc.ToolName,
					Result: map[string]any{
						"error": "tool call was interrupted and did not receive a result",
					},
					IsError: true,
				})
			}
		}
		if len(missing) > 0 {
			result = append(result, toolMessageFromResults(missing))
		}
	}
	return result
}

func injectMissingToolUses(
	prompt []fantasy.Message,
	toolNameByCallID map[string]string,
	subagentReportToolCallIDPrefix string,
) []fantasy.Message {
	result := make([]fantasy.Message, 0, len(prompt))
	for _, msg := range prompt {
		if msg.Role != fantasy.MessageRoleTool {
			result = append(result, msg)
			continue
		}

		toolResults := make([]fantasy.ToolResultPart, 0, len(msg.Content))
		for _, part := range msg.Content {
			toolResult, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
			if !ok {
				continue
			}
			toolResults = append(toolResults, toolResult)
		}
		if len(toolResults) == 0 {
			result = append(result, msg)
			continue
		}

		// Walk backwards through the result to find the nearest
		// preceding assistant message (skipping over other tool
		// messages that belong to the same batch of results).
		answeredByPrevious := make(map[string]struct{})
		for k := len(result) - 1; k >= 0; k-- {
			if result[k].Role == fantasy.MessageRoleAssistant {
				for _, toolCall := range ExtractToolCalls(result[k].Content) {
					toolCallID := sanitizeToolCallID(toolCall.ToolCallID)
					if toolCallID == "" {
						continue
					}
					answeredByPrevious[toolCallID] = struct{}{}
				}
				break
			}
			if result[k].Role != fantasy.MessageRoleTool {
				break
			}
		}

		matchingResults := make([]fantasy.ToolResultPart, 0, len(toolResults))
		orphanResults := make([]fantasy.ToolResultPart, 0, len(toolResults))
		for _, toolResult := range toolResults {
			toolCallID := sanitizeToolCallID(toolResult.ToolCallID)
			if _, ok := answeredByPrevious[toolCallID]; ok {
				matchingResults = append(matchingResults, toolResult)
				continue
			}
			orphanResults = append(orphanResults, toolResult)
		}

		if len(orphanResults) == 0 {
			result = append(result, msg)
			continue
		}

		syntheticToolUse := syntheticToolUseMessage(
			orphanResults,
			toolNameByCallID,
			subagentReportToolCallIDPrefix,
		)
		if len(syntheticToolUse.Content) == 0 {
			result = append(result, msg)
			continue
		}

		if len(matchingResults) > 0 {
			result = append(result, toolMessageFromToolResultParts(matchingResults))
		}
		result = append(result, syntheticToolUse)
		result = append(result, toolMessageFromToolResultParts(orphanResults))
	}

	return result
}

func toolMessageFromToolResultParts(results []fantasy.ToolResultPart) fantasy.Message {
	parts := make([]fantasy.MessagePart, 0, len(results))
	for _, result := range results {
		parts = append(parts, result)
	}
	return fantasy.Message{
		Role:    fantasy.MessageRoleTool,
		Content: parts,
	}
}

func syntheticToolUseMessage(
	toolResults []fantasy.ToolResultPart,
	toolNameByCallID map[string]string,
	subagentReportToolCallIDPrefix string,
) fantasy.Message {
	parts := make([]fantasy.MessagePart, 0, len(toolResults))
	seen := make(map[string]struct{}, len(toolResults))

	for _, toolResult := range toolResults {
		toolCallID := sanitizeToolCallID(toolResult.ToolCallID)
		if toolCallID == "" {
			continue
		}
		if _, ok := seen[toolCallID]; ok {
			continue
		}

		toolName := strings.TrimSpace(toolNameByCallID[toolCallID])
		if toolName == "" && strings.HasPrefix(toolCallID, subagentReportToolCallIDPrefix) {
			toolName = "subagent_report"
		}
		if toolName == "" {
			continue
		}

		seen[toolCallID] = struct{}{}
		parts = append(parts, fantasy.ToolCallPart{
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Input:      "{}",
		})
	}

	return fantasy.Message{
		Role:    fantasy.MessageRoleAssistant,
		Content: parts,
	}
}

func parseSystemContent(raw pqtype.NullRawMessage) (string, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return "", nil
	}

	var content string
	if err := json.Unmarshal(raw.RawMessage, &content); err != nil {
		return "", xerrors.Errorf("parse system message content: %w", err)
	}
	return content, nil
}

func toolMessageFromResults(results []ToolResultBlock) fantasy.Message {
	parts := make([]fantasy.MessagePart, 0, len(results))
	for _, result := range results {
		parts = append(parts, toolResultToMessagePart(result))
	}
	return fantasy.Message{
		Role:    fantasy.MessageRoleTool,
		Content: parts,
	}
}

func toolResultToMessagePart(result ToolResultBlock) fantasy.ToolResultPart {
	toolCallID := sanitizeToolCallID(result.ToolCallID)

	payload := result.Result
	if payload == nil {
		payload = map[string]any{}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(`{}`)
	}

	if result.IsError {
		message := strings.TrimSpace(string(raw))
		if fields, ok := payload.(map[string]any); ok {
			if extracted, ok := fields["error"].(string); ok && strings.TrimSpace(extracted) != "" {
				message = extracted
			}
		}
		return fantasy.ToolResultPart{
			ToolCallID: toolCallID,
			Output: fantasy.ToolResultOutputContentError{
				Error: xerrors.New(message),
			},
		}
	}

	return fantasy.ToolResultPart{
		ToolCallID: toolCallID,
		Output: fantasy.ToolResultOutputContentText{
			Text: string(raw),
		},
	}
}

func sanitizeToolCallID(id string) string {
	if id == "" {
		return ""
	}
	return toolCallIDSanitizer.ReplaceAllString(id, "_")
}

func marshalContentBlock(block fantasy.Content) (json.RawMessage, error) {
	encoded, err := json.Marshal(block)
	if err != nil {
		return nil, err
	}

	title, ok := reasoningTitleFromContent(block)
	if !ok || title == "" {
		return encoded, nil
	}

	var envelope struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return nil, err
	}

	if !strings.EqualFold(envelope.Type, string(fantasy.ContentTypeReasoning)) {
		return encoded, nil
	}
	if envelope.Data == nil {
		envelope.Data = map[string]any{}
	}
	envelope.Data["title"] = title

	encodedWithTitle, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	return encodedWithTitle, nil
}

func reasoningTitleFromContent(block fantasy.Content) (string, bool) {
	switch value := block.(type) {
	case fantasy.ReasoningContent:
		return ReasoningTitleFromFirstLine(value.Text), true
	case *fantasy.ReasoningContent:
		if value == nil {
			return "", false
		}
		return ReasoningTitleFromFirstLine(value.Text), true
	default:
		return "", false
	}
}

func reasoningSummaryTitle(metadata fantasy.ProviderMetadata) string {
	if len(metadata) == 0 {
		return ""
	}

	reasoningMetadata := fantasyopenai.GetReasoningMetadata(
		fantasy.ProviderOptions(metadata),
	)
	if reasoningMetadata == nil {
		return ""
	}

	for _, summary := range reasoningMetadata.Summary {
		if title := compactReasoningSummaryTitle(summary); title != "" {
			return title
		}
	}

	return ""
}

func compactReasoningSummaryTitle(summary string) string {
	const maxWords = 8
	const maxRunes = 80

	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}

	summary = strings.Trim(summary, "\"'`")
	summary = reasoningSummaryHeadline(summary)
	words := strings.Fields(summary)
	if len(words) == 0 {
		return ""
	}

	truncated := false
	if len(words) > maxWords {
		words = words[:maxWords]
		truncated = true
	}

	title := strings.Join(words, " ")
	if truncated {
		title += "…"
	}
	return truncateRunes(title, maxRunes)
}

func reasoningSummaryHeadline(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}

	// OpenAI summary_text may be markdown like:
	// "**Title**\n\nLonger explanation ...".
	// Keep only the heading segment for UI titles.
	if idx := strings.Index(summary, "\n\n"); idx >= 0 {
		summary = summary[:idx]
	}

	if idx := strings.IndexAny(summary, "\r\n"); idx >= 0 {
		summary = summary[:idx]
	}

	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}

	if strings.HasPrefix(summary, "**") {
		rest := summary[2:]
		if end := strings.Index(rest, "**"); end >= 0 {
			bold := strings.TrimSpace(rest[:end])
			if bold != "" {
				summary = bold
			}
		}
	}

	return strings.TrimSpace(strings.Trim(summary, "\"'`"))
}

func toRawJSON(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func toolResultMetadata(value any) *codersdk.ChatToolResultMetadata {
	fields, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	meta := codersdk.ChatToolResultMetadata{}
	if s, ok := stringValue(fields["error"]); ok {
		meta.Error = s
	}
	if s, ok := stringValue(fields["output"]); ok {
		meta.Output = s
	}
	if n, ok := intValue(fields["exit_code"]); ok {
		meta.ExitCode = &n
	}
	if s, ok := stringValue(fields["content"]); ok {
		meta.Content = s
	}
	if s, ok := stringValue(fields["mime_type"]); ok {
		meta.MimeType = s
	}
	if b, ok := boolValue(fields["created"]); ok {
		meta.Created = &b
	}
	if s, ok := stringValue(fields["workspace_id"]); ok {
		meta.WorkspaceID = s
	}
	if s, ok := stringValue(fields["workspace_agent_id"]); ok {
		meta.WorkspaceAgentID = s
	}
	if s, ok := stringValue(fields["workspace_name"]); ok {
		meta.WorkspaceName = s
	}
	if s, ok := stringValue(fields["workspace_url"]); ok {
		meta.WorkspaceURL = s
	}
	if s, ok := stringValue(fields["reason"]); ok {
		meta.Reason = s
	}

	if meta.Error == "" &&
		meta.Output == "" &&
		meta.ExitCode == nil &&
		meta.Content == "" &&
		meta.MimeType == "" &&
		meta.Created == nil &&
		meta.WorkspaceID == "" &&
		meta.WorkspaceAgentID == "" &&
		meta.WorkspaceName == "" &&
		meta.WorkspaceURL == "" &&
		meta.Reason == "" {
		return nil
	}

	return &meta
}

func stringValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	default:
		return "", false
	}
}

func boolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	default:
		return false, false
	}
}

func intValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		n, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= max {
		return value
	}

	return string(runes[:max])
}
