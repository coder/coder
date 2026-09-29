package scriptorder

import (
	"fmt"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestResolveScriptOrder(t *testing.T) {
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
			},
			scriptOrderRuleAttributes{
				Run:      []string{"coder_script.c"},
				After:    []string{"coder_script.b"},
				Requires: "completion",
			},
		),
	}}
	scripts := scriptOrderTestScripts(
		"coder_agent.main", ScriptOrderPhaseStart,
		"coder_script.a", "coder_script.b", "coder_script.c",
	)

	order, err := resolveScriptOrder([]*tfjson.StateModule{module}, nil, scripts)
	require.NoError(t, err)
	require.Empty(t, order.warnings)
	require.Equal(t, []resolvedScriptOrderRule{
		{
			dataSourceAddress: "data.coder_script_order.order",
			ruleIndex:         0,
			runtimeAddress:    "coder_agent.main",
			phase:             ScriptOrderPhaseStart,
			requirement:       ScriptOrderRequirementSuccess,
			run: []resolvedScriptOrderSelector{{
				field:     "run",
				raw:       "coder_script.b",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.b"},
			}},
			after: []resolvedScriptOrderSelector{{
				field:     "after",
				raw:       "coder_script.a",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.a"},
			}},
		},
		{
			dataSourceAddress: "data.coder_script_order.order",
			ruleIndex:         1,
			runtimeAddress:    "coder_agent.main",
			phase:             ScriptOrderPhaseStart,
			requirement:       ScriptOrderRequirementCompletion,
			run: []resolvedScriptOrderSelector{{
				field:     "run",
				raw:       "coder_script.c",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.c"},
			}},
			after: []resolvedScriptOrderSelector{{
				field:     "after",
				raw:       "coder_script.b",
				kind:      scriptOrderSelectorScript,
				addresses: []string{"coder_script.b"},
			}},
		},
	}, order.rules)
}

func TestResolveScriptOrderInfersStopPhaseFromScriptSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.archive", "archive"),
		scriptOrderManagedCoderScript("coder_script.shutdown", "shutdown"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.archive"},
				After: []string{"coder_script.shutdown"},
			},
		),
	}}
	scripts := scriptOrderTestScripts(
		"coder_agent.main", ScriptOrderPhaseStop,
		"coder_script.archive", "coder_script.shutdown",
	)

	order, err := resolveScriptOrder([]*tfjson.StateModule{module}, nil, scripts)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, ScriptOrderPhaseStop, order.rules[0].phase)
	require.Equal(t, "coder_agent.main", order.rules[0].runtimeAddress)
	require.Empty(t, order.warnings)
}

func TestResolveScriptOrderFiltersModuleScriptsByPhase(t *testing.T) {
	t.Parallel()

	module := scriptOrderModuleFilteringFixture("")
	scripts := scriptOrderTestScripts(
		"coder_agent.main", ScriptOrderPhaseStart,
		"coder_script.dependent",
		"module.alpha.coder_script.start",
		"module.beta.coder_script.start",
	)
	// Scripts omitted by phase filtering do not participate in
	// runtime validation.
	scripts["module.alpha.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.other",
		runOnStop:      true,
	}
	scripts["module.beta.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.other",
		runOnStop:      true,
	}

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module}, rootScriptOrderConfigWithModuleCalls("alpha", "beta"), scripts,
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, ScriptOrderPhaseStart, order.rules[0].phase)
	require.Equal(t,
		[]string{"module.beta.coder_script.start"},
		order.rules[0].after[0].addresses,
	)
	require.Equal(t,
		[]string{"module.alpha.coder_script.start"},
		order.rules[0].after[1].addresses,
	)
	require.Equal(t, []scriptOrderPhaseFilterWarning{{
		dataSourceAddress:            "data.coder_script_order.order",
		ruleIndex:                    0,
		inferredPhase:                ScriptOrderPhaseStart,
		moduleSelectorsWithOmissions: []string{"module.alpha", "module.beta"},
	}}, order.warnings)
	require.Equal(t,
		`script order data source "data.coder_script_order.order" rule 0 inferred phase "start" and omitted scripts in the other phase from module selectors: "module.alpha", "module.beta"; set phase = "start" explicitly to make this filtering intentional`,
		order.warnings[0].String(),
	)
}

func TestResolveScriptOrderExplicitPhaseFiltersWithoutWarning(t *testing.T) {
	t.Parallel()

	module := scriptOrderModuleFilteringFixture("start")
	module.Resources = []*tfjson.StateResource{dataCoderScriptOrder(
		"data.coder_script_order.order",
		"order",
		scriptOrderRuleAttributes{
			Run:   []string{"module.beta"},
			After: []string{"module.alpha"},
			Phase: "start",
		},
	)}
	scripts := scriptOrderTestScripts(
		"coder_agent.main", ScriptOrderPhaseStart,
		"module.alpha.coder_script.start",
		"module.beta.coder_script.start",
	)
	scripts["module.alpha.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}
	scripts["module.beta.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module}, rootScriptOrderConfigWithModuleCalls("alpha", "beta"), scripts,
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, ScriptOrderPhaseStart, order.rules[0].phase)
	require.Equal(t,
		[]string{"module.beta.coder_script.start"},
		order.rules[0].run[0].addresses,
	)
	require.Equal(t,
		[]string{"module.alpha.coder_script.start"},
		order.rules[0].after[0].addresses,
	)
	require.Empty(t, order.warnings)
}

func TestResolveScriptOrderAllowsSideEmptiedByPhaseFiltering(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("coder_script.first", "first"),
			scriptOrderManagedCoderScript("coder_script.second", "second"),
			dataCoderScriptOrder(
				"data.coder_script_order.order",
				"order",
				scriptOrderRuleAttributes{
					Run:   []string{"coder_script.first", "coder_script.second"},
					After: []string{"module.shutdown"},
				},
			),
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.shutdown",
			Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
				"module.shutdown.coder_script.stop", "stop",
			)},
		}},
	}
	scripts := scriptOrderTestScripts(
		"coder_agent.main", ScriptOrderPhaseStart, "coder_script.first",
	)
	scripts["coder_script.second"] = scriptOrderScript{
		runtimeAddress: "coder_agent.other",
		runOnStart:     true,
	}
	scripts["module.shutdown.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module}, rootScriptOrderConfigWithModuleCalls("shutdown"), scripts,
	)
	// Filtering every after address makes the rule a valid no-op
	// while retaining the inferred-phase warning.
	require.NoError(t, err)
	require.Empty(t, order.rules)
	require.Equal(t, []scriptOrderPhaseFilterWarning{{
		dataSourceAddress:            "data.coder_script_order.order",
		ruleIndex:                    0,
		inferredPhase:                ScriptOrderPhaseStart,
		moduleSelectorsWithOmissions: []string{"module.shutdown"},
	}}, order.warnings)
}

func TestResolveScriptOrderInfersPhaseFromModules(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"module.application"},
				After: []string{"module.checkout"},
			},
		)},
		ChildModules: []*tfjson.StateModule{
			{
				Address: "module.application",
				Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
					"module.application.coder_script.shutdown", "shutdown",
				)},
			},
			{
				Address: "module.checkout",
				Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
					"module.checkout.coder_script.archive", "archive",
				)},
			},
		},
	}
	scripts := scriptOrderTestScripts(
		"coder_devcontainer.repo", ScriptOrderPhaseStop,
		"module.application.coder_script.shutdown",
		"module.checkout.coder_script.archive",
	)

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("application", "checkout"),
		scripts,
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, ScriptOrderPhaseStop, order.rules[0].phase)
	require.Equal(t, "coder_devcontainer.repo", order.rules[0].runtimeAddress)
	require.Empty(t, order.warnings)
}

func TestResolveScriptOrderRejectsMixedPhasesWithinSelector(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.setup[0]", "setup"),
		scriptOrderManagedCoderScript("coder_script.setup[1]", "setup"),
		scriptOrderManagedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.setup"},
				After: []string{"coder_script.prerequisite"},
			},
		),
	}}
	// The unindexed setup selector expands to one start instance and
	// one stop instance.
	scripts := scriptOrderTestScripts(
		"coder_agent.main", ScriptOrderPhaseStart,
		"coder_script.setup[0]", "coder_script.prerequisite",
	)
	scripts["coder_script.setup[1]"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	order, err := resolveScriptOrder([]*tfjson.StateModule{module}, nil, scripts)
	require.ErrorContains(t, err, `run selector "coder_script.setup"`)
	require.ErrorContains(t, err, "cannot mix start and stop scripts")
	require.Empty(t, order)
}

func TestResolveScriptOrderAllowsEmptyModuleRule(t *testing.T) {
	t.Parallel()

	// The evaluated module tree contains no script instances from
	// either child module. The config below declares both module
	// calls, so their selectors are valid but expand to no scripts.
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"module.application"},
				After: []string{"module.checkout"},
			},
		),
	}}

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("application", "checkout"),
		nil,
	)
	require.NoError(t, err)
	require.Empty(t, order)
}

func TestResolveScriptOrderAllowsEmptyScriptRule(t *testing.T) {
	t.Parallel()

	// The evaluated module contains no concrete script instances. The
	// config below declares both scripts with zero instances.
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.optional_run"},
				After: []string{"coder_script.optional_after"},
			},
		),
	}}
	config := rootScriptOrderConfigWithScripts(
		scriptOrderConfigCountCoderScript("optional_run"),
		scriptOrderConfigForEachCoderScript("optional_after"),
	)

	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module}, config, nil,
	)
	require.NoError(t, err)
	require.Empty(t, order)
}

func TestResolveScriptOrderAllowsEmptyScriptSelectorAlongsideResolvedSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.dependent", "dependent"),
		scriptOrderManagedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run: []string{"coder_script.dependent"},
				After: []string{
					"coder_script.optional",
					"coder_script.prerequisite",
				},
			},
		),
	}}
	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithScripts(
			scriptOrderConfigCountCoderScript("optional"),
		),
		scriptOrderTestScripts(
			"coder_agent.main", ScriptOrderPhaseStart,
			"coder_script.dependent", "coder_script.prerequisite",
		),
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Empty(t, order.rules[0].after[0].addresses)
	require.Equal(t,
		[]string{"coder_script.prerequisite"},
		order.rules[0].after[1].addresses,
	)
	require.Empty(t, order.warnings)
}

func TestResolveScriptOrderAllowsEmptyModuleSelectorAlongsideResolvedSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.dependent", "dependent"),
		scriptOrderManagedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run: []string{"coder_script.dependent"},
				After: []string{
					"module.disabled",
					"coder_script.prerequisite",
				},
			},
		),
	}}
	// The plan declares module.disabled, but the evaluated state
	// contains no instance, modeling a module call with count = 0.
	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("disabled"),
		scriptOrderTestScripts(
			"coder_agent.main", ScriptOrderPhaseStart,
			"coder_script.dependent", "coder_script.prerequisite",
		),
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Empty(t, order.rules[0].after[0].addresses)
	require.Equal(t,
		[]string{"coder_script.prerequisite"},
		order.rules[0].after[1].addresses,
	)
}

func TestResolveScriptOrderRejectsInvalidRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rule      scriptOrderRuleAttributes
		resources []*tfjson.StateResource
		scripts   map[string]scriptOrderScript
		contains  []string
	}{
		{
			name: "ScriptNotFound",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: scriptOrderTestScripts(
				"coder_agent.main", ScriptOrderPhaseStart, "coder_script.a",
			),
			contains: []string{"rule 0", "coder_script.b", "was not found"},
		},
		{
			name: "NoLifecyclePhase",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {runtimeAddress: "coder_agent.main"},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStart: true},
			},
			contains: []string{"coder_script.a", "neither run_on_start nor run_on_stop enabled"},
		},
		{
			name: "CronOnly",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {
					runtimeAddress: "coder_agent.main", cron: "0 * * * *",
				},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStart: true},
			},
			contains: []string{"coder_script.a", "cron-only"},
		},
		{
			name: "BothLifecyclePhases",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {
					runtimeAddress: "coder_agent.main", runOnStart: true, runOnStop: true,
				},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStart: true},
			},
			contains: []string{"coder_script.a", "both run_on_start and run_on_stop enabled"},
		},
		{
			name: "MixedResourcePhases",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {runtimeAddress: "coder_agent.main", runOnStart: true},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStop: true},
			},
			contains: []string{"rule 0", "start script", "stop script", "cannot mix start and stop scripts"},
		},
		{
			name: "ResourceConflictsWithDeclaredPhase",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"}, Phase: "stop",
			},
			scripts: scriptOrderTestScripts(
				"coder_agent.main", ScriptOrderPhaseStart, "coder_script.a", "coder_script.b",
			),
			contains: []string{"coder_script.b", "start script", `rule phase is "stop"`},
		},
		{
			name: "CrossRuntime",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {runtimeAddress: "coder_agent.main", runOnStart: true},
				"coder_script.b": {runtimeAddress: "coder_devcontainer.repo", runOnStart: true},
			},
			contains: []string{
				"coder_agent.main", "coder_devcontainer.repo", "only within the same agent or devcontainer subagent",
			},
		},
		{
			name: "MissingRuntime",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {runtimeAddress: "coder_agent.main", runOnStart: true},
				"coder_script.b": {runOnStart: true},
			},
			contains: []string{"coder_script.b", "could not be associated with an agent or devcontainer subagent"},
		},
		{
			name: "SelfDependency",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.a"}, After: []string{"coder_script.a"},
			},
			scripts: scriptOrderTestScripts(
				"coder_agent.main", ScriptOrderPhaseStart, "coder_script.a", "coder_script.b",
			),
			contains: []string{"run selector", "after selector", "coder_script.a", "depend on itself"},
		},
		{
			name: "SelfDependencyAfterSelectorExpansion",
			rule: scriptOrderRuleAttributes{
				Run: []string{"coder_script.setup"}, After: []string{"coder_script.setup[0]"},
			},
			resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript("coder_script.setup[0]", "setup"),
			},
			scripts: scriptOrderTestScripts(
				"coder_agent.main", ScriptOrderPhaseStart, "coder_script.setup[0]",
			),
			contains: []string{
				"run selector", "coder_script.setup", "after selector",
				"coder_script.setup[0]", "depend on itself",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resources := []*tfjson.StateResource{
				scriptOrderManagedCoderScript("coder_script.a", "a"),
				scriptOrderManagedCoderScript("coder_script.b", "b"),
			}
			resources = append(resources, test.resources...)
			resources = append(resources, dataCoderScriptOrder(
				"data.coder_script_order.order", "order", test.rule,
			))
			module := &tfjson.StateModule{Resources: resources}
			order, err := resolveScriptOrder(
				[]*tfjson.StateModule{module}, nil, test.scripts,
			)
			require.Error(t, err)
			require.Empty(t, order)
			for _, contains := range test.contains {
				require.ErrorContains(t, err, contains)
			}
		})
	}
}

func TestResolveScriptOrderRejectsInvalidModuleSelectedScripts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		script   scriptOrderScript
		contains string
	}{
		{
			name: "CronOnly",
			script: scriptOrderScript{
				runtimeAddress: "coder_agent.main",
				cron:           "0 * * * *",
			},
			contains: "cron-only",
		},
		{
			name: "BothLifecyclePhases",
			script: scriptOrderScript{
				runtimeAddress: "coder_agent.main",
				runOnStart:     true,
				runOnStop:      true,
			},
			contains: "both run_on_start and run_on_stop enabled",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			module := &tfjson.StateModule{
				Resources: []*tfjson.StateResource{
					scriptOrderManagedCoderScript("coder_script.dependent", "dependent"),
					dataCoderScriptOrder(
						"data.coder_script_order.order",
						"order",
						scriptOrderRuleAttributes{
							Run:   []string{"coder_script.dependent"},
							After: []string{"module.bootstrap"},
						},
					),
				},
				ChildModules: []*tfjson.StateModule{{
					Address: "module.bootstrap",
					Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
						"module.bootstrap.coder_script.setup", "setup",
					)},
				}},
			}
			scripts := scriptOrderTestScripts(
				"coder_agent.main", ScriptOrderPhaseStart, "coder_script.dependent",
			)
			scripts["module.bootstrap.coder_script.setup"] = test.script

			order, err := resolveScriptOrder(
				[]*tfjson.StateModule{module}, rootScriptOrderConfigWithModuleCalls("bootstrap"), scripts,
			)
			require.ErrorContains(t, err, `after selector "module.bootstrap"`)
			require.ErrorContains(t, err, test.contains)
			require.Empty(t, order)
		})
	}
}

func TestResolveScriptOrderRejectsAmbiguousModulePhase(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			scriptOrderRuleAttributes{
				Run:   []string{"module.application"},
				After: []string{"module.checkout"},
			},
		)},
		ChildModules: []*tfjson.StateModule{
			{
				Address: "module.application",
				Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
					"module.application.coder_script.start", "start",
				)},
			},
			{
				Address: "module.checkout",
				Resources: []*tfjson.StateResource{scriptOrderManagedCoderScript(
					"module.checkout.coder_script.stop", "stop",
				)},
			},
		},
	}
	scripts := scriptOrderTestScripts(
		"coder_agent.main", ScriptOrderPhaseStart,
		"module.application.coder_script.start",
	)
	scripts["module.checkout.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	// With only module selectors spanning both phases and no explicit
	// phase, the rule's phase cannot be inferred.
	order, err := resolveScriptOrder(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("application", "checkout"),
		scripts,
	)
	require.ErrorContains(t, err, "phase cannot be inferred")
	require.ErrorContains(t, err, `set phase to "start" or "stop"`)
	require.Empty(t, order)
}

func TestPrepareScriptOrderBoundsPhaseFilterWarnings(t *testing.T) {
	t.Parallel()

	rules := make([]scriptOrderRuleAttributes, maxScriptOrderPhaseFilterWarnings+2)
	for index := range rules {
		rules[index] = scriptOrderRuleAttributes{
			Run:   []string{"coder_script.dependent"},
			After: []string{"module.alpha"},
		}
	}
	module := scriptOrderModuleFilteringFixture("")
	module.Resources[1] = dataCoderScriptOrder(
		"data.coder_script_order.order", "order", rules...,
	)
	scripts := scriptOrderTestScripts(
		"coder_agent.main", ScriptOrderPhaseStart,
		"coder_script.dependent", "module.alpha.coder_script.start",
	)
	scripts["module.alpha.coder_script.stop"] = scriptOrderScript{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	prepared, err := prepareScriptOrderForTest(
		[]*tfjson.StateModule{module},
		rootScriptOrderConfigWithModuleCalls("alpha"),
		scripts,
	)
	require.NoError(t, err)
	require.Len(t, prepared.rules, len(rules))
	require.Len(t, prepared.warnings, maxScriptOrderPhaseFilterWarnings)
	require.Equal(t, 2, prepared.warningsOmitted)
}

func TestScriptOrderPhaseFilterWarningBoundsDiagnostic(t *testing.T) {
	t.Parallel()

	selectors := make(
		[]string, 0, maxScriptOrderPhaseFilterWarningSelectors+2,
	)
	for index := range maxScriptOrderPhaseFilterWarningSelectors + 2 {
		selectors = append(selectors, fmt.Sprintf(
			"module.%d_%s", index, strings.Repeat("selector", 60),
		))
	}
	dataSourceAddress := "data.coder_script_order." + strings.Repeat("order", 60)
	retained, omitted := boundedScriptOrderPhaseFilterWarningSelectors(selectors)
	require.Len(t, retained, maxScriptOrderPhaseFilterWarningSelectors)
	require.Equal(t, 2, omitted)
	warning := scriptOrderPhaseFilterWarning{
		dataSourceAddress:                dataSourceAddress,
		ruleIndex:                        0,
		inferredPhase:                    ScriptOrderPhaseStart,
		moduleSelectorsWithOmissions:     retained,
		additionalModuleSelectorsOmitted: omitted,
	}.String()

	require.NotContains(t, warning, dataSourceAddress)
	require.NotContains(t, warning, selectors[0])
	require.Contains(t, warning, `data.coder_script_order.`)
	require.Contains(t, warning, `"module.0_`)
	require.Contains(t, warning, "… (2 more)")
	require.Less(t, len(warning), 8*1024)
}

func scriptOrderModuleFilteringFixture(phase string) *tfjson.StateModule {
	return &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			scriptOrderManagedCoderScript("coder_script.dependent", "dependent"),
			dataCoderScriptOrder(
				"data.coder_script_order.order",
				"order",
				scriptOrderRuleAttributes{
					Run:   []string{"coder_script.dependent"},
					After: []string{"module.beta", "module.alpha"},
					Phase: phase,
				},
			),
		},
		ChildModules: []*tfjson.StateModule{
			{
				Address: "module.alpha",
				Resources: []*tfjson.StateResource{
					scriptOrderManagedCoderScript("module.alpha.coder_script.start", "start"),
					scriptOrderManagedCoderScript("module.alpha.coder_script.stop", "stop"),
				},
			},
			{
				Address: "module.beta",
				Resources: []*tfjson.StateResource{
					scriptOrderManagedCoderScript("module.beta.coder_script.start", "start"),
					scriptOrderManagedCoderScript("module.beta.coder_script.stop", "stop"),
				},
			},
		},
	}
}

func scriptOrderTestScripts(
	runtimeAddress string,
	phase ScriptOrderPhase,
	addresses ...string,
) map[string]scriptOrderScript {
	scripts := make(map[string]scriptOrderScript, len(addresses))
	for _, address := range addresses {
		script := scriptOrderScript{runtimeAddress: runtimeAddress}
		switch phase {
		case ScriptOrderPhaseStart:
			script.runOnStart = true
		case ScriptOrderPhaseStop:
			script.runOnStop = true
		default:
			panic("unknown script order phase")
		}
		scripts[address] = script
	}
	return scripts
}
