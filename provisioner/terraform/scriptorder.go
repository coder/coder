package terraform

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	stringutil "github.com/coder/coder/v2/coderd/util/strings"
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

// ScriptOrderRequirement describes the prerequisite outcome required
// by a dependency.
type ScriptOrderRequirement string

const (
	ScriptOrderRequirementSuccess    ScriptOrderRequirement = "success"
	ScriptOrderRequirementCompletion ScriptOrderRequirement = "completion"
)

// ScriptOrderPhase identifies the lifecycle phase, start or stop,
// containing a graph.
type ScriptOrderPhase string

const (
	ScriptOrderPhaseStart ScriptOrderPhase = "start"
	ScriptOrderPhaseStop  ScriptOrderPhase = "stop"
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
	// Rule's zero-based index in the data source, used for diagnostics.
	ruleIndex     int
	declaredPhase ScriptOrderPhase
	requirement   ScriptOrderRequirement
	run           []resolvedScriptOrderSelector
	after         []resolvedScriptOrderSelector
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

func parseScriptOrderRequirement(raw string) (ScriptOrderRequirement, error) {
	switch ScriptOrderRequirement(raw) {
	case "", ScriptOrderRequirementSuccess:
		return ScriptOrderRequirementSuccess, nil
	case ScriptOrderRequirementCompletion:
		return ScriptOrderRequirementCompletion, nil
	default:
		return "", xerrors.Errorf(
			"requires must be %q or %q, got %q",
			ScriptOrderRequirementSuccess, ScriptOrderRequirementCompletion, raw,
		)
	}
}

func parseScriptOrderPhase(raw string) (ScriptOrderPhase, error) {
	switch ScriptOrderPhase(raw) {
	case "":
		return "", nil
	case ScriptOrderPhaseStart:
		return ScriptOrderPhaseStart, nil
	case ScriptOrderPhaseStop:
		return ScriptOrderPhaseStop, nil
	default:
		return "", xerrors.Errorf(
			"phase must be %q or %q, got %q",
			ScriptOrderPhaseStart, ScriptOrderPhaseStop, raw,
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
		truncateScriptOrderDiagnosticValue(dataSourceAddress), ruleIndex, err,
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
	phase             ScriptOrderPhase
	requirement       ScriptOrderRequirement
	run               []resolvedScriptOrderSelector
	after             []resolvedScriptOrderSelector
}

type scriptOrderPhaseFilterWarning struct {
	dataSourceAddress            string
	ruleIndex                    int
	inferredPhase                ScriptOrderPhase
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

// ScriptOrder contains one deterministic dependency graph per runtime
// and lifecycle phase. Independent groups within the same runtime and
// phase are disconnected components of the same graph.
type ScriptOrder struct {
	Graphs []ScriptOrderGraph
}

// ScriptOrderGraph contains dependencies for one runtime and
// lifecycle phase. Scripts in the graph are identified by the
// dependent and prerequisite addresses in Dependencies.
type ScriptOrderGraph struct {
	RuntimeAddress string
	Phase          ScriptOrderPhase
	Dependencies   []ScriptOrderDependency
}

// ScriptOrderDependency makes DependentAddress wait for PrerequisiteAddress.
type ScriptOrderDependency struct {
	DependentAddress    string
	PrerequisiteAddress string
	Requirement         ScriptOrderRequirement
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
) (ScriptOrderPhase, bool, error) {
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
				ScriptOrderPhaseStart, ScriptOrderPhaseStop,
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
	phase    ScriptOrderPhase
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
	observed := map[ScriptOrderPhase]scriptOrderObservedPhase{}
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
	phase ScriptOrderPhase,
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
	phase ScriptOrderPhase,
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
	for _, selection := range uniqueScriptOrderAddressSelections(after) {
		if _, ok := afterSelectorsByAddress[selection.address]; !ok {
			afterSelectorsByAddress[selection.address] = selection.selector
		}
	}
	for _, runSelection := range uniqueScriptOrderAddressSelections(run) {
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

// uniqueScriptOrderAddressSelections retains the first selector that
// expands to each address so diagnostics remain deterministic.
func uniqueScriptOrderAddressSelections(
	selectors []resolvedScriptOrderSelector,
) []scriptOrderAddressSelection {
	seen := map[string]struct{}{}
	var result []scriptOrderAddressSelection
	for _, selector := range selectors {
		for _, addr := range selector.addresses {
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			result = append(result, scriptOrderAddressSelection{
				selector: selector.raw,
				address:  addr,
			})
		}
	}
	return result
}

func scriptOrderScriptPhase(script scriptOrderScript) ScriptOrderPhase {
	switch {
	case script.runOnStart && !script.runOnStop:
		return ScriptOrderPhaseStart
	case script.runOnStop && !script.runOnStart:
		return ScriptOrderPhaseStop
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

// scriptOrderGraphKey contains only graph identity fields so it
// remains comparable for use as a map key.
type scriptOrderGraphKey struct {
	runtimeAddress string
	phase          ScriptOrderPhase
}

// scriptOrderEdge stores the dependency requirement and the Terraform
// declaration retained for validation diagnostics.
type scriptOrderEdge struct {
	requirement       ScriptOrderRequirement
	dataSourceAddress string
	ruleIndex         int
	runSelector       string
	afterSelector     string
}

type scriptOrderCycleEdge struct {
	dependentAddress    string
	prerequisiteAddress string
	edge                scriptOrderEdge
}

// scriptOrderGraphAccumulator holds mutable construction state for
// one runtime-and-phase graph before it is validated and converted to
// ScriptOrderGraph.
type scriptOrderGraphAccumulator struct {
	// Dependent address -> prerequisite address -> scriptOrderEdge
	dependencies map[string]map[string]scriptOrderEdge
	// Tracks unique edges for preallocating ScriptOrderGraph.Dependencies.
	dependencyCount int
}

type scriptOrderCombinationBudget struct {
	used  int
	limit int
}

const (
	// Diagnostic values are template-controlled, so bound their length
	// to keep provisioner diagnostics well below the dRPC message limit.
	maxScriptOrderDiagnosticValueRunes = 256
	// A cycle can contain many edges, so bound their count as well.
	maxScriptOrderCycleDiagnosticEdges = 20
)

// maxScriptOrderCandidateDependencies bounds the number of run/after
// script combinations before graph construction can exhaust provisioner
// CPU or memory. It applies across all runtime and phase graphs in one
// conversion and remains well above typical script counts.
// TODO(PLAT-554): Benchmark large script-order graphs and tune this limit.
const maxScriptOrderCandidateDependencies = 100_000

// buildScriptOrderGraphs combines resolved rules into deterministic
// graphs scoped by runtime and lifecycle phase. It deduplicates
// identical edges, enforces the dependency-combination limit, and
// rejects conflicting requirements and cycles.
func buildScriptOrderGraphs(rules []resolvedScriptOrderRule) (ScriptOrder, error) {
	return buildScriptOrderGraphsWithCombinationLimit(
		rules,
		maxScriptOrderCandidateDependencies,
	)
}

func buildScriptOrderGraphsWithCombinationLimit(
	rules []resolvedScriptOrderRule,
	combinationLimit int,
) (ScriptOrder, error) {
	graphs := map[scriptOrderGraphKey]*scriptOrderGraphAccumulator{}
	budget := scriptOrderCombinationBudget{limit: combinationLimit}
	for _, rule := range rules {
		key := scriptOrderGraphKey{
			runtimeAddress: rule.runtimeAddress,
			phase:          rule.phase,
		}
		graph := graphs[key]
		if graph == nil {
			graph = &scriptOrderGraphAccumulator{
				dependencies: map[string]map[string]scriptOrderEdge{},
			}
			graphs[key] = graph
		}

		if err := addScriptOrderRuleEdges(graph, rule, &budget); err != nil {
			return ScriptOrder{}, err
		}
	}

	keys := slices.SortedFunc(maps.Keys(graphs), func(a, b scriptOrderGraphKey) int {
		return cmp.Or(
			cmp.Compare(a.runtimeAddress, b.runtimeAddress),
			cmp.Compare(a.phase, b.phase),
		)
	})
	var result ScriptOrder
	for _, key := range keys {
		graph := graphs[key]
		if cycle := findScriptOrderCycle(graph); cycle != nil {
			return ScriptOrder{}, scriptOrderCycleError(cycle)
		}
		result.Graphs = append(result.Graphs, scriptOrderGraphResult(key, graph))
	}
	return result, nil
}

func addScriptOrderRuleEdges(
	graph *scriptOrderGraphAccumulator,
	rule resolvedScriptOrderRule,
	budget *scriptOrderCombinationBudget,
) error {
	runSelections := uniqueScriptOrderAddressSelections(rule.run)
	afterSelections := uniqueScriptOrderAddressSelections(rule.after)
	// Check the rule against the full limit before multiplying to
	// avoid integer overflow. The cumulative budget check follows
	// once the product is safe to calculate.
	if len(afterSelections) > 0 &&
		len(runSelections) > budget.limit/len(afterSelections) {
		return scriptOrderRuleError(
			rule.dataSourceAddress,
			rule.ruleIndex,
			xerrors.Errorf(
				"run selectors resolve to %d scripts and after selectors resolve to %d scripts; the rule creates one dependency for each run/after script combination, exceeding the overall limit of %d combinations across all script ordering rules",
				len(runSelections), len(afterSelections), budget.limit,
			),
		)
	}
	comboCount := len(runSelections) * len(afterSelections)
	if comboCount > budget.limit-budget.used {
		return scriptOrderRuleError(
			rule.dataSourceAddress,
			rule.ruleIndex,
			xerrors.Errorf(
				"script ordering rules are limited to %d run/after script combinations in total; %d from earlier rules together with %d from this rule exceed the limit",
				budget.limit, budget.used, comboCount,
			),
		)
	}
	budget.used += comboCount

	// A rule declares a dependency for every run/after selection
	// pair. Explicit transitive edges are retained because
	// requirements are evaluated independently for each edge.
	for _, run := range runSelections {
		prerequisites := graph.dependencies[run.address]
		if prerequisites == nil {
			prerequisites = map[string]scriptOrderEdge{}
			graph.dependencies[run.address] = prerequisites
		}
		for _, after := range afterSelections {
			if existing, ok := prerequisites[after.address]; ok {
				if existing.requirement != rule.requirement {
					return scriptOrderRuleError(
						rule.dataSourceAddress,
						rule.ruleIndex,
						xerrors.Errorf(
							"run selector %q and after selector %q declare script %q after %q with requires %q, conflicting with requires %q from data source %q rule %d",
							truncateScriptOrderDiagnosticValue(run.selector),
							truncateScriptOrderDiagnosticValue(after.selector),
							truncateScriptOrderDiagnosticValue(run.address),
							truncateScriptOrderDiagnosticValue(after.address),
							rule.requirement, existing.requirement,
							truncateScriptOrderDiagnosticValue(existing.dataSourceAddress),
							existing.ruleIndex,
						),
					)
				}
				continue
			}
			prerequisites[after.address] = scriptOrderEdge{
				requirement:       rule.requirement,
				dataSourceAddress: rule.dataSourceAddress,
				ruleIndex:         rule.ruleIndex,
				runSelector:       run.selector,
				afterSelector:     after.selector,
			}
			graph.dependencyCount++
		}
	}
	return nil
}

func scriptOrderGraphResult(
	key scriptOrderGraphKey,
	graph *scriptOrderGraphAccumulator,
) ScriptOrderGraph {
	dependencies := make([]ScriptOrderDependency, 0, graph.dependencyCount)
	for dependentAddr, prereq := range graph.dependencies {
		for prereqAddr, edge := range prereq {
			dependencies = append(dependencies, ScriptOrderDependency{
				DependentAddress:    dependentAddr,
				PrerequisiteAddress: prereqAddr,
				Requirement:         edge.requirement,
			})
		}
	}
	// Dependency order has no runtime semantics; sort it for deterministic output.
	slices.SortFunc(dependencies, func(a, b ScriptOrderDependency) int {
		return cmp.Or(
			cmp.Compare(a.DependentAddress, b.DependentAddress),
			cmp.Compare(a.PrerequisiteAddress, b.PrerequisiteAddress),
			cmp.Compare(a.Requirement, b.Requirement),
		)
	})
	return ScriptOrderGraph{
		RuntimeAddress: key.runtimeAddress,
		Phase:          key.phase,
		Dependencies:   dependencies,
	}
}

// findScriptOrderCycle returns the edges of one deterministic cycle
// in traversal order. It returns nil when the graph is acyclic.
func findScriptOrderCycle(
	graph *scriptOrderGraphAccumulator,
) []scriptOrderCycleEdge {
	// A valid graph can contain a chain as long as the overall
	// dependency limit permits, so use an explicit traversal stack
	// instead of recursion.
	const (
		unvisited = iota
		visiting
		visited
	)
	visitStates := map[string]int{}
	stackPositions := map[string]int{}
	type stackFrame struct {
		address       string
		prerequisites []string
		nextPrereqIdx int
	}

	// Sort roots and prerequisites so the same graph produces the
	// same cycle diagnostic.
	for _, rootAddr := range slices.Sorted(maps.Keys(graph.dependencies)) {
		if visitStates[rootAddr] != unvisited {
			continue
		}

		visitStates[rootAddr] = visiting
		stackPositions[rootAddr] = 0
		stack := []stackFrame{{
			address: rootAddr,
			prerequisites: slices.Sorted(
				maps.Keys(graph.dependencies[rootAddr]),
			),
		}}
		for len(stack) > 0 {
			frame := &stack[len(stack)-1]
			if frame.nextPrereqIdx == len(frame.prerequisites) {
				visitStates[frame.address] = visited
				delete(stackPositions, frame.address)
				stack = stack[:len(stack)-1]
				continue
			}

			prereq := frame.prerequisites[frame.nextPrereqIdx]
			frame.nextPrereqIdx++
			switch visitStates[prereq] {
			case unvisited:
				visitStates[prereq] = visiting
				stackPositions[prereq] = len(stack)
				stack = append(stack, stackFrame{
					address: prereq,
					prerequisites: slices.Sorted(
						maps.Keys(graph.dependencies[prereq]),
					),
				})
			case visiting:
				cycleStart := stackPositions[prereq]
				cycle := make([]scriptOrderCycleEdge, 0, len(stack)-cycleStart)
				for i := cycleStart; i < len(stack)-1; i++ {
					dependent := stack[i].address
					nextPrereq := stack[i+1].address
					cycle = append(cycle, scriptOrderCycleEdge{
						dependentAddress:    dependent,
						prerequisiteAddress: nextPrereq,
						edge:                graph.dependencies[dependent][nextPrereq],
					})
				}
				// Include the closing edge so diagnostics describe
				// the complete cycle.
				dependent := stack[len(stack)-1].address
				cycle = append(cycle, scriptOrderCycleEdge{
					dependentAddress:    dependent,
					prerequisiteAddress: prereq,
					edge:                graph.dependencies[dependent][prereq],
				})
				return cycle
			}
		}
	}
	return nil
}

func scriptOrderCycleError(cycle []scriptOrderCycleEdge) error {
	renderedEdges := min(len(cycle), maxScriptOrderCycleDiagnosticEdges)
	path := make([]string, 1, renderedEdges+1)
	path[0] = truncateScriptOrderDiagnosticValue(cycle[0].dependentAddress)
	details := make([]string, 0, renderedEdges)
	// Cycle detection follows dependent-to-prerequisite edges.
	// Present them in reverse so the arrows match execution order.
	for _, cycleEdge := range slices.Backward(cycle) {
		if len(details) == renderedEdges {
			break
		}
		dependentAddr := truncateScriptOrderDiagnosticValue(
			cycleEdge.dependentAddress,
		)
		prereqAddr := truncateScriptOrderDiagnosticValue(
			cycleEdge.prerequisiteAddress,
		)
		dataSourceAddr := truncateScriptOrderDiagnosticValue(
			cycleEdge.edge.dataSourceAddress,
		)
		runSelector := truncateScriptOrderDiagnosticValue(
			cycleEdge.edge.runSelector,
		)
		afterSelector := truncateScriptOrderDiagnosticValue(
			cycleEdge.edge.afterSelector,
		)
		path = append(path, dependentAddr)
		details = append(details, fmt.Sprintf(
			"%q after %q from run selector %q and after selector %q "+
				"(data source %q rule %d)",
			dependentAddr, prereqAddr, runSelector, afterSelector,
			dataSourceAddr, cycleEdge.edge.ruleIndex,
		))
	}
	omitted := len(cycle) - renderedEdges
	if omitted > 0 {
		path = append(path, "…", path[0])
	}
	message := fmt.Sprintf(
		"script order dependency cycle: %s; cycle edges: %s",
		strings.Join(path, " -> "),
		strings.Join(details, "; "),
	)
	if omitted > 0 {
		message += fmt.Sprintf("; %d cycle edges omitted", omitted)
	}
	return xerrors.New(message)
}

func truncateScriptOrderDiagnosticValue(value string) string {
	return stringutil.Truncate(
		value,
		maxScriptOrderDiagnosticValueRunes,
		stringutil.TruncateWithEllipsis,
	)
}
