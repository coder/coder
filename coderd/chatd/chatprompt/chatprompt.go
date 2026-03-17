package chatprompt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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
// messages without resolving file references from storage. Inline
// file data is preserved when present (backward compat).
func ConvertMessages(
	messages []database.ChatMessage,
) ([]fantasy.Message, error) {
	return ConvertMessagesWithFiles(context.Background(), messages, nil, slog.Logger{})
}

// ConvertMessagesWithFiles converts persisted chat messages into LLM
// prompt messages, resolving file references via the provided
// resolver. When resolver is nil, file blocks without inline data
// are passed through as-is (same behavior as ConvertMessages).
func ConvertMessagesWithFiles(
	ctx context.Context,
	messages []database.ChatMessage,
	resolver FileResolver,
	logger slog.Logger,
) ([]fantasy.Message, error) {
	// Phase 1: Parse all messages via ParseContent (→ SDK parts)
	// and collect file_id references from user messages for batch
	// resolution.
	type parsedMessage struct {
		role  codersdk.ChatMessageRole
		parts []codersdk.ChatMessagePart
	}
	parsed := make([]parsedMessage, len(messages))
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

		parts, err := ParseContent(msg)
		if err != nil {
			return nil, err
		}
		parsed[i] = parsedMessage{role: codersdk.ChatMessageRole(msg.Role), parts: parts}

		// Collect file IDs from user messages for resolution.
		if resolver != nil && msg.Role == database.ChatMessageRoleUser {
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeFile && part.FileID.Valid {
					if _, seen := seenFileIDs[part.FileID.UUID]; !seen {
						seenFileIDs[part.FileID.UUID] = struct{}{}
						allFileIDs = append(allFileIDs, part.FileID.UUID)
					}
				}
			}
		}
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

	// Phase 3: Build fantasy messages from SDK parts via
	// partsToMessageParts. Track tool names for injection.
	prompt := make([]fantasy.Message, 0, len(messages))
	toolNameByCallID := make(map[string]string)
	for _, pm := range parsed {
		if len(pm.parts) == 0 {
			continue
		}

		switch pm.role {
		case codersdk.ChatMessageRoleSystem:
			// System parts are always a single text part.
			prompt = append(prompt, fantasy.Message{
				Role: fantasy.MessageRoleSystem,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: pm.parts[0].Text},
				},
			})
		case codersdk.ChatMessageRoleUser:
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleUser,
				Content: partsToMessageParts(logger, pm.parts, resolved),
			})
		case codersdk.ChatMessageRoleAssistant:
			fantasyParts := normalizeAssistantToolCallInputs(
				partsToMessageParts(logger, pm.parts, resolved),
			)
			for _, toolCall := range ExtractToolCalls(fantasyParts) {
				if toolCall.ToolCallID == "" || strings.TrimSpace(toolCall.ToolName) == "" {
					continue
				}
				toolNameByCallID[sanitizeToolCallID(toolCall.ToolCallID)] = toolCall.ToolName
			}
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleAssistant,
				Content: fantasyParts,
			})
		case codersdk.ChatMessageRoleTool:
			// Track tool names from SDK parts before conversion.
			for _, part := range pm.parts {
				if part.Type == codersdk.ChatMessagePartTypeToolResult {
					if part.ToolCallID != "" && part.ToolName != "" {
						toolNameByCallID[sanitizeToolCallID(part.ToolCallID)] = part.ToolName
					}
				}
			}
			prompt = append(prompt, fantasy.Message{
				Role:    fantasy.MessageRoleTool,
				Content: partsToMessageParts(logger, pm.parts, resolved),
			})
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

const (
	// ContentVersionV0 is the legacy content format. Parsing uses
	// role-aware heuristics to distinguish fantasy envelope format
	// from SDK parts.
	ContentVersionV0 int16 = 0
	// ContentVersionV1 stores content as []codersdk.ChatMessagePart
	// JSON for all roles.
	ContentVersionV1 int16 = 1

	// CurrentContentVersion is the version used for new inserts.
	CurrentContentVersion = ContentVersionV1
)

// ParseContent decodes persisted chat message content blocks into
// SDK parts. Dispatches on content version: version 0 (legacy) uses
// a role-aware heuristic to distinguish fantasy envelope format
// from SDK parts, version 1 (current) unmarshals SDK-format
// []ChatMessagePart directly.
func ParseContent(msg database.ChatMessage) ([]codersdk.ChatMessagePart, error) {
	if !msg.Content.Valid || len(msg.Content.RawMessage) == 0 {
		return nil, nil
	}

	role := codersdk.ChatMessageRole(msg.Role)

	switch msg.ContentVersion {
	case ContentVersionV0:
		return parseLegacyContent(role, msg.Content)
	case ContentVersionV1:
		return parseContentV1(role, msg.Content)
	default:
		return nil, xerrors.Errorf("unsupported content version %d", msg.ContentVersion)
	}
}

// parseLegacyContent handles content version 0, where the format
// varies by role and era. Uses structural heuristics to distinguish
// fantasy envelope format from SDK parts.
func parseLegacyContent(role codersdk.ChatMessageRole, raw pqtype.NullRawMessage) ([]codersdk.ChatMessagePart, error) {
	switch role {
	case codersdk.ChatMessageRoleSystem:
		return parseSystemRole(raw)
	case codersdk.ChatMessageRoleAssistant:
		return parseAssistantRole(raw)
	case codersdk.ChatMessageRoleTool:
		return parseToolRole(raw)
	case codersdk.ChatMessageRoleUser:
		return parseUserRole(raw)
	default:
		return nil, xerrors.Errorf("unsupported chat message role %q", role)
	}
}

// parseContentV1 handles content version 1. Content is a JSON
// array of ChatMessagePart structs.
func parseContentV1(role codersdk.ChatMessageRole, raw pqtype.NullRawMessage) ([]codersdk.ChatMessagePart, error) {
	var parts []codersdk.ChatMessagePart
	if err := json.Unmarshal(raw.RawMessage, &parts); err != nil {
		return nil, xerrors.Errorf("parse %s content: %w", role, err)
	}
	return parts, nil
}

// parseSystemRole decodes a system message (JSON string) into a
// single text part.
func parseSystemRole(raw pqtype.NullRawMessage) ([]codersdk.ChatMessagePart, error) {
	var text string
	if err := json.Unmarshal(raw.RawMessage, &text); err != nil {
		return nil, xerrors.Errorf("parse system content: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}, nil
}

// parseAssistantRole uses the structural heuristic to distinguish
// legacy fantasy envelope from new SDK parts. We don't use
// try/fallback here because json.Unmarshal of a fantasy envelope
// into []ChatMessagePart can partially succeed (Type gets set from
// the envelope's "type" field) while silently losing content. The
// only thing preventing that today is that Data ([]byte) rejects
// the envelope's "data" JSON object, but that's a brittle
// invariant tied to Go's json decoder behavior for []byte.
func parseAssistantRole(raw pqtype.NullRawMessage) ([]codersdk.ChatMessagePart, error) {
	if isFantasyEnvelopeFormat(raw.RawMessage) {
		return parseLegacyFantasyBlocks(string(codersdk.ChatMessageRoleAssistant), raw)
	}

	// New SDK format.
	var parts []codersdk.ChatMessagePart
	if err := json.Unmarshal(raw.RawMessage, &parts); err != nil {
		return nil, xerrors.Errorf("parse assistant content: %w", err)
	}
	if !hasNonEmptyType(parts) {
		return nil, nil
	}
	return parts, nil
}

// parseToolRole tries SDK parts first, then falls back to legacy
// tool result rows. Unlike assistant/user roles, tool messages
// don't need the isFantasyEnvelopeFormat heuristic: legacy tool
// result rows have no "type" field (just tool_call_id, tool_name,
// result), so hasToolResultType reliably rejects them.
func parseToolRole(raw pqtype.NullRawMessage) ([]codersdk.ChatMessagePart, error) {
	// Try SDK parts.
	var parts []codersdk.ChatMessagePart
	if err := json.Unmarshal(raw.RawMessage, &parts); err == nil && hasToolResultType(parts) {
		return parts, nil
	}

	// Fall back to legacy tool result rows.
	rows, err := parseToolResultRows(raw)
	if err != nil {
		return nil, err
	}
	parts = make([]codersdk.ChatMessagePart, 0, len(rows))
	for _, row := range rows {
		part := codersdk.ChatMessageToolResult(row.ToolCallID, row.ToolName, row.Result, row.IsError)
		part.ProviderExecuted = row.ProviderExecuted
		part.ProviderMetadata = row.ProviderMetadata
		parts = append(parts, part)
	}
	return parts, nil
}

// parseUserRole uses a structural heuristic to distinguish legacy
// fantasy envelope from new SDK parts.
func parseUserRole(raw pqtype.NullRawMessage) ([]codersdk.ChatMessagePart, error) {
	// Legacy: plain JSON string (very old format).
	var text string
	if err := json.Unmarshal(raw.RawMessage, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		return []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}, nil
	}

	if isFantasyEnvelopeFormat(raw.RawMessage) {
		return parseLegacyUserBlocks(raw)
	}

	// New SDK format.
	var parts []codersdk.ChatMessagePart
	if err := json.Unmarshal(raw.RawMessage, &parts); err != nil {
		return nil, xerrors.Errorf("parse user content: %w", err)
	}
	if !hasNonEmptyType(parts) {
		return nil, nil
	}
	return parts, nil
}

// parseLegacyUserBlocks decodes a user message stored in fantasy
// envelope format, extracting file_id references from the raw
// envelope for file-type blocks.
func parseLegacyUserBlocks(raw pqtype.NullRawMessage) ([]codersdk.ChatMessagePart, error) {
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(raw.RawMessage, &rawBlocks); err != nil {
		return nil, xerrors.Errorf("parse user content: %w", err)
	}

	parts := make([]codersdk.ChatMessagePart, 0, len(rawBlocks))
	for i, rawBlock := range rawBlocks {
		block, err := fantasy.UnmarshalContent(rawBlock)
		if err != nil {
			return nil, xerrors.Errorf("parse user content block %d: %w", i, err)
		}
		part := PartFromContent(block)
		if part.Type == "" {
			continue
		}
		// For file-type blocks, extract file_id from the raw
		// envelope's data sub-object.
		if part.Type == codersdk.ChatMessagePartTypeFile {
			if fid, err := ExtractFileID(rawBlock); err == nil {
				part.FileID = uuid.NullUUID{UUID: fid, Valid: true}
				// Clear inline data when file_id is present;
				// resolved at LLM dispatch time.
				part.Data = nil
			}
		}
		parts = append(parts, part)
	}
	return parts, nil
}

// parseLegacyFantasyBlocks decodes an assistant message stored in
// fantasy envelope format, converting each block via PartFromContent
// which preserves ProviderMetadata.
func parseLegacyFantasyBlocks(role string, raw pqtype.NullRawMessage) ([]codersdk.ChatMessagePart, error) {
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
		parts = append(parts, part)
	}
	return parts, nil
}

// hasNonEmptyType returns true if at least one part has a non-empty
// Type field, indicating a valid SDK parts array.
func hasNonEmptyType(parts []codersdk.ChatMessagePart) bool {
	for _, p := range parts {
		if p.Type != "" {
			return true
		}
	}
	return false
}

// hasToolResultType returns true if at least one part has Type ==
// ToolResult, indicating a valid SDK tool-result array.
func hasToolResultType(parts []codersdk.ChatMessagePart) bool {
	for _, p := range parts {
		if p.Type == codersdk.ChatMessagePartTypeToolResult {
			return true
		}
	}
	return false
}

// toolResultRaw is an untyped representation of a persisted tool
// result row. We intentionally avoid a strict Go struct so that
// historical shapes are never rejected.
type toolResultRaw struct {
	ToolCallID       string          `json:"tool_call_id"`
	ToolName         string          `json:"tool_name"`
	Result           json.RawMessage `json:"result"`
	IsError          bool            `json:"is_error,omitempty"`
	ProviderExecuted bool            `json:"provider_executed,omitempty"`
	ProviderMetadata json.RawMessage `json:"provider_metadata,omitempty"`
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

// MarshalContent encodes message content blocks in legacy fantasy
// envelope format. Retained for backward-compatible test fixtures
// that create legacy-format DB rows. Production write paths use
// MarshalParts instead.
func MarshalContent(blocks []fantasy.Content, fileIDs map[int]uuid.UUID) (pqtype.NullRawMessage, error) {
	if len(blocks) == 0 {
		return pqtype.NullRawMessage{}, nil
	}

	encodedBlocks := make([]json.RawMessage, 0, len(blocks))
	for i, block := range blocks {
		encoded, err := json.Marshal(block)
		if err != nil {
			return pqtype.NullRawMessage{}, xerrors.Errorf(
				"encode content block %d: %w",
				i,
				err,
			)
		}
		if fid, ok := fileIDs[i]; ok {
			// Inline file_id injection into the fantasy envelope's
			// data sub-object, stripping inline data.
			var envelope struct {
				Type string `json:"type"`
				Data struct {
					MediaType        string           `json:"media_type"`
					Data             json.RawMessage  `json:"data,omitempty"`
					FileID           string           `json:"file_id,omitempty"`
					ProviderMetadata *json.RawMessage `json:"provider_metadata,omitempty"`
				} `json:"data"`
			}
			if err := json.Unmarshal(encoded, &envelope); err == nil {
				envelope.Data.FileID = fid.String()
				envelope.Data.Data = nil
				if patched, err := json.Marshal(envelope); err == nil {
					encoded = patched
				}
			}
		}
		encodedBlocks = append(encodedBlocks, encoded)
	}

	data, err := json.Marshal(encodedBlocks)
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode content blocks: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

// MarshalToolResult encodes a single tool result in the legacy
// tool-row format. Retained for test fixtures that create
// legacy-format DB rows. Production write paths use MarshalParts.
// The stored shape is
// [{"tool_call_id":…,"tool_name":…,"result":…,"is_error":…}].
func MarshalToolResult(toolCallID, toolName string, result json.RawMessage, isError bool, providerExecuted bool, providerMetadata fantasy.ProviderMetadata) (pqtype.NullRawMessage, error) {
	var metaJSON json.RawMessage
	if len(providerMetadata) > 0 {
		var err error
		metaJSON, err = json.Marshal(providerMetadata)
		if err != nil {
			return pqtype.NullRawMessage{}, xerrors.Errorf("encode provider metadata: %w", err)
		}
	}
	row := toolResultRaw{
		ToolCallID:       toolCallID,
		ToolName:         toolName,
		Result:           result,
		IsError:          isError,
		ProviderExecuted: providerExecuted,
		ProviderMetadata: metaJSON,
	}
	data, err := json.Marshal([]toolResultRaw{row})
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode tool result: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

// PartFromContent converts fantasy content into a SDK chat message
// part, preserving ProviderMetadata and ProviderExecuted fields.
func PartFromContent(block fantasy.Content) codersdk.ChatMessagePart {
	switch value := block.(type) {
	case fantasy.TextContent:
		return codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeText,
			Text:             value.Text,
			ProviderMetadata: marshalProviderMetadata(value.ProviderMetadata),
		}
	case *fantasy.TextContent:
		return codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeText,
			Text:             value.Text,
			ProviderMetadata: marshalProviderMetadata(value.ProviderMetadata),
		}
	case fantasy.ReasoningContent:
		return codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeReasoning,
			Text:             value.Text,
			ProviderMetadata: marshalProviderMetadata(value.ProviderMetadata),
		}
	case *fantasy.ReasoningContent:
		return codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeReasoning,
			Text:             value.Text,
			ProviderMetadata: marshalProviderMetadata(value.ProviderMetadata),
		}
	case fantasy.ToolCallContent:
		return codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeToolCall,
			ToolCallID:       value.ToolCallID,
			ToolName:         value.ToolName,
			Args:             safeToolCallArgs(value.Input),
			ProviderExecuted: value.ProviderExecuted,
			ProviderMetadata: marshalProviderMetadata(value.ProviderMetadata),
		}
	case *fantasy.ToolCallContent:
		return codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeToolCall,
			ToolCallID:       value.ToolCallID,
			ToolName:         value.ToolName,
			Args:             safeToolCallArgs(value.Input),
			ProviderExecuted: value.ProviderExecuted,
			ProviderMetadata: marshalProviderMetadata(value.ProviderMetadata),
		}
	case fantasy.SourceContent:
		return codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeSource,
			SourceID:         value.ID,
			URL:              value.URL,
			Title:            value.Title,
			ProviderMetadata: marshalProviderMetadata(value.ProviderMetadata),
		}
	case *fantasy.SourceContent:
		return codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeSource,
			SourceID:         value.ID,
			URL:              value.URL,
			Title:            value.Title,
			ProviderMetadata: marshalProviderMetadata(value.ProviderMetadata),
		}
	case fantasy.FileContent:
		return codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeFile,
			MediaType:        value.MediaType,
			Data:             value.Data,
			ProviderMetadata: marshalProviderMetadata(value.ProviderMetadata),
		}
	case *fantasy.FileContent:
		return codersdk.ChatMessagePart{
			Type:             codersdk.ChatMessagePartTypeFile,
			MediaType:        value.MediaType,
			Data:             value.Data,
			ProviderMetadata: marshalProviderMetadata(value.ProviderMetadata),
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
	return codersdk.ChatMessageToolResult(toolCallID, toolName, result, isError)
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

	part := ToolResultToPart(content.ToolCallID, content.ToolName, result, isError)
	part.ProviderExecuted = content.ProviderExecuted
	part.ProviderMetadata = marshalProviderMetadata(content.ProviderMetadata)
	return part
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
		// Provider-executed tool calls (e.g. web_search) are
		// handled server-side by the LLM provider. Their results
		// may arrive in a later step and end up stored out of
		// position, so we must not inject synthetic error results
		// for them. The provider will re-execute the tool when it
		// sees the server_tool_use without a matching result.
		var missing []fantasy.MessagePart
		for _, tc := range toolCalls {
			if tc.ProviderExecuted {
				continue
			}
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

		allToolResults := make([]fantasy.ToolResultPart, 0, len(msg.Content))
		for _, part := range msg.Content {
			toolResult, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
			if !ok {
				continue
			}
			allToolResults = append(allToolResults, toolResult)
		}
		if len(allToolResults) == 0 {
			result = append(result, msg)
			continue
		}

		// Provider-executed tool results (e.g. web_search) may be
		// persisted in a later step than the assistant message that
		// initiated the tool call. When that happens they appear as
		// orphans after the wrong assistant message. Filter them
		// out before matching — the provider will re-execute the
		// tool, and the search results are already captured in the
		// subsequent assistant message's sources/text.
		toolResults := make([]fantasy.ToolResultPart, 0, len(allToolResults))
		for _, tr := range allToolResults {
			if !tr.ProviderExecuted {
				toolResults = append(toolResults, tr)
			}
		}
		if len(toolResults) == 0 {
			// All results were provider-executed; drop the message.
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
			// Rebuild the message from the filtered results so
			// dropped provider-executed results are excluded.
			result = append(result, toolMessageFromToolResultParts(matchingResults))
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

func sanitizeToolCallID(id string) string {
	if id == "" {
		return ""
	}
	return toolCallIDSanitizer.ReplaceAllString(id, "_")
}

// MarshalParts encodes SDK chat message parts for persistence.
func MarshalParts(parts []codersdk.ChatMessagePart) (pqtype.NullRawMessage, error) {
	if len(parts) == 0 {
		return pqtype.NullRawMessage{}, nil
	}
	data, err := json.Marshal(parts)
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode chat message parts: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

// isFantasyEnvelopeFormat checks whether raw message content uses
// the fantasy envelope format (legacy) vs SDK parts (new). It
// examines the first array element for a "data" field containing a
// JSON object (starts with '{'). Fantasy always serializes Data
// from json.Marshal(struct{...}), producing a JSON object.
// ChatMessagePart.Data is []byte, which serializes to a base64
// string or is omitted via omitempty. This structural invariant
// means a "data" field starting with '{' can only come from
// fantasy.
func isFantasyEnvelopeFormat(raw json.RawMessage) bool {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) == 0 {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(arr[0], &fields); err != nil {
		return false
	}
	data, ok := fields["data"]
	if !ok {
		return false
	}
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// marshalProviderMetadata converts fantasy provider metadata to raw
// JSON for storage in SDK parts.
func marshalProviderMetadata(metadata fantasy.ProviderMetadata) json.RawMessage {
	if len(metadata) == 0 {
		return nil
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return nil
	}
	return data
}

// providerMetadataToOptions reconstructs fantasy ProviderOptions
// from raw JSON stored in an SDK part's ProviderMetadata field.
// Uses fantasy.UnmarshalProviderOptions to restore registered
// provider-specific types. Returns nil on failure.
func providerMetadataToOptions(logger slog.Logger, raw json.RawMessage) fantasy.ProviderOptions {
	if len(raw) == 0 {
		return nil
	}
	var intermediate map[string]json.RawMessage
	if err := json.Unmarshal(raw, &intermediate); err != nil {
		logger.Warn(context.Background(), "failed to unmarshal provider metadata", slog.Error(err))
		return nil
	}
	opts, err := fantasy.UnmarshalProviderOptions(intermediate)
	if err != nil {
		logger.Warn(context.Background(), "failed to decode provider options", slog.Error(err))
		return nil
	}
	return opts
}

// safeToolCallArgs ensures tool call args are valid JSON. Returns
// nil for empty or invalid input so the field is omitted.
func safeToolCallArgs(input string) json.RawMessage {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	raw := json.RawMessage(input)
	if !json.Valid(raw) {
		return nil
	}
	return raw
}

// fileReferencePartToText formats a file-reference SDK part as
// plain text for LLM consumption. LLMs don't understand
// file-reference natively, so we convert to a readable text
// representation.
func fileReferencePartToText(part codersdk.ChatMessagePart) string {
	lineRange := fmt.Sprintf("%d", part.StartLine)
	if part.StartLine != part.EndLine {
		lineRange = fmt.Sprintf("%d-%d", part.StartLine, part.EndLine)
	}
	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "[file-reference] %s:%s", part.FileName, lineRange)
	if content := strings.TrimSpace(part.Content); content != "" {
		_, _ = fmt.Fprintf(&sb, "\n```%s\n%s\n```", part.FileName, content)
	}
	return sb.String()
}

// toolResultPartToMessagePart converts an SDK tool-result part
// into a fantasy ToolResultPart for LLM dispatch.
func toolResultPartToMessagePart(logger slog.Logger, part codersdk.ChatMessagePart) fantasy.ToolResultPart {
	toolCallID := sanitizeToolCallID(part.ToolCallID)
	resultText := string(part.Result)
	if resultText == "" || resultText == "null" {
		resultText = "{}"
	}

	opts := providerMetadataToOptions(logger, part.ProviderMetadata)

	if part.IsError {
		message := strings.TrimSpace(resultText)
		if extracted := extractErrorString(part.Result); extracted != "" {
			message = extracted
		}
		return fantasy.ToolResultPart{
			ToolCallID:       toolCallID,
			ProviderExecuted: part.ProviderExecuted,
			Output: fantasy.ToolResultOutputContentError{
				Error: xerrors.New(message),
			},
			ProviderOptions: opts,
		}
	}

	return fantasy.ToolResultPart{
		ToolCallID:       toolCallID,
		ProviderExecuted: part.ProviderExecuted,
		Output: fantasy.ToolResultOutputContentText{
			Text: resultText,
		},
		ProviderOptions: opts,
	}
}

// partsToMessageParts converts SDK chat message parts into fantasy
// message parts for LLM dispatch. It handles file data injection
// from resolved files, file-reference to text conversion, and
// source part skipping.
func partsToMessageParts(
	logger slog.Logger,
	parts []codersdk.ChatMessagePart,
	resolved map[uuid.UUID]FileData,
) []fantasy.MessagePart {
	result := make([]fantasy.MessagePart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeText:
			result = append(result, fantasy.TextPart{
				Text:            part.Text,
				ProviderOptions: providerMetadataToOptions(logger, part.ProviderMetadata),
			})
		case codersdk.ChatMessagePartTypeReasoning:
			result = append(result, fantasy.ReasoningPart{
				Text:            part.Text,
				ProviderOptions: providerMetadataToOptions(logger, part.ProviderMetadata),
			})
		case codersdk.ChatMessagePartTypeToolCall:
			result = append(result, fantasy.ToolCallPart{
				ToolCallID:       sanitizeToolCallID(part.ToolCallID),
				ToolName:         part.ToolName,
				Input:            string(part.Args),
				ProviderExecuted: part.ProviderExecuted,
				ProviderOptions:  providerMetadataToOptions(logger, part.ProviderMetadata),
			})
		case codersdk.ChatMessagePartTypeToolResult:
			result = append(result, toolResultPartToMessagePart(logger, part))
		case codersdk.ChatMessagePartTypeFile:
			data := part.Data
			mediaType := part.MediaType
			if part.FileID.Valid {
				if fd, ok := resolved[part.FileID.UUID]; ok {
					data = fd.Data
					if mediaType == "" {
						mediaType = fd.MediaType
					}
				}
			}
			result = append(result, fantasy.FilePart{
				Data:            data,
				MediaType:       mediaType,
				ProviderOptions: providerMetadataToOptions(logger, part.ProviderMetadata),
			})
		case codersdk.ChatMessagePartTypeFileReference:
			// LLMs don't understand file-reference natively.
			result = append(result, fantasy.TextPart{
				Text: fileReferencePartToText(part),
			})
		case codersdk.ChatMessagePartTypeSource:
			// Source parts are metadata-only, not sent to LLM.
			continue
		}
	}
	return result
}
