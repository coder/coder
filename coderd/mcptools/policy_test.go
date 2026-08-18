package mcptools_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/mcptools"
	"github.com/coder/coder/v2/codersdk"
)

func TestAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy mcptools.Policy
		tool   string
		want   bool
	}{
		{name: "no policies", tool: "read", want: true},
		{name: "allow exact match", policy: mcptools.Policy{AllowList: []string{"read"}}, tool: "read", want: true},
		{name: "allow requires exact match", policy: mcptools.Policy{AllowList: []string{"read"}}, tool: "read_all", want: false},
		{name: "deny exact match", policy: mcptools.Policy{DenyList: []string{"delete"}}, tool: "delete", want: false},
		{
			name: "matching rule overrides disabled default",
			policy: mcptools.Policy{
				Rules:   []codersdk.MCPServerToolRule{{Tool: "read", Enabled: true}},
				Default: "disabled",
			},
			tool: "read",
			want: true,
		},
		{
			name: "matching rule overrides enabled default",
			policy: mcptools.Policy{
				Rules:   []codersdk.MCPServerToolRule{{Tool: "delete", Enabled: false}},
				Default: "enabled",
			},
			tool: "delete",
			want: false,
		},
		{
			name: "disabled default denies unmatched tool",
			policy: mcptools.Policy{
				Rules:   []codersdk.MCPServerToolRule{{Tool: "read", Enabled: true}},
				Default: "disabled",
			},
			tool: "write",
			want: false,
		},
		{
			name: "deny and explicit enable use and semantics",
			policy: mcptools.Policy{
				DenyList: []string{"delete"},
				Rules:    []codersdk.MCPServerToolRule{{Tool: "delete", Enabled: true}},
				Default:  "disabled",
			},
			tool: "delete",
			want: false,
		},
		{
			name: "allow and explicit disable use and semantics",
			policy: mcptools.Policy{
				AllowList: []string{"read"},
				Rules:     []codersdk.MCPServerToolRule{{Tool: "read", Enabled: false}},
				Default:   "enabled",
			},
			tool: "read",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, mcptools.Allowed(tt.policy, tt.tool))
		})
	}
}
