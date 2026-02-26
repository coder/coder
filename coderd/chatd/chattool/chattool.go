package chattool

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
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
