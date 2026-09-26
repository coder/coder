package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTurnExperimentDecisions(t *testing.T) {
	t.Parallel()

	var d turnExperimentDecisions
	calls := 0
	evaluate := func(result bool) func() bool {
		return func() bool {
			calls++
			return result
		}
	}

	// Later steps of a turn reuse the first decision even when the rule
	// has changed since.
	require.True(t, d.mcpToolSearchEnabled(7, evaluate(true)))
	require.True(t, d.mcpToolSearchEnabled(7, evaluate(false)))
	require.Equal(t, 1, calls)

	// A new prompt row is a new turn and decides again.
	require.False(t, d.mcpToolSearchEnabled(8, evaluate(false)))
	require.Equal(t, 2, calls)

	// A canceled task of an older turn that finishes evaluating late must
	// not replace the newer turn's decision. Prompt row IDs increase.
	require.True(t, d.mcpToolSearchEnabled(9, evaluate(true)))
	require.False(t, d.mcpToolSearchEnabled(8, evaluate(false)))
	require.True(t, d.mcpToolSearchEnabled(9, evaluate(false)))
	require.Equal(t, 4, calls)

	// Without a prompt row there is no turn identity, so nothing is cached.
	require.True(t, d.mcpToolSearchEnabled(0, evaluate(true)))
	require.False(t, d.mcpToolSearchEnabled(0, evaluate(false)))
	require.Equal(t, 6, calls)
}
