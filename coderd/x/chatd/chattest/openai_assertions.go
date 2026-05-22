package chattest

import (
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"
)

// RequireOpenAIStoreDisabled asserts that OpenAI provider options disable store
// and do not carry previous_response_id state.
func RequireOpenAIStoreDisabled(t *testing.T, opts fantasy.ProviderOptions) {
	t.Helper()

	entry, ok := opts[fantasyopenai.Name]
	require.True(t, ok)
	switch options := entry.(type) {
	case *fantasyopenai.ResponsesProviderOptions:
		require.NotNil(t, options.Store)
		require.False(t, *options.Store)
		require.Nil(t, options.PreviousResponseID)
	case *fantasyopenai.ProviderOptions:
		require.NotNil(t, options.Store)
		require.False(t, *options.Store)
	default:
		require.Failf(t, "unexpected OpenAI options type", "%T", entry)
	}
}
