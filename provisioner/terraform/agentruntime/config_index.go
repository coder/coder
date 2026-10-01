package agentruntime

import (
	"context"
	"slices"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

type configResourceKey struct {
	moduleAddress string
	mode          tfjson.ResourceMode
	resourceType  string
	resourceName  string
}

type configResource struct {
	agentIDReferences []string
	forEachReferences []string
}

// configIndex maps evaluated module instances to shared configuration
// declarations. Instance keys are omitted from its keys.
type configIndex struct {
	resources map[configResourceKey]configResource
}

func newConfigIndex(
	ctx context.Context,
	config *tfjson.Config,
) (*configIndex, error) {
	if config == nil || config.RootModule == nil {
		return nil, nil
	}
	index := &configIndex{
		resources: map[configResourceKey]configResource{},
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
		if call == nil || call.Module == nil {
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
