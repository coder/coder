package chattool

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
)

func toolError(result chatprompt.ToolResultBlock, err error) chatprompt.ToolResultBlock {
	result.IsError = true
	result.Result = map[string]any{"error": err.Error()}
	return result
}

func toolResultBlockToAgentResponse(result chatprompt.ToolResultBlock) fantasy.ToolResponse {
	content := ""
	if result.IsError {
		if fields, ok := result.Result.(map[string]any); ok {
			if extracted, ok := fields["error"].(string); ok && strings.TrimSpace(extracted) != "" {
				content = extracted
			}
		}
		if content == "" {
			if raw, err := json.Marshal(result.Result); err == nil {
				content = strings.TrimSpace(string(raw))
			}
		}
	} else if payload, err := json.Marshal(result.Result); err == nil {
		content = string(payload)
	}

	metadata := ""
	if raw, err := json.Marshal(result); err == nil {
		metadata = string(raw)
	}

	return fantasy.ToolResponse{
		Content:  content,
		IsError:  result.IsError,
		Metadata: metadata,
	}
}

// parseOwnerID parses a UUID string into a uuid.UUID, returning
// an error if the string is empty or not a valid UUID.
func parseOwnerID(raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, xerrors.New("owner ID is empty")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("invalid owner ID %q: %w", raw, err)
	}
	return id, nil
}

func truncateRunes(value string, maxLen int) string {
	if maxLen <= 0 || value == "" {
		return ""
	}
	if utf8.RuneCountInString(value) <= maxLen {
		return value
	}

	runes := []rune(value)
	if maxLen > len(runes) {
		maxLen = len(runes)
	}
	return string(runes[:maxLen])
}
