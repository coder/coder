package scriptorder

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"
)

// configResourceKey uniquely identifies a resource declaration within
// a configuration module.
type configResourceKey struct {
	moduleAddress string
	mode          tfjson.ResourceMode
	resourceType  string
	resourceName  string
}

// configIndex maps evaluated module instances to shared
// configuration declarations. Instance keys are omitted from its keys.
type configIndex struct {
	// Inncludes calls with no script instances, so selector resolution can
	// distinguish an empty declared call from an undeclared call.
	moduleCalls map[moduleCallKey]struct{}
	resources   map[configResourceKey]*tfjson.ConfigResource
}

func newConfigIndex(
	ctx context.Context,
	config *tfjson.Config,
) (*configIndex, error) {
	if config == nil || config.RootModule == nil {
		return nil, xerrors.New("Terraform plan configuration is unavailable")
	}
	idx := &configIndex{
		moduleCalls: map[moduleCallKey]struct{}{},
		resources:   map[configResourceKey]*tfjson.ConfigResource{},
	}
	if err := idx.indexModule(ctx, "", config.RootModule); err != nil {
		return nil, err
	}
	return idx, nil
}

func (idx *configIndex) indexModule(
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
		isScript := resource.Mode == tfjson.ManagedResourceMode &&
			resource.Type == coderScriptResourceType
		isScriptOrder := resource.Mode == tfjson.DataResourceMode &&
			resource.Type == coderScriptOrderResourceType
		if !isScript && !isScriptOrder {
			continue
		}
		key := configResourceKey{
			moduleAddress: moduleAddress,
			mode:          resource.Mode,
			resourceType:  resource.Type,
			resourceName:  resource.Name,
		}
		if idx.resources[key] == nil {
			idx.resources[key] = resource
		}
	}
	for name, call := range module.ModuleCalls {
		if err := ctx.Err(); err != nil {
			return err
		}
		if call == nil {
			continue
		}
		idx.moduleCalls[moduleCallKey{
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
		if err := idx.indexModule(ctx, childAddress, call.Module); err != nil {
			return err
		}
	}
	return nil
}

func configurationModuleAddress(evaluatedAddress string) (string, error) {
	modulePath, err := parseStateModuleAddress(evaluatedAddress)
	if err != nil {
		return "", err
	}
	return modulePath.ConfigurationAddress(), nil
}

func (idx *configIndex) moduleCallDeclared(
	evaluatedModuleAddress string,
	name string,
) (bool, error) {
	if idx == nil {
		return false, xerrors.New("terraform plan configuration is required to resolve a module selector")
	}
	moduleAddress, err := configurationModuleAddress(evaluatedModuleAddress)
	if err != nil {
		return false, err
	}
	_, declared := idx.moduleCalls[moduleCallKey{
		moduleAddress: moduleAddress,
		moduleName:    name,
	}]
	return declared, nil
}

// lookupResource maps a resource instance to the configuration
// declaration shared by all instances of that resource.  Terraform
// configuration records resource blocks rather than their expanded
// instances, so the lookup ignores module and resource instance keys.
// For example:
// module.apps["api"].coder_script.setup[0] maps to
// module.apps.coder_script.setup.
func (idx *configIndex) lookupResource(
	evaluatedModuleAddress string,
	mode tfjson.ResourceMode,
	resourceType string,
	name string,
) (*tfjson.ConfigResource, error) {
	moduleAddress, err := configurationModuleAddress(evaluatedModuleAddress)
	if err != nil {
		return nil, err
	}
	return idx.resources[configResourceKey{
		moduleAddress: moduleAddress,
		mode:          mode,
		resourceType:  resourceType,
		resourceName:  name,
	}], nil
}

func (idx *configIndex) resourceDeclared(
	evaluatedModuleAddress string,
	resourceType string,
	name string,
) (bool, error) {
	if idx == nil {
		return false, xerrors.New(
			"cannot validate empty coder_script selector because Terraform plan configuration is unavailable",
		)
	}
	resource, err := idx.lookupResource(
		evaluatedModuleAddress, tfjson.ManagedResourceMode, resourceType, name,
	)
	return resource != nil, err
}
