package templatebuilder_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
)

func TestClassifyProvisionerError(t *testing.T) {
	t.Parallel()

	t.Run("NetworkErrors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			jobError string
			logs     []string
		}{
			{
				name:     "DNSFailure",
				jobError: "init failed",
				logs:     []string{"Error: Failed to query available provider packages", "dial tcp: lookup registry.terraform.io: no such host"},
			},
			{
				name:     "ConnectionRefused",
				jobError: "terraform init: connection refused",
				logs:     nil,
			},
			{
				name:     "IOTimeout",
				jobError: "context deadline exceeded",
				logs:     []string{"dial tcp 1.2.3.4:443: i/o timeout"},
			},
			{
				name:     "TLSTimeout",
				jobError: "init error",
				logs:     []string{"net/http: TLS handshake timeout"},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				result := templatebuilder.ClassifyProvisionerError(tc.jobError, tc.logs)
				require.Contains(t, result, "unreachable from your provisioner")
			})
		}
	})

	t.Run("NetworkTakesPrecedenceOverAuth", func(t *testing.T) {
		t.Parallel()
		// When both network and auth patterns appear, network wins
		// (checked first).
		result := templatebuilder.ClassifyProvisionerError(
			"connection refused",
			[]string{"AccessDenied: check credentials"},
		)
		require.Contains(t, result, "unreachable from your provisioner")
	})

	t.Run("AuthErrors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			jobError string
			logs     []string
		}{
			{
				name:     "AWSNoCredentials",
				jobError: "terraform plan: exit status 1",
				logs: []string{
					"Initializing the backend...",
					"Initializing provider plugins...",
					"Error: No valid credential sources found",
					"",
					"  on main.tf line 10, in provider \"aws\":",
					"  10:   region = \"us-east-1\"",
				},
			},
			{
				name:     "GCPDefaultCredentials",
				jobError: "terraform plan: exit status 1",
				logs: []string{
					"Error: could not find default credentials",
				},
			},
			{
				name:     "AzureAuthFailure",
				jobError: "terraform plan: exit status 1",
				logs:     []string{"Error: AuthorizationFailed for subscription"},
			},
			{
				name:     "CaseInsensitive",
				jobError: "terraform plan: exit status 1",
				logs:     []string{"Error: ACCESSDENIED on resource"},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				result := templatebuilder.ClassifyProvisionerError(tc.jobError, tc.logs)
				require.Contains(t, result, "Cloud provider authentication failed")
			})
		}
	})

	t.Run("AuthErrorNoLogsNoDanglingNewlines", func(t *testing.T) {
		t.Parallel()
		// Auth pattern in jobError with nil logs should not produce
		// trailing newlines.
		result := templatebuilder.ClassifyProvisionerError(
			"AccessDenied: you are not allowed",
			nil,
		)
		require.Contains(t, result, "Cloud provider authentication failed")
		require.False(t, strings.HasSuffix(result, "\n"), "should not end with newline")
	})

	t.Run("UnrecognizedErrorIncludesLogs", func(t *testing.T) {
		t.Parallel()
		result := templatebuilder.ClassifyProvisionerError(
			"terraform plan: exit status 1",
			[]string{
				"Initializing the backend...",
				"Initializing provider plugins...",
				"",
				"Error: Unsupported block type",
				"",
				"  on main.tf line 5, in resource \"null_resource\" \"test\":",
				"  5:   invalid_block {",
			},
		)
		require.Contains(t, result, "terraform plan: exit status 1")
		require.Contains(t, result, "Unsupported block type")
		require.Contains(t, result, "on main.tf line 5")
	})

	t.Run("UnrecognizedErrorNoLogs", func(t *testing.T) {
		t.Parallel()
		result := templatebuilder.ClassifyProvisionerError(
			"terraform plan: exit status 1",
			nil,
		)
		require.Equal(t, "terraform plan: exit status 1", result)
	})

	t.Run("EmptyErrorPassthrough", func(t *testing.T) {
		t.Parallel()
		result := templatebuilder.ClassifyProvisionerError("", nil)
		require.Equal(t, "", result)
	})

	t.Run("MultipleDiagnosticBlocks", func(t *testing.T) {
		t.Parallel()
		result := templatebuilder.ClassifyProvisionerError(
			"terraform plan: exit status 1",
			[]string{
				"Warning: Deprecated resource",
				"  Use the new resource instead.",
				"",
				"Error: Invalid reference",
				"  on main.tf line 12:",
				"  12:   value = var.undefined",
			},
		)
		require.Contains(t, result, "Deprecated resource")
		require.Contains(t, result, "Invalid reference")
		require.Contains(t, result, "on main.tf line 12")
	})

	t.Run("LogContextCappedAtMax", func(t *testing.T) {
		t.Parallel()
		var logs []string
		for i := 0; i < 50; i++ {
			logs = append(logs, "Error: problem number "+strings.Repeat("x", 10))
		}
		result := templatebuilder.ClassifyProvisionerError("exit status 1", logs)
		parts := strings.SplitN(result, "\n\n", 2)
		require.Len(t, parts, 2, "should have jobError and log context")
		contextLines := strings.Split(strings.TrimSpace(parts[1]), "\n")
		// 20 diagnostic lines + 1 truncation marker.
		require.Equal(t, 21, len(contextLines))
		require.Equal(t, "... (truncated)", contextLines[20])
	})

	t.Run("NonDiagnosticLogsFiltered", func(t *testing.T) {
		t.Parallel()
		result := templatebuilder.ClassifyProvisionerError(
			"terraform plan: exit status 1",
			[]string{
				"Initializing the backend...",
				"Initializing provider plugins...",
				"- Finding latest version of hashicorp/aws...",
				"- Installing hashicorp/aws v5.0.0...",
			},
		)
		require.Equal(t, "terraform plan: exit status 1", result)
	})
}
