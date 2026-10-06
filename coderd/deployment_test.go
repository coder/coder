package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestDeploymentValues(t *testing.T) {
	t.Parallel()
	hi := "hi"
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	cfg := coderdtest.DeploymentValues(t)
	// values should be returned
	cfg.BrowserOnly = true
	// values should not be returned
	cfg.OAuth2.Github.ClientSecret.Set(hi)
	cfg.OIDC.ClientSecret.Set(hi)
	cfg.OIDC.AuthURLParams.Set(`{"foo":"bar"}`)
	cfg.OIDC.EmailField.Set("some_random_field_you_never_expected")
	cfg.PostgresURL.Set(hi)
	cfg.SCIMAPIKey.Set(hi)
	cfg.ExternalTokenEncryptionKeys.Set("the_random_key_we_never_expected,an_other_key_we_never_unexpected")
	cfg.Provisioner.DaemonPSK = "provisionersftw"

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: cfg,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	scrubbed, err := client.DeploymentConfig(ctx)
	require.NoError(t, err)
	// ensure normal values pass through
	require.EqualValues(t, true, scrubbed.Values.BrowserOnly.Value())
	require.NotEmpty(t, cfg.OIDC.AuthURLParams)
	require.EqualValues(t, cfg.OIDC.AuthURLParams, scrubbed.Values.OIDC.AuthURLParams)
	require.NotEmpty(t, cfg.OIDC.EmailField)
	require.EqualValues(t, cfg.OIDC.EmailField, scrubbed.Values.OIDC.EmailField)
	// ensure secrets are removed
	require.Empty(t, scrubbed.Values.OAuth2.Github.ClientSecret.Value())
	require.Empty(t, scrubbed.Values.OIDC.ClientSecret.Value())
	require.Empty(t, scrubbed.Values.PostgresURL.Value())
	require.Empty(t, scrubbed.Values.SCIMAPIKey.Value())
	require.Empty(t, scrubbed.Values.ExternalTokenEncryptionKeys.Value())
	require.Empty(t, scrubbed.Values.Provisioner.DaemonPSK.Value())
}

func TestDeploymentStats(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	dbgen.WorkspaceAgentStat(t, db, database.WorkspaceAgentStat{
		ConnectionMedianLatencyMS: 10,
		SessionCounts:             dbgen.SessionCounts(t, map[string]int64{"vscode": 1, "cursor": 2, "future_ide": 3}),
	})
	client := coderdtest.New(t, &coderdtest.Options{Database: db})
	_ = coderdtest.CreateFirstUser(t, client)
	var stats codersdk.DeploymentStats
	require.True(t, testutil.Eventually(ctx, t, func(tctx context.Context) bool {
		var err error
		stats, err = client.DeploymentStats(tctx)
		return err == nil
	}, testutil.IntervalMedium), "failed to get deployment stats in time")
	// The legacy total folds both known names into VS Code.
	require.Equal(t, codersdk.SessionCountDeploymentStats{
		Apps: map[string]codersdk.SessionCountApp{
			"vscode":     {Count: 1, DisplayName: "VS Code", Icon: "/icon/code.svg", Family: codersdk.AppFamilyVSCode},
			"cursor":     {Count: 2, DisplayName: "Cursor", Icon: "/icon/cursor.svg", Family: codersdk.AppFamilyVSCode},
			"future_ide": {Count: 3, DisplayName: "future_ide", Family: codersdk.AppFamilyUnknown},
		},
		VSCode: 3,
	}, stats.SessionCount)
}
