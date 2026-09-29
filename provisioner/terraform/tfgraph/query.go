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

const (
	maxReferenceLookups = 1_000_000
	maxTraversedEdges   = 1_000_000
)

// Query owns the mutable work budget for graph queries made by one request.
// It must not be reused concurrently. The underlying Index remains immutable
// and may be shared by multiple queries.
type Query struct {
	index  *Index
	limits queryLimits

	referenceLookups int
	traversedEdges   int
}

type queryLimits struct {
	referenceLookups int
	traversedEdges   int
}

// NewQuery creates a request-local bounded query for index.
func NewQuery(index *Index) (*Query, error) {
	return newQueryWithLimits(index, defaultQueryLimits())
}

func defaultQueryLimits() queryLimits {
	return queryLimits{
		referenceLookups: maxReferenceLookups,
		traversedEdges:   maxTraversedEdges,
	}
}

func newQueryWithLimits(index *Index, limits queryLimits) (*Query, error) {
	if index == nil {
		return nil, xerrors.New("Terraform graph index is required")
	}
	return &Query{index: index, limits: limits}, nil
}

// ConfigurationNodesForReferences resolves expression references relative to
// an evaluated module address. Each reference resolves to the most specific
// configuration address present in the index.
func (q *Query) ConfigurationNodesForReferences(
	ctx context.Context,
	moduleAddress string,
	references []string,
) ([]NodeID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	modulePath, err := tfaddr.ParseModulePath(moduleAddress)
	if err != nil {
		return nil, xerrors.Errorf(
			"parse declaring module address %q: %w", moduleAddress, err,
		)
	}

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
		for address := range reference.ConfigurationAddresses(
			modulePath.ConfigurationAddress(),
		) {
			if err := q.consumeReferenceLookups(1); err != nil {
				return nil, err
			}
			nodes := q.index.nodesByConfigurationAddress[address]
			if len(nodes) == 0 {
				continue
			}
			for _, id := range nodes {
				startNodes[id] = struct{}{}
			}
			break
		}
	}
	return sortedNodeIDs(maps.Keys(startNodes)), nil
}

// ReachableTerminalNodes traverses forward dependencies from startNodes. A
// terminal node is returned without traversing its dependencies.
func (q *Query) ReachableTerminalNodes(
	ctx context.Context,
	startNodes []NodeID,
	isTerminal func(Node) bool,
) ([]NodeID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if isTerminal == nil {
		return nil, xerrors.New("Terraform graph terminal predicate is required")
	}

	visited := map[NodeID]struct{}{}
	remaining := slices.Clone(startNodes)
	slices.SortFunc(remaining, compareNodeIDs)
	terminals := map[NodeID]struct{}{}
	for next := 0; next < len(remaining); next++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := remaining[next]
		position, ok := q.index.nodePosition(id)
		if !ok {
			return nil, xerrors.New(
				"Terraform graph query references a node outside its index",
			)
		}
		if _, ok := visited[id]; ok {
			continue
		}
		visited[id] = struct{}{}
		if isTerminal(q.index.nodes[position]) {
			terminals[id] = struct{}{}
			continue
		}

		dependencies := q.index.dependencies[position]
		if err := q.consumeTraversedEdges(len(dependencies)); err != nil {
			return nil, err
		}
		remaining = append(remaining, dependencies...)
	}
	return sortedNodeIDs(maps.Keys(terminals)), nil
}

func (q *Query) consumeReferenceLookups(count int) error {
	if exceedsLimit(
		q.referenceLookups, count, q.limits.referenceLookups,
	) {
		return xerrors.Errorf(
			"Terraform graph reference lookups exceed the limit of %d",
			q.limits.referenceLookups,
		)
	}
	q.referenceLookups += count
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
