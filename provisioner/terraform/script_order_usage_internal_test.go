package terraform

import (
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

	t.Run("DeclarationsAndRuleBlocks", func(t *testing.T) {
		t.Parallel()

		config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
			Resources: []*tfjson.ConfigResource{
				scriptOrderConfigDataSource("data.coder_script_order.root", 2),
				{
					Address: "data.coder_parameter.unrelated",
					Mode:    tfjson.DataResourceMode,
					Type:    "coder_parameter",
				},
				{
					Address: "coder_script_order.prototype",
					Mode:    tfjson.ManagedResourceMode,
					Type:    "coder_script_order",
				},
			},
			ModuleCalls: map[string]*tfjson.ModuleCall{
				"development": {
					ForEachExpression: &tfjson.Expression{},
					Module: &tfjson.ConfigModule{
						Resources: []*tfjson.ConfigResource{
							scriptOrderConfigDataSource("data.coder_script_order.module", 1),
						},
						ModuleCalls: map[string]*tfjson.ModuleCall{
							"nested": {
								CountExpression: &tfjson.Expression{},
								Module: &tfjson.ConfigModule{Resources: []*tfjson.ConfigResource{
									scriptOrderConfigDataSource("data.coder_script_order.nested", 3),
								}},
							},
						},
					},
				},
			},
		}}

		require.Equal(t, scriptOrderUsage{
			dataSourceCount: 3,
			ruleCount:       6,
		}, countScriptOrderUsage(config))
	})

	t.Run("RepeatedDataSourceDeclarationCountsOnce", func(t *testing.T) {
		t.Parallel()

		resource := scriptOrderConfigDataSource("data.coder_script_order.repeated", 2)
		resource.ForEachExpression = &tfjson.Expression{}
		config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
			Resources: []*tfjson.ConfigResource{resource},
		}}

		require.Equal(t, scriptOrderUsage{
			dataSourceCount: 1,
			ruleCount:       2,
		}, countScriptOrderUsage(config))
	})
}

func scriptOrderConfigDataSource(address string, ruleCount int) *tfjson.ConfigResource {
	rules := make([]map[string]*tfjson.Expression, ruleCount)
	for index := range rules {
		rules[index] = map[string]*tfjson.Expression{}
	}
	return &tfjson.ConfigResource{
		Address: address,
		Mode:    tfjson.DataResourceMode,
		Type:    "coder_script_order",
		Expressions: map[string]*tfjson.Expression{
			"rule": {ExpressionData: &tfjson.ExpressionData{NestedBlocks: rules}},
		},
	}
}
