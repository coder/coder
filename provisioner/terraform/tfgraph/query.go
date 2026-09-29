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

// Query owns a mutable work budget shared across its graph-query calls. It
// must not be used concurrently. The underlying Index remains immutable and
// may be shared by multiple queries.
type Query struct {
	index  *Index
	limits queryLimits

	referenceLookups int
	nodeVisits       int
	traversedEdges   int
}

type queryLimits struct {
	referenceLookups int
	nodeVisits       int
	traversedEdges   int
}

// NewQuery creates a query over index with fresh limits on reference lookups,
// node visits, and edge traversals. Reuse it for graph-query calls that should
// share those cumulative limits.
func NewQuery(index *Index) (*Query, error) {
	return newQueryWithLimits(index, defaultQueryLimits())
}

func defaultQueryLimits() queryLimits {
	return queryLimits{
		referenceLookups: maxReferenceLookups,
		nodeVisits:       maxNodeVisits,
		traversedEdges:   maxTraversedEdges,
	}
}

func newQueryWithLimits(index *Index, limits queryLimits) (*Query, error) {
	if index == nil {
		return nil, xerrors.New("Terraform graph index is required")
	}
	return &Query{index: index, limits: limits}, nil
}

// ConfigurationNodesForReferences resolves Terraform value references relative
// to their declaring module. An empty declaringModuleAddress identifies the
// root module. Each reference resolves to the most specific configuration
// address present in the index.
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

// ReachableBoundaryNodes returns nodes matching isBoundary that are reachable
// by following forward dependencies from startNodes. A matching node ends its
// dependency path, so matches beyond it are excluded.
//
// Each graph node is traversed at most once, so duplicate start nodes and
// converging paths do not duplicate results. Results are returned in
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

	if err := q.consumeNodeVisits(len(startNodes)); err != nil {
		return nil, err
	}
	queued := make(map[NodeID]struct{}, len(startNodes))
	for _, id := range startNodes {
		queued[id] = struct{}{}
	}
	// Restore deterministic traversal order after deduplicating through the map.
	remaining := sortedNodeIDs(maps.Keys(queued))
	boundaries := map[NodeID]struct{}{}
	for i := 0; i < len(remaining); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := remaining[i]
		position, ok := q.index.nodePosition(id)
		if !ok {
			return nil, xerrors.New(
				"Terraform graph query references a node outside its index",
			)
		}
		if isBoundary(q.index.nodes[position]) {
			boundaries[id] = struct{}{}
			continue
		}

		dependencies := q.index.dependencies[position]
		if err := q.consumeTraversedEdges(len(dependencies)); err != nil {
			return nil, err
		}
		for _, dependency := range dependencies {
			if _, ok := queued[dependency]; ok {
				continue
			}
			queued[dependency] = struct{}{}
			remaining = append(remaining, dependency)
		}
	}
	return sortedNodeIDs(maps.Keys(boundaries)), nil
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
