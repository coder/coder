package scriptorder

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
)

func scriptOrderProgramForTest(
	modules []*tfjson.StateModule,
	config *tfjson.Config,
) (*Program, error) {
	stateIndex, err := newScriptOrderStateIndex(context.Background(), modules)
	if err != nil {
		return nil, err
	}
	configIndex, err := newScriptOrderConfigIndex(context.Background(), config)
	if err != nil {
		return nil, err
	}
	return &Program{
		stateIndex:  stateIndex,
		configIndex: configIndex,
		dataSources: stateIndex.dataSources,
	}, nil
}

func resolveScriptOrderSelectorForTest(
	modules []*tfjson.StateModule,
	config *tfjson.Config,
	moduleAddress string,
	selector scriptOrderSelector,
) (scriptOrderSelectorResolution, error) {
	stateIndex, err := newScriptOrderStateIndex(context.Background(), modules)
	if err != nil {
		return scriptOrderSelectorResolution{}, err
	}
	configIndex, err := newScriptOrderConfigIndex(context.Background(), config)
	if err != nil {
		return scriptOrderSelectorResolution{}, err
	}
	return resolveScriptOrderSelector(
		stateIndex, configIndex, moduleAddress, selector, nil,
	)
}

func collectScriptOrderRuleDeclarationsForTest(
	modules []*tfjson.StateModule,
	config *tfjson.Config,
) ([]scriptOrderRuleDeclaration, error) {
	program, err := scriptOrderProgramForTest(modules, config)
	if err != nil {
		return nil, err
	}
	return collectScriptOrderRuleDeclarations(program)
}

func prepareScriptOrderForTest(
	modules []*tfjson.StateModule,
	config *tfjson.Config,
	scripts map[string]scriptOrderScript,
) (preparedScriptOrder, error) {
	program, err := scriptOrderProgramForTest(modules, config)
	if err != nil {
		return preparedScriptOrder{}, err
	}
	return prepareScriptOrder(program, scripts)
}

type resolvedScriptOrderForTest struct {
	rules    []resolvedScriptOrderRule
	warnings []scriptOrderPhaseFilterWarning
}

func resolveScriptOrder(
	modules []*tfjson.StateModule,
	config *tfjson.Config,
	scripts map[string]scriptOrderScript,
) (resolvedScriptOrderForTest, error) {
	prepared, err := prepareScriptOrderForTest(modules, config, scripts)
	if err != nil {
		return resolvedScriptOrderForTest{}, err
	}
	resolved, err := finalizeScriptOrder(prepared, scripts)
	if err != nil {
		return resolvedScriptOrderForTest{}, err
	}
	return resolvedScriptOrderForTest{
		rules:    resolved.rules,
		warnings: prepared.warnings,
	}, nil
}
