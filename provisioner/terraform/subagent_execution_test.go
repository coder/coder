package terraform_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/provisionersdk/proto"
)

func subagentTestResource(address, resourceType, name string, attributes map[string]any) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address:         address,
		Mode:            tfjson.ManagedResourceMode,
		Type:            resourceType,
		Name:            name,
		AttributeValues: attributes,
	}
}

func subagentTestAgentResource(address, name, id string) *tfjson.StateResource {
	return subagentTestResource(address, "coder_agent", name, map[string]any{
		"arch":  "amd64",
		"auth":  "token",
		"id":    id,
		"os":    "linux",
		"token": "",
	})
}

func subagentTestExecutionResource(address, name, agentID, subagentID string) *tfjson.StateResource {
	addressParts := strings.Split(address, ".")
	resourceName, _, _ := strings.Cut(addressParts[len(addressParts)-1], "[")
	return subagentTestResource(address, "coder_subagent_execution", resourceName, map[string]any{
		"agent_id":          agentID,
		"driver":            "driver configuration",
		"driver_protocol":   1,
		"id":                "execution-id-" + name,
		"name":              name,
		"restart_policy":    "on-failure",
		"shared_child_path": "/workspace/project",
		"shared_host_path":  "/home/coder/project",
		"startup_timeout":   120,
		"subagent_id":       subagentID,
	})
}

func subagentTestAppResource(address, name, agentID, slug string) *tfjson.StateResource {
	return subagentTestResource(address, "coder_app", name, map[string]any{
		"agent_id":     agentID,
		"display_name": slug,
		"id":           "app-id-" + slug,
		"slug":         slug,
	})
}

func subagentTestScriptResource(address, name, agentID string) *tfjson.StateResource {
	return subagentTestResource(address, "coder_script", name, map[string]any{
		"agent_id":     agentID,
		"display_name": name,
		"run_on_start": true,
		"script":       "echo ready",
	})
}

func subagentTestEnvResource(address, name, agentID string) *tfjson.StateResource {
	return subagentTestResource(address, "coder_env", name, map[string]any{
		"agent_id": agentID,
		"name":     strings.ToUpper(strings.ReplaceAll(name, "-", "_")),
		"value":    name,
	})
}

func subagentTestDevcontainerResource(address, name, agentID, subagentID string) *tfjson.StateResource {
	return subagentTestResource(address, "coder_devcontainer", name, map[string]any{
		"agent_id":         agentID,
		"config_path":      "/workspace/.devcontainer/devcontainer.json",
		"id":               "devcontainer-id-" + name,
		"subagent_id":      subagentID,
		"workspace_folder": "/workspace",
	})
}

func subagentTestGraph(nodes []string, edges ...[2]string) string {
	var builder strings.Builder
	_, _ = builder.WriteString("digraph {\n")
	for _, label := range nodes {
		nodeName := fmt.Sprintf("[root] %s (expand)", label)
		_, _ = fmt.Fprintf(&builder, "\t%q [label = %q, shape = \"box\"]\n", nodeName, label)
	}
	for _, edge := range edges {
		source := fmt.Sprintf("[root] %s (expand)", edge[0])
		destination := fmt.Sprintf("[root] %s (expand)", edge[1])
		_, _ = fmt.Fprintf(&builder, "\t%q -> %q\n", source, destination)
	}
	_, _ = builder.WriteString("}\n")
	return builder.String()
}

func subagentTestConvert(t *testing.T, module *tfjson.StateModule, graph string) (*terraform.State, error) {
	t.Helper()
	ctx, logger := ctxAndLogger(t)
	return terraform.ConvertState(ctx, []*tfjson.StateModule{module}, graph, logger)
}

func subagentTestFindAgent(t *testing.T, state *terraform.State, name string) *proto.Agent {
	t.Helper()
	for _, resource := range state.Resources {
		for _, agent := range resource.Agents {
			if agent.Name == name {
				return agent
			}
		}
	}
	require.FailNow(t, "agent not found", name)
	return nil
}

func subagentTestBaseModule(agentID string, extraResources ...*tfjson.StateResource) *tfjson.StateModule {
	resources := []*tfjson.StateResource{
		subagentTestResource("null_resource.workspace", "null_resource", "workspace", map[string]any{}),
		subagentTestAgentResource("coder_agent.parent", "parent", agentID),
	}
	resources = append(resources, extraResources...)
	return &tfjson.StateModule{Resources: resources}
}

func subagentTestBaseGraph(nodes []string, edges ...[2]string) string {
	nodes = append([]string{"null_resource.workspace", "coder_agent.parent"}, nodes...)
	edges = append([][2]string{{"null_resource.workspace", "coder_agent.parent"}}, edges...)
	return subagentTestGraph(nodes, edges...)
}

func TestConvertStateSubagentExecutionFullDecodeAndRetention(t *testing.T) {
	t.Parallel()

	executionResource := subagentTestExecutionResource("coder_subagent_execution.child", "child", "parent-id", "terraform-association-id")
	executionResource.AttributeValues["driver"] = "#!/bin/sh\necho driver"
	executionResource.AttributeValues["driver_protocol"] = 7
	executionResource.AttributeValues["id"] = "terraform-declaration-id"
	executionResource.AttributeValues["restart_policy"] = "never"
	executionResource.AttributeValues["shared_child_path"] = "/child/project"
	executionResource.AttributeValues["shared_host_path"] = "/host/project"
	executionResource.AttributeValues["startup_timeout"] = 321

	state, err := subagentTestConvert(t, subagentTestBaseModule("parent-id", executionResource), subagentTestBaseGraph(
		[]string{"coder_subagent_execution.child"},
	))
	require.NoError(t, err)

	agent := subagentTestFindAgent(t, state, "parent")
	require.Len(t, agent.SubagentExecutions, 1)
	require.Equal(t, &proto.SubagentExecution{
		Id:                    "terraform-declaration-id",
		SubagentId:            "terraform-association-id",
		Name:                  "child",
		Driver:                "#!/bin/sh\necho driver",
		DriverProtocol:        7,
		SharedHostPath:        "/host/project",
		SharedChildPath:       "/child/project",
		StartupTimeoutSeconds: 321,
		RestartPolicy:         "never",
	}, agent.SubagentExecutions[0])
	require.False(t, slices.ContainsFunc(state.Resources, func(resource *proto.Resource) bool {
		return resource.Type == "coder_subagent_execution"
	}))
}

func TestConvertStateSubagentExecutionPlanChildren(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
		subagentTestAppResource("coder_app.child", "child", "", "child-app"),
		subagentTestScriptResource("coder_script.child", "child-script", ""),
		subagentTestEnvResource("coder_env.child", "child-env", ""),
	)
	graph := subagentTestBaseGraph(
		[]string{
			"coder_subagent_execution.child",
			"coder_app.child",
			"coder_script.child",
			"coder_env.child",
			"local.execution-association",
		},
		[2]string{"coder_subagent_execution.child", "coder_agent.parent"},
		[2]string{"coder_app.child", "local.execution-association"},
		[2]string{"local.execution-association", "coder_subagent_execution.child"},
		[2]string{"coder_script.child", "coder_subagent_execution.child"},
		[2]string{"coder_env.child", "coder_subagent_execution.child"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.NoError(t, err)
	execution := subagentTestFindAgent(t, state, "parent").SubagentExecutions[0]
	require.Equal(t, []string{"child-app"}, []string{execution.Apps[0].Slug})
	require.Equal(t, []string{"child-script"}, []string{execution.Scripts[0].DisplayName})
	require.Equal(t, []string{"CHILD_ENV"}, []string{execution.Envs[0].Name})
}

func TestConvertStateSubagentExecutionPlanChildPrefersDirectAgent(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
		subagentTestAppResource("coder_app.direct", "direct", "", "direct-app"),
	)
	graph := subagentTestBaseGraph(
		[]string{
			"coder_subagent_execution.child",
			"coder_app.direct",
			"local.unrelated_execution",
		},
		[2]string{"coder_subagent_execution.child", "coder_agent.parent"},
		[2]string{"coder_app.direct", "coder_agent.parent"},
		[2]string{"coder_app.direct", "local.unrelated_execution"},
		[2]string{"local.unrelated_execution", "coder_subagent_execution.child"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.NoError(t, err)
	agent := subagentTestFindAgent(t, state, "parent")
	require.Equal(t, []string{"direct-app"}, []string{agent.Apps[0].Slug})
	require.Empty(t, agent.SubagentExecutions[0].Apps)
}

func TestConvertStateSubagentExecutionPlanChildPrefersDirectDevcontainer(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestDevcontainerResource("coder_devcontainer.dev", "dev", "", ""),
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
		subagentTestAppResource("coder_app.direct", "direct", "", "direct-app"),
	)
	graph := subagentTestBaseGraph(
		[]string{
			"coder_devcontainer.dev",
			"coder_subagent_execution.child",
			"coder_app.direct",
			"local.unrelated_execution",
		},
		[2]string{"coder_devcontainer.dev", "coder_agent.parent"},
		[2]string{"coder_subagent_execution.child", "coder_agent.parent"},
		[2]string{"coder_app.direct", "coder_devcontainer.dev"},
		[2]string{"coder_app.direct", "local.unrelated_execution"},
		[2]string{"local.unrelated_execution", "coder_subagent_execution.child"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.NoError(t, err)
	agent := subagentTestFindAgent(t, state, "parent")
	require.Empty(t, agent.Apps)
	require.Equal(t, []string{"direct-app"}, []string{agent.Devcontainers[0].Apps[0].Slug})
	require.Empty(t, agent.SubagentExecutions[0].Apps)
}

func TestConvertStateSubagentExecutionAppliedChildrenUseAssociationID(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("parent-id",
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "parent-id", "association-id"),
		subagentTestAppResource("coder_app.child", "child", "association-id", "child-app"),
		subagentTestScriptResource("coder_script.child", "child-script", "association-id"),
		subagentTestEnvResource("coder_env.child", "child-env", "association-id"),
	)
	graph := subagentTestBaseGraph([]string{
		"coder_subagent_execution.child",
		"coder_app.child",
		"coder_script.child",
		"coder_env.child",
	})

	state, err := subagentTestConvert(t, module, graph)
	require.NoError(t, err)
	execution := subagentTestFindAgent(t, state, "parent").SubagentExecutions[0]
	require.Len(t, execution.Apps, 1)
	require.Len(t, execution.Scripts, 1)
	require.Len(t, execution.Envs, 1)
}

func TestConvertStateSubagentExecutionPlanAndAppliedAttachmentEquivalent(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name                string
		parentAgentID       string
		executionParentID   string
		executionSubagentID string
		appAgentID          string
		edges               [][2]string
	}{
		{
			name: "plan",
			edges: [][2]string{
				{"coder_subagent_execution.child", "coder_agent.parent"},
				{"coder_app.child", "coder_subagent_execution.child"},
			},
		},
		{
			name:                "applied",
			parentAgentID:       "parent-id",
			executionParentID:   "parent-id",
			executionSubagentID: "association-id",
			appAgentID:          "association-id",
			edges: [][2]string{
				{"coder_app.child", "coder_agent.parent"},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			module := subagentTestBaseModule(testCase.parentAgentID,
				subagentTestExecutionResource(
					"coder_subagent_execution.child",
					"child",
					testCase.executionParentID,
					testCase.executionSubagentID,
				),
				subagentTestAppResource("coder_app.child", "child", testCase.appAgentID, "child-app"),
			)
			graph := subagentTestBaseGraph(
				[]string{"coder_subagent_execution.child", "coder_app.child"},
				testCase.edges...,
			)

			state, err := subagentTestConvert(t, module, graph)
			require.NoError(t, err)
			agent := subagentTestFindAgent(t, state, "parent")
			require.Empty(t, agent.Apps)
			require.Equal(t, []string{"child-app"}, []string{agent.SubagentExecutions[0].Apps[0].Slug})
		})
	}
}

func TestConvertStateSubagentExecutionsDoNotCrossAttach(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestExecutionResource("coder_subagent_execution.one", "one", "", ""),
		subagentTestExecutionResource("coder_subagent_execution.two", "two", "", ""),
		subagentTestAppResource("coder_app.one", "one", "", "one-app"),
		subagentTestAppResource("coder_app.two", "two", "", "two-app"),
		subagentTestScriptResource("coder_script.one", "one-script", ""),
		subagentTestEnvResource("coder_env.two", "two-env", ""),
	)
	graph := subagentTestBaseGraph(
		[]string{
			"coder_subagent_execution.one",
			"coder_subagent_execution.two",
			"coder_app.one",
			"coder_app.two",
			"coder_script.one",
			"coder_env.two",
		},
		[2]string{"coder_subagent_execution.one", "coder_agent.parent"},
		[2]string{"coder_subagent_execution.two", "coder_agent.parent"},
		[2]string{"coder_app.one", "coder_subagent_execution.one"},
		[2]string{"coder_app.two", "coder_subagent_execution.two"},
		[2]string{"coder_script.one", "coder_subagent_execution.one"},
		[2]string{"coder_env.two", "coder_subagent_execution.two"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.NoError(t, err)
	executions := subagentTestFindAgent(t, state, "parent").SubagentExecutions
	require.Len(t, executions, 2)
	require.Equal(t, "one", executions[0].Name)
	require.Equal(t, []string{"one-app"}, []string{executions[0].Apps[0].Slug})
	require.Len(t, executions[0].Scripts, 1)
	require.Empty(t, executions[0].Envs)
	require.Equal(t, "two", executions[1].Name)
	require.Equal(t, []string{"two-app"}, []string{executions[1].Apps[0].Slug})
	require.Empty(t, executions[1].Scripts)
	require.Len(t, executions[1].Envs, 1)
}

func TestConvertStateSubagentExecutionModuleLocalResources(t *testing.T) {
	t.Parallel()

	rootModule := subagentTestBaseModule("")
	rootModule.ChildModules = []*tfjson.StateModule{{
		Address: `module.child["one"]`,
		Resources: []*tfjson.StateResource{
			subagentTestExecutionResource(`module.child["one"].coder_subagent_execution.child`, "module-child", "", ""),
			subagentTestAppResource(`module.child["one"].coder_app.child`, "child", "", "module-app"),
			subagentTestScriptResource(`module.child["one"].coder_script.child`, "module-script", ""),
			subagentTestEnvResource(`module.child["one"].coder_env.child`, "module-env", ""),
		},
	}}
	graph := subagentTestBaseGraph(
		[]string{
			"module.child.coder_subagent_execution.child",
			"module.child.coder_app.child",
			"module.child.coder_script.child",
			"module.child.coder_env.child",
			"module.child.var.parent_agent_id",
			"module.child.local.subagent_id",
		},
		[2]string{"module.child.coder_subagent_execution.child", "module.child.var.parent_agent_id"},
		[2]string{"module.child.var.parent_agent_id", "coder_agent.parent"},
		[2]string{"module.child.coder_app.child", "module.child.local.subagent_id"},
		[2]string{"module.child.coder_script.child", "module.child.local.subagent_id"},
		[2]string{"module.child.coder_env.child", "module.child.local.subagent_id"},
		[2]string{"module.child.local.subagent_id", "module.child.coder_subagent_execution.child"},
	)

	state, err := subagentTestConvert(t, rootModule, graph)
	require.NoError(t, err)
	execution := subagentTestFindAgent(t, state, "parent").SubagentExecutions[0]
	require.Equal(t, "module-child", execution.Name)
	require.Len(t, execution.Apps, 1)
	require.Len(t, execution.Scripts, 1)
	require.Len(t, execution.Envs, 1)
}

func TestConvertStateSubagentExecutionIndexedModule(t *testing.T) {
	t.Parallel()

	moduleAddress := `module.child["one"]`
	rootModule := &tfjson.StateModule{ChildModules: []*tfjson.StateModule{{
		Address: moduleAddress,
		Resources: []*tfjson.StateResource{
			subagentTestResource(moduleAddress+".null_resource.workspace", "null_resource", "workspace", map[string]any{}),
			subagentTestAgentResource(moduleAddress+".coder_agent.parent", "parent", ""),
			subagentTestExecutionResource(moduleAddress+".coder_subagent_execution.child", "child", "", ""),
			subagentTestAppResource(moduleAddress+".coder_app.child", "child", "", "module-app"),
			subagentTestScriptResource(moduleAddress+".coder_script.child", "module-script", ""),
			subagentTestEnvResource(moduleAddress+".coder_env.child", "module-env", ""),
		},
	}}}
	graph := subagentTestGraph(
		[]string{
			"module.child.null_resource.workspace",
			"module.child.coder_agent.parent",
			"module.child.coder_subagent_execution.child",
			"module.child.coder_app.child",
			"module.child.coder_script.child",
			"module.child.coder_env.child",
		},
		[2]string{"module.child.null_resource.workspace", "module.child.coder_agent.parent"},
		[2]string{"module.child.coder_subagent_execution.child", "module.child.coder_agent.parent"},
		[2]string{"module.child.coder_app.child", "module.child.coder_subagent_execution.child"},
		[2]string{"module.child.coder_script.child", "module.child.coder_subagent_execution.child"},
		[2]string{"module.child.coder_env.child", "module.child.coder_subagent_execution.child"},
	)

	state, err := subagentTestConvert(t, rootModule, graph)
	require.NoError(t, err)
	agent := subagentTestFindAgent(t, state, "parent")
	require.Len(t, agent.SubagentExecutions, 1)
	require.Len(t, agent.SubagentExecutions[0].Apps, 1)
	require.Len(t, agent.SubagentExecutions[0].Scripts, 1)
	require.Len(t, agent.SubagentExecutions[0].Envs, 1)
}

func TestConvertStateSubagentExecutionCountedAndMappedChildren(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
		subagentTestAppResource("coder_app.child[0]", "child", "", "child-app-zero"),
		subagentTestAppResource("coder_app.child[1]", "child", "", "child-app-one"),
		subagentTestScriptResource(`coder_script.child["first"]`, "first-script", ""),
		subagentTestScriptResource(`coder_script.child["second"]`, "second-script", ""),
		subagentTestEnvResource(`coder_env.child["first"]`, "first-env", ""),
		subagentTestEnvResource(`coder_env.child["second"]`, "second-env", ""),
	)
	graph := subagentTestBaseGraph(
		[]string{
			"coder_subagent_execution.child",
			"coder_app.child",
			"coder_script.child",
			"coder_env.child",
		},
		[2]string{"coder_subagent_execution.child", "coder_agent.parent"},
		[2]string{"coder_app.child", "coder_subagent_execution.child"},
		[2]string{"coder_script.child", "coder_subagent_execution.child"},
		[2]string{"coder_env.child", "coder_subagent_execution.child"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.NoError(t, err)
	execution := subagentTestFindAgent(t, state, "parent").SubagentExecutions[0]
	require.Len(t, execution.Apps, 2)
	require.Len(t, execution.Scripts, 2)
	require.Len(t, execution.Envs, 2)
}

func TestConvertStateSubagentExecutionParentPrefersCloserAgent(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
		subagentTestExecutionResource("coder_subagent_execution.unrelated", "unrelated", "", ""),
	)
	graph := subagentTestBaseGraph(
		[]string{
			"coder_subagent_execution.child",
			"coder_subagent_execution.unrelated",
			"local.unrelated_execution",
		},
		[2]string{"coder_subagent_execution.child", "coder_agent.parent"},
		[2]string{"coder_subagent_execution.child", "local.unrelated_execution"},
		[2]string{"local.unrelated_execution", "coder_subagent_execution.unrelated"},
		[2]string{"coder_subagent_execution.unrelated", "coder_agent.parent"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.NoError(t, err)
	require.Len(t, subagentTestFindAgent(t, state, "parent").SubagentExecutions, 2)
}

func TestConvertStateSubagentExecutionParentRejectsCrossKindAmbiguity(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
		subagentTestExecutionResource("coder_subagent_execution.unrelated", "unrelated", "", ""),
	)
	graph := subagentTestBaseGraph(
		[]string{"coder_subagent_execution.child", "coder_subagent_execution.unrelated"},
		[2]string{"coder_subagent_execution.child", "coder_agent.parent"},
		[2]string{"coder_subagent_execution.child", "coder_subagent_execution.unrelated"},
		[2]string{"coder_subagent_execution.unrelated", "coder_agent.parent"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.Nil(t, state)
	require.ErrorContains(t, err, "parent has multiple matches")
	require.ErrorContains(t, err, `coder_agent "coder_agent.parent"`)
	require.ErrorContains(t, err, `coder_subagent_execution "coder_subagent_execution.unrelated"`)
}

func TestConvertStateSubagentExecutionCrossKindAmbiguity(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
		subagentTestAppResource("coder_app.ambiguous", "ambiguous", "", "ambiguous-app"),
	)
	graph := subagentTestBaseGraph(
		[]string{"coder_subagent_execution.child", "coder_app.ambiguous"},
		[2]string{"coder_subagent_execution.child", "coder_agent.parent"},
		[2]string{"coder_app.ambiguous", "coder_agent.parent"},
		[2]string{"coder_app.ambiguous", "coder_subagent_execution.child"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.Nil(t, state)
	require.ErrorContains(t, err, "ambiguous Terraform association with multiple matches")
	require.ErrorContains(t, err, `coder_agent "coder_agent.parent"`)
	require.ErrorContains(t, err, `coder_subagent_execution "coder_subagent_execution.child"`)
}

func TestConvertStateSubagentExecutionAmbiguousInstances(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestExecutionResource("coder_subagent_execution.child[0]", "child-zero", "", ""),
		subagentTestExecutionResource("coder_subagent_execution.child[1]", "child-one", "", ""),
		subagentTestAppResource("coder_app.child", "child", "", "child-app"),
	)
	graph := subagentTestBaseGraph(
		[]string{"coder_subagent_execution.child", "coder_app.child"},
		[2]string{"coder_subagent_execution.child", "coder_agent.parent"},
		[2]string{"coder_app.child", "coder_subagent_execution.child"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.Nil(t, state)
	require.ErrorContains(t, err, "ambiguous coder_subagent_execution association with multiple matches")
}

func TestConvertStateSubagentExecutionUnresolvedParent(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
	)
	graph := subagentTestBaseGraph([]string{"coder_subagent_execution.child"})

	state, err := subagentTestConvert(t, module, graph)
	require.Nil(t, state)
	require.ErrorContains(t, err, "unresolved parent")
}

func TestConvertStateSubagentExecutionRejectsNestedParent(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestDevcontainerResource("coder_devcontainer.child", "child", "", ""),
		subagentTestExecutionResource("coder_subagent_execution.grandchild", "grandchild", "", ""),
	)
	graph := subagentTestBaseGraph(
		[]string{"coder_devcontainer.child", "coder_subagent_execution.grandchild"},
		[2]string{"coder_devcontainer.child", "coder_agent.parent"},
		[2]string{"coder_subagent_execution.grandchild", "coder_devcontainer.child"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.Nil(t, state)
	require.ErrorContains(t, err, "is not a top-level coder_agent")
	require.ErrorContains(t, err, "coder_devcontainer")
}

func TestConvertStateSubagentExecutionMultipleParents(t *testing.T) {
	t.Parallel()

	module := &tfjson.StateModule{Resources: []*tfjson.StateResource{
		subagentTestResource("null_resource.one", "null_resource", "one", map[string]any{}),
		subagentTestResource("null_resource.two", "null_resource", "two", map[string]any{}),
		subagentTestAgentResource("coder_agent.one", "one", ""),
		subagentTestAgentResource("coder_agent.two", "two", ""),
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
	}}
	graph := subagentTestGraph(
		[]string{
			"null_resource.one",
			"null_resource.two",
			"coder_agent.one",
			"coder_agent.two",
			"coder_subagent_execution.child",
		},
		[2]string{"null_resource.one", "coder_agent.one"},
		[2]string{"null_resource.two", "coder_agent.two"},
		[2]string{"coder_subagent_execution.child", "coder_agent.one"},
		[2]string{"coder_subagent_execution.child", "coder_agent.two"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.Nil(t, state)
	require.ErrorContains(t, err, "parent has multiple matches")
}

func TestConvertStateSubagentExecutionNameDuplicate(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestExecutionResource("coder_subagent_execution.child", "Parent", "", ""),
	)
	graph := subagentTestBaseGraph(
		[]string{"coder_subagent_execution.child"},
		[2]string{"coder_subagent_execution.child", "coder_agent.parent"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.Nil(t, state)
	require.ErrorContains(t, err, "duplicate agent name")
}

func TestConvertStateSubagentExecutionNameConflictsWithDevcontainer(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestDevcontainerResource("coder_devcontainer.child", "Child", "", ""),
		subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
	)
	graph := subagentTestBaseGraph(
		[]string{"coder_devcontainer.child", "coder_subagent_execution.child"},
		[2]string{"coder_devcontainer.child", "coder_agent.parent"},
		[2]string{"coder_subagent_execution.child", "coder_agent.parent"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.Nil(t, state)
	require.ErrorContains(t, err, "duplicate agent name")
}

func TestConvertStateSubagentExecutionDuplicateDevcontainerNamesRemainAllowed(t *testing.T) {
	t.Parallel()

	module := subagentTestBaseModule("",
		subagentTestDevcontainerResource("coder_devcontainer.dev[0]", "dev", "", ""),
		subagentTestDevcontainerResource("coder_devcontainer.dev[1]", "dev", "", ""),
	)
	graph := subagentTestBaseGraph(
		[]string{"coder_devcontainer.dev"},
		[2]string{"coder_devcontainer.dev", "coder_agent.parent"},
	)

	state, err := subagentTestConvert(t, module, graph)
	require.NoError(t, err)
	require.Len(t, subagentTestFindAgent(t, state, "parent").Devcontainers, 2)
}

func TestConvertStateSubagentExecutionAppSlugDuplicate(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		resources []*tfjson.StateResource
		nodes     []string
		edges     [][2]string
	}{
		{
			name: "top-level-and-execution",
			resources: []*tfjson.StateResource{
				subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
				subagentTestAppResource("coder_app.parent", "parent", "", "duplicate-app"),
				subagentTestAppResource("coder_app.execution", "execution", "", "duplicate-app"),
			},
			nodes: []string{"coder_subagent_execution.child", "coder_app.parent", "coder_app.execution"},
			edges: [][2]string{
				{"coder_subagent_execution.child", "coder_agent.parent"},
				{"coder_app.parent", "coder_agent.parent"},
				{"coder_app.execution", "coder_subagent_execution.child"},
			},
		},
		{
			name: "devcontainer-and-execution",
			resources: []*tfjson.StateResource{
				subagentTestDevcontainerResource("coder_devcontainer.dev", "dev", "", ""),
				subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
				subagentTestAppResource("coder_app.devcontainer", "devcontainer", "", "duplicate-app"),
				subagentTestAppResource("coder_app.execution", "execution", "", "duplicate-app"),
			},
			nodes: []string{
				"coder_devcontainer.dev",
				"coder_subagent_execution.child",
				"coder_app.devcontainer",
				"coder_app.execution",
			},
			edges: [][2]string{
				{"coder_devcontainer.dev", "coder_agent.parent"},
				{"coder_subagent_execution.child", "coder_agent.parent"},
				{"coder_app.devcontainer", "coder_devcontainer.dev"},
				{"coder_app.execution", "coder_subagent_execution.child"},
			},
		},
		{
			name: "top-level-and-devcontainer",
			resources: []*tfjson.StateResource{
				subagentTestDevcontainerResource("coder_devcontainer.dev", "dev", "", ""),
				subagentTestExecutionResource("coder_subagent_execution.child", "child", "", ""),
				subagentTestAppResource("coder_app.parent", "parent", "", "duplicate-app"),
				subagentTestAppResource("coder_app.devcontainer", "devcontainer", "", "duplicate-app"),
			},
			nodes: []string{
				"coder_devcontainer.dev",
				"coder_subagent_execution.child",
				"coder_app.parent",
				"coder_app.devcontainer",
			},
			edges: [][2]string{
				{"coder_devcontainer.dev", "coder_agent.parent"},
				{"coder_subagent_execution.child", "coder_agent.parent"},
				{"coder_app.parent", "coder_agent.parent"},
				{"coder_app.devcontainer", "coder_devcontainer.dev"},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			module := subagentTestBaseModule("", testCase.resources...)
			graph := subagentTestBaseGraph(testCase.nodes, testCase.edges...)
			state, err := subagentTestConvert(t, module, graph)
			require.Nil(t, state)
			require.ErrorContains(t, err, "duplicate app slug")
		})
	}
}
