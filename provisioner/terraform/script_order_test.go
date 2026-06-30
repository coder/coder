package terraform_test

import (
	"fmt"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func scriptOrderAgentResource(name, id string) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: "coder_agent." + name,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_agent",
		Name:    name,
		AttributeValues: map[string]any{
			"id":   id,
			"arch": "amd64",
			"auth": "token",
			"os":   "linux",
		},
	}
}

// scriptOrderAgentWithStartupScript returns a coder_agent resource that also
// sets the legacy inline startup_script attribute, so it produces an
// orderable "Startup Script" addressable as coder_agent.<name>.
func scriptOrderAgentWithStartupScript(name, id, script string) *tfjson.StateResource {
	agent := scriptOrderAgentResource(name, id)
	agent.AttributeValues["startup_script"] = script
	return agent
}

func scriptOrderScriptResource(address, id, agentID, displayName string, opts ...func(map[string]any)) *tfjson.StateResource {
	parts := strings.Split(address, ".")
	values := map[string]any{
		"id":           id,
		"agent_id":     agentID,
		"display_name": displayName,
		"script":       "#!/bin/sh\necho " + displayName,
		"run_on_start": true,
		"run_on_stop":  false,
	}
	for _, opt := range opts {
		opt(values)
	}
	return &tfjson.StateResource{
		Address:         address,
		Mode:            tfjson.ManagedResourceMode,
		Type:            "coder_script",
		Name:            parts[len(parts)-1],
		AttributeValues: values,
	}
}

func scriptOrderRuleResource(address string, rules ...map[string]any) *tfjson.StateResource {
	parts := strings.Split(address, ".")
	ruleValues := make([]any, 0, len(rules))
	for _, rule := range rules {
		ruleValues = append(ruleValues, rule)
	}
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.DataResourceMode,
		Type:    "coder_script_order",
		Name:    parts[len(parts)-1],
		AttributeValues: map[string]any{
			"id":   "order-" + parts[len(parts)-1],
			"rule": ruleValues,
		},
	}
}

func scriptOrderComputeResource() *tfjson.StateResource {
	return &tfjson.StateResource{
		Address:         "null_resource.about",
		Mode:            tfjson.ManagedResourceMode,
		Type:            "null_resource",
		Name:            "about",
		AttributeValues: map[string]any{"id": "12345"},
	}
}

// scriptOrderGraph builds the minimal terraform graph output that associates
// every named agent with null_resource.about.
func scriptOrderGraph(agentNames ...string) string {
	var b strings.Builder
	_, _ = b.WriteString("digraph {\n\tcompound = \"true\"\n\tnewrank = \"true\"\n\tsubgraph \"root\" {\n")
	_, _ = b.WriteString("\t\t\"[root] null_resource.about (expand)\" [label = \"null_resource.about\", shape = \"box\"]\n")
	for _, name := range agentNames {
		_, _ = fmt.Fprintf(&b, "\t\t\"[root] coder_agent.%s (expand)\" [label = \"coder_agent.%s\", shape = \"box\"]\n", name, name)
		_, _ = fmt.Fprintf(&b, "\t\t\"[root] null_resource.about (expand)\" -> \"[root] coder_agent.%s (expand)\"\n", name)
	}
	_, _ = b.WriteString("\t}\n}\n")
	return b.String()
}

func findConvertedScript(t *testing.T, state *terraform.State, displayName string) *proto.Script {
	t.Helper()
	for _, resource := range state.Resources {
		for _, agent := range resource.Agents {
			for _, script := range agent.Scripts {
				if script.DisplayName == displayName {
					return script
				}
			}
		}
	}
	t.Fatalf("script %q not found in converted state", displayName)
	return nil
}

func TestScriptOrder(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.apt", "id-apt", "agent-main", "apt"),
				scriptOrderScriptResource("coder_script.install", "id-install", "agent-main", "install"),
				scriptOrderScriptResource("coder_script.dotfiles", "id-dotfiles", "agent-main", "dotfiles"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.install"},
					"after": []any{"coder_script.apt"},
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.NoError(t, err)
		require.Empty(t, state.Warnings)

		install := findConvertedScript(t, state, "install")
		require.Equal(t, []*proto.ScriptOrderDependency{
			{ScriptId: "id-apt", Requires: "success"},
		}, install.OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "apt").OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "dotfiles").OrderDependencies)
	})

	t.Run("ModuleSelectorExpands", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.apt", "id-apt", "agent-main", "apt"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":      []any{"module.git"},
					"after":    []any{"coder_script.apt"},
					"requires": "completion",
				}),
			},
			ChildModules: []*tfjson.StateModule{
				{
					Address: "module.git",
					Resources: []*tfjson.StateResource{
						scriptOrderScriptResource("module.git.coder_script.clone", "id-clone", "agent-main", "clone"),
					},
					ChildModules: []*tfjson.StateModule{{
						Address: "module.git.module.nested",
						Resources: []*tfjson.StateResource{
							scriptOrderScriptResource("module.git.module.nested.coder_script.deep", "id-deep", "agent-main", "deep"),
						},
					}},
				},
				{
					// Shares the "module.git" prefix but must not match the
					// module.git selector.
					Address: "module.gitx",
					Resources: []*tfjson.StateResource{
						scriptOrderScriptResource("module.gitx.coder_script.other", "id-other", "agent-main", "other"),
					},
				},
			},
		}}, scriptOrderGraph("main"), logger)
		require.NoError(t, err)

		wantDeps := []*proto.ScriptOrderDependency{
			{ScriptId: "id-apt", Requires: "completion"},
		}
		require.Equal(t, wantDeps, findConvertedScript(t, state, "clone").OrderDependencies)
		require.Equal(t, wantDeps, findConvertedScript(t, state, "deep").OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "other").OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "apt").OrderDependencies)
	})

	t.Run("ModuleScopedSelectors", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				// Same label as the script inside module.m1. The rule in
				// module.m1 must resolve to the module-local script.
				scriptOrderScriptResource("coder_script.a", "id-root-a", "agent-main", "root-a"),
			},
			ChildModules: []*tfjson.StateModule{{
				Address: "module.m1",
				Resources: []*tfjson.StateResource{
					scriptOrderScriptResource("module.m1.coder_script.a", "id-m1-a", "agent-main", "m1-a"),
					scriptOrderScriptResource("module.m1.coder_script.b", "id-m1-b", "agent-main", "m1-b"),
					scriptOrderRuleResource("module.m1.data.coder_script_order.inner", map[string]any{
						"run":   []any{"coder_script.b"},
						"after": []any{"coder_script.a"},
					}),
				},
			}},
		}}, scriptOrderGraph("main"), logger)
		require.NoError(t, err)

		require.Equal(t, []*proto.ScriptOrderDependency{
			{ScriptId: "id-m1-a", Requires: "success"},
		}, findConvertedScript(t, state, "m1-b").OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "root-a").OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "m1-a").OrderDependencies)
	})

	t.Run("OverlappingSelectorsSkipSelfEdge", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"module.git"},
					"after": []any{"module.git.coder_script.clone"},
				}),
			},
			ChildModules: []*tfjson.StateModule{{
				Address: "module.git",
				Resources: []*tfjson.StateResource{
					scriptOrderScriptResource("module.git.coder_script.clone", "id-clone", "agent-main", "clone"),
					scriptOrderScriptResource("module.git.coder_script.deep", "id-deep", "agent-main", "deep"),
				},
			}},
		}}, scriptOrderGraph("main"), logger)
		require.NoError(t, err)

		require.Equal(t, []*proto.ScriptOrderDependency{
			{ScriptId: "id-clone", Requires: "success"},
		}, findConvertedScript(t, state, "deep").OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "clone").OrderDependencies)
	})

	t.Run("MultipleRunSelectors", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.prep", "id-prep", "agent-main", "prep"),
				scriptOrderScriptResource("coder_script.a", "id-a", "agent-main", "a"),
				scriptOrderScriptResource("coder_script.b", "id-b", "agent-main", "b"),
				scriptOrderScriptResource("coder_script.other", "id-other", "agent-main", "other"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.a", "coder_script.b"},
					"after": []any{"coder_script.prep"},
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.NoError(t, err)

		wantDeps := []*proto.ScriptOrderDependency{
			{ScriptId: "id-prep", Requires: "success"},
		}
		require.Equal(t, wantDeps, findConvertedScript(t, state, "a").OrderDependencies)
		require.Equal(t, wantDeps, findConvertedScript(t, state, "b").OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "prep").OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "other").OrderDependencies)
	})

	t.Run("RunMustListAtLeastOneSelector", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.apt", "id-apt", "agent-main", "apt"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{},
					"after": []any{"coder_script.apt"},
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.ErrorContains(t, err, "run must list at least one selector")
	})

	t.Run("LegacyAgentStartupScriptAsAfter", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentWithStartupScript("main", "agent-main", "echo legacy-startup"),
				scriptOrderScriptResource("coder_script.install", "id-install", "agent-main", "install"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.install"},
					"after": []any{"coder_agent.main"},
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.NoError(t, err)

		require.Equal(t, []*proto.ScriptOrderDependency{
			{ScriptId: "coder_agent.main", Requires: "success"},
		}, findConvertedScript(t, state, "install").OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "Startup Script").OrderDependencies)
	})

	t.Run("LegacyAgentStartupScriptAsRun", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentWithStartupScript("main", "agent-main", "echo legacy-startup"),
				scriptOrderScriptResource("coder_script.prep", "id-prep", "agent-main", "prep"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_agent.main"},
					"after": []any{"coder_script.prep"},
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.NoError(t, err)

		legacyStartup := findConvertedScript(t, state, "Startup Script")
		require.Equal(t, []*proto.ScriptOrderDependency{
			{ScriptId: "id-prep", Requires: "success"},
		}, legacyStartup.OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "prep").OrderDependencies)
	})

	t.Run("LegacyAgentStartupScriptModuleScoped", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
			},
			ChildModules: []*tfjson.StateModule{{
				Address: "module.m1",
				Resources: []*tfjson.StateResource{
					func() *tfjson.StateResource {
						agent := scriptOrderAgentWithStartupScript("main", "agent-main", "echo legacy-startup")
						agent.Address = "module.m1.coder_agent.main"
						return agent
					}(),
					scriptOrderScriptResource("module.m1.coder_script.prep", "id-prep", "agent-main", "prep"),
					func() *tfjson.StateResource {
						rule := scriptOrderRuleResource("module.m1.data.coder_script_order.inner", map[string]any{
							"run":   []any{"coder_agent.main"},
							"after": []any{"coder_script.prep"},
						})
						return rule
					}(),
				},
			}},
		}}, `digraph {
	compound = "true"
	newrank = "true"
	subgraph "root" {
		"[root] null_resource.about (expand)" [label = "null_resource.about", shape = "box"]
		"[root] module.m1.coder_agent.main (expand)" [label = "module.m1.coder_agent.main", shape = "box"]
		"[root] null_resource.about (expand)" -> "[root] module.m1.coder_agent.main (expand)"
	}
}
`, logger)
		require.NoError(t, err)

		legacyStartup := findConvertedScript(t, state, "Startup Script")
		require.Equal(t, []*proto.ScriptOrderDependency{
			{ScriptId: "id-prep", Requires: "success"},
		}, legacyStartup.OrderDependencies)
	})

	t.Run("LegacyAgentStartupScriptUnattachedDuringStop", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		// In a stop build, the agent's compute resource is destroyed, so
		// agentResource resolves to nil and the agent (and its legacy
		// startup script) never reaches resourceAgents. The selector must
		// resolve to nothing instead of failing the build.
		stopGraph := `digraph {
	compound = "true"
	newrank = "true"
	subgraph "root" {
		"[root] coder_agent.main (expand)" [label = "coder_agent.main", shape = "box"]
	}
}
`
		_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderAgentWithStartupScript("main", "agent-main", "echo legacy-startup"),
				scriptOrderScriptResource("coder_script.prep", "id-prep", "agent-main", "prep"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_agent.main"},
					"after": []any{"coder_script.prep"},
				}),
			},
		}}, stopGraph, logger)
		require.NoError(t, err)
	})

	t.Run("LegacyAgentSelectorWithoutStartupScript", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				// No startup_script set: coder_agent.main produces no
				// orderable script at all.
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.install", "id-install", "agent-main", "install"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.install"},
					"after": []any{"coder_agent.main"},
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.ErrorContains(t, err, "matches nothing")
	})

	t.Run("UnknownSelector", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.install", "id-install", "agent-main", "install"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.instal"},
					"after": []any{"coder_script.install"},
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.ErrorContains(t, err, `matches nothing`)
		require.ErrorContains(t, err, "coder_script.install")
	})

	t.Run("CrossAgent", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderAgentResource("second", "agent-second"),
				scriptOrderScriptResource("coder_script.apt", "id-apt", "agent-main", "apt"),
				scriptOrderScriptResource("coder_script.install", "id-install", "agent-second", "install"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.install"},
					"after": []any{"coder_script.apt"},
				}),
			},
		}}, scriptOrderGraph("main", "second"), logger)
		require.ErrorContains(t, err, "different agents")
	})

	t.Run("Cycle", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.a", "id-a", "agent-main", "a"),
				scriptOrderScriptResource("coder_script.b", "id-b", "agent-main", "b"),
				scriptOrderRuleResource("data.coder_script_order.boot",
					map[string]any{
						"run":   []any{"coder_script.a"},
						"after": []any{"coder_script.b"},
					},
					map[string]any{
						"run":   []any{"coder_script.b"},
						"after": []any{"coder_script.a"},
					},
				),
			},
		}}, scriptOrderGraph("main"), logger)
		require.ErrorContains(t, err, "cycle")
		require.ErrorContains(t, err, "coder_script.a")
	})

	t.Run("ConflictingRequires", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.apt", "id-apt", "agent-main", "apt"),
				scriptOrderScriptResource("coder_script.install", "id-install", "agent-main", "install"),
				scriptOrderRuleResource("data.coder_script_order.one", map[string]any{
					"run":   []any{"coder_script.install"},
					"after": []any{"coder_script.apt"},
				}),
				scriptOrderRuleResource("data.coder_script_order.two", map[string]any{
					"run":      []any{"coder_script.install"},
					"after":    []any{"coder_script.apt"},
					"requires": "completion",
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.ErrorContains(t, err, "requires")
		require.ErrorContains(t, err, "data.coder_script_order.one")
		require.ErrorContains(t, err, "data.coder_script_order.two")
	})

	t.Run("DependencyMustRunOnStart", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.apt", "id-apt", "agent-main", "apt", func(values map[string]any) {
					values["run_on_start"] = false
				}),
				scriptOrderScriptResource("coder_script.install", "id-install", "agent-main", "install"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.install"},
					"after": []any{"coder_script.apt"},
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.ErrorContains(t, err, "run_on_start")
	})

	t.Run("SelfOrder", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		_, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.apt", "id-apt", "agent-main", "apt"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.apt"},
					"after": []any{"coder_script.apt"},
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.ErrorContains(t, err, "after itself")
	})

	t.Run("SyncMixWarning", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.apt", "id-apt", "agent-main", "apt"),
				scriptOrderScriptResource("coder_script.install", "id-install", "agent-main", "install", func(values map[string]any) {
					values["script"] = "#!/bin/sh\ncoder exp sync want apt\nmake install"
				}),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.install"},
					"after": []any{"coder_script.apt"},
				}),
			},
		}}, scriptOrderGraph("main"), logger)
		require.NoError(t, err)
		require.Len(t, state.Warnings, 1)
		require.Contains(t, state.Warnings[0], "coder exp sync")
		require.Contains(t, state.Warnings[0], "install")
	})

	t.Run("PlanWithoutIDsValidatesOnly", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		// During plan, computed ids are unknown and agent association falls
		// back to graph traversal. Rules are validated but no dependencies
		// are emitted.
		graph := `digraph {
	compound = "true"
	newrank = "true"
	subgraph "root" {
		"[root] null_resource.about (expand)" [label = "null_resource.about", shape = "box"]
		"[root] coder_agent.main (expand)" [label = "coder_agent.main", shape = "box"]
		"[root] coder_script.apt (expand)" [label = "coder_script.apt", shape = "box"]
		"[root] coder_script.install (expand)" [label = "coder_script.install", shape = "box"]
		"[root] null_resource.about (expand)" -> "[root] coder_agent.main (expand)"
		"[root] coder_script.apt (expand)" -> "[root] coder_agent.main (expand)"
		"[root] coder_script.install (expand)" -> "[root] coder_agent.main (expand)"
	}
}
`
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", ""),
				scriptOrderScriptResource("coder_script.apt", "", "", "apt"),
				scriptOrderScriptResource("coder_script.install", "", "", "install"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.install"},
					"after": []any{"coder_script.apt"},
				}),
			},
		}}, graph, logger)
		require.NoError(t, err)
		require.Empty(t, findConvertedScript(t, state, "install").OrderDependencies)
		require.Empty(t, findConvertedScript(t, state, "apt").OrderDependencies)
	})

	t.Run("UnattachedScriptsAreInert", func(t *testing.T) {
		t.Parallel()
		ctx, logger := ctxAndLogger(t)
		// In a plan graph, module-nested scripts have no direct edge to the
		// agent; the dependency routes through the module's var.agent_id
		// node. The script cannot be associated with an agent, so selectors
		// matching it resolve to nothing instead of failing the plan.
		graph := `digraph {
	compound = "true"
	newrank = "true"
	subgraph "root" {
		"[root] coder_agent.main (expand)" [label = "coder_agent.main", shape = "box"]
		"[root] coder_script.prep (expand)" [label = "coder_script.prep", shape = "box"]
		"[root] module.git_clone.coder_script.git_clone (expand)" [label = "module.git_clone.coder_script.git_clone", shape = "box"]
		"[root] null_resource.about (expand)" [label = "null_resource.about", shape = "box"]
		"[root] coder_script.prep (expand)" -> "[root] coder_agent.main (expand)"
		"[root] module.git_clone.coder_script.git_clone (expand)" -> "[root] module.git_clone.var.agent_id (expand)"
		"[root] module.git_clone.var.agent_id (expand)" -> "[root] coder_agent.main (expand)"
		"[root] null_resource.about (expand)" -> "[root] coder_agent.main (expand)"
	}
}
`
		state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderComputeResource(),
				scriptOrderAgentResource("main", ""),
				scriptOrderScriptResource("coder_script.prep", "", "", "prep"),
				scriptOrderScriptResource("module.git_clone.coder_script.git_clone", "", "", "git-clone"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.prep"},
					"after": []any{"module.git_clone"},
				}),
			},
		}}, graph, logger)
		require.NoError(t, err)
		require.Empty(t, findConvertedScript(t, state, "prep").OrderDependencies)

		// In a stop build, the agent's compute resource is destroyed, so
		// the agent is dropped and every script is unattached, even with
		// concrete ids. The rules stay inert; the build must not fail.
		stopGraph := `digraph {
	compound = "true"
	newrank = "true"
	subgraph "root" {
		"[root] coder_agent.main (expand)" [label = "coder_agent.main", shape = "box"]
	}
}
`
		state, err = terraform.ConvertState(ctx, []*tfjson.StateModule{{
			Resources: []*tfjson.StateResource{
				scriptOrderAgentResource("main", "agent-main"),
				scriptOrderScriptResource("coder_script.prep", "id-prep", "agent-main", "prep"),
				scriptOrderScriptResource("module.git_clone.coder_script.git_clone", "id-clone", "agent-main", "git-clone"),
				scriptOrderRuleResource("data.coder_script_order.boot", map[string]any{
					"run":   []any{"coder_script.prep"},
					"after": []any{"module.git_clone"},
				}),
			},
		}}, stopGraph, logger)
		require.NoError(t, err)
		require.Empty(t, state.Warnings)
	})
}

// TestScriptOrder_NestedModulesWithOwnDataSources covers Requirement 2's
// module composition story end to end: a module and a module nested inside
// it each declare their own coder_script_order data source, and a
// root-level data source orders a root script after the outer module as a
// whole. Selectors in each data source must stay scoped to the module that
// declared it, while a module selector at any level still expands into
// every nested module's scripts.
func TestScriptOrder_NestedModulesWithOwnDataSources(t *testing.T) {
	t.Parallel()
	ctx, logger := ctxAndLogger(t)

	state, err := terraform.ConvertState(ctx, []*tfjson.StateModule{{
		Resources: []*tfjson.StateResource{
			scriptOrderComputeResource(),
			scriptOrderAgentResource("main", "agent-main"),
			scriptOrderScriptResource("coder_script.root_prep", "id-root-prep", "agent-main", "root-prep"),
			// Orders the entire outer module, including its nested module,
			// after a root-level script.
			scriptOrderRuleResource("data.coder_script_order.root_order", map[string]any{
				"run":   []any{"module.outer"},
				"after": []any{"coder_script.root_prep"},
			}),
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.outer",
			Resources: []*tfjson.StateResource{
				scriptOrderScriptResource("module.outer.coder_script.outer_a", "id-outer-a", "agent-main", "outer-a"),
				scriptOrderScriptResource("module.outer.coder_script.outer_b", "id-outer-b", "agent-main", "outer-b"),
				// module.outer's own ordering: outer_b after outer_a.
				// Selectors here resolve relative to module.outer, so
				// "coder_script.outer_a" must not be confused with any
				// root-level or sibling-module script of the same local name.
				scriptOrderRuleResource("module.outer.data.coder_script_order.outer_order", map[string]any{
					"run":   []any{"coder_script.outer_b"},
					"after": []any{"coder_script.outer_a"},
				}),
			},
			ChildModules: []*tfjson.StateModule{{
				Address: "module.outer.module.inner",
				Resources: []*tfjson.StateResource{
					scriptOrderScriptResource("module.outer.module.inner.coder_script.inner_x", "id-inner-x", "agent-main", "inner-x"),
					scriptOrderScriptResource("module.outer.module.inner.coder_script.inner_y", "id-inner-y", "agent-main", "inner-y"),
					// module.inner's own ordering: inner_y after inner_x,
					// scoped two levels deep. Must not be affected by, or
					// leak into, module.outer's own data source above.
					scriptOrderRuleResource("module.outer.module.inner.data.coder_script_order.inner_order", map[string]any{
						"run":   []any{"coder_script.inner_y"},
						"after": []any{"coder_script.inner_x"},
					}),
				},
			}},
		}},
	}}, scriptOrderGraph("main"), logger)
	require.NoError(t, err)
	require.Empty(t, state.Warnings)

	// The root rule's "module.outer" selector expands into every script in
	// the outer module's subtree, including the nested inner module, so all
	// four wait on root_prep in addition to their own module-local rules.
	require.ElementsMatch(t, []*proto.ScriptOrderDependency{
		{ScriptId: "id-root-prep", Requires: "success"},
	}, findConvertedScript(t, state, "outer-a").OrderDependencies)
	require.ElementsMatch(t, []*proto.ScriptOrderDependency{
		{ScriptId: "id-outer-a", Requires: "success"},
		{ScriptId: "id-root-prep", Requires: "success"},
	}, findConvertedScript(t, state, "outer-b").OrderDependencies)
	require.ElementsMatch(t, []*proto.ScriptOrderDependency{
		{ScriptId: "id-root-prep", Requires: "success"},
	}, findConvertedScript(t, state, "inner-x").OrderDependencies)
	require.ElementsMatch(t, []*proto.ScriptOrderDependency{
		{ScriptId: "id-inner-x", Requires: "success"},
		{ScriptId: "id-root-prep", Requires: "success"},
	}, findConvertedScript(t, state, "inner-y").OrderDependencies)

	// root_prep is never ordered itself.
	require.Empty(t, findConvertedScript(t, state, "root-prep").OrderDependencies)
}
