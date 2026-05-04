package terraform

import (
	"fmt"
	"sort"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

type resourceReplacements map[string][]string

type resourceReplacementEntry struct {
	resource string
	paths    []string
}

// findResourceReplacementsWithPaths finds resources that Terraform will
// replace and for which Terraform reported replacement-causing paths.
//
// NOTE: "replacement" in Terraform terms means that a resource will be
// destroyed and recreated rather than updated in place. This can happen
// because an immutable attribute changed or because Terraform requires
// replacement for some other reason.
//
// This helper intentionally skips replacements with empty ReplacePaths.
// Terraform can plan a replacement without attribute paths, for example
// when a prior failed apply left a resource tainted in state. Those
// replacements are still logged via findAllResourceReplacements, but we
// do not synthesize fake paths for PlanComplete.ResourceReplacements.
// Downstream prebuild metrics and notifications therefore only receive
// replacements with Terraform-reported paths.
func findResourceReplacementsWithPaths(plan *tfjson.Plan) resourceReplacements {
	if plan == nil {
		return nil
	}

	// No changes, no problem!
	if len(plan.ResourceChanges) == 0 {
		return nil
	}

	replacements := make(resourceReplacements, len(plan.ResourceChanges))

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

// findAllResourceReplacements finds all non-coder resources Terraform will
// replace, including replacements without Terraform-reported paths. See
// findResourceReplacementsWithPaths for why pathless replacements are
// handled differently.
func findAllResourceReplacements(plan *tfjson.Plan) []resourceReplacementEntry {
	if plan == nil {
		return nil
	}

	replacements := make([]resourceReplacementEntry, 0, len(plan.ResourceChanges))

	for _, ch := range plan.ResourceChanges {
		if ch.Change == nil || !ch.Change.Actions.Replace() {
			continue
		}
		if strings.Index(ch.Type, "coder_") == 0 {
			continue
		}
		// Include all replacement actions, even when Terraform did not
		// provide ReplacePaths. Pathless replacements are rendered with a
		// fallback message in logResourceReplacements.
		replacements = append(replacements, resourceReplacementEntry{
			resource: ch.Address,
			paths:    replacePathsToStrings(ch.Change.ReplacePaths),
		})
	}

	return replacements
}

func replacePathsToStrings(in []any) []string {
	out := make([]string, 0, len(in))
	for _, path := range in {
		out = append(out, replacePathToString(path))
	}
	return out
}

// replacePathToString formats a Terraform ReplacePaths entry.  A
// ReplacePaths entry can be a scalar path element or a nested path
// like ["root_block_device", 0, "volume_size"].
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

func logResourceReplacements(replacements []resourceReplacementEntry, sink logSink) {
	if len(replacements) == 0 {
		return
	}

	// Sort a copy so the log output is deterministic without mutating
	// the caller's slice.
	logs := make([]resourceReplacementEntry, len(replacements))
	copy(logs, replacements)
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].resource < logs[j].resource
	})

	sink.ProvisionLog(proto.LogLevel_WARN, "Resource replacements:")
	for _, replacement := range logs {
		sink.ProvisionLog(proto.LogLevel_WARN, fmt.Sprintf("  -/+ %s (replace)", replacement.resource))

		if len(replacement.paths) == 0 {
			sink.ProvisionLog(proto.LogLevel_WARN, "      ~ replacement reason unavailable")
			continue
		}

		// Use a copy so we don't mutate the replacement entry.
		paths := make([]string, len(replacement.paths))
		copy(paths, replacement.paths)
		sort.Strings(paths)

		for _, path := range paths {
			sink.ProvisionLog(proto.LogLevel_WARN, fmt.Sprintf("      ~ %s (forces replacement)", path))
		}
	}
}
