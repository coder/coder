package agentruntime

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"
)

func TestRuntimeResolverCorrelatesRepeatedInstances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		rawGraph      string
		config        *tfjson.Config
		resource      *tfjson.StateResource
		agents        []string
		devcontainers []string
		expected      Target
		errorContains string
	}{
		{
			name: "IndexedAgentInstance",
			rawGraph: `digraph {
				"[root] coder_agent.service (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", `coder_agent.service["api"].id`,
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			agents: []string{
				`coder_agent.service["worker"]`,
				`coder_agent.service["api"]`,
			},
			expected: Target{
				Kind: KindWorkspaceAgent, Address: `coder_agent.service["api"]`,
			},
		},
		{
			name: "IndexedDevcontainerInstance",
			rawGraph: `digraph {
				"[root] coder_devcontainer.repo (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work",
					`coder_devcontainer.repo["api"].subagent_id`,
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			devcontainers: []string{
				`coder_devcontainer.repo["api"]`,
				`coder_devcontainer.repo["worker"]`,
			},
			expected: Target{
				Kind: KindDevcontainer, Address: `coder_devcontainer.repo["api"]`,
			},
		},
		{
			name: "IndexedDevcontainerAlternativesAreAmbiguous",
			rawGraph: `digraph {
				"[root] coder_devcontainer.repo (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work",
					`coder_devcontainer.repo["api"].subagent_id`,
					`coder_devcontainer.repo["worker"].subagent_id`,
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			devcontainers: []string{
				`coder_devcontainer.repo["api"]`,
				`coder_devcontainer.repo["worker"]`,
			},
			errorContains: "agent_id may refer to multiple devcontainer subagents",
		},
		{
			name: "IndexedModuleOutput",
			rawGraph: `digraph {
				"[root] module.runtime.output.agent_id (expand)"
				"[root] module.runtime.coder_agent.main (expand)"
				"[root] module.runtime.output.agent_id (expand)" -> "[root] module.runtime.coder_agent.main (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", `module.runtime["api"].agent_id`,
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			agents: []string{
				`module.runtime["worker"].coder_agent.main`,
				`module.runtime["api"].coder_agent.main`,
			},
			expected: Target{
				Kind:    KindWorkspaceAgent,
				Address: `module.runtime["api"].coder_agent.main`,
			},
		},
		{
			name: "IndexedModuleOutputReexportsRootAgent",
			rawGraph: `digraph {
				"[root] module.runtime.output.agent_id (expand)"
				"[root] coder_agent.shared (expand)"
				"[root] module.runtime.output.agent_id (expand)" -> "[root] coder_agent.shared (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", `module.runtime["api"].agent_id`,
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			agents: []string{"coder_agent.shared"},
			expected: Target{
				Kind: KindWorkspaceAgent, Address: "coder_agent.shared",
			},
		},
		{
			name: "ConcreteForEachInstance",
			rawGraph: `digraph {
				"[root] coder_agent.service (expand)"
				"[root] coder_script.work[\"api\"]"
				"[root] coder_agent.service[\"api\"]"
				"[root] coder_script.work[\"api\"]" -> "[root] coder_agent.service[\"api\"]"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.service", "each.key",
				),
			),
			resource: runtimeResolverTestStateResource(
				`coder_script.work["api"]`, "coder_script", "work",
			),
			agents: []string{
				`coder_agent.service["worker"]`,
				`coder_agent.service["api"]`,
			},
			expected: Target{
				Kind: KindWorkspaceAgent, Address: `coder_agent.service["api"]`,
			},
		},
		{
			name: "ConcreteCountInstanceUsesGraphKey",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
				"[root] coder_script.work[0]"
				"[root] coder_agent.main[0]"
				"[root] coder_agent.main[1]"
				"[root] coder_script.work[0]" -> "[root] coder_agent.main[1]"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.main", "count.index",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work[0]", "coder_script", "work",
			),
			agents: []string{
				"coder_agent.main[0]",
				"coder_agent.main[1]",
			},
			expected: Target{
				Kind: KindWorkspaceAgent, Address: "coder_agent.main[1]",
			},
		},
		{
			name: "UnrelatedRepeatedInstanceIsAmbiguous",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
				"[root] coder_script.work[0]"
				"[root] coder_agent.main[0]"
				"[root] coder_agent.main[1]"
				"[root] coder_script.work[0]" -> "[root] coder_agent.main[0]"
				"[root] coder_script.work[0]" -> "[root] coder_agent.main[1]"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.main", "count.index",
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work[0]", "coder_script", "work",
			),
			agents: []string{
				"coder_agent.main[0]",
				"coder_agent.main[1]",
			},
			errorContains: "agent_id may refer to multiple workspace agents",
		},
		{
			name: "ConcreteForEachDevcontainerInstance",
			rawGraph: `digraph {
				"[root] coder_devcontainer.repo (expand)"
				"[root] coder_script.work[\"api\"]"
				"[root] coder_devcontainer.repo[\"api\"]"
				"[root] coder_devcontainer.repo[\"worker\"]"
				"[root] coder_script.work[\"api\"]" -> "[root] coder_devcontainer.repo[\"api\"]"
			}`,
			config: func() *tfjson.Config {
				resource := runtimeResolverTestConfigResource(
					"coder_script", "work", "each.value.subagent_id",
				)
				resource.ForEachExpression = &tfjson.Expression{
					ExpressionData: &tfjson.ExpressionData{
						References: []string{"coder_devcontainer.repo"},
					},
				}
				return runtimeResolverTestRootConfig(resource)
			}(),
			resource: runtimeResolverTestStateResource(
				`coder_script.work["api"]`, "coder_script", "work",
			),
			devcontainers: []string{
				`coder_devcontainer.repo["worker"]`,
				`coder_devcontainer.repo["api"]`,
			},
			expected: Target{
				Kind: KindDevcontainer, Address: `coder_devcontainer.repo["api"]`,
			},
		},
		{
			name: "RepeatedDeclaringModule",
			rawGraph: `digraph {
				"[root] module.service.coder_agent.main (expand)"
			}`,
			config: runtimeResolverTestNestedConfig(
				"service",
				runtimeResolverTestConfigResource(
					"coder_script", "work", "coder_agent.main.id",
				),
			),
			resource: runtimeResolverTestStateResource(
				`module.service["api"].coder_script.work`,
				"coder_script", "work",
			),
			agents: []string{
				`module.service["worker"].coder_agent.main`,
				`module.service["api"].coder_agent.main`,
			},
			expected: Target{
				Kind:    KindWorkspaceAgent,
				Address: `module.service["api"].coder_agent.main`,
			},
		},
		{
			name: "RepeatedModuleInput",
			rawGraph: `digraph {
				"[root] module.feature.var.agent_id (expand)"
				"[root] coder_agent.main (expand)"
				"[root] module.feature[\"api\"].coder_script.work"
				"[root] module.feature[\"api\"].var.agent_id"
				"[root] coder_agent.main[\"api\"]"
				"[root] coder_agent.main[\"worker\"]"
				"[root] module.feature.var.agent_id (expand)" -> "[root] coder_agent.main (expand)"
				"[root] module.feature[\"api\"].coder_script.work" -> "[root] module.feature[\"api\"].var.agent_id"
				"[root] module.feature[\"api\"].var.agent_id" -> "[root] coder_agent.main[\"api\"]"
			}`,
			config: runtimeResolverTestNestedConfig(
				"feature",
				runtimeResolverTestConfigResource(
					"coder_script", "work", "var.agent_id",
				),
			),
			resource: runtimeResolverTestStateResource(
				`module.feature["api"].coder_script.work`,
				"coder_script", "work",
			),
			agents: []string{
				`coder_agent.main["worker"]`,
				`coder_agent.main["api"]`,
			},
			expected: Target{
				Kind: KindWorkspaceAgent, Address: `coder_agent.main["api"]`,
			},
		},
		{
			name: "MixedModuleAndResourceKeys",
			rawGraph: `digraph {
				"[root] module.service.coder_agent.main (expand)"
			}`,
			config: runtimeResolverTestNestedConfig(
				"service",
				runtimeResolverTestConfigResource(
					"coder_script", "work", `coder_agent.main["worker"].id`,
				),
			),
			resource: runtimeResolverTestStateResource(
				`module.service["api"].coder_script.work`,
				"coder_script", "work",
			),
			agents: []string{
				`module.service["api"].coder_agent.main["api"]`,
				`module.service["api"].coder_agent.main["worker"]`,
				`module.service["worker"].coder_agent.main["worker"]`,
			},
			expected: Target{
				Kind:    KindWorkspaceAgent,
				Address: `module.service["api"].coder_agent.main["worker"]`,
			},
		},
		{
			name: "IndexedResourceKeyMismatch",
			rawGraph: `digraph {
				"[root] coder_agent.main (expand)"
			}`,
			config: runtimeResolverTestRootConfig(
				runtimeResolverTestConfigResource(
					"coder_script", "work", `coder_agent.main["missing"].id`,
				),
			),
			resource: runtimeResolverTestStateResource(
				"coder_script.work", "coder_script", "work",
			),
			agents: []string{
				`coder_agent.main["api"]`,
				`coder_agent.main["worker"]`,
			},
			errorContains: "does not resolve to an agent runtime",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolver := runtimeResolverForTest(
				t, test.rawGraph, test.config,
				runtimeResolverTestTargets(test.agents, test.devcontainers),
			)
			actual, err := resolver.ResolveResourceRuntime(t.Context(), test.resource)
			if test.errorContains != "" {
				require.ErrorContains(t, err, test.errorContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}

func runtimeResolverTestNestedConfig(
	moduleName string,
	resources ...*tfjson.ConfigResource,
) *tfjson.Config {
	return &tfjson.Config{RootModule: &tfjson.ConfigModule{
		ModuleCalls: map[string]*tfjson.ModuleCall{
			moduleName: {Module: &tfjson.ConfigModule{Resources: resources}},
		},
	}}
}

func runtimeResolverTestTargets(
	agents []string,
	devcontainers []string,
) []Target {
	targets := make([]Target, 0, len(agents)+len(devcontainers))
	for _, address := range agents {
		targets = append(targets, Target{
			Kind: KindWorkspaceAgent, Address: address,
		})
	}
	for _, address := range devcontainers {
		targets = append(targets, Target{
			Kind: KindDevcontainer, Address: address,
		})
	}
	return targets
}
