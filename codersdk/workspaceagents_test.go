package codersdk_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestProvisionerJobLogText(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
	log := codersdk.ProvisionerJobLog{
		CreatedAt: ts,
		Level:     codersdk.LogLevelInfo,
		Source:    codersdk.LogSourceProvisioner,
		Stage:     "Planning",
		Output:    "Terraform init complete",
	}
	result := log.Text()
	require.Equal(t, "2024-01-28T10:30:00Z [info] [provisioner|Planning] Terraform init complete", result)
}

func TestProvisionerJobLogTextEmptyOutput(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
	log := codersdk.ProvisionerJobLog{
		CreatedAt: ts,
		Level:     codersdk.LogLevelInfo,
		Source:    codersdk.LogSourceProvisioner,
		Stage:     "Planning",
		Output:    "",
	}
	result := log.Text()
	require.Equal(t, "2024-01-28T10:30:00Z [info] [provisioner|Planning] ", result)
}

func TestProvisionerJobLogTextSpecialChars(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
	log := codersdk.ProvisionerJobLog{
		CreatedAt: ts,
		Level:     codersdk.LogLevelInfo,
		Source:    codersdk.LogSourceProvisioner,
		Stage:     "Applying",
		Output:    "\033[32mSuccess!\033[0m Unicode: 你好世界",
	}
	result := log.Text()
	require.Equal(t, "2024-01-28T10:30:00Z [info] [provisioner|Applying] \033[32mSuccess!\033[0m Unicode: 你好世界", result)
}

func TestWorkspaceAgentLogText(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
	log := codersdk.WorkspaceAgentLog{
		CreatedAt: ts,
		Level:     codersdk.LogLevelInfo,
		Output:    "Agent started successfully",
		SourceID:  uuid.New(),
	}
	result := log.Text("main", "startup_script")
	require.Equal(t, "2024-01-28T10:30:00Z [info] [agent.main|startup_script] Agent started successfully", result)
}

func TestWorkspaceAgentLogTextEmptySourceAndAgent(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
	log := codersdk.WorkspaceAgentLog{
		CreatedAt: ts,
		Level:     codersdk.LogLevelWarn,
		Output:    "Warning message",
		SourceID:  uuid.New(),
	}
	result := log.Text("", "")
	require.Equal(t, "2024-01-28T10:30:00Z [warn] [agent] Warning message", result)
}

func TestWorkspaceAgentLogTextMultiline(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
	log := codersdk.WorkspaceAgentLog{
		CreatedAt: ts,
		Level:     codersdk.LogLevelInfo,
		Output:    "Line 1\nLine 2\nLine 3",
		SourceID:  uuid.New(),
	}
	result := log.Text("main", "startup_script")
	require.Equal(t, "2024-01-28T10:30:00Z [info] [agent.main|startup_script] Line 1\nLine 2\nLine 3", result)
}

func TestWorkspaceAgentLogTextSpecialChars(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
	log := codersdk.WorkspaceAgentLog{
		CreatedAt: ts,
		Level:     codersdk.LogLevelDebug,
		Output:    "\033[31mError!\033[0m 🚀 Unicode: 日本語",
		SourceID:  uuid.New(),
	}
	result := log.Text("main", "startup_script")
	require.Equal(t, "2024-01-28T10:30:00Z [debug] [agent.main|startup_script] \033[31mError!\033[0m 🚀 Unicode: 日本語", result)
}

func TestWorkspaceAgentDevcontainerEquals(t *testing.T) {
	t.Parallel()

	baseID := uuid.New()
	subagentID := uuid.New()
	containerID := "container-123"
	agentID := uuid.New()

	base := codersdk.WorkspaceAgentDevcontainer{
		ID:              baseID,
		Name:            "test-dc",
		WorkspaceFolder: "/workspace",
		Status:          codersdk.WorkspaceAgentDevcontainerStatusRunning,
		Dirty:           false,
		Container:       &codersdk.WorkspaceAgentContainer{ID: containerID},
		Agent:           &codersdk.WorkspaceAgentDevcontainerAgent{ID: agentID, Name: "agent-1"},
		Error:           "",
	}

	tests := []struct {
		name        string
		modify      func(codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer
		expectEqual bool
	}{
		{
			name:        "identical",
			modify:      func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer { return d },
			expectEqual: true,
		},
		{
			name: "different ID",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.ID = uuid.New()
				return d
			},
			expectEqual: false,
		},
		{
			name: "different Name",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.Name = "other-dc"
				return d
			},
			expectEqual: false,
		},
		{
			name: "different WorkspaceFolder",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.WorkspaceFolder = "/other"
				return d
			},
			expectEqual: false,
		},
		{
			name: "different SubagentID (one valid, one nil)",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.SubagentID = uuid.NullUUID{Valid: true, UUID: subagentID}
				return d
			},
			expectEqual: false,
		},
		{
			name: "different SubagentID UUIDs",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.SubagentID = uuid.NullUUID{Valid: true, UUID: uuid.New()}
				return d
			},
			expectEqual: false,
		},
		{
			name: "different Status",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.Status = codersdk.WorkspaceAgentDevcontainerStatusStopped
				return d
			},
			expectEqual: false,
		},
		{
			name: "different Dirty",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.Dirty = true
				return d
			},
			expectEqual: false,
		},
		{
			name: "different Container (one nil)",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.Container = nil
				return d
			},
			expectEqual: false,
		},
		{
			name: "different Container IDs",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.Container = &codersdk.WorkspaceAgentContainer{ID: "different-container"}
				return d
			},
			expectEqual: false,
		},
		{
			name: "different Agent (one nil)",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.Agent = nil
				return d
			},
			expectEqual: false,
		},
		{
			name: "different Agent values",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.Agent = &codersdk.WorkspaceAgentDevcontainerAgent{ID: agentID, Name: "agent-2"}
				return d
			},
			expectEqual: false,
		},
		{
			name: "different Error",
			modify: func(d codersdk.WorkspaceAgentDevcontainer) codersdk.WorkspaceAgentDevcontainer {
				d.Error = "some error"
				return d
			},
			expectEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			other := tt.modify(base)
			require.Equal(t, tt.expectEqual, base.Equals(other))
		})
	}
}

func TestWorkspaceAgentDevcontainerIsTerraformDefined(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		subagentID               uuid.NullUUID
		expectIsTerraformDefined bool
	}{
		{
			name:                     "false when SubagentID is not valid",
			subagentID:               uuid.NullUUID{},
			expectIsTerraformDefined: false,
		},
		{
			name:                     "true when SubagentID is valid",
			subagentID:               uuid.NullUUID{Valid: true, UUID: uuid.New()},
			expectIsTerraformDefined: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dc := codersdk.WorkspaceAgentDevcontainer{
				ID:              uuid.New(),
				Name:            "test-dc",
				WorkspaceFolder: "/workspace",
				SubagentID:      tt.subagentID,
			}
			require.Equal(t, tt.expectIsTerraformDefined, dc.IsTerraformDefined())
		})
	}
}
