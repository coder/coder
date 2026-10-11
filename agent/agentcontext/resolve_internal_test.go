package agentcontext

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactResourcesDropsReplacedOccurrence(t *testing.T) {
	t.Parallel()

	var out []Resource
	seenID := make(map[string]int)
	appendResource(&out, seenID, Resource{ID: "instruction_file:/repo/rules.md", Source: "/repo/AGENTS.md", Status: StatusUnreadable})
	appendResource(&out, seenID, Resource{ID: "instruction_file:/repo/rules.md", Source: "/repo/CLAUDE.md", Status: StatusOK})
	require.Len(t, out, 2, "the replaced occurrence is left as a tombstone")
	require.Equal(t, []Resource{{ID: "instruction_file:/repo/rules.md", Source: "/repo/CLAUDE.md", Status: StatusOK}}, compactResources(out))
}
