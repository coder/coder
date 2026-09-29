package scriptorder

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestCollectScriptOrderRuleDeclarations(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("coder_script.a", "a"),
			scriptOrderManagedCoderScript("coder_script.b", "b"),
			scriptOrderManagedCoderScript("coder_script.c", "c"),
			dataCoderScriptOrder(
				"data.coder_script_order.second",
				"second",
				scriptOrderRuleAttributes{
					Run:      []string{"coder_script.c"},
					After:    []string{"module.bootstrap"},
					Requires: "completion",
					Phase:    "start",
				},
			),
			dataCoderScriptOrder(
				"data.coder_script_order.first",
				"first",
				scriptOrderRuleAttributes{
					Run:      []string{"coder_script.b", "coder_script.c"},
					After:    []string{"coder_script.a", "module.bootstrap"},
					Requires: "success",
				},
			),
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.bootstrap",
			Resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript("module.bootstrap.coder_script.install", "install"),
			},
		}},
	}

	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{module}, rootScriptOrderConfigWithModuleCalls("bootstrap"),
	)
	require.NoError(t, err)
	require.Equal(t, []scriptOrderRuleDeclaration{
		{
			dataSourceAddress: "data.coder_script_order.first",
			ruleIndex:         0,
			declaredPhase:     "",
			requirement:       ScriptOrderRequirementSuccess,
			run: []resolvedScriptOrderSelector{{
				field:     "run",
				raw:       "coder_script.b",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.b"},
			}, {
				field:     "run",
				raw:       "coder_script.c",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.c"},
			}},
			after: []resolvedScriptOrderSelector{{
				field:     "after",
				raw:       "coder_script.a",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.a"},
			}, {
				field:     "after",
				raw:       "module.bootstrap",
				kind:      scriptOrderSelectorModule,
				addresses: []string{"module.bootstrap.coder_script.install"},
			}},
		},
		{
			dataSourceAddress: "data.coder_script_order.second",
			ruleIndex:         0,
			declaredPhase:     ScriptOrderPhaseStart,
			requirement:       ScriptOrderRequirementCompletion,
			run: []resolvedScriptOrderSelector{{
				field:     "run",
				raw:       "coder_script.c",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.c"},
			}},
			after: []resolvedScriptOrderSelector{{
				field:     "after",
				raw:       "module.bootstrap",
				kind:      scriptOrderSelectorModule,
				addresses: []string{"module.bootstrap.coder_script.install"},
			}},
		},
	}, declarations)
}

func TestCollectScriptOrderRuleDeclarationsPreservesRuleOrderAndIndexes(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.a", "a"),
		scriptOrderManagedCoderScript("coder_script.b", "b"),
		scriptOrderManagedCoderScript("coder_script.c", "c"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.b"},
				After: []string{"coder_script.a"},
				Phase: "stop",
			},
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.c"},
				After: []string{"coder_script.b"},
			},
		),
	}}

	// Rule indexes identify the originating Terraform rule in
	// diagnostics, so declarations must retain source order.
	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{module}, nil,
	)
	require.NoError(t, err)
	require.Len(t, declarations, 2)
	require.Equal(t, 0, declarations[0].ruleIndex)
	require.Equal(t, ScriptOrderPhaseStop, declarations[0].declaredPhase)
	require.Equal(t, "coder_script.b", declarations[0].run[0].raw)
	require.Equal(t, 1, declarations[1].ruleIndex)
	require.Equal(t, ScriptOrderRequirementSuccess, declarations[1].requirement)
	require.Equal(t, "coder_script.c", declarations[1].run[0].raw)
}

func TestCollectScriptOrderRuleDeclarationsRelativeToDeclaringModule(t *testing.T) {
	t.Parallel()

	declaringModule := &tfjson.StateModule{
		Address: "module.development",
		Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("module.development.coder_script.configure", "configure"),
			dataCoderScriptOrder(
				"module.development.data.coder_script_order.order",
				"order",
				scriptOrderRuleAttributes{
					Run:   []string{"coder_script.configure"},
					After: []string{"module.bootstrap"},
				},
			),
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.development.module.bootstrap",
			Resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript(
					"module.development.module.bootstrap.coder_script.install", "install",
				),
			},
		}},
	}

	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{{ChildModules: []*tfjson.StateModule{declaringModule}}},
		nestedScriptOrderConfigWithModuleCalls("development", "bootstrap"),
	)
	require.NoError(t, err)
	require.Len(t, declarations, 1)
	require.Equal(t,
		[]string{"module.development.coder_script.configure"},
		declarations[0].run[0].addresses,
	)
	require.Equal(t,
		[]string{"module.development.module.bootstrap.coder_script.install"},
		declarations[0].after[0].addresses,
	)
}

func TestCollectScriptOrderRuleDeclarationsAllowsEmptyScripts(t *testing.T) {
	t.Parallel()

	// Only prerequisite has a concrete instance. The plan config below
	// declares optional_count and optional_each with zero instances.
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.optional_count"},
				After: []string{"coder_script.optional_each"},
			},
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.optional_count"},
				After: []string{"coder_script.prerequisite"},
			},
		),
	}}
	config := rootScriptOrderConfigWithScripts(
		scriptOrderConfigCountCoderScript("optional_count"),
		scriptOrderConfigForEachCoderScript("optional_each"),
	)

	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{module}, config,
	)
	require.NoError(t, err)
	require.Len(t, declarations, 2)
	require.Nil(t, declarations[0].run[0].addresses)
	require.Nil(t, declarations[0].after[0].addresses)
	require.Nil(t, declarations[1].run[0].addresses)
	require.Equal(t,
		[]string{"coder_script.prerequisite"},
		declarations[1].after[0].addresses,
	)
}

func TestCollectScriptOrderRuleDeclarationsAllowsEmptyModules(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("coder_script.configure", "configure"),
			dataCoderScriptOrder(
				"data.coder_script_order.order",
				"order",
				scriptOrderRuleAttributes{
					Run:   []string{"module.disabled"},
					After: []string{"module.without_scripts"},
				},
				scriptOrderRuleAttributes{
					Run:   []string{"coder_script.configure"},
					After: []string{"module.disabled"},
				},
			),
		},
		ChildModules: []*tfjson.StateModule{{Address: "module.without_scripts"}},
	}

	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("disabled", "without_scripts"),
	)
	require.NoError(t, err)
	require.Len(t, declarations, 2)
	require.Nil(t, declarations[0].run[0].addresses)
	require.Nil(t, declarations[0].after[0].addresses)
	require.Equal(t,
		[]string{"coder_script.configure"}, declarations[1].run[0].addresses,
	)
	require.Nil(t, declarations[1].after[0].addresses)
}

func TestCollectScriptOrderRuleDeclarationsWithoutScriptOrderDataSources(t *testing.T) {
	t.Parallel()

	// Templates that do not opt into script ordering must remain unaffected.
	declarations, err := collectScriptOrderRuleDeclarations(
		[]*tfjson.StateModule{{Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("coder_script.configure", "configure"),
			{
				Address: "data.coder_workspace.me",
				Mode:    tfjson.DataResourceMode,
				Type:    "coder_workspace",
				Name:    "me",
			},
			{
				Address: "coder_script_order.order",
				Mode:    tfjson.ManagedResourceMode,
				Type:    "coder_script_order",
				Name:    "order",
			},
		}}},
		nil,
	)
	require.NoError(t, err)
	require.Nil(t, declarations)
}

func TestCollectScriptOrderRuleDeclarationsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rule       scriptOrderRuleAttributes
		resources  []*tfjson.StateResource
		modules    []*tfjson.StateModule
		planConfig *tfjson.Config
		contains   []string
	}{
		{
			name: "EmptyRunList",
			rule: scriptOrderRuleAttributes{After: []string{"coder_script.a"}},
			contains: []string{
				"rule 0", "run must contain at least one selector",
			},
		},
		{
			name: "EmptyAfterList",
			rule: scriptOrderRuleAttributes{Run: []string{"coder_script.a"}},
			contains: []string{
				"rule 0", "after must contain at least one selector",
			},
		},
		{
			name: "InvalidRequirement",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"}, Requires: "started",
			},
			contains: []string{"rule 0", "requires", "success", "completion", "started"},
		},
		{
			name: "InvalidPhase",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"}, Phase: "startup",
			},
			contains: []string{"rule 0", "phase", "start", "stop", "startup"},
		},
		{
			name: "MalformedSelector",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b["}, After: []string{"coder_script.a"},
			},
			contains: []string{"rule 0", "run selector", "coder_script.b["},
		},
		{
			name: "UndeclaredScriptWithNoInstances",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.missing"}, After: []string{"coder_script.a"},
			},
			planConfig: rootScriptOrderConfigWithScripts(
				scriptOrderConfigCoderScript("other"),
			),
			contains: []string{
				"rule 0", "run selector", "coder_script.missing", "does not name a declared coder_script resource",
			},
		},
		{
			name: "EmptyScriptSelectorRequiresPlanConfiguration",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.missing"}, After: []string{"coder_script.a"},
			},
			planConfig: nil,
			contains: []string{
				"rule 0", "resolve run selector", "coder_script.missing",
				"Terraform plan configuration is unavailable",
			},
		},
		{
			name: "DeclaredCountResourceWithoutEvaluatedInstance",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.a[0]"}, After: []string{"coder_script.b"},
			},
			resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript("coder_script.b", "b"),
			},
			planConfig: rootScriptOrderConfigWithScripts(
				scriptOrderConfigCountCoderScript("a"),
			),
			contains: []string{
				"rule 0", "run selector", "coder_script.a[0]", "expanded to no coder_script resources",
			},
		},
		{
			name: "DeclaredForEachResourceWithoutEvaluatedInstance",
			rule: scriptOrderRuleAttributes{
				Run: []string{`coder_script.a["api"]`}, After: []string{"coder_script.b"},
			},
			resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript("coder_script.b", "b"),
			},
			planConfig: rootScriptOrderConfigWithScripts(
				scriptOrderConfigForEachCoderScript("a"),
			),
			contains: []string{
				"rule 0", "run selector", "coder_script.a", "api",
				"expanded to no coder_script resources",
			},
		},
		{
			name: "UndeclaredModule",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"module.missing"},
			},
			planConfig: rootScriptOrderConfigWithModuleCalls("other"),
			contains: []string{
				"rule 0", "after selector", "module.missing", "does not name a declared child module call",
			},
		},
		{
			name: "UndeclaredModuleWithStateInstance",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"module.missing"},
			},
			// A module removed from an updated template can remain in
			// prior state even though its call is absent from the
			// current configuration.
			modules: []*tfjson.StateModule{{
				ChildModules: []*tfjson.StateModule{{
					Address: "module.missing",
					Resources: []*tfjson.StateResource{
						scriptOrderManagedCoderScript("module.missing.coder_script.install", "install"),
					},
				}},
			}},
			planConfig: rootScriptOrderConfigWithModuleCalls("other"),
			contains: []string{
				"rule 0", "after selector", "module.missing", "does not name a declared child module call",
			},
		},
		{
			name: "ModuleSelectorRequiresPlanConfiguration",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"module.bootstrap"},
			},
			planConfig: nil,
			contains: []string{
				"rule 0", "resolve after selector", "terraform plan configuration is required",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resources := test.resources
			if resources == nil {
				resources = []*tfjson.StateResource{
					scriptOrderManagedCoderScript("coder_script.a", "a"),
					scriptOrderManagedCoderScript("coder_script.b", "b"),
				}
			}
			resources = append(resources, dataCoderScriptOrder(
				"data.coder_script_order.order", "order", test.rule,
			))
			modules := append([]*tfjson.StateModule{{Resources: resources}}, test.modules...)

			declarations, err := collectScriptOrderRuleDeclarations(
				modules, test.planConfig,
			)
			require.Error(t, err)
			require.Nil(t, declarations)
			for _, contains := range test.contains {
				require.ErrorContains(t, err, contains)
			}
		})
	}
}

func TestCollectScriptOrderRuleDeclarationsDataSourceValidation(t *testing.T) {
	t.Parallel()

	t.Run("MalformedAttributeValues", func(t *testing.T) {
		t.Parallel()

		// Malformed serialized rule values must fail with enough
		// context to identify the data source.
		declarations, err := collectScriptOrderRuleDeclarations(
			[]*tfjson.StateModule{{Resources: []*tfjson.StateResource{{
				Address: "data.coder_script_order.order",
				Mode:    tfjson.DataResourceMode,
				Type:    "coder_script_order",
				Name:    "order",
				AttributeValues: map[string]any{
					"rule": "invalid",
				},
			}}}},
			nil,
		)
		require.ErrorContains(t, err, `decode script order data source "data.coder_script_order.order"`)
		require.Nil(t, declarations)
	})

	t.Run("NoRuleBlocks", func(t *testing.T) {
		t.Parallel()

		declarations, err := collectScriptOrderRuleDeclarations(
			[]*tfjson.StateModule{{Resources: []*tfjson.StateResource{
				dataCoderScriptOrder("data.coder_script_order.order", "order"),
			}}},
			nil,
		)
		require.ErrorContains(t, err, "must contain at least one rule")
		require.Nil(t, declarations)
	})
}

func dataCoderScriptOrder(
	address string,
	name string,
	rules ...scriptOrderRuleAttributes,
) *tfjson.StateResource {
	ruleValues := make([]any, 0, len(rules))
	for _, rule := range rules {
		value := map[string]any{
			"run":   rule.Run,
			"after": rule.After,
		}
		if rule.Requires != "" {
			value["requires"] = rule.Requires
		}
		if rule.Phase != "" {
			value["phase"] = rule.Phase
		}
		ruleValues = append(ruleValues, value)
	}
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.DataResourceMode,
		Type:    "coder_script_order",
		Name:    name,
		AttributeValues: map[string]any{
			"rule": ruleValues,
		},
	}
}
