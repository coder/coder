package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAISandboxActivityChannel(t *testing.T) {
	t.Parallel()

	channel := workspaceAISandboxActivityChannel(uuid.New())
	require.LessOrEqual(t, len(channel), 63)
}
