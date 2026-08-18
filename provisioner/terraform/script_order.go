package terraform

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	tfaddr "github.com/hashicorp/go-terraform-address"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/xerrors"
)

type ScriptOrderRequirement string

const (
	ScriptOrderRequirementSuccess    ScriptOrderRequirement = "success"
	ScriptOrderRequirementCompletion ScriptOrderRequirement = "completion"
)

type ScriptOrderPhase string

const (
	ScriptOrderPhaseStartup  ScriptOrderPhase = "startup"
	ScriptOrderPhaseShutdown ScriptOrderPhase = "shutdown"
)

type scriptOrderAttributes struct {
	Rules []scriptOrderRuleAttributes `mapstructure:"rule"`
}

type scriptOrderRuleAttributes struct {
	Run      []string `mapstructure:"run"`
	After    []string `mapstructure:"after"`
	Requires string   `mapstructure:"requires"`
}

type scriptOrderScript struct {
	RuntimeAddress string
	RunOnStart     bool
	RunOnStop      bool
	Cron           string
}

type scriptOrderDataSource struct {
	address       string
	moduleAddress string
	resource      *tfjson.StateResource
}

type resolvedScriptOrderSelector struct {
	field     string
	raw       string
	addresses []string
}

type resolvedScriptOrderAddress struct {
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

type resolvedScriptOrder struct {
	rules []resolvedScriptOrderRule
}

type ScriptOrder struct {
	Graphs []ScriptOrderGraph
}

type ScriptOrderGraph struct {
	RuntimeAddress string
	Phase          ScriptOrderPhase
	Scripts        []string
	Dependencies   []ScriptOrderDependency
}

type ScriptOrderDependency struct {
	ScriptAddress       string
	PrerequisiteAddress string
	Requirement         ScriptOrderRequirement
}

type scriptOrderGraphKey struct {
	runtimeAddress string
	phase          ScriptOrderPhase
}

type scriptOrderEdgeKey struct {
	scriptAddress       string
	prerequisiteAddress string
}

type scriptOrderEdgeOrigin struct {
	dataSourceAddress string
	ruleIndex         int
	runSelector       string
	afterSelector     string
}

type scriptOrderEdge struct {
	requirement ScriptOrderRequirement
	origin      scriptOrderEdgeOrigin
}

type scriptOrderGraph struct {
	scripts map[string]struct{}
	edges   map[scriptOrderEdgeKey]scriptOrderEdge
}

func resolveScriptOrderRules(modules []*tfjson.StateModule, scripts map[string]scriptOrderScript) (resolvedScriptOrder, error) {
	dataSources, err := collectScriptOrderDataSources(modules)
	if err != nil {
		return resolvedScriptOrder{}, err
	}
	if len(dataSources) == 0 {
		return resolvedScriptOrder{}, nil
	}

	var result resolvedScriptOrder
	for _, dataSource := range dataSources {
		var attrs scriptOrderAttributes
		if err := mapstructure.Decode(dataSource.resource.AttributeValues, &attrs); err != nil {
			return resolvedScriptOrder{}, xerrors.Errorf("decode script order data source %q: %w", dataSource.address, err)
		}
		if len(attrs.Rules) == 0 {
			return resolvedScriptOrder{}, xerrors.Errorf("script order data source %q must contain at least one rule", dataSource.address)
		}

		for ruleIndex, rule := range attrs.Rules {
			requirement, err := parseScriptOrderRequirement(rule.Requires)
			if err != nil {
				return resolvedScriptOrder{}, xerrors.Errorf("script order data source %q rule %d: %w", dataSource.address, ruleIndex, err)
			}

			run, err := resolveScriptOrderSelectors(modules, dataSource, ruleIndex, "run", rule.Run)
			if err != nil {
				return resolvedScriptOrder{}, err
			}
			after, err := resolveScriptOrderSelectors(modules, dataSource, ruleIndex, "after", rule.After)
			if err != nil {
				return resolvedScriptOrder{}, err
			}
			if err := validateResolvedScriptOrderSelectors(dataSource, ruleIndex, run, scripts); err != nil {
				return resolvedScriptOrder{}, err
			}
			if err := validateResolvedScriptOrderSelectors(dataSource, ruleIndex, after, scripts); err != nil {
				return resolvedScriptOrder{}, err
			}

			runtimeAddress, phase, err := validateScriptOrderRuleScope(dataSource, ruleIndex, run, after, scripts)
			if err != nil {
				return resolvedScriptOrder{}, err
			}
			if err := validateScriptOrderNoSelfDependency(dataSource, ruleIndex, run, after); err != nil {
				return resolvedScriptOrder{}, err
			}

			result.rules = append(result.rules, resolvedScriptOrderRule{
				dataSourceAddress: dataSource.address,
				ruleIndex:         ruleIndex,
				runtimeAddress:    runtimeAddress,
				phase:             phase,
				requirement:       requirement,
				run:               run,
				after:             after,
			})
		}
	}
	return result, nil
}

func buildScriptOrder(modules []*tfjson.StateModule, scripts map[string]scriptOrderScript) (ScriptOrder, error) {
	resolved, err := resolveScriptOrderRules(modules, scripts)
	if err != nil {
		return ScriptOrder{}, err
	}
	if len(resolved.rules) == 0 {
		return ScriptOrder{}, nil
	}

	graphs := map[scriptOrderGraphKey]*scriptOrderGraph{}
	for _, rule := range resolved.rules {
		graphKey := scriptOrderGraphKey{
			runtimeAddress: rule.runtimeAddress,
			phase:          rule.phase,
		}
		graph := graphs[graphKey]
		if graph == nil {
			graph = &scriptOrderGraph{
				scripts: map[string]struct{}{},
				edges:   map[scriptOrderEdgeKey]scriptOrderEdge{},
			}
			graphs[graphKey] = graph
		}

		for _, address := range resolvedScriptOrderAddresses(rule.run) {
			graph.scripts[address] = struct{}{}
		}
		for _, address := range resolvedScriptOrderAddresses(rule.after) {
			graph.scripts[address] = struct{}{}
		}
		if err := addScriptOrderRuleEdges(graph, rule); err != nil {
			return ScriptOrder{}, err
		}
	}

	var result ScriptOrder
	graphKeys := slices.SortedFunc(maps.Keys(graphs), func(a, b scriptOrderGraphKey) int {
		if n := cmp.Compare(a.runtimeAddress, b.runtimeAddress); n != 0 {
			return n
		}
		return cmp.Compare(a.phase, b.phase)
	})
	for _, graphKey := range graphKeys {
		graph := graphs[graphKey]
		if cycle, origin := findScriptOrderCycle(graph); len(cycle) > 0 {
			return ScriptOrder{}, xerrors.Errorf("script order data source %q rule %d: run selector %q and after selector %q create dependency cycle: %s", origin.dataSourceAddress, origin.ruleIndex, origin.runSelector, origin.afterSelector,
				strings.Join(cycle, " -> "))
		}
		result.Graphs = append(result.Graphs, scriptOrderGraphResult(graphKey, graph))
	}
	return result, nil
}

func addScriptOrderRuleEdges(graph *scriptOrderGraph, rule resolvedScriptOrderRule) error {
	for _, runAddress := range expandResolvedScriptOrderAddresses(rule.run) {
		for _, afterAddress := range expandResolvedScriptOrderAddresses(rule.after) {
			edgeKey := scriptOrderEdgeKey{
				scriptAddress:       runAddress.address,
				prerequisiteAddress: afterAddress.address,
			}
			origin := scriptOrderEdgeOrigin{
				dataSourceAddress: rule.dataSourceAddress,
				ruleIndex:         rule.ruleIndex,
				runSelector:       runAddress.selector,
				afterSelector:     afterAddress.selector,
			}
			if existing, ok := graph.edges[edgeKey]; ok {
				if existing.requirement != rule.requirement {
					return xerrors.Errorf("script order data source %q rule %d: run selector %q and after selector %q declare script %q after %q with requires %q, conflicting with requires %q from data source %q rule %d", rule.dataSourceAddress, rule.ruleIndex, runAddress.selector, afterAddress.selector,
						runAddress.address, afterAddress.address, rule.requirement, existing.requirement,
						existing.origin.dataSourceAddress, existing.origin.ruleIndex)
				}
				continue
			}
			graph.edges[edgeKey] = scriptOrderEdge{
				requirement: rule.requirement,
				origin:      origin,
			}
		}
	}
	return nil
}

func scriptOrderGraphResult(graphKey scriptOrderGraphKey, graph *scriptOrderGraph) ScriptOrderGraph {
	dependencies := make([]ScriptOrderDependency, 0, len(graph.edges))
	for edgeKey, edge := range graph.edges {
		dependencies = append(dependencies, ScriptOrderDependency{
			ScriptAddress:       edgeKey.scriptAddress,
			PrerequisiteAddress: edgeKey.prerequisiteAddress,
			Requirement:         edge.requirement,
		})
	}
	slices.SortFunc(dependencies, func(a, b ScriptOrderDependency) int {
		if n := cmp.Compare(a.ScriptAddress, b.ScriptAddress); n != 0 {
			return n
		}
		if n := cmp.Compare(a.PrerequisiteAddress, b.PrerequisiteAddress); n != 0 {
			return n
		}
		return cmp.Compare(a.Requirement, b.Requirement)
	})
	return ScriptOrderGraph{
		RuntimeAddress: graphKey.runtimeAddress,
		Phase:          graphKey.phase,
		Scripts:        slices.Sorted(maps.Keys(graph.scripts)),
		Dependencies:   dependencies,
	}
}

func findScriptOrderCycle(graph *scriptOrderGraph) ([]string, scriptOrderEdgeOrigin) {
	adjacent := map[string][]string{}
	for edgeKey := range graph.edges {
		adjacent[edgeKey.scriptAddress] = append(adjacent[edgeKey.scriptAddress], edgeKey.prerequisiteAddress)
	}
	for address := range adjacent {
		slices.Sort(adjacent[address])
	}

	const (
		unvisited = iota
		visiting
		visited
	)
	states := map[string]int{}
	positions := map[string]int{}
	stack := make([]string, 0, len(graph.scripts))
	var cycle []string
	var origin scriptOrderEdgeOrigin
	var visit func(string) bool
	visit = func(address string) bool {
		states[address] = visiting
		positions[address] = len(stack)
		stack = append(stack, address)
		for _, prerequisite := range adjacent[address] {
			switch states[prerequisite] {
			case unvisited:
				if visit(prerequisite) {
					return true
				}
			case visiting:
				cycle = append(slices.Clone(stack[positions[prerequisite]:]), prerequisite)
				origin = graph.edges[scriptOrderEdgeKey{
					scriptAddress:       address,
					prerequisiteAddress: prerequisite,
				}].origin
				return true
			}
		}
		stack = stack[:len(stack)-1]
		delete(positions, address)
		states[address] = visited
		return false
	}

	for _, address := range slices.Sorted(maps.Keys(graph.scripts)) {
		if states[address] == unvisited && visit(address) {
			return cycle, origin
		}
	}
	return nil, scriptOrderEdgeOrigin{}
}

func collectScriptOrderDataSources(modules []*tfjson.StateModule) ([]scriptOrderDataSource, error) {
	byAddress := map[string]scriptOrderDataSource{}
	for _, rootModule := range modules {
		if err := forEachModule(rootModule, func(module *tfjson.StateModule) error {
			for _, resource := range module.Resources {
				if resource == nil || resource.Mode != tfjson.DataResourceMode || resource.Type != "coder_script_order" {
					continue
				}
				byAddress[resource.Address] = scriptOrderDataSource{
					address:       resource.Address,
					moduleAddress: module.Address,
					resource:      resource,
				}
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	addresses := slices.Sorted(maps.Keys(byAddress))
	dataSources := make([]scriptOrderDataSource, 0, len(addresses))
	for _, address := range addresses {
		dataSources = append(dataSources, byAddress[address])
	}
	return dataSources, nil
}

func parseScriptOrderRequirement(raw string) (ScriptOrderRequirement, error) {
	switch ScriptOrderRequirement(raw) {
	case "", ScriptOrderRequirementSuccess:
		return ScriptOrderRequirementSuccess, nil
	case ScriptOrderRequirementCompletion:
		return ScriptOrderRequirementCompletion, nil
	default:
		return "", xerrors.Errorf("requires must be %q or %q, got %q", ScriptOrderRequirementSuccess, ScriptOrderRequirementCompletion, raw)
	}
}

func resolveScriptOrderSelectors(
	modules []*tfjson.StateModule,
	dataSource scriptOrderDataSource,
	ruleIndex int,
	field string,
	rawSelectors []string,
) ([]resolvedScriptOrderSelector, error) {
	if len(rawSelectors) == 0 {
		return nil, xerrors.Errorf("script order data source %q rule %d: %s must contain at least one selector", dataSource.address, ruleIndex, field)
	}

	selectors := make([]resolvedScriptOrderSelector, 0, len(rawSelectors))
	for _, raw := range rawSelectors {
		selector, err := parseScriptOrderSelector(raw)
		if err != nil {
			return nil, xerrors.Errorf("script order data source %q rule %d: %s selector %q: %w", dataSource.address, ruleIndex, field, raw, err)
		}
		addresses, err := resolveScriptOrderSelector(modules, dataSource.moduleAddress, selector)
		if err != nil {
			return nil, xerrors.Errorf("script order data source %q rule %d: resolve %s selector %q: %w", dataSource.address, ruleIndex, field, raw, err)
		}
		if len(addresses) == 0 {
			return nil, xerrors.Errorf("script order data source %q rule %d: %s selector %q expanded to no coder_script resources; legacy coder_agent inline scripts cannot be selected", dataSource.address, ruleIndex, field, raw)
		}
		selectors = append(selectors, resolvedScriptOrderSelector{
			field:     field,
			raw:       raw,
			addresses: addresses,
		})
	}
	return selectors, nil
}

func validateResolvedScriptOrderSelectors(
	dataSource scriptOrderDataSource,
	ruleIndex int,
	selectors []resolvedScriptOrderSelector,
	scripts map[string]scriptOrderScript,
) error {
	for _, selector := range selectors {
		for _, address := range selector.addresses {
			script, ok := scripts[address]
			if !ok {
				return xerrors.Errorf("script order data source %q rule %d: %s selector %q expanded to %q, but script %q was not found during graph conversion", dataSource.address, ruleIndex, selector.field, selector.raw, selector.addresses, address)
			}
			if script.RunOnStart && script.RunOnStop {
				return xerrors.Errorf("script order data source %q rule %d: %s selector %q expanded to %q, but script %q has both startup and shutdown phases enabled", dataSource.address, ruleIndex, selector.field, selector.raw, selector.addresses, address)
			}
			if !script.RunOnStart && !script.RunOnStop {
				reason := "has no startup or shutdown lifecycle phase"
				if script.Cron != "" {
					reason = "is cron-only and has no startup or shutdown lifecycle phase"
				}
				return xerrors.Errorf("script order data source %q rule %d: %s selector %q expanded to %q, but script %q %s", dataSource.address, ruleIndex, selector.field, selector.raw, selector.addresses, address, reason)
			}
			if script.RuntimeAddress == "" {
				return xerrors.Errorf("script order data source %q rule %d: %s selector %q expanded to %q, but script %q could not be associated with an agent runtime", dataSource.address, ruleIndex, selector.field, selector.raw, selector.addresses, address)
			}
		}
	}
	return nil
}

func validateScriptOrderRuleScope(
	dataSource scriptOrderDataSource,
	ruleIndex int,
	run []resolvedScriptOrderSelector,
	after []resolvedScriptOrderSelector,
	scripts map[string]scriptOrderScript,
) (string, ScriptOrderPhase, error) {
	runAddresses := resolvedScriptOrderAddresses(run)
	afterAddresses := resolvedScriptOrderAddresses(after)
	allAddresses := append(slices.Clone(runAddresses), afterAddresses...)
	firstAddress := allAddresses[0]
	firstScript := scripts[firstAddress]
	firstPhase := scriptOrderScriptPhase(firstScript)
	for _, address := range allAddresses[1:] {
		script := scripts[address]
		selector := findResolvedScriptOrderSelector(address, run, after)
		firstSelector := findResolvedScriptOrderSelector(firstAddress, run, after)
		if script.RuntimeAddress != firstScript.RuntimeAddress {
			return "", "", xerrors.Errorf("script order data source %q rule %d: selector %q expands to script %q on agent runtime %q, but selector %q expands to script %q on agent runtime %q; cross-agent dependencies are not supported", dataSource.address, ruleIndex,
				firstSelector.raw, firstAddress, firstScript.RuntimeAddress,
				selector.raw, address, script.RuntimeAddress)
		}
		phase := scriptOrderScriptPhase(script)
		if phase != firstPhase {
			return "", "", xerrors.Errorf("script order data source %q rule %d: selector %q expands to %s script %q, but selector %q expands to %s script %q; startup and shutdown phases cannot be mixed", dataSource.address, ruleIndex,
				firstSelector.raw, firstPhase, firstAddress,
				selector.raw, phase, address)
		}
	}
	return firstScript.RuntimeAddress, firstPhase, nil
}

func validateScriptOrderNoSelfDependency(
	dataSource scriptOrderDataSource,
	ruleIndex int,
	run []resolvedScriptOrderSelector,
	after []resolvedScriptOrderSelector,
) error {
	for _, runAddress := range expandResolvedScriptOrderAddresses(run) {
		for _, afterAddress := range expandResolvedScriptOrderAddresses(after) {
			if runAddress.address == afterAddress.address {
				return xerrors.Errorf("script order data source %q rule %d: run selector %q and after selector %q both expand to script %q; a script cannot depend on itself", dataSource.address, ruleIndex, runAddress.selector, afterAddress.selector, runAddress.address)
			}
		}
	}
	return nil
}

func resolvedScriptOrderAddresses(selectors []resolvedScriptOrderSelector) []string {
	addresses := map[string]struct{}{}
	for _, selector := range selectors {
		for _, address := range selector.addresses {
			addresses[address] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(addresses))
}

func expandResolvedScriptOrderAddresses(selectors []resolvedScriptOrderSelector) []resolvedScriptOrderAddress {
	var addresses []resolvedScriptOrderAddress
	for _, selector := range selectors {
		for _, address := range selector.addresses {
			addresses = append(addresses, resolvedScriptOrderAddress{
				selector: selector.raw,
				address:  address,
			})
		}
	}
	return addresses
}

func scriptOrderScriptPhase(script scriptOrderScript) ScriptOrderPhase {
	if script.RunOnStart {
		return ScriptOrderPhaseStartup
	}
	return ScriptOrderPhaseShutdown
}

func findResolvedScriptOrderSelector(address string, selectorGroups ...[]resolvedScriptOrderSelector) resolvedScriptOrderSelector {
	for _, selectors := range selectorGroups {
		for _, selector := range selectors {
			if slices.Contains(selector.addresses, address) {
				return selector
			}
		}
	}
	return resolvedScriptOrderSelector{}
}

type scriptOrderSelectorKind int

const (
	_ scriptOrderSelectorKind = iota
	scriptOrderSelectorScript
	scriptOrderSelectorModule
)

type scriptOrderSelector struct {
	kind  scriptOrderSelectorKind
	name  string
	index string
}

func parseScriptOrderSelector(raw string) (scriptOrderSelector, error) {
	address, err := tfaddr.NewAddress(raw)
	if err != nil {
		return scriptOrderSelector{}, xerrors.Errorf("parse script order selector %q: %w", raw, err)
	}
	if len(address.ModulePath) != 0 {
		return scriptOrderSelector{}, xerrors.Errorf("script order selector %q must be relative to its declaring module", raw)
	}

	switch address.ResourceSpec.Type {
	case "coder_script":
		return scriptOrderSelector{
			kind:  scriptOrderSelectorScript,
			name:  address.ResourceSpec.Name,
			index: address.ResourceSpec.Index.String(),
		}, nil
	case "module":
		if address.ResourceSpec.Index.String() != "" {
			return scriptOrderSelector{}, xerrors.Errorf("module selector %q must select all module instances", raw)
		}
		return scriptOrderSelector{
			kind: scriptOrderSelectorModule,
			name: address.ResourceSpec.Name,
		}, nil
	default:
		return scriptOrderSelector{}, xerrors.Errorf("script order selector %q must select a coder_script or module", raw)
	}
}

func resolveScriptOrderSelector(modules []*tfjson.StateModule, moduleAddress string, selector scriptOrderSelector) ([]string, error) {
	resolved := map[string]struct{}{}
	for _, rootModule := range modules {
		err := forEachModule(rootModule, func(module *tfjson.StateModule) error {
			if module.Address != moduleAddress {
				return nil
			}

			switch selector.kind {
			case scriptOrderSelectorScript:
				return resolveScriptSelector(module, selector, resolved)
			case scriptOrderSelectorModule:
				return resolveModuleSelector(module, selector, resolved)
			default:
				return xerrors.Errorf("unknown script order selector kind %d", selector.kind)
			}
		})
		if err != nil {
			return nil, err
		}
	}
	return slices.Sorted(maps.Keys(resolved)), nil
}

func resolveScriptSelector(module *tfjson.StateModule, selector scriptOrderSelector, resolved map[string]struct{}) error {
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
		if selector.index != "" && address.ResourceSpec.Index.String() != selector.index {
			continue
		}
		resolved[resource.Address] = struct{}{}
	}
	return nil
}

func resolveModuleSelector(module *tfjson.StateModule, selector scriptOrderSelector, resolved map[string]struct{}) error {
	for _, child := range module.ChildModules {
		if child == nil {
			continue
		}

		modulePath, err := parseStateModuleAddress(child.Address)
		if err != nil {
			return err
		}
		if len(modulePath) == 0 || modulePath[len(modulePath)-1].Name != selector.name {
			continue
		}
		if err := collectModuleScripts(child, resolved); err != nil {
			return err
		}
	}
	return nil
}

func collectModuleScripts(module *tfjson.StateModule, resolved map[string]struct{}) error {
	for _, resource := range module.Resources {
		if resource == nil || resource.Mode != tfjson.ManagedResourceMode || resource.Type != "coder_script" {
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
		if err := collectModuleScripts(child, resolved); err != nil {
			return err
		}
	}
	return nil
}

func parseStateResourceAddress(module *tfjson.StateModule, resource *tfjson.StateResource) (*tfaddr.Address, error) {
	address, err := tfaddr.NewAddress(resource.Address)
	if err != nil {
		return nil, xerrors.Errorf("parse Terraform resource address %q: %w", resource.Address, err)
	}
	if address.ModulePath.String() != module.Address || address.ResourceSpec.Type != resource.Type || address.ResourceSpec.Name != resource.Name {
		return nil, xerrors.Errorf("Terraform resource address %q does not match its state fields", resource.Address)
	}
	return address, nil
}

func parseStateModuleAddress(address string) (tfaddr.ModulePath, error) {
	// go-terraform-address parses a module path only as part of a resource
	// address, so append a placeholder resource before parsing it.
	parsed, err := tfaddr.NewAddress(address + ".placeholder_resource.placeholder")
	if err != nil {
		return nil, xerrors.Errorf("parse module address %q: %w", address, err)
	}
	if len(parsed.ModulePath) == 0 {
		return nil, xerrors.Errorf("module address %q has no module path", address)
	}
	return parsed.ModulePath, nil
}

func forEachModule(module *tfjson.StateModule, visit func(*tfjson.StateModule) error) error {
	if module == nil {
		return nil
	}
	if err := visit(module); err != nil {
		return err
	}
	for _, child := range module.ChildModules {
		if err := forEachModule(child, visit); err != nil {
			return err
		}
	}
	return nil
}
