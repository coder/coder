package terraform

import tfjson "github.com/hashicorp/terraform-json"

type scriptOrderUsage struct {
	dataSourceCount int32
	ruleCount       int32
}

// countScriptOrderUsage counts configuration declarations, not expanded
// instances, so count, for_each, and module instance counts do not inflate usage.
func countScriptOrderUsage(config *tfjson.Config) scriptOrderUsage {
	var usage scriptOrderUsage
	if config != nil {
		countScriptOrderUsageInModule(config.RootModule, &usage)
	}
	return usage
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
