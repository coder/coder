package chatdebug_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/codersdk"
)

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
			toStrings(chatdebug.AllRunKinds),
			toStrings(codersdk.AllChatDebugRunKinds),
			"chatdebug.AllRunKinds and codersdk.AllChatDebugRunKinds have diverged",
		)
	})

	t.Run("Status", func(t *testing.T) {
		t.Parallel()
		require.ElementsMatch(t,
			toStrings(chatdebug.AllStatuses),
			toStrings(codersdk.AllChatDebugStatuses),
			"chatdebug.AllStatuses and codersdk.AllChatDebugStatuses have diverged",
		)
	})

	t.Run("Operation", func(t *testing.T) {
		t.Parallel()
		require.ElementsMatch(t,
			toStrings(chatdebug.AllOperations),
			toStrings(codersdk.AllChatDebugStepOperations),
			"chatdebug.AllOperations and codersdk.AllChatDebugStepOperations have diverged",
		)
	})
}
