package chatloop

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolLabel(t *testing.T) {
	t.Parallel()

	builtins := map[string]bool{
		"read_file":  true,
		"write_file": true,
		"execute":    true,
	}

	tests := []struct {
		name     string
		toolName string
		want     string
	}{
		{
			name:     "builtin tool",
			toolName: "read_file",
			want:     "read_file",
		},
		{
			name:     "mcp tool",
			toolName: "server__grep",
			want:     "mcp:server__grep",
		},
		{
			name:     "unknown tool",
			toolName: "some_random_tool",
			want:     "mcp:some_random_tool",
		},
		{
			name:     "empty builtins map treats all as mcp",
			toolName: "read_file",
			want:     "mcp:read_file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := builtins
			if tt.name == "empty builtins map treats all as mcp" {
				b = nil
			}
			assert.Equal(t, tt.want, toolLabel(tt.toolName, b))
		})
	}
}
