// Package tfgraph provides reusable parsing, indexing, and query
// primitives for Terraform DOT graphs. Its API supports Coder's
// script-ordering pipeline but does not yet cover the broader graph
// processing in ConvertState.
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

// NodeID identifies a node within one Index. Its representation is opaque and
// remains stable for the lifetime of that index.
type NodeID struct {
	index *Index
	// position is one-based so the zero value is invalid.
	position int
}

// Node contains normalized read-only metadata for a Terraform graph node.
type Node struct {
	rawID     string
	label     string
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

// ConfigurationAddress returns the normalized address if the node represents
// a configuration expansion, or an empty string otherwise.
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

// Index is an immutable index of a Terraform graph's normalized nodes and
// forward dependency topology. A NodeID is valid only with the exact Index
// pointer that produced it, so Index values must not be copied.
type Index struct {
	nodes []Node
	// dependencies[source] contains the graph nodes that source depends on.
	dependencies                [][]NodeID
	nodesByConfigurationAddress map[string][]NodeID
	nodesByInstanceAddress      map[string][]NodeID
}

// Node returns one node from the index.
func (i *Index) Node(id NodeID) (Node, bool) {
	position, ok := i.nodePosition(id)
	if !ok {
		return Node{}, false
	}
	return i.nodes[position], true
}

// Nodes returns all nodes in deterministic node-ID order.
func (i *Index) Nodes() iter.Seq2[NodeID, Node] {
	return func(yield func(NodeID, Node) bool) {
		for position, node := range i.nodes {
			if !yield(nodeID(i, position), node) {
				return
			}
		}
	}
}

// NodesForConfigurationAddress returns the expansion nodes with address.
func (i *Index) NodesForConfigurationAddress(address string) []NodeID {
	return slices.Clone(i.nodesByConfigurationAddress[address])
}

// NodesForInstanceAddress returns the concrete instance nodes with address.
func (i *Index) NodesForInstanceAddress(address string) []NodeID {
	return slices.Clone(i.nodesByInstanceAddress[address])
}

// Parse parses a bounded Terraform DOT graph into an immutable index.
func Parse(ctx context.Context, rawGraph string) (*Index, error) {
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
) (*Index, error) {
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
	index := &Index{
		nodes: make([]Node, 0, len(rawNodeIDs)),
		dependencies: make(
			[][]NodeID, 0, len(rawNodeIDs),
		),
		nodesByConfigurationAddress: map[string][]NodeID{},
		nodesByInstanceAddress:      map[string][]NodeID{},
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
		node.label = graph.Nodes.Lookup[rawNodeID].Attrs["label"]
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
	return index, nil
}

// preflight bounds parser allocation before gographviz builds its syntax
// tree. Terraform Core's current DOT emitter writes one simple node or edge
// statement per line with quoted node IDs. Preflight recognizes that subset.
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

	remaining := rawGraph
	for lineIndex := 0; ; lineIndex++ {
		if lineIndex%256 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		line, rest, hasMore := strings.Cut(remaining, "\n")
		remaining = rest
		statement := strings.TrimSpace(line)

		rawNodeID, remainder, ok := consumeQuotedNodeID(statement)
		if !ok {
			// Reject edge operators on unrecognized statements because preflight
			// cannot safely count the graph structure they represent.
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

func nodeID(index *Index, position int) NodeID {
	return NodeID{index: index, position: position + 1}
}

func (i *Index) nodePosition(id NodeID) (int, bool) {
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
