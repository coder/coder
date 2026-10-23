package provisionerdserver

import (
	"encoding/json"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"
)

type scriptOrderUsage struct {
	dataSourceCount int32
	ruleCount       int32
}

// countScriptOrderUsage counts declarations in the supplied plan configuration,
// so count, for_each, and module instance counts do not inflate usage.
func countScriptOrderUsage(planJSON []byte) (scriptOrderUsage, error) {
	var usage scriptOrderUsage
	if len(planJSON) == 0 {
		return usage, nil
	}
	var plan struct {
		Config *tfjson.Config `json:"configuration"`
	}
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return usage, xerrors.Errorf("decode plan configuration: %w", err)
	}
	if plan.Config != nil {
		countScriptOrderUsageInModule(plan.Config.RootModule, &usage)
	}
	return usage, nil
}

func countScriptOrderUsageInModule(module *tfjson.ConfigModule, usage *scriptOrderUsage) {
	if module == nil {
		return
	}
	for _, r := range module.Resources {
		if r == nil || r.Mode != tfjson.DataResourceMode || r.Type != "coder_script_order" {
			continue
		}
		usage.dataSourceCount++
		// Terraform's configuration JSON omits dynamic blocks, so only literal
		// rule blocks contribute to this count.
		if rules := r.Expressions["rule"]; rules != nil {
			// #nosec G115 - Terraform rule counts are expected to fit in int32.
			usage.ruleCount += int32(len(rules.NestedBlocks))
		}
	}
	for _, call := range module.ModuleCalls {
		if call != nil {
			countScriptOrderUsageInModule(call.Module, usage)
		}
	}
}
