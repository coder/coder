package tfgraph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestIndexRetainsNodeLabels(t *testing.T) {
	t.Parallel()

	index, err := Parse(t.Context(), `digraph {
		"[root] coder_agent.main" [label = "coder_agent.main"]
	}`)
	require.NoError(t, err)
	require.Len(t, index.nodes, 1)
	require.Equal(t, `"coder_agent.main"`, index.nodes[0].label)
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

func TestParseHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Parse(ctx, "digraph {}")
	require.ErrorIs(t, err, context.Canceled)
}
