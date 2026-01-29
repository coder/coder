package coderd_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
)

func TestFormatProvisionerJobLogsAsText(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		result := coderd.FormatProvisionerJobLogsAsText(nil)
		require.Empty(t, result)

		result = coderd.FormatProvisionerJobLogsAsText([]database.ProvisionerJobLog{})
		require.Empty(t, result)
	})

	t.Run("SingleLog", func(t *testing.T) {
		t.Parallel()
		ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
		logs := []database.ProvisionerJobLog{
			{
				CreatedAt: ts,
				Level:     database.LogLevelInfo,
				Source:    database.LogSourceProvisioner,
				Stage:     "Planning",
				Output:    "Terraform init complete",
			},
		}
		result := coderd.FormatProvisionerJobLogsAsText(logs)
		require.Equal(t, "2024-01-28T10:30:00.000Z [info] [provisioner] Planning: Terraform init complete\n", result)
	})

	t.Run("MultipleLogs", func(t *testing.T) {
		t.Parallel()
		ts1 := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
		ts2 := time.Date(2024, 1, 28, 10, 30, 1, 0, time.UTC)
		logs := []database.ProvisionerJobLog{
			{
				CreatedAt: ts1,
				Level:     database.LogLevelInfo,
				Source:    database.LogSourceProvisioner,
				Stage:     "Planning",
				Output:    "First log",
			},
			{
				CreatedAt: ts2,
				Level:     database.LogLevelError,
				Source:    database.LogSourceProvisionerDaemon,
				Stage:     "Applying",
				Output:    "Second log",
			},
		}
		result := coderd.FormatProvisionerJobLogsAsText(logs)
		lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
		require.Len(t, lines, 2)
		require.Equal(t, "2024-01-28T10:30:00.000Z [info] [provisioner] Planning: First log", lines[0])
		require.Equal(t, "2024-01-28T10:30:01.000Z [error] [provisioner_daemon] Applying: Second log", lines[1])
	})

	t.Run("EmptyStage", func(t *testing.T) {
		t.Parallel()
		ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
		logs := []database.ProvisionerJobLog{
			{
				CreatedAt: ts,
				Level:     database.LogLevelDebug,
				Source:    database.LogSourceProvisioner,
				Stage:     "",
				Output:    "No stage log",
			},
		}
		result := coderd.FormatProvisionerJobLogsAsText(logs)
		require.Equal(t, "2024-01-28T10:30:00.000Z [debug] [provisioner] No stage log\n", result)
	})

	t.Run("PreservesANSI", func(t *testing.T) {
		t.Parallel()
		ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
		// ANSI escape code for red text
		ansiOutput := "\x1b[31mError: something went wrong\x1b[0m"
		logs := []database.ProvisionerJobLog{
			{
				CreatedAt: ts,
				Level:     database.LogLevelError,
				Source:    database.LogSourceProvisioner,
				Stage:     "Apply",
				Output:    ansiOutput,
			},
		}
		result := coderd.FormatProvisionerJobLogsAsText(logs)
		require.Contains(t, result, ansiOutput)
	})
}

func TestFormatWorkspaceAgentLogsAsText(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		result := coderd.FormatWorkspaceAgentLogsAsText(nil)
		require.Empty(t, result)

		result = coderd.FormatWorkspaceAgentLogsAsText([]database.WorkspaceAgentLog{})
		require.Empty(t, result)
	})

	t.Run("SingleLog", func(t *testing.T) {
		t.Parallel()
		ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
		logs := []database.WorkspaceAgentLog{
			{
				CreatedAt: ts,
				Level:     database.LogLevelInfo,
				Output:    "Agent started successfully",
			},
		}
		result := coderd.FormatWorkspaceAgentLogsAsText(logs)
		require.Equal(t, "2024-01-28T10:30:00.000Z [info] Agent started successfully\n", result)
	})

	t.Run("MultipleLogs", func(t *testing.T) {
		t.Parallel()
		ts1 := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
		ts2 := time.Date(2024, 1, 28, 10, 30, 1, 0, time.UTC)
		logs := []database.WorkspaceAgentLog{
			{
				CreatedAt: ts1,
				Level:     database.LogLevelInfo,
				Output:    "First agent log",
			},
			{
				CreatedAt: ts2,
				Level:     database.LogLevelWarn,
				Output:    "Second agent log",
			},
		}
		result := coderd.FormatWorkspaceAgentLogsAsText(logs)
		lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
		require.Len(t, lines, 2)
		require.Equal(t, "2024-01-28T10:30:00.000Z [info] First agent log", lines[0])
		require.Equal(t, "2024-01-28T10:30:01.000Z [warn] Second agent log", lines[1])
	})

	t.Run("PreservesANSI", func(t *testing.T) {
		t.Parallel()
		ts := time.Date(2024, 1, 28, 10, 30, 0, 0, time.UTC)
		// ANSI escape code for green text
		ansiOutput := "\x1b[32mSuccess: operation completed\x1b[0m"
		logs := []database.WorkspaceAgentLog{
			{
				CreatedAt: ts,
				Level:     database.LogLevelInfo,
				Output:    ansiOutput,
			},
		}
		result := coderd.FormatWorkspaceAgentLogsAsText(logs)
		require.Contains(t, result, ansiOutput)
	})
}
