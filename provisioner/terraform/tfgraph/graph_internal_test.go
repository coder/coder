package tfgraph

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/awalterschulze/gographviz"
	"github.com/stretchr/testify/require"
)

func TestIndexRejectsInvalidOrExcessiveInput(t *testing.T) {
	t.Parallel()

	rawGraph := `digraph {
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
			name: "DuplicateNodeStatementsBeforeParsing",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
				"[root] coder_agent.main (expand)"
				invalid [
			}`,
			configure: func(limits *indexLimits) {
				limits.statements = 1
			},
			errorContains: "DOT statements",
		},
		{
			name: "DuplicateEdgeStatementsBeforeParsing",
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
			name: "SubgraphEdgeEndpoint",
			rawGraph: `digraph {
				{ "[root] first"; "[root] second" } -> "[root] third"
			}`,
			configure: func(*indexLimits) {
			},
			errorContains: "unsupported subgraph edge endpoint",
		},
		{
			name: "InputBytes",
			configure: func(limits *indexLimits) {
				limits.inputBytes = len(rawGraph) - 1
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
				input = rawGraph
			}
			_, err := parseWithLimits(t.Context(), input, limits)
			require.ErrorContains(t, err, test.errorContains)
		})
	}
}

func TestPreflightCountsDOTStructure(t *testing.T) {
	t.Parallel()

	rawGraph := `digraph {
		compound = "true"
		subgraph "root" {
			"[root] local.bridge (expand)" [label = "not -> an edge"]
			"[root] coder_agent.main (expand)" [label = "coder_agent.main"]
			// "[root] ignored" -> "[root] ignored"
			"[root] local.bridge (expand)" -> "[root] coder_agent.main (expand)"
		}
	}`
	limits := defaultIndexLimits()
	limits.statements = 3
	limits.nodes = 2
	limits.edges = 1
	require.NoError(t, preflight(t.Context(), rawGraph, limits))

	limits.statements = 2
	require.ErrorContains(
		t, preflight(t.Context(), rawGraph, limits), "DOT statements",
	)
	limits.statements = 3
	limits.nodes = 1
	require.ErrorContains(
		t, preflight(t.Context(), rawGraph, limits), "Terraform graph nodes",
	)
	limits.nodes = 2
	limits.edges = 0
	require.ErrorContains(
		t, preflight(t.Context(), rawGraph, limits), "Terraform graph edges",
	)
}

func TestIndexHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Parse(ctx, "digraph {}")
	require.ErrorIs(t, err, context.Canceled)

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		parserStarted := make(chan struct{})
		parserRelease := make(chan struct{})
		parserResult := make(chan error, 1)
		go func() {
			_, err := runParser(
				ctx,
				func() (*gographviz.Graph, error) {
					close(parserStarted)
					<-parserRelease
					return &gographviz.Graph{}, nil
				},
			)
			parserResult <- err
		}()

		<-parserStarted
		cancel()
		synctest.Wait()
		returnedBeforeParserFinished := false
		select {
		case <-parserResult:
			returnedBeforeParserFinished = true
		default:
		}
		close(parserRelease)
		require.False(t, returnedBeforeParserFinished)
		require.ErrorIs(t, <-parserResult, context.Canceled)
	})
}
