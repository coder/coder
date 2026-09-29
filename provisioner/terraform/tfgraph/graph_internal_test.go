package tfgraph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexRejectsInvalidOrExcessiveInput(t *testing.T) {
	t.Parallel()

	validGraph := `digraph {
		"[root] local.bridge (expand)"
		"[root] coder_agent.main (expand)"
		"[root] local.bridge (expand)" -> "[root] coder_agent.main (expand)"
	}`
	for _, test := range []struct {
		name          string
		rawGraph      string
		configure     func(*indexLimits)
		errorContains string
	}{
		{
			name:     "MalformedGraph",
			rawGraph: "not a graph",
			configure: func(*indexLimits) {
			},
			errorContains: "parse Terraform graph",
		},
		{
			name: "DuplicateNodeStatementsExceedLimitBeforeParsing",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
				"[root] coder_agent.main (expand)"
				invalid [
			}`,
			configure: func(limits *indexLimits) {
				limits.nodes = 1
			},
			errorContains: "Terraform graph nodes",
		},
		{
			name: "DuplicateEdgeStatementsExceedLimitBeforeParsing",
			rawGraph: `digraph {
				"[root] local.bridge (expand)" -> "[root] coder_agent.main (expand)"
				"[root] local.bridge (expand)" -> "[root] coder_agent.main (expand)"
				invalid [
			}`,
			configure: func(limits *indexLimits) {
				limits.edges = 1
			},
			errorContains: "Terraform graph edges",
		},
		{
			name: "GroupedSourceEndpoint",
			rawGraph: `digraph {
				{ "[root] first"; "[root] second" } -> "[root] third"
			}`,
			configure: func(*indexLimits) {
			},
			errorContains: "unsupported edge syntax",
		},
		{
			name: "GroupedSourceEndpointWithoutWhitespace",
			rawGraph: `digraph {
				{"[root] first";"[root] second"}->"[root] third"
			}`,
			configure: func(*indexLimits) {
			},
			errorContains: "unsupported edge syntax",
		},
		{
			name:     "GroupedSourceEndpointWithTabs",
			rawGraph: "digraph {\n\t{ \"[root] first\" }\t->\t\"[root] third\"\n}",
			configure: func(*indexLimits) {
			},
			errorContains: "unsupported edge syntax",
		},
		{
			name: "GroupedSourceEndpointWithComment",
			rawGraph: `digraph {
				{ "[root] first" } /* grouped source */ -> "[root] third"
			}`,
			configure: func(*indexLimits) {
			},
			errorContains: "unsupported edge syntax",
		},
		{
			name: "MultilineGroupedSourceEndpoint",
			rawGraph: `digraph {
				{
					"[root] first"
					"[root] second"
				}
				->
				"[root] third"
			}`,
			configure: func(*indexLimits) {
			},
			errorContains: "unsupported edge syntax",
		},
		{
			name: "GroupedDestinationEndpoint",
			rawGraph: `digraph {
				"[root] first" -> { "[root] second"; "[root] third" }
			}`,
			configure: func(*indexLimits) {
			},
			errorContains: "unsupported edge syntax",
		},
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
			input := test.rawGraph
			if input == "" {
				input = validGraph
			}
			_, err := parseWithLimits(t.Context(), input, limits)
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

func TestPreflightCountsDOTStructure(t *testing.T) {
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
