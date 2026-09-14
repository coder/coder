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
			sink := testutil.NewFakeSink(t)
			db := dbmock.NewMockStore(gomock.NewController(t))
			// The query runs only while the provider is disabled. An
			// unexpected call fails the test.
			if !tc.enabled {
				db.EXPECT().GetOAuth2ProviderApps(gomock.Any()).Return(tc.apps, tc.queryErr)
			}

			cfg := codersdk.OAuth2ProviderConfig{Enable: serpent.Bool(tc.enabled)}
			LogOAuth2ProviderState(ctx, sink.Logger(), db, cfg)

			require.Equal(t, []string{tc.wantInfo}, loggedMessages(sink, slog.LevelInfo),
				"exactly one info line per start")
			require.Equal(t, tc.wantWarn, loggedMessages(sink, slog.LevelWarn))
		})
	}
}

// loggedMessages returns the messages captured at exactly the given level,
// in the order they were logged. It returns nil when nothing was logged so
// it compares equal to an unset expectation.
func loggedMessages(sink *testutil.FakeSink, level slog.Level) []string {
	var messages []string
	for _, e := range sink.Entries(func(e slog.SinkEntry) bool { return e.Level == level }) {
		messages = append(messages, e.Message)
	}
	return messages
}
