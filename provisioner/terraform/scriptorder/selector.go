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
// declaring module using indexes built once for the conversion input.
func resolveScriptOrderSelector(
	stateIndex *scriptOrderStateIndex,
	configIndex *scriptOrderConfigIndex,
	moduleAddress string,
	selector scriptOrderSelector,
	expansionBudget *scriptOrderExpansionBudget,
) (scriptOrderSelectorResolution, error) {
	if selector.kind != scriptOrderSelectorScript && selector.kind != scriptOrderSelectorModule {
		return scriptOrderSelectorResolution{},
			xerrors.Errorf("unknown script order selector kind %d", selector.kind)
	}

	resolved := map[string]struct{}{}
	switch selector.kind {
	case scriptOrderSelectorScript:
		if stateIndex != nil {
			key := scriptOrderStateResourceKey{
				moduleAddress: moduleAddress,
				resourceName:  selector.name,
			}
			for _, scriptIndex := range stateIndex.scriptsByResource[key] {
				script := stateIndex.scripts[scriptIndex]
				if script.err != nil {
					return scriptOrderSelectorResolution{}, script.err
				}
				if selector.instanceKey != cty.NilVal &&
					!tfaddr.InstanceKeysEqual(script.instanceKey, selector.instanceKey) {
					continue
				}
				if err := addScriptOrderExpandedAddress(
					resolved, script.address, expansionBudget,
				); err != nil {
					return scriptOrderSelectorResolution{}, err
				}
			}
		}
	case scriptOrderSelectorModule:
		if stateIndex != nil {
			if err := stateIndex.moduleAddressError[moduleAddress]; err != nil {
				return scriptOrderSelectorResolution{}, err
			}
			key := scriptOrderModuleCallKey{
				moduleAddress: moduleAddress,
				moduleName:    selector.name,
			}
			for _, span := range stateIndex.scriptsByModule[key] {
				for scriptIndex := span.start; scriptIndex < span.end; scriptIndex++ {
					script := stateIndex.scripts[scriptIndex]
					if script.err != nil {
						return scriptOrderSelectorResolution{}, script.err
					}
					if err := addScriptOrderExpandedAddress(
						resolved, script.address, expansionBudget,
					); err != nil {
						return scriptOrderSelectorResolution{}, err
					}
				}
			}
		}
	}

	resolution := scriptOrderSelectorResolution{addresses: slices.Sorted(maps.Keys(resolved))}
	switch selector.kind {
	case scriptOrderSelectorScript:
		if selector.instanceKey == cty.NilVal && len(resolution.addresses) == 0 {
			declared, err := configIndex.managedResourceDeclared(
				moduleAddress, "coder_script", selector.name,
			)
			if err != nil {
				return scriptOrderSelectorResolution{}, err
			}
			resolution.scriptResourceDeclared = declared
		}
	case scriptOrderSelectorModule:
		declared, err := configIndex.moduleCallDeclared(moduleAddress, selector.name)
		if err != nil {
			return scriptOrderSelectorResolution{}, err
		}
		resolution.moduleCallDeclared = declared
	}
	return resolution, nil
}

type scriptOrderExpansionBudget struct {
	used  int
	limit int
}

func addScriptOrderExpandedAddress(
	resolved map[string]struct{},
	address string,
	budget *scriptOrderExpansionBudget,
) error {
	if _, ok := resolved[address]; ok {
		return nil
	}
	if budget != nil {
		if budget.used >= budget.limit {
			return xerrors.Errorf(
				"script order selectors are limited to %d expanded script addresses in total",
				budget.limit,
			)
		}
		budget.used++
	}
	resolved[address] = struct{}{}
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
