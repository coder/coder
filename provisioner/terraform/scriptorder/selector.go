// Package scriptorder resolves deterministic lifecycle ordering for
// Terraform-managed Coder scripts.
package scriptorder

import (
	"maps"
	"slices"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

type scriptOrderSelectorKind int

const (
	_ scriptOrderSelectorKind = iota
	scriptOrderSelectorScript
	scriptOrderSelectorModule
)

var errScriptOrderConfigModuleNotFound = xerrors.New("script order configuration module not found")

type scriptOrderSelector struct {
	kind        scriptOrderSelectorKind
	name        string
	instanceKey cty.Value
}

type scriptOrderSelectorResolution struct {
	// Contains the sorted, deduplicated Terraform addresses of all
	// concrete scripts selected.
	addresses []string
	// Populated only for empty unindexed script selectors. It
	// distinguishes a declared resource with no concrete instances,
	// such as one with count = 0, from an unknown resource selector.
	scriptResourceDeclared bool
	// Populated for every module selector. It distinguishes a
	// declared module call with no resolved scripts from an unknown
	// module selector. Scripts may resolve to no instances when a
	// module conditionally sets their count to zero.
	moduleCallDeclared bool
}

func invalidScriptOrderSelectorError(raw string) error {
	return xerrors.Errorf(
		"script order selector %q must reference a coder_script in the "+
			"declaring module or an entire direct child module call",
		raw,
	)
}

// parseScriptOrderSelector currently limits selectors to scripts in
// the declaring module and whole child module calls.
func parseScriptOrderSelector(raw string) (scriptOrderSelector, error) {
	modulePath, moduleErr := tfaddr.ParseModulePath(raw)
	if moduleErr == nil {
		steps := modulePath.Steps()
		if len(steps) != 1 {
			return scriptOrderSelector{}, invalidScriptOrderSelectorError(raw)
		}
		if steps[0].InstanceKey() != cty.NilVal {
			return scriptOrderSelector{}, xerrors.Errorf(
				"module selector %q must select all module instances", raw,
			)
		}
		return scriptOrderSelector{
			kind: scriptOrderSelectorModule,
			name: steps[0].Name(),
		}, nil
	}

	address, err := tfaddr.ParseManagedResourceAddress(raw)
	if err != nil {
		return scriptOrderSelector{},
			xerrors.Errorf("parse script order selector %q: %w", raw, err)
	}
	if address.ModulePath().String() != "" {
		return scriptOrderSelector{}, invalidScriptOrderSelectorError(raw)
	}

	switch address.ResourceType() {
	case "coder_script":
		return scriptOrderSelector{
			kind:        scriptOrderSelectorScript,
			name:        address.ResourceName(),
			instanceKey: address.InstanceKey(),
		}, nil
	default:
		return scriptOrderSelector{}, xerrors.Errorf("script order selector %q must select a coder_script or module", raw)
	}
}

// resolveScriptOrderSelector expands a selector relative to its
// declaring module. For selectors with no resolved scripts, it also
// reports whether the selected resource or module call is declared so
// callers can distinguish an empty declaration from an unknown selector.
//
// `modules` contains one or more evaluated module trees and their
// concrete script instances. During plan conversion, it may include
// prior state filtered to data resources alongside planned values.
// `planConfig` contains resource and module call declarations, including
// declarations with no instances after evaluation.
// `selector` must have been produced by parseScriptOrderSelector.
func resolveScriptOrderSelector(
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
	moduleAddress string,
	selector scriptOrderSelector,
) (scriptOrderSelectorResolution, error) {
	if selector.kind != scriptOrderSelectorScript &&
		selector.kind != scriptOrderSelectorModule {
		return scriptOrderSelectorResolution{},
			xerrors.Errorf("unknown script order selector kind %d", selector.kind)
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

	resolution := scriptOrderSelectorResolution{
		addresses: slices.Sorted(maps.Keys(resolved)),
	}
	switch selector.kind {
	case scriptOrderSelectorScript:
		// Only unindexed selectors may be valid without concrete
		// instances. The caller rejects missing indexed instances.
		if selector.instanceKey == cty.NilVal && len(resolution.addresses) == 0 {
			declared, err := isCoderScriptResourceInConfig(
				planConfig, moduleAddress, selector.name,
			)
			if err != nil {
				return scriptOrderSelectorResolution{}, err
			}
			resolution.scriptResourceDeclared = declared
		}
	case scriptOrderSelectorModule:
		// Always validate module selectors against the plan config.
		// This distinguishes unknown selectors from declared calls
		// with no scripts.
		declared, err := isModuleCallInConfig(
			planConfig, moduleAddress, selector.name,
		)
		if err != nil {
			return scriptOrderSelectorResolution{}, err
		}
		resolution.moduleCallDeclared = declared
	}
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

	module, err := configModuleForAddress(config.RootModule, declaringModuleAddress)
	if xerrors.Is(err, errScriptOrderConfigModuleNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return module.ModuleCalls[name] != nil, nil
}

// isCoderScriptResourceInConfig reports whether `name` is a managed
// coder_script resource in the configuration of the declaring module
// instance.
func isCoderScriptResourceInConfig(
	config *tfjson.Config,
	declaringModuleAddress string,
	name string,
) (bool, error) {
	if config == nil || config.RootModule == nil {
		return false, xerrors.New(
			"cannot validate empty coder_script selector because Terraform plan configuration is unavailable",
		)
	}

	module, err := configModuleForAddress(config.RootModule, declaringModuleAddress)
	if xerrors.Is(err, errScriptOrderConfigModuleNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, resource := range module.Resources {
		if resource != nil &&
			resource.Mode == tfjson.ManagedResourceMode &&
			resource.Type == "coder_script" &&
			resource.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// configModuleForAddress maps an evaluated module-instance address to
// its configuration. Instance keys are ignored because repeated
// module instances share one configuration.
func configModuleForAddress(
	root *tfjson.ConfigModule,
	moduleAddress string,
) (*tfjson.ConfigModule, error) {
	module := root
	if moduleAddress == "" {
		return module, nil
	}

	modulePath, err := parseStateModuleAddress(moduleAddress)
	if err != nil {
		return nil, err
	}
	for _, step := range modulePath.Steps() {
		call := module.ModuleCalls[step.Name()]
		if call == nil || call.Module == nil {
			return nil, errScriptOrderConfigModuleNotFound
		}
		module = call.Module
	}
	return module, nil
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
		if selector.instanceKey != cty.NilVal &&
			!tfaddr.InstanceKeysEqual(address.InstanceKey(), selector.instanceKey) {
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
		steps := modulePath.Steps()
		if len(steps) == 0 || steps[len(steps)-1].Name() != selector.name {
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
) (*tfaddr.ManagedResourceAddress, error) {
	address, err := tfaddr.ParseManagedResourceAddress(resource.Address)
	if err != nil {
		return nil, xerrors.Errorf("parse Terraform resource address %q: %w", resource.Address, err)
	}
	// Defensive: TF should always emit an address consistent with
	// these state fields.
	if address.ModulePath().String() != module.Address ||
		address.ResourceType() != resource.Type ||
		address.ResourceName() != resource.Name {
		return nil, xerrors.Errorf("Terraform resource address %q does not match its state fields", resource.Address)
	}
	return &address, nil
}

func parseStateModuleAddress(address string) (tfaddr.ModulePath, error) {
	parsed, err := tfaddr.ParseModulePath(address)
	if err != nil {
		return tfaddr.ModulePath{}, xerrors.Errorf("parse module address %q: %w", address, err)
	}
	return parsed, nil
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
