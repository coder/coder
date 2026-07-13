package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestParseSlackConfig(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	slackProvider := &externalauth.Config{
		ID:   "slack",
		Type: string(codersdk.EnhancedExternalAuthProviderSlack),
	}
	githubProvider := &externalauth.Config{
		ID:   "github",
		Type: string(codersdk.EnhancedExternalAuthProviderGitHub),
	}

	values := func(botToken, appToken, owner, providerID string) *codersdk.DeploymentValues {
		vals := &codersdk.DeploymentValues{}
		require.NoError(t, vals.AI.Slack.BotToken.Set(botToken))
		require.NoError(t, vals.AI.Slack.AppToken.Set(appToken))
		require.NoError(t, vals.AI.Slack.ChatOwnerUserID.Set(owner))
		require.NoError(t, vals.AI.Slack.ExternalAuthProviderID.Set(providerID))
		return vals
	}

	cases := []struct {
		name        string
		values      *codersdk.DeploymentValues
		providers   []*externalauth.Config
		wantEnabled bool
		// wantProviderID is asserted only when enabled.
		wantProviderID string
	}{
		{
			name:        "Unconfigured",
			values:      values("", "", "", ""),
			wantEnabled: false,
		},
		{
			name:        "PartialConfigurationDisables",
			values:      values("xoxb-bot", "", ownerID.String(), ""),
			wantEnabled: false,
		},
		{
			name:        "InvalidOwnerUUIDDisables",
			values:      values("xoxb-bot", "xapp-app", "not-a-uuid", ""),
			wantEnabled: false,
		},
		{
			name:           "ProviderIDOmitted",
			values:         values("xoxb-bot", "xapp-app", ownerID.String(), ""),
			providers:      []*externalauth.Config{slackProvider},
			wantEnabled:    true,
			wantProviderID: "",
		},
		{
			name:           "ValidSlackProvider",
			values:         values("xoxb-bot", "xapp-app", ownerID.String(), "slack"),
			providers:      []*externalauth.Config{githubProvider, slackProvider},
			wantEnabled:    true,
			wantProviderID: "slack",
		},
		{
			// External auth config parsing preserves the configured
			// spelling of the type, so matching must ignore case.
			name:   "MixedCaseSlackProviderType",
			values: values("xoxb-bot", "xapp-app", ownerID.String(), "slack-mixed"),
			providers: []*externalauth.Config{{
				ID:   "slack-mixed",
				Type: "Slack",
			}},
			wantEnabled:    true,
			wantProviderID: "slack-mixed",
		},
		{
			name:        "UnknownProviderDisables",
			values:      values("xoxb-bot", "xapp-app", ownerID.String(), "missing"),
			providers:   []*externalauth.Config{slackProvider},
			wantEnabled: false,
		},
		{
			name:        "NonSlackProviderDisables",
			values:      values("xoxb-bot", "xapp-app", ownerID.String(), "github"),
			providers:   []*externalauth.Config{githubProvider, slackProvider},
			wantEnabled: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

			cfg, enabled := parseSlackConfig(ctx, logger, tc.values, tc.providers)
			require.Equal(t, tc.wantEnabled, enabled)
			if !tc.wantEnabled {
				require.Nil(t, cfg)
				return
			}
			require.NotNil(t, cfg)
			require.Equal(t, ownerID, cfg.ownerID)
			require.Equal(t, tc.wantProviderID, cfg.externalAuthProviderID)
		})
	}
}
