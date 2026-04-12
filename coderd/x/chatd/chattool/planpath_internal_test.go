package chattool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAbsolutePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"/home/coder/PLAN.md", true},
		{`C:\\Users\\coder\\PLAN.md`, true},
		{"C:/Users/coder/PLAN.md", true},
		{`d:\\data\\plan.md`, true},
		{"plan.md", false},
		{"./plan.md", false},
		{"../plan.md", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isAbsolutePath(tt.path))
		})
	}
}

func TestLooksLikePlanFileName(t *testing.T) {
	t.Parallel()

	require.True(t, looksLikePlanFileName("plan.md"))
	require.True(t, looksLikePlanFileName("./Plan.md"))
	require.True(t, looksLikePlanFileName(`C:\\Users\\coder\\PLAN.md`))
	require.True(t, looksLikePlanFileName(`C:\\Users\\coder\\plan.md`))
	require.False(t, looksLikePlanFileName(`C:\\Users\\coder\\README.md`))
}

func TestIsLegacySharedPlanPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested string
		want      bool
	}{
		{
			name:      "ExactMatch",
			requested: "/home/coder/PLAN.md",
			want:      true,
		},
		{
			name:      "DifferentFilename",
			requested: "/home/coder/OTHER.md",
			want:      false,
		},
		{
			name:      "DifferentDirectory",
			requested: "/home/dev/PLAN.md",
			want:      false,
		},
		{
			name:      "PerChatPath",
			requested: "/home/coder/.coder/plans/PLAN-123e4567-e89b-12d3-a456-426614174000.md",
			want:      false,
		},
		{
			name:      "EmptyString",
			requested: "",
			want:      false,
		},
		{
			name:      "SubstringMatch",
			requested: "/home/coder/PLAN.md/extra",
			want:      false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := isLegacySharedPlanPath(testCase.requested)

			require.Equal(t, testCase.want, got)
		})
	}
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
		),
	)
	require.Equal(
		t,
		"the plan path /home/coder/plan.md could not be verified because the workspace is currently unavailable to resolve the chat-specific plan path, try again shortly",
		planPathVerificationMessage("/home/coder/plan.md"),
	)
}
