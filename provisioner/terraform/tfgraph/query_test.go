package tfgraph_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

func TestQueryConfigurationNodesForReferences(t *testing.T) {
	t.Parallel()

	graph, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] module.workspace.data.coder_workspace.me (expand)"
		"[root] module.runtime (expand)"
		"[root] module.runtime.output.agent_id (expand)"
		"[root] module.runtime.output.token (expand)"
	}`)
	require.NoError(t, err)

	for _, test := range []struct {
		name          string
		moduleAddress string
		references    []string
		expected      []string
	}{
		{
			name:          "ModuleInstanceQualificationAndResourceFallback",
			moduleAddress: `module.workspace["alice"]`,
			references:    []string{"data.coder_workspace.me.name"},
			expected:      []string{"module.workspace.data.coder_workspace.me"},
		},
		{
			name:       "ModuleOutput",
			references: []string{`module.runtime["primary"].agent_id`},
			expected:   []string{"module.runtime.output.agent_id"},
		},
		{
			name: "PlanModuleOutputPrefix",
			references: []string{
				"module.runtime.agent_id",
				"module.runtime",
			},
			expected: []string{"module.runtime.output.agent_id"},
		},
		{
			name: "PlanKeyedModuleOutputPrefix",
			references: []string{
				`module.runtime["primary"].agent_id`,
				`module.runtime["primary"]`,
			},
			expected: []string{"module.runtime.output.agent_id"},
		},
		{
			name: "PlanNestedOutputTraversalPrefix",
			references: []string{
				"module.runtime.agent_id.value",
				"module.runtime.agent_id",
				"module.runtime",
			},
			expected: []string{"module.runtime.output.agent_id"},
		},
		{
			name:       "WholeModule",
			references: []string{`module.runtime`},
			expected: []string{
				"module.runtime.output.agent_id",
				"module.runtime.output.token",
			},
		},
		{
			name: "ExplicitWholeModuleAndOutput",
			references: []string{
				"module.runtime",
				"module.runtime.agent_id",
				"module.runtime",
			},
			expected: []string{
				"module.runtime.output.agent_id",
				"module.runtime.output.token",
			},
		},
		{
			name: "DifferentModuleInstances",
			references: []string{
				`module.runtime["primary"].agent_id`,
				`module.runtime["primary"]`,
				`module.runtime["secondary"]`,
			},
			expected: []string{
				"module.runtime.output.agent_id",
				"module.runtime.output.token",
			},
		},
		{
			name:       "MissingModuleOutput",
			references: []string{`module.runtime.missing`},
			expected:   []string{},
		},
		{
			name:       "MissingReference",
			references: []string{"local.missing"},
			expected:   []string{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			query, err := tfgraph.NewQuery(graph)
			require.NoError(t, err)
			nodes, err := query.ConfigurationNodesForReferences(
				t.Context(), test.moduleAddress, test.references,
			)
			require.NoError(t, err)
			if len(test.expected) == 0 {
				require.Empty(t, nodes)
				return
			}
			require.Equal(t, test.expected, nodeAddresses(t, graph, nodes))
		})
	}
}

func TestQueryReachableBoundaryNodes(t *testing.T) {
	t.Parallel()

	sourceGraph, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] coder_script.direct_to_agent[\"api\"]"
		"[root] coder_agent.service[\"api\"]"
		"[root] module.service[\"worker\"].coder_script.via_devcontainer"
		"[root] module.service[\"worker\"].coder_devcontainer.repo"
		"[root] module.service[\"worker\"].coder_agent.main"
		"[root] coder_script.converging"
		"[root] terraform_data.left"
		"[root] terraform_data.right"
		"[root] coder_agent.shared"
		"[root] coder_script.direct_to_agent[\"api\"]" -> "[root] coder_agent.service[\"api\"]"
		"[root] module.service[\"worker\"].coder_script.via_devcontainer" -> "[root] module.service[\"worker\"].coder_devcontainer.repo"
		"[root] module.service[\"worker\"].coder_devcontainer.repo" -> "[root] module.service[\"worker\"].coder_agent.main"
		"[root] coder_script.converging" -> "[root] terraform_data.left"
		"[root] coder_script.converging" -> "[root] terraform_data.right"
		"[root] terraform_data.left" -> "[root] coder_agent.shared"
		"[root] terraform_data.right" -> "[root] coder_agent.shared"
	}`)
	require.NoError(t, err)

	isAgentBoundary := func(node tfgraph.Node) bool {
		return strings.Contains(node.Address(), ".coder_agent.") ||
			strings.HasPrefix(node.Address(), "coder_agent.")
	}
	isRuntimeBoundary := func(node tfgraph.Node) bool {
		return isAgentBoundary(node) ||
			strings.Contains(node.Address(), ".coder_devcontainer.") ||
			strings.HasPrefix(node.Address(), "coder_devcontainer.")
	}

	for _, test := range []struct {
		name         string
		startAddress string
		isBoundary   func(tfgraph.Node) bool
		expected     []string
	}{
		{
			name:         "DirectToAgent",
			startAddress: `coder_script.direct_to_agent["api"]`,
			isBoundary:   isRuntimeBoundary,
			expected:     []string{`coder_agent.service["api"]`},
		},
		{
			name:         "ViaDevcontainerStopsAtRuntime",
			startAddress: `module.service["worker"].coder_script.via_devcontainer`,
			isBoundary:   isRuntimeBoundary,
			expected: []string{
				`module.service["worker"].coder_devcontainer.repo`,
			},
		},
		{
			name:         "ViaDevcontainerContinuesToAgent",
			startAddress: `module.service["worker"].coder_script.via_devcontainer`,
			isBoundary:   isAgentBoundary,
			expected: []string{
				`module.service["worker"].coder_agent.main`,
			},
		},
		// coder_script.converging
		// ├── terraform_data.left  ─┐
		// └── terraform_data.right ─┴─> coder_agent.shared
		{
			name:         "ConvergingPathsReturnBoundaryOnce",
			startAddress: "coder_script.converging",
			isBoundary:   isAgentBoundary,
			expected:     []string{"coder_agent.shared"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			query, err := tfgraph.NewQuery(sourceGraph)
			require.NoError(t, err)
			startNodes := sourceGraph.NodesForInstanceAddress(test.startAddress)
			boundaries, err := query.ReachableBoundaryNodes(
				t.Context(), startNodes, test.isBoundary,
			)
			require.NoError(t, err)
			require.Equal(
				t, test.expected, nodeAddresses(t, sourceGraph, boundaries),
			)
		})
	}

	t.Run("ResolvedDependenciesDoNotModifySourceGraph", func(t *testing.T) {
		t.Parallel()

		startNodes := sourceGraph.NodesForInstanceAddress(
			`coder_script.direct_to_agent["api"]`,
		)
		replacement := sourceGraph.NodesForInstanceAddress("coder_agent.shared")
		require.Len(t, replacement, 1)

		resolver := func(
			_ context.Context,
			_ tfgraph.ReferenceLookup,
			_ tfgraph.Node,
			graphDependencies []tfgraph.NodeID,
		) ([]tfgraph.NodeID, error) {
			graphDependencies[0] = replacement[0]
			return graphDependencies, nil
		}
		resolvedGraph, err := sourceGraph.WithResolvedDependencies(resolver)
		require.NoError(t, err)
		query, err := tfgraph.NewQuery(resolvedGraph)
		require.NoError(t, err)
		boundaries, err := query.ReachableBoundaryNodes(
			t.Context(), startNodes, isAgentBoundary,
		)
		require.NoError(t, err)
		require.Equal(
			t, []string{"coder_agent.shared"},
			nodeAddresses(t, resolvedGraph, boundaries),
		)

		query, err = tfgraph.NewQuery(sourceGraph)
		require.NoError(t, err)
		boundaries, err = query.ReachableBoundaryNodes(
			t.Context(), startNodes, isAgentBoundary,
		)
		require.NoError(t, err)
		require.Equal(
			t, []string{`coder_agent.service["api"]`},
			nodeAddresses(t, sourceGraph, boundaries),
		)
	})
}

func TestQueryFiltersModuleExpansionEdges(t *testing.T) {
	t.Parallel()

	sourceGraph, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] module.tool.var.direct_agent_id (expand)"
		"[root] module.tool.var.repeated_agent_id (expand)"
		"[root] module.tool.output.direct_agent_id (expand)"
		"[root] module.tool.output.constant (expand)"
		"[root] module.tool.output.inner (expand)"
		"[root] module.tool.local.constant (expand)"
		"[root] module.tool.terraform_data.inner (expand)"
		"[root] module.tool (expand)"
		"[root] coder_agent.main (expand)"
		"[root] coder_agent.source (expand)"
		"[root] coder_agent.other (expand)"
		"[root] module.tool.var.direct_agent_id (expand)" -> "[root] coder_agent.main (expand)"
		"[root] module.tool.var.direct_agent_id (expand)" -> "[root] module.tool (expand)"
		"[root] module.tool.var.repeated_agent_id (expand)" -> "[root] module.tool (expand)"
		"[root] module.tool.output.direct_agent_id (expand)" -> "[root] coder_agent.main (expand)"
		"[root] module.tool.output.direct_agent_id (expand)" -> "[root] module.tool (expand)"
		"[root] module.tool.output.constant (expand)" -> "[root] module.tool.local.constant (expand)"
		"[root] module.tool.output.constant (expand)" -> "[root] module.tool (expand)"
		"[root] module.tool.output.inner (expand)" -> "[root] module.tool.terraform_data.inner (expand)"
		"[root] module.tool.output.inner (expand)" -> "[root] module.tool (expand)"
		"[root] module.tool.local.constant (expand)" -> "[root] module.tool (expand)"
		"[root] module.tool.terraform_data.inner (expand)" -> "[root] module.tool (expand)"
		"[root] module.tool (expand)" -> "[root] coder_agent.source (expand)"
		"[root] module.tool (expand)" -> "[root] coder_agent.other (expand)"
	}`)
	require.NoError(t, err)

	// This models the module call's configuration: repeated_agent_id uses
	// each.value, coder_agent.source is the for_each expression dependency,
	// and coder_agent.other is a depends_on dependency. All other edges into
	// the module expansion node are ordering-only edges.
	resolver := func(
		_ context.Context,
		_ tfgraph.ReferenceLookup,
		source tfgraph.Node,
		graphDependencies []tfgraph.NodeID,
	) ([]tfgraph.NodeID, error) {
		sourceAddress := source.ConfigurationAddress()
		result := make([]tfgraph.NodeID, 0, len(graphDependencies))
		for _, dependencyID := range graphDependencies {
			dependency, _ := sourceGraph.Node(dependencyID)
			dependencyAddress := dependency.ConfigurationAddress()
			switch {
			case dependencyAddress == "module.tool" &&
				sourceAddress != "module.tool.var.repeated_agent_id":
				continue
			case sourceAddress == "module.tool" &&
				dependencyAddress != "coder_agent.source":
				continue
			}
			result = append(result, dependencyID)
		}
		return result, nil
	}

	for _, test := range []struct {
		name          string
		moduleAddress string
		reference     string
		expected      []string
	}{
		{
			name:          "DirectModuleInput",
			moduleAddress: "module.tool",
			reference:     "var.direct_agent_id",
			expected:      []string{"coder_agent.main"},
		},
		{
			name:          "ForEachModuleInput",
			moduleAddress: "module.tool",
			reference:     "var.repeated_agent_id",
			expected:      []string{"coder_agent.source"},
		},
		{
			name:      "DirectModuleOutput",
			reference: "module.tool.direct_agent_id",
			expected:  []string{"coder_agent.main"},
		},
		{
			name:      "ConstantLocalModuleOutput",
			reference: "module.tool.constant",
			expected:  []string{},
		},
		{
			name:      "InnerResourceModuleOutput",
			reference: "module.tool.inner",
			expected:  []string{},
		},
		{
			name:      "WholeModule",
			reference: "module.tool",
			expected:  []string{"coder_agent.main"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolvedGraph, err := sourceGraph.WithResolvedDependencies(resolver)
			require.NoError(t, err)
			query, err := tfgraph.NewQuery(resolvedGraph)
			require.NoError(t, err)
			start, err := query.ConfigurationNodesForReferences(
				t.Context(), test.moduleAddress, []string{test.reference},
			)
			require.NoError(t, err)
			boundaries, err := query.ReachableBoundaryNodes(
				t.Context(), start,
				func(node tfgraph.Node) bool {
					return strings.HasPrefix(
						node.ConfigurationAddress(), "coder_agent.",
					)
				},
			)
			require.NoError(t, err)
			require.Equal(
				t, test.expected, nodeAddresses(t, resolvedGraph, boundaries),
			)
		})
	}
}

func TestQueryMatchesCapturedTerraformGraphs(t *testing.T) {
	t.Parallel()

	t.Run("SavedPlanFollowsRepeatedRuntimeInstanceDependency", func(t *testing.T) {
		t.Parallel()

		// The saved-plan graph contains two devcontainer instances. The script
		// depends specifically on repo[1], so traversal must return repo[1], not
		// repo[0] or the resource's expansion node.
		graph := graphFromFile(
			t, "testdata/repeated-runtime.tfplan.saved.dot",
		)
		start := graph.NodesForInstanceAddress("coder_script.prerequisite")
		require.Len(t, start, 1)
		query, err := tfgraph.NewQuery(graph)
		require.NoError(t, err)
		boundaries, err := query.ReachableBoundaryNodes(
			t.Context(), start,
			func(node tfgraph.Node) bool {
				return strings.HasPrefix(
					node.InstanceAddress(), "coder_devcontainer.repo[",
				)
			},
		)
		require.NoError(t, err)
		require.Equal(t,
			[]string{"coder_devcontainer.repo[1]"},
			nodeAddresses(t, graph, boundaries),
		)
	})

	t.Run("ConfigDerivedPlanTraversesExpansionNodeDependency", func(t *testing.T) {
		t.Parallel()

		// This config-derived plan graph expresses the devcontainer-to-agent
		// dependency through configuration expansion nodes rather than instances.
		graph := graphFromFile(
			t, "../testdata/resources/devcontainer/devcontainer.tfplan.dot",
		)
		start := graph.NodesForConfigurationAddress("coder_devcontainer.dev1")
		require.Len(t, start, 1)
		query, err := tfgraph.NewQuery(graph)
		require.NoError(t, err)
		boundaries, err := query.ReachableBoundaryNodes(
			t.Context(), start,
			func(node tfgraph.Node) bool {
				return node.ConfigurationAddress() == "coder_agent.main"
			},
		)
		require.NoError(t, err)
		require.Equal(
			t, []string{"coder_agent.main"},
			nodeAddresses(t, graph, boundaries),
		)
	})

	t.Run("ConfigDerivedPlanFollowsModuleInputDependency", func(t *testing.T) {
		t.Parallel()

		graph := graphFromFile(
			t,
			"../testdata/resources/calling-module/calling-module.tfplan.dot",
		)
		query, err := tfgraph.NewQuery(graph)
		require.NoError(t, err)
		// The DOT file declares var.script only as the shared source
		// of two edges.  DOT treats edge endpoints as nodes, so
		// reference lookup returns one node.
		startNodes, err := query.ConfigurationNodesForReferences(
			t.Context(), "module.module", []string{"var.script"},
		)
		require.NoError(t, err)
		require.Equal(t,
			[]string{"module.module.var.script"},
			nodeAddresses(t, graph, startNodes),
		)
		boundaries, err := query.ReachableBoundaryNodes(
			t.Context(), startNodes,
			func(node tfgraph.Node) bool {
				return node.ConfigurationAddress() == "coder_agent.main"
			},
		)
		require.NoError(t, err)
		require.Equal(t,
			[]string{"coder_agent.main"},
			nodeAddresses(t, graph, boundaries),
		)
	})

	t.Run("ConfigDependenciesRestoreReducedModuleOutputPath", func(t *testing.T) {
		t.Parallel()

		// The module input and depends_on reference the same resource. Starting
		// from the module output, Terraform's transitive reduction leaves only:
		//
		// output.agent_id -> var.agent_id -> module.tool -> terraform_data.agent
		//
		// The dependency resolver uses plan configuration at both the output and
		// input nodes, restoring the omitted value dependency during traversal.
		sourceGraph := graphFromFile(
			t, "testdata/reduced-module-input-edge.tfplan.dot",
		)
		type configuredDependencies struct {
			moduleAddress string
			references    []string
		}
		configured := map[string]configuredDependencies{
			"module.tool.output.agent_id": {
				moduleAddress: "module.tool",
				references:    []string{"var.agent_id"},
			},
			"module.tool.var.agent_id": {
				references: []string{"terraform_data.agent.id"},
			},
			"module.tool.output.unrelated_id": {
				moduleAddress: "module.tool",
				references:    []string{"var.unrelated_id"},
			},
			"module.tool.var.unrelated_id": {},
		}

		for _, test := range []struct {
			name               string
			reference          string
			expectedBoundaries []string
		}{
			{
				name:               "ReferencedOutput",
				reference:          "module.tool.agent_id",
				expectedBoundaries: []string{"terraform_data.agent"},
			},
			{
				name:               "UnrelatedOutput",
				reference:          "module.tool.unrelated_id",
				expectedBoundaries: []string{},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				resolver := func(
					_ context.Context,
					lookup tfgraph.ReferenceLookup,
					source tfgraph.Node,
					graphDependencies []tfgraph.NodeID,
				) ([]tfgraph.NodeID, error) {
					valueDependencies, ok := configured[source.ConfigurationAddress()]
					if !ok {
						return graphDependencies, nil
					}
					return lookup(
						valueDependencies.moduleAddress,
						valueDependencies.references,
					)
				}
				resolvedGraph, err := sourceGraph.WithResolvedDependencies(resolver)
				require.NoError(t, err)
				query, err := tfgraph.NewQuery(resolvedGraph)
				require.NoError(t, err)
				startNodes, err := query.ConfigurationNodesForReferences(
					t.Context(), "", []string{test.reference},
				)
				require.NoError(t, err)

				boundaries, err := query.ReachableBoundaryNodes(
					t.Context(), startNodes,
					func(node tfgraph.Node) bool {
						return node.ConfigurationAddress() ==
							"terraform_data.agent"
					},
				)
				require.NoError(t, err)
				require.Equal(
					t, test.expectedBoundaries,
					nodeAddresses(t, resolvedGraph, boundaries),
				)
			})
		}
	})

	t.Run("HCLDependenciesRestoreReducedLocalPath", func(t *testing.T) {
		t.Parallel()

		// Terraform plan configuration omits locals. Parsed HCL shows that
		// local.both references both agents, while Terraform's reduced graph
		// retains only its edge to agent_b.
		sourceGraph := graphFromFile(
			t, "testdata/reduced-local-value-edge.tfplan.dot",
		)
		startNodes := sourceGraph.NodesForConfigurationAddress(
			"terraform_data.script",
		)
		require.Len(t, startNodes, 1)
		boundaryAddresses := func(t *testing.T, graph *tfgraph.Graph) []string {
			t.Helper()

			query, err := tfgraph.NewQuery(graph)
			require.NoError(t, err)
			boundaries, err := query.ReachableBoundaryNodes(
				t.Context(), startNodes,
				func(node tfgraph.Node) bool {
					return node.Address() == "terraform_data.agent_a" ||
						node.Address() == "terraform_data.agent_b"
				},
			)
			require.NoError(t, err)
			return nodeAddresses(t, graph, boundaries)
		}

		t.Run("WithoutResolver", func(t *testing.T) {
			t.Parallel()

			require.Equal(
				t, []string{"terraform_data.agent_b"},
				boundaryAddresses(t, sourceGraph),
			)
		})

		t.Run("WithRestoredLocalReferences", func(t *testing.T) {
			t.Parallel()

			resolver := func(
				_ context.Context,
				lookup tfgraph.ReferenceLookup,
				source tfgraph.Node,
				graphDependencies []tfgraph.NodeID,
			) ([]tfgraph.NodeID, error) {
				if source.ConfigurationAddress() != "local.both" {
					return graphDependencies, nil
				}
				return lookup("", []string{
					"terraform_data.agent_a.id",
					"terraform_data.agent_b.id",
				})
			}
			resolvedGraph, err := sourceGraph.WithResolvedDependencies(resolver)
			require.NoError(t, err)

			require.Equal(
				t,
				[]string{
					"terraform_data.agent_a",
					"terraform_data.agent_b",
				},
				boundaryAddresses(t, resolvedGraph),
			)
		})
	})

	t.Run("HCLNarrowingSelectsDynamicModuleOutput", func(t *testing.T) {
		t.Parallel()

		// Terraform reports the plan references for
		// module.rep[each.key].agent_id as only module.rep and each.key. The
		// parsed HCL expression AST retains the selected output name.
		sourceGraph := graphFromFile(
			t, "testdata/dynamic-module-output.tfplan.dot",
		)
		startNodes := sourceGraph.NodesForConfigurationAddress(
			"terraform_data.script",
		)
		require.Len(t, startNodes, 1)
		boundaryAddresses := func(t *testing.T, references []string) []string {
			t.Helper()

			resolver := func(
				_ context.Context,
				lookup tfgraph.ReferenceLookup,
				source tfgraph.Node,
				graphDependencies []tfgraph.NodeID,
			) ([]tfgraph.NodeID, error) {
				if source.ConfigurationAddress() != "terraform_data.script" {
					return graphDependencies, nil
				}
				return lookup("", references)
			}
			resolvedGraph, err := sourceGraph.WithResolvedDependencies(resolver)
			require.NoError(t, err)
			query, err := tfgraph.NewQuery(resolvedGraph)
			require.NoError(t, err)
			boundaries, err := query.ReachableBoundaryNodes(
				t.Context(), startNodes,
				func(node tfgraph.Node) bool {
					return node.Address() == "terraform_data.agent" ||
						node.Address() == "terraform_data.other"
				},
			)
			require.NoError(t, err)
			return nodeAddresses(t, resolvedGraph, boundaries)
		}

		t.Run("PlanReferences", func(t *testing.T) {
			t.Parallel()

			require.Equal(
				t,
				[]string{
					"terraform_data.agent",
					"terraform_data.other",
				},
				boundaryAddresses(t, []string{"module.rep", "each.key"}),
			)
		})

		t.Run("ASTNarrowedReferences", func(t *testing.T) {
			t.Parallel()

			require.Equal(
				t, []string{"terraform_data.agent"},
				boundaryAddresses(
					t, []string{"module.rep.agent_id", "each.key"},
				),
			)
		})
	})

	t.Run("PlanGraphSkipsModuleCloseCompletionOnlyEdges", func(t *testing.T) {
		t.Parallel()

		sourceGraph := graphFromFile(
			t, "testdata/whole-module-reference.tfplan.dot",
		)
		boundaryAddresses := func(t *testing.T, graph *tfgraph.Graph) []string {
			t.Helper()

			query, err := tfgraph.NewQuery(graph)
			require.NoError(t, err)
			startNodes, err := query.ConfigurationNodesForReferences(
				t.Context(), "", []string{"module.runtime.component"},
			)
			require.NoError(t, err)
			require.Equal(
				t, []string{"module.runtime.output.component"},
				nodeAddresses(t, graph, startNodes),
			)

			// A module close node depends on both its output nodes and unrelated
			// nodes that only order completion. Only outputs contribute to the
			// whole-module value.
			boundaries, err := query.ReachableBoundaryNodes(
				t.Context(), startNodes,
				func(node tfgraph.Node) bool {
					return strings.HasPrefix(
						node.ConfigurationAddress(),
						"module.runtime.module.component.terraform_data.",
					)
				},
			)
			require.NoError(t, err)
			return nodeAddresses(t, graph, boundaries)
		}
		expected := []string{
			"module.runtime.module.component.terraform_data.referenced",
		}

		t.Run("WithoutResolver", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, expected, boundaryAddresses(t, sourceGraph))
		})

		t.Run("WithResolver", func(t *testing.T) {
			t.Parallel()

			unrelated := sourceGraph.NodesForConfigurationAddress(
				"module.runtime.module.component.terraform_data.unrelated",
			)
			require.Len(t, unrelated, 1)
			resolvedModuleClose := false
			resolver := func(
				_ context.Context,
				_ tfgraph.ReferenceLookup,
				source tfgraph.Node,
				graphDependencies []tfgraph.NodeID,
			) ([]tfgraph.NodeID, error) {
				if source.Address() != "module.runtime.module.component" ||
					source.Operation() != "close" {
					return graphDependencies, nil
				}
				resolvedModuleClose = true
				require.Equal(
					t,
					[]string{"module.runtime.module.component.output.result"},
					nodeAddresses(t, sourceGraph, graphDependencies),
				)
				return append(graphDependencies, unrelated[0]), nil
			}
			resolvedGraph, err := sourceGraph.WithResolvedDependencies(resolver)
			require.NoError(t, err)

			require.Equal(t, expected, boundaryAddresses(t, resolvedGraph))
			require.True(t, resolvedModuleClose)
		})
	})
}

func TestQueryResultsAreDeterministic(t *testing.T) {
	t.Parallel()

	graph, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] local.second (expand)"
		"[root] local.first (expand)"
		"[root] local.start (expand)"
		"[root] null_resource.second"
		"[root] null_resource.first"
		"[root] local.start (expand)" -> "[root] null_resource.second"
		"[root] local.start (expand)" -> "[root] null_resource.first"
	}`)
	require.NoError(t, err)
	query, err := tfgraph.NewQuery(graph)
	require.NoError(t, err)
	startNodes := graph.NodesForConfigurationAddress("local.start")
	require.Len(t, startNodes, 1)
	//nolint:gocritic // Intentionally duplicate the start node.
	duplicatedStartNodes := append(startNodes, startNodes[0])

	for range 3 {
		configurationNodes, err := query.ConfigurationNodesForReferences(
			t.Context(), "", []string{"local.second", "local.first"},
		)
		require.NoError(t, err)
		require.Equal(
			t, []string{"local.first", "local.second"},
			nodeAddresses(t, graph, configurationNodes),
		)

		boundaries, err := query.ReachableBoundaryNodes(
			t.Context(), duplicatedStartNodes, func(node tfgraph.Node) bool {
				return strings.HasPrefix(node.Address(), "null_resource.")
			},
		)
		require.NoError(t, err)
		require.Equal(
			t,
			[]string{"null_resource.first", "null_resource.second"},
			nodeAddresses(t, graph, boundaries),
		)
	}
}

func TestQueryRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	_, err := tfgraph.NewQuery(nil)
	require.ErrorContains(t, err, "Terraform graph is required")
	_, err = tfgraph.NewQuery(&tfgraph.Graph{})
	require.ErrorContains(t, err, "Terraform graph is required")

	first, err := tfgraph.Parse(t.Context(), `digraph { "[root] local.first" }`)
	require.NoError(t, err)
	_, err = first.WithResolvedDependencies(nil)
	require.ErrorContains(t, err, "dependency resolver is required")

	second, err := tfgraph.Parse(t.Context(), `digraph { "[root] local.second" }`)
	require.NoError(t, err)
	firstQuery, err := tfgraph.NewQuery(first)
	require.NoError(t, err)

	_, err = firstQuery.ReachableBoundaryNodes(
		t.Context(), []tfgraph.NodeID{{}}, func(tfgraph.Node) bool { return false },
	)
	require.ErrorContains(t, err, "outside its graph")
	_, err = firstQuery.ReachableBoundaryNodes(
		t.Context(), second.NodesForInstanceAddress("local.second"),
		func(tfgraph.Node) bool { return false },
	)
	require.ErrorContains(t, err, "outside its graph")
	_, err = firstQuery.ReachableBoundaryNodes(
		t.Context(), nil, nil,
	)
	require.ErrorContains(t, err, "boundary predicate is required")

	foreignResolver := func(
		_ context.Context,
		_ tfgraph.ReferenceLookup,
		_ tfgraph.Node,
		_ []tfgraph.NodeID,
	) ([]tfgraph.NodeID, error) {
		return second.NodesForInstanceAddress("local.second"), nil
	}
	foreignGraph, err := first.WithResolvedDependencies(foreignResolver)
	require.NoError(t, err)
	foreignQuery, err := tfgraph.NewQuery(foreignGraph)
	require.NoError(t, err)
	_, err = foreignQuery.ReachableBoundaryNodes(
		t.Context(), first.NodesForInstanceAddress("local.first"),
		func(tfgraph.Node) bool { return false },
	)
	require.ErrorContains(t, err, "resolver returned a node outside its graph")

	errorResolver := func(
		_ context.Context,
		_ tfgraph.ReferenceLookup,
		_ tfgraph.Node,
		_ []tfgraph.NodeID,
	) ([]tfgraph.NodeID, error) {
		return nil, context.Canceled
	}
	errorGraph, err := first.WithResolvedDependencies(errorResolver)
	require.NoError(t, err)
	errorQuery, err := tfgraph.NewQuery(errorGraph)
	require.NoError(t, err)
	_, err = errorQuery.ReachableBoundaryNodes(
		t.Context(), first.NodesForInstanceAddress("local.first"),
		func(tfgraph.Node) bool { return false },
	)
	require.ErrorIs(t, err, context.Canceled)
}

func TestQueryHonorsCancellation(t *testing.T) {
	t.Parallel()

	graph, err := tfgraph.Parse(t.Context(), `digraph {
		"[root] local.first (expand)"
		"[root] local.second (expand)"
		"[root] local.first (expand)" -> "[root] local.second (expand)"
	}`)
	require.NoError(t, err)

	t.Run("BeforeEntry", func(t *testing.T) {
		t.Parallel()

		query, err := tfgraph.NewQuery(graph)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err = query.ConfigurationNodesForReferences(
			ctx, "", []string{"local.first"},
		)
		require.ErrorIs(t, err, context.Canceled)
		_, err = query.ReachableBoundaryNodes(
			ctx, graph.NodesForConfigurationAddress("local.first"),
			func(tfgraph.Node) bool { return false },
		)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("BetweenReferences", func(t *testing.T) {
		t.Parallel()

		query, err := tfgraph.NewQuery(graph)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		_, err = query.ConfigurationNodesForReferences(
			&cancelAfterContextChecks{
				Context:           ctx,
				cancel:            cancel,
				checksUntilCancel: 3,
			},
			"", []string{"local.first", "local.second"},
		)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("BetweenNodes", func(t *testing.T) {
		t.Parallel()

		query, err := tfgraph.NewQuery(graph)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		visited := 0
		_, err = query.ReachableBoundaryNodes(
			ctx,
			graph.NodesForConfigurationAddress("local.first"),
			func(tfgraph.Node) bool {
				visited++
				cancel()
				return false
			},
		)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, visited)
	})
}

type cancelAfterContextChecks struct {
	context.Context
	cancel            context.CancelFunc
	checksUntilCancel int
}

func (c *cancelAfterContextChecks) Err() error {
	c.checksUntilCancel--
	if c.checksUntilCancel == 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func graphFromFile(t *testing.T, path string) *tfgraph.Graph {
	t.Helper()

	rawGraph, err := os.ReadFile(path)
	require.NoError(t, err)
	graph, err := tfgraph.Parse(t.Context(), string(rawGraph))
	require.NoError(t, err)
	return graph
}

func nodeAddresses(
	t *testing.T,
	graph *tfgraph.Graph,
	nodeIDs []tfgraph.NodeID,
) []string {
	t.Helper()

	addresses := make([]string, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, ok := graph.Node(nodeID)
		require.True(t, ok)
		addresses = append(addresses, node.Address())
	}
	return addresses
}
