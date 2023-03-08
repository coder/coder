package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/testutil"
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
	cfg.PostgresURL.Set(hi)
	cfg.SCIMAPIKey.Set(hi)

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: cfg,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	scrubbed, err := client.DeploymentValues(ctx)
	require.NoError(t, err)
	// ensure normal values pass through
	require.EqualValues(t, true, scrubbed.Values.BrowserOnly.Value())
	// ensure secrets are removed
	require.Empty(t, scrubbed.Values.OAuth2.Github.ClientSecret.Value())
	require.Empty(t, scrubbed.Values.OIDC.ClientSecret.Value())
	require.Empty(t, scrubbed.Values.PostgresURL.Value())
	require.Empty(t, scrubbed.Values.SCIMAPIKey.Value())
}
