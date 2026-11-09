package terraform

import (
	"context"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/terraform/scriptorder"
)

func TestResolveAppliedAgentBindings(t *testing.T) {
	t.Parallel()

	root := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			scriptOrderTestScript("coder_script.agent", "agent", "agent-id"),
			scriptOrderTestScript("coder_script.unselected", "unselected", "missing-id"),
		},
		ChildModules: []*tfjson.StateModule{{
			Address: "module.repo",
			Resources: []*tfjson.StateResource{
				scriptOrderTestScript(
					"module.repo.coder_script.devcontainer", "devcontainer", "subagent-id",
				),
			},
		}},
	}
	agentCatalog := scriptOrderAgentCatalog{}
	agentCatalog.index("agent-id", "coder_agent.main")
	agentCatalog.index("agent-id", "coder_agent.main")
	agentCatalog.index("subagent-id", "module.repo.coder_devcontainer.main")

	bindings, err := resolveAppliedAgentBindings(
		t.Context(),
		[]*tfjson.StateModule{root},
		agentCatalog,
		[]string{"coder_script.agent", "module.repo.coder_script.devcontainer"},
	)
	require.NoError(t, err)
	require.Equal(t, map[string]scriptorder.RuntimeBinding{
		"coder_script.agent": {
			RuntimeAddress: "coder_agent.main",
		},
		"module.repo.coder_script.devcontainer": {
			RuntimeAddress: "module.repo.coder_devcontainer.main",
		},
	}, bindings)
	require.NotContains(t, bindings, "coder_script.unselected")
}

func TestResolveAppliedAgentBindingsRecordsErrors(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name           string
		agentAddresses map[string][]string
		scriptID       string
		errorContains  string
	}{
		{
			name:          "Missing",
			scriptID:      "missing-id",
			errorContains: "does not match any workspace agent or devcontainer subagent",
		},
		{
			name: "Ambiguous",
			agentAddresses: map[string][]string{
				"duplicate-id": {"coder_agent.second", "coder_agent.first"},
			},
			scriptID:      "duplicate-id",
			errorContains: "matches multiple workspace agents or devcontainer subagents",
		},
		{
			name:          "Empty",
			errorContains: "agent_id is empty",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			const address = "coder_script.selected"
			root := &tfjson.StateModule{Resources: []*tfjson.StateResource{
				scriptOrderTestScript(address, "selected", test.scriptID),
			}}
			agentCatalog := scriptOrderAgentCatalog{}
			for id, addresses := range test.agentAddresses {
				for _, agentAddress := range addresses {
					agentCatalog.index(id, agentAddress)
				}
			}
			bindings, err := resolveAppliedAgentBindings(
				t.Context(), []*tfjson.StateModule{root}, agentCatalog,
				[]string{address},
			)
			require.NoError(t, err)
			require.Empty(t, bindings[address].RuntimeAddress)
			require.Contains(t, bindings[address].Error, test.errorContains)
		})
	}
}

func TestResolveAppliedAgentBindingsContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	bindings, err := resolveAppliedAgentBindings(ctx, nil, nil, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, bindings)
}

func TestResolveAppliedAgentBindingsMissingSelectedScript(t *testing.T) {
	t.Parallel()

	bindings, err := resolveAppliedAgentBindings(
		t.Context(), nil, nil, []string{"coder_script.missing"},
	)
	require.NoError(t, err)
	require.Equal(t, scriptorder.RuntimeBinding{
		Error: "script was not found in applied Terraform state",
	}, bindings["coder_script.missing"])
}

func scriptOrderTestAgent(address, name, id string) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_agent",
		Name:    name,
		AttributeValues: map[string]any{
			"arch": "amd64",
			"id":   id,
		},
	}
}

func scriptOrderTestDevcontainer(
	address, name, agentID, subagentID string,
) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_devcontainer",
		Name:    name,
		AttributeValues: map[string]any{
			"agent_id":         agentID,
			"subagent_id":      subagentID,
			"workspace_folder": "/workspace",
		},
	}
}

func scriptOrderTestScript(address, name, agentID string) *tfjson.StateResource {
	return &tfjson.StateResource{
		Address: address,
		Mode:    tfjson.ManagedResourceMode,
		Type:    "coder_script",
		Name:    name,
		AttributeValues: map[string]any{
			"agent_id":     agentID,
			"display_name": name,
			"run_on_start": true,
		},
	}
}
