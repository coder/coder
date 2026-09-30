package terraform

import (
	"fmt"
	"maps"
	"slices"
	"strings"

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
	for _, step := range modulePath.steps {
		call := module.ModuleCalls[step.name]
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
	// empty for a declared unindexed coder_script resource or module
	// call with no script instances.
	addresses []string
}

// scriptOrderRuleDeclaration contains decoded rule values and resolved
// selectors before phase filtering.
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

// resolveScriptOrderSelectors resolves one run or after field. An
// unindexed selector naming a declared script resource or child module
// call may expand to no scripts.
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
				if selector.instanceKey != cty.NilVal {
					return nil, scriptOrderRuleError(
						dataSource.address,
						ruleIndex,
						xerrors.Errorf(
							"%s selector %q expanded to no coder_script resources",
							selectorField, raw,
						),
					)
				}
				if !resolution.scriptResourceDeclared {
					return nil, scriptOrderRuleError(
						dataSource.address,
						ruleIndex,
						xerrors.Errorf(
							"%s selector %q does not name a declared coder_script resource",
							selectorField, raw,
						),
					)
				}
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

type scriptOrderScript struct {
	// runtimeAddress is the Terraform resource address identifying
	// the workspace agent or devcontainer subagent that executes the
	// script, such as "coder_agent.main".
	runtimeAddress string
	// runOnStart and runOnStop mirror coder_script attributes so
	// validation can distinguish scripts configured for both
	// lifecycle phases or neither.
	runOnStart bool
	runOnStop  bool
	cron       string
}

type scriptOrderAddressSelection struct {
	selector string
	address  string
}

type resolvedScriptOrderRule struct {
	dataSourceAddress string
	ruleIndex         int
	runtimeAddress    string
	phase             scriptOrderPhase
	requirement       scriptOrderRequirement
	run               []resolvedScriptOrderSelector
	after             []resolvedScriptOrderSelector
}

type scriptOrderPhaseFilterWarning struct {
	dataSourceAddress            string
	ruleIndex                    int
	inferredPhase                scriptOrderPhase
	moduleSelectorsWithOmissions []string
}

func (w scriptOrderPhaseFilterWarning) String() string {
	selectors := make([]string, 0, len(w.moduleSelectorsWithOmissions))
	for _, selector := range w.moduleSelectorsWithOmissions {
		selectors = append(selectors, `"`+selector+`"`)
	}
	return fmt.Sprintf(
		"script order data source %q rule %d inferred phase %q and omitted "+
			"scripts in the other phase from module selectors: "+
			"%s; set phase = %q explicitly to make this filtering intentional",
		w.dataSourceAddress, w.ruleIndex, w.inferredPhase,
		strings.Join(selectors, ", "), w.inferredPhase,
	)
}

// resolvedScriptOrder contains phase- and runtime-resolved rules
// ready for graph construction, plus warnings produced during
// resolution.
type resolvedScriptOrder struct {
	rules    []resolvedScriptOrderRule
	warnings []scriptOrderPhaseFilterWarning
}

// resolveScriptOrder collects rule declarations and resolves their
// selectors, lifecycle phases, and runtimes. It does not construct
// dependency graphs.
func resolveScriptOrder(
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
	scripts map[string]scriptOrderScript,
) (resolvedScriptOrder, error) {
	declarations, err := collectScriptOrderRuleDeclarations(modules, planConfig)
	if err != nil {
		return resolvedScriptOrder{}, err
	}

	result := resolvedScriptOrder{}
	for _, dec := range declarations {
		rule, warning, err := resolveScriptOrderRule(dec, scripts)
		if err != nil {
			return resolvedScriptOrder{}, err
		}
		if warning != nil {
			result.warnings = append(result.warnings, *warning)
		}
		// A rule with an empty run or after address set contributes
		// no dependency edges.
		if !hasResolvedScriptOrderAddresses(rule.run) ||
			!hasResolvedScriptOrderAddresses(rule.after) {
			continue
		}
		result.rules = append(result.rules, rule)
	}
	return result, nil
}

// resolveScriptOrderRule resolves a declaration to one lifecycle
// phase, filters module selectors to that phase, and validates script
// and runtime compatibility. It returns a warning when inferred-phase
// filtering omits scripts.
func resolveScriptOrderRule(
	declaration scriptOrderRuleDeclaration,
	scripts map[string]scriptOrderScript,
) (resolvedScriptOrderRule, *scriptOrderPhaseFilterWarning, error) {
	err := validateScriptOrderSelectedScripts(declaration, scripts)
	if err != nil {
		return resolvedScriptOrderRule{}, nil, err
	}

	phase, inferred, err := determineScriptOrderRulePhase(declaration, scripts)
	if err != nil {
		return resolvedScriptOrderRule{}, nil, err
	}
	err = validateScriptOrderResourceSelectorPhases(declaration, phase, scripts)
	if err != nil {
		return resolvedScriptOrderRule{}, nil, err
	}

	run, runOmissions := filterScriptOrderModuleSelectorAddressesByPhase(
		declaration.run, phase, scripts,
	)
	after, afterOmissions := filterScriptOrderModuleSelectorAddressesByPhase(
		declaration.after, phase, scripts,
	)

	var runtimeAddr string
	if hasResolvedScriptOrderAddresses(run) && hasResolvedScriptOrderAddresses(after) {
		runtimeAddr, err = validateScriptOrderRuleRuntime(
			declaration, run, after, scripts,
		)
		if err != nil {
			return resolvedScriptOrderRule{}, nil, err
		}
		err = validateScriptOrderNoSelfDependency(declaration, run, after)
		if err != nil {
			return resolvedScriptOrderRule{}, nil, err
		}
	}

	var warning *scriptOrderPhaseFilterWarning
	if inferred {
		selectorsWithOmissions := map[string]struct{}{}
		for _, selector := range slices.Concat(runOmissions, afterOmissions) {
			selectorsWithOmissions[selector] = struct{}{}
		}
		if len(selectorsWithOmissions) > 0 {
			warning = &scriptOrderPhaseFilterWarning{
				dataSourceAddress:            declaration.dataSourceAddress,
				ruleIndex:                    declaration.ruleIndex,
				inferredPhase:                phase,
				moduleSelectorsWithOmissions: slices.Sorted(maps.Keys(selectorsWithOmissions)),
			}
		}
	}

	return resolvedScriptOrderRule{
		dataSourceAddress: declaration.dataSourceAddress,
		ruleIndex:         declaration.ruleIndex,
		runtimeAddress:    runtimeAddr,
		phase:             phase,
		requirement:       declaration.requirement,
		run:               run,
		after:             after,
	}, warning, nil
}

func validateScriptOrderSelectedScripts(
	declaration scriptOrderRuleDeclaration,
	scripts map[string]scriptOrderScript,
) error {
	for _, selector := range slices.Concat(declaration.run, declaration.after) {
		for _, address := range selector.addresses {
			script, ok := scripts[address]
			if !ok {
				return scriptOrderRuleError(
					declaration.dataSourceAddress,
					declaration.ruleIndex,
					xerrors.Errorf(
						"%s selector %q expanded to %q, but script %q was not found",
						selector.field, selector.raw, selector.addresses, address,
					),
				)
			}
			if script.runOnStart && script.runOnStop {
				return scriptOrderRuleError(
					declaration.dataSourceAddress,
					declaration.ruleIndex,
					xerrors.Errorf(
						"%s selector %q expanded to %q, but script %q has both run_on_start and run_on_stop enabled",
						selector.field, selector.raw, selector.addresses, address,
					),
				)
			}
			if !script.runOnStart && !script.runOnStop {
				reason := "has neither run_on_start nor run_on_stop enabled"
				if script.cron != "" {
					reason = "is cron-only; script ordering requires run_on_start or run_on_stop"
				}
				return scriptOrderRuleError(
					declaration.dataSourceAddress,
					declaration.ruleIndex,
					xerrors.Errorf(
						"%s selector %q expanded to %q, but script %q %s",
						selector.field, selector.raw, selector.addresses, address, reason,
					),
				)
			}
		}
	}
	return nil
}

// determineScriptOrderRulePhase returns the rule phase and whether it
// was inferred rather than explicitly declared. An empty phase with
// inferred false means every selector resolved only to declared
// script resources or module calls with no scripts.
func determineScriptOrderRulePhase(
	declaration scriptOrderRuleDeclaration,
	scripts map[string]scriptOrderScript,
) (scriptOrderPhase, bool, error) {
	if declaration.declaredPhase != "" {
		return declaration.declaredPhase, false, nil
	}

	scriptSelectorPhases := collectScriptOrderObservedPhases(
		declaration.run, declaration.after, scripts, scriptOrderSelectorScript,
	)
	// Any two distinct observed phases are sufficient to demonstrate a conflict.
	if len(scriptSelectorPhases) > 1 {
		one := scriptSelectorPhases[0]
		two := scriptSelectorPhases[1]
		return "", false, scriptOrderRuleError(
			declaration.dataSourceAddress,
			declaration.ruleIndex,
			xerrors.Errorf(
				"%s selector %q selects %s script %q, "+
					"but %s selector %q selects %s script %q; "+
					"coder_script resource selectors cannot mix start and stop scripts",
				one.selector.field, one.selector.raw, one.phase, one.address,
				two.selector.field, two.selector.raw, two.phase, two.address,
			),
		)
	}
	// coder_script resource selectors take precedence over module
	// selectors for phase inference.
	if len(scriptSelectorPhases) == 1 {
		return scriptSelectorPhases[0].phase, true, nil
	}

	moduleSelectorPhases := collectScriptOrderObservedPhases(
		declaration.run, declaration.after, scripts, scriptOrderSelectorModule,
	)
	if len(moduleSelectorPhases) > 1 {
		one := moduleSelectorPhases[0]
		two := moduleSelectorPhases[1]
		return "", false, scriptOrderRuleError(
			declaration.dataSourceAddress,
			declaration.ruleIndex,
			xerrors.Errorf(
				"%s selector %q selects %s script %q, "+
					"but %s selector %q selects %s script %q; "+
					"phase cannot be inferred from module selectors that "+
					"expand to both start and stop scripts; set phase to %q or %q",
				one.selector.field, one.selector.raw, one.phase, one.address,
				two.selector.field, two.selector.raw, two.phase, two.address,
				scriptOrderPhaseStart, scriptOrderPhaseStop,
			),
		)
	}
	if len(moduleSelectorPhases) == 1 {
		return moduleSelectorPhases[0].phase, true, nil
	}

	// Every selector names a declared script resource or module call
	// with no concrete scripts.
	return "", false, nil
}

// scriptOrderObservedPhase records the first selector and script
// address observed for a lifecycle phase.
type scriptOrderObservedPhase struct {
	phase    scriptOrderPhase
	selector resolvedScriptOrderSelector
	address  string
}

// collectScriptOrderObservedPhases returns the first selector and
// script address observed for each phase among selectors of the
// requested kind. Only one observation per phase is needed to detect
// and report phase conflicts. Results are sorted by phase for
// deterministic diagnostics.
func collectScriptOrderObservedPhases(
	run []resolvedScriptOrderSelector,
	after []resolvedScriptOrderSelector,
	scripts map[string]scriptOrderScript,
	kind scriptOrderSelectorKind,
) []scriptOrderObservedPhase {
	observed := map[scriptOrderPhase]scriptOrderObservedPhase{}
	for _, selector := range slices.Concat(run, after) {
		if selector.kind != kind {
			continue
		}
		for _, address := range selector.addresses {
			phase := scriptOrderScriptPhase(scripts[address])
			if _, ok := observed[phase]; !ok {
				observed[phase] = scriptOrderObservedPhase{
					phase:    phase,
					selector: selector,
					address:  address,
				}
			}
		}
	}
	phases := slices.Sorted(maps.Keys(observed))
	result := make([]scriptOrderObservedPhase, 0, len(phases))
	for _, phase := range phases {
		result = append(result, observed[phase])
	}
	return result
}

func validateScriptOrderResourceSelectorPhases(
	declaration scriptOrderRuleDeclaration,
	phase scriptOrderPhase,
	scripts map[string]scriptOrderScript,
) error {
	for _, selector := range slices.Concat(declaration.run, declaration.after) {
		if selector.kind != scriptOrderSelectorScript {
			continue
		}
		for _, address := range selector.addresses {
			actual := scriptOrderScriptPhase(scripts[address])
			if actual != phase {
				return scriptOrderRuleError(
					declaration.dataSourceAddress,
					declaration.ruleIndex,
					xerrors.Errorf(
						"%s selector %q selects %s script %q, but the rule phase is %q",
						selector.field, selector.raw, actual, address, phase,
					),
				)
			}
		}
	}
	return nil
}

// filterScriptOrderModuleSelectorAddressesByPhase filters each module
// selector's expanded script addresses to those matching `phase`.
// coder_script resource selectors are unchanged. The second return
// value identifies module selectors from which one or more addresses
// were omitted.
func filterScriptOrderModuleSelectorAddressesByPhase(
	selectors []resolvedScriptOrderSelector,
	phase scriptOrderPhase,
	scripts map[string]scriptOrderScript,
) ([]resolvedScriptOrderSelector, []string) {
	result := make([]resolvedScriptOrderSelector, 0, len(selectors))
	var filtered []string
	for _, selector := range selectors {
		// determineScriptOrderRulePhase returns an empty phase only
		// when every selector names a declared script resource or
		// module call that expanded to no scripts.
		if selector.kind != scriptOrderSelectorModule || phase == "" {
			result = append(result, selector)
			continue
		}

		addrs := make([]string, 0, len(selector.addresses))
		for _, addr := range selector.addresses {
			if scriptOrderScriptPhase(scripts[addr]) == phase {
				addrs = append(addrs, addr)
				continue
			}
			filtered = append(filtered, selector.raw)
		}
		selector.addresses = addrs
		result = append(result, selector)
	}
	return result, filtered
}

func validateScriptOrderRuleRuntime(
	declaration scriptOrderRuleDeclaration,
	run []resolvedScriptOrderSelector,
	after []resolvedScriptOrderSelector,
	scripts map[string]scriptOrderScript,
) (string, error) {
	addrs := uniqueResolvedScriptOrderAddresses(run, after)
	if len(addrs) == 0 {
		return "", nil
	}

	// Comparing every remaining script with the first proves that all
	// selected scripts share one runtime.
	firstAddr := addrs[0]
	firstScript := scripts[firstAddr]
	if firstScript.runtimeAddress == "" {
		return "", scriptOrderMissingRuntimeError(declaration, firstAddr, run, after)
	}
	for _, addr := range addrs[1:] {
		script := scripts[addr]
		if script.runtimeAddress == "" {
			return "", scriptOrderMissingRuntimeError(declaration, addr, run, after)
		}
		if script.runtimeAddress != firstScript.runtimeAddress {
			firstSelector := findResolvedScriptOrderSelector(firstAddr, run, after)
			selector := findResolvedScriptOrderSelector(addr, run, after)
			return "", scriptOrderRuleError(
				declaration.dataSourceAddress,
				declaration.ruleIndex,
				xerrors.Errorf(
					"selector %q expands to script %q executed by %q, but "+
						"selector %q expands to script %q executed by %q; "+
						"scripts can be ordered only within the same agent or devcontainer subagent",
					firstSelector.raw, firstAddr, firstScript.runtimeAddress,
					selector.raw, addr, script.runtimeAddress,
				),
			)
		}
	}
	return firstScript.runtimeAddress, nil
}

func scriptOrderMissingRuntimeError(
	declaration scriptOrderRuleDeclaration,
	address string,
	run []resolvedScriptOrderSelector,
	after []resolvedScriptOrderSelector,
) error {
	selector := findResolvedScriptOrderSelector(address, run, after)
	return scriptOrderRuleError(
		declaration.dataSourceAddress,
		declaration.ruleIndex,
		xerrors.Errorf(
			"%s selector %q selects script %q, but it could not be associated with an agent or devcontainer subagent",
			selector.field, selector.raw, address,
		),
	)
}

func validateScriptOrderNoSelfDependency(
	declaration scriptOrderRuleDeclaration,
	run []resolvedScriptOrderSelector,
	after []resolvedScriptOrderSelector,
) error {
	afterSelectorsByAddress := map[string]string{}
	for _, selection := range scriptOrderAddressSelections(after) {
		if _, ok := afterSelectorsByAddress[selection.address]; !ok {
			afterSelectorsByAddress[selection.address] = selection.selector
		}
	}
	for _, runSelection := range scriptOrderAddressSelections(run) {
		afterSelector, ok := afterSelectorsByAddress[runSelection.address]
		if ok {
			return scriptOrderRuleError(
				declaration.dataSourceAddress,
				declaration.ruleIndex,
				xerrors.Errorf(
					"run selector %q and after selector %q both select script %q; a script cannot depend on itself",
					runSelection.selector, afterSelector, runSelection.address,
				),
			)
		}
	}
	return nil
}

func uniqueResolvedScriptOrderAddresses(
	selectorGroups ...[]resolvedScriptOrderSelector,
) []string {
	addrs := map[string]struct{}{}
	for _, selectors := range selectorGroups {
		for _, selector := range selectors {
			for _, addr := range selector.addresses {
				addrs[addr] = struct{}{}
			}
		}
	}
	return slices.Sorted(maps.Keys(addrs))
}

func hasResolvedScriptOrderAddresses(selectors []resolvedScriptOrderSelector) bool {
	for _, selector := range selectors {
		if len(selector.addresses) > 0 {
			return true
		}
	}
	return false
}

func scriptOrderAddressSelections(
	selectors []resolvedScriptOrderSelector,
) []scriptOrderAddressSelection {
	var result []scriptOrderAddressSelection
	for _, selector := range selectors {
		for _, addr := range selector.addresses {
			result = append(result, scriptOrderAddressSelection{
				selector: selector.raw,
				address:  addr,
			})
		}
	}
	return result
}

func scriptOrderScriptPhase(script scriptOrderScript) scriptOrderPhase {
	switch {
	case script.runOnStart && !script.runOnStop:
		return scriptOrderPhaseStart
	case script.runOnStop && !script.runOnStart:
		return scriptOrderPhaseStop
	default:
		// Validation rejects scripts configured for both phases or neither.
		return ""
	}
}

func findResolvedScriptOrderSelector(
	address string,
	selectorGroups ...[]resolvedScriptOrderSelector,
) resolvedScriptOrderSelector {
	for _, selectors := range selectorGroups {
		for _, selector := range selectors {
			if slices.Contains(selector.addresses, address) {
				return selector
			}
		}
	}
	return resolvedScriptOrderSelector{}
}
