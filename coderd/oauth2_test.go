package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/identityprovider"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOAuth2ProviderApps(t *testing.T) {
	t.Parallel()

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		topCtx := testutil.Context(t, testutil.WaitLong)

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
		_, err := client.PostOAuth2ProviderApp(topCtx, req)
		require.NoError(t, err)

		// Generate an application for testing PUTs.
		req = codersdk.PostOAuth2ProviderAppRequest{
			Name:        "quark",
			CallbackURL: "http://coder.com",
		}
		//nolint:gocritic // OAauth2 app management requires owner permission.
		existingApp, err := client.PostOAuth2ProviderApp(topCtx, req)
		require.NoError(t, err)

		for _, test := range tests {
			test := test
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

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

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		another, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := another.OAuth2ProviderApp(ctx, uuid.New())
		require.Error(t, err)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
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
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
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

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	topCtx := testutil.Context(t, testutil.WaitLong)

	// Make some apps.
	apps := generateApps(topCtx, t, client, "app-secrets")

	t.Run("DeleteNonExisting", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

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
		ctx := testutil.Context(t, testutil.WaitLong)

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

func TestOAuth2ProviderTokenExchange(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	ownerClient := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	topCtx := testutil.Context(t, testutil.WaitLong)
	apps := generateApps(topCtx, t, ownerClient, "token-exchange")

	//nolint:gocritic // OAauth2 app management requires owner permission.
	secret, err := ownerClient.PostOAuth2ProviderAppSecret(topCtx, apps.Default.ID)
	require.NoError(t, err)

	// The typical oauth2 flow from this point is:
	// Create an oauth2.Config using the id, secret, endpoints, and redirect:
	//	cfg := oauth2.Config{ ... }
	// Display url for the user to click:
	//	userClickURL := cfg.AuthCodeURL("random_state")
	//	userClickURL looks like: https://idp url/authorize?
	//								client_id=...
	//								response_type=code
	//								redirect_uri=.. (back to backstage url) ..
	//								scope=...
	//								state=...
	// *1* User clicks "Allow" on provided page above
	// The redirect_uri is followed which sends back to backstage with the code and state
	// Now backstage has the info to do a cfg.Exchange() in the back to get an access token.
	//
	// ---NOTE---: If the user has already approved this oauth app, then *1* is optional.
	//             Coder can just immediately redirect back to backstage without user intervention.
	tests := []struct {
		name string
		app  codersdk.OAuth2ProviderApp
		// The flow is setup(ctx, client, user) -> preAuth(cfg) -> cfg.AuthCodeURL() -> preToken(cfg) -> cfg.Exchange()
		setup      func(context.Context, *codersdk.Client, codersdk.User) error
		preAuth    func(valid *oauth2.Config)
		authError  string
		preToken   func(valid *oauth2.Config)
		tokenError string

		// If null, assume the code should be valid.
		defaultCode *string
		// custom allows some more advanced manipulation of the oauth2 exchange.
		exchangeMutate []oauth2.AuthCodeOption
	}{
		{
			name: "AuthInParams",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				valid.Endpoint.AuthStyle = oauth2.AuthStyleInParams
			},
		},
		{
			name: "AuthInvalidAppID",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				valid.ClientID = uuid.NewString()
			},
			authError: "Resource not found",
		},
		{
			name: "TokenInvalidAppID",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientID = uuid.NewString()
			},
			tokenError: "Resource not found",
		},
		{
			name: "InvalidPort",
			app:  apps.NoPort,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Host = newURL.Hostname() + ":8081"
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params",
		},
		{
			name: "WrongAppHost",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				valid.RedirectURL = apps.NoPort.CallbackURL
			},
			authError: "Invalid query params",
		},
		{
			name: "InvalidHostPrefix",
			app:  apps.NoPort,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Host = "prefix" + newURL.Hostname()
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params",
		},
		{
			name: "InvalidHost",
			app:  apps.NoPort,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Host = "invalid"
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params",
		},
		{
			name: "InvalidHostAndPort",
			app:  apps.NoPort,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Host = "invalid:8080"
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params",
		},
		{
			name: "InvalidPath",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Path = path.Join("/prepend", newURL.Path)
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params",
		},
		{
			name: "MissingPath",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Path = "/"
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params",
		},
		{
			// TODO: This is valid for now, but should it be?
			name: "DifferentProtocol",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Scheme = "https"
				valid.RedirectURL = newURL.String()
			},
		},
		{
			name: "NestedPath",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Path = path.Join(newURL.Path, "nested")
				valid.RedirectURL = newURL.String()
			},
		},
		{
			// Some oauth implementations allow this, but our users can host
			// at subdomains. So we should not.
			name: "Subdomain",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Host = "sub." + newURL.Host
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params",
		},
		{
			name: "NoSecretScheme",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = "1234_4321"
			},
			tokenError: "Invalid client secret",
		},
		{
			name: "InvalidSecretScheme",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = "notcoder_1234_4321"
			},
			tokenError: "Invalid client secret",
		},
		{
			name: "MissingSecretSecret",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = "coder_1234"
			},
			tokenError: "Invalid client secret",
		},
		{
			name: "MissingSecretPrefix",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = "coder__1234"
			},
			tokenError: "Invalid client secret",
		},
		{
			name: "InvalidSecretPrefix",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = "coder_1234_4321"
			},
			tokenError: "Invalid client secret",
		},
		{
			name: "MissingSecret",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = ""
			},
			tokenError: "Invalid query params",
		},
		{
			name:        "NoCodeScheme",
			app:         apps.Default,
			defaultCode: ptr.Ref("1234_4321"),
			tokenError:  "Invalid code",
		},
		{
			name:        "InvalidCodeScheme",
			app:         apps.Default,
			defaultCode: ptr.Ref("notcoder_1234_4321"),
			tokenError:  "Invalid code",
		},
		{
			name:        "MissingCodeSecret",
			app:         apps.Default,
			defaultCode: ptr.Ref("coder_1234"),
			tokenError:  "Invalid code",
		},
		{
			name:        "MissingCodePrefix",
			app:         apps.Default,
			defaultCode: ptr.Ref("coder__1234"),
			tokenError:  "Invalid code",
		},
		{
			name:        "InvalidCodePrefix",
			app:         apps.Default,
			defaultCode: ptr.Ref("coder_1234_4321"),
			tokenError:  "Invalid code",
		},
		{
			name:        "MissingCode",
			app:         apps.Default,
			defaultCode: ptr.Ref(""),
			tokenError:  "Invalid query params",
		},
		{
			name:       "InvalidGrantType",
			app:        apps.Default,
			tokenError: "Invalid query params",
			exchangeMutate: []oauth2.AuthCodeOption{
				oauth2.SetAuthURLParam("grant_type", "foobar"),
			},
		},
		{
			name:       "EmptyGrantType",
			app:        apps.Default,
			tokenError: "Invalid query params",
			exchangeMutate: []oauth2.AuthCodeOption{
				oauth2.SetAuthURLParam("grant_type", ""),
			},
		},
		{
			name:        "ExpiredCode",
			app:         apps.Default,
			defaultCode: ptr.Ref("coder_prefix_code"),
			tokenError:  "Invalid code",
			setup: func(ctx context.Context, client *codersdk.Client, user codersdk.User) error {
				// Insert an expired code.
				hashedCode, err := userpassword.Hash("prefix_code")
				if err != nil {
					return err
				}
				_, err = db.InsertOAuth2ProviderAppCode(ctx, database.InsertOAuth2ProviderAppCodeParams{
					ID:           uuid.New(),
					CreatedAt:    dbtime.Now().Add(-time.Minute * 11),
					ExpiresAt:    dbtime.Now().Add(-time.Minute),
					SecretPrefix: []byte("prefix"),
					HashedSecret: []byte(hashedCode),
					AppID:        apps.Default.ID,
					UserID:       user.ID,
				})
				return err
			},
		},
		{
			name: "OK",
			app:  apps.Default,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			// Each test gets its own user, since we allow only one code per user and
			// app at a time and running tests in parallel could clobber each other.
			userClient, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
			if test.setup != nil {
				err := test.setup(ctx, userClient, user)
				require.NoError(t, err)
			}

			// Each test gets its own oauth2.Config so they can run in parallel.
			// In practice, you would only use 1 as a singleton.
			valid := &oauth2.Config{
				ClientID:     test.app.ID.String(),
				ClientSecret: secret.ClientSecretFull,
				Endpoint: oauth2.Endpoint{
					AuthURL:       test.app.Endpoints.Authorization,
					DeviceAuthURL: test.app.Endpoints.DeviceAuth,
					TokenURL:      test.app.Endpoints.Token,
					// TODO: @emyrk we should support both types.
					AuthStyle: oauth2.AuthStyleInParams,
				},
				RedirectURL: test.app.CallbackURL,
				Scopes:      []string{},
			}

			if test.preAuth != nil {
				test.preAuth(valid)
			}

			var code string
			if test.defaultCode != nil {
				code = *test.defaultCode
			} else {
				var err error
				code, err = authorizationFlow(ctx, userClient, valid)
				if test.authError != "" {
					require.Error(t, err)
					require.ErrorContains(t, err, test.authError)
					// If this errors the token exchange will fail. So end here.
					return
				}
				require.NoError(t, err)
			}

			// Mutate the valid config for the exchange.
			if test.preToken != nil {
				test.preToken(valid)
			}

			// Do the actual exchange.
			token, err := valid.Exchange(ctx, code, test.exchangeMutate...)
			if test.tokenError != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, test.tokenError)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, token.AccessToken)
				require.True(t, time.Now().After(token.Expiry))

				// Check that the token works.
				newClient := codersdk.New(userClient.URL)
				newClient.SetSessionToken(token.AccessToken)

				gotUser, err := newClient.User(ctx, codersdk.Me)
				require.NoError(t, err)
				require.Equal(t, user.ID, gotUser.ID)
			}
		})
	}
}

func TestOAuth2ProviderTokenRefresh(t *testing.T) {
	t.Parallel()
	topCtx := testutil.Context(t, testutil.WaitLong)

	db, pubsub := dbtestutil.NewDB(t)
	ownerClient := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	apps := generateApps(topCtx, t, ownerClient, "token-refresh")

	//nolint:gocritic // OAauth2 app management requires owner permission.
	secret, err := ownerClient.PostOAuth2ProviderAppSecret(topCtx, apps.Default.ID)
	require.NoError(t, err)

	// One path not tested here is when the token is empty, because Go's OAuth2
	// client library will not even try to make the request.
	tests := []struct {
		name string
		app  codersdk.OAuth2ProviderApp
		// If null, assume the token should be valid.
		defaultToken *string
		error        string
		expires      time.Time
	}{
		{
			name:         "NoTokenScheme",
			app:          apps.Default,
			defaultToken: ptr.Ref("1234_4321"),
			error:        "Invalid token",
		},
		{
			name:         "InvalidTokenScheme",
			app:          apps.Default,
			defaultToken: ptr.Ref("notcoder_1234_4321"),
			error:        "Invalid token",
		},
		{
			name:         "MissingTokenSecret",
			app:          apps.Default,
			defaultToken: ptr.Ref("coder_1234"),
			error:        "Invalid token",
		},
		{
			name:         "MissingTokenPrefix",
			app:          apps.Default,
			defaultToken: ptr.Ref("coder__1234"),
			error:        "Invalid token",
		},
		{
			name:         "InvalidTokenPrefix",
			app:          apps.Default,
			defaultToken: ptr.Ref("coder_1234_4321"),
			error:        "Invalid token",
		},
		{
			name:    "Expired",
			app:     apps.Default,
			expires: time.Now().Add(time.Minute * -1),
			error:   "Invalid token",
		},
		{
			name: "OK",
			app:  apps.Default,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			userClient, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

			// Insert the token and its key.
			key, sessionToken, err := apikey.Generate(apikey.CreateParams{
				UserID:    user.ID,
				LoginType: database.LoginTypeOAuth2ProviderApp,
				ExpiresAt: time.Now().Add(time.Hour * 10),
			})
			require.NoError(t, err)

			newKey, err := db.InsertAPIKey(ctx, key)
			require.NoError(t, err)

			token, err := identityprovider.GenerateSecret()
			require.NoError(t, err)

			expires := test.expires
			if expires.IsZero() {
				expires = time.Now().Add(time.Hour * 10)
			}

			_, err = db.InsertOAuth2ProviderAppToken(ctx, database.InsertOAuth2ProviderAppTokenParams{
				ID:          uuid.New(),
				CreatedAt:   dbtime.Now(),
				ExpiresAt:   expires,
				HashPrefix:  []byte(token.Prefix),
				RefreshHash: []byte(token.Hashed),
				AppSecretID: secret.ID,
				APIKeyID:    newKey.ID,
			})
			require.NoError(t, err)

			// Check that the key works.
			newClient := codersdk.New(userClient.URL)
			newClient.SetSessionToken(sessionToken)
			gotUser, err := newClient.User(ctx, codersdk.Me)
			require.NoError(t, err)
			require.Equal(t, user.ID, gotUser.ID)

			cfg := &oauth2.Config{
				ClientID:     test.app.ID.String(),
				ClientSecret: secret.ClientSecretFull,
				Endpoint: oauth2.Endpoint{
					AuthURL:       test.app.Endpoints.Authorization,
					DeviceAuthURL: test.app.Endpoints.DeviceAuth,
					TokenURL:      test.app.Endpoints.Token,
					AuthStyle:     oauth2.AuthStyleInParams,
				},
				RedirectURL: test.app.CallbackURL,
				Scopes:      []string{},
			}

			// Test whether it can be refreshed.
			refreshToken := token.Formatted
			if test.defaultToken != nil {
				refreshToken = *test.defaultToken
			}
			refreshed, err := cfg.TokenSource(ctx, &oauth2.Token{
				AccessToken:  sessionToken,
				RefreshToken: refreshToken,
				Expiry:       time.Now().Add(time.Minute * -1),
			}).Token()

			if test.error != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, test.error)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, refreshed.AccessToken)

				// Old token is now invalid.
				_, err = newClient.User(ctx, codersdk.Me)
				require.Error(t, err)
				require.ErrorContains(t, err, "401")

				// Refresh token is valid.
				newClient := codersdk.New(userClient.URL)
				newClient.SetSessionToken(refreshed.AccessToken)

				gotUser, err := newClient.User(ctx, codersdk.Me)
				require.NoError(t, err)
				require.Equal(t, user.ID, gotUser.ID)
			}
		})
	}
}

type exchangeSetup struct {
	cfg    *oauth2.Config
	app    codersdk.OAuth2ProviderApp
	secret codersdk.OAuth2ProviderAppSecretFull
	code   string
}

func TestOAuth2ProviderRevoke(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)

	tests := []struct {
		name string
		// fn performs some action that removes the user's code and token.
		fn func(context.Context, *codersdk.Client, exchangeSetup)
		// replacesToken specifies whether the action replaces the token or only
		// deletes it.
		replacesToken bool
	}{
		{
			name: "DeleteApp",
			fn: func(ctx context.Context, _ *codersdk.Client, s exchangeSetup) {
				//nolint:gocritic // OAauth2 app management requires owner permission.
				err := client.DeleteOAuth2ProviderApp(ctx, s.app.ID)
				require.NoError(t, err)
			},
		},
		{
			name: "DeleteSecret",
			fn: func(ctx context.Context, _ *codersdk.Client, s exchangeSetup) {
				//nolint:gocritic // OAauth2 app management requires owner permission.
				err := client.DeleteOAuth2ProviderAppSecret(ctx, s.app.ID, s.secret.ID)
				require.NoError(t, err)
			},
		},
		{
			name: "DeleteToken",
			fn: func(ctx context.Context, client *codersdk.Client, s exchangeSetup) {
				err := client.RevokeOAuth2ProviderApp(ctx, s.app.ID)
				require.NoError(t, err)
			},
		},
		{
			name: "OverrideCodeAndToken",
			fn: func(ctx context.Context, client *codersdk.Client, s exchangeSetup) {
				// Generating a new code should wipe out the old code.
				code, err := authorizationFlow(ctx, client, s.cfg)
				require.NoError(t, err)

				// Generating a new token should wipe out the old token.
				_, err = s.cfg.Exchange(ctx, code)
				require.NoError(t, err)
			},
			replacesToken: true,
		},
	}

	setup := func(ctx context.Context, testClient *codersdk.Client, name string) exchangeSetup {
		// We need a new app each time because we only allow one code and token per
		// app and user at the moment and because the test might delete the app.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        name,
			CallbackURL: "http://localhost",
		})
		require.NoError(t, err)

		// We need a new secret every time because the test might delete the secret.
		//nolint:gocritic // OAauth2 app management requires owner permission.
		secret, err := client.PostOAuth2ProviderAppSecret(ctx, app.ID)
		require.NoError(t, err)

		cfg := &oauth2.Config{
			ClientID:     app.ID.String(),
			ClientSecret: secret.ClientSecretFull,
			Endpoint: oauth2.Endpoint{
				AuthURL:       app.Endpoints.Authorization,
				DeviceAuthURL: app.Endpoints.DeviceAuth,
				TokenURL:      app.Endpoints.Token,
				AuthStyle:     oauth2.AuthStyleInParams,
			},
			RedirectURL: app.CallbackURL,
			Scopes:      []string{},
		}

		// Go through the auth flow to get a code.
		code, err := authorizationFlow(ctx, testClient, cfg)
		require.NoError(t, err)

		return exchangeSetup{
			cfg:    cfg,
			app:    app,
			secret: secret,
			code:   code,
		}
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			testClient, testUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

			testEntities := setup(ctx, testClient, test.name+"-1")

			// Delete before the exchange completes (code should delete and attempting
			// to finish the exchange should fail).
			test.fn(ctx, testClient, testEntities)

			// Exchange should fail because the code should be gone.
			_, err := testEntities.cfg.Exchange(ctx, testEntities.code)
			require.Error(t, err)

			// Try again, this time letting the exchange complete first.
			testEntities = setup(ctx, testClient, test.name+"-2")
			token, err := testEntities.cfg.Exchange(ctx, testEntities.code)
			require.NoError(t, err)

			// Validate the returned access token and that the app is listed.
			newClient := codersdk.New(client.URL)
			newClient.SetSessionToken(token.AccessToken)

			gotUser, err := newClient.User(ctx, codersdk.Me)
			require.NoError(t, err)
			require.Equal(t, testUser.ID, gotUser.ID)

			filter := codersdk.OAuth2ProviderAppFilter{UserID: testUser.ID}
			apps, err := testClient.OAuth2ProviderApps(ctx, filter)
			require.NoError(t, err)
			require.Contains(t, apps, testEntities.app)

			// Should not show up for another user.
			apps, err = client.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{UserID: owner.UserID})
			require.NoError(t, err)
			require.Len(t, apps, 0)

			// Perform the deletion.
			test.fn(ctx, testClient, testEntities)

			// App should no longer show up for the user unless it was replaced.
			if !test.replacesToken {
				apps, err = testClient.OAuth2ProviderApps(ctx, filter)
				require.NoError(t, err)
				require.NotContains(t, apps, testEntities.app, fmt.Sprintf("contains %q", testEntities.app.Name))
			}

			// The token should no longer be valid.
			_, err = newClient.User(ctx, codersdk.Me)
			require.Error(t, err)
			require.ErrorContains(t, err, "401")
		})
	}
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

func authorizationFlow(ctx context.Context, client *codersdk.Client, cfg *oauth2.Config) (string, error) {
	state := uuid.NewString()
	return oidctest.OAuth2GetCode(
		cfg.AuthCodeURL(state),
		func(req *http.Request) (*http.Response, error) {
			// TODO: Would be better if client had a .Do() method.
			// TODO: Is this the best way to handle redirects?
			client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
			return client.Request(ctx, req.Method, req.URL.String(), nil, func(req *http.Request) {
				// Set the referer so the request bypasses the HTML page (normally you
				// have to click "allow" first, and the way we detect that is using the
				// referer header).
				req.Header.Set("Referer", req.URL.String())
			})
		},
	)
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
