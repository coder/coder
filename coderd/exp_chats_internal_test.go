package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

func TestShouldCleanUnboundModelsAfterProviderDelete(t *testing.T) {
	t.Parallel()

	deletedProvider := database.ChatProvider{
		ID:       uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Provider: "openai",
		Enabled:  true,
	}

	t.Run("LastEnabledProviderWithoutFallback", func(t *testing.T) {
		t.Parallel()

		require.True(t, shouldCleanUnboundModelsAfterProviderDelete(
			deletedProvider,
			1,
			[]database.ChatProvider{deletedProvider},
			chatprovider.ProviderAPIKeys{},
		))
	})

	t.Run("DeploymentFallbackKeepsFamilyRunnable", func(t *testing.T) {
		t.Parallel()

		require.False(t, shouldCleanUnboundModelsAfterProviderDelete(
			deletedProvider,
			0,
			[]database.ChatProvider{deletedProvider},
			chatprovider.ProviderAPIKeys{
				ByProvider: map[string]string{"openai": "env-key"},
			},
		))
	})

	t.Run("DeletingDisabledProviderWithDisabledSiblingDoesNotClean", func(t *testing.T) {
		t.Parallel()

		disabledProvider := deletedProvider
		disabledProvider.Enabled = false

		require.False(t, shouldCleanUnboundModelsAfterProviderDelete(
			disabledProvider,
			1,
			[]database.ChatProvider{},
			chatprovider.ProviderAPIKeys{},
		))
	})
}
