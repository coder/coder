package chattool

import (
	"encoding/json"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
)

// toolResponse builds a fantasy.ToolResponse from a JSON-serializable
// result map. The map constraint ensures all tool results serialize
// to JSON objects so the frontend can safely parse them.
func toolResponse(result map[string]any) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextResponse("{}")
	}
	return fantasy.NewTextResponse(string(data))
}

// buildToolResponse marshals a buildErrorResult into a tool response.
// Separate from toolResponse to keep the map[string]any constraint
// on the general helper while allowing typed error structs.
func buildToolResponse(r buildErrorResult) fantasy.ToolResponse {
	data, err := json.Marshal(r)
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

// buildErrorResult is a structured error response that preserves
// the build ID alongside the error message. This lets the frontend
// keep showing build logs when a build fails instead of losing
// them on the error transition.
type buildErrorResult struct {
	Error   string `json:"error"`
	BuildID string `json:"build_id,omitempty"`
}

func newBuildError(msg string, buildID uuid.UUID) buildErrorResult {
	r := buildErrorResult{Error: msg}
	if buildID != uuid.Nil {
		r.BuildID = buildID.String()
	}
	return r
}

// setBuildID adds the build_id field to a tool response map when
// the build ID is known (non-zero).
func setBuildID(result map[string]any, buildID uuid.UUID) {
	if buildID != uuid.Nil {
		result["build_id"] = buildID.String()
	}
}

// setNoBuild marks the response with no_build: true when no build
// was triggered. The frontend uses this flag to suppress the
// build-log section for already-running workspaces.
func setNoBuild(result map[string]any, buildID uuid.UUID) {
	if buildID == uuid.Nil {
		result["no_build"] = true
	}
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
