package coderd_test

import (
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestOAuthApps(t *testing.T) {
	t.Parallel()

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureOAuth2Provider: 1,
			},
		}})

		ctx := testutil.Context(t, testutil.WaitLong)

		tests := []struct {
			name string
			req  codersdk.PostOAuth2ProviderAppRequest
		}{
			{
				name: "NameMissing",
				req: codersdk.PostOAuth2ProviderAppRequest{
					CallbackURL: "http://localhost:3000",
				},
			},
			{
				name: "NameSpaces",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo bar",
					CallbackURL: "http://localhost:3000",
				},
			},
			{
				name: "NameTooLong",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "too loooooooooooooooooooooooooong",
					CallbackURL: "http://localhost:3000",
				},
			},
			{
				name: "NameTaken",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "taken",
					CallbackURL: "http://localhost:3000",
				},
			},
			{
				name: "URLMissing",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name: "foo",
				},
			},
			{
				name: "URLLocalhostNoScheme",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo",
					CallbackURL: "localhost:3000",
				},
			},
			{
				name: "URLNoScheme",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo",
					CallbackURL: "coder.com",
				},
			},
			{
				name: "URLNoColon",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo",
					CallbackURL: "http//coder",
				},
			},
			{
				name: "URLJustBar",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo",
					CallbackURL: "bar",
				},
			},
			{
				name: "URLPathOnly",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo",
					CallbackURL: "/bar/baz/qux",
				},
			},
			{
				name: "URLJustHttp",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo",
					CallbackURL: "http",
				},
			},
			{
				name: "URLNoHost",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo",
					CallbackURL: "http://",
				},
			},
			{
				name: "URLSpaces",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo",
					CallbackURL: "bar baz qux",
				},
			},
		}

		// Generate an application for testing name conflicts.
		req := codersdk.PostOAuth2ProviderAppRequest{
			Name:        "taken",
			CallbackURL: "http://coder.com",
		}
		//nolint:gocritic // OAauth2 app management requires owner permission.
		_, err := client.PostOAuth2ProviderApp(ctx, req)
		require.NoError(t, err)

		// Generate an application for testing PUTs.
		req = codersdk.PostOAuth2ProviderAppRequest{
			Name:        "quark",
			CallbackURL: "http://coder.com",
		}
		//nolint:gocritic // OAauth2 app management requires owner permission.
		existingApp, err := client.PostOAuth2ProviderApp(ctx, req)
		require.NoError(t, err)

		for _, test := range tests {
			test := test
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				//nolint:gocritic // OAauth2 app management requires owner permission.
				_, err := client.PostOAuth2ProviderApp(ctx, test.req)
				require.Error(t, err)

				//nolint:gocritic // OAauth2 app management requires owner permission.
				_, err = client.PutOAuth2ProviderApp(ctx, existingApp.ID, codersdk.PutOAuth2ProviderAppRequest{
					Name:        test.req.Name,
					CallbackURL: test.req.CallbackURL,
				})
				require.Error(t, err)
			})
		}
	})

	t.Run("DeleteNonExisting", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureOAuth2Provider: 1,
			},
		}})

		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAauth2 app management requires owner permission.
		_, err := client.OAuth2ProviderApp(ctx, uuid.New())
		require.Error(t, err)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureOAuth2Provider: 1,
			},
		}})

		ctx := testutil.Context(t, testutil.WaitLong)

		// No apps yet.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		apps, err := client.OAuth2ProviderApps(ctx)
		require.NoError(t, err)
		require.Len(t, apps, 0)

		// Should be able to add apps.
		expected := []codersdk.OAuth2ProviderApp{}
		for i := 0; i < 5; i++ {
			postReq := codersdk.PostOAuth2ProviderAppRequest{
				Name:        "foo-" + strconv.Itoa(i),
				CallbackURL: "http://" + strconv.Itoa(i) + ".localhost:3000",
			}
			//nolint:gocritic // OAauth2 app management requires owner permission.
			app, err := client.PostOAuth2ProviderApp(ctx, postReq)
			require.NoError(t, err)
			require.Equal(t, postReq.Name, app.Name)
			require.Equal(t, postReq.CallbackURL, app.CallbackURL)
			expected = append(expected, app)
		}

		// Should get all the apps now.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		apps, err = client.OAuth2ProviderApps(ctx)
		require.NoError(t, err)
		require.Len(t, apps, 5)
		require.Equal(t, expected, apps)

		// Should be able to keep the same name when updating.
		req := codersdk.PutOAuth2ProviderAppRequest{
			Name:        expected[0].Name,
			CallbackURL: "http://coder.com",
			Icon:        "test",
		}
		//nolint:gocritic // OAauth2 app management requires owner permission.
		newApp, err := client.PutOAuth2ProviderApp(ctx, expected[0].ID, req)
		require.NoError(t, err)
		require.Equal(t, req.Name, newApp.Name)
		require.Equal(t, req.CallbackURL, newApp.CallbackURL)
		require.Equal(t, req.Icon, newApp.Icon)
		require.Equal(t, expected[0].ID, newApp.ID)

		// Should be able to update name.
		req = codersdk.PutOAuth2ProviderAppRequest{
			Name:        "new-foo",
			CallbackURL: "http://coder.com",
			Icon:        "test",
		}
		//nolint:gocritic // OAauth2 app management requires owner permission.
		newApp, err = client.PutOAuth2ProviderApp(ctx, expected[0].ID, req)
		require.NoError(t, err)
		require.Equal(t, req.Name, newApp.Name)
		require.Equal(t, req.CallbackURL, newApp.CallbackURL)
		require.Equal(t, req.Icon, newApp.Icon)
		require.Equal(t, expected[0].ID, newApp.ID)

		// Should be able to get a single app.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		got, err := client.OAuth2ProviderApp(ctx, expected[0].ID)
		require.NoError(t, err)
		require.Equal(t, newApp, got)

		// Should be able to delete an app.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderApp(ctx, expected[0].ID)
		require.NoError(t, err)

		// Should show the new count.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		newApps, err := client.OAuth2ProviderApps(ctx)
		require.NoError(t, err)
		require.Len(t, newApps, 4)
		require.Equal(t, expected[1:], newApps)
	})
}

func TestOAuthAppSecrets(t *testing.T) {
	t.Parallel()

	client, _ := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
		Features: license.Features{
			codersdk.FeatureOAuth2Provider: 1,
		},
	}})

	ctx := testutil.Context(t, testutil.WaitLong)

	// Make some apps.
	//nolint:gocritic // OAauth2 app management requires owner permission.
	app1, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
		Name:        "razzle-dazzle",
		CallbackURL: "http://localhost",
	})
	require.NoError(t, err)

	//nolint:gocritic // OAauth2 app management requires owner permission.
	app2, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
		Name:        "razzle-dazzle-the-sequel",
		CallbackURL: "http://localhost",
	})
	require.NoError(t, err)

	t.Run("DeleteNonExisting", func(t *testing.T) {
		t.Parallel()

		// Should not be able to create secrets for a non-existent app.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		_, err = client.OAuth2ProviderAppSecrets(ctx, uuid.New())
		require.Error(t, err)

		// Should not be able to delete non-existing secrets when there is no app.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderAppSecret(ctx, uuid.New(), uuid.New())
		require.Error(t, err)

		// Should not be able to delete non-existing secrets when the app exists.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderAppSecret(ctx, app1.ID, uuid.New())
		require.Error(t, err)

		// Should not be able to delete an existing secret with the wrong app ID.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		secret, err := client.PostOAuth2ProviderAppSecret(ctx, app2.ID)
		require.NoError(t, err)

		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderAppSecret(ctx, app1.ID, secret.ID)
		require.Error(t, err)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		// No secrets yet.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		secrets, err := client.OAuth2ProviderAppSecrets(ctx, app1.ID)
		require.NoError(t, err)
		require.Len(t, secrets, 0)

		// Should be able to create secrets.
		for i := 0; i < 5; i++ {
			//nolint:gocritic // OAauth2 app management requires owner permission.
			secret, err := client.PostOAuth2ProviderAppSecret(ctx, app1.ID)
			require.NoError(t, err)
			require.NotEmpty(t, secret.ClientSecretFull)
			require.True(t, len(secret.ClientSecretFull) > 6)

			//nolint:gocritic // OAauth2 app management requires owner permission.
			_, err = client.PostOAuth2ProviderAppSecret(ctx, app2.ID)
			require.NoError(t, err)
		}

		// Should get secrets now, but only for the one app.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		secrets, err = client.OAuth2ProviderAppSecrets(ctx, app1.ID)
		require.NoError(t, err)
		require.Len(t, secrets, 5)
		for _, secret := range secrets {
			require.Len(t, secret.ClientSecretTruncated, 6)
		}

		// Should be able to delete a secret.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderAppSecret(ctx, app1.ID, secrets[0].ID)
		require.NoError(t, err)
		secrets, err = client.OAuth2ProviderAppSecrets(ctx, app1.ID)
		require.NoError(t, err)
		require.Len(t, secrets, 4)

		// No secrets once the app is deleted.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderApp(ctx, app1.ID)
		require.NoError(t, err)

		//nolint:gocritic // OAauth2 app management requires owner permission.
		_, err = client.OAuth2ProviderAppSecrets(ctx, app1.ID)
		require.Error(t, err)
	})
}
