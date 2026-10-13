package scriptorder

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
)

func programForTest(
	modules []*tfjson.StateModule,
	config *tfjson.Config,
) (*Program, error) {
	stateIndex, err := newStateIndex(context.Background(), modules)
	if err != nil {
		return nil, err
	}
	var configIndex *configIndex
	if config != nil && config.RootModule != nil {
		configIndex, err = newConfigIndex(context.Background(), config)
		if err != nil {
			return nil, err
		}
	}
	return &Program{
		stateIndex:  stateIndex,
		configIndex: configIndex,
		dataSources: stateIndex.dataSources,
	}, nil
}

func resolveSelectorForTest(
	modules []*tfjson.StateModule,
	config *tfjson.Config,
	moduleAddress string,
	selector selector,
) (selectorResolution, error) {
	stateIndex, err := newStateIndex(context.Background(), modules)
	if err != nil {
		return selectorResolution{}, err
	}
	var configIndex *configIndex
	if config != nil && config.RootModule != nil {
		configIndex, err = newConfigIndex(context.Background(), config)
		if err != nil {
			return selectorResolution{}, err
		}
	}
	program := &Program{stateIndex: stateIndex, configIndex: configIndex}
	return program.resolveSelector(moduleAddress, selector, nil)
}

func collectRuleDeclarationsForTest(
	modules []*tfjson.StateModule,
	config *tfjson.Config,
) ([]ruleDeclaration, error) {
	program, err := programForTest(modules, config)
	if err != nil {
		return nil, err
	}
	return program.collectRuleDeclarations()
}

func prepareOrderForTest(
	modules []*tfjson.StateModule,
	config *tfjson.Config,
	scripts map[string]script,
) (Prepared, error) {
	program, err := programForTest(modules, config)
	if err != nil {
		return Prepared{}, err
	}
	program.stateIndex.scriptLifecycles = testScriptLifecycles(scripts)
	return program.prepareOrder()
}

type resolvedOrderForTest struct {
	rules    []resolvedRule
	warnings []phaseFilterWarning
}

func resolveOrder(
	modules []*tfjson.StateModule,
	config *tfjson.Config,
	scripts map[string]script,
) (resolvedOrderForTest, error) {
	prepared, err := prepareOrderForTest(modules, config, scripts)
	if err != nil {
		return resolvedOrderForTest{}, err
	}
	resolved, err := prepared.finalizeOrder(testRuntimeBindings(scripts))
	if err != nil {
		return resolvedOrderForTest{}, err
	}
	return resolvedOrderForTest{
		rules:    resolved,
		warnings: prepared.warnings,
	}, nil
}

type script struct {
	runtimeAddress string
	runOnStart     bool
	runOnStop      bool
	hasCron        bool
}

func testScriptLifecycles(
	scripts map[string]script,
) map[string]scriptLifecycle {
	result := make(map[string]scriptLifecycle, len(scripts))
	for address, input := range scripts {
		result[address] = scriptLifecycle{
			runOnStart: input.runOnStart,
			runOnStop:  input.runOnStop,
			hasCron:    input.hasCron,
		}
	}
	return result
}

func testRuntimeBindings(
	scripts map[string]script,
) map[string]RuntimeBinding {
	result := make(map[string]RuntimeBinding, len(scripts))
	for address, input := range scripts {
		result[address] = RuntimeBinding{
			RuntimeAddress: input.runtimeAddress,
		}
	}
	return result
}
