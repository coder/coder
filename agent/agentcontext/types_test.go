package agentcontext_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
)

func TestResourceKindString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind agentcontext.ResourceKind
		want string
	}{
		{agentcontext.KindUnspecified, "unknown"},
		{agentcontext.KindInstructionFile, "instruction_file"},
		{agentcontext.KindSkill, "skill"},
		{agentcontext.KindMCPConfig, "mcp_config"},
		{agentcontext.KindMCPServer, "mcp_server"},
		{agentcontext.KindPlugin, "plugin"},
		{agentcontext.KindHook, "hook"},
		{agentcontext.KindSubagent, "subagent"},
		{agentcontext.KindCommand, "command"},
		{agentcontext.ResourceKind(999), "unknown"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, tt.kind.String())
	}
}

func TestResourceStatusString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status agentcontext.ResourceStatus
		want   string
	}{
		{agentcontext.StatusOK, "ok"},
		{agentcontext.StatusOversize, "oversize"},
		{agentcontext.StatusUnreadable, "unreadable"},
		{agentcontext.StatusInvalid, "invalid"},
		{agentcontext.StatusExcluded, "excluded"},
		{agentcontext.ResourceStatus(999), "unknown"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, tt.status.String())
	}
}

func TestComputeAggregateHash_DeterministicAcrossOrder(t *testing.T) {
	t.Parallel()
	a := agentcontext.Resource{
		ID:     "instruction_file:/a/AGENTS.md",
		Kind:   agentcontext.KindInstructionFile,
		Source: "/a/AGENTS.md",
		Status: agentcontext.StatusOK,
	}
	b := agentcontext.Resource{
		ID:     "instruction_file:/b/AGENTS.md",
		Kind:   agentcontext.KindInstructionFile,
		Source: "/b/AGENTS.md",
		Status: agentcontext.StatusOK,
	}
	got1 := agentcontext.ComputeAggregateHash([]agentcontext.Resource{a, b})
	got2 := agentcontext.ComputeAggregateHash([]agentcontext.Resource{b, a})
	require.Equal(t, got1, got2)
}

func TestComputeAggregateHash_ChangesOnContent(t *testing.T) {
	t.Parallel()
	base := agentcontext.Resource{
		ID:     "instruction_file:/a/AGENTS.md",
		Kind:   agentcontext.KindInstructionFile,
		Source: "/a/AGENTS.md",
		Status: agentcontext.StatusOK,
	}
	hash1 := agentcontext.ComputeAggregateHash([]agentcontext.Resource{base})

	withContent := base
	withContent.ContentHash = [32]byte{0x01}
	hash2 := agentcontext.ComputeAggregateHash([]agentcontext.Resource{withContent})
	require.NotEqual(t, hash1, hash2)

	withStatus := base
	withStatus.Status = agentcontext.StatusOversize
	hash3 := agentcontext.ComputeAggregateHash([]agentcontext.Resource{withStatus})
	require.NotEqual(t, hash1, hash3)
}
