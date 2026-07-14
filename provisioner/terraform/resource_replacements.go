package terraform

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

type resourceReplacementPaths map[string][]string

type replacementLogEntry struct {
	resource string
	paths    []string
	values   map[string]replacementValues
}

type replacementValues struct {
	before replacementValue
	after  replacementValue
}

type replacementValue struct {
	text  string
	valid bool
}

// findResourceReplacementsWithPaths returns a map from Terraform resource
// address to replacement-causing paths for resources that Terraform will
// replace and for which Terraform reported ReplacePaths.
//
// "Replacement" in Terraform means that a resource will be destroyed
// and recreated rather than updated in place. This can happen because
// an immutable attribute changed or because Terraform requires
// replacement for some other reason.
//
// This helper intentionally skips replacements with empty ReplacePaths.
// Terraform can plan a replacement without attribute paths, for example
// when a prior failed apply left a resource tainted in state. Those
// replacements are still logged via findAllResourceReplacements, but we
// do not synthesize fake paths for PlanComplete.ResourceReplacements.
// Downstream prebuild metrics and notifications therefore only receive
// replacements with Terraform-reported paths.
func findResourceReplacementsWithPaths(plan *tfjson.Plan) resourceReplacementPaths {
	if plan == nil {
		return nil
	}

	// No changes, no problem!
	if len(plan.ResourceChanges) == 0 {
		return nil
	}

	replacements := make(resourceReplacementPaths, len(plan.ResourceChanges))

	for _, ch := range plan.ResourceChanges {
		// No change, no problem!
		if ch.Change == nil {
			continue
		}

		// No-op change, no problem!
		if ch.Change.Actions.NoOp() {
			continue
		}

		// No replacements, no problem!
		if len(ch.Change.ReplacePaths) == 0 {
			continue
		}

		// Replacing our resources: could be a problem - but we ignore since they're "virtual" resources. If any of these
		// resources' attributes are referenced by non-coder resources, those will show up as transitive changes there.
		// i.e. if the coder_agent.id attribute is used in docker_container.env
		//
		// Replacing our resources is not strictly a problem in and of itself.
		//
		// NOTE:
		// We may need to special-case coder_agent in the future. Currently, coder_agent is replaced on every build
		// because it only supports Create but not Update: https://github.com/coder/terraform-provider-coder/blob/5648efb/provider/agent.go#L28
		// When we can modify an agent's attributes, some of which may be immutable (like "arch") and some may not (like "env"),
		// then we'll have to handle this specifically.
		// This will only become relevant once we support multiple agents: https://github.com/coder/coder/issues/17388
		if strings.Index(ch.Type, "coder_") == 0 {
			continue
		}

		replacements[ch.Address] = append(
			replacements[ch.Address],
			replacePathsToStrings(ch.Change.ReplacePaths)...,
		)
	}

	if len(replacements) == 0 {
		return nil
	}

	return replacements
}

// findAllResourceReplacements returns all non-coder resources Terraform
// will replace in the form used by compact replacement logging, including
// replacements without Terraform-reported paths.
//
// See findResourceReplacementsWithPaths for why pathless replacements
// are handled differently.
func findAllResourceReplacements(plan *tfjson.Plan) []replacementLogEntry {
	if plan == nil {
		return nil
	}

	replacements := make([]replacementLogEntry, 0, len(plan.ResourceChanges))

	for _, ch := range plan.ResourceChanges {
		if !isNonCoderResourceReplacement(ch) {
			continue
		}
		paths, values := replacementPathsAndValues(ch.Change)
		replacements = append(replacements, replacementLogEntry{
			resource: ch.Address,
			paths:    paths,
			values:   values,
		})
	}

	return replacements
}

func hasResourceReplacement(plan *tfjson.Plan) bool {
	if plan == nil {
		return false
	}

	for _, ch := range plan.ResourceChanges {
		if isNonCoderResourceReplacement(ch) {
			return true
		}
	}
	return false
}

func isNonCoderResourceReplacement(ch *tfjson.ResourceChange) bool {
	if ch == nil || ch.Change == nil || !ch.Change.Actions.Replace() {
		return false
	}
	return strings.Index(ch.Type, "coder_") != 0
}

func replacePathsToStrings(in []any) []string {
	out := make([]string, 0, len(in))
	for _, path := range in {
		out = append(out, replacePathToString(path))
	}
	return out
}

// replacePathToString formats a Terraform ReplacePaths entry.
// Terraform represents each replacement path as a slice of string or
// numeric path segments, which we format as a dotted string:
//
//	["root_block_device", 0, "volume_size"] -> "root_block_device.0.volume_size"
//
// Terraform is expected to provide the documented shape. The fallback
// preserves best-effort logging if an unexpected shape appears.
func replacePathToString(path any) string {
	switch path := path.(type) {
	case []any:
		segments := make([]string, 0, len(path))
		for _, seg := range path {
			segments = append(segments, fmt.Sprintf("%v", seg))
		}
		return strings.Join(segments, ".")
	default:
		return fmt.Sprintf("%v", path)
	}
}

// replacementPathsAndValues returns formatted replacement paths and
// any printable before/after values for those paths. If Terraform
// does not provide ReplacePaths, both return values are nil, so
// logResourceReplacements will render a pathless fallback message.
func replacementPathsAndValues(change *tfjson.Change) ([]string, map[string]replacementValues) {
	if change == nil || len(change.ReplacePaths) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, len(change.ReplacePaths))
	values := make(map[string]replacementValues, len(change.ReplacePaths))

	for _, rawPath := range change.ReplacePaths {
		path := replacePathToString(rawPath)
		paths = append(paths, path)
		before := replacementValueAtPath(
			change.Before,
			change.BeforeSensitive,
			nil,
			rawPath,
		)
		after := replacementValueAtPath(
			change.After,
			change.AfterSensitive,
			change.AfterUnknown,
			rawPath,
		)
		if !before.valid && !after.valid {
			// Keep the path, but omit value details when neither side
			// has a printable value. The logger will still render the
			// path-only replacement reason.
			continue
		}

		values[path] = replacementValues{
			before: before,
			after:  after,
		}
	}

	if len(values) == 0 {
		return paths, nil
	}
	return paths, values
}

func replacementValueAtPath(resourceValue, sensitive, unknown, path any) replacementValue {
	r := replacementPathResolver{}

	if r.isMarkedAtPath(sensitive, path) {
		return replacementValue{text: "(sensitive value)", valid: true}
	}
	if r.isMarkedAtPath(unknown, path) {
		return replacementValue{text: "(known after apply)", valid: true}
	}

	value, ok := r.valueAtPath(resourceValue, path)
	if !ok {
		// Terraform can omit one side of a replacement value, for
		// example when a value is created, deleted, or unavailable in
		// the plan.
		return replacementValue{}
	}

	// JSON formatting keeps arbitrary Terraform values unambiguous in
	// logs: strings stay quoted, null stays null, and lists/maps do
	// not use Go syntax.
	formatted, err := json.Marshal(value)
	if err != nil {
		return replacementValue{}
	}
	return replacementValue{text: string(formatted), valid: true}
}

// replacementPathResolver groups helpers for traversing Terraform
// JSON value and marker trees by replacement path.
type replacementPathResolver struct{}

func (r replacementPathResolver) valueAtPath(valueTree, path any) (any, bool) {
	current := valueTree
	for _, segment := range r.pathSegments(path) {
		var ok bool
		current, ok = r.childAtPathSegment(current, segment)
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func (r replacementPathResolver) isMarkedAtPath(markerTree, path any) bool {
	current := markerTree
	for _, segment := range r.pathSegments(path) {
		if isMarked, ok := current.(bool); ok {
			return isMarked
		}

		next, ok := r.childAtPathSegment(current, segment)
		if !ok {
			return false
		}
		current = next
	}

	// A parent path is sensitive if any descendant is sensitive. Terraform
	// can report both "subject" and "subject.0.common_name" as replacement
	// paths, while only marking the nested value sensitive.
	return r.containsMarkedValue(current)
}

func (r replacementPathResolver) containsMarkedValue(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case map[string]any:
		for _, child := range value {
			if r.containsMarkedValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if r.containsMarkedValue(child) {
				return true
			}
		}
	}
	return false
}

func (replacementPathResolver) pathSegments(path any) []any {
	switch path := path.(type) {
	case []any:
		return path
	default:
		return []any{path}
	}
}

func (r replacementPathResolver) childAtPathSegment(node, segment any) (any, bool) {
	switch node := node.(type) {
	case map[string]any:
		key, ok := segment.(string)
		if !ok {
			return nil, false
		}
		child, ok := node[key]
		return child, ok
	case []any:
		index, ok := r.pathIndex(segment)
		if !ok || index < 0 || index >= len(node) {
			return nil, false
		}
		return node[index], true
	default:
		return nil, false
	}
}

// pathIndex accepts both JSON-decoded (float64) numeric path segments
// and hand-built integer segments that may be used in tests.
func (replacementPathResolver) pathIndex(segment any) (int, bool) {
	switch segment := segment.(type) {
	case int:
		return segment, true
	case float64:
		index := int(segment)
		return index, float64(index) == segment
	default:
		return 0, false
	}
}

func logResourceReplacements(replacements []replacementLogEntry, sink logSink) {
	if len(replacements) == 0 {
		return
	}

	// Sort a copy so the log output is deterministic without mutating
	// the caller's slice.
	logs := make([]replacementLogEntry, len(replacements))
	copy(logs, replacements)
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].resource < logs[j].resource
	})

	sink.ProvisionLog(proto.LogLevel_WARN, "Resource replacements:")
	for _, replacement := range logs {
		sink.ProvisionLog(
			proto.LogLevel_WARN, fmt.Sprintf("  -/+ %s (replace)", replacement.resource))

		if len(replacement.paths) == 0 {
			sink.ProvisionLog(
				proto.LogLevel_WARN, "      ~ replacement reason unavailable")
			continue
		}

		// Use a copy so we don't mutate the replacement entry.
		paths := make([]string, len(replacement.paths))
		copy(paths, replacement.paths)
		sort.Strings(paths)

		for _, path := range paths {
			vals, ok := replacement.values[path]
			if ok {
				sink.ProvisionLog(
					proto.LogLevel_WARN, fmt.Sprintf("      ~ %s: %s -> %s (forces replacement)",
						path,
						formatReplacementValue(vals.before),
						formatReplacementValue(vals.after),
					),
				)
				continue
			}

			sink.ProvisionLog(
				proto.LogLevel_WARN, fmt.Sprintf("      ~ %s (forces replacement)", path),
			)
		}
	}
}

func formatReplacementValue(value replacementValue) string {
	if !value.valid {
		return "(unavailable)"
	}
	return value.text
}
