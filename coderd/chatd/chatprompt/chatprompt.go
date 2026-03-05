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

func ConvertMessages(
	messages []database.ChatMessage,
) ([]fantasy.Message, error) {
	prompt := make([]fantasy.Message, 0, len(messages))
	toolNameByCallID := make(map[string]string)
	for _, message := range messages {
		visibility := message.Visibility
		if visibility == "" {
			visibility = database.ChatMessageVisibilityBoth
		}
		if visibility != database.ChatMessageVisibilityModel &&
			visibility != database.ChatMessageVisibilityBoth {
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
			parts := normalizeAssistantToolCallInputs(ToMessageParts(content))
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
			rows, err := parseToolResultRows(message.Content)
			if err != nil {
				return nil, err
			}
			parts := make([]fantasy.MessagePart, 0, len(rows))
			for _, row := range rows {
				if row.ToolCallID != "" && row.ToolName != "" {
					toolNameByCallID[sanitizeToolCallID(row.ToolCallID)] = row.ToolName
				}
				parts = append(parts, row.toToolResultPart())
			}
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleTool,
				Content: parts,
			})
		default:
			return nil, xerrors.Errorf("unsupported chat message role %q", message.Role)
		}
	}
	prompt = injectMissingToolResults(prompt)
	prompt = injectMissingToolUses(
		prompt,
		toolNameByCallID,
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

// toolResultRaw is an untyped representation of a persisted tool
// result row. We intentionally avoid a strict Go struct so that
// historical shapes are never rejected.
type toolResultRaw struct {
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	Result     json.RawMessage `json:"result"`
	IsError    bool            `json:"is_error,omitempty"`
}

// parseToolResultRows decodes persisted tool result rows.
func parseToolResultRows(raw pqtype.NullRawMessage) ([]toolResultRaw, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return nil, nil
	}

	var rows []toolResultRaw
	if err := json.Unmarshal(raw.RawMessage, &rows); err != nil {
		return nil, xerrors.Errorf("parse tool content: %w", err)
	}
	return rows, nil
}

func (r toolResultRaw) toToolResultPart() fantasy.ToolResultPart {
	toolCallID := sanitizeToolCallID(r.ToolCallID)
	resultText := string(r.Result)
	if resultText == "" || resultText == "null" {
		resultText = "{}"
	}

	if r.IsError {
		message := strings.TrimSpace(resultText)
		if extracted := extractErrorString(r.Result); extracted != "" {
			message = extracted
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
			Text: resultText,
		},
	}
}

// extractErrorString pulls the "error" field from a JSON object if
// present, returning it as a string. Returns "" if the field is
// missing or the input is not an object.
func extractErrorString(raw json.RawMessage) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	errField, ok := fields["error"]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(errField, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
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

func normalizeAssistantToolCallInputs(
	parts []fantasy.MessagePart,
) []fantasy.MessagePart {
	normalized := make([]fantasy.MessagePart, 0, len(parts))
	for _, part := range parts {
		toolCall, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
		if !ok {
			normalized = append(normalized, part)
			continue
		}

		toolCall.Input = normalizeToolCallInput(toolCall.Input)
		normalized = append(normalized, toolCall)
	}
	return normalized
}

// normalizeToolCallInput guarantees tool call input is a JSON object string.
// Anthropic drops assistant tool calls with malformed input, which can leave
// following tool results orphaned.
func normalizeToolCallInput(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "{}"
	}

	var object map[string]any
	if err := json.Unmarshal([]byte(input), &object); err != nil || object == nil {
		return "{}"
	}

	return input
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

// MarshalToolResult encodes a single tool result for persistence as
// an opaque JSON blob. The stored shape is
// [{"tool_call_id":…,"tool_name":…,"result":…,"is_error":…}].
func MarshalToolResult(toolCallID, toolName string, result json.RawMessage, isError bool) (pqtype.NullRawMessage, error) {
	row := toolResultRaw{
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Result:     result,
		IsError:    isError,
	}
	data, err := json.Marshal([]toolResultRaw{row})
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode tool result: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

// MarshalToolResultContent encodes a fantasy tool result content
// block for persistence. It extracts the raw fields and delegates
// to MarshalToolResult.
func MarshalToolResultContent(content fantasy.ToolResultContent) (pqtype.NullRawMessage, error) {
	var result json.RawMessage
	var isError bool

	switch output := content.Result.(type) {
	case fantasy.ToolResultOutputContentError:
		isError = true
		if output.Error != nil {
			result, _ = json.Marshal(map[string]any{"error": output.Error.Error()})
		} else {
			result = []byte(`{"error":""}`)
		}
	case fantasy.ToolResultOutputContentText:
		result = json.RawMessage(output.Text)
		if !json.Valid(result) {
			result, _ = json.Marshal(map[string]any{"output": output.Text})
		}
	case fantasy.ToolResultOutputContentMedia:
		result, _ = json.Marshal(map[string]any{
			"data":      output.Data,
			"mime_type": output.MediaType,
			"text":      output.Text,
		})
	default:
		result = []byte(`{}`)
	}

	return MarshalToolResult(content.ToolCallID, content.ToolName, result, isError)
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
		return toolResultContentToPart(value)
	case *fantasy.ToolResultContent:
		return toolResultContentToPart(*value)
	default:
		return codersdk.ChatMessagePart{}
	}
}

// ToolResultToPart converts a tool call ID, raw result, and error
// flag into a ChatMessagePart. This is the minimal conversion used
// both during streaming and when reading from the database.
func ToolResultToPart(toolCallID, toolName string, result json.RawMessage, isError bool) codersdk.ChatMessagePart {
	return codersdk.ChatMessagePart{
		Type:       codersdk.ChatMessagePartTypeToolResult,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Result:     result,
		IsError:    isError,
	}
}

// toolResultContentToPart converts a fantasy ToolResultContent
// directly into a ChatMessagePart without an intermediate struct.
func toolResultContentToPart(content fantasy.ToolResultContent) codersdk.ChatMessagePart {
	var result json.RawMessage
	var isError bool

	switch output := content.Result.(type) {
	case fantasy.ToolResultOutputContentError:
		isError = true
		if output.Error != nil {
			result, _ = json.Marshal(map[string]any{"error": output.Error.Error()})
		} else {
			result = []byte(`{"error":""}`)
		}
	case fantasy.ToolResultOutputContentText:
		result = json.RawMessage(output.Text)
		// Ensure valid JSON; wrap in an object if not.
		if !json.Valid(result) {
			result, _ = json.Marshal(map[string]any{"output": output.Text})
		}
	case fantasy.ToolResultOutputContentMedia:
		result, _ = json.Marshal(map[string]any{
			"data":      output.Data,
			"mime_type": output.MediaType,
			"text":      output.Text,
		})
	default:
		result = []byte(`{}`)
	}

	return ToolResultToPart(content.ToolCallID, content.ToolName, result, isError)
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
		var missing []fantasy.MessagePart
		for _, tc := range toolCalls {
			if _, ok := answered[tc.ToolCallID]; !ok {
				missing = append(missing, fantasy.ToolResultPart{
					ToolCallID: tc.ToolCallID,
					Output: fantasy.ToolResultOutputContentError{
						Error: xerrors.New("tool call was interrupted and did not receive a result"),
					},
				})
			}
		}
		if len(missing) > 0 {
			result = append(result, fantasy.Message{
				Role:    fantasy.MessageRoleTool,
				Content: missing,
			})
		}
	}
	return result
}

func injectMissingToolUses(
	prompt []fantasy.Message,
	toolNameByCallID map[string]string,
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

func truncateRunes(value string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}

	return string(runes[:maxLen])
}
