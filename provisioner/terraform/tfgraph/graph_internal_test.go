package tfgraph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type cancelOnErrCheckContext struct {
	context.Context
	cancel          context.CancelFunc
	remainingChecks int
}

func (c *cancelOnErrCheckContext) Err() error {
	if err := c.Context.Err(); err != nil {
		return err
	}
	c.remainingChecks--
	if c.remainingChecks == 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestIndexRejectsExcessiveTerraformOutput(t *testing.T) {
	t.Parallel()

	validGraph := `digraph {
		"[root] local.bridge (expand)"
		"[root] coder_agent.main (expand)"
		"[root] local.bridge (expand)" -> "[root] coder_agent.main (expand)"
	}`
	for _, test := range []struct {
		name          string
		configure     func(*indexLimits)
		errorContains string
	}{
		{
			name: "InputBytes",
			configure: func(limits *indexLimits) {
				limits.inputBytes = len(validGraph) - 1
			},
			errorContains: "DOT input bytes",
		},
		{
			name: "Statements",
			configure: func(limits *indexLimits) {
				limits.statements = 1
			},
			errorContains: "DOT statements",
		},
		{
			name: "Nodes",
			configure: func(limits *indexLimits) {
				limits.nodes = 1
			},
			errorContains: "Terraform graph nodes",
		},
		{
			name: "Edges",
			configure: func(limits *indexLimits) {
				limits.edges = 0
			},
			errorContains: "Terraform graph edges",
		},
		{
			name: "NodeIDBytes",
			configure: func(limits *indexLimits) {
				limits.nodeIDBytes = 1
			},
			errorContains: "Terraform graph node ID bytes",
		},
		{
			name: "RetainedAddressBytes",
			configure: func(limits *indexLimits) {
				limits.retainedAddressBytes = 1
			},
			errorContains: "Terraform graph address bytes",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			limits := defaultIndexLimits()
			test.configure(&limits)
			_, err := parseWithLimits(t.Context(), validGraph, limits)
			require.ErrorContains(t, err, test.errorContains)
		})
	}
}

func TestPreflightBoundsTerraformGraphOutput(t *testing.T) {
	t.Parallel()

	rawGraph := `digraph {
		compound = "true"
		subgraph "root" {
			"[root] local.bridge (expand)" [label = "not -> an edge"]
			"[root] coder_agent.main (expand)" [label = "coder_agent.main"]
			"[root] coder_agent.main[\"} ->\"]" [label = "quoted edge syntax"]
			"[root] local.bridge (expand)" -> "[root] coder_agent.main (expand)"
		}
	}`
	limits := defaultIndexLimits()
	limits.statements = 4
	limits.nodes = 3
	limits.edges = 1
	require.NoError(t, preflight(t.Context(), rawGraph, limits))

	limits.statements = 3
	require.ErrorContains(
		t, preflight(t.Context(), rawGraph, limits), "DOT statements",
	)
	limits.statements = 4
	limits.nodes = 2
	require.ErrorContains(
		t, preflight(t.Context(), rawGraph, limits), "Terraform graph nodes",
	)
	limits.nodes = 3
	limits.edges = 0
	require.ErrorContains(
		t, preflight(t.Context(), rawGraph, limits), "Terraform graph edges",
	)
}

func TestPreflightCountsEdgeEndpointNodes(t *testing.T) {
	t.Parallel()

	limits := defaultIndexLimits()
	limits.nodes = 1
	err := preflight(
		t.Context(),
		"digraph {\n\t\"[root] a\" -> \"[root] b\"\n}",
		limits,
	)
	require.ErrorContains(t, err, "Terraform graph nodes")
}

func TestParseHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Parse(ctx, "digraph {}")
	require.ErrorIs(t, err, context.Canceled)
}

func TestParseHonorsCancellationDuringIndexing(t *testing.T) {
	t.Parallel()

	const rawGraph = "digraph {\n\t\"[root] a\" -> \"[root] b\"\n}"
	// Parse checks the context nine times before traversing destinations for
	// this two-node graph.
	for _, test := range []struct {
		name          string
		cancelOnCheck int
	}{
		{name: "Destination", cancelOnCheck: 10},
		{name: "BeforeReturn", cancelOnCheck: 11},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			cancelCtx := &cancelOnErrCheckContext{
				Context:         ctx,
				cancel:          cancel,
				remainingChecks: test.cancelOnCheck,
			}
			_, err := Parse(cancelCtx, rawGraph)
			require.ErrorIs(t, err, context.Canceled)
		})
	}
}
