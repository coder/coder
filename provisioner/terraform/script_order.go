package terraform

import (
	"fmt"
	"maps"
	"slices"

	tfaddr "github.com/hashicorp/go-terraform-address"
	tfjson "github.com/hashicorp/terraform-json"
)

type scriptOrderSelectorKind int

const (
	_ scriptOrderSelectorKind = iota
	scriptOrderSelectorScript
	scriptOrderSelectorModule
)

type scriptOrderSelector struct {
	kind  scriptOrderSelectorKind
	name  string
	index string
}

func parseScriptOrderSelector(raw string) (scriptOrderSelector, error) {
	address, err := tfaddr.NewAddress(raw)
	if err != nil {
		return scriptOrderSelector{}, fmt.Errorf("parse script order selector %q: %w", raw, err)
	}
	if len(address.ModulePath) != 0 {
		return scriptOrderSelector{}, fmt.Errorf("script order selector %q must be relative to its declaring module", raw)
	}

	switch address.ResourceSpec.Type {
	case "coder_script":
		return scriptOrderSelector{
			kind:  scriptOrderSelectorScript,
			name:  address.ResourceSpec.Name,
			index: address.ResourceSpec.Index.String(),
		}, nil
	case "module":
		if address.ResourceSpec.Index.String() != "" {
			return scriptOrderSelector{}, fmt.Errorf("module selector %q must select all module instances", raw)
		}
		return scriptOrderSelector{
			kind: scriptOrderSelectorModule,
			name: address.ResourceSpec.Name,
		}, nil
	default:
		return scriptOrderSelector{}, fmt.Errorf("script order selector %q must select a coder_script or module", raw)
	}
}

func resolveScriptOrderSelector(modules []*tfjson.StateModule, moduleAddress string, selector scriptOrderSelector) ([]string, error) {
	resolved := map[string]struct{}{}
	for _, rootModule := range modules {
		err := forEachModule(rootModule, func(module *tfjson.StateModule) error {
			if module.Address != moduleAddress {
				return nil
			}

			switch selector.kind {
			case scriptOrderSelectorScript:
				return resolveScriptSelector(module, selector, resolved)
			case scriptOrderSelectorModule:
				return resolveModuleSelector(module, selector, resolved)
			default:
				return fmt.Errorf("unknown script order selector kind %d", selector.kind)
			}
		})
		if err != nil {
			return nil, err
		}
	}
	return slices.Sorted(maps.Keys(resolved)), nil
}

func resolveScriptSelector(module *tfjson.StateModule, selector scriptOrderSelector, resolved map[string]struct{}) error {
	for _, resource := range module.Resources {
		if resource == nil ||
			resource.Mode != tfjson.ManagedResourceMode ||
			resource.Type != "coder_script" ||
			resource.Name != selector.name {
			continue
		}

		address, err := parseStateResourceAddress(module, resource)
		if err != nil {
			return err
		}
		if selector.index != "" && address.ResourceSpec.Index.String() != selector.index {
			continue
		}
		resolved[resource.Address] = struct{}{}
	}
	return nil
}

func resolveModuleSelector(module *tfjson.StateModule, selector scriptOrderSelector, resolved map[string]struct{}) error {
	for _, child := range module.ChildModules {
		if child == nil {
			continue
		}

		modulePath, err := parseStateModuleAddress(child.Address)
		if err != nil {
			return err
		}
		if len(modulePath) == 0 || modulePath[len(modulePath)-1].Name != selector.name {
			continue
		}
		if err := collectModuleScripts(child, resolved); err != nil {
			return err
		}
	}
	return nil
}

func collectModuleScripts(module *tfjson.StateModule, resolved map[string]struct{}) error {
	for _, resource := range module.Resources {
		if resource == nil || resource.Mode != tfjson.ManagedResourceMode || resource.Type != "coder_script" {
			continue
		}
		if _, err := parseStateResourceAddress(module, resource); err != nil {
			return err
		}
		resolved[resource.Address] = struct{}{}
	}
	for _, child := range module.ChildModules {
		if child == nil {
			continue
		}
		if err := collectModuleScripts(child, resolved); err != nil {
			return err
		}
	}
	return nil
}

func parseStateResourceAddress(module *tfjson.StateModule, resource *tfjson.StateResource) (*tfaddr.Address, error) {
	address, err := tfaddr.NewAddress(resource.Address)
	if err != nil {
		return nil, fmt.Errorf("parse Terraform resource address %q: %w", resource.Address, err)
	}
	if address.ModulePath.String() != module.Address || address.ResourceSpec.Type != resource.Type || address.ResourceSpec.Name != resource.Name {
		return nil, fmt.Errorf("Terraform resource address %q does not match its state fields", resource.Address)
	}
	return address, nil
}

func parseStateModuleAddress(address string) (tfaddr.ModulePath, error) {
	// go-terraform-address parses a module path only as part of a resource
	// address, so append a placeholder resource before parsing it.
	parsed, err := tfaddr.NewAddress(address + ".placeholder_resource.placeholder")
	if err != nil {
		return nil, fmt.Errorf("parse module address %q: %w", address, err)
	}
	if len(parsed.ModulePath) == 0 {
		return nil, fmt.Errorf("module address %q has no module path", address)
	}
	return parsed.ModulePath, nil
}

func forEachModule(module *tfjson.StateModule, visit func(*tfjson.StateModule) error) error {
	if module == nil {
		return nil
	}
	if err := visit(module); err != nil {
		return err
	}
	for _, child := range module.ChildModules {
		if err := forEachModule(child, visit); err != nil {
			return err
		}
	}
	return nil
}
