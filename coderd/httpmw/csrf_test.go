package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justinas/nosurf"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

func TestCSRFExemptList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name   string
		URL    string
		Exempt bool
	}{
		{
			Name:   "Root",
			URL:    "https://example.com",
			Exempt: true,
		},
		{
			Name:   "WorkspacePage",
			URL:    "https://coder.com/workspaces",
			Exempt: true,
		},
		{
			Name:   "SubApp",
			URL:    "https://app--dev--coder--user--apps.coder.com/",
			Exempt: true,
		},
		{
			Name:   "PathApp",
			URL:    "https://coder.com/@USER/test.instance/apps/app",
			Exempt: true,
		},
		{
			Name:   "API",
			URL:    "https://coder.com/api/v2",
			Exempt: false,
		},
		{
			Name:   "APIMe",
			URL:    "https://coder.com/api/v2/me",
			Exempt: false,
		},
		{
			Name:   "OAuth2Authorize",
			URL:    "https://coder.com/oauth2/authorize",
			Exempt: false,
		},
		{
			Name:   "OAuth2AuthorizeQuery",
			URL:    "https://coder.com/oauth2/authorize?client_id=test",
			Exempt: false,
		},
		{
			Name:   "OAuth2Tokens",
			URL:    "https://coder.com/oauth2/tokens",
			Exempt: true,
		},
		{
			Name:   "OAuth2Register",
			URL:    "https://coder.com/oauth2/register",
			Exempt: true,
		},
		{
			Name:   "APIRepeatedLeadingSlash",
			URL:    "https://coder.com//api/v2/me",
			Exempt: false,
		},
		{
			Name:   "APIRepeatedInnerSlash",
			URL:    "https://coder.com/api//v2/me",
			Exempt: false,
		},
		{
			Name:   "OAuth2AuthorizeRepeatedLeadingSlash",
			URL:    "https://coder.com//oauth2/authorize",
			Exempt: false,
		},
		{
			Name:   "OAuth2AuthorizeRepeatedInnerSlash",
			URL:    "https://coder.com/oauth2//authorize?client_id=test",
			Exempt: false,
		},
		{
			Name:   "CSPReports",
			URL:    "https://coder.com/api/v2/csp/reports",
			Exempt: true,
		},
		{
			Name:   "FirstUser",
			URL:    "https://coder.com/api/v2/users/first",
			Exempt: true,
		},
		{
			Name:   "WorkspaceAgentsMe",
			URL:    "https://coder.com/api/v2/workspaceagents/me/rpc",
			Exempt: true,
		},
		{
			Name:   "WorkspaceAgentsMeLogs",
			URL:    "https://coder.com/api/v2/workspaceagents/me/logs",
			Exempt: true,
		},
		{
			Name:   "AWSInstanceIdentity",
			URL:    "https://coder.com/api/v2/workspaceagents/aws-instance-identity",
			Exempt: true,
		},
		{
			Name:   "WorkspaceProxiesMe",
			URL:    "https://coder.com/api/v2/workspaceproxies/me/register",
			Exempt: true,
		},
		{
			Name:   "DERP",
			URL:    "https://coder.com/derp",
			Exempt: true,
		},
		{
			Name:   "SCIM",
			URL:    "https://coder.com/scim/v2/Users",
			Exempt: true,
		},
		{
			Name:   "ProvisionerDaemonsServe",
			URL:    "https://coder.com/api/v2/organizations/default/provisionerdaemons/serve",
			Exempt: true,
		},
		// The old patterns matched these by substring. A cookie-authenticated
		// request to any of them must be checked.
		{
			Name:   "UsernameContainingDerp",
			URL:    "https://coder.com/api/v2/users/derp-x/roles",
			Exempt: false,
		},
		{
			Name:   "UsernameWithFirstPrefix",
			URL:    "https://coder.com/api/v2/users/firstx/roles",
			Exempt: false,
		},
		{
			Name:   "OrgMemberContainingDerp",
			URL:    "https://coder.com/api/v2/organizations/8d3e19b7-4b9e-4a25-a367-927384ee6c2f/members/derpy/roles",
			Exempt: false,
		},
		{
			Name:   "WorkspaceAgentsDevcontainerRecreate",
			URL:    "https://coder.com/api/v2/workspaceagents/8d3e19b7-4b9e-4a25-a367-927384ee6c2f/containers/devcontainers/dc/recreate",
			Exempt: false,
		},
		{
			Name:   "WorkspaceAgentsDevcontainerDelete",
			URL:    "https://coder.com/api/v2/workspaceagents/8d3e19b7-4b9e-4a25-a367-927384ee6c2f/containers/devcontainers/dc",
			Exempt: false,
		},
		{
			Name:   "WorkspaceProxyNameStartingWithMe",
			URL:    "https://coder.com/api/v2/workspaceproxies/me-east",
			Exempt: false,
		},
		{
			Name:   "InstanceIdentityWithSuffix",
			URL:    "https://coder.com/api/v2/workspaceagents/aws-instance-identity/extra",
			Exempt: false,
		},
		{
			Name:   "SCIMUnderAPI",
			URL:    "https://coder.com/api/v2/scim/v2/Users",
			Exempt: false,
		},
		{
			Name:   "WorkspaceAgentsMeRepeatedLeadingSlash",
			URL:    "https://coder.com//api/v2/workspaceagents/me/rpc",
			Exempt: false,
		},
	}

	mw := httpmw.CSRF(codersdk.HTTPCookieConfig{})
	csrfmw := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).(*nosurf.CSRFHandler)

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.URL, nil)
			require.NoError(t, err)

			r.AddCookie(&http.Cookie{Name: codersdk.SessionTokenCookie, Value: "test"})
			exempt := csrfmw.IsExempt(r)
			require.Equal(t, c.Exempt, exempt)
		})
	}

	// Agents, workspace proxies, provisioner daemons and SCIM clients
	// authenticate with headers and never send a session cookie, so they
	// are exempt whatever the pattern list says.
	t.Run("NoSessionCookie", func(t *testing.T) {
		t.Parallel()

		for _, u := range []string{
			"https://coder.com/api/v2/workspaceagents/me/rpc",
			"https://coder.com/api/v2/workspaceproxies/me/register",
			"https://coder.com/api/v2/organizations/default/provisionerdaemons/serve",
			"https://coder.com/api/v2/users/first",
			"https://coder.com/api/v2/workspaceagents/8d3e19b7-4b9e-4a25-a367-927384ee6c2f/containers/devcontainers/dc/recreate",
		} {
			r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, u, nil)
			require.NoError(t, err)
			require.True(t, csrfmw.IsExempt(r), "no-cookie request to %s should be exempt", u)
		}
	})
}

// TestCSRFError verifies the error message returned to a user when CSRF
// checks fail.
//
//nolint:bodyclose // Using httptest.Recorders
func TestCSRFError(t *testing.T) {
	t.Parallel()

	// Hard coded matching CSRF values
	const csrfCookieValue = "JXm9hOUdZctWt0ZZGAy9xiS/gxMKYOThdxjjMnMUyn4="
	const csrfHeaderValue = "KNKvagCBEHZK7ihe2t7fj6VeJ0UyTDco1yVUJE8N06oNqxLu5Zx1vRxZbgfC0mJJgeGkVjgs08mgPbcWPBkZ1A=="
	// Use a url with "/api" as the root, other routes bypass CSRF.
	const urlPath = "https://coder.com/api/v2/hello"

	var handler http.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	handler = httpmw.CSRF(codersdk.HTTPCookieConfig{})(handler)

	// Not testing the error case, just providing the example of things working
	// to base the failure tests off of.
	t.Run("ValidCSRF", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, urlPath, nil)
		require.NoError(t, err)

		req.AddCookie(&http.Cookie{Name: codersdk.SessionTokenCookie, Value: "session_token_value"})
		req.AddCookie(&http.Cookie{Name: nosurf.CookieName, Value: csrfCookieValue})
		req.Header.Add(nosurf.HeaderName, csrfHeaderValue)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		resp := rec.Result()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// The classic CSRF failure returns the generic error.
	t.Run("MissingCSRFHeader", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, urlPath, nil)
		require.NoError(t, err)

		req.AddCookie(&http.Cookie{Name: codersdk.SessionTokenCookie, Value: "session_token_value"})
		req.AddCookie(&http.Cookie{Name: nosurf.CookieName, Value: csrfCookieValue})

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		resp := rec.Result()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Contains(t, rec.Body.String(), "Something is wrong with your CSRF token.")
	})

	// Include the CSRF cookie, but not the CSRF header value.
	// Including the 'codersdk.SessionTokenHeader' will bypass CSRF only if
	// it matches the cookie. If it does not, we expect a more helpful error.
	t.Run("MismatchedHeaderAndCookie", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, urlPath, nil)
		require.NoError(t, err)

		req.AddCookie(&http.Cookie{Name: codersdk.SessionTokenCookie, Value: "session_token_value"})
		req.AddCookie(&http.Cookie{Name: nosurf.CookieName, Value: csrfCookieValue})
		req.Header.Add(codersdk.SessionTokenHeader, "mismatched_value")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		resp := rec.Result()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Contains(t, rec.Body.String(), "CSRF error encountered. Authentication via")
	})

	// A forged request to a path the old "derp/*" pattern exempted. The
	// browser attaches both cookies but cannot add the header.
	t.Run("FormerlyExemptPath", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPut,
			"https://coder.com/api/v2/users/derp-x/roles", strings.NewReader(`{"roles":["owner"]}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("Origin", "https://app--dev--coder--attacker--apps.coder.com")
		req.AddCookie(&http.Cookie{Name: codersdk.SessionTokenCookie, Value: "session_token_value"})
		req.AddCookie(&http.Cookie{Name: nosurf.CookieName, Value: csrfCookieValue})

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		resp := rec.Result()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Contains(t, rec.Body.String(), "Something is wrong with your CSRF token.")
	})
}
