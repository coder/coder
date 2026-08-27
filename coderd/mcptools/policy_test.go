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

func TestEvaluate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		policy mcptools.Policy
		tool   string
		want   mcptools.Action
	}{
		{
			name: "escalate rule",
			policy: mcptools.Policy{
				Rules: []codersdk.MCPServerToolRule{{Tool: "delete_repo", Action: codersdk.MCPServerToolActionEscalate}},
			},
			tool: "delete_repo",
			want: mcptools.ActionEscalate,
		},
		{
			name:   "escalate default for unmatched tool",
			policy: mcptools.Policy{Default: "escalate"},
			tool:   "anything",
			want:   mcptools.ActionEscalate,
		},
		{
			name: "legacy enabled boolean still decides when action is empty",
			policy: mcptools.Policy{
				Default: "disabled",
				Rules:   []codersdk.MCPServerToolRule{{Tool: "read", Enabled: true}},
			},
			tool: "read",
			want: mcptools.ActionPermit,
		},
		{
			name: "action wins over a contradictory legacy boolean",
			policy: mcptools.Policy{
				Rules: []codersdk.MCPServerToolRule{{Tool: "delete_repo", Action: codersdk.MCPServerToolActionEscalate, Enabled: false}},
			},
			tool: "delete_repo",
			want: mcptools.ActionEscalate,
		},
		{
			name: "deny list blocks an escalated tool outright",
			policy: mcptools.Policy{
				DenyList: []string{"delete_repo"},
				Rules:    []codersdk.MCPServerToolRule{{Tool: "delete_repo", Action: codersdk.MCPServerToolActionEscalate}},
			},
			tool: "delete_repo",
			want: mcptools.ActionBlock,
		},
		{
			name: "allow list excludes an escalated tool outright",
			policy: mcptools.Policy{
				AllowList: []string{"read"},
				Rules:     []codersdk.MCPServerToolRule{{Tool: "delete_repo", Action: codersdk.MCPServerToolActionEscalate}},
			},
			tool: "delete_repo",
			want: mcptools.ActionBlock,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, mcptools.Evaluate(tc.policy, tc.tool))
		})
	}
}

func TestAllowedFailsClosedOnEscalate(t *testing.T) {
	t.Parallel()

	// Consumers without an escalation path (for example chat MCP clients)
	// must treat escalated tools as not allowed.
	policy := mcptools.Policy{
		Rules: []codersdk.MCPServerToolRule{{Tool: "delete_repo", Action: codersdk.MCPServerToolActionEscalate}},
	}
	require.False(t, mcptools.Allowed(policy, "delete_repo"))
}
