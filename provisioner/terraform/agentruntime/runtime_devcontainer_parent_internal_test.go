package agentruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/tfgraph"
)

func TestRuntimeResolverResolvesIndirectDevcontainerParents(t *testing.T) {
	t.Parallel()

	t.Run("CorrelatedLocalCollection", func(t *testing.T) {
		t.Parallel()

		workdir := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(workdir, "main.tf"),
			[]byte(`locals {
  repos = coder_devcontainer.repo
}

resource "coder_agent" "main" {
  for_each = {}
}

resource "coder_devcontainer" "repo" {
  for_each = coder_agent.main
  agent_id  = each.value.id
}

resource "coder_script" "work" {
  for_each = local.repos
  agent_id = each.value.agent_id
}
`),
			0o600,
		))
		script := runtimeResolverTestConfigResource(
			"coder_script", "work", "each.value.agent_id", "each.value",
		)
		script.ForEachExpression = runtimeProvenanceTestExpression("local.repos")
		devcontainer := runtimeResolverTestConfigResource(
			"coder_devcontainer", "repo", "each.value.id", "each.value",
		)
		devcontainer.ForEachExpression = runtimeProvenanceTestExpression(
			"coder_agent.main",
		)
		config := runtimeResolverTestRootConfig(script, devcontainer)
		configIndex, err := newConfigIndex(t.Context(), config)
		require.NoError(t, err)
		require.NoError(t, configIndex.indexRuntimeSourceExpressions(
			t.Context(), workdir,
		))
		graph, err := tfgraph.Parse(t.Context(), `digraph {
    "[root] coder_devcontainer.repo (expand)"
    "[root] coder_agent.main (expand)"
  }`)
		require.NoError(t, err)
		resolver, err := NewResolver(
			t.Context(),
			graph,
			&Program{configIndex: configIndex},
			runtimeResolverTestTargets(
				[]string{
					`coder_agent.main["api"]`,
					`coder_agent.main["worker"]`,
				},
				[]string{
					`coder_devcontainer.repo["api"]`,
					`coder_devcontainer.repo["worker"]`,
				},
			),
		)
		require.NoError(t, err)

		actual, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				`coder_script.work["api"]`, "coder_script", "work",
			),
		)
		require.NoError(t, err)
		require.Equal(t, Target{
			Kind: KindWorkspaceAgent, Address: `coder_agent.main["api"]`,
		}, actual)
	})

	t.Run("IndexedLocalReference", func(t *testing.T) {
		t.Parallel()

		workdir := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(workdir, "main.tf"),
			[]byte(`locals {
  parent = coder_devcontainer.repo["api"].agent_id
}

resource "coder_script" "work" {
  agent_id = local.parent
}

resource "coder_devcontainer" "repo" {
  for_each = coder_agent.main
  agent_id  = each.value.id
}
`),
			0o600,
		))
		devcontainer := runtimeResolverTestConfigResource(
			"coder_devcontainer", "repo", "each.value.id", "each.value",
		)
		devcontainer.ForEachExpression = runtimeProvenanceTestExpression(
			"coder_agent.main",
		)
		config := runtimeResolverTestRootConfig(
			runtimeResolverTestConfigResource(
				"coder_script", "work", "local.parent",
			),
			devcontainer,
		)
		configIndex, err := newConfigIndex(t.Context(), config)
		require.NoError(t, err)
		require.NoError(t, configIndex.indexRuntimeSourceExpressions(
			t.Context(), workdir,
		))
		graph, err := tfgraph.Parse(t.Context(), `digraph {
    "[root] coder_devcontainer.repo (expand)"
    "[root] coder_agent.main (expand)"
  }`)
		require.NoError(t, err)
		resolver, err := NewResolver(
			t.Context(),
			graph,
			&Program{configIndex: configIndex},
			runtimeResolverTestTargets(
				[]string{
					`coder_agent.main["api"]`,
					`coder_agent.main["worker"]`,
				},
				[]string{
					`coder_devcontainer.repo["api"]`,
					`coder_devcontainer.repo["worker"]`,
				},
			),
		)
		require.NoError(t, err)

		actual, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
		)
		require.NoError(t, err)
		require.Equal(t, Target{
			Kind: KindWorkspaceAgent, Address: `coder_agent.main["api"]`,
		}, actual)
	})
}
