package agentruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

func TestRuntimeResolverIndirectReferences(t *testing.T) {
	t.Parallel()

	t.Run("Local", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t,
			`digraph { "[root] coder_agent.main (expand)" }`,
			runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "local.runtime_id",
				),
			),
			runtimeResolverTestTargets([]string{"coder_agent.main"}, nil),
		)
		resolver.configIndex.localRuntime[runtimeLocalKey{
			name: "runtime_id",
		}] = []runtimeLocalReference{{
			reference: "coder_agent.main.id",
		}}

		actual, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
		)
		require.NoError(t, err)
		require.Equal(t, Target{
			Kind: KindWorkspaceAgent, Address: "coder_agent.main",
		}, actual)
	})

	t.Run("ModuleInput", func(t *testing.T) {
		t.Parallel()

		config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
			ModuleCalls: map[string]*tfjson.ModuleCall{
				"feature": {
					Expressions: map[string]*tfjson.Expression{
						"runtime_id": runtimeProvenanceTestExpression(
							"coder_agent.main.id",
						),
					},
					Module: &tfjson.ConfigModule{},
				},
			},
		}}
		resolver := runtimeResolverForTest(t, "digraph {}", config, nil)
		modulePath, err := tfaddr.ParseModulePath("module.feature")
		require.NoError(t, err)

		actual, err := resolver.resolveConfiguredRuntimeReferences(
			t.Context(),
			modulePath,
			[]runtimeConfiguredReference{{reference: "var.runtime_id"}},
			newProvenanceBudget(
				defaultProvenanceLimits(),
			),
		)
		require.NoError(t, err)
		require.Equal(t, []runtimeResolvedReference{{
			reference: "coder_agent.main.id",
		}}, actual)
	})

	t.Run("ChainedModuleOutput", func(t *testing.T) {
		t.Parallel()

		config := &tfjson.Config{RootModule: &tfjson.ConfigModule{
			ModuleCalls: map[string]*tfjson.ModuleCall{
				"runtime": {Module: &tfjson.ConfigModule{
					Outputs: map[string]*tfjson.ConfigOutput{
						"runtime_id": {
							Expression: runtimeProvenanceTestExpression(
								"local.runtime_id",
							),
						},
					},
				}},
			},
		}}
		resolver := runtimeResolverForTest(t, "digraph {}", config, nil)
		resolver.configIndex.localRuntime[runtimeLocalKey{
			moduleAddress: "module.runtime",
			name:          "runtime_id",
		}] = []runtimeLocalReference{{
			reference: "coder_agent.main.id",
		}}

		actual, err := resolver.runtimeReferenceTarget(
			t.Context(), tfaddr.ModulePath{}, "module.runtime.runtime_id",
		)
		require.NoError(t, err)
		require.Equal(t, runtimeReferenceTargetWorkspaceAgent, actual)
	})

	t.Run("Cycle", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t, "digraph {}", runtimeResolverTestRootConfig(), nil,
		)
		resolver.configIndex.localRuntime[runtimeLocalKey{
			name: "first",
		}] = []runtimeLocalReference{{reference: "local.second"}}
		resolver.configIndex.localRuntime[runtimeLocalKey{
			name: "second",
		}] = []runtimeLocalReference{{reference: "local.first"}}

		actual, err := resolver.resolveConfiguredRuntimeReferences(
			t.Context(),
			tfaddr.ModulePath{},
			[]runtimeConfiguredReference{{reference: "local.first"}},
			newProvenanceBudget(
				defaultProvenanceLimits(),
			),
		)
		require.NoError(t, err)
		require.Equal(t, []runtimeResolvedReference{{
			reference: "local.first",
		}}, actual)
	})

	t.Run("RejectsAggregateSelection", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t, "digraph {}", runtimeResolverTestRootConfig(), nil,
		)
		_, err := resolver.resolveConfiguredRuntimeReferences(
			t.Context(),
			tfaddr.ModulePath{},
			[]runtimeConfiguredReference{{
				reference: "local.runtimes.primary",
			}},
			newProvenanceBudget(
				defaultProvenanceLimits(),
			),
		)
		require.ErrorContains(t, err, "aggregate indirection is unsupported")
	})

	t.Run("RejectsTransformedLocal", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t, "digraph {}", runtimeResolverTestRootConfig(), nil,
		)
		resolver.configIndex.runtimeSourceIndexed = true
		resolver.configIndex.localRuntime[runtimeLocalKey{
			name: "runtime_id",
		}] = []runtimeLocalReference{{
			reference: "coder_agent.main.id",
		}}
		_, err := resolver.resolveConfiguredRuntimeReferences(
			t.Context(),
			tfaddr.ModulePath{},
			[]runtimeConfiguredReference{{reference: "local.runtime_id"}},
			newProvenanceBudget(
				defaultProvenanceLimits(),
			),
		)
		require.ErrorContains(
			t, err,
			`local value "runtime_id" does not preserve agent runtime identity`,
		)
	})

	t.Run("BoundsDepth", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t, "digraph {}", runtimeResolverTestRootConfig(), nil,
		)
		for level := range 4 {
			reference := "coder_agent.main.id"
			if level > 0 {
				reference = fmt.Sprintf("local.value_%d", level-1)
			}
			resolver.configIndex.localRuntime[runtimeLocalKey{
				name: fmt.Sprintf("value_%d", level),
			}] = []runtimeLocalReference{{reference: reference}}
		}
		limits := defaultProvenanceLimits()
		limits.referenceDepth = 3
		_, err := resolver.resolveConfiguredRuntimeReferences(
			t.Context(),
			tfaddr.ModulePath{},
			[]runtimeConfiguredReference{{reference: "local.value_3"}},
			newProvenanceBudget(limits),
		)
		require.ErrorContains(t, err, "limit of 3 indirect Terraform references")
	})

	t.Run("BoundsReferences", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t, "digraph {}", runtimeResolverTestRootConfig(), nil,
		)
		references := make([]runtimeLocalReference, 0, 6)
		for index := range 6 {
			references = append(references, runtimeLocalReference{
				reference: fmt.Sprintf("coder_agent.agent_%d.id", index),
			})
		}
		resolver.configIndex.localRuntime[runtimeLocalKey{
			name: "agents",
		}] = references
		limits := defaultProvenanceLimits()
		limits.referenceCount = 5
		budget := newProvenanceBudget(limits)
		_, err := resolver.resolveConfiguredRuntimeReferences(
			t.Context(),
			tfaddr.ModulePath{},
			[]runtimeConfiguredReference{{reference: "local.agents"}},
			budget,
		)
		require.ErrorContains(t, err, "limit of 5 Terraform references")
		require.Equal(t, 5, budget.referenceCount)
	})

	t.Run("BoundsReferenceBytes", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t, "digraph {}", runtimeResolverTestRootConfig(), nil,
		)
		const (
			localReference = "local.runtime"
			agentReference = "coder_agent.an_intentionally_long_name.id"
		)
		resolver.configIndex.localRuntime[runtimeLocalKey{
			name: "runtime",
		}] = []runtimeLocalReference{{reference: agentReference}}
		limits := defaultProvenanceLimits()
		limits.referenceBytes = len(localReference) + len(agentReference) - 1
		_, err := resolver.resolveConfiguredRuntimeReferences(
			t.Context(),
			tfaddr.ModulePath{},
			[]runtimeConfiguredReference{{reference: localReference}},
			newProvenanceBudget(limits),
		)
		require.ErrorContains(
			t, err,
			fmt.Sprintf("limit of %d Terraform reference bytes", limits.referenceBytes),
		)
	})

	t.Run("DeduplicatesSharedChains", func(t *testing.T) {
		t.Parallel()

		resolver := runtimeResolverForTest(
			t, "digraph {}", runtimeResolverTestRootConfig(), nil,
		)
		const levels = 12
		for level := range levels {
			left := fmt.Sprintf("left_%d", level)
			right := fmt.Sprintf("right_%d", level)
			references := []runtimeLocalReference{{
				reference: "coder_agent.main.id",
			}}
			if level > 0 {
				references = []runtimeLocalReference{
					{reference: fmt.Sprintf("local.left_%d", level-1)},
					{reference: fmt.Sprintf("local.right_%d", level-1)},
				}
			}
			resolver.configIndex.localRuntime[runtimeLocalKey{
				name: left,
			}] = references
			resolver.configIndex.localRuntime[runtimeLocalKey{
				name: right,
			}] = references
		}
		limits := defaultProvenanceLimits()
		limits.referenceCount = 100
		budget := newProvenanceBudget(limits)
		actual, err := resolver.resolveConfiguredRuntimeReferences(
			t.Context(),
			tfaddr.ModulePath{},
			[]runtimeConfiguredReference{{
				reference: fmt.Sprintf("local.left_%d", levels-1),
			}},
			budget,
		)
		require.NoError(t, err)
		require.Equal(t, []runtimeResolvedReference{{
			reference: "coder_agent.main.id",
		}}, actual)
		require.Less(t, budget.referenceCount, limits.referenceCount)
	})

	t.Run("UsesIndexedSourceProvenance", func(t *testing.T) {
		t.Parallel()

		workdir := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(workdir, "main.tf"),
			[]byte(`locals {
  runtime_id = coder_agent.main.id
}

resource "coder_script" "work" {
  agent_id = local.runtime_id
}
`),
			0o600,
		))
		config := runtimeResolverTestRootConfig(
			runtimeResolverTestConfigResource(
				"coder_script", "work", "local.runtime_id",
			),
		)
		configIndex, err := newConfigIndex(t.Context(), config)
		require.NoError(t, err)
		require.NoError(t, configIndex.indexRuntimeSourceExpressions(
			t.Context(), workdir,
		))
		resolver := runtimeResolverForTest(
			t,
			`digraph { "[root] coder_agent.main (expand)" }`,
			config,
			runtimeResolverTestTargets([]string{"coder_agent.main"}, nil),
		)
		resolver.configIndex = configIndex

		actual, err := resolver.ResolveResourceRuntime(
			t.Context(),
			runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
		)
		require.NoError(t, err)
		require.Equal(t, Target{
			Kind: KindWorkspaceAgent, Address: "coder_agent.main",
		}, actual)
	})
}

func runtimeProvenanceTestExpression(
	references ...string,
) *tfjson.Expression {
	return &tfjson.Expression{
		ExpressionData: &tfjson.ExpressionData{References: references},
	}
}
