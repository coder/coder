package chattool_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

func TestToolCallID(t *testing.T) {
	t.Parallel()

	// The agent keys runs by this ID, so a changed value would run
	// retried calls again.
	chatID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	require.Equal(t, uuid.MustParse("ca92e3e7-ee42-5889-b1df-75869f9117ee"), chattool.ToolCallID(chatID, 42, "call_1"))
}
