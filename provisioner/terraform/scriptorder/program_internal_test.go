package scriptorder

import (
	"context"
	"errors"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestCollectScriptOrderRuleDeclarationsDeduplicatesSelectors(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.a", "a"),
		scriptOrderManagedCoderScript("coder_script.b", "b"),
		dataCoderScriptOrder(
			"data.coder_script_order.order", "order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.b", "coder_script.b"},
				After: []string{"coder_script.a", "coder_script.a"},
			},
		),
	}}

	program, err := scriptOrderProgramForTest([]*tfjson.StateModule{module}, nil)
	require.NoError(t, err)
	declarations, err := collectScriptOrderRuleDeclarations(program)
	require.NoError(t, err)
	require.Len(t, declarations, 1)
	require.Len(t, declarations[0].run, 1)
	require.Len(t, declarations[0].after, 1)
}

func TestCollectScriptOrderRuleDeclarationsRejectsExpansionAboveLimit(t *testing.T) {
	t.Parallel()

	root := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{dataCoderScriptOrder(
			"data.coder_script_order.order", "order",
			scriptOrderRuleAttributes{
				Run:   []string{"module.work"},
				After: []string{"module.empty"},
			},
			scriptOrderRuleAttributes{
				Run:   []string{"module.work"},
				After: []string{"module.empty"},
			},
		)},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.work",
			Resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript("module.work.coder_script.a", "a"),
				scriptOrderManagedCoderScript("module.work.coder_script.b", "b"),
			},
		}},
	}
	program, err := scriptOrderProgramForTest(
		[]*tfjson.StateModule{root},
		rootScriptOrderConfigWithModuleCalls("work", "empty"),
	)
	require.NoError(t, err)

	declarations, err := collectScriptOrderRuleDeclarationsWithExpansionLimit(program, 3)
	require.Error(t, err)
	require.Nil(t, declarations)
	require.ErrorContains(t, err, `rule 1`)
	require.ErrorContains(t, err, `resolve run selector "module.work"`)
	require.ErrorContains(t, err, "limited to 3 expanded script addresses in total")
}

func TestScriptOrderStateIndexStoresNestedScriptsOnce(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{ChildModules: []*tfjson.StateModule{{
		Address: "module.outer",
		ChildModules: []*tfjson.StateModule{{
			Address: "module.outer.module.inner",
			Resources: []*tfjson.StateResource{
				scriptOrderManagedCoderScript(
					"module.outer.module.inner.coder_script.setup", "setup",
				),
			},
		}},
	}}}

	index, err := newScriptOrderStateIndex(
		context.Background(), []*tfjson.StateModule{module},
	)
	require.NoError(t, err)
	require.Len(t, index.scripts, 1)
	require.Equal(t, []scriptOrderScriptSpan{{start: 0, end: 1}},
		index.scriptsByModule[scriptOrderModuleCallKey{moduleName: "outer"}],
	)

	selector, err := parseScriptOrderSelector("module.outer")
	require.NoError(t, err)
	configIndex, err := newScriptOrderConfigIndex(
		context.Background(), rootScriptOrderConfigWithModuleCalls("outer"),
	)
	require.NoError(t, err)
	resolved, err := resolveScriptOrderSelector(
		index, configIndex, "", selector, nil,
	)
	require.NoError(t, err)
	require.Equal(t, []string{
		"module.outer.module.inner.coder_script.setup",
	}, resolved.addresses)
}

func TestScriptOrderIndexesHonorCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newScriptOrderStateIndex(ctx, []*tfjson.StateModule{{}})
	require.ErrorIs(t, err, context.Canceled)
	_, err = newScriptOrderConfigIndex(
		ctx, &tfjson.Config{RootModule: &tfjson.ConfigModule{}},
	)
	require.ErrorIs(t, err, context.Canceled)
}

func TestNewScriptOrderProgramFiltersUndeclaredDataSources(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		dataCoderScriptOrder("data.coder_script_order.active", "active"),
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
	require.Equal(t, "data.coder_script_order.active", program.dataSources[0].address)
}

func TestScriptOrderRuleErrorBoundsDiagnostic(t *testing.T) {
	t.Parallel()

	longDetail := strings.Repeat("a", maxScriptOrderRuleDiagnosticRunes+1)
	cause := errors.New(longDetail)
	err := scriptOrderRuleError("data.coder_script_order.order", 0, cause)

	require.ErrorIs(t, err, cause)
	require.NotContains(t, err.Error(), longDetail)
	require.Less(t, len(err.Error()), maxScriptOrderRuleDiagnosticRunes+1024)
}

func TestPrepareScriptOrderDefersRuntimeValidation(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.dependent", "dependent"),
		scriptOrderManagedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order", "order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.dependent"},
				After: []string{"coder_script.prerequisite"},
			},
		),
	}}
	scripts := scriptOrderTestScripts(
		"", ScriptOrderPhaseStart,
		"coder_script.dependent", "coder_script.prerequisite",
	)
	program, err := scriptOrderProgramForTest([]*tfjson.StateModule{module}, nil)
	require.NoError(t, err)

	prepared, err := prepareScriptOrder(program, scripts)
	require.NoError(t, err)
	require.Equal(t, []string{
		"coder_script.dependent", "coder_script.prerequisite",
	}, prepared.selectedScriptAddresses)

	_, err = finalizeScriptOrder(prepared, scripts)
	require.ErrorContains(
		t, err, "could not be associated with an agent or devcontainer subagent",
	)
}

func TestPreparedSelectedScriptAddressesReturnsCopy(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		scriptOrderManagedCoderScript("coder_script.dependent", "dependent"),
		scriptOrderManagedCoderScript("coder_script.prerequisite", "prerequisite"),
		dataCoderScriptOrder(
			"data.coder_script_order.order", "order",
			scriptOrderRuleAttributes{
				Run:   []string{"coder_script.dependent"},
				After: []string{"coder_script.prerequisite"},
			},
		),
	}}
	scripts := map[string]Script{
		"coder_script.dependent": {
			RuntimeAddress: "coder_agent.main",
			RunOnStart:     true,
		},
		"coder_script.prerequisite": {
			RuntimeAddress: "coder_agent.main",
			RunOnStart:     true,
		},
	}
	program, err := scriptOrderProgramForTest([]*tfjson.StateModule{module}, nil)
	require.NoError(t, err)
	prepared, err := program.Prepare(scripts)
	require.NoError(t, err)

	addresses := prepared.SelectedScriptAddresses()
	addresses[0] = "changed"
	require.Equal(t, []string{
		"coder_script.dependent", "coder_script.prerequisite",
	}, prepared.SelectedScriptAddresses())
}
