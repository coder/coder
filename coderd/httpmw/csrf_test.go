package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		// Exact-path exemptions.
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
		// Non-/api paths are exempt via the ExemptFunc prefix check.
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
		// Regression tests for the removed unanchored regex exemptions
		// (previously exempt as substring matches): cookie-authenticated
		// requests on these paths MUST be CSRF-protected. Attacker-chosen
		// names (usernames, org names, task names) may legally contain
		// substrings like "derp" or a "first" prefix.
		{
			Name:   "UsernameContainingDerp",
			URL:    "https://coder.com/api/v2/users/derp-attacker/workspaces",
			Exempt: false,
		},
		{
			Name:   "UsernameWithFirstPrefix",
			URL:    "https://coder.com/api/v2/users/firstuser/keys",
			Exempt: false,
		},
		{
			Name:   "TaskUserContainingDerp",
			URL:    "https://coder.com/api/v2/tasks/derp-attacker",
			Exempt: false,
		},
		{
			Name:   "OrgNameContainingDerp",
			URL:    "https://coder.com/api/v2/organizations/derp/members/someone/workspaces",
			Exempt: false,
		},
		{
			Name:   "WorkspaceAgentsDevcontainerRecreate",
			URL:    "https://coder.com/api/v2/workspaceagents/8d3e19b7-4b9e-4a25-a367-927384ee6c2f/containers/devcontainers/dc/recreate",
			Exempt: false,
		},
		{
			Name:   "WorkspaceAgentsMe",
			URL:    "https://coder.com/api/v2/workspaceagents/me/rpc",
			Exempt: false,
		},
		{
			Name:   "WorkspaceProxiesMe",
			URL:    "https://coder.com/api/v2/workspaceproxies/me/register",
			Exempt: false,
		},
		{
			Name:   "ProvisionerDaemons",
			URL:    "https://coder.com/api/v2/organizations/default/provisionerdaemons",
			Exempt: false,
		},
		{
			Name:   "SCIMUnderAPI",
			URL:    "https://coder.com/api/v2/scim/v2/Users",
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

	// Requests without a session cookie are not CSRF-relevant and are exempt
	// via the ExemptFunc. This models agents, workspace proxies, and
	// provisioner daemons, which authenticate with headers/PSK and carry no
	// cookies; the removed regex exemptions for those routes were redundant
	// with this behavior.
	t.Run("NoSessionCookie", func(t *testing.T) {
		t.Parallel()

		for _, u := range []string{
			"https://coder.com/api/v2/workspaceagents/me/rpc",
			"https://coder.com/api/v2/workspaceproxies/me/register",
			"https://coder.com/api/v2/organizations/default/provisionerdaemons",
			"https://coder.com/api/v2/users/first",
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
}
