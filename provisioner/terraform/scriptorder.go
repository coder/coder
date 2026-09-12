package terraform

import (
	"maps"
	"slices"

	tfaddr "github.com/hashicorp/go-terraform-address"
	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"
)

type scriptOrderSelectorKind int

const (
	_ scriptOrderSelectorKind = iota
	scriptOrderSelectorScript
	scriptOrderSelectorModule
)

type scriptOrderSelector struct {
	kind        scriptOrderSelectorKind
	name        string
	instanceKey string
}

type scriptOrderSelectorResolution struct {
	// contains the sorted, deduplicated Terraform addresses of all
	// concrete scripts selected.
	addresses []string
	// For module selectors, distinguishes a declared module call with
	// no resolved scripts from an unknown module selector. Scripts
	// may resolve to no instances when a module conditionally sets
	// their count to zero.
	moduleCallDeclared bool
}

// parseScriptOrderSelector currently limits selectors to scripts in
// the declaring module and whole child module calls.
func parseScriptOrderSelector(raw string) (scriptOrderSelector, error) {
	address, err := tfaddr.NewAddress(raw)
	if err != nil {
		return scriptOrderSelector{}, xerrors.Errorf("parse script order selector %q: %w", raw, err)
	}
	if len(address.ModulePath) != 0 {
		return scriptOrderSelector{}, xerrors.Errorf(
			"script order selector %q must reference a coder_script in the declaring module or an entire direct child module call",
			raw,
		)
	}

	switch address.ResourceSpec.Type {
	case "coder_script":
		return scriptOrderSelector{
			kind:        scriptOrderSelectorScript,
			name:        address.ResourceSpec.Name,
			instanceKey: address.ResourceSpec.Index.String(),
		}, nil
	case "module":
		if address.ResourceSpec.Index.String() != "" {
			return scriptOrderSelector{}, xerrors.Errorf("module selector %q must select all module instances", raw)
		}
		return scriptOrderSelector{
			kind: scriptOrderSelectorModule,
			name: address.ResourceSpec.Name,
		}, nil
	default:
		return scriptOrderSelector{}, xerrors.Errorf("script order selector %q must select a coder_script or module", raw)
	}
}

// resolveScriptOrderSelector expands a selector relative to its
// declaring module. For module selectors, it also reports whether the
// module call is declared so callers can distinguish an empty module
// call from an unknown selector.
//
// `modules` is the evaluated module tree and its concrete script instances.
// `planConfig` contains declared module calls, including calls with no
// instances after evaluation.
// `selector` must have been produced by parseScriptOrderSelector.
func resolveScriptOrderSelector(
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
	moduleAddress string,
	selector scriptOrderSelector,
) (scriptOrderSelectorResolution, error) {
	if selector.kind != scriptOrderSelectorScript && selector.kind != scriptOrderSelectorModule {
		return scriptOrderSelectorResolution{},
			xerrors.Errorf("unknown script order selector kind %d", selector.kind)
	}

	var resolution scriptOrderSelectorResolution
	if selector.kind == scriptOrderSelectorModule {
		declared, err := isModuleCallInConfig(planConfig, moduleAddress, selector.name)
		if err != nil {
			return scriptOrderSelectorResolution{}, err
		}
		resolution.moduleCallDeclared = declared
	}

	resolved := map[string]struct{}{}
	for _, rootModule := range modules {
		err := walkStateModuleTree(rootModule, func(module *tfjson.StateModule) error {
			if module.Address != moduleAddress {
				return nil
			}

			switch selector.kind {
			case scriptOrderSelectorScript:
				return resolveScriptOrderScriptSelector(module, selector, resolved)
			case scriptOrderSelectorModule:
				return resolveScriptOrderModuleSelector(module, selector, resolved)
			default:
				return xerrors.Errorf("unknown script order selector kind %d", selector.kind)
			}
		})
		if err != nil {
			return scriptOrderSelectorResolution{}, err
		}
	}

	resolution.addresses = slices.Sorted(maps.Keys(resolved))
	return resolution, nil
}

// isModuleCallInConfig reports whether name is a direct child module
// call in the configuration of the declaring module instance.
func isModuleCallInConfig(
	config *tfjson.Config,
	declaringModuleAddress string,
	name string,
) (bool, error) {
	if config == nil || config.RootModule == nil {
		return false, xerrors.New("terraform plan configuration is required to resolve a module selector")
	}

	module := config.RootModule
	if declaringModuleAddress != "" {
		modulePath, err := parseStateModuleAddress(declaringModuleAddress)
		if err != nil {
			return false, err
		}
		for _, step := range modulePath {
			call := module.ModuleCalls[step.Name]
			if call == nil || call.Module == nil {
				return false, nil
			}
			module = call.Module
		}
	}

	return module.ModuleCalls[name] != nil, nil
}

func resolveScriptOrderScriptSelector(
	module *tfjson.StateModule,
	selector scriptOrderSelector,
	resolved map[string]struct{},
) error {
	// An unindexed script selector expands every count and for_each
	// instance; an indexed selector retains only the matching
	// instance key.
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
		if selector.instanceKey != "" &&
			address.ResourceSpec.Index.String() != selector.instanceKey {
			continue
		}
		resolved[resource.Address] = struct{}{}
	}
	return nil
}

func resolveScriptOrderModuleSelector(
	module *tfjson.StateModule,
	selector scriptOrderSelector,
	resolved map[string]struct{},
) error {
	// An unindexed module selector expands every count and for_each
	// instance of the selected child module call.
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
		if err := collectModuleCoderScriptAddresses(child, resolved); err != nil {
			return err
		}
	}
	return nil
}

func collectModuleCoderScriptAddresses(
	module *tfjson.StateModule, resolved map[string]struct{},
) error {
	for _, resource := range module.Resources {
		if resource == nil ||
			resource.Mode != tfjson.ManagedResourceMode ||
			resource.Type != "coder_script" {
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
		if err := collectModuleCoderScriptAddresses(child, resolved); err != nil {
			return err
		}
	}
	return nil
}

// parseStateResourceAddress parses a concrete resource address and
// verifies that it matches its containing state module and resource
// fields. This prevents inconsistent Terraform output from assigning
// dependencies to the wrong resource.
func parseStateResourceAddress(
	module *tfjson.StateModule, resource *tfjson.StateResource,
) (*tfaddr.Address, error) {
	address, err := tfaddr.NewAddress(resource.Address)
	if err != nil {
		return nil, xerrors.Errorf("parse Terraform resource address %q: %w", resource.Address, err)
	}
	// Defensive: TF should always emit an address consistent with
	// these state fields.
	if address.ModulePath.String() != module.Address ||
		address.ResourceSpec.Type != resource.Type ||
		address.ResourceSpec.Name != resource.Name {
		return nil, xerrors.Errorf("Terraform resource address %q does not match its state fields", resource.Address)
	}
	return address, nil
}

func parseStateModuleAddress(address string) (tfaddr.ModulePath, error) {
	// go-terraform-address parses a module path only as part of a
	// resource address, so append a placeholder resource before
	// parsing it.
	parsed, err := tfaddr.NewAddress(address + ".placeholder_resource.placeholder")
	if err != nil {
		return nil, xerrors.Errorf("parse module address %q: %w", address, err)
	}
	return parsed.ModulePath, nil
}

func walkStateModuleTree(module *tfjson.StateModule, visit func(*tfjson.StateModule) error) error {
	if module == nil {
		return nil
	}
	if err := visit(module); err != nil {
		return err
	}
	for _, child := range module.ChildModules {
		if err := walkStateModuleTree(child, visit); err != nil {
			return err
		}
	}
	return nil
}
