package terraform

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

type scriptOrderTestRule struct {
	run      []string
	after    []string
	requires string
}

func TestResolveScriptOrderRules(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		managedScript("coder_script.a", "a"),
		managedScript("coder_script.b", "b"),
		managedScript("coder_script.c", "c"),
		scriptOrderTestDataSource("data.coder_script_order.first", "first",
			scriptOrderTestRule{
				run:   []string{"coder_script.b"},
				after: []string{"coder_script.a"},
			},
		),
		scriptOrderTestDataSource("data.coder_script_order.second", "second",
			scriptOrderTestRule{
				run:      []string{"coder_script.c"},
				after:    []string{"coder_script.b"},
				requires: "completion",
			},
		),
	}}
	scripts := scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup,
		"coder_script.a", "coder_script.b", "coder_script.c")

	order, err := resolveScriptOrderRules([]*tfjson.StateModule{module}, scripts)
	require.NoError(t, err)
	require.Equal(t, resolvedScriptOrder{
		rules: []resolvedScriptOrderRule{
			{
				dataSourceAddress: "data.coder_script_order.first",
				ruleIndex:         0,
				runtimeAddress:    "coder_agent.main",
				phase:             ScriptOrderPhaseStartup,
				requirement:       ScriptOrderRequirementSuccess,
				run: []resolvedScriptOrderSelector{{
					field:     "run",
					raw:       "coder_script.b",
					addresses: []string{"coder_script.b"},
				}},
				after: []resolvedScriptOrderSelector{{
					field:     "after",
					raw:       "coder_script.a",
					addresses: []string{"coder_script.a"},
				}},
			},
			{
				dataSourceAddress: "data.coder_script_order.second",
				ruleIndex:         0,
				runtimeAddress:    "coder_agent.main",
				phase:             ScriptOrderPhaseStartup,
				requirement:       ScriptOrderRequirementCompletion,
				run: []resolvedScriptOrderSelector{{
					field:     "run",
					raw:       "coder_script.c",
					addresses: []string{"coder_script.c"},
				}},
				after: []resolvedScriptOrderSelector{{
					field:     "after",
					raw:       "coder_script.b",
					addresses: []string{"coder_script.b"},
				}},
			},
		},
	}, order)
}

func TestResolveScriptOrderRulesSupportsShutdownAndDevcontainerSubagents(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		managedScript("coder_script.stop_a", "stop_a"),
		managedScript("coder_script.stop_b", "stop_b"),
		managedScript("coder_script.dc_a", "dc_a"),
		managedScript("coder_script.dc_b", "dc_b"),
		scriptOrderTestDataSource("data.coder_script_order.order", "order",
			scriptOrderTestRule{run: []string{"coder_script.stop_b"}, after: []string{"coder_script.stop_a"}},
			scriptOrderTestRule{run: []string{"coder_script.dc_b"}, after: []string{"coder_script.dc_a"}},
		),
	}}
	scripts := scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseShutdown,
		"coder_script.stop_a", "coder_script.stop_b")
	for address, script := range scriptOrderTestScripts("coder_devcontainer.dev", ScriptOrderPhaseStartup,
		"coder_script.dc_a", "coder_script.dc_b") {
		scripts[address] = script
	}

	order, err := resolveScriptOrderRules([]*tfjson.StateModule{module}, scripts)
	require.NoError(t, err)
	require.Len(t, order.rules, 2)
	require.Equal(t, "coder_agent.main", order.rules[0].runtimeAddress)
	require.Equal(t, ScriptOrderPhaseShutdown, order.rules[0].phase)
	require.Equal(t, "coder_devcontainer.dev", order.rules[1].runtimeAddress)
	require.Equal(t, ScriptOrderPhaseStartup, order.rules[1].phase)
}

func TestResolveScriptOrderRulesValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		resources []*tfjson.StateResource
		scripts   map[string]scriptOrderScript
		rule      scriptOrderTestRule
		contains  []string
	}{
		{
			name: "EmptyExpansionAndInlineScripts",
			resources: []*tfjson.StateResource{
				{
					Address: "coder_agent.main",
					Mode:    tfjson.ManagedResourceMode,
					Type:    "coder_agent",
					Name:    "main",
					AttributeValues: map[string]any{
						"startup_script": "echo inline",
					},
				},
				managedScript("coder_script.a", "a"),
			},
			scripts:  scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup, "coder_script.a"),
			rule:     scriptOrderTestRule{run: []string{"coder_script.missing"}, after: []string{"coder_script.a"}},
			contains: []string{"data.coder_script_order.order", "rule 0", "coder_script.missing", "expanded to no coder_script resources", "inline scripts cannot be selected"},
		},
		{
			name:      "NoLifecyclePhase",
			resources: scriptOrderTestManagedScripts("a", "b"),
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {RuntimeAddress: "coder_agent.main"},
				"coder_script.b": {RuntimeAddress: "coder_agent.main", RunOnStart: true},
			},
			rule:     scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
			contains: []string{"after selector", "coder_script.a", "no startup or shutdown lifecycle phase"},
		},
		{
			name:      "CronOnly",
			resources: scriptOrderTestManagedScripts("a", "b"),
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {RuntimeAddress: "coder_agent.main", Cron: "0 * * * *"},
				"coder_script.b": {RuntimeAddress: "coder_agent.main", RunOnStart: true},
			},
			rule:     scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
			contains: []string{"coder_script.a", "cron-only"},
		},
		{
			name:      "BothLifecyclePhases",
			resources: scriptOrderTestManagedScripts("a", "b"),
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {RuntimeAddress: "coder_agent.main", RunOnStart: true, RunOnStop: true},
				"coder_script.b": {RuntimeAddress: "coder_agent.main", RunOnStart: true},
			},
			rule:     scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
			contains: []string{"coder_script.a", "both startup and shutdown phases enabled"},
		},
		{
			name:      "MixedPhases",
			resources: scriptOrderTestManagedScripts("a", "b"),
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {RuntimeAddress: "coder_agent.main", RunOnStart: true},
				"coder_script.b": {RuntimeAddress: "coder_agent.main", RunOnStop: true},
			},
			rule:     scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
			contains: []string{"coder_script.a", "coder_script.b", "startup and shutdown phases cannot be mixed"},
		},
		{
			name:      "CrossAgent",
			resources: scriptOrderTestManagedScripts("a", "b"),
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {RuntimeAddress: "coder_agent.one", RunOnStart: true},
				"coder_script.b": {RuntimeAddress: "coder_agent.two", RunOnStart: true},
			},
			rule:     scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
			contains: []string{"coder_agent.one", "coder_agent.two", "cross-agent dependencies are not supported"},
		},
		{
			name:      "ParentAgentToDevcontainerSubagent",
			resources: scriptOrderTestManagedScripts("a", "b"),
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {RuntimeAddress: "coder_agent.main", RunOnStart: true},
				"coder_script.b": {RuntimeAddress: "coder_devcontainer.dev", RunOnStart: true},
			},
			rule:     scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
			contains: []string{"coder_agent.main", "coder_devcontainer.dev", "cross-agent dependencies are not supported"},
		},
		{
			name:      "MissingAgentRuntime",
			resources: scriptOrderTestManagedScripts("a", "b"),
			scripts: map[string]scriptOrderScript{
				"coder_script.a": {RuntimeAddress: "coder_agent.main", RunOnStart: true},
				"coder_script.b": {RunOnStart: true},
			},
			rule:     scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
			contains: []string{"coder_script.b", "could not be associated with an agent runtime"},
		},
		{
			name:      "SelfDependency",
			resources: scriptOrderTestManagedScripts("a"),
			scripts:   scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup, "coder_script.a"),
			rule:      scriptOrderTestRule{run: []string{"coder_script.a"}, after: []string{"coder_script.a"}},
			contains:  []string{"run selector", "after selector", "coder_script.a", "cannot depend on itself"},
		},
		{
			name:      "InvalidRequirement",
			resources: scriptOrderTestManagedScripts("a", "b"),
			scripts:   scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup, "coder_script.a", "coder_script.b"),
			rule:      scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}, requires: "started"},
			contains:  []string{"rule 0", "requires", "success", "completion", "started"},
		},
		{
			name:      "EmptyRunList",
			resources: scriptOrderTestManagedScripts("a"),
			scripts:   scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup, "coder_script.a"),
			rule:      scriptOrderTestRule{after: []string{"coder_script.a"}},
			contains:  []string{"rule 0", "run must contain at least one selector"},
		},
		{
			name:      "EmptyAfterList",
			resources: scriptOrderTestManagedScripts("a"),
			scripts:   scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup, "coder_script.a"),
			rule:      scriptOrderTestRule{run: []string{"coder_script.a"}},
			contains:  []string{"rule 0", "after must contain at least one selector"},
		},
		{
			name:      "MalformedSelector",
			resources: scriptOrderTestManagedScripts("a"),
			scripts:   scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup, "coder_script.a"),
			rule:      scriptOrderTestRule{run: []string{"coder_script.a["}, after: []string{"coder_script.a"}},
			contains:  []string{"run selector", "coder_script.a["},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resources := append([]*tfjson.StateResource{}, test.resources...)
			resources = append(resources, scriptOrderTestDataSource("data.coder_script_order.order", "order", test.rule))
			order, err := resolveScriptOrderRules([]*tfjson.StateModule{{Resources: resources}}, test.scripts)
			require.Error(t, err)
			require.Empty(t, order)
			for _, contains := range test.contains {
				require.ErrorContains(t, err, contains)
			}
		})
	}
}

func TestResolveScriptOrderRulesRejectsMixedModuleExpansion(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			managedScript("coder_script.dependent", "dependent"),
			scriptOrderTestDataSource("data.coder_script_order.order", "order",
				scriptOrderTestRule{run: []string{"coder_script.dependent"}, after: []string{"module.bootstrap"}},
			),
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.bootstrap",
			Resources: []*tfjson.StateResource{
				managedScript("module.bootstrap.coder_script.start", "start"),
				managedScript("module.bootstrap.coder_script.stop", "stop"),
			},
		}},
	}
	scripts := scriptOrderTestScripts("coder_agent.main", ScriptOrderPhaseStartup,
		"coder_script.dependent", "module.bootstrap.coder_script.start")
	scripts["module.bootstrap.coder_script.stop"] = scriptOrderScript{
		RuntimeAddress: "coder_agent.main",
		RunOnStop:      true,
	}

	order, err := resolveScriptOrderRules([]*tfjson.StateModule{module}, scripts)
	require.Error(t, err)
	require.Empty(t, order)
	require.ErrorContains(t, err, "module.bootstrap")
	require.ErrorContains(t, err, "module.bootstrap.coder_script.stop")
	require.ErrorContains(t, err, "startup and shutdown phases cannot be mixed")
}

func TestResolveScriptOrderRulesDataSourceValidation(t *testing.T) {
	t.Parallel()

	t.Run("NoRuleBlocks", func(t *testing.T) {
		t.Parallel()

		order, err := resolveScriptOrderRules([]*tfjson.StateModule{{Resources: []*tfjson.StateResource{
			scriptOrderTestDataSource("data.coder_script_order.order", "order"),
		}}}, nil)
		require.ErrorContains(t, err, "must contain at least one rule")
		require.Empty(t, order)
	})

	t.Run("ManagedPrototypeResourceIsIgnored", func(t *testing.T) {
		t.Parallel()

		order, err := resolveScriptOrderRules([]*tfjson.StateModule{{Resources: []*tfjson.StateResource{{
			Address: "coder_script_order.order",
			Mode:    tfjson.ManagedResourceMode,
			Type:    "coder_script_order",
			Name:    "order",
		}}}}, nil)
		require.NoError(t, err)
		require.Empty(t, order)
	})
}

func TestResolveScriptOrderRulesRelativeToDataSourceModule(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{
		Address: "module.development",
		Resources: []*tfjson.StateResource{
			managedScript("module.development.coder_script.a", "a"),
			managedScript("module.development.coder_script.b", "b"),
			scriptOrderTestDataSource("module.development.data.coder_script_order.order", "order",
				scriptOrderTestRule{run: []string{"coder_script.b"}, after: []string{"coder_script.a"}},
			),
		},
	}
	scripts := scriptOrderTestScripts("module.development.coder_agent.main", ScriptOrderPhaseStartup,
		"module.development.coder_script.a", "module.development.coder_script.b")

	order, err := resolveScriptOrderRules([]*tfjson.StateModule{{ChildModules: []*tfjson.StateModule{module}}}, scripts)
	require.NoError(t, err)
	require.Len(t, order.rules, 1)
	require.Equal(t, []string{"module.development.coder_script.b"}, order.rules[0].run[0].addresses)
	require.Equal(t, []string{"module.development.coder_script.a"}, order.rules[0].after[0].addresses)
}

func scriptOrderTestDataSource(address, name string, rules ...scriptOrderTestRule) *tfjson.StateResource {
	ruleValues := make([]any, 0, len(rules))
	for _, rule := range rules {
		value := map[string]any{
			"run":   rule.run,
			"after": rule.after,
		}
		if rule.requires != "" {
			value["requires"] = rule.requires
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

func scriptOrderTestScripts(runtimeAddress string, phase ScriptOrderPhase, addresses ...string) map[string]scriptOrderScript {
	scripts := make(map[string]scriptOrderScript, len(addresses))
	for _, address := range addresses {
		script := scriptOrderScript{RuntimeAddress: runtimeAddress}
		switch phase {
		case ScriptOrderPhaseStartup:
			script.RunOnStart = true
		case ScriptOrderPhaseShutdown:
			script.RunOnStop = true
		default:
			panic("unknown script order phase")
		}
		scripts[address] = script
	}
	return scripts
}

func scriptOrderTestManagedScripts(names ...string) []*tfjson.StateResource {
	resources := make([]*tfjson.StateResource, 0, len(names))
	for _, name := range names {
		resources = append(resources, managedScript("coder_script."+name, name))
	}
	return resources
}
