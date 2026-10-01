package agentruntime

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

func TestRuntimeResolverDirectReferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rawGraph      string
		config        *tfjson.Config
		resource      *tfjson.StateResource
		targets       []Target
		workspaceOnly bool
		expected      Target
		errorContains string
	}{
		{
			name: "WorkspaceAgent",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.main.id",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			targets: []Target{
				{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
			},
			expected: Target{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
		},
		{
			name: "UnavailableAgentStopsTraversal",
			rawGraph: `digraph {
				"[root] coder_agent.unavailable (expand)"
				"[root] coder_agent.available (expand)"
				"[root] coder_agent.unavailable (expand)" -> "[root] coder_agent.available (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.unavailable.id",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			targets: []Target{
				{Kind: KindWorkspaceAgent, Address: "coder_agent.available"},
			},
			errorContains: "does not resolve to an agent runtime",
		},
		{
			name: "DevcontainerStopsAtSubagent",
			rawGraph: `digraph {
				"[root] coder_devcontainer.repo (expand)"
				"[root] coder_agent.main (expand)"
				"[root] coder_devcontainer.repo (expand)" -> "[root] coder_agent.main (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_devcontainer.repo.subagent_id",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			targets: []Target{
				{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
				{Kind: KindDevcontainer, Address: "coder_devcontainer.repo"},
			},
			expected: Target{Kind: KindDevcontainer, Address: "coder_devcontainer.repo"},
		},
		{
			name: "DevcontainerAgentIDTraversesToParentWorkspaceAgent",
			rawGraph: `digraph {
				"[root] coder_devcontainer.repo (expand)"
				"[root] coder_agent.main (expand)"
				"[root] coder_devcontainer.repo (expand)" -> "[root] coder_agent.main (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_devcontainer.repo.agent_id",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			targets: []Target{
				{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
				{Kind: KindDevcontainer, Address: "coder_devcontainer.repo"},
			},
			expected: Target{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
		},
		{
			name: "AgentAndDevcontainerAreAmbiguous",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
				"[root] coder_devcontainer.repo (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work",
					"coder_agent.main.id",
					"coder_devcontainer.repo.subagent_id",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			targets: []Target{
				{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
				{Kind: KindDevcontainer, Address: "coder_devcontainer.repo"},
			},
			errorContains: "may refer to both devcontainer subagents",
		},
		{
			name: "RepeatedDevcontainerIsAmbiguous",
			rawGraph: `digraph {
				"[root] coder_devcontainer.repo (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_devcontainer.repo.subagent_id",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			targets: []Target{
				{Kind: KindDevcontainer, Address: `coder_devcontainer.repo["api"]`},
				{Kind: KindDevcontainer, Address: `coder_devcontainer.repo["worker"]`},
			},
			errorContains: "agent_id may refer to multiple devcontainer subagents",
		},
		{
			name: "DirectReferenceIgnoresUnrelatedDependency",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
				"[root] coder_agent.helper (expand)"
				"[root] coder_agent.main (expand)" -> "[root] coder_agent.helper (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.main.id",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			targets: []Target{
				{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
				{Kind: KindWorkspaceAgent, Address: "coder_agent.helper"},
			},
			expected: Target{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
		},
		{
			name: "DevcontainerParentWorkspaceAgent",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_devcontainer", "repo", "coder_agent.main.id",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_devcontainer.repo", "coder_devcontainer", "repo",
			),
			targets: []Target{
				{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
				{Kind: KindDevcontainer, Address: "coder_devcontainer.repo"},
			},
			workspaceOnly: true,
			expected:      Target{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
		},
		{
			name: "DevcontainerParentMissing",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_devcontainer", "repo", "local.missing",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_devcontainer.repo", "coder_devcontainer", "repo",
			),
			targets: []Target{
				{Kind: KindWorkspaceAgent, Address: "coder_agent.main"},
				{Kind: KindDevcontainer, Address: "coder_devcontainer.repo"},
			},
			workspaceOnly: true,
			errorContains: "resolves to 0 workspace agents",
		},
		{
			name: "WorkspaceAgentRejectsDevcontainer",
			rawGraph: `digraph {
				"[root] coder_devcontainer.other (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_devcontainer", "repo",
					"coder_devcontainer.other.subagent_id",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_devcontainer.repo", "coder_devcontainer", "repo",
			),
			targets: []Target{
				{Kind: KindDevcontainer, Address: "coder_devcontainer.repo"},
				{Kind: KindDevcontainer, Address: "coder_devcontainer.other"},
			},
			workspaceOnly: true,
			errorContains: "a workspace agent is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolver := runtimeResolverForTest(
				t, test.rawGraph, test.config, test.targets,
			)
			var actual Target
			var err error
			if test.workspaceOnly {
				actual, err = resolver.ResolveWorkspaceAgent(
					t.Context(), test.resource,
				)
			} else {
				actual, err = resolver.ResolveResourceRuntime(
					t.Context(), test.resource,
				)
			}
			if test.errorContains != "" {
				require.ErrorContains(t, err, test.errorContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestMostSpecificTerraformReferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		references []string
		expected   []string
	}{
		{
			name: "IndexedTraversalPrefixes",
			references: []string{
				`coder_agent.main[0].id`,
				`coder_agent.main[0]`,
				"coder_agent.main",
			},
			expected: []string{`coder_agent.main[0].id`},
		},
		{
			name: "IndependentUnindexedTraversal",
			references: []string{
				`coder_agent.main[0].id`,
				`coder_agent.main[0]`,
				"coder_agent.main",
				"coder_agent.main",
			},
			expected: []string{
				`coder_agent.main[0].id`, "coder_agent.main",
			},
		},
		{
			name: "RepeatedIndexedTraversal",
			references: []string{
				`coder_agent.main[0].id`,
				`coder_agent.main[0]`,
				"coder_agent.main",
				`coder_agent.main[0].id`,
				`coder_agent.main[0]`,
				"coder_agent.main",
			},
			expected: []string{`coder_agent.main[0].id`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			actual, err := mostSpecificTerraformReferences(
				t.Context(), test.references,
			)
			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}

func runtimeResolverForTest(
	t *testing.T,
	rawGraph string,
	config *tfjson.Config,
	targets []Target,
) *Resolver {
	t.Helper()

	graph, err := tfgraph.Parse(t.Context(), rawGraph)
	require.NoError(t, err)
	program, err := NewProgram(t.Context(), config)
	require.NoError(t, err)
	resolver, err := NewResolver(t.Context(), graph, program, targets)
	require.NoError(t, err)
	return resolver
}

func runtimeResolverTestConfigResource(
	resourceType string,
	name string,
	references ...string,
) *tfjson.ConfigResource {
	return &tfjson.ConfigResource{
		Address: resourceType + "." + name,
		Mode:    tfjson.ManagedResourceMode,
		Type:    resourceType,
		Name:    name,
		Expressions: map[string]*tfjson.Expression{
			"agent_id": {
				ExpressionData: &tfjson.ExpressionData{References: references},
			},
		},
	}
}

func runtimeResolverTestStateResource(
	address string,
	resourceType string,
	name string,
) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    resourceType,
		Name:    name,
	}
}

func runtimeResolverTestRootConfig(
	resources ...*tfjson.ConfigResource,
) *tfjson.Config {
	return &tfjson.Config{RootModule: &tfjson.ConfigModule{
		Resources: resources,
	}}
}
