package agentruntime

import (
	"os"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestRuntimeResolverCorrelatesNestedAndWrappedInstances(t *testing.T) {
	t.Parallel()

	t.Run("LocalCollections", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			expression string
		}{
			{name: "Direct", expression: "coder_agent.service"},
			{
				name: "Comprehension",
				expression: `{
    for name, agent in coder_agent.service : name => agent.id
  }`,
			},
			{
				name: "Parentheses",
				expression: `({
    for name, agent in coder_agent.service : name => agent.id
  })`,
			},
			{
				name: "ToMap",
				expression: `tomap({
    for name, agent in coder_agent.service : name => agent.id
  })`,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				workdir := t.TempDir()
				source := `resource "coder_agent" "service" {
  for_each = {}
}

locals {
  runtime_ids = ` + test.expression + `
}

resource "coder_script" "work" {
  for_each = local.runtime_ids
  agent_id = each.value.id
}
`
				require.NoError(t, os.WriteFile(
					filepath.Join(workdir, "main.tf"), []byte(source), 0o600,
				))
				script := runtimeResolverTestConfigResource(
					"coder_script", "work", "each.value.id", "each.value",
				)
				script.ForEachExpression = runtimeProvenanceTestExpression(
					"local.runtime_ids",
				)
				config := runtimeResolverTestRootConfig(script)
				configIndex, err := newConfigIndex(t.Context(), config)
				require.NoError(t, err)
				require.NoError(t, configIndex.indexRuntimeSourceExpressions(
					t.Context(), workdir,
				))
				resolver := runtimeResolverForTest(
					t,
					`digraph { "[root] coder_agent.service (expand)" }`,
					config,
					runtimeResolverTestTargets([]string{
						`coder_agent.service["api"]`,
						`coder_agent.service["worker"]`,
					}, nil),
				)
				resolver.configIndex = configIndex

				actual, err := resolver.ResolveResourceRuntime(
					t.Context(),
					runtimeResolverTestStateResource(
						`coder_script.work["api"]`, "coder_script", "work",
					),
				)
				require.NoError(t, err)
				require.Equal(t, Target{
					Kind: KindWorkspaceAgent, Address: `coder_agent.service["api"]`,
				}, actual)
			})
		}
	})

	t.Run("SourceWrappers", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name                 string
			filename             string
			source               string
			reference            string
			correlatedCollection string
		}{
			{
				name:     "Parentheses",
				filename: "locals.tf",
				source: `locals {
  runtime_ids = ({
    for name, runtime in module.runtime : name => runtime.agent_id
  })
}

module "runtime" {
  source   = "./runtime"
  for_each = {}
}
`,
			},
			{
				name:     "ToMap",
				filename: "locals.tf",
				source: `module "runtime" {
  source   = "./runtime"
  for_each = {}
}

locals {
  runtime_ids = tomap({
    for name, runtime in module.runtime : name => runtime.agent_id
  })
}
`,
			},
			{
				name:     "JSON",
				filename: "locals.tf.json",
				source: `{
  "module": {
    "runtime": {
      "source": "./runtime",
      "for_each": {}
    }
  },
  "locals": [{
    "runtime_ids": "${{ for name, runtime in module.runtime : name => runtime.agent_id }}"
  }]
}
`,
			},
			{
				name:     "JSONWrapped",
				filename: "locals.tf.json",
				source: `{
  "module": {
    "runtime": {
      "source": "./runtime",
      "for_each": {}
    }
  },
  "locals": [{
    "runtime_ids": "${tomap(({ for name, runtime in module.runtime : name => runtime.agent_id }))}"
  }]
}
`,
			},
			{
				name:     "ExplicitResource",
				filename: "locals.tf",
				source: `resource "coder_agent" "main" {
  for_each = {}
}

locals {
  runtime_ids = {
    for name, resource in resource.coder_agent.main : name => resource.id
  }
}
`,
				reference:            "coder_agent.main.id",
				correlatedCollection: "coder_agent.main",
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				workdir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(workdir, test.filename),
					[]byte(test.source),
					0o600,
				))
				configIndex, err := newConfigIndex(
					t.Context(), runtimeResolverTestRootConfig(),
				)
				require.NoError(t, err)
				require.NoError(t, configIndex.indexRuntimeSourceExpressions(
					t.Context(), workdir,
				))
				reference := test.reference
				if reference == "" {
					reference = "module.runtime.agent_id"
				}
				correlatedCollection := test.correlatedCollection
				if correlatedCollection == "" {
					correlatedCollection = "module.runtime"
				}
				require.Equal(t, []runtimeLocalReference{{
					reference:            reference,
					correlatedCollection: correlatedCollection,
				}}, configIndex.localRuntime[runtimeLocalKey{
					name: "runtime_ids",
				}])
			})
		}
	})

	t.Run("DoesNotCorrelateNumericKeys", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			source    string
			reference string
		}{
			{
				name: "LocalTuple",
				source: `locals {
  agents = [coder_agent.first, coder_agent.second]
  runtime_ids = {
    for index, agent in local.agents : index => agent.id
  }
}
`,
				reference: "local.agents.id",
			},
			{
				name: "CountModule",
				source: `module "runtime" {
  source = "./runtime"
  count  = 2
}

locals {
  runtime_ids = {
    for index, runtime in module.runtime : index => runtime.agent_id
  }
}
`,
				reference: "module.runtime.agent_id",
			},
			{
				name: "NestedCountResource",
				source: `resource "coder_agent" "runtime" {
  count = 2
}

locals {
  runtime_ids = {
    for key, agent in {
      for index, agent in coder_agent.runtime : index => agent
    } : key => agent.id
  }
}
`,
				reference: "coder_agent.runtime.id",
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				workdir := t.TempDir()
				require.NoError(t, os.WriteFile(
					filepath.Join(workdir, "locals.tf"),
					[]byte(test.source),
					0o600,
				))
				configIndex, err := newConfigIndex(
					t.Context(), runtimeResolverTestRootConfig(),
				)
				require.NoError(t, err)
				require.NoError(t, configIndex.indexRuntimeSourceExpressions(
					t.Context(), workdir,
				))
				require.Equal(t, []runtimeLocalReference{{
					reference: test.reference,
				}}, configIndex.localRuntime[runtimeLocalKey{
					name: "runtime_ids",
				}])
			})
		}
	})

	t.Run("PartiallyInstantiatedNestedModuleInput", func(t *testing.T) {
		t.Parallel()

		config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
			ModuleCalls: map[string]*tfjson.ModuleCall{
				"outer": {
					Expressions: map[string]*tfjson.Expression{
						"agent_id": runtimeProvenanceTestExpression(
							"each.value.subagent_id",
						),
					},
					ForEachExpression: runtimeProvenanceTestExpression(
						"coder_devcontainer.repo",
					),
					Module: &tfjson.ConfigModule{ModuleCalls: map[string]*tfjson.ModuleCall{
						"inner": {
							Expressions: map[string]*tfjson.Expression{
								"agent_id": runtimeProvenanceTestExpression("var.agent_id"),
							},
							Module: &tfjson.ConfigModule{Resources: []*tfjson.ConfigResource{
								runtimeResolverTestConfigResource(
									"coder_script", "install", "var.agent_id",
								),
							}},
						},
					}},
				},
			},
		}}
		resolver := runtimeResolverForTest(
			t,
			`digraph { "[root] coder_devcontainer.repo (expand)" }`,
			config,
			runtimeResolverTestTargets(nil, []string{
				`coder_devcontainer.repo["api"]`,
				`coder_devcontainer.repo["worker"]`,
			}),
		)

		actual, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				`module.outer["api"].module.inner.coder_script.install`,
				"coder_script", "install",
			),
		)
		require.NoError(t, err)
		require.Equal(t, Target{
			Kind: KindDevcontainer, Address: `coder_devcontainer.repo["api"]`,
		}, actual)
	})
}
