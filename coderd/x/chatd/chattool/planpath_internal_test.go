package chattool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestIsAbsolutePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"/home/coder/PLAN.md", true},
		{"/workspace/project/plan.md", true},
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
	require.True(t, looksLikePlanFileName("/home/coder/PLAN.md"))
	require.False(t, looksLikePlanFileName("/home/coder/README.md"))
}

func TestLooksLikeLegacySharedPlanPath(t *testing.T) {
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
			name:      "CaseInsensitive",
			requested: "/home/coder/plan.md",
			want:      true,
		},
		{
			name:      "MixedCase",
			requested: "/home/coder/Plan.md",
			want:      true,
		},
		{
			name:      "NestedPath",
			requested: "/home/coder/myproject/plan.md",
			want:      false,
		},
		{
			name:      "DifferentHome",
			requested: "/Users/dev/PLAN.md",
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
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, testCase.want, looksLikeLegacySharedPlanPath(testCase.requested))
		})
	}
}

func TestRejectSharedPlanPath(t *testing.T) {
	t.Parallel()

	resp, rejected := rejectSharedPlanPath(
		LegacySharedPlanPath,
		"/Users/dev",
		"/Users/dev/.coder/plans/PLAN-chat.md",
		nil,
	)

	require.True(t, rejected)
	require.True(t, resp.IsError)
	require.Equal(
		t,
		sharedPlanPathMessage(
			LegacySharedPlanPath,
			"/Users/dev/.coder/plans/PLAN-chat.md",
		),
		resp.Content,
	)
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

func TestValidatePlanPath(t *testing.T) {
	t.Parallel()

	const (
		home     = "/home/coder"
		chatPath = "/home/coder/.coder/plans/PLAN-chat.md"
	)

	okResolver := func(context.Context) (string, string, error) {
		return chatPath, home, nil
	}
	errResolver := func(context.Context) (string, string, error) {
		return "", "", xerrors.New("workspace unavailable")
	}

	tests := []struct {
		name           string
		requestedPath  string
		resolver       ResolveChatPlanPath
		wantRejected   bool
		wantContentSub string
	}{
		{
			name:          "NonPlanFileNoOp",
			requestedPath: "/workspace/src/main.go",
			resolver:      okResolver,
			wantRejected:  false,
		},
		{
			name:           "PlanFileRelativePathRejected",
			requestedPath:  "plan.md",
			resolver:       okResolver,
			wantRejected:   true,
			wantContentSub: "plan files must use absolute paths",
		},
		{
			name:           "PlanFileDotRelativeRejected",
			requestedPath:  "./PLAN.md",
			resolver:       okResolver,
			wantRejected:   true,
			wantContentSub: "plan files must use absolute paths",
		},
		{
			name:           "PlanFileInHomeRejected",
			requestedPath:  "/home/coder/plan.md",
			resolver:       okResolver,
			wantRejected:   true,
			wantContentSub: "use the chat-specific plan path",
		},
		{
			name:          "ChatSpecificPathAccepted",
			requestedPath: chatPath,
			resolver:      okResolver,
			wantRejected:  false,
		},
		{
			name:          "NoResolverNonPlanAccepted",
			requestedPath: "/workspace/src/main.go",
			resolver:      nil,
			wantRejected:  false,
		},
		{
			name:           "NoResolverPlanFileRelativeRejected",
			requestedPath:  "plan.md",
			resolver:       nil,
			wantRejected:   true,
			wantContentSub: "plan files must use absolute paths",
		},
		{
			name:          "NoResolverAbsolutePlanFileAccepted",
			requestedPath: "/home/coder/plan.md",
			resolver:      nil,
			wantRejected:  false,
		},
		{
			name:           "ResolverErrorWithLegacyPathRejected",
			requestedPath:  LegacySharedPlanPath,
			resolver:       errResolver,
			wantRejected:   true,
			wantContentSub: "could not be verified",
		},
		{
			name:          "ResolverErrorWithNonLegacyPathAccepted",
			requestedPath: "/some/other/plan.md",
			resolver:      errResolver,
			wantRejected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, rejected := validatePlanPath(t.Context(), tt.requestedPath, tt.resolver)
			require.Equal(t, tt.wantRejected, rejected)
			if tt.wantRejected {
				require.True(t, resp.IsError)
				require.Contains(t, resp.Content, tt.wantContentSub)
			}
		})
	}
}

func TestMemoizedPlanPathResolver(t *testing.T) {
	t.Parallel()

	t.Run("NilReturnsNil", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, memoizedPlanPathResolver(nil))
	})

	t.Run("CachesResult", func(t *testing.T) {
		t.Parallel()
		var calls int
		base := func(context.Context) (string, string, error) {
			calls++
			return "/chat/plan.md", "/home/coder", nil
		}
		memo := memoizedPlanPathResolver(base)
		require.NotNil(t, memo)

		for range 5 {
			chatPath, home, err := memo(t.Context())
			require.NoError(t, err)
			require.Equal(t, "/chat/plan.md", chatPath)
			require.Equal(t, "/home/coder", home)
		}
		require.Equal(t, 1, calls, "underlying resolver should only run once")
	})

	t.Run("CachesError", func(t *testing.T) {
		t.Parallel()
		var calls int
		sentinel := xerrors.New("boom")
		base := func(context.Context) (string, string, error) {
			calls++
			return "", "", sentinel
		}
		memo := memoizedPlanPathResolver(base)

		_, _, err1 := memo(t.Context())
		_, _, err2 := memo(t.Context())
		require.ErrorIs(t, err1, sentinel)
		require.ErrorIs(t, err2, sentinel)
		require.Equal(t, 1, calls)
	})
}
