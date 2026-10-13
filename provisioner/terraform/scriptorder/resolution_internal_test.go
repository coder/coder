package scriptorder

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestResolveOrder(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		managedCoderScript("coder_script.a", "a"),
		managedCoderScript("coder_script.b", "b"),
		managedCoderScript("coder_script.c", "c"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			ruleAttributes{
				Run:   []string{"coder_script.b"},
				After: []string{"coder_script.a"},
			},
			ruleAttributes{
				Run:      []string{"coder_script.c"},
				After:    []string{"coder_script.b"},
				Requires: "completion",
			},
		),
	}}
	scripts := testScripts(
		"coder_agent.main", PhaseStart,
		"coder_script.a", "coder_script.b", "coder_script.c",
	)

	order, err := resolveOrder([]*tfjson.StateModule{module}, nil, scripts)
	require.NoError(t, err)
	require.Empty(t, order.warnings)
	require.Equal(t, []resolvedRule{
		{
			dataSourceAddress: "data.coder_script_order.order",
			ruleIndex:         0,
			runtimeAddress:    "coder_agent.main",
			phase:             PhaseStart,
			requirement:       RequirementSuccess,
			run: []resolvedSelector{{
				field:     "run",
				raw:       "coder_script.b",
				kind:      selectorScript,
				addresses: []string{"coder_script.b"},
			}},
			after: []resolvedSelector{{
				field:     "after",
				raw:       "coder_script.a",
				kind:      selectorScript,
				addresses: []string{"coder_script.a"},
			}},
		},
		{
			dataSourceAddress: "data.coder_script_order.order",
			ruleIndex:         1,
			runtimeAddress:    "coder_agent.main",
			phase:             PhaseStart,
			requirement:       RequirementCompletion,
			run: []resolvedSelector{{
				field:     "run",
				raw:       "coder_script.c",
				kind:      selectorScript,
				addresses: []string{"coder_script.c"},
			}},
			after: []resolvedSelector{{
				field:     "after",
				raw:       "coder_script.b",
				kind:      selectorScript,
				addresses: []string{"coder_script.b"},
			}},
		},
	}, order.rules)
}

func TestResolveOrderInfersStopPhaseFromScriptSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		managedCoderScript("coder_script.archive", "archive"),
		managedCoderScript("coder_script.shutdown", "shutdown"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			ruleAttributes{
				Run:   []string{"coder_script.archive"},
				After: []string{"coder_script.shutdown"},
			},
		),
	}}
	scripts := testScripts(
		"coder_agent.main", PhaseStop,
		"coder_script.archive", "coder_script.shutdown",
	)

	order, err := resolveOrder([]*tfjson.StateModule{module}, nil, scripts)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, PhaseStop, order.rules[0].phase)
	require.Equal(t, "coder_agent.main", order.rules[0].runtimeAddress)
	require.Empty(t, order.warnings)
}

func TestResolveOrderFiltersModuleScriptsByPhase(t *testing.T) {
	t.Parallel()

	module := moduleFilteringFixture("")
	scripts := testScripts(
		"coder_agent.main", PhaseStart,
		"coder_script.dependent",
		"module.alpha.coder_script.start",
		"module.beta.coder_script.start",
	)
	// Scripts omitted by phase filtering do not participate in
	// runtime validation.
	scripts["module.alpha.coder_script.stop"] = script{
		runtimeAddress: "coder_agent.other",
		runOnStop:      true,
	}
	scripts["module.beta.coder_script.stop"] = script{
		runtimeAddress: "coder_agent.other",
		runOnStop:      true,
	}

	order, err := resolveOrder(
		[]*tfjson.StateModule{module}, rootConfigWithModuleCalls("alpha", "beta"), scripts,
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, PhaseStart, order.rules[0].phase)
	require.Equal(t,
		[]string{"module.beta.coder_script.start"},
		order.rules[0].after[0].addresses,
	)
	require.Equal(t,
		[]string{"module.alpha.coder_script.start"},
		order.rules[0].after[1].addresses,
	)
	require.Equal(t, []phaseFilterWarning{{
		dataSourceAddress:            "data.coder_script_order.order",
		ruleIndex:                    0,
		inferredPhase:                PhaseStart,
		moduleSelectorsWithOmissions: []string{"module.alpha", "module.beta"},
	}}, order.warnings)
	require.Equal(t,
		`script order data source "data.coder_script_order.order" rule 0 inferred phase "start" and omitted scripts in the other phase from module selectors: "module.alpha", "module.beta"; set phase = "start" explicitly to make this filtering intentional`,
		order.warnings[0].String(),
	)
}

func TestResolveOrderExplicitPhaseFiltersWithoutWarning(t *testing.T) {
	t.Parallel()

	module := moduleFilteringFixture("start")
	module.Resources = []*tfjson.StateResource{dataCoderScriptOrder(
		"data.coder_script_order.order",
		"order",
		ruleAttributes{
			Run:   []string{"module.beta"},
			After: []string{"module.alpha"},
			Phase: "start",
		},
	)}
	scripts := testScripts(
		"coder_agent.main", PhaseStart,
		"module.alpha.coder_script.start",
		"module.beta.coder_script.start",
	)
	scripts["module.alpha.coder_script.stop"] = script{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}
	scripts["module.beta.coder_script.stop"] = script{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	order, err := resolveOrder(
		[]*tfjson.StateModule{module}, rootConfigWithModuleCalls("alpha", "beta"), scripts,
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, PhaseStart, order.rules[0].phase)
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

func TestResolveOrderAllowsSideEmptiedByPhaseFiltering(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			managedCoderScript("coder_script.first", "first"),
			managedCoderScript("coder_script.second", "second"),
			dataCoderScriptOrder(
				"data.coder_script_order.order",
				"order",
				ruleAttributes{
					Run:   []string{"coder_script.first", "coder_script.second"},
					After: []string{"module.shutdown"},
				},
			),
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.shutdown",
			Resources: []*tfjson.StateResource{managedCoderScript(
				"module.shutdown.coder_script.stop", "stop",
			)},
		}},
	}
	scripts := testScripts(
		"coder_agent.main", PhaseStart, "coder_script.first",
	)
	scripts["coder_script.second"] = script{
		runtimeAddress: "coder_agent.other",
		runOnStart:     true,
	}
	scripts["module.shutdown.coder_script.stop"] = script{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	order, err := resolveOrder(
		[]*tfjson.StateModule{module}, rootConfigWithModuleCalls("shutdown"), scripts,
	)
	// Filtering every after address makes the rule a valid no-op
	// while retaining the inferred-phase warning.
	require.NoError(t, err)
	require.Empty(t, order.rules)
	require.Equal(t, []phaseFilterWarning{{
		dataSourceAddress:            "data.coder_script_order.order",
		ruleIndex:                    0,
		inferredPhase:                PhaseStart,
		moduleSelectorsWithOmissions: []string{"module.shutdown"},
	}}, order.warnings)
}

func TestResolveOrderInfersPhaseFromModules(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			ruleAttributes{
				Run:   []string{"module.application"},
				After: []string{"module.checkout"},
			},
		)},
		ChildModules: []*tfjson.StateModule{
			{
				Address: "module.application",
				Resources: []*tfjson.StateResource{managedCoderScript(
					"module.application.coder_script.shutdown", "shutdown",
				)},
			},
			{
				Address: "module.checkout",
				Resources: []*tfjson.StateResource{managedCoderScript(
					"module.checkout.coder_script.archive", "archive",
				)},
			},
		},
	}
	scripts := testScripts(
		"coder_devcontainer.repo", PhaseStop,
		"module.application.coder_script.shutdown",
		"module.checkout.coder_script.archive",
	)

	order, err := resolveOrder(
		[]*tfjson.StateModule{module},
		rootConfigWithModuleCalls("application", "checkout"),
		scripts,
	)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, PhaseStop, order.rules[0].phase)
	require.Equal(t, "coder_devcontainer.repo", order.rules[0].runtimeAddress)
	require.Empty(t, order.warnings)
}

func TestResolveOrderRejectsMixedPhasesWithinSelector(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		managedCoderScript("coder_script.setup[0]", "setup"),
		managedCoderScript("coder_script.setup[1]", "setup"),
		managedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			ruleAttributes{
				Run:   []string{"coder_script.setup"},
				After: []string{"coder_script.prerequisite"},
			},
		),
	}}
	// The unindexed setup selector expands to one start instance and
	// one stop instance.
	scripts := testScripts(
		"coder_agent.main", PhaseStart,
		"coder_script.setup[0]", "coder_script.prerequisite",
	)
	scripts["coder_script.setup[1]"] = script{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	order, err := resolveOrder([]*tfjson.StateModule{module}, nil, scripts)
	require.ErrorContains(t, err, `run selector "coder_script.setup"`)
	require.ErrorContains(t, err, "cannot mix start and stop scripts")
	require.Empty(t, order)
}

func TestResolveOrderAllowsEmptyModuleRule(t *testing.T) {
	t.Parallel()

	// The evaluated module tree contains no script instances from
	// either child module. The config below declares both module
	// calls, so their selectors are valid but expand to no scripts.
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			ruleAttributes{
				Run:   []string{"module.application"},
				After: []string{"module.checkout"},
			},
		),
	}}

	order, err := resolveOrder(
		[]*tfjson.StateModule{module},
		rootConfigWithModuleCalls("application", "checkout"),
		nil,
	)
	require.NoError(t, err)
	require.Empty(t, order)
}

func TestResolveOrderAllowsEmptyScriptRule(t *testing.T) {
	t.Parallel()

	// The evaluated module contains no concrete script instances. The
	// config below declares both scripts with zero instances.
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			ruleAttributes{
				Run:   []string{"coder_script.optional_run"},
				After: []string{"coder_script.optional_after"},
			},
		),
	}}
	config := rootConfigWithScripts(
		configCountCoderScript("optional_run"),
		configForEachCoderScript("optional_after"),
	)

	order, err := resolveOrder(
		[]*tfjson.StateModule{module}, config, nil,
	)
	require.NoError(t, err)
	require.Empty(t, order)
}

func TestResolveOrderAllowsEmptyScriptSelectorAlongsideResolvedSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		managedCoderScript("coder_script.dependent", "dependent"),
		managedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			ruleAttributes{
				Run: []string{"coder_script.dependent"},
				After: []string{
					"coder_script.optional",
					"coder_script.prerequisite",
				},
			},
		),
	}}
	order, err := resolveOrder(
		[]*tfjson.StateModule{module},
		rootConfigWithScripts(
			configCountCoderScript("optional"),
		),
		testScripts(
			"coder_agent.main", PhaseStart,
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

func TestResolveOrderAllowsEmptyModuleSelectorAlongsideResolvedSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		managedCoderScript("coder_script.dependent", "dependent"),
		managedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			ruleAttributes{
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
	order, err := resolveOrder(
		[]*tfjson.StateModule{module},
		rootConfigWithModuleCalls("disabled"),
		testScripts(
			"coder_agent.main", PhaseStart,
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

func TestResolveOrderRejectsInvalidRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rule      ruleAttributes
		resources []*tfjson.StateResource
		scripts   map[string]script
		contains  []string
	}{
		{
			name: "ScriptNotFound",
			rule: ruleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: testScripts(
				"coder_agent.main", PhaseStart, "coder_script.a",
			),
			contains: []string{"rule 0", "coder_script.b", "was not found"},
		},
		{
			name: "NoLifecyclePhase",
			rule: ruleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]script{
				"coder_script.a": {runtimeAddress: "coder_agent.main"},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStart: true},
			},
			contains: []string{"coder_script.a", "neither run_on_start nor run_on_stop enabled"},
		},
		{
			name: "CronOnly",
			rule: ruleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]script{
				"coder_script.a": {
					runtimeAddress: "coder_agent.main", hasCron: true,
				},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStart: true},
			},
			contains: []string{"coder_script.a", "cron-only"},
		},
		{
			name: "BothLifecyclePhases",
			rule: ruleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]script{
				"coder_script.a": {
					runtimeAddress: "coder_agent.main", runOnStart: true, runOnStop: true,
				},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStart: true},
			},
			contains: []string{"coder_script.a", "both run_on_start and run_on_stop enabled"},
		},
		{
			name: "MixedResourcePhases",
			rule: ruleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]script{
				"coder_script.a": {runtimeAddress: "coder_agent.main", runOnStart: true},
				"coder_script.b": {runtimeAddress: "coder_agent.main", runOnStop: true},
			},
			contains: []string{"rule 0", "start script", "stop script", "cannot mix start and stop scripts"},
		},
		{
			name: "ResourceConflictsWithDeclaredPhase",
			rule: ruleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"}, Phase: "stop",
			},
			scripts: testScripts(
				"coder_agent.main", PhaseStart, "coder_script.a", "coder_script.b",
			),
			contains: []string{"coder_script.b", "start script", `rule phase is "stop"`},
		},
		{
			name: "CrossRuntime",
			rule: ruleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]script{
				"coder_script.a": {runtimeAddress: "coder_agent.main", runOnStart: true},
				"coder_script.b": {runtimeAddress: "coder_devcontainer.repo", runOnStart: true},
			},
			contains: []string{
				"coder_agent.main", "coder_devcontainer.repo", "only within the same agent or devcontainer subagent",
			},
		},
		{
			name: "MissingRuntime",
			rule: ruleAttributes{
				Run: []string{"coder_script.b"}, After: []string{"coder_script.a"},
			},
			scripts: map[string]script{
				"coder_script.a": {runtimeAddress: "coder_agent.main", runOnStart: true},
				"coder_script.b": {runOnStart: true},
			},
			contains: []string{"coder_script.b", "could not be associated with an agent or devcontainer subagent"},
		},
		{
			name: "SelfDependency",
			rule: ruleAttributes{
				Run: []string{"coder_script.a"}, After: []string{"coder_script.a"},
			},
			scripts: testScripts(
				"coder_agent.main", PhaseStart, "coder_script.a", "coder_script.b",
			),
			contains: []string{"run selector", "after selector", "coder_script.a", "depend on itself"},
		},
		{
			name: "SelfDependencyAfterSelectorExpansion",
			rule: ruleAttributes{
				Run: []string{"coder_script.setup"}, After: []string{"coder_script.setup[0]"},
			},
			resources: []*tfjson.StateResource{
				managedCoderScript("coder_script.setup[0]", "setup"),
			},
			scripts: testScripts(
				"coder_agent.main", PhaseStart, "coder_script.setup[0]",
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
				managedCoderScript("coder_script.a", "a"),
				managedCoderScript("coder_script.b", "b"),
			}
			resources = append(resources, test.resources...)
			resources = append(resources, dataCoderScriptOrder(
				"data.coder_script_order.order", "order", test.rule,
			))
			module := &tfjson.StateModule{Resources: resources}
			order, err := resolveOrder(
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

func TestResolveOrderRejectsInvalidModuleSelectedScripts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		script   script
		contains string
	}{
		{
			name: "CronOnly",
			script: script{
				runtimeAddress: "coder_agent.main",
				hasCron:        true,
			},
			contains: "cron-only",
		},
		{
			name: "BothLifecyclePhases",
			script: script{
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
					managedCoderScript("coder_script.dependent", "dependent"),
					dataCoderScriptOrder(
						"data.coder_script_order.order",
						"order",
						ruleAttributes{
							Run:   []string{"coder_script.dependent"},
							After: []string{"module.bootstrap"},
						},
					),
				},
				ChildModules: []*tfjson.StateModule{{
					Address: "module.bootstrap",
					Resources: []*tfjson.StateResource{managedCoderScript(
						"module.bootstrap.coder_script.setup", "setup",
					)},
				}},
			}
			scripts := testScripts(
				"coder_agent.main", PhaseStart, "coder_script.dependent",
			)
			scripts["module.bootstrap.coder_script.setup"] = test.script

			order, err := resolveOrder(
				[]*tfjson.StateModule{module}, rootConfigWithModuleCalls("bootstrap"), scripts,
			)
			require.ErrorContains(t, err, `after selector "module.bootstrap"`)
			require.ErrorContains(t, err, test.contains)
			require.Empty(t, order)
		})
	}
}

func TestResolveOrderRejectsAmbiguousModulePhase(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{dataCoderScriptOrder(
			"data.coder_script_order.order",
			"order",
			ruleAttributes{
				Run:   []string{"module.application"},
				After: []string{"module.checkout"},
			},
		)},
		ChildModules: []*tfjson.StateModule{
			{
				Address: "module.application",
				Resources: []*tfjson.StateResource{managedCoderScript(
					"module.application.coder_script.start", "start",
				)},
			},
			{
				Address: "module.checkout",
				Resources: []*tfjson.StateResource{managedCoderScript(
					"module.checkout.coder_script.stop", "stop",
				)},
			},
		},
	}
	scripts := testScripts(
		"coder_agent.main", PhaseStart,
		"module.application.coder_script.start",
	)
	scripts["module.checkout.coder_script.stop"] = script{
		runtimeAddress: "coder_agent.main",
		runOnStop:      true,
	}

	// With only module selectors spanning both phases and no explicit
	// phase, the rule's phase cannot be inferred.
	order, err := resolveOrder(
		[]*tfjson.StateModule{module},
		rootConfigWithModuleCalls("application", "checkout"),
		scripts,
	)
	require.ErrorContains(t, err, "phase cannot be inferred")
	require.ErrorContains(t, err, `set phase to "start" or "stop"`)
	require.Empty(t, order)
}

func moduleFilteringFixture(phase string) *tfjson.StateModule {
	return &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			managedCoderScript("coder_script.dependent", "dependent"),
			dataCoderScriptOrder(
				"data.coder_script_order.order",
				"order",
				ruleAttributes{
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
					managedCoderScript("module.alpha.coder_script.start", "start"),
					managedCoderScript("module.alpha.coder_script.stop", "stop"),
				},
			},
			{
				Address: "module.beta",
				Resources: []*tfjson.StateResource{
					managedCoderScript("module.beta.coder_script.start", "start"),
					managedCoderScript("module.beta.coder_script.stop", "stop"),
				},
			},
		},
	}
}

func testScripts(
	runtimeAddress string,
	phase Phase,
	addresses ...string,
) map[string]script {
	scripts := make(map[string]script, len(addresses))
	for _, address := range addresses {
		script := script{runtimeAddress: runtimeAddress}
		switch phase {
		case PhaseStart:
			script.runOnStart = true
		case PhaseStop:
			script.runOnStop = true
		default:
			panic("unknown script order phase")
		}
		scripts[address] = script
	}
	return scripts
}
