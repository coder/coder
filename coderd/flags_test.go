package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/testutil"
)

const (
	secretValue = "********"
)

func TestDeploymentFlagSecrets(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	df := deployment.NewFlags()
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentFlags: &df,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	scrubbed, err := client.DeploymentFlags(ctx)
	require.NoError(t, err)
	require.EqualValues(t, secretValue, scrubbed.Oauth2GithubClientSecret.Value)
	require.EqualValues(t, secretValue, scrubbed.OidcClientSecret.Value)
	require.EqualValues(t, secretValue, scrubbed.PostgresURL.Value)
	require.EqualValues(t, secretValue, scrubbed.ScimAuthHeader.Value)
}
