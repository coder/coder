//go:build !slim

package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestResolveSetupToken(t *testing.T) {
	t.Parallel()

	const supplied = "0123456789abcdefghijklmn"

	tests := []struct {
		name     string
		supplied string
		onTunnel bool
		// users is the user count the database reports. It is nil when
		// the database must not be asked.
		users      *int64
		wantToken  func(t *testing.T, token string)
		wantStdout string
		wantStderr string
		wantErr    string
	}{
		{
			name:     "TunnelWithoutUsersGeneratesAndPrintsToken",
			onTunnel: true,
			users:    ptrInt64(0),
			wantToken: func(t *testing.T, token string) {
				require.Len(t, token, 32)
			},
			wantStdout: "Setup token: ",
		},
		{
			name:      "TunnelWithUsersRequiresNoToken",
			onTunnel:  true,
			users:     ptrInt64(1),
			wantToken: requireNoToken,
		},
		{
			name:     "TunnelWithSuppliedTokenUsesItWithoutPrintingIt",
			supplied: supplied,
			onTunnel: true,
			users:    ptrInt64(0),
			wantToken: func(t *testing.T, token string) {
				require.Equal(t, supplied, token)
			},
			wantStdout: "It was set with --setup-token or CODER_SETUP_TOKEN.",
		},
		{
			name:      "AccessURLRequiresNoToken",
			wantToken: requireNoToken,
		},
		{
			name:       "AccessURLIgnoresSuppliedTokenWithWarning",
			supplied:   supplied,
			wantToken:  requireNoToken,
			wantStderr: "run coder server create-admin-user",
		},
		{
			name:     "TunnelWithoutUsersRefusesShortSuppliedToken",
			supplied: "tooshort",
			onTunnel: true,
			users:    ptrInt64(0),
			wantErr:  "--setup-token must be at least 24 characters",
		},
		{
			name:       "AccessURLIgnoresShortSuppliedTokenWithWarning",
			supplied:   "tooshort",
			wantToken:  requireNoToken,
			wantStderr: "--setup-token is ignored because an access URL is set.",
		},
		{
			name:      "TunnelWithUsersIgnoresShortSuppliedToken",
			supplied:  "tooshort",
			onTunnel:  true,
			users:     ptrInt64(1),
			wantToken: requireNoToken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := dbmock.NewMockStore(gomock.NewController(t))
			if tt.users != nil {
				db.EXPECT().GetUserCount(gomock.Any(), false).Return(*tt.users, nil)
			}
			var stdout, stderr bytes.Buffer
			inv := &serpent.Invocation{Stdout: &stdout, Stderr: &stderr}

			token, err := resolveSetupToken(testutil.Context(t, testutil.WaitShort), inv, db, tt.supplied, tt.onTunnel)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			tt.wantToken(t, token)

			if tt.wantStdout == "" {
				require.Empty(t, stdout.String())
			} else {
				require.Contains(t, stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr == "" {
				require.Empty(t, stderr.String())
			} else {
				require.Contains(t, stderr.String(), tt.wantStderr)
			}
			if tt.supplied != "" {
				require.NotContains(t, stdout.String()+stderr.String(), tt.supplied)
			} else if token != "" {
				require.Contains(t, stdout.String(), "Setup token: "+token)
			}
		})
	}

	t.Run("EachStartGeneratesADifferentToken", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetUserCount(gomock.Any(), false).Return(int64(0), nil).Times(2)
		ctx := testutil.Context(t, testutil.WaitShort)

		var tokens []string
		for range 2 {
			var out bytes.Buffer
			token, err := resolveSetupToken(ctx, &serpent.Invocation{Stdout: &out, Stderr: &out}, db, "", true)
			require.NoError(t, err)
			tokens = append(tokens, token)
		}
		require.NotEqual(t, tokens[0], tokens[1])
	})
}

func requireNoToken(t *testing.T, token string) {
	require.Empty(t, token)
}

func ptrInt64(v int64) *int64 {
	return &v
}
