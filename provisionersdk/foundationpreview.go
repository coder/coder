package provisionersdk

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

// FoundationPreviewAdvisory applies even when no references or changes are found.
const FoundationPreviewAdvisory = "Advisory only. References may be incomplete or dynamic. No detected references is not a compatibility or safety guarantee."

// FoundationPlanPreview extracts only root output names and change metadata.
// Invalid or absent plans are unavailable, not evidence of no changes. Neither
// decoding errors nor output values are returned, since both can contain secrets.
func FoundationPlanPreview(plan []byte) codersdk.FoundationPlanPreview {
	preview := codersdk.FoundationPlanPreview{
		OutputChanges:      []codersdk.FoundationOutputChange{},
		ConsumerReferences: []codersdk.FoundationConsumerReference{},
		Advisory:           FoundationPreviewAdvisory,
	}
	var input struct {
		FormatVersion string `json:"format_version"`
		OutputChanges map[string]struct {
			Actions         []string        `json:"actions"`
			AfterUnknown    json.RawMessage `json:"after_unknown"`
			BeforeSensitive json.RawMessage `json:"before_sensitive"`
			AfterSensitive  json.RawMessage `json:"after_sensitive"`
		} `json:"output_changes"`
	}
	// Bound the work independently of the transport's message-size limit.
	if len(plan) == 0 || len(plan) > 16<<20 || json.Unmarshal(plan, &input) != nil || !strings.HasPrefix(input.FormatVersion, "1.") {
		return preview
	}
	changes := []codersdk.FoundationOutputChange{}
	for name, change := range input.OutputChanges {
		if !validFoundationOutputName(name) || len(change.Actions) == 0 {
			return preview
		}
		for _, action := range change.Actions {
			switch action {
			case "no-op", "create", "read", "update", "delete", "forget":
			default:
				return preview
			}
		}
		unknown, ok := foundationPreviewMarker(change.AfterUnknown)
		if !ok {
			return preview
		}
		beforeSensitive, ok := foundationPreviewMarker(change.BeforeSensitive)
		if !ok {
			return preview
		}
		afterSensitive, ok := foundationPreviewMarker(change.AfterSensitive)
		if !ok {
			return preview
		}
		if len(change.Actions) == 1 && change.Actions[0] == "no-op" {
			continue
		}
		changes = append(changes, codersdk.FoundationOutputChange{
			Name: name, Actions: change.Actions, Unknown: unknown, Sensitive: beforeSensitive || afterSensitive,
		})
	}
	slices.SortFunc(changes, func(a, b codersdk.FoundationOutputChange) int {
		return cmp.Compare(a.Name, b.Name)
	})
	preview.OutputChanges = changes
	preview.Available = true
	return preview
}

func validFoundationOutputName(name string) bool {
	if name == "" || len(name) > 256 {
		return false
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && ((r >= '0' && r <= '9') || r == '-')) {
			continue
		}
		return false
	}
	return true
}

// Terraform represents nested unknown and sensitive markers as trees of booleans.
func foundationPreviewMarker(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 {
		return false, true
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false, false
	}
	var walk func(any) (bool, bool)
	walk = func(value any) (bool, bool) {
		switch value := value.(type) {
		case nil:
			return false, true
		case bool:
			return value, true
		case map[string]any:
			marked := false
			for _, child := range value {
				yes, ok := walk(child)
				if !ok {
					return false, false
				}
				marked = marked || yes
			}
			return marked, true
		case []any:
			marked := false
			for _, child := range value {
				yes, ok := walk(child)
				if !ok {
					return false, false
				}
				marked = marked || yes
			}
			return marked, true
		default:
			return false, false
		}
	}
	return walk(value)
}
