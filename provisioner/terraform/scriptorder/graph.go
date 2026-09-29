package scriptorder

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	stringutil "github.com/coder/coder/v2/coderd/util/strings"
)

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
