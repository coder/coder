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

		// Replacing our resources, no problem!
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
