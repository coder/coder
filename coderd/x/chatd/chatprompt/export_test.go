package chatprompt

import (
	"encoding/json"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// SyntheticPasteTitleBudgetForTest exposes syntheticPasteTitleBudget
// for external tests.
const SyntheticPasteTitleBudgetForTest = syntheticPasteTitleBudget

// ToolResultPartToMessagePartForTest exposes toolResultPartToMessagePart
// for external tests.
var ToolResultPartToMessagePartForTest = toolResultPartToMessagePart

// EncodeNulInJSONForTest exposes encodeNulInJSON for external tests.
var EncodeNulInJSONForTest = encodeNulInJSON

// NeedsNulEncodingInJSONForTest exposes needsNulEncodingInJSON for external tests.
var NeedsNulEncodingInJSONForTest = needsNulEncodingInJSON

// ToolResultContentToPartForTest exposes toolResultContentToPart
// for external tests.
var ToolResultContentToPartForTest = func(logger slog.Logger, content fantasy.ToolResultContent) codersdk.ChatMessagePart {
	return toolResultContentToPart(logger, content, nil)
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
