package scriptorder

import (
	"maps"
	"slices"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

type selectorKind int

const (
	_ selectorKind = iota
	selectorScript
	selectorModule
)

var errConfigModuleNotFound = xerrors.New("script order configuration module not found")

type selector struct {
	kind        selectorKind
	name        string
	instanceKey cty.Value
}

type selectorResolution struct {
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

func invalidSelectorError(raw string) error {
	return xerrors.Errorf(
		"script order selector %q must reference a coder_script in the "+
			"declaring module or an entire direct child module call",
		raw,
	)
}

// parseSelector currently limits selectors to scripts in
// the declaring module and whole child module calls.
func parseSelector(raw string) (selector, error) {
	modulePath, moduleErr := tfaddr.ParseModulePath(raw)
	if moduleErr == nil {
		steps := modulePath.Steps()
		if len(steps) != 1 {
			return selector{}, invalidSelectorError(raw)
		}
		if steps[0].InstanceKey() != cty.NilVal {
			return selector{}, xerrors.Errorf(
				"module selector %q must select all module instances", raw,
			)
		}
		return selector{
			kind: selectorModule,
			name: steps[0].Name(),
		}, nil
	}

	address, err := tfaddr.ParseManagedResourceAddress(raw)
	if err != nil {
		return selector{},
			xerrors.Errorf("parse script order selector %q: %w", raw, err)
	}
	// Resource selectors qualified by a module path are not supported yet.
	if address.ModulePath().String() != "" {
		return selector{}, invalidSelectorError(raw)
	}

	switch address.ResourceType() {
	case "coder_script":
		return selector{
			kind:        selectorScript,
			name:        address.ResourceName(),
			instanceKey: address.InstanceKey(),
		}, nil
	default:
		return selector{}, xerrors.Errorf(
			"script order selector %q must select a coder_script or module", raw)
	}
}

// resolveSelector expands a selector relative to its
// declaring module. For selectors with no resolved scripts, it also
// reports whether the selected resource or module call is declared so
// callers can distinguish an empty declaration from an unknown selector.
//
// `modules` contains one or more evaluated module trees and their
// concrete script instances. During plan conversion, it may include
// prior state filtered to data resources alongside planned values.
// `planConfig` contains resource and module call declarations, including
// declarations with no instances after evaluation.
// `parsedSelector` must have been produced by parseSelector.
func resolveSelector(
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
	moduleAddress string,
	parsedSelector selector,
) (selectorResolution, error) {
	if parsedSelector.kind != selectorScript &&
		parsedSelector.kind != selectorModule {
		return selectorResolution{},
			xerrors.Errorf("unknown script order selector kind %d", parsedSelector.kind)
	}

	resolved := map[string]struct{}{}
	for _, rootModule := range modules {
		err := walkStateModuleTree(rootModule, func(module *tfjson.StateModule) error {
			if module.Address != moduleAddress {
				return nil
			}

			switch parsedSelector.kind {
			case selectorScript:
				return resolveScriptSelector(module, parsedSelector, resolved)
			case selectorModule:
				return resolveModuleSelector(module, parsedSelector, resolved)
			default:
				return xerrors.Errorf("unknown script order selector kind %d", parsedSelector.kind)
			}
		})
		if err != nil {
			return selectorResolution{}, err
		}
	}

	resolution := selectorResolution{
		addresses: slices.Sorted(maps.Keys(resolved)),
	}
	switch parsedSelector.kind {
	case selectorScript:
		// Only unindexed selectors may be valid without concrete
		// instances. The caller rejects missing indexed instances.
		if parsedSelector.instanceKey == cty.NilVal && len(resolution.addresses) == 0 {
			declared, err := isCoderScriptResourceInConfig(
				planConfig, moduleAddress, parsedSelector.name,
			)
			if err != nil {
				return selectorResolution{}, err
			}
			resolution.scriptResourceDeclared = declared
		}
	case selectorModule:
		// Always validate module selectors against the plan config.
		// This distinguishes unknown selectors from declared calls
		// with no scripts.
		declared, err := isModuleCallInConfig(
			planConfig, moduleAddress, parsedSelector.name,
		)
		if err != nil {
			return selectorResolution{}, err
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
	if xerrors.Is(err, errConfigModuleNotFound) {
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
	if xerrors.Is(err, errConfigModuleNotFound) {
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
			return nil, errConfigModuleNotFound
		}
		module = call.Module
	}
	return module, nil
}

func resolveScriptSelector(
	module *tfjson.StateModule,
	parsedSelector selector,
	resolved map[string]struct{},
) error {
	// An unindexed script selector expands every count and for_each
	// instance; an indexed selector retains only the matching
	// instance key.
	for _, resource := range module.Resources {
		if resource == nil ||
			resource.Mode != tfjson.ManagedResourceMode ||
			resource.Type != "coder_script" ||
			resource.Name != parsedSelector.name {
			continue
		}

		address, err := parseStateResourceAddress(module, resource)
		if err != nil {
			return err
		}
		if parsedSelector.instanceKey != cty.NilVal &&
			!tfaddr.InstanceKeysEqual(address.InstanceKey(), parsedSelector.instanceKey) {
			continue
		}
		resolved[resource.Address] = struct{}{}
	}
	return nil
}

func resolveModuleSelector(
	module *tfjson.StateModule,
	parsedSelector selector,
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
		if len(steps) == 0 || steps[len(steps)-1].Name() != parsedSelector.name {
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
