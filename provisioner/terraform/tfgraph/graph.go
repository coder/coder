// Package tfgraph provides reusable parsing, indexing, and query
// primitives for operation graphs emitted by Terraform Core for Coder
// templates. It is not a general-purpose DOT parser. Parsing limits
// rely on Terraform Core's emitted line structure.
//
// Its API supports Coder's script-ordering pipeline but does not yet
// cover the broader graph processing in ConvertState.
//
// Query the parsed graph directly to use Terraform's dependency edges:
//
//	graph, err := Parse(ctx, rawGraph)
//	query, err := NewQuery(graph)
//
// Create a resolved graph view when configuration data must filter or restore
// dependencies:
//
//	view, err := graph.WithResolvedDependencies(resolver)
//	query, err = NewQuery(view)
package tfgraph

import (
	"context"
	"iter"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/awalterschulze/gographviz"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

// TODO(PLAT-554): Measure parser time and memory use with
// representative large Terraform graphs, then tune these limits.
const (
	maxInputBytes           = 64 << 20 // 64 MiB
	maxNodes                = 100_000
	maxEdges                = 1_000_000
	maxStatements           = maxNodes + maxEdges
	maxNodeIDBytes          = 16 << 20 // 16 MiB
	maxRetainedAddressBytes = 16 << 20 // 16 MiB
	maxDiagnosticValueBytes = 256
)

// NodeID identifies a node in a graph returned by Parse. It can also be used
// with graphs derived from that graph, but not with an unrelated graph. Its
// zero value is invalid.
type NodeID struct {
	index *graphIndex
	// position is one-based within graphIndex.nodes so the zero value is invalid.
	position int
}

// Node contains normalized read-only metadata for a Terraform graph node.
type Node struct {
	rawID     string
	address   string
	operation string
}

// RawID returns the node identifier as it appeared in the DOT graph.
func (n Node) RawID() string {
	return n.rawID
}

// Address returns the normalized Terraform address, without the graph node's
// root prefix or operation suffix.
func (n Node) Address() string {
	return n.address
}

// Operation returns the graph operation suffix, such as expand or destroy.
func (n Node) Operation() string {
	return n.operation
}

// ConfigurationAddress returns an expansion node's normalized configuration
// address, or an empty string for non-expansion nodes.
func (n Node) ConfigurationAddress() string {
	if n.operation != "expand" {
		return ""
	}
	return n.address
}

// InstanceAddress returns the normalized address if the node represents a
// concrete instance, or an empty string otherwise.
func (n Node) InstanceAddress() string {
	if n.operation != "" {
		return ""
	}
	return n.address
}

// Graph is a queryable representation of a Terraform graph's normalized nodes
// and forward dependency topology. A Graph returned by WithResolvedDependencies
// shares the immutable indexed nodes and edges of its source graph.
type Graph struct {
	index               *graphIndex
	resolveDependencies DependencyResolver
}

type graphIndex struct {
	nodes []Node
	// dependencies[source] contains the graph nodes that source depends on.
	dependencies                [][]NodeID
	nodesByConfigurationAddress map[string][]NodeID
	nodesByInstanceAddress      map[string][]NodeID
	outputNodesByModuleAddress  map[string][]NodeID
}

// Node returns one node from the graph.
func (g *Graph) Node(id NodeID) (Node, bool) {
	if g == nil || g.index == nil {
		return Node{}, false
	}
	position, ok := g.index.nodePosition(id)
	if !ok {
		return Node{}, false
	}
	return g.index.nodes[position], true
}

// Nodes returns all nodes in deterministic node-ID order.
func (g *Graph) Nodes() iter.Seq2[NodeID, Node] {
	return func(yield func(NodeID, Node) bool) {
		if g == nil || g.index == nil {
			return
		}
		for position, node := range g.index.nodes {
			if !yield(nodeID(g.index, position), node) {
				return
			}
		}
	}
}

// NodesForConfigurationAddress returns expansion nodes for the given
// configuration address. Modifying the returned slice does not affect
// the graph.
func (g *Graph) NodesForConfigurationAddress(address string) []NodeID {
	if g == nil || g.index == nil {
		return nil
	}
	return slices.Clone(g.index.nodesByConfigurationAddress[address])
}

// NodesForInstanceAddress returns concrete instance nodes for the given
// instance address. Modifying the returned slice does not affect the graph.
func (g *Graph) NodesForInstanceAddress(address string) []NodeID {
	if g == nil || g.index == nil {
		return nil
	}
	return slices.Clone(g.index.nodesByInstanceAddress[address])
}

// Parse parses successful DOT output from Terraform Core's
// operation-graph emitter for a Coder template into an immutable Graph.
// Callers must not pass arbitrary DOT.
func Parse(ctx context.Context, rawGraph string) (*Graph, error) {
	return parseWithLimits(ctx, rawGraph, defaultIndexLimits())
}

type indexLimits struct {
	inputBytes           int
	statements           int
	nodes                int
	edges                int
	nodeIDBytes          int
	retainedAddressBytes int
}

func defaultIndexLimits() indexLimits {
	return indexLimits{
		inputBytes:           maxInputBytes,
		statements:           maxStatements,
		nodes:                maxNodes,
		edges:                maxEdges,
		nodeIDBytes:          maxNodeIDBytes,
		retainedAddressBytes: maxRetainedAddressBytes,
	}
}

func parseWithLimits(
	ctx context.Context,
	rawGraph string,
	limits indexLimits,
) (*Graph, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(rawGraph) > limits.inputBytes {
		return nil, xerrors.Errorf(
			"Terraform graph DOT input bytes exceed the limit of %d",
			limits.inputBytes,
		)
	}
	if err := preflight(ctx, rawGraph, limits); err != nil {
		return nil, err
	}

	// gographviz has no context-aware API, so check cancellation around its
	// synchronous parsing phases.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parsed, err := gographviz.ParseString(rawGraph)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, xerrors.Errorf("parse Terraform graph: %w", err)
	}
	graph, err := gographviz.NewAnalysedGraph(parsed)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, xerrors.Errorf("analyze Terraform graph: %w", err)
	}
	if len(graph.Nodes.Lookup) > limits.nodes {
		return nil, xerrors.Errorf(
			"Terraform graph nodes exceed the limit of %d", limits.nodes,
		)
	}

	rawNodeIDs := slices.Sorted(maps.Keys(graph.Nodes.Lookup))
	index := &graphIndex{
		nodes: make([]Node, 0, len(rawNodeIDs)),
		dependencies: make(
			[][]NodeID, 0, len(rawNodeIDs),
		),
		nodesByConfigurationAddress: map[string][]NodeID{},
		nodesByInstanceAddress:      map[string][]NodeID{},
		outputNodesByModuleAddress:  map[string][]NodeID{},
	}
	nodeIDByRawID := make(map[string]NodeID, len(rawNodeIDs))
	var nodeIDBytes, retainedAddressBytes int
	for position, rawNodeID := range rawNodeIDs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if exceedsLimit(nodeIDBytes, len(rawNodeID), limits.nodeIDBytes) {
			return nil, xerrors.Errorf(
				"Terraform graph node ID bytes exceed the limit of %d",
				limits.nodeIDBytes,
			)
		}
		nodeIDBytes += len(rawNodeID)

		node := parseNode(rawNodeID)
		if exceedsLimit(
			retainedAddressBytes, len(node.address), limits.retainedAddressBytes,
		) {
			return nil, xerrors.Errorf(
				"Terraform graph address bytes exceed the limit of %d",
				limits.retainedAddressBytes,
			)
		}
		retainedAddressBytes += len(node.address)

		id := nodeID(index, position)
		nodeIDByRawID[rawNodeID] = id
		index.nodes = append(index.nodes, node)
		index.dependencies = append(index.dependencies, nil)
		if address := node.ConfigurationAddress(); address != "" {
			index.nodesByConfigurationAddress[address] = append(
				index.nodesByConfigurationAddress[address], id,
			)
			if moduleAddress, ok := moduleAddressForOutput(address); ok {
				index.outputNodesByModuleAddress[moduleAddress] = append(
					index.outputNodesByModuleAddress[moduleAddress], id,
				)
			}
		}
		if address := node.InstanceAddress(); address != "" {
			index.nodesByInstanceAddress[address] = append(
				index.nodesByInstanceAddress[address], id,
			)
		}
	}

	edgeCount := 0
	for _, rawSourceID := range slices.Sorted(maps.Keys(graph.Edges.SrcToDsts)) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sourceID, ok := nodeIDByRawID[rawSourceID]
		if !ok {
			return nil, xerrors.Errorf(
				"Terraform graph edge references unknown source node %q",
				truncateDiagnosticValue(rawSourceID),
			)
		}
		rawDestinations := slices.Sorted(
			maps.Keys(graph.Edges.SrcToDsts[rawSourceID]),
		)
		if exceedsLimit(edgeCount, len(rawDestinations), limits.edges) {
			return nil, xerrors.Errorf(
				"Terraform graph edges exceed the limit of %d", limits.edges,
			)
		}
		edgeCount += len(rawDestinations)
		for _, rawDestinationID := range rawDestinations {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			destinationID, ok := nodeIDByRawID[rawDestinationID]
			if !ok {
				return nil, xerrors.Errorf(
					"Terraform graph edge references unknown node %q",
					truncateDiagnosticValue(rawDestinationID),
				)
			}
			position, ok := index.nodePosition(sourceID)
			if !ok {
				return nil, xerrors.New("Terraform graph contains an invalid source node")
			}
			index.dependencies[position] = append(
				index.dependencies[position], destinationID,
			)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Graph{index: index}, nil
}

// preflight bounds parser allocation for operation graphs emitted by
// Terraform Core before gographviz builds its syntax tree. It assumes
// rawGraph satisfies Parse's contract and does not validate arbitrary
// DOT. Terraform Core writes each node or edge statement on its own
// line with quoted node IDs.
func preflight(
	ctx context.Context,
	rawGraph string,
	limits indexLimits,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	nodes := map[string]struct{}{}
	statementCount := 0
	nodeDeclarations := 0
	nodeIDBytes := 0
	edges := 0
	addStatement := func() error {
		statementCount++
		if statementCount > limits.statements {
			return xerrors.Errorf(
				"Terraform graph DOT statements exceed the limit of %d",
				limits.statements,
			)
		}
		return nil
	}
	addNode := func(rawNodeID string) error {
		if _, exists := nodes[rawNodeID]; exists {
			return nil
		}
		if exceedsLimit(nodeIDBytes, len(rawNodeID), limits.nodeIDBytes) {
			return xerrors.Errorf(
				"Terraform graph node ID bytes exceed the limit of %d",
				limits.nodeIDBytes,
			)
		}
		nodeIDBytes += len(rawNodeID)
		nodes[rawNodeID] = struct{}{}
		if len(nodes) > limits.nodes {
			return xerrors.Errorf(
				"Terraform graph nodes exceed the limit of %d", limits.nodes,
			)
		}
		return nil
	}

	const contextCheckInterval = 256

	remaining := rawGraph
	for lineIndex := 0; ; lineIndex++ {
		if lineIndex%contextCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		line, rest, hasMore := strings.Cut(remaining, "\n")
		remaining = rest
		statement := strings.TrimSpace(line)

		rawNodeID, remainder, ok := consumeQuotedNodeID(statement)
		if !ok {
			// Terraform's structural lines do not contain edge operators. Reject
			// one here rather than pass uncounted work to gographviz.
			if hasEdgeOperatorOutsideQuotes(statement) {
				return xerrors.New("Terraform graph contains unsupported edge syntax")
			}
		} else {
			if err := addStatement(); err != nil {
				return err
			}
			edgeStatement := false
			for {
				remainder = strings.TrimSpace(remainder)
				if !strings.HasPrefix(remainder, "->") &&
					!strings.HasPrefix(remainder, "--") {
					break
				}
				edgeStatement = true
				edges++
				if edges > limits.edges {
					return xerrors.Errorf(
						"Terraform graph edges exceed the limit of %d",
						limits.edges,
					)
				}
				if err := addNode(rawNodeID); err != nil {
					return err
				}

				destination := strings.TrimSpace(remainder[2:])
				rawNodeID, remainder, ok = consumeQuotedNodeID(destination)
				if !ok {
					return xerrors.New(
						"Terraform graph contains unsupported edge syntax",
					)
				}
				if err := addNode(rawNodeID); err != nil {
					return err
				}
			}
			if hasEdgeOperatorOutsideQuotes(remainder) {
				return xerrors.New("Terraform graph contains unsupported edge syntax")
			}
			if !edgeStatement {
				nodeDeclarations++
				if nodeDeclarations > limits.nodes {
					return xerrors.Errorf(
						"Terraform graph nodes exceed the limit of %d", limits.nodes,
					)
				}
				if err := addNode(rawNodeID); err != nil {
					return err
				}
			}
		}

		if !hasMore {
			break
		}
	}
	return nil
}

func hasEdgeOperatorOutsideQuotes(input string) bool {
	remaining := input
	for {
		quoteStart := strings.IndexByte(remaining, '"')
		unquoted := remaining
		if quoteStart >= 0 {
			unquoted = remaining[:quoteStart]
		}
		if strings.Contains(unquoted, "->") ||
			strings.Contains(unquoted, "--") {
			return true
		}
		if quoteStart < 0 {
			return false
		}
		_, afterQuote, ok := consumeQuotedNodeID(remaining[quoteStart:])
		if !ok {
			return false
		}
		remaining = afterQuote
	}
}

func consumeQuotedNodeID(input string) (nodeID string, remainder string, ok bool) {
	if len(input) == 0 || input[0] != '"' {
		return "", input, false
	}
	for position := 1; position < len(input); position++ {
		switch input[position] {
		case '\\':
			position++
		case '"':
			return input[:position+1], input[position+1:], true
		}
	}
	return "", input, false
}

func parseNode(rawNodeID string) Node {
	nodeID := rawNodeID
	if unquoted, err := strconv.Unquote(rawNodeID); err == nil {
		nodeID = unquoted
	}
	_, rawAddress, ok := strings.Cut(nodeID, "] ")
	if !ok {
		return Node{rawID: rawNodeID}
	}
	address, operation := addressOperation(rawAddress)
	return Node{
		rawID:     rawNodeID,
		address:   address,
		operation: operation,
	}
}

func moduleAddressForOutput(address string) (string, bool) {
	separator := strings.LastIndex(address, ".output.")
	if separator < 0 {
		return "", false
	}
	moduleAddress := address[:separator]
	if _, err := tfaddr.ParseModulePath(moduleAddress); err != nil {
		return "", false
	}
	return moduleAddress, true
}

func addressOperation(raw string) (address string, operation string) {
	if !strings.HasSuffix(raw, ")") {
		return raw, ""
	}
	operationStart := strings.LastIndex(raw, " (")
	if operationStart < 0 {
		return raw, ""
	}
	return raw[:operationStart], raw[operationStart+2 : len(raw)-1]
}

func nodeID(index *graphIndex, position int) NodeID {
	return NodeID{index: index, position: position + 1}
}

func (i *graphIndex) nodePosition(id NodeID) (int, bool) {
	position := id.position - 1
	return position,
		id.index == i && position >= 0 && position < len(i.nodes)
}

func exceedsLimit(used, additional, limit int) bool {
	return used > limit || additional > limit-used
}

// Template-controlled graph values can be large, so bound them to
// keep provisioner diagnostics well below the dRPC message limit.
func truncateDiagnosticValue(value string) string {
	if len(value) <= maxDiagnosticValueBytes {
		return value
	}
	end := maxDiagnosticValueBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end] + "..."
}
