package terraform

import (
	"encoding/json"
	"os"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestCountScriptOrderUsage(t *testing.T) {
	t.Parallel()

	t.Run("NoConfiguration", func(t *testing.T) {
		t.Parallel()

		require.Empty(t, countScriptOrderUsage(nil))
		require.Empty(t, countScriptOrderUsage(&tfjson.Config{}))
	})

	declaration := func(ruleCount int) *tfjson.ConfigResource {
		return &tfjson.ConfigResource{
			Mode: tfjson.DataResourceMode,
			Type: "coder_script_order",
			Expressions: map[string]*tfjson.Expression{
				"rule": {ExpressionData: &tfjson.ExpressionData{
					NestedBlocks: make([]map[string]*tfjson.Expression, ruleCount),
				}},
			},
		}
	}

	t.Run("ConfigurationDeclarationsNotInstances", func(t *testing.T) {
		t.Parallel()

		// Three declarations (root, child, nested) contain 2 + 3 + 1 rules.
		// Resource and module repetition must not multiply either total.
		rootOrder := declaration(2)
		rootOrder.ForEachExpression = &tfjson.Expression{ExpressionData: &tfjson.ExpressionData{
			ConstantValue: map[string]string{"first": "", "second": ""},
		}}
		childOrder := declaration(3)
		childOrder.CountExpression = &tfjson.Expression{ExpressionData: &tfjson.ExpressionData{
			ConstantValue: 3, // Three resource instances per module instance.
		}}
		config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
			Resources: []*tfjson.ConfigResource{
				rootOrder,
				nil,
				{Mode: tfjson.ManagedResourceMode, Type: "coder_script_order"},
				{Mode: tfjson.DataResourceMode, Type: "coder_parameter"},
			},
			ModuleCalls: map[string]*tfjson.ModuleCall{
				"repeated": {
					ForEachExpression: &tfjson.Expression{ExpressionData: &tfjson.ExpressionData{
						ConstantValue: map[string]string{"east": "", "west": ""},
					}},
					Module: &tfjson.ConfigModule{
						Resources: []*tfjson.ConfigResource{childOrder},
						ModuleCalls: map[string]*tfjson.ModuleCall{
							"nested": {
								CountExpression: &tfjson.Expression{ExpressionData: &tfjson.ExpressionData{
									ConstantValue: 2,
								}},
								Module: &tfjson.ConfigModule{
									Resources: []*tfjson.ConfigResource{declaration(1)},
								},
							},
						},
					},
				},
				"nil":   nil,
				"empty": {},
			},
		}}
		require.Equal(t, scriptOrderUsage{dataSourceCount: 3, ruleCount: 6}, countScriptOrderUsage(config))
	})

	t.Run("DeclarationWithZeroInstances", func(t *testing.T) {
		t.Parallel()

		order := declaration(2)
		order.CountExpression = &tfjson.Expression{ExpressionData: &tfjson.ExpressionData{ConstantValue: 0}}
		config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
			Resources: []*tfjson.ConfigResource{order},
		}}
		require.Equal(t, scriptOrderUsage{dataSourceCount: 1, ruleCount: 2}, countScriptOrderUsage(config))
	})

	t.Run("CapturedTerraformPlan", func(t *testing.T) {
		t.Parallel()

		data, err := os.ReadFile("testdata/script-order-plan.json")
		require.NoError(t, err)
		var plan tfjson.Plan
		require.NoError(t, json.Unmarshal(data, &plan))
		require.Equal(t, scriptOrderUsage{dataSourceCount: 1, ruleCount: 1}, countScriptOrderUsage(plan.Config))
	})
}
