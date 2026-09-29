package scriptorder

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"
)

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
