package automations

import (
	"encoding/json"

	"github.com/tidwall/gjson"
)

// MatchFilter evaluates a gjson-based filter against a JSON payload.
// If filter is nil or empty, the match succeeds (everything passes).
// Each key in the filter is a gjson path; each value is the expected
// result. All entries must match for the filter to pass.
func MatchFilter(payload string, filter json.RawMessage) bool {
	if len(filter) == 0 || string(filter) == "null" {
		return true
	}

	var conditions map[string]any
	if err := json.Unmarshal(filter, &conditions); err != nil {
		return false
	}

	for path, expected := range conditions {
		result := gjson.Get(payload, path)
		if !result.Exists() {
			return false
		}
		if result.Value() != expected {
			return false
		}
	}
	return true
}
