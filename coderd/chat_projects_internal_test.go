package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatProjectDeleteFailureMessage(t *testing.T) {
	t.Parallel()

	require.Equal(t, "Failed to delete chat project.", chatProjectDeleteFailureMessage(false, false))
	require.Contains(t, chatProjectDeleteFailureMessage(false, true), "already deleted")
	require.NotContains(t, chatProjectDeleteFailureMessage(true, false), "already deleted")
	require.Contains(t, chatProjectDeleteFailureMessage(true, true), "already deleted")
	require.Contains(t, chatProjectDeleteFailureMessage(true, true), "running")
}
