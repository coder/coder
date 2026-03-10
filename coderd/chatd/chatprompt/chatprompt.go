package chatprompt

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

var toolCallIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// FileData holds resolved file content for LLM prompt building.
type FileData struct {
	Data      []byte
	MediaType string
}

// FileResolver fetches file content by ID for LLM prompt building.
type FileResolver func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]FileData, error)

// ExtractFileID parses the file_id from a serialized file content
// block envelope. Returns uuid.Nil and an error when the block is
// not a file-type block or has no file_id.
func ExtractFileID(raw json.RawMessage) (uuid.UUID, error) {
	var envelope struct {
		Type string `json:"type"`
		Data struct {
			FileID string `json:"file_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return uuid.Nil, xerrors.Errorf("unmarshal content block: %w", err)
	}
	if !strings.EqualFold(envelope.Type, string(fantasy.ContentTypeFile)) {
		return uuid.Nil, xerrors.Errorf("not a file content block: %s", envelope.Type)
	}
	if envelope.Data.FileID == "" {
		return uuid.Nil, xerrors.New("no file_id")
	}
	return uuid.Parse(envelope.Data.FileID)
}

// ConvertMessages converts persisted chat messages into LLM prompt
// messages without resolving file references from storage.
func ConvertMessages(
	messages []database.ChatMessage,
) ([]fantasy.Message, error) {
	return ConvertMessagesWithFiles(context.Background(), messages, nil)
}

// ConvertMessagesWithFiles converts persisted chat messages into LLM
// prompt messages, resolving file references via the provided
// resolver.
func ConvertMessagesWithFiles(
	ctx context.Context,
	messages []database.ChatMessage,
	resolver FileResolver,
) ([]fantasy.Message, error) {
	// Phase 1: Parse all messages and collect file IDs from user messages.
	type parsedMsg struct {
		role    string
		parts   []codersdk.ChatMessagePart
		fileIDs map[int]uuid.UUID // block index → file UUID (file parts only)
	}
	parsed := make([]parsedMsg, len(messages))
	var allFileIDs []uuid.UUID
	seenFileIDs := make(map[uuid.UUID]struct{})

	for i, msg := range messages {
		visibility := msg.Visibility
		if visibility == "" {
			visibility = database.ChatMessageVisibilityBoth
		}
		if visibility != database.ChatMessageVisibilityModel &&
			visibility != database.ChatMessageVisibilityBoth {
			continue
		}

		if msg.Role == string(fantasy.MessageRoleSystem) {
			content, err := parseSystemContent(msg.Content)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(content) != "" {
				parsed[i] = parsedMsg{
					role: msg.Role,
					parts: []codersdk.ChatMessagePart{{
						Type: codersdk.ChatMessagePartTypeText,
						Text: content,
					}},
				}
			}
			continue
		}

		if msg.Role == string(fantasy.MessageRoleTool) {
			rows, err := parseToolResultRows(msg.Content)
			if err != nil {
				return nil, err
			}
			toolParts := make([]codersdk.ChatMessagePart, 0, len(rows))
			for _, row := range rows {
				toolParts = append(toolParts, codersdk.ChatMessagePart{
					Type:       codersdk.ChatMessagePartTypeToolResult,
					ToolCallID: row.ToolCallID,
					ToolName:   row.ToolName,
					Result:     row.Result,
					IsError:    row.IsError,
				})
			}
			parsed[i] = parsedMsg{role: msg.Role, parts: toolParts}
			continue
		}

		// User and assistant messages.
		parts, err := ParseContent(msg.Role, msg.Content)
		if err != nil {
			return nil, err
		}

		pm := parsedMsg{role: msg.Role, parts: parts}

		// Collect file IDs from user messages for batch resolution.
		if resolver != nil && msg.Role == string(fantasy.MessageRoleUser) {
			for j, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeFile && part.FileID.Valid {
					if pm.fileIDs == nil {
						pm.fileIDs = make(map[int]uuid.UUID)
					}
					pm.fileIDs[j] = part.FileID.UUID
					if _, seen := seenFileIDs[part.FileID.UUID]; !seen {
						seenFileIDs[part.FileID.UUID] = struct{}{}
						allFileIDs = append(allFileIDs, part.FileID.UUID)
					}
				}
			}
		}
		parsed[i] = pm
	}

	// Phase 2: Batch resolve file data.
	var resolved map[uuid.UUID]FileData
	if len(allFileIDs) > 0 {
		var err error
		resolved, err = resolver(ctx, allFileIDs)
		if err != nil {
			return nil, xerrors.Errorf("resolve chat files: %w", err)
		}
	}

	// Phase 3: Build fantasy prompt messages.
	prompt := make([]fantasy.Message, 0, len(messages))
	toolNameByCallID := make(map[string]string)
	for _, pm := range parsed {
		if pm.parts == nil {
			continue
		}
		switch pm.role {
		case string(fantasy.MessageRoleSystem):
			prompt = append(prompt, fantasy.Message{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: pm.parts[0].Text},
				},
			})
		case string(fantasy.MessageRoleUser):
			msgParts := partsToMessageParts(pm.parts, pm.fileIDs, resolved)
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleUser,
				Content: msgParts,
			})
		case string(fantasy.MessageRoleAssistant):
			msgParts := partsToMessageParts(pm.parts, nil, nil)
			msgParts = normalizeAssistantToolCallInputs(msgParts)
			for _, toolCall := range ExtractToolCalls(msgParts) {
				if toolCall.ToolCallID == "" || strings.TrimSpace(toolCall.ToolName) == "" {
					continue
				}
				toolNameByCallID[sanitizeToolCallID(toolCall.ToolCallID)] = toolCall.ToolName
			}
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleAssistant,
				Content: msgParts,
			})
		case string(fantasy.MessageRoleTool):
			parts := make([]fantasy.MessagePart, 0, len(pm.parts))
			for _, part := range pm.parts {
				if part.ToolCallID != "" && part.ToolName != "" {
					toolNameByCallID[sanitizeToolCallID(part.ToolCallID)] = part.ToolName
				}
				parts = append(parts, toolResultPartToMessagePart(part))
			}
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleTool,
				Content: parts,
			})
		}
	}
	prompt = injectMissingToolResults(prompt)
	prompt = injectMissingToolUses(prompt, toolNameByCallID)
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

// ParseContent decodes persisted chat message content into SDK
// parts. Handles both the new SDK format and legacy fantasy envelope
// format for backward compatibility.
func ParseContent(role string, raw pqtype.NullRawMessage) ([]codersdk.ChatMessagePart, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return nil, nil
	}

	// Plain JSON string (system messages, some legacy user messages).
	var text string
	if err := json.Unmarshal(raw.RawMessage, &text); err == nil {
		return []codersdk.ChatMessagePart{{
			Type: codersdk.ChatMessagePartTypeText,
			Text: text,
		}}, nil
	}

	// Try SDK format first (new storage format for all roles).
	if !IsFantasyEnvelopeFormat(raw.RawMessage) {
		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(raw.RawMessage, &parts); err == nil && len(parts) > 0 {
			return parts, nil
		}
	}

	// Fall back to fantasy envelope format (legacy rows).
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(raw.RawMessage, &rawBlocks); err != nil {
		return nil, xerrors.Errorf("parse %s content: %w", role, err)
	}

	parts := make([]codersdk.ChatMessagePart, 0, len(rawBlocks))
	for i, rawBlock := range rawBlocks {
		block, err := fantasy.UnmarshalContent(rawBlock)
		if err != nil {
			return nil, xerrors.Errorf("parse %s content block %d: %w", role, i, err)
		}
		part := PartFromContent(block)
		if part.Type == "" {
			continue
		}
		// For file blocks in fantasy envelope, extract file_id.
		if part.Type == codersdk.ChatMessagePartTypeFile {
			if fid, err := ExtractFileID(rawBlock); err == nil {
				part.FileID = uuid.NullUUID{UUID: fid, Valid: true}
				if len(part.Data) == 0 {
					part.Data = nil // Resolved at LLM dispatch time.
				}
			}
		}
		parts = append(parts, part)
	}
	return parts, nil
}

// fileReferencePartToText converts a ChatMessagePart file-reference
// into the text representation used for LLM dispatch.
func fileReferencePartToText(part codersdk.ChatMessagePart) string {
	lineRange := fmt.Sprintf("%d", part.StartLine)
	if part.StartLine != part.EndLine {
		lineRange = fmt.Sprintf("%d-%d", part.StartLine, part.EndLine)
	}
	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "[file-reference] %s:%s", part.FileName, lineRange)
	if strings.TrimSpace(part.Content) != "" {
		_, _ = fmt.Fprintf(&sb, "\n```%s\n%s\n```", part.FileName, strings.TrimSpace(part.Content))
	}
	return sb.String()
}

// IsFantasyEnvelopeFormat checks whether the JSON content uses the
// fantasy {"type": ..., "data": {...}} envelope format. Returns
// false for the newer SDK ChatMessagePart format or non-array
// content.
func IsFantasyEnvelopeFormat(raw json.RawMessage) bool {
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil || len(blocks) == 0 {
		return false
	}
	var probe struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(blocks[0], &probe); err != nil {
		return false
	}
	// Fantasy envelope wraps content in "data" as a JSON object.
	// The SDK ChatMessagePart.Data field serializes as a base64
	// string (not an object), so checking for '{' is safe.
	return len(probe.Data) > 0 && probe.Data[0] == '{'
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

// partsToMessageParts converts SDK parts to fantasy MessageParts for
// LLM dispatch. For file parts with resolved data, the file content
// is injected. For file-reference parts, a text representation is
// used.
func partsToMessageParts(
	parts []codersdk.ChatMessagePart,
	fileIDs map[int]uuid.UUID,
	resolved map[uuid.UUID]FileData,
) []fantasy.MessagePart {
	out := make([]fantasy.MessagePart, 0, len(parts))
	for i, part := range parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeText:
			out = append(out, fantasy.TextPart{
				Text:            part.Text,
				ProviderOptions: providerMetadataToOptions(part.ProviderMetadata),
			})
		case codersdk.ChatMessagePartTypeReasoning:
			out = append(out, fantasy.ReasoningPart{
				Text:            part.Text,
				ProviderOptions: providerMetadataToOptions(part.ProviderMetadata),
			})
		case codersdk.ChatMessagePartTypeToolCall:
			out = append(out, fantasy.ToolCallPart{
				ToolCallID:       sanitizeToolCallID(part.ToolCallID),
				ToolName:         part.ToolName,
				Input:            string(part.Args),
				ProviderExecuted: part.ProviderExecuted,
				ProviderOptions:  providerMetadataToOptions(part.ProviderMetadata),
			})
		case codersdk.ChatMessagePartTypeFile:
			fileData := part.Data
			mediaType := part.MediaType
			// Inject resolved file data if available.
			if fid, ok := fileIDs[i]; ok {
				if data, found := resolved[fid]; found {
					fileData = data.Data
					if mediaType == "" {
						mediaType = data.MediaType
					}
				}
			}
			out = append(out, fantasy.FilePart{
				Data:            fileData,
				MediaType:       mediaType,
				ProviderOptions: providerMetadataToOptions(part.ProviderMetadata),
			})
		case codersdk.ChatMessagePartTypeFileReference:
			out = append(out, fantasy.TextPart{
				Text: fileReferencePartToText(part),
			})
		case codersdk.ChatMessagePartTypeSource:
			// Sources don't have a direct fantasy.MessagePart equivalent;
			// pass as text for LLM context.
			out = append(out, fantasy.TextPart{
				Text: fmt.Sprintf("[source: %s](%s)", part.Title, part.URL),
			})
		case codersdk.ChatMessagePartTypeToolResult:
			out = append(out, toolResultPartToMessagePart(part))
		}
	}
	return out
}

// toolResultPartToMessagePart converts an SDK tool-result part to a
// fantasy ToolResultPart for LLM dispatch.
func toolResultPartToMessagePart(part codersdk.ChatMessagePart) fantasy.ToolResultPart {
	var output fantasy.ToolResultOutputContent
	if part.IsError {
		errMsg := ""
		if len(part.Result) > 0 {
			var errObj struct {
				Error string `json:"error"`
			}
			if json.Unmarshal(part.Result, &errObj) == nil && errObj.Error != "" {
				errMsg = errObj.Error
			} else {
				errMsg = string(part.Result)
			}
		}
		output = fantasy.ToolResultOutputContentError{
			Error: xerrors.New(errMsg),
		}
	} else if len(part.Result) > 0 {
		output = fantasy.ToolResultOutputContentText{
			Text: string(part.Result),
		}
	}
	return fantasy.ToolResultPart{
		ToolCallID:      sanitizeToolCallID(part.ToolCallID),
		Output:          output,
		ProviderOptions: providerMetadataToOptions(part.ProviderMetadata),
	}
}

// providerMetadataToOptions converts stored provider metadata JSON
// back into fantasy ProviderOptions for LLM dispatch round-trip.
func providerMetadataToOptions(raw json.RawMessage) fantasy.ProviderOptions {
	if len(raw) == 0 {
		return nil
	}
	var intermediate map[string]json.RawMessage
	if err := json.Unmarshal(raw, &intermediate); err != nil {
		return nil
	}
	opts, err := fantasy.UnmarshalProviderOptions(intermediate)
	if err != nil {
		return nil
	}
	return opts
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

// safeToolCallArgs converts a tool call input string to a
// json.RawMessage, returning nil for invalid JSON to avoid
// serialization failures in MarshalParts.
func safeToolCallArgs(input string) json.RawMessage {
	if input == "" {
		return nil
	}
	raw := json.RawMessage(input)
	if !json.Valid(raw) {
		return nil
	}
	return raw
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

// MarshalParts serializes message parts for database persistence.
// All roles now use the same SDK-native JSON format:
// [{"type":"text","text":"..."}, {"type":"tool-call",...}, ...].
func MarshalParts(parts []codersdk.ChatMessagePart) (pqtype.NullRawMessage, error) {
	if len(parts) == 0 {
		return pqtype.NullRawMessage{}, nil
	}
	data, err := json.Marshal(parts)
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("marshal content parts: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

// MarshalToolResult encodes a single tool result for persistence.
func MarshalToolResult(toolCallID, toolName string, result json.RawMessage, isError bool) (pqtype.NullRawMessage, error) {
	return MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolResult,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Result:     result,
		IsError:    isError,
	}})
}

// MarshalToolResultContent encodes a fantasy tool result content
// for persistence as a single-element SDK parts array.
func MarshalToolResultContent(content fantasy.ToolResultContent) (pqtype.NullRawMessage, error) {
	part := ToolResultContentToPart(content)
	return MarshalParts([]codersdk.ChatMessagePart{part})
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
			Type: codersdk.ChatMessagePartTypeReasoning,
			Text: value.Text,
		}
	case *fantasy.ReasoningContent:
		return codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeReasoning,
			Text: value.Text,
		}
	case fantasy.ToolCallContent:
		args := safeToolCallArgs(value.Input)
		return codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: value.ToolCallID,
			ToolName:   value.ToolName,
			Args:       args,
		}
	case *fantasy.ToolCallContent:
		args := safeToolCallArgs(value.Input)
		return codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: value.ToolCallID,
			ToolName:   value.ToolName,
			Args:       args,
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
		return ToolResultContentToPart(value)
	case *fantasy.ToolResultContent:
		return ToolResultContentToPart(*value)
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

// ToolResultContentToPart converts a fantasy ToolResultContent
// directly into a ChatMessagePart without an intermediate struct.
func ToolResultContentToPart(content fantasy.ToolResultContent) codersdk.ChatMessagePart {
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




