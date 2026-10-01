package agentruntime_test

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/agentruntime"
	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

func TestProgramDoesNotRequireScriptOrderDeclaration(t *testing.T) {
	t.Parallel()

	program, err := agentruntime.NewProgram(t.Context(), genericResourceConfig(
		"coder_app", "dashboard", "coder_agent.main.id",
	))
	require.NoError(t, err)
	require.NotNil(t, program)
}

func TestResolverResolvesGenericAgentIDResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reference string
		rawGraph  string
		target    agentruntime.Target
	}{
		{
			name:      "WorkspaceAgent",
			reference: "coder_agent.main.id",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
			}`,
			target: agentruntime.Target{
				Kind:    agentruntime.KindWorkspaceAgent,
				Address: "coder_agent.main",
			},
		},
		{
			name:      "DevcontainerSubagent",
			reference: "coder_devcontainer.repo.subagent_id",
			rawGraph: `digraph {
				"[root] coder_devcontainer.repo (expand)"
			}`,
			target: agentruntime.Target{
				Kind:    agentruntime.KindDevcontainer,
				Address: "coder_devcontainer.repo",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			config := genericResourceConfig(
				"coder_app", "dashboard", test.reference,
			)
			program, err := agentruntime.NewProgram(t.Context(), config)
			require.NoError(t, err)
			graph, err := tfgraph.Parse(t.Context(), test.rawGraph)
			require.NoError(t, err)
			resolver, err := agentruntime.NewResolver(
				t.Context(), graph, program, []agentruntime.Target{test.target},
			)
			require.NoError(t, err)

			actual, err := resolver.ResolveResourceRuntime(
				t.Context(), &tfjson.StateResource{
					Address: "coder_app.dashboard",
					Mode:    tfjson.ManagedResourceMode,
					Type:    "coder_app",
					Name:    "dashboard",
				},
			)
			require.NoError(t, err)
			require.Equal(t, test.target, actual)
		})
	}
}

func genericResourceConfig(
	resourceType string,
	name string,
	reference string,
) *tfjson.Config {
	return &tfjson.Config{RootModule: &tfjson.ConfigModule{
		Resources: []*tfjson.ConfigResource{
			{
				Address: resourceType + "." + name,
				Mode:    tfjson.ManagedResourceMode,
				Type:    resourceType,
				Name:    name,
				Expressions: map[string]*tfjson.Expression{
					"agent_id": {
						ExpressionData: &tfjson.ExpressionData{
							References: []string{reference},
						},
					},
				},
			},
		},
	}}
}
