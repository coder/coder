package workspaceapps_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
)

func TestAppCookies(t *testing.T) {
	t.Parallel()

	const (
		domain                  = "example.com"
		hash                    = "a379a6f6eeafb9a55e378c118034e275"
		expectedSubdomainCookie = codersdk.SubdomainAppSessionTokenCookie + "_" + hash
	)

	cookies := workspaceapps.NewAppCookies(domain)
	require.Equal(t, codersdk.PathAppSessionTokenCookie, cookies.PathAppSessionToken)
	require.Equal(t, expectedSubdomainCookie, cookies.SubdomainAppSessionToken)
	require.Equal(t, codersdk.SignedAppTokenCookie, cookies.SignedAppToken)

	require.Equal(t, cookies.PathAppSessionToken, cookies.CookieNameForAccessMethod(workspaceapps.AccessMethodPath))
	require.Equal(t, cookies.PathAppSessionToken, cookies.CookieNameForAccessMethod(workspaceapps.AccessMethodTerminal))
	require.Equal(t, cookies.SubdomainAppSessionToken, cookies.CookieNameForAccessMethod(workspaceapps.AccessMethodSubdomain))

	// A new cookies object with a different domain should have a different
	// subdomain cookie.
	newCookies := workspaceapps.NewAppCookies("different.com")
	require.NotEqual(t, cookies.SubdomainAppSessionToken, newCookies.SubdomainAppSessionToken)
}

func TestAppCookies_TokenFromRequest_PrefersAppCookieOverAuthorizationBearer(t *testing.T) {
	t.Parallel()

	cookies := workspaceapps.NewAppCookies("apps.example.com")

	req := httptest.NewRequest("GET", "https://8081--agent--workspace--user.apps.example.com/", nil)
	req.Header.Set("Authorization", "Bearer whatever")
	req.AddCookie(&http.Cookie{
		Name:  cookies.CookieNameForAccessMethod(workspaceapps.AccessMethodSubdomain),
		Value: "subdomain-session-token",
	})

	got := cookies.TokenFromRequest(req, workspaceapps.AccessMethodSubdomain)
	require.Equal(t, "subdomain-session-token", got)
}
