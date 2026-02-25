package chattool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractGitAuthRequiredMarker(t *testing.T) {
	t.Parallel()

	output := "" +
		"fatal: could not read Username for 'https://github.com': terminal prompts disabled\n" +
		"CODER_GITAUTH_REQUIRED:{\"provider_id\":\"github\",\"provider_type\":\"github\",\"provider_display_name\":\"GitHub\",\"authenticate_url\":\"https://coder.example.com/external-auth/github\",\"host\":\"https://github.com\"}\n" +
		"fatal: Authentication failed\n"

	marker, cleaned := extractGitAuthRequiredMarker(output)
	require.NotNil(t, marker)
	require.Equal(t, "github", marker.ProviderID)
	require.Equal(t, "https://coder.example.com/external-auth/github", marker.AuthenticateURL)
	require.NotContains(t, cleaned, gitAuthRequiredPrefix)
	require.Contains(t, cleaned, "fatal: Authentication failed")
}
