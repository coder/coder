package terraform

import (
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"github.com/zclconf/go-cty/cty"
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
	instanceKey cty.Value
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
	traversal, err := parseTerraformAddressTraversal(raw)
	if err != nil {
		return scriptOrderSelector{},
			xerrors.Errorf("parse script order selector %q: %w", raw, err)
	}
	if len(traversal) < 2 {
		return scriptOrderSelector{}, invalidScriptOrderSelectorError(raw)
	}

	root, rootOK := traversal[0].(hcl.TraverseRoot)
	name, nameOK := traversal[1].(hcl.TraverseAttr)
	if !rootOK || !nameOK {
		return scriptOrderSelector{}, invalidScriptOrderSelectorError(raw)
	}

	instanceKey := cty.NilVal
	position := 2
	if position < len(traversal) {
		index, ok := traversal[position].(hcl.TraverseIndex)
		if !ok {
			return scriptOrderSelector{}, invalidScriptOrderSelectorError(raw)
		}
		instanceKey, err = parseTerraformInstanceKey(index.Key)
		if err != nil {
			return scriptOrderSelector{},
				xerrors.Errorf("parse script order selector %q instance key: %w", raw, err)
		}
		position++
	}
	if position != len(traversal) {
		return scriptOrderSelector{}, invalidScriptOrderSelectorError(raw)
	}

	switch root.Name {
	case "coder_script":
		return scriptOrderSelector{
			kind:        scriptOrderSelectorScript,
			name:        name.Name,
			instanceKey: instanceKey,
		}, nil
	case "module":
		if instanceKey != cty.NilVal {
			return scriptOrderSelector{}, xerrors.Errorf("module selector %q must select all module instances", raw)
		}
		return scriptOrderSelector{
			kind: scriptOrderSelectorModule,
			name: name.Name,
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
		for _, step := range modulePath.steps {
			call := module.ModuleCalls[step.name]
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
		if selector.instanceKey != cty.NilVal &&
			!terraformInstanceKeysEqual(address.instanceKey, selector.instanceKey) {
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
		if len(modulePath.steps) == 0 || modulePath.steps[len(modulePath.steps)-1].name != selector.name {
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
) (*terraformManagedResourceAddress, error) {
	address, err := parseTerraformManagedResourceAddress(resource.Address)
	if err != nil {
		return nil, xerrors.Errorf("parse Terraform resource address %q: %w", resource.Address, err)
	}
	// Defensive: TF should always emit an address consistent with
	// these state fields.
	if address.modulePath.String() != module.Address ||
		address.resourceType != resource.Type ||
		address.resourceName != resource.Name {
		return nil, xerrors.Errorf("Terraform resource address %q does not match its state fields", resource.Address)
	}
	return &address, nil
}

func parseStateModuleAddress(address string) (terraformModulePath, error) {
	parsed, err := parseTerraformModulePath(address)
	if err != nil {
		return terraformModulePath{}, xerrors.Errorf("parse module address %q: %w", address, err)
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

type scriptOrderRequirement string

const (
	scriptOrderRequirementSuccess    scriptOrderRequirement = "success"
	scriptOrderRequirementCompletion scriptOrderRequirement = "completion"
)

type scriptOrderPhase string

const (
	scriptOrderPhaseStart scriptOrderPhase = "start"
	scriptOrderPhaseStop  scriptOrderPhase = "stop"
)

type scriptOrderAttributes struct {
	Rules []scriptOrderRuleAttributes `mapstructure:"rule"`
}

type scriptOrderRuleAttributes struct {
	Run      []string `mapstructure:"run"`
	After    []string `mapstructure:"after"`
	Requires string   `mapstructure:"requires"`
	Phase    string   `mapstructure:"phase"`
}

type scriptOrderDataSource struct {
	address       string
	moduleAddress string
	resource      *tfjson.StateResource
}

type resolvedScriptOrderSelector struct {
	field string
	raw   string
	kind  scriptOrderSelectorKind
	// addresses contains the concrete coder_script instances selected
	// by `raw`. For example, "coder_script.setup" can expand to
	// "coder_script.setup[0]" and "coder_script.setup[1]". It may be
	// empty for a declared module call with no script instances.
	addresses []string
}

// scriptOrderRuleDeclaration contains decoded rule values and resolved
// selectors before phase filtering and runtime validation.
type scriptOrderRuleDeclaration struct {
	dataSourceAddress string
	ruleIndex         int
	declaredPhase     scriptOrderPhase
	requirement       scriptOrderRequirement
	run               []resolvedScriptOrderSelector
	after             []resolvedScriptOrderSelector
}

// collectScriptOrderRuleDeclarations decodes coder_script_order data
// sources and resolves their selectors relative to each declaring module.
func collectScriptOrderRuleDeclarations(
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
) ([]scriptOrderRuleDeclaration, error) {
	srcs, err := collectScriptOrderDataSources(modules)
	if err != nil {
		return nil, err
	}

	var decls []scriptOrderRuleDeclaration
	for _, src := range srcs {
		var attrs scriptOrderAttributes
		err := mapstructure.Decode(src.resource.AttributeValues, &attrs)
		if err != nil {
			return nil, xerrors.Errorf(
				"decode script order data source %q: %w", src.address, err,
			)
		}
		if len(attrs.Rules) == 0 {
			return nil, xerrors.Errorf(
				"script order data source %q must contain at least one rule",
				src.address,
			)
		}

		for i, rule := range attrs.Rules {
			requirement, err := parseScriptOrderRequirement(rule.Requires)
			if err != nil {
				return nil, scriptOrderRuleError(src.address, i, err)
			}
			phase, err := parseScriptOrderPhase(rule.Phase)
			if err != nil {
				return nil, scriptOrderRuleError(src.address, i, err)
			}

			run, err := resolveScriptOrderSelectors(
				modules, planConfig, src, i, "run", rule.Run,
			)
			if err != nil {
				return nil, err
			}
			after, err := resolveScriptOrderSelectors(
				modules, planConfig, src, i, "after", rule.After,
			)
			if err != nil {
				return nil, err
			}

			decls = append(decls, scriptOrderRuleDeclaration{
				dataSourceAddress: src.address,
				ruleIndex:         i,
				declaredPhase:     phase,
				requirement:       requirement,
				run:               run,
				after:             after,
			})
		}
	}
	return decls, nil
}

// collectScriptOrderDataSources returns unique coder_script_order
// data sources sorted by full Terraform address for deterministic
// rule processing and diagnostics.
func collectScriptOrderDataSources(
	modules []*tfjson.StateModule,
) ([]scriptOrderDataSource, error) {
	byAddr := map[string]scriptOrderDataSource{}
	for _, root := range modules {
		if err := walkStateModuleTree(root, func(module *tfjson.StateModule) error {
			for _, rsrc := range module.Resources {
				if rsrc == nil ||
					rsrc.Mode != tfjson.DataResourceMode ||
					rsrc.Type != "coder_script_order" {
					continue
				}
				byAddr[rsrc.Address] = scriptOrderDataSource{
					address:       rsrc.Address,
					moduleAddress: module.Address,
					resource:      rsrc,
				}
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	addrs := slices.Sorted(maps.Keys(byAddr))
	srcs := make([]scriptOrderDataSource, 0, len(addrs))
	for _, addr := range addrs {
		srcs = append(srcs, byAddr[addr])
	}
	return srcs, nil
}

func parseScriptOrderRequirement(raw string) (scriptOrderRequirement, error) {
	switch scriptOrderRequirement(raw) {
	case "", scriptOrderRequirementSuccess:
		return scriptOrderRequirementSuccess, nil
	case scriptOrderRequirementCompletion:
		return scriptOrderRequirementCompletion, nil
	default:
		return "", xerrors.Errorf(
			"requires must be %q or %q, got %q",
			scriptOrderRequirementSuccess, scriptOrderRequirementCompletion, raw,
		)
	}
}

func parseScriptOrderPhase(raw string) (scriptOrderPhase, error) {
	switch scriptOrderPhase(raw) {
	case "":
		return "", nil
	case scriptOrderPhaseStart:
		return scriptOrderPhaseStart, nil
	case scriptOrderPhaseStop:
		return scriptOrderPhaseStop, nil
	default:
		return "", xerrors.Errorf(
			"phase must be %q or %q, got %q",
			scriptOrderPhaseStart, scriptOrderPhaseStop, raw,
		)
	}
}

// resolveScriptOrderSelectors resolves one run or after field. A
// selector naming a declared child module call may expand to no
// scripts, but a script selector must resolve to at least one
// concrete instance.
func resolveScriptOrderSelectors(
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
	dataSource scriptOrderDataSource,
	ruleIndex int,
	selectorField string,
	rawSelectors []string,
) ([]resolvedScriptOrderSelector, error) {
	if len(rawSelectors) == 0 {
		return nil, scriptOrderRuleError(
			dataSource.address,
			ruleIndex,
			xerrors.Errorf("%s must contain at least one selector", selectorField),
		)
	}

	selectors := make([]resolvedScriptOrderSelector, 0, len(rawSelectors))
	for _, raw := range rawSelectors {
		selector, err := parseScriptOrderSelector(raw)
		if err != nil {
			return nil, scriptOrderRuleError(
				dataSource.address,
				ruleIndex,
				xerrors.Errorf("invalid %s selector %q: %w", selectorField, raw, err),
			)
		}

		resolution, err := resolveScriptOrderSelector(
			modules, planConfig, dataSource.moduleAddress, selector,
		)
		if err != nil {
			return nil, scriptOrderRuleError(
				dataSource.address,
				ruleIndex,
				xerrors.Errorf("resolve %s selector %q: %w", selectorField, raw, err),
			)
		}

		switch selector.kind {
		case scriptOrderSelectorScript:
			if len(resolution.addresses) == 0 {
				return nil, scriptOrderRuleError(
					dataSource.address,
					ruleIndex,
					xerrors.Errorf(
						"%s selector %q expanded to no coder_script resources",
						selectorField, raw,
					),
				)
			}
		case scriptOrderSelectorModule:
			if !resolution.moduleCallDeclared {
				return nil, scriptOrderRuleError(
					dataSource.address,
					ruleIndex,
					xerrors.Errorf(
						"%s selector %q does not name a declared child module call",
						selectorField, raw,
					),
				)
			}
		default:
			return nil, scriptOrderRuleError(
				dataSource.address,
				ruleIndex,
				xerrors.Errorf("developer error: %s selector %q has unknown kind %d",
					selectorField, raw, selector.kind),
			)
		}

		selectors = append(selectors, resolvedScriptOrderSelector{
			field:     selectorField,
			raw:       raw,
			kind:      selector.kind,
			addresses: resolution.addresses,
		})
	}
	return selectors, nil
}

func scriptOrderRuleError(dataSourceAddress string, ruleIndex int, err error) error {
	return xerrors.Errorf(
		"script order data source %q rule %d: %w",
		dataSourceAddress, ruleIndex, err,
	)
}
