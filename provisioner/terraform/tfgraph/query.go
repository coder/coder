package tfgraph

import (
	"cmp"
	"context"
	"iter"
	"maps"
	"slices"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

// TODO(PLAT-554): Measure query performance with representative large
// Terraform graphs, then tune these limits.
const (
	maxReferenceLookups = 1_000_000
	maxNodeVisits       = 1_000_000
	maxTraversedEdges   = 1_000_000
)

// ReferenceLookup resolves Terraform value references relative to their
// declaring module. It is bound to the active traversal context and Query, so
// lookups share cancellation and the cumulative work budget. A resolver must
// not call it concurrently or retain it after returning.
type ReferenceLookup func(
	moduleAddress string,
	references []string,
) ([]NodeID, error)

// DependencyResolver returns the effective dependencies for a visited
// non-boundary source. graphDependencies is an independent copy of the
// source's indexed dependencies after module close completion edges have been
// removed. The slice may be modified without changing the source graph.
// The resolver may omit graph dependencies or return other nodes from the same
// parsed graph. It may call lookup to translate plan references; that work
// shares the query's cumulative budget. Work performed outside lookup is not
// included in that budget; callers must ensure it is bounded separately. The
// resolver must return promptly when ctx is canceled.
//
// This lets callers use information outside the graph, such as Terraform plan
// configuration, when graph metadata alone is insufficient. Terraform may
// omit direct value edges during transitive reduction when another path reaches
// the same node. Resolvers must restore those value dependencies for every
// visited source where they can be omitted, while removing ordering-only
// dependencies. This includes dependencies into a module expansion node and
// dependencies from the expansion node. For example, expansion-node edges to
// for_each and depends_on references have the same graph shape, although only
// the for_each collection can contribute to an input expressed as each.value.
type DependencyResolver func(
	ctx context.Context,
	lookup ReferenceLookup,
	source Node,
	graphDependencies []NodeID,
) ([]NodeID, error)

// WithResolvedDependencies returns a lazy graph view that uses resolver to
// filter indexed dependencies and restore dependencies omitted from Terraform's
// graph. It shares immutable nodes and NodeIDs with g and does not modify g.
// The returned Graph may be shared by concurrent queries only when resolver
// is safe for concurrent use.
func (g *Graph) WithResolvedDependencies(
	resolver DependencyResolver,
) (*Graph, error) {
	if g == nil || g.index == nil {
		return nil, xerrors.New("Terraform graph is required")
	}
	if resolver == nil {
		return nil, xerrors.New("Terraform graph dependency resolver is required")
	}
	return &Graph{
		index:               g.index,
		resolveDependencies: resolver,
	}, nil
}

// Query tracks a cumulative work budget shared across its graph-query calls. It
// must not be used concurrently. Query calls do not modify the graph's indexed
// data, so it may be shared by multiple queries.
type Query struct {
	index               *graphIndex
	resolveDependencies DependencyResolver
	limits              queryLimits

	referenceLookups int
	nodeVisits       int
	traversedEdges   int
}

type queryLimits struct {
	referenceLookups int
	nodeVisits       int
	traversedEdges   int
}

// NewQuery creates a query over graph with fresh limits on reference lookups,
// node visits, and edge traversals. Reuse it for graph-query calls that should
// share those cumulative limits.
func NewQuery(graph *Graph) (*Query, error) {
	return newQueryWithLimits(graph, defaultQueryLimits())
}

func defaultQueryLimits() queryLimits {
	return queryLimits{
		referenceLookups: maxReferenceLookups,
		nodeVisits:       maxNodeVisits,
		traversedEdges:   maxTraversedEdges,
	}
}

func newQueryWithLimits(
	graph *Graph,
	limits queryLimits,
) (*Query, error) {
	if graph == nil || graph.index == nil {
		return nil, xerrors.New("Terraform graph is required")
	}
	return &Query{
		index:               graph.index,
		resolveDependencies: graph.resolveDependencies,
		limits:              limits,
	}, nil
}

// ConfigurationNodesForReferences resolves Terraform value references relative
// to their declaring module. An empty declaringModuleAddress identifies the
// root module. Each reference resolves to the most specific configuration
// address present in the graph. A whole-module reference resolves to all of the
// module's output nodes.
func (q *Query) ConfigurationNodesForReferences(
	ctx context.Context,
	moduleAddress string,
	references []string,
) ([]NodeID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Configuration graph nodes use key-free module addresses. Parse the
	// declaring module address to validate it and remove any instance keys.
	modulePath, err := tfaddr.ParseModulePath(moduleAddress)
	if err != nil {
		return nil, xerrors.Errorf(
			"parse declaring module address %q: %w",
			moduleAddress, err,
		)
	}
	moduleConfAddress := modulePath.ConfigurationAddress()

	startNodes := map[NodeID]struct{}{}
	seenReferences := map[string]struct{}{}
	for _, rawReference := range references {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, duplicate := seenReferences[rawReference]; duplicate {
			continue
		}
		seenReferences[rawReference] = struct{}{}

		reference, err := tfaddr.ParseConfigurationReference(rawReference)
		if err != nil {
			return nil, xerrors.Errorf(
				"resolve Terraform reference %q: %w", rawReference, err,
			)
		}
		if moduleAddress, ok := wholeModuleReferenceAddress(
			reference, moduleConfAddress,
		); ok {
			if err := q.consumeReferenceLookups(1); err != nil {
				return nil, err
			}
			nodes := q.index.outputNodesByModuleAddress[moduleAddress]
			if err := q.consumeNodeVisits(len(nodes)); err != nil {
				return nil, err
			}
			for _, id := range nodes {
				startNodes[id] = struct{}{}
			}
			continue
		}
		for address := range reference.ConfigurationAddresses(moduleConfAddress) {
			if err := q.consumeReferenceLookups(1); err != nil {
				return nil, err
			}
			nodes := q.index.nodesByConfigurationAddress[address]
			if len(nodes) == 0 {
				continue
			}
			if err := q.consumeNodeVisits(len(nodes)); err != nil {
				return nil, err
			}
			for _, id := range nodes {
				startNodes[id] = struct{}{}
			}
			break
		}
	}
	return sortedNodeIDs(maps.Keys(startNodes)), nil
}

// wholeModuleReferenceAddress returns the qualified configuration
// address when reference selects an entire direct child module.
func wholeModuleReferenceAddress(
	reference tfaddr.ConfigurationReference,
	declaringModuleAddress string,
) (string, bool) {
	path, err := tfaddr.ParseModulePath(reference.ConfigurationAddress())
	if err != nil || len(path.Steps()) != 1 {
		return "", false
	}
	address := path.ConfigurationAddress()
	if declaringModuleAddress != "" {
		address = declaringModuleAddress + "." + address
	}
	return address, true
}

// ReachableBoundaryNodes returns nodes matching isBoundary that are reachable
// by following forward dependencies from startNodes. A matching node ends its
// dependency path, so matches beyond it are excluded.
//
// When the graph has resolved dependencies, its DependencyResolver is called
// for each visited non-boundary node, including non-boundary start nodes.
// Returning an error stops traversal.
//
// Module close nodes are followed only through their direct output nodes
// because their other dependencies only ensure they finish before the module.
//
// Each graph node is traversed at most once, so duplicate start nodes and
// converging paths do not duplicate results. Indexed and restored dependencies
// count toward the cumulative edge budget. Results are returned in
// deterministic node-ID order.
func (q *Query) ReachableBoundaryNodes(
	ctx context.Context,
	startNodes []NodeID,
	isBoundary func(Node) bool,
) ([]NodeID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if isBoundary == nil {
		return nil, xerrors.New("Terraform graph boundary predicate is required")
	}
	seen := make(map[NodeID]struct{}, len(startNodes))
	for _, id := range startNodes {
		seen[id] = struct{}{}
	}
	// Restore deterministic traversal order after deduplicating through the map.
	remaining := sortedNodeIDs(maps.Keys(seen))
	boundaries := map[NodeID]struct{}{}
	var lookup ReferenceLookup
	if q.resolveDependencies != nil {
		lookup = func(
			moduleAddress string,
			references []string,
		) ([]NodeID, error) {
			return q.ConfigurationNodesForReferences(
				ctx, moduleAddress, references,
			)
		}
	}
	for i := 0; i < len(remaining); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := q.consumeNodeVisits(1); err != nil {
			return nil, err
		}
		id := remaining[i]
		position, ok := q.index.nodePosition(id)
		if !ok {
			return nil, xerrors.New(
				"Terraform graph query references a node outside its graph",
			)
		}
		node := q.index.nodes[position]
		if isBoundary(node) {
			boundaries[id] = struct{}{}
			continue
		}

		dependencies, err := q.effectiveDependenciesForNode(
			ctx, lookup, node, position,
		)
		if err != nil {
			return nil, err
		}
		for _, dependency := range dependencies {
			if _, ok := seen[dependency]; ok {
				continue
			}
			seen[dependency] = struct{}{}
			remaining = append(remaining, dependency)
		}
	}
	return sortedNodeIDs(maps.Keys(boundaries)), nil
}

// effectiveDependenciesForNode applies the optional resolver without allowing
// it to bypass module-close traversal rules or the cumulative edge budget.
func (q *Query) effectiveDependenciesForNode(
	ctx context.Context,
	lookup ReferenceLookup,
	node Node,
	position int,
) ([]NodeID, error) {
	moduleCloseAddress := moduleCloseConfigurationAddress(node)
	graphDependencies, err := q.graphDependenciesForNode(
		position, moduleCloseAddress,
	)
	if err != nil {
		return nil, err
	}

	if q.resolveDependencies == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return graphDependencies, nil
	}

	resolvedDependencies, err := q.resolveDependencies(
		ctx, lookup, node, slices.Clone(graphDependencies),
	)
	if err != nil {
		return nil, xerrors.Errorf(
			"resolve dependencies for Terraform graph node %q: %w",
			truncateDiagnosticValue(node.RawID()), err,
		)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Reuse the validated graph dependencies when the resolver made no change,
	// avoiding normalization allocations on the common path.
	if slices.Equal(resolvedDependencies, graphDependencies) {
		return graphDependencies, nil
	}
	return q.normalizeResolvedDependencies(
		resolvedDependencies,
		graphDependencies,
		moduleCloseAddress,
	)
}

func (q *Query) graphDependenciesForNode(
	position int,
	moduleCloseAddress string,
) ([]NodeID, error) {
	indexedDependencies := q.index.dependencies[position]
	if err := q.consumeTraversedEdges(len(indexedDependencies)); err != nil {
		return nil, err
	}
	graphDependencies := indexedDependencies
	if moduleCloseAddress != "" {
		graphDependencies = make([]NodeID, 0, len(indexedDependencies))
	}
	for _, dependency := range indexedDependencies {
		position, ok := q.index.nodePosition(dependency)
		if !ok {
			return nil, xerrors.New(
				"Terraform graph query references a node outside its graph",
			)
		}
		if moduleCloseAddress == "" {
			continue
		}
		if isModuleCloseCompletionOnlyDependency(
			moduleCloseAddress,
			q.index.nodes[position],
		) {
			continue
		}
		graphDependencies = append(graphDependencies, dependency)
	}
	return graphDependencies, nil
}

// normalizeResolvedDependencies validates resolver output, enforces edge
// budgeting, deduplicates, and prevents module-close completion edges
// from being reintroduced.
func (q *Query) normalizeResolvedDependencies(
	resolvedDependencies []NodeID,
	graphDependencies []NodeID,
	moduleCloseAddress string,
) ([]NodeID, error) {
	// Each graph dependency was charged before resolution, so its
	// first match in the resolver is exempt from another charge.
	// Additions and duplicates are charged.
	dependencyCredits := make(
		map[NodeID]struct{},
		min(len(graphDependencies), len(q.index.nodes)),
	)
	for _, dependency := range graphDependencies {
		dependencyCredits[dependency] = struct{}{}
	}
	seenDependencies := make(
		map[NodeID]struct{},
		min(len(resolvedDependencies), len(q.index.nodes)),
	)
	normalizedDependencies := make(
		[]NodeID, 0,
		min(len(resolvedDependencies), len(q.index.nodes)),
	)
	for _, dependency := range resolvedDependencies {
		position, ok := q.index.nodePosition(dependency)
		if !ok {
			return nil, xerrors.New(
				"Terraform graph dependency resolver returned a node outside its graph",
			)
		}
		if _, credited := dependencyCredits[dependency]; credited {
			delete(dependencyCredits, dependency)
		} else if err := q.consumeTraversedEdges(1); err != nil {
			return nil, err
		}
		if moduleCloseAddress != "" && isModuleCloseCompletionOnlyDependency(
			moduleCloseAddress,
			q.index.nodes[position],
		) {
			continue
		}
		if _, duplicate := seenDependencies[dependency]; duplicate {
			continue
		}
		seenDependencies[dependency] = struct{}{}
		normalizedDependencies = append(normalizedDependencies, dependency)
	}
	return normalizedDependencies, nil
}

func moduleCloseConfigurationAddress(node Node) string {
	if node.Operation() != "close" {
		return ""
	}
	modulePath, err := tfaddr.ParseModulePath(node.Address())
	if err != nil || len(modulePath.Steps()) == 0 {
		return ""
	}
	return modulePath.ConfigurationAddress()
}

func isModuleCloseCompletionOnlyDependency(
	moduleAddress string,
	dependency Node,
) bool {
	dependencyModuleAddress, ok := moduleAddressForOutput(
		dependency.ConfigurationAddress(),
	)
	// Terraform currently emits output edges only for the closing
	// module. But keep checking ownership so nested-module outputs
	// remain completion-only if the graph shape changes.
	return !ok || dependencyModuleAddress != moduleAddress
}

func (q *Query) consumeReferenceLookups(count int) error {
	if exceedsLimit(q.referenceLookups, count, q.limits.referenceLookups) {
		return xerrors.Errorf(
			"Terraform graph reference lookups exceed the limit of %d",
			q.limits.referenceLookups,
		)
	}
	q.referenceLookups += count
	return nil
}

func (q *Query) consumeNodeVisits(count int) error {
	if exceedsLimit(q.nodeVisits, count, q.limits.nodeVisits) {
		return xerrors.Errorf(
			"Terraform graph node visits exceed the limit of %d",
			q.limits.nodeVisits,
		)
	}
	q.nodeVisits += count
	return nil
}

func (q *Query) consumeTraversedEdges(count int) error {
	if exceedsLimit(q.traversedEdges, count, q.limits.traversedEdges) {
		return xerrors.Errorf(
			"Terraform graph traversed edges exceed the limit of %d",
			q.limits.traversedEdges,
		)
	}
	q.traversedEdges += count
	return nil
}

func sortedNodeIDs(ids iter.Seq[NodeID]) []NodeID {
	return slices.SortedFunc(ids, compareNodeIDs)
}

func compareNodeIDs(left, right NodeID) int {
	return cmp.Compare(left.position, right.position)
}
