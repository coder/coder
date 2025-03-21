package mcptools_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	mcptools "github.com/coder/coder/v2/mcp/tools"
)

func TestIsCommandAllowed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		command         string
		allowedCommands []string
		want            bool
		wantErr         bool
		errorMessage    string
	}{
		{
			name:            "empty allowed commands allows all",
			command:         "ls -la",
			allowedCommands: []string{},
			want:            true,
			wantErr:         false,
		},
		{
			name:            "allowed command",
			command:         "ls -la",
			allowedCommands: []string{"ls", "cat", "grep"},
			want:            true,
			wantErr:         false,
		},
		{
			name:            "disallowed command",
			command:         "rm -rf /",
			allowedCommands: []string{"ls", "cat", "grep"},
			want:            false,
			wantErr:         true,
			errorMessage:    "not allowed",
		},
		{
			name:            "command with quotes",
			command:         "echo \"hello world\"",
			allowedCommands: []string{"echo", "cat", "grep"},
			want:            true,
			wantErr:         false,
		},
		{
			name:            "command with path",
			command:         "/bin/ls -la",
			allowedCommands: []string{"/bin/ls", "cat", "grep"},
			want:            true,
			wantErr:         false,
		},
		{
			name:            "empty command",
			command:         "",
			allowedCommands: []string{"ls", "cat", "grep"},
			want:            false,
			wantErr:         true,
			errorMessage:    "empty command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := mcptools.IsCommandAllowed(tt.command, tt.allowedCommands)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errorMessage != "" {
					require.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.want, got)
		})
	}
}
