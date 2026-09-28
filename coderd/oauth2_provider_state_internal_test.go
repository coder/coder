package coderd

import (
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

	flagField := slog.F("flag", "CODER_OAUTH2_PROVIDER_ENABLE")
	queryErr := xerrors.New("boom")

	for _, tc := range []struct {
		name     string
		enabled  bool
		apps     []database.OAuth2ProviderApp
		queryErr error
		wantInfo string
		// wantWarn lists the expected warn entries in order. Only the
		// message and fields are compared.
		wantWarn []slog.SinkEntry
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
			wantWarn: []slog.SinkEntry{{
				Message: oauth2ProviderDisabledWithAppsMessage,
				Fields:  slog.M(flagField, slog.F("count", 2)),
			}},
		},
		{
			name:     "DisabledQueryError",
			queryErr: queryErr,
			wantInfo: "oauth2 provider disabled",
			wantWarn: []slog.SinkEntry{{
				Message: "oauth2 provider: list registered applications",
				Fields:  slog.M(slog.Error(queryErr)),
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			sink := testutil.NewFakeSink(t)
			db := dbmock.NewMockStore(gomock.NewController(t))
			// The query runs only while the provider is disabled. An
			// unexpected call fails the test.
			if !tc.enabled {
				db.EXPECT().GetOAuth2ProviderApps(gomock.Any()).Return(tc.apps, tc.queryErr)
			}

			cfg := codersdk.OAuth2ProviderConfig{Enable: serpent.Bool(tc.enabled)}
			LogOAuth2ProviderState(ctx, sink.Logger(), db, cfg)

			infos := loggedEntries(sink, slog.LevelInfo)
			require.Len(t, infos, 1, "exactly one info line per start")
			require.Equal(t, tc.wantInfo, infos[0].Message)
			require.Equal(t, slog.M(flagField), infos[0].Fields,
				"the info line must record which flag controls the provider")

			warns := loggedEntries(sink, slog.LevelWarn)
			require.Len(t, warns, len(tc.wantWarn))
			for i, want := range tc.wantWarn {
				require.Equal(t, want.Message, warns[i].Message)
				require.Equal(t, want.Fields, warns[i].Fields)
			}
		})
	}
}

// loggedEntries returns the entries captured at exactly the given level, in
// the order they were logged.
func loggedEntries(sink *testutil.FakeSink, level slog.Level) []slog.SinkEntry {
	return sink.Entries(func(e slog.SinkEntry) bool { return e.Level == level })
}
