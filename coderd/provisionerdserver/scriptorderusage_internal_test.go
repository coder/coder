package provisionerdserver

import (
	"encoding/json"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestCountScriptOrderUsage(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		plan    string
		wantErr bool
	}{
		{name: "MissingPlan"},
		{name: "NullPlan", plan: "null"},
		{name: "MissingConfiguration", plan: "{}"},
		{name: "EmptyConfiguration", plan: `{"configuration":{}}`},
		{name: "InvalidJSON", plan: "{", wantErr: true},
		{name: "InvalidConfiguration", plan: `{"configuration":[]}`, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			usage, err := countScriptOrderUsage([]byte(tt.plan))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, usage)
		})
	}

	checkUsage := func(t *testing.T, config *tfjson.Config, want scriptOrderUsage) {
		t.Helper()
		// The provisioner marshals a decoded plan before forwarding it to coderd.
		planJSON, err := json.Marshal(&tfjson.Plan{Config: config})
		require.NoError(t, err)
		usage, err := countScriptOrderUsage(planJSON)
		require.NoError(t, err)
		require.Equal(t, want, usage)
	}

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
		checkUsage(t, config, scriptOrderUsage{dataSourceCount: 3, ruleCount: 6})
	})

	t.Run("DeclarationWithZeroInstances", func(t *testing.T) {
		t.Parallel()

		order := declaration(2)
		order.CountExpression = &tfjson.Expression{ExpressionData: &tfjson.ExpressionData{ConstantValue: 0}}
		config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
			Resources: []*tfjson.ConfigResource{order},
		}}
		checkUsage(t, config, scriptOrderUsage{dataSourceCount: 1, ruleCount: 2})
	})

	t.Run("NoLiteralRules", func(t *testing.T) {
		t.Parallel()

		config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
			Resources: []*tfjson.ConfigResource{declaration(0)},
		}}
		checkUsage(t, config, scriptOrderUsage{dataSourceCount: 1})
	})
}
