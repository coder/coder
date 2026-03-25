package chattool

import (
	"encoding/json"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
)

// toolResponse builds a fantasy.ToolResponse from a JSON-serializable
// result payload.
func toolResponse(result map[string]any) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextResponse("{}")
	}
	return fantasy.NewTextResponse(string(data))
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

// isTemplateAllowed checks whether a template ID is permitted by the
// configured allowlist. A nil function or an empty allowlist means
// all templates are allowed.
func isTemplateAllowed(getAllowlist func() map[uuid.UUID]bool, id uuid.UUID) bool {
	if getAllowlist == nil {
		return true
	}
	allowlist := getAllowlist()
	if len(allowlist) == 0 {
		return true
	}
	return allowlist[id]
}
