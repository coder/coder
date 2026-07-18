package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOAuth2ProviderSettings(t *testing.T) {
	t.Parallel()

	t.Run("DefaultDisabled", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		settings, err := client.OAuth2ProviderSettings(ctx)
		require.NoError(t, err)
		require.False(t, settings.DynamicClientRegistrationEnabled)
	})

	t.Run("RoundTrip", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		updated, err := client.PutOAuth2ProviderSettings(ctx, codersdk.OAuth2ProviderSettings{
			DynamicClientRegistrationEnabled: false,
		})
		require.NoError(t, err)
		require.False(t, updated.DynamicClientRegistrationEnabled)

		settings, err := client.OAuth2ProviderSettings(ctx)
		require.NoError(t, err)
		require.False(t, settings.DynamicClientRegistrationEnabled)

		updated, err = client.PutOAuth2ProviderSettings(ctx, codersdk.OAuth2ProviderSettings{
			DynamicClientRegistrationEnabled: true,
		})
		require.NoError(t, err)
		require.True(t, updated.DynamicClientRegistrationEnabled)
	})

	t.Run("PermissionDenied", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			do   func(ctx context.Context, client *codersdk.Client) error
		}{
			{
				name: "Get",
				do: func(ctx context.Context, client *codersdk.Client) error {
					_, err := client.OAuth2ProviderSettings(ctx)
					return err
				},
			},
			{
				name: "Put",
				do: func(ctx context.Context, client *codersdk.Client) error {
					_, err := client.PutOAuth2ProviderSettings(ctx, codersdk.OAuth2ProviderSettings{
						DynamicClientRegistrationEnabled: false,
					})
					return err
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				client := coderdtest.New(t, nil)
				firstUser := coderdtest.CreateFirstUser(t, client)
				anotherClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
				ctx := testutil.Context(t, testutil.WaitShort)

				err := tt.do(ctx, anotherClient)
				var sdkError *codersdk.Error
				require.Error(t, err)
				require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
				require.Equal(t, http.StatusForbidden, sdkError.StatusCode())
			})
		}
	})
}
