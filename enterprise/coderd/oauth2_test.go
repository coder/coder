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

func TestOAuth2ProviderApps(t *testing.T) {
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

		client, owner := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureOAuth2Provider: 1,
			},
		}})
		another, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := another.OAuth2ProviderApp(ctx, uuid.New())
		require.Error(t, err)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client, owner := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureOAuth2Provider: 1,
			},
		}})
		another, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)

		// No apps yet.
		apps, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{})
		require.NoError(t, err)
		require.Len(t, apps, 0)

		// Should be able to add apps.
		expected := generateApps(ctx, t, client, "get-apps")
		expectedOrder := []codersdk.OAuth2ProviderApp{
			expected.Default, expected.NoPort, expected.Subdomain,
			expected.Extra[0], expected.Extra[1],
		}

		// Should get all the apps now.
		apps, err = another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{})
		require.NoError(t, err)
		require.Len(t, apps, 5)
		require.Equal(t, expectedOrder, apps)

		// Should be able to keep the same name when updating.
		req := codersdk.PutOAuth2ProviderAppRequest{
			Name:        expected.Default.Name,
			CallbackURL: "http://coder.com",
			Icon:        "test",
		}
		//nolint:gocritic // OAauth2 app management requires owner permission.
		newApp, err := client.PutOAuth2ProviderApp(ctx, expected.Default.ID, req)
		require.NoError(t, err)
		require.Equal(t, req.Name, newApp.Name)
		require.Equal(t, req.CallbackURL, newApp.CallbackURL)
		require.Equal(t, req.Icon, newApp.Icon)
		require.Equal(t, expected.Default.ID, newApp.ID)

		// Should be able to update name.
		req = codersdk.PutOAuth2ProviderAppRequest{
			Name:        "new-foo",
			CallbackURL: "http://coder.com",
			Icon:        "test",
		}
		//nolint:gocritic // OAauth2 app management requires owner permission.
		newApp, err = client.PutOAuth2ProviderApp(ctx, expected.Default.ID, req)
		require.NoError(t, err)
		require.Equal(t, req.Name, newApp.Name)
		require.Equal(t, req.CallbackURL, newApp.CallbackURL)
		require.Equal(t, req.Icon, newApp.Icon)
		require.Equal(t, expected.Default.ID, newApp.ID)

		// Should be able to get a single app.
		got, err := another.OAuth2ProviderApp(ctx, expected.Default.ID)
		require.NoError(t, err)
		require.Equal(t, newApp, got)

		// Should be able to delete an app.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderApp(ctx, expected.Default.ID)
		require.NoError(t, err)

		// Should show the new count.
		newApps, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{})
		require.NoError(t, err)
		require.Len(t, newApps, 4)

		require.Equal(t, expectedOrder[1:], newApps)
	})

	t.Run("ByUser", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureOAuth2Provider: 1,
			},
		}})
		another, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)
		_ = generateApps(ctx, t, client, "by-user")
		apps, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			UserID: user.ID,
		})
		require.NoError(t, err)
		require.Len(t, apps, 0)
	})
}

func TestOAuth2ProviderAppSecrets(t *testing.T) {
	t.Parallel()

	client, _ := coderdenttest.New(t, &coderdenttest.Options{LicenseOptions: &coderdenttest.LicenseOptions{
		Features: license.Features{
			codersdk.FeatureOAuth2Provider: 1,
		},
	}})

	ctx := testutil.Context(t, testutil.WaitLong)

	// Make some apps.
	apps := generateApps(ctx, t, client, "app-secrets")

	t.Run("DeleteNonExisting", func(t *testing.T) {
		t.Parallel()

		// Should not be able to create secrets for a non-existent app.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		_, err := client.OAuth2ProviderAppSecrets(ctx, uuid.New())
		require.Error(t, err)

		// Should not be able to delete non-existing secrets when there is no app.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderAppSecret(ctx, uuid.New(), uuid.New())
		require.Error(t, err)

		// Should not be able to delete non-existing secrets when the app exists.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderAppSecret(ctx, apps.Default.ID, uuid.New())
		require.Error(t, err)

		// Should not be able to delete an existing secret with the wrong app ID.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		secret, err := client.PostOAuth2ProviderAppSecret(ctx, apps.NoPort.ID)
		require.NoError(t, err)

		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderAppSecret(ctx, apps.Default.ID, secret.ID)
		require.Error(t, err)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		// No secrets yet.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		secrets, err := client.OAuth2ProviderAppSecrets(ctx, apps.Default.ID)
		require.NoError(t, err)
		require.Len(t, secrets, 0)

		// Should be able to create secrets.
		for i := 0; i < 5; i++ {
			//nolint:gocritic // OAauth2 app management requires owner permission.
			secret, err := client.PostOAuth2ProviderAppSecret(ctx, apps.Default.ID)
			require.NoError(t, err)
			require.NotEmpty(t, secret.ClientSecretFull)
			require.True(t, len(secret.ClientSecretFull) > 6)

			//nolint:gocritic // OAauth2 app management requires owner permission.
			_, err = client.PostOAuth2ProviderAppSecret(ctx, apps.NoPort.ID)
			require.NoError(t, err)
		}

		// Should get secrets now, but only for the one app.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		secrets, err = client.OAuth2ProviderAppSecrets(ctx, apps.Default.ID)
		require.NoError(t, err)
		require.Len(t, secrets, 5)
		for _, secret := range secrets {
			require.Len(t, secret.ClientSecretTruncated, 6)
		}

		// Should be able to delete a secret.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderAppSecret(ctx, apps.Default.ID, secrets[0].ID)
		require.NoError(t, err)
		secrets, err = client.OAuth2ProviderAppSecrets(ctx, apps.Default.ID)
		require.NoError(t, err)
		require.Len(t, secrets, 4)

		// No secrets once the app is deleted.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderApp(ctx, apps.Default.ID)
		require.NoError(t, err)

		//nolint:gocritic // OAauth2 app management requires owner permission.
		_, err = client.OAuth2ProviderAppSecrets(ctx, apps.Default.ID)
		require.Error(t, err)
	})
}

type provisionedApps struct {
	Default   codersdk.OAuth2ProviderApp
	NoPort    codersdk.OAuth2ProviderApp
	Subdomain codersdk.OAuth2ProviderApp
	// For sorting purposes these are included. You will likely never touch them.
	Extra []codersdk.OAuth2ProviderApp
}

func generateApps(ctx context.Context, t *testing.T, client *codersdk.Client, suffix string) provisionedApps {
	create := func(name, callback string) codersdk.OAuth2ProviderApp {
		name = fmt.Sprintf("%s-%s", name, suffix)
		//nolint:gocritic // OAauth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        name,
			CallbackURL: callback,
			Icon:        "",
		})
		require.NoError(t, err)
		require.Equal(t, name, app.Name)
		require.Equal(t, callback, app.CallbackURL)
		return app
	}

	return provisionedApps{
		Default:   create("razzle-dazzle-a", "http://localhost1:8080/foo/bar"),
		NoPort:    create("razzle-dazzle-b", "http://localhost2"),
		Subdomain: create("razzle-dazzle-z", "http://30.localhost:3000"),
		Extra: []codersdk.OAuth2ProviderApp{
			create("second-to-last", "http://20.localhost:3000"),
			create("woo-10", "http://10.localhost:3000"),
		},
	}
}
