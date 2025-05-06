package terraform

import (
	"fmt"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

type resourceReplacements map[string][]string

// resourceReplacements finds all resources which would be replaced by the current plan, and the attribute paths which
// caused the replacement.
//
// NOTE: "replacement" in terraform terms means that a resource will have to be destroyed and replaced with a new resource
// since one of its immutable attributes was modified, which cannot be updated in-place.
func findResourceReplacements(plan *tfjson.Plan) resourceReplacements {
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

		// Replacements found, problem!
		for _, p := range ch.Change.ReplacePaths {
			var path string
			switch p := p.(type) {
			case []interface{}:
				segs := p
				list := make([]string, 0, len(segs))
				for _, s := range segs {
					list = append(list, fmt.Sprintf("%v", s))
				}
				path = strings.Join(list, ".")
			default:
				path = fmt.Sprintf("%v", p)
			}

			replacements[ch.Address] = append(replacements[ch.Address], path)
		}
	}

	if len(replacements) == 0 {
		return nil
	}

	return replacements
}
