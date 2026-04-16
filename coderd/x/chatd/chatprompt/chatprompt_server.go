//go:build !slim

package chatprompt

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// ConvertMessages converts persisted chat messages into LLM prompt
// messages without resolving file references from storage. Inline
// file data is preserved when present (backward compat).
func ConvertMessages(
	messages []database.ChatMessage,
) ([]fantasy.Message, error) {
	return ConvertMessagesWithFiles(context.Background(), messages, nil, slog.Logger{})
}

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
	decodeNulInParts(parts)
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
		part := codersdk.ChatMessageToolResult(row.ToolCallID, row.ToolName, row.Result, row.IsError, row.IsMedia)
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
	IsMedia          bool            `json:"is_media,omitempty"`
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
// [{"tool_call_id":…,"tool_name":…,"result":…,"is_error":…,"is_media":…}].
func MarshalToolResult(toolCallID, toolName string, result json.RawMessage, isError bool, isMedia bool, providerExecuted bool, providerMetadata fantasy.ProviderMetadata) (pqtype.NullRawMessage, error) {
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
		IsMedia:          isMedia,
		ProviderExecuted: providerExecuted,
		ProviderMetadata: metaJSON,
	}
	data, err := json.Marshal([]toolResultRaw{row})
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode tool result: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}

// MarshalParts encodes SDK chat message parts for persistence.
// NUL characters in string fields are encoded as PUA sentinel
// pairs (U+E000 U+E001) before marshaling so the resulting JSON
// never contains \u0000 (rejected by PostgreSQL jsonb). The
// encoding operates on Go string values, not JSON bytes, so it
// survives jsonb text normalization.
func MarshalParts(parts []codersdk.ChatMessagePart) (pqtype.NullRawMessage, error) {
	if len(parts) == 0 {
		return pqtype.NullRawMessage{}, nil
	}
	data, err := json.Marshal(encodeNulInParts(parts))
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("encode chat message parts: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}, nil
}
