package scriptorder

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestCollectRuleDeclarationsDeduplicatesSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		managedCoderScript("coder_script.a", "a"),
		managedCoderScript("coder_script.b", "b"),
		dataCoderScriptOrder(
			"data.coder_script_order.order", "order",
			ruleAttributes{
				Run:   []string{"coder_script.b", "coder_script.b"},
				After: []string{"coder_script.a", "coder_script.a"},
			},
		),
	}}

	program, err := programForTest([]*tfjson.StateModule{module}, nil)
	require.NoError(t, err)
	declarations, err := program.collectRuleDeclarations()
	require.NoError(t, err)
	require.Len(t, declarations, 1)
	require.Len(t, declarations[0].run, 1)
	require.Len(t, declarations[0].after, 1)
}

func TestCollectRuleDeclarationsRejectsExpansionAboveLimit(t *testing.T) {
	t.Parallel()

	root := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{dataCoderScriptOrder(
			"data.coder_script_order.order", "order",
			ruleAttributes{
				Run:   []string{"module.work"},
				After: []string{"module.empty"},
			},
			ruleAttributes{
				Run:   []string{"module.work"},
				After: []string{"module.empty"},
			},
		)},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.work",
			Resources: []*tfjson.StateResource{
				managedCoderScript("module.work.coder_script.a", "a"),
				managedCoderScript("module.work.coder_script.b", "b"),
			},
		}},
	}
	program, err := programForTest(
		[]*tfjson.StateModule{root},
		rootConfigWithModuleCalls("work", "empty"),
	)
	require.NoError(t, err)

	declarations, err := program.collectRuleDeclarationsWithExpansionLimit(3)
	require.Error(t, err)
	require.Nil(t, declarations)
	require.ErrorContains(t, err, `rule 1`)
	require.ErrorContains(t, err, `resolve run selector "module.work"`)
	require.ErrorContains(t, err, "limited to 3 expanded script addresses in total")
}

func TestStateIndexStoresNestedScriptsOnce(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{ChildModules: []*tfjson.StateModule{{
		Address: "module.outer",
		ChildModules: []*tfjson.StateModule{{
			Address: "module.outer.module.inner",
			Resources: []*tfjson.StateResource{
				managedCoderScript(
					"module.outer.module.inner.coder_script.setup", "setup",
				),
			},
		}},
	}}}

	index, err := newStateIndex(
		context.Background(), []*tfjson.StateModule{module},
	)
	require.NoError(t, err)
	require.Len(t, index.scripts, 1)
	require.Equal(t, []scriptSpan{{start: 0, end: 1}},
		index.scriptsByModule[moduleCallKey{moduleName: "outer"}],
	)

	selector, err := parseSelector("module.outer")
	require.NoError(t, err)
	configIndex, err := newConfigIndex(
		context.Background(), rootConfigWithModuleCalls("outer"),
	)
	require.NoError(t, err)
	program := &Program{stateIndex: index, configIndex: configIndex}
	resolved, err := program.resolveSelector("", selector, nil)
	require.NoError(t, err)
	require.Equal(t, []string{
		"module.outer.module.inner.coder_script.setup",
	}, resolved.addresses)
}

func TestNewProgramStoresScriptLifecycle(t *testing.T) {
	t.Parallel()

	start := managedCoderScript("coder_script.start", "start")
	start.AttributeValues = map[string]any{"run_on_start": true}
	stop := managedCoderScript("coder_script.stop", "stop")
	stop.AttributeValues = map[string]any{"run_on_stop": true}
	cron := managedCoderScript("coder_script.cron", "cron")
	cron.AttributeValues = map[string]any{"cron": "0 * * * *"}
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		start,
		stop,
		cron,
		dataCoderScriptOrder("data.coder_script_order.order", "order"),
	}}
	config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
		Resources: []*tfjson.ConfigResource{{
			Address: "data.coder_script_order.order",
			Mode:    tfjson.DataResourceMode,
			Type:    coderScriptOrderResourceType,
			Name:    "order",
		}},
	}}

	program, err := NewProgram(t.Context(), []*tfjson.StateModule{module}, config)
	require.NoError(t, err)
	require.Equal(t, map[string]scriptLifecycle{
		"coder_script.start": {runOnStart: true},
		"coder_script.stop":  {runOnStop: true},
		"coder_script.cron":  {hasCron: true},
	}, program.stateIndex.scriptLifecycles)
}

func TestIndexesHonorCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newStateIndex(ctx, []*tfjson.StateModule{{}})
	require.ErrorIs(t, err, context.Canceled)
	_, err = newConfigIndex(
		ctx, &tfjson.Config{RootModule: &tfjson.ConfigModule{}},
	)
	require.ErrorIs(t, err, context.Canceled)
}

func TestConfigIndexOnlyStoresScriptOrderResources(t *testing.T) {
	t.Parallel()

	resources := []*tfjson.ConfigResource{
		{Mode: tfjson.ManagedResourceMode, Type: "coder_script", Name: "script"},
		{Mode: tfjson.DataResourceMode, Type: "coder_script_order", Name: "order"},
		{Mode: tfjson.ManagedResourceMode, Type: "coder_script_order", Name: "wrong_mode"},
		{Mode: tfjson.DataResourceMode, Type: "coder_script", Name: "wrong_mode"},
		{Mode: tfjson.ManagedResourceMode, Type: "null_resource", Name: "unrelated"},
		{Mode: tfjson.DataResourceMode, Type: "terraform_remote_state", Name: "unrelated"},
	}
	index, err := newConfigIndex(t.Context(), &tfjson.Config{
		RootModule: &tfjson.ConfigModule{Resources: resources},
	})
	require.NoError(t, err)
	require.Equal(t, map[configResourceKey]*tfjson.ConfigResource{
		{
			mode:         tfjson.ManagedResourceMode,
			resourceType: "coder_script",
			resourceName: "script",
		}: resources[0],
		{
			mode:         tfjson.DataResourceMode,
			resourceType: "coder_script_order",
			resourceName: "order",
		}: resources[1],
	}, index.resources)
}

func TestNewProgramRequiresConfiguration(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dataCoderScriptOrder("data.coder_script_order.order", "order"),
	}}
	tests := []struct {
		name   string
		config *tfjson.Config
	}{
		{name: "NilConfig"},
		{name: "NilRootModule", config: &tfjson.Config{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			program, err := NewProgram(
				t.Context(), []*tfjson.StateModule{module}, test.config,
			)
			require.Nil(t, program)
			require.EqualError(t, err, "Terraform plan configuration is unavailable")
		})
	}
}

func TestNewProgramFiltersStaleDataSources(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dataCoderScriptOrder("data.coder_script_order.active[0]", "active"),
		dataCoderScriptOrder("data.coder_script_order.stale", "stale"),
	}}
	config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
		Resources: []*tfjson.ConfigResource{{
			Address: "data.coder_script_order.active",
			Mode:    tfjson.DataResourceMode,
			Type:    "coder_script_order",
			Name:    "active",
		}},
	}}

	program, err := NewProgram(
		context.Background(), []*tfjson.StateModule{module}, config,
	)
	require.NoError(t, err)
	require.Len(t, program.dataSources, 1)
	require.Equal(t, "data.coder_script_order.active[0]", program.dataSources[0].resource.Address)
}

func TestProgramFilterDataSources(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dataCoderScriptOrder("data.coder_script_order.order[0]", "order"),
		dataCoderScriptOrder("data.coder_script_order.order[1]", "order"),
	}}
	program, err := programForTest(
		[]*tfjson.StateModule{module},
		&tfjson.Config{RootModule: &tfjson.ConfigModule{}},
	)
	require.NoError(t, err)
	require.Equal(t, []string{
		"data.coder_script_order.order[0]",
		"data.coder_script_order.order[1]",
	}, program.DataSourceAddresses())

	filtered := program.FilterDataSources(func(address string) bool {
		return address == "data.coder_script_order.order[1]"
	})
	require.NotSame(t, program, filtered)
	require.Same(t, program.stateIndex, filtered.stateIndex)
	require.Same(t, program.configIndex, filtered.configIndex)
	require.Equal(t, []string{
		"data.coder_script_order.order[1]",
	}, filtered.DataSourceAddresses())
	require.Len(t, program.dataSources, 2)

	require.Same(t, program, program.FilterDataSources(func(string) bool {
		return true
	}))
	require.Nil(t, program.FilterDataSources(func(string) bool {
		return false
	}))
}

func TestProgramPrepareAfterFiltering(t *testing.T) {
	t.Parallel()

	dependent := managedCoderScript("coder_script.dependent", "dependent")
	dependent.AttributeValues = map[string]any{"run_on_start": true}
	prerequisite := managedCoderScript("coder_script.prerequisite", "prerequisite")
	prerequisite.AttributeValues = map[string]any{"run_on_start": true}
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dependent,
		prerequisite,
		dataCoderScriptOrder("data.coder_script_order.bad", "bad"),
		dataCoderScriptOrder(
			"data.coder_script_order.good", "good",
			ruleAttributes{
				Run:   []string{"coder_script.dependent"},
				After: []string{"coder_script.prerequisite"},
			},
		),
	}}
	program, err := programForTest([]*tfjson.StateModule{module}, nil)
	require.NoError(t, err)

	prepared, err := program.Prepare()
	require.Nil(t, prepared)
	require.ErrorContains(t, err, `data source "data.coder_script_order.bad"`)

	program = program.FilterDataSources(func(address string) bool {
		return address == "data.coder_script_order.good"
	})
	prepared, err = program.Prepare()
	require.NoError(t, err)
	require.Equal(t, []string{
		"coder_script.dependent",
		"coder_script.prerequisite",
	}, prepared.SelectedScriptAddresses())
}

func TestRuleErrorBoundsDiagnostic(t *testing.T) {
	t.Parallel()

	longDetail := strings.Repeat("a", maxRuleDiagnosticRunes+1)
	cause := xerrors.New(longDetail)
	err := ruleError("data.coder_script_order.order", 0, cause)

	require.ErrorIs(t, err, cause)
	require.NotContains(t, err.Error(), longDetail)
	require.Less(t, len(err.Error()), maxRuleDiagnosticRunes+1024)
}

func TestPrepareOrderDefersRuntimeValidationUntilFinalize(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		managedCoderScript("coder_script.dependent", "dependent"),
		managedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order", "order",
			ruleAttributes{
				Run:   []string{"coder_script.dependent"},
				After: []string{"coder_script.prerequisite"},
			},
		),
	}}
	scripts := testScripts(
		"", PhaseStart,
		"coder_script.dependent", "coder_script.prerequisite",
	)
	program, err := programForTest([]*tfjson.StateModule{module}, nil)
	require.NoError(t, err)

	program.stateIndex.scriptLifecycles = testScriptLifecycles(scripts)
	prepared, err := program.Prepare()
	require.NoError(t, err)
	require.Equal(t, []string{
		"coder_script.dependent", "coder_script.prerequisite",
	}, prepared.SelectedScriptAddresses())

	_, err = prepared.Finalize(nil)
	require.ErrorContains(
		t, err, "could not be associated with an agent or devcontainer subagent",
	)

	_, err = prepared.Finalize(map[string]RuntimeBinding{
		"coder_script.dependent": {
			Error: "agent_id does not match any workspace agent",
		},
	})
	require.ErrorContains(
		t, err,
		`run selector "coder_script.dependent" selects script "coder_script.dependent"`,
	)
	require.ErrorContains(
		t, err,
		"its agent runtime could not be resolved: agent_id does not match any workspace agent",
	)
}

func TestBoundedPhaseFilterWarningSelectors(t *testing.T) {
	t.Parallel()

	selectors := make([]string, 0, maxPhaseFilterWarningSelectors+2)
	for i := range maxPhaseFilterWarningSelectors + 1 {
		selectors = append(selectors, fmt.Sprintf("module.%02d", i))
	}
	bounded, omitted := boundedPhaseFilterWarningSelectors(
		selectors, []string{selectors[0]},
	)
	require.Len(t, bounded, maxPhaseFilterWarningSelectors)
	require.Equal(t, 1, omitted)
	require.Equal(t, "module.00", bounded[0])
	require.Equal(t, "module.19", bounded[len(bounded)-1])

	warning := phaseFilterWarning{
		dataSourceAddress:                "data.coder_script_order.order",
		inferredPhase:                    PhaseStart,
		moduleSelectorsWithOmissions:     bounded,
		additionalModuleSelectorsOmitted: omitted,
	}
	require.Contains(t, warning.String(), "… (1 more)")
}

func TestPrepareOrderBoundsPhaseFilterWarnings(t *testing.T) {
	t.Parallel()

	rules := make([]ruleAttributes, MaxWarnings+1)
	for i := range rules {
		rules[i] = ruleAttributes{
			Run:   []string{"coder_script.dependent"},
			After: []string{"module.alpha", "module.beta"},
		}
	}
	module := moduleFilteringFixture("")
	module.Resources[1] = dataCoderScriptOrder(
		"data.coder_script_order.order", "order", rules...,
	)
	scripts := testScripts(
		"", PhaseStart,
		"coder_script.dependent",
		"module.alpha.coder_script.start",
		"module.beta.coder_script.start",
	)
	scripts["module.alpha.coder_script.stop"] = script{runOnStop: true}
	scripts["module.beta.coder_script.stop"] = script{runOnStop: true}

	prepared, err := prepareOrderForTest(
		[]*tfjson.StateModule{module},
		rootConfigWithModuleCalls("alpha", "beta"),
		scripts,
	)
	require.NoError(t, err)
	require.Len(t, prepared.Warnings(), MaxWarnings)
	require.Equal(t, 1, prepared.WarningsOmitted())
}

func TestPreparedValidateForPlan(t *testing.T) {
	t.Parallel()

	selector := func(field, address string) []resolvedSelector {
		return []resolvedSelector{{
			field:     field,
			raw:       address,
			kind:      selectorScript,
			addresses: []string{address},
		}}
	}
	rule := func(index int, run, after string, requirement Requirement) preparedRule {
		return preparedRule{
			identity: ruleIdentity{
				dataSourceAddress: "data.coder_script_order.order",
				ruleIndex:         index,
			},
			phase:       PhaseStart,
			requirement: requirement,
			run:         selector("run", run),
			after:       selector("after", after),
		}
	}

	t.Run("DefersAgentBindingsUntilFinalize", func(t *testing.T) {
		t.Parallel()

		prepared := &Prepared{rules: []preparedRule{
			rule(0, "coder_script.dependent", "coder_script.prerequisite", RequirementSuccess),
		}}
		require.NoError(t, prepared.ValidateForPlan())
		_, err := prepared.Finalize(nil)
		require.ErrorContains(t, err, "could not be associated with an agent or devcontainer subagent")
	})

	t.Run("SelfDependency", func(t *testing.T) {
		t.Parallel()

		prepared := &Prepared{rules: []preparedRule{
			rule(0, "coder_script.same", "coder_script.same", RequirementSuccess),
		}}
		err := prepared.ValidateForPlan()
		require.ErrorContains(t, err, "a script cannot depend on itself")
	})

	t.Run("ConflictingRequirements", func(t *testing.T) {
		t.Parallel()

		prepared := &Prepared{rules: []preparedRule{
			rule(0, "coder_script.dependent", "coder_script.prerequisite", RequirementSuccess),
			rule(1, "coder_script.dependent", "coder_script.prerequisite", RequirementCompletion),
		}}
		err := prepared.ValidateForPlan()
		require.ErrorContains(t, err, "conflicting with requires")
	})

	t.Run("Cycle", func(t *testing.T) {
		t.Parallel()

		prepared := &Prepared{rules: []preparedRule{
			rule(0, "coder_script.a", "coder_script.b", RequirementSuccess),
			rule(1, "coder_script.b", "coder_script.a", RequirementSuccess),
		}}
		err := prepared.ValidateForPlan()
		require.ErrorContains(t, err, "dependency cycle")
	})

	t.Run("Nil", func(t *testing.T) {
		t.Parallel()

		var prepared *Prepared
		require.NoError(t, prepared.ValidateForPlan())
	})
}

func TestPreparedSelectedScriptAddressesReturnsCopy(t *testing.T) {
	t.Parallel()

	dependent := managedCoderScript("coder_script.dependent", "dependent")
	dependent.AttributeValues = map[string]any{"run_on_start": true}
	prerequisite := managedCoderScript("coder_script.prerequisite", "prerequisite")
	prerequisite.AttributeValues = map[string]any{"run_on_start": true}
	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dependent,
		prerequisite,
		dataCoderScriptOrder(
			"data.coder_script_order.order", "order",
			ruleAttributes{
				Run:   []string{"coder_script.dependent"},
				After: []string{"coder_script.prerequisite"},
			},
		),
	}}
	program, err := programForTest([]*tfjson.StateModule{module}, nil)
	require.NoError(t, err)
	prepared, err := program.Prepare()
	require.NoError(t, err)

	addresses := prepared.SelectedScriptAddresses()
	addresses[0] = "changed"
	require.Equal(t, []string{
		"coder_script.dependent", "coder_script.prerequisite",
	}, prepared.SelectedScriptAddresses())

	order, err := prepared.Finalize(map[string]RuntimeBinding{
		"coder_script.dependent": {
			RuntimeAddress: "coder_agent.main",
		},
		"coder_script.prerequisite": {
			RuntimeAddress: "coder_agent.main",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []Graph{{
		RuntimeAddress: "coder_agent.main",
		Phase:          PhaseStart,
		Dependencies: []Dependency{{
			DependentAddress:    "coder_script.dependent",
			PrerequisiteAddress: "coder_script.prerequisite",
			Requirement:         RequirementSuccess,
		}},
	}}, order.Graphs)
}
