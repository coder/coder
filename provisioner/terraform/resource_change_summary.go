package terraform

import (
	"fmt"
	"sort"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

// logResourceChangeSummary writes a compact summary of resource
// drift and planned changes to the build log. This resolves
// #16999: terraform plan -json only produces opaque messages like
// "Drift detected (update)" and "Plan to replace" with no detail
// on which fields changed. This function extracts the field-level
// diff from the plan's Before/After values.
//
// Example output for a build that would destroy a user's disk:
//
//	Drift detected:
//	  ~ azurerm_network_interface.workspace (update)
//	      ~ mac_address: "00:0D:3A:1B:2C:3D" → "00:0D:3A:4E:5F:6G"
//	Resource changes:
//	  -/+ azurerm_managed_disk.workspace[0] (replace)
//	      ~ disk_size_gb: 128 → 256 (forces replacement)
//	  -/+ azurerm_linux_virtual_machine.workspace (replace)
//	  - docker_volume.home_volume (destroy)
func logResourceChangeSummary(plan *tfjson.Plan, logr logSink) {
	if plan == nil {
		return
	}

	// Log drift first: infrastructure that changed outside of
	// Coder since the last build. Often the root cause of
	// unexpected replacements or destroys.
	if lines := summarizeResourceChanges(plan.ResourceDrift); len(lines) > 0 {
		logr.ProvisionLog(proto.LogLevel_WARN, "Drift detected:")
		for _, l := range lines {
			logr.ProvisionLog(l.level, l.text)
		}
	}

	// Then log planned changes.
	if lines := summarizeResourceChanges(plan.ResourceChanges); len(lines) > 0 {
		logr.ProvisionLog(proto.LogLevel_INFO, "Resource changes:")
		for _, l := range lines {
			logr.ProvisionLog(l.level, l.text)
		}
	}
}

type changeLine struct {
	level proto.LogLevel
	text  string
}

func summarizeResourceChanges(changes []*tfjson.ResourceChange) []changeLine {
	if len(changes) == 0 {
		return nil
	}

	sorted := make([]*tfjson.ResourceChange, len(changes))
	copy(sorted, changes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Address < sorted[j].Address
	})

	var lines []changeLine
	for _, ch := range sorted {
		if ch.Change == nil {
			continue
		}
		if ch.Change.Actions.NoOp() || ch.Change.Actions.Read() {
			continue
		}

		sym, verb := actionLabel(ch.Change.Actions)
		level := proto.LogLevel_INFO
		if ch.Change.Actions.Delete() || ch.Change.Actions.Replace() {
			level = proto.LogLevel_WARN
		}
		lines = append(lines, changeLine{level,
			fmt.Sprintf("  %s %s (%s)", sym, ch.Address, verb)})

		if ch.Change.Actions.Update() || ch.Change.Actions.Replace() {
			lines = append(lines, diffFields(ch.Change)...)
		}
	}
	return lines
}

func actionLabel(a tfjson.Actions) (string, string) {
	switch {
	case a.Create():
		return "+", "create"
	case a.Delete():
		return "-", "destroy"
	case a.Update():
		return "~", "update"
	case a.Replace():
		return "-/+", "replace"
	default:
		return "?", "unknown"
	}
}

// diffFields compares Before/After maps and returns one line per
// changed scalar field. Complex nested values are skipped to keep
// output compact.
func diffFields(c *tfjson.Change) []changeLine {
	bMap, _ := c.Before.(map[string]interface{})
	aMap, _ := c.After.(map[string]interface{})
	if bMap == nil && aMap == nil {
		return nil
	}

	replaceSet := make(map[string]bool)
	for _, p := range c.ReplacePaths {
		switch v := p.(type) {
		case string:
			replaceSet[v] = true
		case []interface{}:
			if len(v) > 0 {
				if s, ok := v[0].(string); ok {
					replaceSet[s] = true
				}
			}
		}
	}

	keys := make(map[string]struct{})
	for k := range bMap {
		keys[k] = struct{}{}
	}
	for k := range aMap {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	var lines []changeLine
	for _, key := range sorted {
		bVal, bOk := bMap[key]
		aVal, aOk := aMap[key]

		if isComplex(bVal) || isComplex(aVal) {
			continue
		}

		bStr := fmtVal(bVal)
		aStr := fmtVal(aVal)
		if bOk && aOk && bStr == aStr {
			continue
		}

		// Skip zero/null → null or null → zero noise. Providers
		// often report empty strings, zeroes, and false as drifted
		// when the actual value is semantically unchanged.
		if isZeroish(bVal) && isZeroish(aVal) {
			continue
		}

		level := proto.LogLevel_INFO
		suffix := ""
		if replaceSet[key] {
			suffix = " (forces replacement)"
			level = proto.LogLevel_WARN
		}

		switch {
		case !bOk:
			lines = append(lines, changeLine{level,
				fmt.Sprintf("      + %s: %s%s", key, aStr, suffix)})
		case !aOk:
			lines = append(lines, changeLine{level,
				fmt.Sprintf("      - %s: %s%s", key, bStr, suffix)})
		default:
			lines = append(lines, changeLine{level,
				fmt.Sprintf("      ~ %s: %s → %s%s", key, bStr, aStr, suffix)})
		}
	}
	return lines
}

func fmtVal(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		if len(val) > 60 {
			return fmt.Sprintf("%q...", val[:57])
		}
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func isComplex(v interface{}) bool {
	switch v.(type) {
	case map[string]interface{}, []interface{}:
		return true
	default:
		return false
	}
}

func isZeroish(v interface{}) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return val == ""
	case bool:
		return !val
	case float64:
		return val == 0
	default:
		return false
	}
}
