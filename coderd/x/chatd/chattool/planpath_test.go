package chattool_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

func TestResolveWorkspaceHome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resp     workspacesdk.LSResponse
		lsErr    error
		want     string
		wantErr  bool
		errMatch string
	}{
		{
			name: "StandardLinuxHome",
			resp: workspacesdk.LSResponse{AbsolutePathString: "/home/coder"},
			want: "/home/coder",
		},
		{
			name: "NonStandardHome",
			resp: workspacesdk.LSResponse{AbsolutePathString: "/Users/dev"},
			want: "/Users/dev",
		},
		{
			name:     "LSError",
			lsErr:    xerrors.New("list failed"),
			wantErr:  true,
			errMatch: "list failed",
		},
		{
			name:     "EmptyAbsolutePathString",
			resp:     workspacesdk.LSResponse{AbsolutePathString: ""},
			wantErr:  true,
			errMatch: "workspace home path is empty",
		},
		{
			name:     "WhitespaceOnlyAbsolutePathString",
			resp:     workspacesdk.LSResponse{AbsolutePathString: " \t\n "},
			wantErr:  true,
			errMatch: "workspace home path is empty",
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			conn := agentconnmock.NewMockAgentConn(ctrl)

			conn.EXPECT().LS(
				gomock.Any(),
				"",
				workspacesdk.LSRequest{
					Path:       []string{},
					Relativity: workspacesdk.LSRelativityHome,
				},
			).Return(testCase.resp, testCase.lsErr)

			got, err := chattool.ResolveWorkspaceHome(context.Background(), conn)
			if testCase.wantErr {
				require.Error(t, err)
				require.ErrorContains(t, err, testCase.errMatch)
				require.Empty(t, got)
				return
			}

			require.NoError(t, err)
			require.Equal(t, testCase.want, got)
		})
	}
}

func TestPlanPathForChat(t *testing.T) {
	t.Parallel()

	t.Run("StandardHome", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")

		got := chattool.PlanPathForChat("/home/coder", chatID)

		require.Equal(
			t,
			"/home/coder/.coder/plans/PLAN-123e4567-e89b-12d3-a456-426614174000.md",
			got,
		)
	})

	t.Run("NonStandardHome", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

		got := chattool.PlanPathForChat("/Users/dev", chatID)

		require.Equal(
			t,
			"/Users/dev/.coder/plans/PLAN-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.md",
			got,
		)
	})

	t.Run("MatchesExpectedFormat", func(t *testing.T) {
		t.Parallel()

		home := "/workspace/home"
		chatID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

		got := chattool.PlanPathForChat(home, chatID)

		require.True(t, strings.HasPrefix(got, home+"/.coder/plans/PLAN-"))
		require.True(t, strings.HasSuffix(got, chatID.String()+".md"))
	})
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
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := chattool.IsLegacySharedPlanPath(testCase.requested)

			require.Equal(t, testCase.want, got)
		})
	}
}
