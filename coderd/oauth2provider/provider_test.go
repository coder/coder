package oauth2provider_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/oauth2provider/oauth2providertest"
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

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		another, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)

		// No apps yet.
		apps, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{})
		require.NoError(t, err)
		require.Len(t, apps.Apps, 0)
		require.Equal(t, 0, apps.Count)

		// Should be able to add apps.
		expectedApps := generateApps(ctx, t, client, "get-apps")
		expectedOrder := []codersdk.OAuth2ProviderApp{
			expectedApps.Default, expectedApps.NoPort,
			expectedApps.Extra[0], expectedApps.Extra[1], expectedApps.Subdomain,
		}

		// Should get all the apps now.
		apps, err = another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{})
		require.NoError(t, err)
		require.Len(t, apps.Apps, 5)
		require.Equal(t, 5, apps.Count)
		require.Equal(t, expectedOrder, apps.Apps)

		// Should be able to search by name.
		filtered, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			SearchQuery: expectedApps.Default.Name,
		})
		require.NoError(t, err)
		require.Len(t, filtered.Apps, 1)
		require.Equal(t, 1, filtered.Count)
		require.Equal(t, expectedApps.Default.ID, filtered.Apps[0].ID)

		// Should be able to paginate.
		page, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			Pagination: codersdk.Pagination{Limit: 2, Offset: 0},
		})
		require.NoError(t, err)
		require.Len(t, page.Apps, 2)
		require.Equal(t, 5, page.Count)
		require.Equal(t, expectedOrder[:2], page.Apps)

		// Walking the cursor must report the same total on every page. The count
		// describes the whole matching set, so it must not shrink as the cursor
		// advances past rows.
		cursor := page.Apps[len(page.Apps)-1].ID
		secondPage, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			Pagination: codersdk.Pagination{Limit: 2, AfterID: cursor},
		})
		require.NoError(t, err)
		require.Equal(t, expectedOrder[2:4], secondPage.Apps)
		require.Equal(t, 5, secondPage.Count)

		cursor = secondPage.Apps[len(secondPage.Apps)-1].ID
		thirdPage, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			Pagination: codersdk.Pagination{Limit: 2, AfterID: cursor},
		})
		require.NoError(t, err)
		require.Equal(t, expectedOrder[4:], thirdPage.Apps)
		require.Equal(t, 5, thirdPage.Count)

		cursor = thirdPage.Apps[len(thirdPage.Apps)-1].ID
		emptyPage, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			Pagination: codersdk.Pagination{Limit: 2, AfterID: cursor},
		})
		require.NoError(t, err)
		require.Empty(t, emptyPage.Apps)
		require.Equal(t, 5, emptyPage.Count)

		pastEnd, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			Pagination: codersdk.Pagination{Limit: 2, Offset: 10},
		})
		require.NoError(t, err)
		require.Empty(t, pastEnd.Apps)
		require.Equal(t, 5, pastEnd.Count)

		// The count also ignores the cursor when a search narrows the set.
		searched, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			SearchQuery: "app-",
			Pagination:  codersdk.Pagination{Limit: 2, AfterID: expectedOrder[0].ID},
		})
		require.NoError(t, err)
		require.Equal(t, expectedOrder[1:3], searched.Apps)
		require.Equal(t, 5, searched.Count)

		// Should be able to keep the same name when updating.
		req := codersdk.PutOAuth2ProviderAppRequest{
			Name:        expectedApps.Default.Name,
			CallbackURL: "http://coder.com",
			Icon:        "test",
		}
		//nolint:gocritic // OAuth2 app management requires owner permission.
		newApp, err := client.PutOAuth2ProviderApp(ctx, expectedApps.Default.ID, req)
		require.NoError(t, err)
		require.Equal(t, req.Name, newApp.Name)
		require.Equal(t, req.CallbackURL, newApp.CallbackURL)
		require.Equal(t, req.Icon, newApp.Icon)
		require.Equal(t, expectedApps.Default.ID, newApp.ID)

		// Should be able to update name.
		req = codersdk.PutOAuth2ProviderAppRequest{
			Name:        "new-foo",
			CallbackURL: "http://coder.com",
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
		require.Len(t, newApps.Apps, 4)
		require.Equal(t, 4, newApps.Count)

		require.Equal(t, expectedOrder[1:], newApps.Apps)
	})

	t.Run("SearchByURL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		another, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)
		apps := generateApps(ctx, t, client, "search-url")

		// A pasted callback URL matches on the callback URL instead of failing to
		// parse because of the ':' in its scheme and port.
		found, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			SearchQuery: apps.Default.CallbackURL,
		})
		require.NoError(t, err)
		require.Len(t, found.Apps, 1)
		require.Equal(t, 1, found.Count)
		require.Equal(t, apps.Default.ID, found.Apps[0].ID)

		// The url filter matches the callback URL explicitly.
		found, err = another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			SearchQuery: fmt.Sprintf("url:%s", apps.Subdomain.CallbackURL),
		})
		require.NoError(t, err)
		require.Len(t, found.Apps, 1)
		require.Equal(t, apps.Subdomain.ID, found.Apps[0].ID)

		// A URL is matched no matter where it appears in the query, and is
		// combined with the other terms.
		found, err = another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			SearchQuery: fmt.Sprintf("%s %s", apps.Subdomain.Name, apps.Subdomain.CallbackURL),
		})
		require.NoError(t, err)
		require.Len(t, found.Apps, 1)
		require.Equal(t, apps.Subdomain.ID, found.Apps[0].ID)

		// A name and URL belonging to different apps match nothing.
		found, err = another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			SearchQuery: fmt.Sprintf("%s %s", apps.Default.Name, apps.Subdomain.CallbackURL),
		})
		require.NoError(t, err)
		require.Empty(t, found.Apps)
		require.Equal(t, 0, found.Count)
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
		require.Len(t, apps.Apps, 0)
		require.Equal(t, 0, apps.Count)
	})

	t.Run("ByUserRejectsSearchAndPagination", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		another, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)
		_ = generateApps(ctx, t, client, "by-user-reject")

		// Search and pagination are not supported alongside user_id and must be
		// rejected rather than silently ignored.
		_, err := another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			UserID:      user.ID,
			SearchQuery: "anything",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

		_, err = another.OAuth2ProviderApps(ctx, codersdk.OAuth2ProviderAppFilter{
			UserID:     user.ID,
			Pagination: codersdk.Pagination{Limit: 2},
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})
}

// Helper functions

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
