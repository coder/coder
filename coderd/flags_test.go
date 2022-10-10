package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/deployment"
)

const (
	secretValue = "********"
)

func TestDeploymentFlagSecrets(t *testing.T) {
	t.Parallel()

	df := deployment.NewFlags()
	scrubbed := deployment.RemoveSensitiveValues(df)
	require.EqualValues(t, secretValue, scrubbed.Oauth2GithubClientSecret.Value)
	require.EqualValues(t, secretValue, scrubbed.OidcClientSecret.Value)
	require.EqualValues(t, secretValue, scrubbed.PostgresURL.Value)
	require.EqualValues(t, secretValue, scrubbed.ScimAuthHeader.Value)
}
