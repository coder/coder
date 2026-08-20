package terraform

import tfjson "github.com/hashicorp/terraform-json"

type scriptOrderUsage struct {
	dataSourceCount int32
	ruleCount       int32
}

// countScriptOrderUsage counts configuration declarations rather than planned
// instances, so count and for_each do not inflate telemetry.
func countScriptOrderUsage(config *tfjson.Config) scriptOrderUsage {
	if config == nil || config.RootModule == nil {
		return scriptOrderUsage{}
	}

	var usage scriptOrderUsage
	countScriptOrderUsageInModule(config.RootModule, &usage)
	return usage
}

func countScriptOrderUsageInModule(module *tfjson.ConfigModule, usage *scriptOrderUsage) {
	if module == nil {
		return
	}

	for _, resource := range module.Resources {
		if resource == nil || resource.Mode != tfjson.DataResourceMode || resource.Type != "coder_script_order" {
			continue
		}
		usage.dataSourceCount++
		if rules := resource.Expressions["rule"]; rules != nil {
			for range rules.NestedBlocks {
				usage.ruleCount++
			}
		}
	}

	for _, call := range module.ModuleCalls {
		if call != nil {
			countScriptOrderUsageInModule(call.Module, usage)
		}
	}
}
