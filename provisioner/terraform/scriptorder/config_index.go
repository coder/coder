package scriptorder

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"
)

type scriptOrderConfigResourceKey struct {
	moduleAddress string
	mode          tfjson.ResourceMode
	resourceType  string
	resourceName  string
}

// scriptOrderConfigIndex maps evaluated module instances to shared
// configuration declarations. Instance keys are omitted from its keys.
type scriptOrderConfigIndex struct {
	moduleCalls map[scriptOrderModuleCallKey]struct{}
	resources   map[scriptOrderConfigResourceKey]*tfjson.ConfigResource
}

func newScriptOrderConfigIndex(
	ctx context.Context,
	config *tfjson.Config,
) (*scriptOrderConfigIndex, error) {
	if config == nil || config.RootModule == nil {
		return nil, nil //nolint:nilnil // Missing configuration is valid here.
	}
	index := &scriptOrderConfigIndex{
		moduleCalls: map[scriptOrderModuleCallKey]struct{}{},
		resources:   map[scriptOrderConfigResourceKey]*tfjson.ConfigResource{},
	}
	if err := index.indexModule(ctx, "", config.RootModule); err != nil {
		return nil, err
	}
	return index, nil
}

func (i *scriptOrderConfigIndex) indexModule(
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
		key := scriptOrderConfigResourceKey{
			moduleAddress: moduleAddress,
			mode:          resource.Mode,
			resourceType:  resource.Type,
			resourceName:  resource.Name,
		}
		if i.resources[key] == nil {
			i.resources[key] = resource
		}
	}
	for name, call := range module.ModuleCalls {
		if err := ctx.Err(); err != nil {
			return err
		}
		if call == nil {
			continue
		}
		i.moduleCalls[scriptOrderModuleCallKey{
			moduleAddress: moduleAddress,
			moduleName:    name,
		}] = struct{}{}
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

func scriptOrderConfigurationModuleAddress(evaluatedAddress string) (string, error) {
	modulePath, err := parseStateModuleAddress(evaluatedAddress)
	if err != nil {
		return "", err
	}
	return modulePath.ConfigurationAddress(), nil
}

func (i *scriptOrderConfigIndex) moduleCallDeclared(
	evaluatedModuleAddress string,
	name string,
) (bool, error) {
	if i == nil {
		return false, xerrors.New("terraform plan configuration is required to resolve a module selector")
	}
	moduleAddress, err := scriptOrderConfigurationModuleAddress(evaluatedModuleAddress)
	if err != nil {
		return false, err
	}
	_, declared := i.moduleCalls[scriptOrderModuleCallKey{
		moduleAddress: moduleAddress,
		moduleName:    name,
	}]
	return declared, nil
}

func (i *scriptOrderConfigIndex) resource(
	evaluatedModuleAddress string,
	mode tfjson.ResourceMode,
	resourceType string,
	name string,
) (*tfjson.ConfigResource, error) {
	moduleAddress, err := scriptOrderConfigurationModuleAddress(evaluatedModuleAddress)
	if err != nil {
		return nil, err
	}
	return i.resources[scriptOrderConfigResourceKey{
		moduleAddress: moduleAddress,
		mode:          mode,
		resourceType:  resourceType,
		resourceName:  name,
	}], nil
}

func (i *scriptOrderConfigIndex) managedResourceDeclared(
	evaluatedModuleAddress string,
	resourceType string,
	name string,
) (bool, error) {
	if i == nil {
		return false, xerrors.New(
			"cannot validate empty coder_script selector because Terraform plan configuration is unavailable",
		)
	}
	resource, err := i.resource(
		evaluatedModuleAddress, tfjson.ManagedResourceMode, resourceType, name,
	)
	return resource != nil, err
}

func (i *scriptOrderConfigIndex) dataResourceDeclared(
	evaluatedModuleAddress string,
	resourceType string,
	name string,
) (bool, error) {
	if i == nil {
		return false, xerrors.New("Terraform plan configuration is unavailable")
	}
	resource, err := i.resource(
		evaluatedModuleAddress, tfjson.DataResourceMode, resourceType, name,
	)
	return resource != nil, err
}
