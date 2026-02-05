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

	agentID := uuid.New()

	base := codersdk.WorkspaceAgentDevcontainer{
		ID:              uuid.New(),
		Name:            "test-dc",
		WorkspaceFolder: "/workspace",
		Status:          codersdk.WorkspaceAgentDevcontainerStatusRunning,
		Dirty:           false,
		Container:       &codersdk.WorkspaceAgentContainer{ID: "container-123"},
		Agent:           &codersdk.WorkspaceAgentDevcontainerAgent{ID: agentID, Name: "agent-1"},
		Error:           "",
	}

	tests := []struct {
		name      string
		modify    func(*codersdk.WorkspaceAgentDevcontainer)
		wantEqual bool
	}{
		{
			name:      "identical",
			modify:    func(d *codersdk.WorkspaceAgentDevcontainer) {},
			wantEqual: true,
		},
		{
			name:      "different ID",
			modify:    func(d *codersdk.WorkspaceAgentDevcontainer) { d.ID = uuid.New() },
			wantEqual: false,
		},
		{
			name:      "different Name",
			modify:    func(d *codersdk.WorkspaceAgentDevcontainer) { d.Name = "other-dc" },
			wantEqual: false,
		},
		{
			name:      "different WorkspaceFolder",
			modify:    func(d *codersdk.WorkspaceAgentDevcontainer) { d.WorkspaceFolder = "/other" },
			wantEqual: false,
		},
		{
			name: "different SubagentID (one valid, one nil)",
			modify: func(d *codersdk.WorkspaceAgentDevcontainer) {
				d.SubagentID = uuid.NullUUID{Valid: true, UUID: uuid.New()}
			},
			wantEqual: false,
		},
		{
			name: "different SubagentID UUIDs",
			modify: func(d *codersdk.WorkspaceAgentDevcontainer) {
				d.SubagentID = uuid.NullUUID{Valid: true, UUID: uuid.New()}
			},
			wantEqual: false,
		},
		{
			name: "different Status",
			modify: func(d *codersdk.WorkspaceAgentDevcontainer) {
				d.Status = codersdk.WorkspaceAgentDevcontainerStatusStopped
			},
			wantEqual: false,
		},
		{
			name:      "different Dirty",
			modify:    func(d *codersdk.WorkspaceAgentDevcontainer) { d.Dirty = true },
			wantEqual: false,
		},
		{
			name:      "different Container (one nil)",
			modify:    func(d *codersdk.WorkspaceAgentDevcontainer) { d.Container = nil },
			wantEqual: false,
		},
		{
			name: "different Container IDs",
			modify: func(d *codersdk.WorkspaceAgentDevcontainer) {
				d.Container = &codersdk.WorkspaceAgentContainer{ID: "different-container"}
			},
			wantEqual: false,
		},
		{
			name:      "different Agent (one nil)",
			modify:    func(d *codersdk.WorkspaceAgentDevcontainer) { d.Agent = nil },
			wantEqual: false,
		},
		{
			name: "different Agent values",
			modify: func(d *codersdk.WorkspaceAgentDevcontainer) {
				d.Agent = &codersdk.WorkspaceAgentDevcontainerAgent{ID: agentID, Name: "agent-2"}
			},
			wantEqual: false,
		},
		{
			name:      "different Error",
			modify:    func(d *codersdk.WorkspaceAgentDevcontainer) { d.Error = "some error" },
			wantEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			modified := base
			tt.modify(&modified)
			require.Equal(t, tt.wantEqual, base.Equals(modified))
		})
	}
}

func TestWorkspaceAgentDevcontainerIsTerraformDefined(t *testing.T) {
	t.Parallel()

	t.Run("SubagentID Valid", func(t *testing.T) {
		dc := codersdk.WorkspaceAgentDevcontainer{
			ID:              uuid.New(),
			Name:            "test-dc",
			WorkspaceFolder: "/workspace",
			SubagentID:      uuid.NullUUID{Valid: true, UUID: uuid.New()},
		}

		require.True(t, dc.IsTerraformDefined())
	})

	t.Run("SubagentID Null", func(t *testing.T) {
		dc := codersdk.WorkspaceAgentDevcontainer{
			ID:              uuid.New(),
			Name:            "test-dc",
			WorkspaceFolder: "/workspace",
			SubagentID:      uuid.NullUUID{Valid: false},
		}

		require.False(t, dc.IsTerraformDefined())
	})
}
