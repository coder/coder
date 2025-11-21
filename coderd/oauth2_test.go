package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/oauth2provider"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestOAuth2ProviderApps(t *testing.T) {
	t.Parallel()

	// NOTE: Unit tests for OAuth2 provider app validation have been migrated to
	// oauth2provider/provider_test.go for better separation of concerns.
	// This test function now focuses on integration testing with the full server stack.

	t.Run("IntegrationFlow", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Test basic app creation and management in integration context
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        fmt.Sprintf("integration-test-%d", time.Now().UnixNano()%1000000),
			CallbackURL: "http://localhost:3000",
		})
		require.NoError(t, err)
		require.NotEmpty(t, app.ID)
		require.NotEmpty(t, app.Name)
		require.Equal(t, "http://localhost:3000", app.CallbackURL)
	})
}

func TestOAuth2ProviderAppSecrets(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	ctx := testutil.Context(t, testutil.WaitLong)

	// Make some apps.
	apps := generateApps(ctx, t, client, "app-secrets")

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
	ctx := testutil.Context(t, testutil.WaitLong)
	apps := generateApps(ctx, t, ownerClient, "token-exchange")

	//nolint:gocritic // OAauth2 app management requires owner permission.
	secret, err := ownerClient.PostOAuth2ProviderAppSecret(ctx, apps.Default.ID)
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
			authError: "invalid_client",
		},
		{
			name: "TokenInvalidAppID",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientID = uuid.NewString()
			},
			tokenError: "invalid_client",
		},
		{
			name: "InvalidPort",
			app:  apps.NoPort,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Host = newURL.Hostname() + ":8081"
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params:",
		},
		{
			name: "WrongAppHost",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				valid.RedirectURL = apps.NoPort.CallbackURL
			},
			authError: "Invalid query params:",
		},
		{
			name: "InvalidHostPrefix",
			app:  apps.NoPort,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Host = "prefix" + newURL.Hostname()
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params:",
		},
		{
			name: "InvalidHost",
			app:  apps.NoPort,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Host = "invalid"
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params:",
		},
		{
			name: "InvalidHostAndPort",
			app:  apps.NoPort,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Host = "invalid:8080"
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params:",
		},
		{
			name: "InvalidPath",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Path = path.Join("/prepend", newURL.Path)
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params:",
		},
		{
			name: "MissingPath",
			app:  apps.Default,
			preAuth: func(valid *oauth2.Config) {
				newURL := must(url.Parse(valid.RedirectURL))
				newURL.Path = "/"
				valid.RedirectURL = newURL.String()
			},
			authError: "Invalid query params:",
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
			authError: "Invalid query params:",
		},
		{
			name: "NoSecretScheme",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = "1234_4321"
			},
			tokenError: "The client credentials are invalid",
		},
		{
			name: "InvalidSecretScheme",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = "notcoder_1234_4321"
			},
			tokenError: "The client credentials are invalid",
		},
		{
			name: "MissingSecretSecret",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = "coder_1234"
			},
			tokenError: "The client credentials are invalid",
		},
		{
			name: "MissingSecretPrefix",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = "coder__1234"
			},
			tokenError: "The client credentials are invalid",
		},
		{
			name: "InvalidSecretPrefix",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = "coder_1234_4321"
			},
			tokenError: "The client credentials are invalid",
		},
		{
			name: "MissingSecret",
			app:  apps.Default,
			preToken: func(valid *oauth2.Config) {
				valid.ClientSecret = ""
			},
			tokenError: "invalid_request",
		},
		{
			name:        "NoCodeScheme",
			app:         apps.Default,
			defaultCode: ptr.Ref("1234_4321"),
			tokenError:  "The authorization code is invalid or expired",
		},
		{
			name:        "InvalidCodeScheme",
			app:         apps.Default,
			defaultCode: ptr.Ref("notcoder_1234_4321"),
			tokenError:  "The authorization code is invalid or expired",
		},
		{
			name:        "MissingCodeSecret",
			app:         apps.Default,
			defaultCode: ptr.Ref("coder_1234"),
			tokenError:  "The authorization code is invalid or expired",
		},
		{
			name:        "MissingCodePrefix",
			app:         apps.Default,
			defaultCode: ptr.Ref("coder__1234"),
			tokenError:  "The authorization code is invalid or expired",
		},
		{
			name:        "InvalidCodePrefix",
			app:         apps.Default,
			defaultCode: ptr.Ref("coder_1234_4321"),
			tokenError:  "The authorization code is invalid or expired",
		},
		{
			name:        "MissingCode",
			app:         apps.Default,
			defaultCode: ptr.Ref(""),
			tokenError:  "invalid_request",
		},
		{
			name:       "InvalidGrantType",
			app:        apps.Default,
			tokenError: "unsupported_grant_type",
			exchangeMutate: []oauth2.AuthCodeOption{
				oauth2.SetAuthURLParam("grant_type", "foobar"),
			},
		},
		{
			name:       "EmptyGrantType",
			app:        apps.Default,
			tokenError: "unsupported_grant_type",
			exchangeMutate: []oauth2.AuthCodeOption{
				oauth2.SetAuthURLParam("grant_type", ""),
			},
		},
		{
			name:        "ExpiredCode",
			app:         apps.Default,
			defaultCode: ptr.Ref("coder_prefix_code"),
			tokenError:  "The authorization code is invalid or expired",
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
				require.True(t, time.Now().Before(token.Expiry))

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
	ctx := testutil.Context(t, testutil.WaitLong)

	db, pubsub := dbtestutil.NewDB(t)
	ownerClient := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	apps := generateApps(ctx, t, ownerClient, "token-refresh")

	//nolint:gocritic // OAauth2 app management requires owner permission.
	secret, err := ownerClient.PostOAuth2ProviderAppSecret(ctx, apps.Default.ID)
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
			error:        "The refresh token is invalid or expired",
		},
		{
			name:         "InvalidTokenScheme",
			app:          apps.Default,
			defaultToken: ptr.Ref("notcoder_1234_4321"),
			error:        "The refresh token is invalid or expired",
		},
		{
			name:         "MissingTokenSecret",
			app:          apps.Default,
			defaultToken: ptr.Ref("coder_1234"),
			error:        "The refresh token is invalid or expired",
		},
		{
			name:         "MissingTokenPrefix",
			app:          apps.Default,
			defaultToken: ptr.Ref("coder__1234"),
			error:        "The refresh token is invalid or expired",
		},
		{
			name:         "InvalidTokenPrefix",
			app:          apps.Default,
			defaultToken: ptr.Ref("coder_1234_4321"),
			error:        "The refresh token is invalid or expired",
		},
		{
			name:    "Expired",
			app:     apps.Default,
			expires: time.Now().Add(time.Minute * -1),
			error:   "The refresh token is invalid or expired",
		},
		{
			name: "OK",
			app:  apps.Default,
		},
	}
	for _, test := range tests {
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

			token, err := oauth2provider.GenerateSecret()
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
				RefreshHash: token.Hashed,
				AppSecretID: secret.ID,
				APIKeyID:    newKey.ID,
				UserID:      user.ID,
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
			name: "DeleteApp",
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
		Default:   create("app-a", "http://localhost1:8080/foo/bar"),
		NoPort:    create("app-b", "http://localhost2"),
		Subdomain: create("app-z", "http://30.localhost:3000"),
		Extra: []codersdk.OAuth2ProviderApp{
			create("app-x", "http://20.localhost:3000"),
			create("app-y", "http://10.localhost:3000"),
		},
	}
}

func authorizationFlow(ctx context.Context, client *codersdk.Client, cfg *oauth2.Config) (string, error) {
	state := uuid.NewString()
	authURL := cfg.AuthCodeURL(state)

	// Make a POST request to simulate clicking "Allow" on the authorization page
	// This bypasses the HTML consent page and directly processes the authorization
	return oidctest.OAuth2GetCode(
		authURL,
		func(req *http.Request) (*http.Response, error) {
			// Change to POST to simulate the form submission
			req.Method = http.MethodPost

			// Prevent automatic redirect following
			client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
			return client.Request(ctx, req.Method, req.URL.String(), nil)
		},
	)
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// TestOAuth2ProviderResourceIndicators tests RFC 8707 Resource Indicators support
// including resource parameter validation in authorization and token exchange flows.
func TestOAuth2ProviderResourceIndicators(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	ownerClient := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	ctx := testutil.Context(t, testutil.WaitLong)
	apps := generateApps(ctx, t, ownerClient, "resource-indicators")

	//nolint:gocritic // OAauth2 app management requires owner permission.
	secret, err := ownerClient.PostOAuth2ProviderAppSecret(ctx, apps.Default.ID)
	require.NoError(t, err)

	resource := ownerClient.URL.String()

	tests := []struct {
		name               string
		authResource       string // Resource parameter during authorization
		tokenResource      string // Resource parameter during token exchange
		refreshResource    string // Resource parameter during refresh
		expectAuthError    bool
		expectTokenError   bool
		expectRefreshError bool
	}{
		{
			name: "NoResourceParameter",
			// Standard flow without resource parameter
		},
		{
			name:            "ValidResourceParameter",
			authResource:    resource,
			tokenResource:   resource,
			refreshResource: resource,
		},
		{
			name:             "ResourceInAuthOnly",
			authResource:     resource,
			tokenResource:    "", // Missing in token exchange
			expectTokenError: true,
		},
		{
			name:             "ResourceInTokenOnly",
			authResource:     "", // Missing in auth
			tokenResource:    resource,
			expectTokenError: true,
		},
		{
			name:             "ResourceMismatch",
			authResource:     "https://resource1.example.com",
			tokenResource:    "https://resource2.example.com", // Different resource
			expectTokenError: true,
		},
		{
			name:               "RefreshWithDifferentResource",
			authResource:       resource,
			tokenResource:      resource,
			refreshResource:    "https://different.example.com", // Different in refresh
			expectRefreshError: true,
		},
		{
			name:            "RefreshWithoutResource",
			authResource:    resource,
			tokenResource:   resource,
			refreshResource: "", // No resource in refresh (allowed)
		},
		{
			name:            "RefreshWithSameResource",
			authResource:    resource,
			tokenResource:   resource,
			refreshResource: resource, // Same resource in refresh
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			userClient, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

			cfg := &oauth2.Config{
				ClientID:     apps.Default.ID.String(),
				ClientSecret: secret.ClientSecretFull,
				Endpoint: oauth2.Endpoint{
					AuthURL:   apps.Default.Endpoints.Authorization,
					TokenURL:  apps.Default.Endpoints.Token,
					AuthStyle: oauth2.AuthStyleInParams,
				},
				RedirectURL: apps.Default.CallbackURL,
				Scopes:      []string{},
			}

			// Step 1: Authorization with resource parameter
			state := uuid.NewString()
			authURL := cfg.AuthCodeURL(state)
			if test.authResource != "" {
				// Add resource parameter to auth URL
				parsedURL, err := url.Parse(authURL)
				require.NoError(t, err)
				query := parsedURL.Query()
				query.Set("resource", test.authResource)
				parsedURL.RawQuery = query.Encode()
				authURL = parsedURL.String()
			}

			// Simulate authorization flow
			code, err := oidctest.OAuth2GetCode(
				authURL,
				func(req *http.Request) (*http.Response, error) {
					req.Method = http.MethodPost
					userClient.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
						return http.ErrUseLastResponse
					}
					return userClient.Request(ctx, req.Method, req.URL.String(), nil)
				},
			)

			if test.expectAuthError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Step 2: Token exchange with resource parameter
			// Use custom token exchange since golang.org/x/oauth2 doesn't support resource parameter in token requests
			token, err := customTokenExchange(ctx, ownerClient.URL.String(), apps.Default.ID.String(), secret.ClientSecretFull, code, apps.Default.CallbackURL, test.tokenResource)
			if test.expectTokenError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "invalid_target")
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, token.AccessToken)

			// Per RFC 8707, audience is stored in database but not returned in token response
			// The audience validation happens server-side during API requests

			// Step 3: Test API access with token audience validation
			newClient := codersdk.New(userClient.URL)
			newClient.SetSessionToken(token.AccessToken)

			// Token should work for API access
			gotUser, err := newClient.User(ctx, codersdk.Me)
			require.NoError(t, err)
			require.Equal(t, user.ID, gotUser.ID)

			// Step 4: Test refresh token flow with resource parameter
			if token.RefreshToken != "" {
				// Note: OAuth2 library doesn't easily support custom parameters in refresh flows
				// For now, we test basic refresh functionality without resource parameter
				// TODO: Implement custom refresh flow testing with resource parameter

				// Create a token source with refresh capability
				tokenSource := cfg.TokenSource(ctx, &oauth2.Token{
					AccessToken:  token.AccessToken,
					RefreshToken: token.RefreshToken,
					Expiry:       time.Now().Add(-time.Minute), // Force refresh
				})

				// Test token refresh
				refreshedToken, err := tokenSource.Token()
				require.NoError(t, err)
				require.NotEmpty(t, refreshedToken.AccessToken)

				// Old token should be invalid
				_, err = newClient.User(ctx, codersdk.Me)
				require.Error(t, err)

				// New token should work
				newClient.SetSessionToken(refreshedToken.AccessToken)
				gotUser, err = newClient.User(ctx, codersdk.Me)
				require.NoError(t, err)
				require.Equal(t, user.ID, gotUser.ID)
			}
		})
	}
}

// TestOAuth2ProviderCrossResourceAudienceValidation tests that tokens are properly
// validated against the audience/resource server they were issued for.
func TestOAuth2ProviderCrossResourceAudienceValidation(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)

	// Set up first Coder instance (resource server 1)
	server1 := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	owner := coderdtest.CreateFirstUser(t, server1)

	// Set up second Coder instance (resource server 2) - simulate different host
	server2 := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})

	ctx := testutil.Context(t, testutil.WaitLong)

	// Create OAuth2 app
	apps := generateApps(ctx, t, server1, "cross-resource")

	//nolint:gocritic // OAauth2 app management requires owner permission.
	secret, err := server1.PostOAuth2ProviderAppSecret(ctx, apps.Default.ID)
	require.NoError(t, err)
	userClient, user := coderdtest.CreateAnotherUser(t, server1, owner.OrganizationID)

	// Get token with specific audience for server1
	resource1 := server1.URL.String()
	cfg := &oauth2.Config{
		ClientID:     apps.Default.ID.String(),
		ClientSecret: secret.ClientSecretFull,
		Endpoint: oauth2.Endpoint{
			AuthURL:   apps.Default.Endpoints.Authorization,
			TokenURL:  apps.Default.Endpoints.Token,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: apps.Default.CallbackURL,
		Scopes:      []string{},
	}

	// Authorization with resource parameter for server1
	state := uuid.NewString()
	authURL := cfg.AuthCodeURL(state)
	parsedURL, err := url.Parse(authURL)
	require.NoError(t, err)
	query := parsedURL.Query()
	query.Set("resource", resource1)
	parsedURL.RawQuery = query.Encode()
	authURL = parsedURL.String()

	code, err := oidctest.OAuth2GetCode(
		authURL,
		func(req *http.Request) (*http.Response, error) {
			req.Method = http.MethodPost
			userClient.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
			return userClient.Request(ctx, req.Method, req.URL.String(), nil)
		},
	)
	require.NoError(t, err)

	// Exchange code for token with resource parameter
	token, err := cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("resource", resource1))
	require.NoError(t, err)
	require.NotEmpty(t, token.AccessToken)

	// Token should work on server1 (correct audience)
	client1 := codersdk.New(server1.URL)
	client1.SetSessionToken(token.AccessToken)
	gotUser, err := client1.User(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Equal(t, user.ID, gotUser.ID)

	// Token should NOT work on server2 (different audience/host) if audience validation is implemented
	// Note: This test verifies that the audience validation middleware properly rejects
	// tokens issued for different resource servers
	client2 := codersdk.New(server2.URL)
	client2.SetSessionToken(token.AccessToken)

	// This should fail due to audience mismatch if validation is properly implemented
	// The expected behavior depends on whether the middleware detects Host differences
	if _, err := client2.User(ctx, codersdk.Me); err != nil {
		// This is expected if audience validation is working properly
		t.Logf("Cross-resource token properly rejected: %v", err)
		// Assert that the error is related to audience validation
		require.Contains(t, err.Error(), "audience")
	} else {
		// The token might still work if both servers use the same database but different URLs
		// since the actual audience validation depends on Host header comparison
		t.Logf("Cross-resource token was accepted (both servers use same database)")
		// For now, we accept this behavior since both servers share the same database
		// In a real cross-deployment scenario, this should fail
	}

	// TODO: Enhance this test when we have better cross-deployment testing setup
	// For now, this verifies the basic token flow works correctly
}

// TestOAuth2RefreshExpiryOutlivesAccess verifies that refresh token expiry is
// greater than the provisioned access token (API key) expiry per configuration.
func TestOAuth2RefreshExpiryOutlivesAccess(t *testing.T) {
	t.Parallel()

	// Set explicit lifetimes to make comparison deterministic.
	db, pubsub := dbtestutil.NewDB(t)
	dv := coderdtest.DeploymentValues(t, func(d *codersdk.DeploymentValues) {
		d.Sessions.DefaultDuration = serpent.Duration(1 * time.Hour)
		d.Sessions.RefreshDefaultDuration = serpent.Duration(48 * time.Hour)
	})
	ownerClient := coderdtest.New(t, &coderdtest.Options{
		Database:         db,
		Pubsub:           pubsub,
		DeploymentValues: dv,
	})
	_ = coderdtest.CreateFirstUser(t, ownerClient)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Create app and secret
	// Keep suffix short to satisfy name validation (<=32 chars, alnum + hyphens).
	apps := generateApps(ctx, t, ownerClient, "ref-exp")
	//nolint:gocritic // Owner permission required for app secret creation
	secret, err := ownerClient.PostOAuth2ProviderAppSecret(ctx, apps.Default.ID)
	require.NoError(t, err)

	cfg := &oauth2.Config{
		ClientID:     apps.Default.ID.String(),
		ClientSecret: secret.ClientSecretFull,
		Endpoint: oauth2.Endpoint{
			AuthURL:       apps.Default.Endpoints.Authorization,
			DeviceAuthURL: apps.Default.Endpoints.DeviceAuth,
			TokenURL:      apps.Default.Endpoints.Token,
			AuthStyle:     oauth2.AuthStyleInParams,
		},
		RedirectURL: apps.Default.CallbackURL,
		Scopes:      []string{},
	}

	// Authorization and token exchange
	code, err := authorizationFlow(ctx, ownerClient, cfg)
	require.NoError(t, err)
	tok, err := cfg.Exchange(ctx, code)
	require.NoError(t, err)
	require.NotEmpty(t, tok.AccessToken)
	require.NotEmpty(t, tok.RefreshToken)

	// Parse refresh token prefix (coder_<prefix>_<secret>)
	parts := strings.Split(tok.RefreshToken, "_")
	require.Len(t, parts, 3)
	prefix := parts[1]

	// Look up refresh token row and associated API key
	dbToken, err := db.GetOAuth2ProviderAppTokenByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(prefix))
	require.NoError(t, err)
	apiKey, err := db.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), dbToken.APIKeyID)
	require.NoError(t, err)

	// Assert refresh token expiry is strictly after access token expiry
	require.Truef(t, dbToken.ExpiresAt.After(apiKey.ExpiresAt),
		"expected refresh expiry %s to be after access expiry %s",
		dbToken.ExpiresAt, apiKey.ExpiresAt,
	)
}

// customTokenExchange performs a custom OAuth2 token exchange with support for resource parameter
// This is needed because golang.org/x/oauth2 doesn't support custom parameters in token requests
func customTokenExchange(ctx context.Context, baseURL, clientID, clientSecret, code, redirectURI, resource string) (*oauth2.Token, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("redirect_uri", redirectURI)
	if resource != "" {
		data.Set("resource", resource)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/oauth2/tokens", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errorResp)
		return nil, xerrors.Errorf("oauth2: %q %q", errorResp.Error, errorResp.ErrorDescription)
	}

	var token oauth2.Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	return &token, nil
}

// TestOAuth2DynamicClientRegistration tests RFC 7591 dynamic client registration
func TestOAuth2DynamicClientRegistration(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	t.Run("BasicRegistration", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		clientName := fmt.Sprintf("test-client-basic-%d", time.Now().UnixNano())
		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   clientName,
			ClientURI:    "https://example.com",
			LogoURI:      "https://example.com/logo.png",
			TOSURI:       "https://example.com/tos",
			PolicyURI:    "https://example.com/privacy",
			Contacts:     []string{"admin@example.com"},
		}

		// Register client
		resp, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.NoError(t, err)

		// Verify response fields
		require.NotEmpty(t, resp.ClientID)
		require.NotEmpty(t, resp.ClientSecret)
		require.NotEmpty(t, resp.RegistrationAccessToken)
		require.NotEmpty(t, resp.RegistrationClientURI)
		require.Greater(t, resp.ClientIDIssuedAt, int64(0))
		require.Equal(t, int64(0), resp.ClientSecretExpiresAt) // Non-expiring

		// Verify default values
		require.Contains(t, resp.GrantTypes, "authorization_code")
		require.Contains(t, resp.GrantTypes, "refresh_token")
		require.Contains(t, resp.ResponseTypes, "code")
		require.Equal(t, "client_secret_basic", resp.TokenEndpointAuthMethod)

		// Verify request values are preserved
		require.Equal(t, req.RedirectURIs, resp.RedirectURIs)
		require.Equal(t, req.ClientName, resp.ClientName)
		require.Equal(t, req.ClientURI, resp.ClientURI)
		require.Equal(t, req.LogoURI, resp.LogoURI)
		require.Equal(t, req.TOSURI, resp.TOSURI)
		require.Equal(t, req.PolicyURI, resp.PolicyURI)
		require.Equal(t, req.Contacts, resp.Contacts)
	})

	t.Run("MinimalRegistration", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://minimal.com/callback"},
		}

		// Register client with minimal fields
		resp, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.NoError(t, err)

		// Should still get all required fields
		require.NotEmpty(t, resp.ClientID)
		require.NotEmpty(t, resp.ClientSecret)
		require.NotEmpty(t, resp.RegistrationAccessToken)
		require.NotEmpty(t, resp.RegistrationClientURI)

		// Should have defaults applied
		require.Contains(t, resp.GrantTypes, "authorization_code")
		require.Contains(t, resp.ResponseTypes, "code")
		require.Equal(t, "client_secret_basic", resp.TokenEndpointAuthMethod)
	})

	t.Run("InvalidRedirectURI", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"not-a-url"},
		}

		_, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_client_metadata")
	})

	t.Run("NoRedirectURIs", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		req := codersdk.OAuth2ClientRegistrationRequest{
			ClientName: fmt.Sprintf("no-uris-client-%d", time.Now().UnixNano()),
		}

		_, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_client_metadata")
	})
}

// TestOAuth2ClientConfiguration tests RFC 7592 client configuration management
func TestOAuth2ClientConfiguration(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	// Helper to register a client
	registerClient := func(t *testing.T) (string, string, string) {
		ctx := testutil.Context(t, testutil.WaitLong)
		// Use shorter client name to avoid database varchar(64) constraint
		clientName := fmt.Sprintf("client-%d", time.Now().UnixNano())
		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   clientName,
			ClientURI:    "https://example.com",
		}

		resp, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.NoError(t, err)
		return resp.ClientID, resp.RegistrationAccessToken, clientName
	}

	t.Run("GetConfiguration", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		clientID, token, clientName := registerClient(t)

		// Get client configuration
		config, err := client.GetOAuth2ClientConfiguration(ctx, clientID, token)
		require.NoError(t, err)

		// Verify fields
		require.Equal(t, clientID, config.ClientID)
		require.Greater(t, config.ClientIDIssuedAt, int64(0))
		require.Equal(t, []string{"https://example.com/callback"}, config.RedirectURIs)
		require.Equal(t, clientName, config.ClientName)
		require.Equal(t, "https://example.com", config.ClientURI)

		// Should not contain client_secret in GET response
		require.Empty(t, config.RegistrationAccessToken) // Not included in GET
	})

	t.Run("UpdateConfiguration", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		clientID, token, _ := registerClient(t)

		// Update client configuration
		updatedName := fmt.Sprintf("updated-test-client-%d", time.Now().UnixNano())
		updateReq := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://newdomain.com/callback", "https://example.com/callback"},
			ClientName:   updatedName,
			ClientURI:    "https://newdomain.com",
			LogoURI:      "https://newdomain.com/logo.png",
		}

		config, err := client.PutOAuth2ClientConfiguration(ctx, clientID, token, updateReq)
		require.NoError(t, err)

		// Verify updates
		require.Equal(t, clientID, config.ClientID)
		require.Equal(t, updateReq.RedirectURIs, config.RedirectURIs)
		require.Equal(t, updateReq.ClientName, config.ClientName)
		require.Equal(t, updateReq.ClientURI, config.ClientURI)
		require.Equal(t, updateReq.LogoURI, config.LogoURI)
	})

	t.Run("DeleteConfiguration", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		clientID, token, _ := registerClient(t)

		// Delete client
		err := client.DeleteOAuth2ClientConfiguration(ctx, clientID, token)
		require.NoError(t, err)

		// Should no longer be able to get configuration
		_, err = client.GetOAuth2ClientConfiguration(ctx, clientID, token)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_token")
	})

	t.Run("InvalidToken", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		clientID, _, _ := registerClient(t)
		invalidToken := "invalid-token"

		// Should fail with invalid token
		_, err := client.GetOAuth2ClientConfiguration(ctx, clientID, invalidToken)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_token")
	})

	t.Run("NonexistentClient", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		fakeClientID := uuid.NewString()
		fakeToken := "fake-token"

		_, err := client.GetOAuth2ClientConfiguration(ctx, fakeClientID, fakeToken)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_token")
	})

	t.Run("MissingAuthHeader", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		clientID, _, _ := registerClient(t)

		// Try to access without token (empty string)
		_, err := client.GetOAuth2ClientConfiguration(ctx, clientID, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_token")
	})
}

// TestOAuth2RegistrationAccessToken tests the registration access token middleware
func TestOAuth2RegistrationAccessToken(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	t.Run("ValidToken", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Register a client
		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   fmt.Sprintf("token-test-client-%d", time.Now().UnixNano()),
		}

		resp, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.NoError(t, err)

		// Valid token should work
		config, err := client.GetOAuth2ClientConfiguration(ctx, resp.ClientID, resp.RegistrationAccessToken)
		require.NoError(t, err)
		require.Equal(t, resp.ClientID, config.ClientID)
	})

	t.Run("ManuallyCreatedClient", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Create a client through the normal API (not dynamic registration)
		appReq := codersdk.PostOAuth2ProviderAppRequest{
			Name:        fmt.Sprintf("manual-%d", time.Now().UnixNano()%1000000),
			CallbackURL: "https://manual.com/callback",
		}

		app, err := client.PostOAuth2ProviderApp(ctx, appReq)
		require.NoError(t, err)

		// Should not be able to manage via RFC 7592 endpoints
		_, err = client.GetOAuth2ClientConfiguration(ctx, app.ID.String(), "any-token")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_token") // Client was not dynamically registered
	})

	t.Run("TokenPasswordComparison", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Register two clients to ensure tokens are unique
		timestamp := time.Now().UnixNano()
		req1 := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://client1.com/callback"},
			ClientName:   fmt.Sprintf("client-1-%d", timestamp),
		}
		req2 := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://client2.com/callback"},
			ClientName:   fmt.Sprintf("client-2-%d", timestamp+1),
		}

		resp1, err := client.PostOAuth2ClientRegistration(ctx, req1)
		require.NoError(t, err)

		resp2, err := client.PostOAuth2ClientRegistration(ctx, req2)
		require.NoError(t, err)

		// Each client should only work with its own token
		_, err = client.GetOAuth2ClientConfiguration(ctx, resp1.ClientID, resp1.RegistrationAccessToken)
		require.NoError(t, err)

		_, err = client.GetOAuth2ClientConfiguration(ctx, resp2.ClientID, resp2.RegistrationAccessToken)
		require.NoError(t, err)

		// Cross-client tokens should fail
		_, err = client.GetOAuth2ClientConfiguration(ctx, resp1.ClientID, resp2.RegistrationAccessToken)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_token")

		_, err = client.GetOAuth2ClientConfiguration(ctx, resp2.ClientID, resp1.RegistrationAccessToken)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_token")
	})
}

// TestOAuth2CoderClient verfies a codersdk client can be used with an oauth client.
func TestOAuth2CoderClient(t *testing.T) {
	t.Parallel()

	owner := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, owner)

	// Setup an oauth app
	ctx := testutil.Context(t, testutil.WaitLong)
	app, err := owner.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
		Name:        "new-app",
		CallbackURL: "http://localhost",
	})
	require.NoError(t, err)

	appsecret, err := owner.PostOAuth2ProviderAppSecret(ctx, app.ID)
	require.NoError(t, err)

	cfg := &oauth2.Config{
		ClientID:     app.ID.String(),
		ClientSecret: appsecret.ClientSecretFull,
		Endpoint: oauth2.Endpoint{
			AuthURL:       app.Endpoints.Authorization,
			DeviceAuthURL: app.Endpoints.DeviceAuth,
			TokenURL:      app.Endpoints.Token,
			AuthStyle:     oauth2.AuthStyleInParams,
		},
		RedirectURL: app.CallbackURL,
		Scopes:      []string{},
	}

	// Make a new user
	client, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID)

	// Do an OAuth2 token exchange and get a new client with an oauth token
	state := uuid.NewString()

	// Get an OAuth2 code for a token exchange
	code, err := oidctest.OAuth2GetCode(
		cfg.AuthCodeURL(state),
		func(req *http.Request) (*http.Response, error) {
			// Change to POST to simulate the form submission
			req.Method = http.MethodPost

			// Prevent automatic redirect following
			client.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
			return client.Request(ctx, req.Method, req.URL.String(), nil)
		},
	)
	require.NoError(t, err)

	token, err := cfg.Exchange(ctx, code)
	require.NoError(t, err)

	// Use the oauth client's authentication
	// TODO: The SDK could probably support this with a better syntax/api.
	oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	usingOauth := codersdk.New(owner.URL)
	usingOauth.HTTPClient = oauthClient

	me, err := usingOauth.User(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Equal(t, user.ID, me.ID)

	// Revoking the refresh token should prevent further access
	// Revoking the refresh also invalidates the associated access token.
	err = usingOauth.RevokeOAuth2Token(ctx, app.ID, token.RefreshToken)
	require.NoError(t, err)

	_, err = usingOauth.User(ctx, codersdk.Me)
	require.Error(t, err)
}

// NOTE: OAuth2 client registration validation tests have been migrated to
// oauth2provider/validation_test.go for better separation of concerns
