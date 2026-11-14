package chatdebug_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/codersdk"
)

// allRunKinds contains every RunKind value for SDK parity checks.
var allRunKinds = []chatdebug.RunKind{
	chatdebug.KindChatTurn,
	chatdebug.KindTitleGeneration,
	chatdebug.KindQuickgen,
	chatdebug.KindCompaction,
}

// allStatuses contains every Status value for SDK parity checks.
var allStatuses = []chatdebug.Status{
	chatdebug.StatusInProgress,
	chatdebug.StatusCompleted,
	chatdebug.StatusError,
	chatdebug.StatusInterrupted,
}

// allOperations contains every Operation value for SDK parity checks.
var allOperations = []chatdebug.Operation{
	chatdebug.OperationStream,
	chatdebug.OperationGenerate,
}

// toStrings converts a typed string slice to []string for comparison.
func toStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

// TestTypesMatchSDK verifies that every chatdebug constant has a
// corresponding codersdk constant with the same string value.
// If this test fails you probably added a constant to one package
// but forgot to update the other.
func TestTypesMatchSDK(t *testing.T) {
	t.Parallel()

	t.Run("RunKind", func(t *testing.T) {
		t.Parallel()
		require.ElementsMatch(t,
			toStrings(allRunKinds),
			toStrings(codersdk.AllChatDebugRunKinds),
			"allRunKinds and codersdk.AllChatDebugRunKinds have diverged",
		)
	})

	t.Run("Status", func(t *testing.T) {
		t.Parallel()
		require.ElementsMatch(t,
			toStrings(allStatuses),
			toStrings(codersdk.AllChatDebugStatuses),
			"allStatuses and codersdk.AllChatDebugStatuses have diverged",
		)
	})

	t.Run("Operation", func(t *testing.T) {
		t.Parallel()
		require.ElementsMatch(t,
			toStrings(allOperations),
			toStrings(codersdk.AllChatDebugStepOperations),
			"allOperations and codersdk.AllChatDebugStepOperations have diverged",
		)
	})
}
