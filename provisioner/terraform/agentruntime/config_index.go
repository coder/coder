package agentruntime

import (
	"context"
	"slices"

	"github.com/hashicorp/hcl/v2"
	tfjson "github.com/hashicorp/terraform-json"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

type configResourceKey struct {
	moduleAddress string
	mode          tfjson.ResourceMode
	resourceType  string
	resourceName  string
}

type moduleCallKey struct {
	moduleAddress string
	moduleName    string
}

type configModuleCall struct {
	source            string
	inputReferences   map[string][]string
	forEachReferences []string
}

type configModule struct {
	outputReferences map[string][]string
}

type configResource struct {
	agentIDReferences []string
	forEachReferences []string
}

// configIndex maps evaluated module instances to shared configuration
// declarations. Instance keys are omitted from its keys.
type configIndex struct {
	modules                     map[string]configModule
	moduleCalls                 map[moduleCallKey]configModuleCall
	resources                   map[configResourceKey]configResource
	runtimeSourceExpressions    map[runtimeExpressionKey]hcl.Expression
	localRuntime                map[runtimeLocalKey][]runtimeLocalReference
	runtimeSourceIndexed        bool
	runtimeIdentityExpressions  map[runtimeExpressionKey]bool
	runtimeEachValueExpressions map[runtimeExpressionKey]bool
	runtimeResultSuffixes       map[runtimeExpressionKey]string
	runtimeValueReferences      map[runtimeExpressionKey][]string
}

func newConfigIndex(
	ctx context.Context,
	config *tfjson.Config,
) (*configIndex, error) {
	if config == nil || config.RootModule == nil {
		return nil, nil //nolint:nilnil // Missing configuration is valid here.
	}
	index := &configIndex{
		modules:                     map[string]configModule{},
		moduleCalls:                 map[moduleCallKey]configModuleCall{},
		resources:                   map[configResourceKey]configResource{},
		runtimeSourceExpressions:    map[runtimeExpressionKey]hcl.Expression{},
		localRuntime:                map[runtimeLocalKey][]runtimeLocalReference{},
		runtimeIdentityExpressions:  map[runtimeExpressionKey]bool{},
		runtimeEachValueExpressions: map[runtimeExpressionKey]bool{},
		runtimeResultSuffixes:       map[runtimeExpressionKey]string{},
		runtimeValueReferences:      map[runtimeExpressionKey][]string{},
	}
	if err := index.indexModule(ctx, "", config.RootModule); err != nil {
		return nil, err
	}
	return index, nil
}

func (i *configIndex) indexModule(
	ctx context.Context,
	moduleAddress string,
	module *tfjson.ConfigModule,
) error {
	if module == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	outputReferences := make(map[string][]string, len(module.Outputs))
	for name, output := range module.Outputs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if output == nil || output.Expression == nil {
			continue
		}
		outputReferences[name] = configExpressionReferences(output.Expression)
	}
	i.modules[moduleAddress] = configModule{
		outputReferences: outputReferences,
	}
	for _, resource := range module.Resources {
		if err := ctx.Err(); err != nil {
			return err
		}
		if resource == nil {
			continue
		}
		key := configResourceKey{
			moduleAddress: moduleAddress,
			mode:          resource.Mode,
			resourceType:  resource.Type,
			resourceName:  resource.Name,
		}
		if _, exists := i.resources[key]; exists {
			continue
		}
		var agentIDReferences []string
		agentID := resource.Expressions["agent_id"]
		if agentID != nil && agentID.ExpressionData != nil {
			agentIDReferences = slices.Clone(agentID.References)
		}
		var forEachReferences []string
		if resource.ForEachExpression != nil &&
			resource.ForEachExpression.ExpressionData != nil {
			forEachReferences = slices.Clone(
				resource.ForEachExpression.References,
			)
		}
		i.resources[key] = configResource{
			agentIDReferences: agentIDReferences,
			forEachReferences: forEachReferences,
		}
	}
	for name, call := range module.ModuleCalls {
		if err := ctx.Err(); err != nil {
			return err
		}
		if call == nil {
			continue
		}
		inputReferences := make(
			map[string][]string, len(call.Expressions),
		)
		for inputName, expression := range call.Expressions {
			if err := ctx.Err(); err != nil {
				return err
			}
			if expression == nil {
				continue
			}
			inputReferences[inputName] = configExpressionReferences(expression)
		}
		var forEachReferences []string
		if call.ForEachExpression != nil &&
			call.ForEachExpression.ExpressionData != nil {
			forEachReferences = slices.Clone(
				call.ForEachExpression.References,
			)
		}
		i.moduleCalls[moduleCallKey{
			moduleAddress: moduleAddress,
			moduleName:    name,
		}] = configModuleCall{
			source:            call.Source,
			inputReferences:   inputReferences,
			forEachReferences: forEachReferences,
		}
		if call.Module == nil {
			continue
		}
		childAddress := "module." + name
		if moduleAddress != "" {
			childAddress = moduleAddress + "." + childAddress
		}
		if err := i.indexModule(ctx, childAddress, call.Module); err != nil {
			return err
		}
	}
	return nil
}

func configExpressionReferences(expression *tfjson.Expression) []string {
	if expression == nil || expression.ExpressionData == nil {
		return nil
	}
	return slices.Clone(expression.References)
}

func configurationModuleAddress(evaluatedAddress string) (string, error) {
	modulePath, err := tfaddr.ParseModulePath(evaluatedAddress)
	if err != nil {
		return "", err
	}
	return modulePath.ConfigurationAddress(), nil
}

func (i *configIndex) resource(
	evaluatedModuleAddress string,
	mode tfjson.ResourceMode,
	resourceType string,
	name string,
) (configResource, bool, error) {
	moduleAddress, err := configurationModuleAddress(evaluatedModuleAddress)
	if err != nil {
		return configResource{}, false, err
	}
	resource, ok := i.resources[configResourceKey{
		moduleAddress: moduleAddress,
		mode:          mode,
		resourceType:  resourceType,
		resourceName:  name,
	}]
	return resource, ok, nil
}
