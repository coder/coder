package coderd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestLogOAuth2ProviderState(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		enabled  bool
		apps     []database.OAuth2ProviderApp
		queryErr error
		wantInfo string
		wantWarn []string
	}{
		{
			name:     "Enabled",
			enabled:  true,
			wantInfo: "oauth2 provider enabled",
		},
		{
			name:     "DisabledNoApps",
			wantInfo: "oauth2 provider disabled",
		},
		{
			name:     "DisabledWithApps",
			apps:     []database.OAuth2ProviderApp{{}, {}},
			wantInfo: "oauth2 provider disabled",
			wantWarn: []string{fmt.Sprintf(oauth2ProviderDisabledWithAppsMessage, 2)},
		},
		{
			name:     "DisabledQueryError",
			queryErr: xerrors.New("boom"),
			wantInfo: "oauth2 provider disabled",
			wantWarn: []string{"oauth2 provider: list registered applications"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			rec := &logRecorder{}
			db := dbmock.NewMockStore(gomock.NewController(t))
			// The query runs only while the provider is disabled. An
			// unexpected call fails the test.
			if !tc.enabled {
				db.EXPECT().GetOAuth2ProviderApps(gomock.Any()).Return(tc.apps, tc.queryErr)
			}

			cfg := codersdk.OAuth2ProviderConfig{Enable: serpent.Bool(tc.enabled)}
			LogOAuth2ProviderState(ctx, slog.Make(rec), db, cfg)

			require.Equal(t, []string{tc.wantInfo}, rec.messages(slog.LevelInfo),
				"exactly one info line per start")
			require.Equal(t, tc.wantWarn, rec.messages(slog.LevelWarn))
		})
	}
}
