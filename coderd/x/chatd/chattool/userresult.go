package chattool

import (
	"encoding/json"

	"charm.land/fantasy"
	"golang.org/x/xerrors"
)

// WithUserResult attaches a user-visible result without adding it to the
// model's tool response. The persistence layer stores both projections.
func WithUserResult(response fantasy.ToolResponse, result any) fantasy.ToolResponse {
	return fantasy.WithResponseMetadata(response, struct {
		UserResult any `json:"user_result"`
	}{UserResult: result})
}

// UserResultFromMetadata reads an optional user-visible tool projection.
func UserResultFromMetadata(metadata string) (json.RawMessage, error) {
	if metadata == "" {
		return nil, nil
	}
	var decoded struct {
		UserResult json.RawMessage `json:"user_result"`
	}
	if err := json.Unmarshal([]byte(metadata), &decoded); err != nil {
		return nil, xerrors.Errorf("decode user tool result: %w", err)
	}
	if string(decoded.UserResult) == "null" {
		return nil, nil
	}
	return decoded.UserResult, nil
}
