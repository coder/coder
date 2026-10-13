package scriptorder

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"golang.org/x/xerrors"
)

type scriptLifecycle struct {
	// runOnStart and runOnStop mirror coder_script attributes so
	// validation can distinguish scripts configured for both
	// lifecycle phases or neither.
	runOnStart bool
	runOnStop  bool
	hasCron    bool
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

type preparedRule struct {
	identity    ruleIdentity
	phase       Phase
	requirement Requirement
	run         []resolvedSelector
	after       []resolvedSelector
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

// prepareOrder resolves selectors and lifecycle phases before runtime
// association. Rules with an empty side remain valid no-ops.
func (p *Program) prepareOrder() (Prepared, error) {
	declarations, err := p.collectRuleDeclarations()
	if err != nil {
		return Prepared{}, err
	}

	result := Prepared{}
	selectedAddresses := map[string]struct{}{}
	for i := range declarations {
		dec := &declarations[i]
		rule, warning, err := prepareRule(dec, p.stateIndex.scriptLifecycles)
		if err != nil {
			return Prepared{}, err
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
		selectorGroups := [...][]resolvedSelector{rule.run, rule.after}
		for _, selectors := range selectorGroups {
			for i := range selectors {
				for _, address := range selectors[i].addresses {
					selectedAddresses[address] = struct{}{}
				}
			}
		}
	}
	result.selectedScriptAddresses = slices.Sorted(maps.Keys(selectedAddresses))
	return result, nil
}

// prepareRule resolves a declaration to one lifecycle phase and
// filters module selectors to that phase.
func prepareRule(
	declaration *ruleDeclaration,
	scripts map[string]scriptLifecycle,
) (preparedRule, *phaseFilterWarning, error) {
	err := validateSelectedScripts(declaration, scripts)
	if err != nil {
		return preparedRule{}, nil, err
	}

	phase, inferred, err := determineRulePhase(declaration, scripts)
	if err != nil {
		return preparedRule{}, nil, err
	}
	err = validateResourceSelectorPhases(declaration, phase, scripts)
	if err != nil {
		return preparedRule{}, nil, err
	}

	run, runOmissions := filterModuleSelectorAddressesByPhase(
		declaration.run, phase, scripts,
	)
	after, afterOmissions := filterModuleSelectorAddressesByPhase(
		declaration.after, phase, scripts,
	)

	var warning *phaseFilterWarning
	if inferred {
		selectorsWithOmissions := map[string]struct{}{}
		omissionGroups := [...][]string{runOmissions, afterOmissions}
		for _, omissions := range omissionGroups {
			for _, selector := range omissions {
				selectorsWithOmissions[selector] = struct{}{}
			}
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

	return preparedRule{
		identity:    declaration.identity(),
		phase:       phase,
		requirement: declaration.requirement,
		run:         run,
		after:       after,
	}, warning, nil
}

// finalizeOrder validates runtime compatibility and self-dependencies
// after every selected script has been associated with a runtime.
func (p *Prepared) finalizeOrder(
	runtimeBindings map[string]RuntimeBinding,
) ([]resolvedRule, error) {
	var result []resolvedRule
	for i := range p.rules {
		rule := &p.rules[i]
		runtimeAddress, err := rule.validateRuntime(runtimeBindings)
		if err != nil {
			return nil, err
		}
		if err := rule.validateNoSelfDependency(); err != nil {
			return nil, err
		}
		result = append(result, resolvedRule{
			dataSourceAddress: rule.identity.dataSourceAddress,
			ruleIndex:         rule.identity.ruleIndex,
			runtimeAddress:    runtimeAddress,
			phase:             rule.phase,
			requirement:       rule.requirement,
			run:               rule.run,
			after:             rule.after,
		})
	}
	return result, nil
}

func validateSelectedScripts(
	declaration *ruleDeclaration,
	scripts map[string]scriptLifecycle,
) error {
	selectorGroups := [...][]resolvedSelector{declaration.run, declaration.after}
	for _, selectors := range selectorGroups {
		for i := range selectors {
			selector := &selectors[i]
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
					if script.hasCron {
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
	}
	return nil
}

// determineRulePhase returns the rule phase and whether it was
// inferred rather than explicitly declared. An empty phase with
// inferred false means every selector resolved only to declared
// script resources or module calls with no scripts.
func determineRulePhase(
	declaration *ruleDeclaration,
	scripts map[string]scriptLifecycle,
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
	scripts map[string]scriptLifecycle,
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
	declaration *ruleDeclaration,
	phase Phase,
	scripts map[string]scriptLifecycle,
) error {
	selectorGroups := [...][]resolvedSelector{declaration.run, declaration.after}
	for _, selectors := range selectorGroups {
		for i := range selectors {
			selector := &selectors[i]
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
	scripts map[string]scriptLifecycle,
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

func (r *preparedRule) validateRuntime(
	runtimeBindings map[string]RuntimeBinding,
) (string, error) {
	addrs := uniqueResolvedAddresses(r.run, r.after)
	if len(addrs) == 0 {
		return "", nil
	}

	// Comparing every remaining script with the first proves that all
	// selected scripts share one runtime.
	firstAddr := addrs[0]
	firstBinding := runtimeBindings[firstAddr]
	if firstBinding.RuntimeAddress == "" {
		return "", missingRuntimeError(r.identity, firstAddr, r.run, r.after)
	}
	for _, addr := range addrs[1:] {
		binding := runtimeBindings[addr]
		if binding.RuntimeAddress == "" {
			return "", missingRuntimeError(r.identity, addr, r.run, r.after)
		}
		if binding.RuntimeAddress != firstBinding.RuntimeAddress {
			firstSelector := findResolvedSelector(firstAddr, r.run, r.after)
			selector := findResolvedSelector(addr, r.run, r.after)
			return "", ruleError(
				r.identity.dataSourceAddress,
				r.identity.ruleIndex,
				xerrors.Errorf(
					"selector %q expands to script %q executed by %q, but "+
						"selector %q expands to script %q executed by %q; "+
						"scripts can be ordered only within the same agent or devcontainer subagent",
					firstSelector.raw, firstAddr, firstBinding.RuntimeAddress,
					selector.raw, addr, binding.RuntimeAddress,
				),
			)
		}
	}
	return firstBinding.RuntimeAddress, nil
}

func missingRuntimeError(
	identity ruleIdentity,
	address string,
	run []resolvedSelector,
	after []resolvedSelector,
) error {
	selector := findResolvedSelector(address, run, after)
	return ruleError(
		identity.dataSourceAddress,
		identity.ruleIndex,
		xerrors.Errorf(
			"%s selector %q selects script %q, but it could not be associated with an agent or devcontainer subagent",
			selector.field, selector.raw, address,
		),
	)
}

func (r *preparedRule) validateNoSelfDependency() error {
	afterSelectorsByAddress := map[string]string{}
	for _, selection := range uniqueAddressSelections(r.after) {
		if _, ok := afterSelectorsByAddress[selection.address]; !ok {
			afterSelectorsByAddress[selection.address] = selection.selector
		}
	}
	for _, runSelection := range uniqueAddressSelections(r.run) {
		afterSelector, ok := afterSelectorsByAddress[runSelection.address]
		if ok {
			return ruleError(
				r.identity.dataSourceAddress,
				r.identity.ruleIndex,
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
	for i := range selectors {
		selector := &selectors[i]
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

func scriptPhase(s scriptLifecycle) Phase {
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
