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
