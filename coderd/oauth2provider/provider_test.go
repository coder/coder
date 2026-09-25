package oauth2provider_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/oauth2provider/oauth2providertest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestOAuth2ProviderAppValidation tests validation logic for OAuth2 provider app requests
func TestOAuth2ProviderAppValidation(t *testing.T) {
	t.Parallel()

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

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
				name: "NameTooLong",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        strings.Repeat("a", 65),
					CallbackURL: "http://localhost:3000",
				},
			},
			{
				name: "NameLeadingSpace",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        " foo",
					CallbackURL: "http://localhost:3000",
				},
			},
			{
				name: "ScopeTooManyNames",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo",
					CallbackURL: "http://localhost:3000",
					Scope:       strings.Repeat("s ", codersdk.OAuth2ScopeListMaxNames+1),
				},
			},
			{
				name: "ScopeTooLong",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name:        "foo",
					CallbackURL: "http://localhost:3000",
					Scope:       strings.Repeat("a", codersdk.OAuth2ScopeListMaxBytes+1),
				},
			},
			{
				name: "URLMissing",
				req: codersdk.PostOAuth2ProviderAppRequest{
					Name: "foo",
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

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				testCtx := testutil.Context(t, testutil.WaitLong)

				//nolint:gocritic // OAuth2 app management requires owner permission.
				_, err := client.PostOAuth2ProviderApp(testCtx, test.req)
				require.Error(t, err)
			})
		}
	})

	t.Run("AcceptsDCRValues", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Values registered through Dynamic Client Registration (RFC 7591),
		// such as a name with spaces and a custom native-app callback scheme,
		// must be creatable and editable via the admin OAuth2 app settings.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "VS Code Coder Extension",
			CallbackURL: "vscode://coder.coder-remote/oauth/callback",
		})
		require.NoError(t, err)
		require.Equal(t, "VS Code Coder Extension", app.Name)
		require.Equal(t, "vscode://coder.coder-remote/oauth/callback", app.CallbackURL)

		updated, err := client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        "Cursor (MCP)",
			CallbackURL: "cursor://anysphere.cursor-mcp/oauth/callback",
		})
		require.NoError(t, err)
		require.Equal(t, "Cursor (MCP)", updated.Name)
		require.Equal(t, "cursor://anysphere.cursor-mcp/oauth/callback", updated.CallbackURL)
	})

	t.Run("CallbackURLSchemes", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		tests := []struct {
			name        string
			callbackURL string
			valid       bool
		}{
			{name: "OpaqueLocalhost", callbackURL: "localhost:3000"},
			{name: "SchemeOnly", callbackURL: "vscode:"},
			{name: "ShortSchemeOnly", callbackURL: "a:"},
			{name: "EmptyNativeTarget", callbackURL: "vscode://"},
			{name: "NativeScheme", callbackURL: "vscode://coder.coder-remote/oauth/callback", valid: true},
			{name: "PathOnlyNativeScheme", callbackURL: "com.example.app:/oauth2redirect", valid: true},
			{name: "UppercaseOOBURN", callbackURL: "URN:ietf:wg:oauth:2.0:oob", valid: true},
			{name: "MalformedHTTP", callbackURL: "http:foo"},
			{name: "HostlessHTTP", callbackURL: "http:/callback"},
			{name: "HostlessHTTPS", callbackURL: "https:///callback"},
			{name: "DangerousSchemeMixedCase", callbackURL: "JaVaScRiPt:alert(1)"},
			{name: "DangerousDataSchemeMixedCase", callbackURL: "DaTa:text/plain,invalid"},
			{name: "DangerousFileSchemeMixedCase", callbackURL: "FiLe:///tmp/invalid"},
			{name: "DangerousFTPSchemeMixedCase", callbackURL: "FtP://example.com/invalid"},
			{name: "UnsupportedURN", callbackURL: "urn:example:invalid"},
			{name: "UnsupportedURNMixedCase", callbackURL: "URN:example:invalid"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitLong)
				app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
					Name:        testutil.GetRandomName(t),
					CallbackURL: "https://example.com/callback",
				})
				require.NoError(t, err)

				_, postErr := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
					Name:        testutil.GetRandomName(t),
					CallbackURL: test.callbackURL,
				})
				if test.valid {
					require.NoError(t, postErr)
				} else {
					requireCallbackURLValidationError(t, postErr)
				}

				_, putErr := client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
					Name:        testutil.GetRandomName(t),
					CallbackURL: test.callbackURL,
				})
				if test.valid {
					require.NoError(t, putErr)
				} else {
					requireCallbackURLValidationError(t, putErr)
				}
			})
		}
	})

	t.Run("PublicDCRCallbackURLPolicy", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)

		tests := []struct {
			name        string
			callbackURL string
			valid       bool
		}{
			{name: "Mailto", callbackURL: "mailto:user@example.com"},
			{name: "MailtoHierarchical", callbackURL: "mailto://user@example.com"},
			{name: "TelHierarchical", callbackURL: "tel:/15551234567"},
			{name: "SMSHierarchical", callbackURL: "sms:/15551234567"},
			{name: "Tel", callbackURL: "tel:+15551234567"},
			{name: "SMS", callbackURL: "sms:+15551234567"},
			{name: "ExternalHTTP", callbackURL: "http://example.com/callback"},
			{name: "Fragment", callbackURL: "https://example.com/callback#fragment"},
			{name: "LoopbackHTTP", callbackURL: "http://127.0.0.1:8080/callback", valid: true},
			{name: "Native", callbackURL: "com.example.app:/oauth2redirect", valid: true},
			{name: "OpaqueNative", callbackURL: "com.example.app:oauth2redirect"},
			{name: "HTTPS", callbackURL: "https://example.com/updated-callback", valid: true},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitLong)
				registered, err := client.PostOAuth2ClientRegistration(ctx, codersdk.OAuth2ClientRegistrationRequest{
					ClientName:              testutil.GetRandomName(t),
					RedirectURIs:            []string{"https://example.com/callback"},
					TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethodNone,
				})
				require.NoError(t, err)
				appID, err := uuid.Parse(registered.ClientID)
				require.NoError(t, err)

				updated, err := client.PutOAuth2ProviderApp(ctx, appID, codersdk.PutOAuth2ProviderAppRequest{
					Name:        testutil.GetRandomName(t),
					CallbackURL: test.callbackURL,
				})
				if !test.valid {
					requireCallbackURLValidationError(t, err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, test.callbackURL, updated.CallbackURL)
				require.Equal(t, []string{test.callbackURL}, updated.RedirectURIs)
				require.Equal(t, codersdk.OAuth2ClientTypePublic, updated.ClientType)
				require.True(t, updated.DynamicallyRegistered)

				// The edit replaces the registered URI rather than adding to it.
				stored, err := db.GetOAuth2ProviderAppByID(ctx, appID)
				require.NoError(t, err)
				require.Equal(t, []string{test.callbackURL}, stored.RedirectUris)
			})
		}
	})

	t.Run("DuplicateNames", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Create multiple OAuth2 apps with the same name to verify RFC 7591 compliance
		// RFC 7591 allows multiple apps to have the same name
		appName := fmt.Sprintf("duplicate-name-%d", time.Now().UnixNano()%1000000)

		// Create first app
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app1, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        appName,
			CallbackURL: "http://localhost:3001",
		})
		require.NoError(t, err)
		require.Equal(t, appName, app1.Name)

		// Create second app with the same name
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app2, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        appName,
			CallbackURL: "http://localhost:3002",
		})
		require.NoError(t, err)
		require.Equal(t, appName, app2.Name)

		// Create third app with the same name
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app3, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        appName,
			CallbackURL: "http://localhost:3003",
		})
		require.NoError(t, err)
		require.Equal(t, appName, app3.Name)

		// Verify all apps have different IDs but same name
		require.NotEqual(t, app1.ID, app2.ID)
		require.NotEqual(t, app1.ID, app3.ID)
		require.NotEqual(t, app2.ID, app3.ID)
	})
}

// TestOAuth2ClientRegistrationValidation tests OAuth2 client registration validation
func TestOAuth2ClientRegistrationValidation(t *testing.T) {
	t.Parallel()

	t.Run("ValidURIs", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		validURIs := []string{
			"https://example.com/callback",
			"http://localhost:8080/callback",
			"custom-scheme://app/callback",
		}

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: validURIs,
			ClientName:   fmt.Sprintf("valid-uris-client-%d", time.Now().UnixNano()),
		}

		resp, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.NoError(t, err)
		require.Equal(t, validURIs, resp.RedirectURIs)
	})

	t.Run("InvalidURIs", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string
			uris []string
		}{
			{
				name: "InvalidURL",
				uris: []string{"not-a-url"},
			},
			{
				name: "EmptyFragment",
				uris: []string{"https://example.com/callback#"},
			},
			{
				name: "Fragment",
				uris: []string{"https://example.com/callback#fragment"},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Create new client for each sub-test to avoid shared state issues
				subClient := coderdtest.New(t, nil)
				_ = coderdtest.CreateFirstUser(t, subClient)
				oauth2providertest.EnableDCR(t, subClient)
				subCtx := testutil.Context(t, testutil.WaitLong)

				req := codersdk.OAuth2ClientRegistrationRequest{
					RedirectURIs: tc.uris,
					ClientName:   fmt.Sprintf("invalid-uri-client-%s-%d", tc.name, time.Now().UnixNano()),
				}

				_, err := subClient.PostOAuth2ClientRegistration(subCtx, req)
				require.Error(t, err)
				require.Contains(t, err.Error(), "invalid_client_metadata")
			})
		}
	})

	t.Run("ValidGrantTypes", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   fmt.Sprintf("valid-grant-types-client-%d", time.Now().UnixNano()),
			GrantTypes:   []codersdk.OAuth2ProviderGrantType{codersdk.OAuth2ProviderGrantTypeAuthorizationCode, codersdk.OAuth2ProviderGrantTypeRefreshToken},
		}

		resp, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.NoError(t, err)
		require.Equal(t, req.GrantTypes, resp.GrantTypes)
	})

	t.Run("InvalidGrantTypes", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   fmt.Sprintf("invalid-grant-types-client-%d", time.Now().UnixNano()),
			GrantTypes:   []codersdk.OAuth2ProviderGrantType{"unsupported_grant"},
		}

		_, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_client_metadata")
	})

	t.Run("ValidResponseTypes", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs:  []string{"https://example.com/callback"},
			ClientName:    fmt.Sprintf("valid-response-types-client-%d", time.Now().UnixNano()),
			ResponseTypes: []codersdk.OAuth2ProviderResponseType{codersdk.OAuth2ProviderResponseTypeCode},
		}

		resp, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.NoError(t, err)
		require.Equal(t, req.ResponseTypes, resp.ResponseTypes)
	})

	t.Run("InvalidResponseTypes", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs:  []string{"https://example.com/callback"},
			ClientName:    fmt.Sprintf("invalid-response-types-client-%d", time.Now().UnixNano()),
			ResponseTypes: []codersdk.OAuth2ProviderResponseType{"token"}, // Not supported
		}

		_, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid_client_metadata")
	})
}

// TestOAuth2ProviderAppOperations tests basic CRUD operations for OAuth2 provider apps
func TestOAuth2ProviderAppOperations(t *testing.T) {
	t.Parallel()

	t.Run("DeleteNonExisting", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		another, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := another.OAuth2ProviderApp(ctx, uuid.New())
		require.Error(t, err)
	})

	t.Run("BasicOperations", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		owner := coderdtest.CreateFirstUser(t, client)
		another, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)

		// No apps yet.
		apps, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{})
		require.NoError(t, err)
		require.Len(t, apps, 0)

		// Should be able to add apps.
		expectedApps := generateApps(ctx, t, client, "get-apps")
		expectedOrder := []codersdk.OAuth2ProviderApp{
			expectedApps.Default, expectedApps.NoPort,
			expectedApps.Extra[0], expectedApps.Extra[1], expectedApps.Subdomain,
		}

		// Should get all the apps now.
		apps, err = another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{})
		require.NoError(t, err)
		require.Len(t, apps, 5)
		require.Equal(t, expectedOrder, apps)

		// Should be able to keep the same name when updating.
		req := codersdk.PutOAuth2ProviderAppRequest{
			Name:        expectedApps.Default.Name,
			CallbackURL: "https://coder.com",
			Icon:        "test",
		}
		//nolint:gocritic // OAuth2 app management requires owner permission.
		newApp, err := client.PutOAuth2ProviderApp(ctx, expectedApps.Default.ID, req)
		require.NoError(t, err)
		require.Equal(t, req.Name, newApp.Name)
		require.Equal(t, req.CallbackURL, newApp.CallbackURL)
		require.Equal(t, req.Icon, newApp.Icon)
		require.Equal(t, expectedApps.Default.ID, newApp.ID)

		// The callback is stored as the app's only redirect URI.
		require.Equal(t, []string{req.CallbackURL}, newApp.RedirectURIs)
		stored, err := db.GetOAuth2ProviderAppByID(ctx, newApp.ID)
		require.NoError(t, err)
		require.Equal(t, []string{req.CallbackURL}, stored.RedirectUris)
		require.Equal(t, req.CallbackURL, stored.CallbackURL)

		// Should be able to update name.
		req = codersdk.PutOAuth2ProviderAppRequest{
			Name:        "new-foo",
			CallbackURL: "https://coder.com",
			Icon:        "test",
		}
		//nolint:gocritic // OAuth2 app management requires owner permission.
		newApp, err = client.PutOAuth2ProviderApp(ctx, expectedApps.Default.ID, req)
		require.NoError(t, err)
		require.Equal(t, req.Name, newApp.Name)
		require.Equal(t, req.CallbackURL, newApp.CallbackURL)
		require.Equal(t, req.Icon, newApp.Icon)
		require.Equal(t, expectedApps.Default.ID, newApp.ID)

		// Should be able to get a single app.
		got, err := another.OAuth2ProviderApp(ctx, expectedApps.Default.ID)
		require.NoError(t, err)
		require.Equal(t, newApp, got)

		// Should be able to delete an app.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		err = client.DeleteOAuth2ProviderApp(ctx, expectedApps.Default.ID)
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

	t.Run("Scope", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// No scope means unrestricted, same as before this field existed.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "scope-test-unrestricted",
			CallbackURL: "https://coder.com",
		})
		require.NoError(t, err)
		require.Empty(t, app.Scope)

		// A scope is stored and echoed back, and aliases and duplicates are
		// rewritten to their canonical, deduplicated form.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err = client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "scope-test-scoped",
			CallbackURL: "https://coder.com",
			Scope:       "all workspace:read all",
		})
		require.NoError(t, err)
		require.Equal(t, "coder:all workspace:read", app.Scope)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		got, err := client.OAuth2ProviderApp(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, app.Scope, got.Scope)

		// Updating replaces the allowlist rather than merging with it.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        app.Name,
			CallbackURL: app.CallbackURL,
			Scope:       ptr.Ref("coder:templates.author"),
		})
		require.NoError(t, err)
		require.Equal(t, "coder:templates.author", app.Scope)

		// Omitting scope on update leaves the allowlist untouched.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        app.Name,
			CallbackURL: app.CallbackURL,
		})
		require.NoError(t, err)
		require.Equal(t, "coder:templates.author", app.Scope)

		// An oversized scope on update is rejected and leaves the allowlist
		// untouched.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        app.Name,
			CallbackURL: app.CallbackURL,
			Scope:       ptr.Ref(strings.Repeat("s ", codersdk.OAuth2ScopeListMaxNames+1)),
		})
		require.ErrorContains(t, err, "at most 100 names")
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err = client.OAuth2ProviderApp(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, "coder:templates.author", app.Scope)

		// An explicit empty scope on update clears the allowlist back to
		// unrestricted.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        app.Name,
			CallbackURL: app.CallbackURL,
			Scope:       ptr.Ref(""),
		})
		require.NoError(t, err)
		require.Empty(t, app.Scope)
	})

	t.Run("ScopeSpellingMatchesAcrossOrigins", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// The same allowlist written through either path reads back the same
		// way, even though both store the caller's spelling as given.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		admin, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "scope-origin-admin",
			CallbackURL: "https://coder.com",
			Scope:       "all workspace:read all",
		})
		require.NoError(t, err)

		registered, err := client.PostOAuth2ClientRegistration(ctx, codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://coder.com/callback"},
			ClientName:   "scope-origin-dcr",
			Scope:        "all workspace:read all",
		})
		require.NoError(t, err)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		dcr, err := client.OAuth2ProviderApp(ctx, uuid.MustParse(registered.ClientID))
		require.NoError(t, err)

		require.Equal(t, "coder:all workspace:read", admin.Scope)
		require.Equal(t, admin.Scope, dcr.Scope)
	})

	t.Run("RegistrationOrigin", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		admin, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        testutil.GetRandomName(t),
			CallbackURL: "https://example.com/callback",
		})
		require.NoError(t, err)
		require.False(t, admin.DynamicallyRegistered)

		registered, err := client.PostOAuth2ClientRegistration(ctx, codersdk.OAuth2ClientRegistrationRequest{
			ClientName:   testutil.GetRandomName(t),
			RedirectURIs: []string{"https://example.com/callback"},
		})
		require.NoError(t, err)
		//nolint:gocritic // OAuth2 app management requires owner permission.
		dcr, err := client.OAuth2ProviderApp(ctx, uuid.MustParse(registered.ClientID))
		require.NoError(t, err)
		require.True(t, dcr.DynamicallyRegistered)
	})
}

func requireCallbackURLValidationError(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)
	var apiErr *codersdk.Error
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	require.True(t, slices.ContainsFunc(apiErr.Validations, func(validation codersdk.ValidationError) bool {
		return validation.Field == "callback_url"
	}), "expected callback_url validation error, got: %+v", apiErr.Validations)
}

// requireRedirectURIsValidationError asserts err is an HTTP 400 carrying a
// redirect_uris validation error with the given detail.
func requireRedirectURIsValidationError(t *testing.T, err error, detail string) {
	t.Helper()

	require.Error(t, err)
	var apiErr *codersdk.Error
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	require.Contains(t, apiErr.Validations, codersdk.ValidationError{
		Field:  "redirect_uris",
		Detail: detail,
	}, "expected redirect_uris validation error, got: %+v", apiErr.Validations)
}

// competingRedirectURIWriteStore removes the second redirect URI of an app
// right after the middleware reads it, once armed, and returns the row as it
// was before the removal. This stands in for another admin whose update
// commits while a request is in flight.
type competingRedirectURIWriteStore struct {
	database.Store

	armed atomic.Bool
}

func (s *competingRedirectURIWriteStore) GetOAuth2ProviderAppByID(ctx context.Context, id uuid.UUID) (database.OAuth2ProviderApp, error) {
	app, err := s.Store.GetOAuth2ProviderAppByID(ctx, id)
	if err != nil || !s.armed.CompareAndSwap(true, false) {
		return app, err
	}
	_, err = s.UpdateOAuth2ProviderAppByID(ctx, database.UpdateOAuth2ProviderAppByIDParams{
		ID:                      app.ID,
		UpdatedAt:               app.UpdatedAt,
		Name:                    app.Name,
		Icon:                    app.Icon,
		CallbackURL:             app.RedirectUris[0],
		RedirectUris:            app.RedirectUris[:1],
		ClientType:              app.ClientType,
		DynamicallyRegistered:   app.DynamicallyRegistered,
		ClientSecretExpiresAt:   app.ClientSecretExpiresAt,
		GrantTypes:              app.GrantTypes,
		ResponseTypes:           app.ResponseTypes,
		TokenEndpointAuthMethod: app.TokenEndpointAuthMethod,
		Scope:                   app.Scope,
		Contacts:                app.Contacts,
		ClientUri:               app.ClientUri,
		LogoUri:                 app.LogoUri,
		TosUri:                  app.TosUri,
		PolicyUri:               app.PolicyUri,
		JwksUri:                 app.JwksUri,
		Jwks:                    app.Jwks,
		SoftwareID:              app.SoftwareID,
		SoftwareVersion:         app.SoftwareVersion,
	})
	if err != nil {
		return database.OAuth2ProviderApp{}, err
	}
	return app, nil
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
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         name,
			RedirectURIs: []string{callback},
			Icon:         "",
		})
		require.NoError(t, err)
		require.Equal(t, name, app.Name)
		require.Equal(t, callback, app.CallbackURL)
		require.Equal(t, []string{callback}, app.RedirectURIs)
		return app
	}

	return provisionedApps{
		Default:   create("app-a", "https://localhost1:8080/foo/bar"),
		NoPort:    create("app-b", "https://localhost2"),
		Subdomain: create("app-z", "http://30.localhost:3000"),
		Extra: []codersdk.OAuth2ProviderApp{
			create("app-x", "http://20.localhost:3000"),
			create("app-y", "http://10.localhost:3000"),
		},
	}
}

// TestOAuth2ProviderAppRedirectURIs covers how the admin API reads and writes
// an app's registered redirect URIs now that the list is the source of truth.
func TestOAuth2ProviderAppRedirectURIs(t *testing.T) {
	t.Parallel()

	const (
		first  = "https://a.example.com/callback"
		second = "https://b.example.com/callback"
		third  = "https://c.example.com/callback"
	)

	// A callback-only update replaces the previous primary and keeps the
	// other registered URIs, so editing the callback moves it rather than
	// adding to the set.
	t.Run("LegacyCallbackMovesPrimary", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		registered := oauth2providertest.RegisterPublicClientWithRedirectURIs(t, client, "move-primary", first, second)
		appID := uuid.MustParse(registered.ClientID)

		stored, err := db.GetOAuth2ProviderAppByID(ctx, appID)
		require.NoError(t, err)
		require.Equal(t, []string{first, second}, stored.RedirectUris)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		updated, err := client.PutOAuth2ProviderApp(ctx, appID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        "move-primary",
			CallbackURL: third,
		})
		require.NoError(t, err)
		require.Equal(t, third, updated.CallbackURL)

		stored, err = db.GetOAuth2ProviderAppByID(ctx, appID)
		require.NoError(t, err)
		require.Equal(t, []string{third, second}, stored.RedirectUris)
		require.Equal(t, third, stored.CallbackURL)

		// Sending the current primary again changes nothing.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, appID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        "renamed",
			CallbackURL: third,
		})
		require.NoError(t, err)

		stored, err = db.GetOAuth2ProviderAppByID(ctx, appID)
		require.NoError(t, err)
		require.Equal(t, []string{third, second}, stored.RedirectUris)

		// Choosing an existing alternate as the callback makes it primary
		// and drops the previous one.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, appID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        "renamed",
			CallbackURL: second,
		})
		require.NoError(t, err)

		stored, err = db.GetOAuth2ProviderAppByID(ctx, appID)
		require.NoError(t, err)
		require.Equal(t, []string{second}, stored.RedirectUris)
	})

	// callback_url is read from the list, never from the column.
	t.Run("ColumnIsIgnoredOnRead", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
			CallbackURL:  "https://stale.example.com/callback",
			RedirectUris: []string{first, second},
		})

		//nolint:gocritic // OAuth2 app management requires owner permission.
		got, err := client.OAuth2ProviderApp(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, first, got.CallbackURL)
	})

	// A created app stores its callback as the whole list, and both columns
	// agree.
	t.Run("CreateStoresCallbackAsList", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "create-list",
			CallbackURL: first,
		})
		require.NoError(t, err)

		stored, err := db.GetOAuth2ProviderAppByID(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, []string{first}, stored.RedirectUris)
		require.Equal(t, first, stored.CallbackURL)
	})

	// An oversized callback is refused before it is parsed, on both create
	// and update.
	t.Run("OversizedCallback", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		prefix := "https://example.com/"
		long := prefix + strings.Repeat("a", codersdk.OAuth2RedirectURIMaxBytes-len(prefix)+1)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "too-long",
			CallbackURL: long,
		})
		requireCallbackURLValidationError(t, err)
		require.ErrorContains(t, err, "callback URL must be at most 2048 bytes")

		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "too-long",
			CallbackURL: first,
		})
		require.NoError(t, err)
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        "too-long",
			CallbackURL: long,
		})
		requireCallbackURLValidationError(t, err)
	})

	// A callback the shape check rejects reports the reason against the
	// callback_url field instead of a generic tag failure.
	t.Run("CallbackWithFragment", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "fragment",
			CallbackURL: first + "#fragment",
		})
		requireCallbackURLValidationError(t, err)
		require.ErrorContains(t, err, "callback URL must not contain a fragment component")
	})

	// Stored entries are validated again on every update, so an app whose
	// list predates the caps is rejected even when the callback is unchanged
	// and nothing about it is written.
	t.Run("StoredListOverCount", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		uris := make([]string, 0, codersdk.OAuth2RedirectURIsMaxCount+1)
		uris = append(uris, first)
		for i := 1; i <= codersdk.OAuth2RedirectURIsMaxCount; i++ {
			uris = append(uris, fmt.Sprintf("https://alt-%d.example.com/callback", i))
		}
		app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
			Name:         "over-count",
			CallbackURL:  first,
			RedirectUris: uris,
		})

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err := client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        "renamed",
			CallbackURL: first,
		})
		requireRedirectURIsValidationError(t, err, "at most 32 redirect URIs are allowed")

		stored, err := db.GetOAuth2ProviderAppByID(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, "over-count", stored.Name)
		require.Equal(t, first, stored.CallbackURL)
		require.Equal(t, uris, stored.RedirectUris)
	})

	// An oversized alternate is reported against redirect_uris with its
	// index, since the request never sent it.
	t.Run("StoredAlternateOversized", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		prefix := "https://example.com/"
		long := prefix + strings.Repeat("a", codersdk.OAuth2RedirectURIMaxBytes-len(prefix)+1)
		app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
			Name:         "long-alternate",
			CallbackURL:  first,
			RedirectUris: []string{first, long},
		})

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err := client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        "renamed",
			CallbackURL: first,
		})
		requireRedirectURIsValidationError(t, err, "redirect URI 2 must be at most 2048 bytes")

		stored, err := db.GetOAuth2ProviderAppByID(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, "long-alternate", stored.Name)
		require.Equal(t, first, stored.CallbackURL)
		require.Equal(t, []string{first, long}, stored.RedirectUris)
	})

	// Registration stores a deduplicated list, and the configuration
	// endpoint returns it in that form.
	t.Run("RegistrationDedupsList", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		registered, err := client.PostOAuth2ClientRegistration(ctx, codersdk.OAuth2ClientRegistrationRequest{
			ClientName:   testutil.GetRandomName(t),
			RedirectURIs: []string{first, second, first},
		})
		require.NoError(t, err)
		require.Equal(t, []string{first, second}, registered.RedirectURIs)

		config, err := client.GetOAuth2ClientConfiguration(ctx, registered.ClientID, registered.RegistrationAccessToken)
		require.NoError(t, err)
		require.Equal(t, []string{first, second}, config.RedirectURIs)
	})

	// A stored list with a duplicate, which DCR accepted before the caps
	// existed, is returned deduplicated by both the admin API and the client
	// configuration endpoint.
	t.Run("StoredDuplicateReadsAgree", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		oauth2providertest.EnableDCR(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		registered, err := client.PostOAuth2ClientRegistration(ctx, codersdk.OAuth2ClientRegistrationRequest{
			ClientName:   testutil.GetRandomName(t),
			RedirectURIs: []string{first, second},
		})
		require.NoError(t, err)
		appID, err := uuid.Parse(registered.ClientID)
		require.NoError(t, err)

		// Every write path deduplicates, so the duplicate goes in directly.
		stored, err := db.GetOAuth2ProviderAppByID(ctx, appID)
		require.NoError(t, err)
		_, err = db.UpdateOAuth2ProviderAppByID(ctx, database.UpdateOAuth2ProviderAppByIDParams{
			ID:                      stored.ID,
			UpdatedAt:               stored.UpdatedAt,
			Name:                    stored.Name,
			Icon:                    stored.Icon,
			CallbackURL:             stored.CallbackURL,
			RedirectUris:            []string{first, second, first},
			ClientType:              stored.ClientType,
			DynamicallyRegistered:   stored.DynamicallyRegistered,
			ClientSecretExpiresAt:   stored.ClientSecretExpiresAt,
			GrantTypes:              stored.GrantTypes,
			ResponseTypes:           stored.ResponseTypes,
			TokenEndpointAuthMethod: stored.TokenEndpointAuthMethod,
			Scope:                   stored.Scope,
			Contacts:                stored.Contacts,
			ClientUri:               stored.ClientUri,
			LogoUri:                 stored.LogoUri,
			TosUri:                  stored.TosUri,
			PolicyUri:               stored.PolicyUri,
			JwksUri:                 stored.JwksUri,
			Jwks:                    stored.Jwks,
			SoftwareID:              stored.SoftwareID,
			SoftwareVersion:         stored.SoftwareVersion,
		})
		require.NoError(t, err)

		config, err := client.GetOAuth2ClientConfiguration(ctx, registered.ClientID, registered.RegistrationAccessToken)
		require.NoError(t, err)
		require.Equal(t, []string{first, second}, config.RedirectURIs)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.OAuth2ProviderApp(ctx, appID)
		require.NoError(t, err)
		require.Equal(t, config.RedirectURIs, app.RedirectURIs)
	})

	// Creating and updating an app with the redirect_uris field stores and
	// returns the list as given, with callback_url equal to the first entry.
	t.Run("ExplicitListOnCreateAndUpdate", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "explicit-list",
			RedirectURIs: []string{first, second},
		})
		require.NoError(t, err)
		require.Equal(t, first, app.CallbackURL)
		require.Equal(t, []string{first, second}, app.RedirectURIs)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		updated, err := client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         "explicit-list",
			RedirectURIs: []string{third, second},
		})
		require.NoError(t, err)
		require.Equal(t, third, updated.CallbackURL)
		require.Equal(t, []string{third, second}, updated.RedirectURIs)

		stored, err := db.GetOAuth2ProviderAppByID(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, []string{third, second}, stored.RedirectUris)
	})

	// Omitting both fields on an update keeps the stored list unchanged.
	t.Run("NeitherFieldKeepsStoredList", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "keep-list",
			RedirectURIs: []string{first, second},
		})
		require.NoError(t, err)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		updated, err := client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name: "renamed",
		})
		require.NoError(t, err)
		require.Equal(t, []string{first, second}, updated.RedirectURIs)

		stored, err := db.GetOAuth2ProviderAppByID(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, []string{first, second}, stored.RedirectUris)
	})

	// Sending both fields is a 400 unless callback_url is the first redirect
	// URI, so an edit to redirect_uris is never undone by a stale callback_url.
	t.Run("BothFieldsMustAgree", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "both-fields-create",
			CallbackURL:  second,
			RedirectURIs: []string{first, second},
		})
		requireCallbackURLValidationError(t, err)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "both-fields",
			RedirectURIs: []string{first, second},
		})
		require.NoError(t, err)

		// Echoing the GET body back unchanged still works.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		updated, err := client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         app.Name,
			Icon:         app.Icon,
			CallbackURL:  app.CallbackURL,
			RedirectURIs: app.RedirectURIs,
		})
		require.NoError(t, err)
		require.Equal(t, []string{first, second}, updated.RedirectURIs)

		// Removing the primary from the list while echoing the old callback_url.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         app.Name,
			CallbackURL:  first,
			RedirectURIs: []string{second},
		})
		requireCallbackURLValidationError(t, err)

		// Reordering the list while echoing the old callback_url.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         app.Name,
			CallbackURL:  first,
			RedirectURIs: []string{second, first},
		})
		requireCallbackURLValidationError(t, err)

		// A callback_url that is not in the list at all.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         app.Name,
			CallbackURL:  third,
			RedirectURIs: []string{first, second},
		})
		requireCallbackURLValidationError(t, err)

		stored, err := db.GetOAuth2ProviderAppByID(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, []string{first, second}, stored.RedirectUris)
	})

	// A rename-only update writes the stored redirect URIs back, so a change
	// to them that lands between the middleware read and the update must not
	// be undone.
	t.Run("RenameKeepsConcurrentRedirectURIChange", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		store := &competingRedirectURIWriteStore{Store: db}
		client := coderdtest.New(t, &coderdtest.Options{Database: store, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "concurrent",
			RedirectURIs: []string{first, second},
		})
		require.NoError(t, err)

		store.armed.Store(true)
		//nolint:gocritic // OAuth2 app management requires owner permission.
		updated, err := client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name: "renamed",
		})
		require.NoError(t, err)
		require.False(t, store.armed.Load(), "the competing write did not run")
		require.Equal(t, "renamed", updated.Name)
		require.Equal(t, []string{first}, updated.RedirectURIs)

		stored, err := db.GetOAuth2ProviderAppByID(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, []string{first}, stored.RedirectUris)
	})

	// The cap on redirect_uris is reachable from a single admin request.
	t.Run("ListCap", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Entries are distinct because duplicates are dropped before the count.
		uris := make([]string, 0, codersdk.OAuth2RedirectURIsMaxCount+1)
		for i := range codersdk.OAuth2RedirectURIsMaxCount + 1 {
			uris = append(uris, fmt.Sprintf("https://example.com/callback/%d", i))
		}

		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "at-cap",
			RedirectURIs: uris[:codersdk.OAuth2RedirectURIsMaxCount],
		})
		require.NoError(t, err)
		require.Len(t, app.RedirectURIs, codersdk.OAuth2RedirectURIsMaxCount)

		var sdkErr *codersdk.Error
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "over-cap",
			RedirectURIs: uris,
		})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "redirect_uris", sdkErr.Validations[0].Field)
		require.Equal(t, "at most 32 redirect URIs are allowed", sdkErr.Validations[0].Detail)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         "at-cap",
			RedirectURIs: uris,
		})
		require.ErrorAs(t, err, &sdkErr)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "redirect_uris", sdkErr.Validations[0].Field)
		require.Equal(t, "at most 32 redirect URIs are allowed", sdkErr.Validations[0].Detail)
	})

	// The admin path applies the same transport rule as dynamic client
	// registration, so an admin cannot store a cleartext target that a client
	// could not register for itself.
	t.Run("CleartextHTTPIsRefused", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		var sdkErr *codersdk.Error
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "cleartext-create",
			RedirectURIs: []string{"http://plaintext.example.com/callback"},
		})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "redirect_uris", sdkErr.Validations[0].Field)
		require.Equal(t, "redirect URI 1 must use https scheme for non-localhost URLs", sdkErr.Validations[0].Detail)

		// The row number names the offending entry rather than the first one.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "cleartext-create-second",
			RedirectURIs: []string{first, "http://plaintext.example.com/callback"},
		})
		sdkErr = nil
		require.ErrorAs(t, err, &sdkErr)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "redirect URI 2 must use https scheme for non-localhost URLs", sdkErr.Validations[0].Detail)

		// The deprecated field is reported against its own name.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "cleartext-create-callback",
			CallbackURL: "http://plaintext.example.com/callback",
		})
		sdkErr = nil
		require.ErrorAs(t, err, &sdkErr)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "callback_url", sdkErr.Validations[0].Field)
		require.Equal(t, "callback URL must use https scheme for non-localhost URLs", sdkErr.Validations[0].Detail)

		// Loopback and localhost subdomains stay usable for local development,
		// and https to any host is unaffected.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name: "cleartext-allowed-forms",
			RedirectURIs: []string{
				"http://localhost:3000/callback",
				"http://127.0.0.1:3000/callback",
				"http://app.localhost/callback",
				"https://plaintext.example.com/callback",
			},
		})
		require.NoError(t, err)
		require.Len(t, app.RedirectURIs, 4)

		// An update is checked the same way.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         "cleartext-allowed-forms",
			RedirectURIs: []string{"http://plaintext.example.com/callback"},
		})
		sdkErr = nil
		require.ErrorAs(t, err, &sdkErr)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "redirect_uris", sdkErr.Validations[0].Field)
		require.Equal(t, "redirect URI 1 must use https scheme for non-localhost URLs", sdkErr.Validations[0].Detail)
	})

	t.Run("LegacyCleartextHTTPCallback", func(t *testing.T) {
		t.Parallel()

		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: pubsub})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		const legacy = "http://intranet.example.com/callback"
		app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
			Name:         "legacy-cleartext",
			CallbackURL:  legacy,
			RedirectUris: []string{legacy},
		})

		query := authorizeQuery(t, app.ID.String(), "")
		query.Set("redirect_uri", legacy)
		resp := sendAuthorizeRequest(ctx, t, client, http.MethodGet, query)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, readBody(t, resp))

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err := client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name: "renamed",
		})
		requireRedirectURIsValidationError(t, err, "redirect URI 1 must use https scheme for non-localhost URLs")

		stored, err := db.GetOAuth2ProviderAppByID(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, "legacy-cleartext", stored.Name)
		require.Equal(t, legacy, stored.CallbackURL)
		require.Equal(t, []string{legacy}, stored.RedirectUris)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		updated, err := client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         "renamed",
			RedirectURIs: []string{first, second},
		})
		require.NoError(t, err)
		require.Equal(t, "renamed", updated.Name)
		require.Equal(t, first, updated.CallbackURL)
		require.Equal(t, []string{first, second}, updated.RedirectURIs)

		stored, err = db.GetOAuth2ProviderAppByID(ctx, app.ID)
		require.NoError(t, err)
		require.Equal(t, first, stored.CallbackURL)
		require.Equal(t, []string{first, second}, stored.RedirectUris)
	})

	// An update sending redirect_uris as an empty list is refused rather than
	// silently keeping the stored list, with or without callback_url.
	t.Run("EmptyListIsRefused", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "empty-list-update",
			RedirectURIs: []string{first},
		})
		require.NoError(t, err)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         "empty-list-update",
			RedirectURIs: []string{},
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "redirect_uris", sdkErr.Validations[0].Field)
		require.Equal(t, "at least one redirect URI is required", sdkErr.Validations[0].Detail)

		// With callback_url in the same request, the message says the empty
		// list is what discarded it.
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         "empty-list-update",
			CallbackURL:  second,
			RedirectURIs: []string{},
		})
		sdkErr = nil
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "redirect_uris", sdkErr.Validations[0].Field)
		require.Equal(t, "redirect_uris was sent as an empty list, which overrides callback_url; send at least one redirect URI, or omit redirect_uris to use callback_url", sdkErr.Validations[0].Detail)
	})

	// The web UI only rejects a fragment for public clients, so a confidential
	// app's fragment reaches the server. The row number in the message counts
	// from one, matching the form labels, rather than the zero-based index.
	t.Run("FragmentNamesVisibleRow", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		app, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "fragment-row",
			RedirectURIs: []string{first},
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.OAuth2ClientTypeConfidential, app.ClientType)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PutOAuth2ProviderApp(ctx, app.ID, codersdk.PutOAuth2ProviderAppRequest{
			Name:         app.Name,
			RedirectURIs: []string{first, "https://example.com/callback#state"},
		})
		requireRedirectURIsValidationError(t, err, "redirect URI 2 must not contain a fragment component")
	})

	// A create that sends neither URI field is refused.
	t.Run("OmittedFieldsOnCreateRefused", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		var sdkErr *codersdk.Error
		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name: "omitted-fields-create",
		})
		require.ErrorAs(t, err, &sdkErr)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "redirect_uris", sdkErr.Validations[0].Field)
		require.Equal(t, "at least one redirect URI is required", sdkErr.Validations[0].Detail)
	})

	// A malformed entry sent through redirect_uris is attributed to that
	// field with its row number; the same value sent through callback_url is
	// attributed to callback_url instead.
	t.Run("AttributionByOriginField", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err := client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:         "bad-list-entry",
			RedirectURIs: []string{first, "javascript:alert(1)"},
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "redirect_uris", sdkErr.Validations[0].Field)
		require.Equal(t, "redirect URI 2 uses the dangerous scheme javascript", sdkErr.Validations[0].Detail)

		//nolint:gocritic // OAuth2 app management requires owner permission.
		_, err = client.PostOAuth2ProviderApp(ctx, codersdk.PostOAuth2ProviderAppRequest{
			Name:        "bad-callback",
			CallbackURL: "javascript:alert(1)",
		})
		requireCallbackURLValidationError(t, err)
		require.ErrorContains(t, err, "callback URL uses the dangerous scheme javascript")
	})
}
