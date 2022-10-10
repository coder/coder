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
	hi := "hi"
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	df := deployment.Flags()
	// check if copy works for non-secret values
	df.AccessURL.Value = hi
	// check if secrets are removed
	df.OAuth2GithubClientSecret.Value = hi
	df.OIDCClientSecret.Value = hi
	df.PostgresURL.Value = hi
	df.SCIMAuthHeader.Value = hi

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentFlags: &df,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	scrubbed, err := client.DeploymentFlags(ctx)
	require.NoError(t, err)
	// ensure df is unchanged
	require.EqualValues(t, hi, df.OAuth2GithubClientSecret.Value)
	// ensure normal values pass through
	require.EqualValues(t, hi, scrubbed.AccessURL.Value)
	// ensure secrets are removed
	require.EqualValues(t, secretValue, scrubbed.OAuth2GithubClientSecret.Value)
	require.EqualValues(t, secretValue, scrubbed.OIDCClientSecret.Value)
	require.EqualValues(t, secretValue, scrubbed.PostgresURL.Value)
	require.EqualValues(t, secretValue, scrubbed.SCIMAuthHeader.Value)
}
