package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/testutil"
)

func TestDeploymentConfig(t *testing.T) {
	t.Parallel()
	hi := "hi"
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	vip := deployment.NewViper()
	cfg, err := deployment.Config(vip)
	require.NoError(t, err)
	// values should be returned
	cfg.AccessURL = hi
	// values should not be returned
	cfg.OAuth2Github.ClientSecret = hi
	cfg.OIDC.ClientSecret = hi
	cfg.PostgresURL = hi
	cfg.SCIMAuthHeader = hi

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentConfig: &cfg,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	scrubbed, err := client.DeploymentConfig(ctx)
	require.NoError(t, err)
	// ensure normal values pass through
	require.EqualValues(t, hi, scrubbed.AccessURL)
	// ensure secrets are removed
	require.Empty(t, scrubbed.OAuth2Github.ClientSecret)
	require.Empty(t, scrubbed.OIDC.ClientSecret)
	require.Empty(t, scrubbed.PostgresURL)
	require.Empty(t, scrubbed.SCIMAuthHeader)
}
