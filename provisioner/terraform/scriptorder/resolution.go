package scriptorder

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"
)

type script struct {
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

type addressSelection struct {
	selector string
	address  string
}

type resolvedRule struct {
	dataSourceAddress string
	ruleIndex         int
	runtimeAddress    string
	phase             Phase
	requirement       Requirement
	run               []resolvedSelector
	after             []resolvedSelector
}

type phaseFilterWarning struct {
	dataSourceAddress            string
	ruleIndex                    int
	inferredPhase                Phase
	moduleSelectorsWithOmissions []string
}

func (w phaseFilterWarning) String() string {
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

// resolvedOrder contains phase- and runtime-resolved rules ready for
// graph construction, plus warnings produced during resolution.
type resolvedOrder struct {
	rules    []resolvedRule
	warnings []phaseFilterWarning
}

// Order contains one deterministic dependency graph per runtime and
// lifecycle phase. Independent groups within the same runtime and
// phase are disconnected components of the same graph.
type Order struct {
	Graphs []Graph
}

// Graph contains dependencies for one runtime and lifecycle
// phase. Scripts in the graph are identified by the dependent and
// prerequisite addresses in Dependencies.
type Graph struct {
	RuntimeAddress string
	Phase          Phase
	Dependencies   []Dependency
}

// Dependency makes DependentAddress wait for PrerequisiteAddress.
type Dependency struct {
	DependentAddress    string
	PrerequisiteAddress string
	Requirement         Requirement
}

// resolveOrder collects rule declarations and resolves their selectors,
// lifecycle phases, and runtimes. It does not construct dependency graphs.
func resolveOrder(
	modules []*tfjson.StateModule,
	planConfig *tfjson.Config,
	scripts map[string]script,
) (resolvedOrder, error) {
	declarations, err := collectRuleDeclarations(modules, planConfig)
	if err != nil {
		return resolvedOrder{}, err
	}

	result := resolvedOrder{}
	for _, dec := range declarations {
		rule, warning, err := resolveRule(dec, scripts)
		if err != nil {
			return resolvedOrder{}, err
		}
		if warning != nil {
			result.warnings = append(result.warnings, *warning)
		}
		// A rule with an empty run or after address set contributes
		// no dependency edges.
		if !hasResolvedAddresses(rule.run) || !hasResolvedAddresses(rule.after) {
			continue
		}
		result.rules = append(result.rules, rule)
	}
	return result, nil
}

// resolveRule resolves a declaration to one lifecycle phase, filters
// module selectors to that phase, and validates script and runtime
// compatibility. It returns a warning when inferred-phase filtering
// omits scripts.
func resolveRule(
	declaration ruleDeclaration,
	scripts map[string]script,
) (resolvedRule, *phaseFilterWarning, error) {
	err := validateSelectedScripts(declaration, scripts)
	if err != nil {
		return resolvedRule{}, nil, err
	}

	phase, inferred, err := determineRulePhase(declaration, scripts)
	if err != nil {
		return resolvedRule{}, nil, err
	}
	err = validateResourceSelectorPhases(declaration, phase, scripts)
	if err != nil {
		return resolvedRule{}, nil, err
	}

	run, runOmissions := filterModuleSelectorAddressesByPhase(
		declaration.run, phase, scripts,
	)
	after, afterOmissions := filterModuleSelectorAddressesByPhase(
		declaration.after, phase, scripts,
	)

	var runtimeAddr string
	if hasResolvedAddresses(run) && hasResolvedAddresses(after) {
		runtimeAddr, err = validateRuleRuntime(
			declaration, run, after, scripts,
		)
		if err != nil {
			return resolvedRule{}, nil, err
		}
		err = validateNoSelfDependency(declaration, run, after)
		if err != nil {
			return resolvedRule{}, nil, err
		}
	}

	var warning *phaseFilterWarning
	if inferred {
		selectorsWithOmissions := map[string]struct{}{}
		for _, selector := range slices.Concat(runOmissions, afterOmissions) {
			selectorsWithOmissions[selector] = struct{}{}
		}
		if len(selectorsWithOmissions) > 0 {
			warning = &phaseFilterWarning{
				dataSourceAddress:            declaration.dataSourceAddress,
				ruleIndex:                    declaration.ruleIndex,
				inferredPhase:                phase,
				moduleSelectorsWithOmissions: slices.Sorted(maps.Keys(selectorsWithOmissions)),
			}
		}
	}

	return resolvedRule{
		dataSourceAddress: declaration.dataSourceAddress,
		ruleIndex:         declaration.ruleIndex,
		runtimeAddress:    runtimeAddr,
		phase:             phase,
		requirement:       declaration.requirement,
		run:               run,
		after:             after,
	}, warning, nil
}

func validateSelectedScripts(
	declaration ruleDeclaration,
	scripts map[string]script,
) error {
	for _, selector := range slices.Concat(declaration.run, declaration.after) {
		for _, address := range selector.addresses {
			script, ok := scripts[address]
			if !ok {
				return ruleError(
					declaration.dataSourceAddress,
					declaration.ruleIndex,
					xerrors.Errorf(
						"%s selector %q expanded to %q, but script %q was not found",
						selector.field, selector.raw, selector.addresses, address,
					),
				)
			}
			if script.runOnStart && script.runOnStop {
				return ruleError(
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
				return ruleError(
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

// determineRulePhase returns the rule phase and whether it was
// inferred rather than explicitly declared. An empty phase with
// inferred false means every selector resolved only to declared
// script resources or module calls with no scripts.
func determineRulePhase(
	declaration ruleDeclaration,
	scripts map[string]script,
) (Phase, bool, error) {
	if declaration.declaredPhase != "" {
		return declaration.declaredPhase, false, nil
	}

	scriptSelectorPhases := collectObservedPhases(
		declaration.run, declaration.after, scripts, selectorScript,
	)
	// Any two distinct observed phases are sufficient to demonstrate a conflict.
	if len(scriptSelectorPhases) > 1 {
		one := scriptSelectorPhases[0]
		two := scriptSelectorPhases[1]
		return "", false, ruleError(
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

	moduleSelectorPhases := collectObservedPhases(
		declaration.run, declaration.after, scripts, selectorModule,
	)
	if len(moduleSelectorPhases) > 1 {
		one := moduleSelectorPhases[0]
		two := moduleSelectorPhases[1]
		return "", false, ruleError(
			declaration.dataSourceAddress,
			declaration.ruleIndex,
			xerrors.Errorf(
				"%s selector %q selects %s script %q, "+
					"but %s selector %q selects %s script %q; "+
					"phase cannot be inferred from module selectors that "+
					"expand to both start and stop scripts; set phase to %q or %q",
				one.selector.field, one.selector.raw, one.phase, one.address,
				two.selector.field, two.selector.raw, two.phase, two.address,
				PhaseStart, PhaseStop,
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

// observedPhase records the first selector and script address
// observed for a lifecycle phase.
type observedPhase struct {
	phase    Phase
	selector resolvedSelector
	address  string
}

// collectObservedPhases returns the first selector and script address
// observed for each phase among selectors of the requested kind. Only
// one observation per phase is needed to detect and report phase
// conflicts. Results are sorted by phase for deterministic
// diagnostics.
func collectObservedPhases(
	run []resolvedSelector,
	after []resolvedSelector,
	scripts map[string]script,
	kind selectorKind,
) []observedPhase {
	observed := map[Phase]observedPhase{}
	for _, selector := range slices.Concat(run, after) {
		if selector.kind != kind {
			continue
		}
		for _, address := range selector.addresses {
			phase := scriptPhase(scripts[address])
			if _, ok := observed[phase]; !ok {
				observed[phase] = observedPhase{
					phase:    phase,
					selector: selector,
					address:  address,
				}
			}
		}
	}
	phases := slices.Sorted(maps.Keys(observed))
	result := make([]observedPhase, 0, len(phases))
	for _, phase := range phases {
		result = append(result, observed[phase])
	}
	return result
}

func validateResourceSelectorPhases(
	declaration ruleDeclaration,
	phase Phase,
	scripts map[string]script,
) error {
	for _, selector := range slices.Concat(declaration.run, declaration.after) {
		if selector.kind != selectorScript {
			continue
		}
		for _, address := range selector.addresses {
			actual := scriptPhase(scripts[address])
			if actual != phase {
				return ruleError(
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

// filterModuleSelectorAddressesByPhase filters each module selector's
// expanded script addresses to those matching `phase`. coder_script
// resource selectors are unchanged. The second return value
// identifies module selectors from which one or more addresses were
// omitted.
func filterModuleSelectorAddressesByPhase(
	selectors []resolvedSelector,
	phase Phase,
	scripts map[string]script,
) ([]resolvedSelector, []string) {
	result := make([]resolvedSelector, 0, len(selectors))
	var filtered []string
	for _, selector := range selectors {
		// determineRulePhase returns an empty phase only
		// when every selector names a declared script resource or
		// module call that expanded to no scripts.
		if selector.kind != selectorModule || phase == "" {
			result = append(result, selector)
			continue
		}

		addrs := make([]string, 0, len(selector.addresses))
		for _, addr := range selector.addresses {
			if scriptPhase(scripts[addr]) == phase {
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

func validateRuleRuntime(
	declaration ruleDeclaration,
	run []resolvedSelector,
	after []resolvedSelector,
	scripts map[string]script,
) (string, error) {
	addrs := uniqueResolvedAddresses(run, after)
	if len(addrs) == 0 {
		return "", nil
	}

	// Comparing every remaining script with the first proves that all
	// selected scripts share one runtime.
	firstAddr := addrs[0]
	firstScript := scripts[firstAddr]
	if firstScript.runtimeAddress == "" {
		return "", missingRuntimeError(declaration, firstAddr, run, after)
	}
	for _, addr := range addrs[1:] {
		script := scripts[addr]
		if script.runtimeAddress == "" {
			return "", missingRuntimeError(declaration, addr, run, after)
		}
		if script.runtimeAddress != firstScript.runtimeAddress {
			firstSelector := findResolvedSelector(firstAddr, run, after)
			selector := findResolvedSelector(addr, run, after)
			return "", ruleError(
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

func missingRuntimeError(
	declaration ruleDeclaration,
	address string,
	run []resolvedSelector,
	after []resolvedSelector,
) error {
	selector := findResolvedSelector(address, run, after)
	return ruleError(
		declaration.dataSourceAddress,
		declaration.ruleIndex,
		xerrors.Errorf(
			"%s selector %q selects script %q, but it could not be associated with an agent or devcontainer subagent",
			selector.field, selector.raw, address,
		),
	)
}

func validateNoSelfDependency(
	declaration ruleDeclaration,
	run []resolvedSelector,
	after []resolvedSelector,
) error {
	afterSelectorsByAddress := map[string]string{}
	for _, selection := range uniqueAddressSelections(after) {
		if _, ok := afterSelectorsByAddress[selection.address]; !ok {
			afterSelectorsByAddress[selection.address] = selection.selector
		}
	}
	for _, runSelection := range uniqueAddressSelections(run) {
		afterSelector, ok := afterSelectorsByAddress[runSelection.address]
		if ok {
			return ruleError(
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

func uniqueResolvedAddresses(
	selectorGroups ...[]resolvedSelector,
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

func hasResolvedAddresses(selectors []resolvedSelector) bool {
	for _, selector := range selectors {
		if len(selector.addresses) > 0 {
			return true
		}
	}
	return false
}

// uniqueAddressSelections retains the first selector that expands to each
// address so diagnostics remain deterministic.
func uniqueAddressSelections(
	selectors []resolvedSelector,
) []addressSelection {
	seen := map[string]struct{}{}
	var result []addressSelection
	for _, selector := range selectors {
		for _, addr := range selector.addresses {
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			result = append(result, addressSelection{
				selector: selector.raw,
				address:  addr,
			})
		}
	}
	return result
}

func scriptPhase(s script) Phase {
	switch {
	case s.runOnStart && !s.runOnStop:
		return PhaseStart
	case s.runOnStop && !s.runOnStart:
		return PhaseStop
	default:
		// Validation rejects scripts configured for both phases or neither.
		return ""
	}
}

func findResolvedSelector(
	address string,
	selectorGroups ...[]resolvedSelector,
) resolvedSelector {
	for _, selectors := range selectorGroups {
		for _, selector := range selectors {
			if slices.Contains(selector.addresses, address) {
				return selector
			}
		}
	}
	return resolvedSelector{}
}
