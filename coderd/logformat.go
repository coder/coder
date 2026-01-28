package coderd

import (
	"strings"

	"github.com/coder/coder/v2/coderd/database"
)

// FormatProvisionerJobLogsAsText converts provisioner job logs to plain text format.
// Each log line is formatted as: {timestamp} [{level}] [{source}] {stage}: {output}
// ANSI escape sequences in the output are preserved.
func FormatProvisionerJobLogsAsText(logs []database.ProvisionerJobLog) string {
	if len(logs) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, log := range logs {
		// Format: 2024-01-28T10:30:00Z [info] [provisioner] Planning: Terraform output
		_, _ = sb.WriteString(log.CreatedAt.Format("2006-01-02T15:04:05.000Z07:00"))
		_, _ = sb.WriteString(" [")
		_, _ = sb.WriteString(string(log.Level))
		_, _ = sb.WriteString("] [")
		_, _ = sb.WriteString(string(log.Source))
		_, _ = sb.WriteString("] ")
		if log.Stage != "" {
			_, _ = sb.WriteString(log.Stage)
			_, _ = sb.WriteString(": ")
		}
		_, _ = sb.WriteString(log.Output)
		_, _ = sb.WriteString("\n")
	}
	return sb.String()
}

// FormatWorkspaceAgentLogsAsText converts workspace agent logs to plain text format.
// Each log line is formatted as: {timestamp} [{level}] {output}
// ANSI escape sequences in the output are preserved.
func FormatWorkspaceAgentLogsAsText(logs []database.WorkspaceAgentLog) string {
	if len(logs) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, log := range logs {
		// Format: 2024-01-28T10:30:00Z [info] Agent started successfully
		_, _ = sb.WriteString(log.CreatedAt.Format("2006-01-02T15:04:05.000Z07:00"))
		_, _ = sb.WriteString(" [")
		_, _ = sb.WriteString(string(log.Level))
		_, _ = sb.WriteString("] ")
		_, _ = sb.WriteString(log.Output)
		_, _ = sb.WriteString("\n")
	}
	return sb.String()
}
