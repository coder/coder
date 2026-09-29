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
	// Resource selectors qualified by a module path are not supported.
	if address.ModulePath().String() != "" {
		return selector{}, invalidSelectorError(raw)
	}

	switch address.ResourceType() {
	case coderScriptResourceType:
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

// resolveSelector expands a selector relative to its declaring module using
// the program indexes. parsedSelector must have been produced by parseSelector.
func (p *Program) resolveSelector(
	moduleAddress string,
	parsedSelector selector,
	budget *expansionBudget,
) (selectorResolution, error) {
	var state *stateIndex
	var config *configIndex
	if p != nil {
		state = p.stateIndex
		config = p.configIndex
	}
	if parsedSelector.kind != selectorScript &&
		parsedSelector.kind != selectorModule {
		return selectorResolution{},
			xerrors.Errorf("unknown script order selector kind %d", parsedSelector.kind)
	}

	resolved := map[string]struct{}{}
	switch parsedSelector.kind {
	case selectorScript:
		if state != nil {
			key := stateResourceKey{
				moduleAddress: moduleAddress,
				resourceName:  parsedSelector.name,
			}
			for _, scriptIndex := range state.scriptsByResource[key] {
				script := &state.scripts[scriptIndex]
				if script.addressErr != nil {
					return selectorResolution{}, script.addressErr
				}
				if parsedSelector.instanceKey != cty.NilVal &&
					!tfaddr.InstanceKeysEqual(script.instanceKey, parsedSelector.instanceKey) {
					continue
				}
				if err := addExpandedAddress(
					resolved, script.address, budget,
				); err != nil {
					return selectorResolution{}, err
				}
			}
		}
	case selectorModule:
		if state != nil {
			if err := state.moduleLookupErrors[moduleAddress]; err != nil {
				return selectorResolution{}, err
			}
			key := moduleCallKey{
				moduleAddress: moduleAddress,
				moduleName:    parsedSelector.name,
			}
			for _, span := range state.scriptsByModule[key] {
				for scriptIndex := span.start; scriptIndex < span.end; scriptIndex++ {
					script := &state.scripts[scriptIndex]
					if script.addressErr != nil {
						return selectorResolution{}, script.addressErr
					}
					if err := addExpandedAddress(
						resolved, script.address, budget,
					); err != nil {
						return selectorResolution{}, err
					}
				}
			}
		}
	}

	resolution := selectorResolution{addresses: slices.Sorted(maps.Keys(resolved))}
	switch parsedSelector.kind {
	case selectorScript:
		// Only unindexed selectors may be valid without concrete
		// instances. The caller rejects missing indexed instances.
		if parsedSelector.instanceKey == cty.NilVal && len(resolution.addresses) == 0 {
			declared, err := config.resourceDeclared(
				moduleAddress, coderScriptResourceType, parsedSelector.name,
			)
			if err != nil {
				return selectorResolution{}, err
			}
			resolution.scriptResourceDeclared = declared
		}
	case selectorModule:
		// Always validate module selectors against the plan config. This
		// distinguishes unknown selectors from declared calls with no scripts.
		declared, err := config.moduleCallDeclared(moduleAddress, parsedSelector.name)
		if err != nil {
			return selectorResolution{}, err
		}
		resolution.moduleCallDeclared = declared
	}
	return resolution, nil
}

type expansionBudget struct {
	used  int
	limit int
}

func addExpandedAddress(
	resolved map[string]struct{},
	address string,
	budget *expansionBudget,
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
	module *tfjson.StateModule,
	resource *tfjson.StateResource,
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
