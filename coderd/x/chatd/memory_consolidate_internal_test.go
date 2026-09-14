package chatd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

func TestValidateMemoryConsolidationMutations(t *testing.T) {
	t.Parallel()

	now := time.Now()
	memories := []chattool.Memory{
		{Name: "old-a", Description: "Old A", Body: "A", UpdatedAt: now.Add(-48 * time.Hour)},
		{Name: "old-b", Description: "Old B", Body: "B", UpdatedAt: now.Add(-48 * time.Hour)},
		{Name: "fresh", Description: "Fresh", Body: "Fresh", UpdatedAt: now.Add(-time.Hour)},
	}
	proposed := []memoryConsolidationMutation{
		{Op: "merge", Into: "merged", From: []string{"old-a", "old-b"}, Description: "Merged", Body: "Merged body"},
		{Op: "update", Name: "fresh", Description: "Changed", Body: "Changed body"},
	}
	for range memoryConsolidationMaxMutations - 2 {
		proposed = append(proposed, memoryConsolidationMutation{Op: "delete", Name: "old-a"})
	}

	mutations := validateMemoryConsolidationMutations(proposed, memories, now)
	require.Len(t, mutations, 1)
	require.Equal(t, "merge", mutations[0].Op)
	require.NotContains(t, mutations, memoryConsolidationMutation{Op: "update", Name: "fresh", Description: "Changed", Body: "Changed body"})
}

func TestFormatMemoryConsolidationInput(t *testing.T) {
	t.Parallel()

	input := formatMemoryConsolidationInput([]chattool.Memory{{Name: "memory", Description: "Description", Body: string(make([]byte, memoryConsolidationBodyBytes+1))}})
	require.LessOrEqual(t, len(input), memoryConsolidationInputBytes)
	require.Contains(t, input, `name="memory"`)
}
