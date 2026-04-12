package chattool

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestLooksLikePlanFileName(t *testing.T) {
	t.Parallel()

	require.True(t, looksLikePlanFileName("plan.md"))
	require.True(t, looksLikePlanFileName("./Plan.md"))
	require.True(t, looksLikePlanFileName(`C:\\Users\\coder\\PLAN.md`))
	require.True(t, looksLikePlanFileName(`C:\\Users\\coder\\plan.md`))
	require.False(t, looksLikePlanFileName(`C:\\Users\\coder\\README.md`))
}

func TestLooksLikeLegacyHomePlanPath(t *testing.T) {
	t.Parallel()

	require.True(t, looksLikeLegacyHomePlanPath(LegacySharedPlanPath, ""))
	require.True(t, looksLikeLegacyHomePlanPath("/home/coder/plan.md", ""))
	require.False(t, looksLikeLegacyHomePlanPath("/home/coder/notes.md", ""))
}

func TestSharedPlanPathMessage(t *testing.T) {
	t.Parallel()

	require.Equal(
		t,
		"the plan path /home/coder/plan.md is no longer supported at the home root; use the chat-specific plan path: /home/coder/.coder/plans/PLAN-chat.md",
		sharedPlanPathMessage(
			"/home/coder/plan.md",
			"/home/coder/.coder/plans/PLAN-chat.md",
			nil,
		),
	)
	require.Equal(
		t,
		"the plan path /home/coder/plan.md is no longer supported at the home root; the workspace is currently unavailable to resolve the chat-specific plan path, try again shortly",
		sharedPlanPathMessage(
			"/home/coder/plan.md",
			"",
			xerrors.New("workspace unavailable"),
		),
	)
}
