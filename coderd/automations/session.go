package automations

import (
	"github.com/tidwall/gjson"
)

// ResolveLabels extracts label values from a JSON payload using gjson
// paths. Each key in labelPaths maps a label name to a gjson path
// expression. If a path doesn't match, that label is omitted.
func ResolveLabels(payload string, labelPaths map[string]string) map[string]string {
	if len(labelPaths) == 0 {
		return nil
	}

	labels := make(map[string]string, len(labelPaths))
	for labelKey, gjsonPath := range labelPaths {
		result := gjson.Get(payload, gjsonPath)
		if result.Exists() {
			labels[labelKey] = result.String()
		}
	}
	return labels
}
