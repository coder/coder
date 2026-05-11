package coderd_test

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestExternalAuthHeaderAutoCreateE2E exercises the auto-create code
// path end to end through coderdtest. It verifies that an unknown
// user asserted via the Coder-Authorization header is provisioned,
// receives the configured default roles, and can immediately access
// authenticated routes.
func TestExternalAuthHeaderAutoCreateE2E(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Dangerous.AllowExternalAuthHeader = true
	dv.Dangerous.AllowExternalAuthHeaderAutoCreateUsers = true

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:         dv,
		ExternalAuthHeaderConfig: trustedExternalAuthHeaderConfig(t, []string{codersdk.RoleAuditor}),
	})
	// CreateFirstUser is required so the default org and an admin
	// exist; the auto-created user will be added to that org.
	_ = coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	resp := doExternalAuthHeaderRequest(ctx, t, client.URL.String(),
		"Basic UserEmail=external-newcomer@example.com, Name=External Newcomer")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "auto-create + auth should succeed")

	// Resolve the freshly-created user to verify roles, status, and
	// login type ended up correct.
	created, err := client.User(ctx, "external-newcomer")
	require.NoError(t, err)
	require.Equal(t, "external-newcomer@example.com", created.Email)
	require.Equal(t, codersdk.UserStatusActive, created.Status)
	require.Equal(t, codersdk.LoginTypeNone, created.LoginType)
	require.Contains(t, roleNames(created.Roles), codersdk.RoleAuditor)
}

// TestExternalAuthHeaderAutoCreateHeaderRolesWin verifies that the
// header's Roles= field overrides the deployment default-role list.
func TestExternalAuthHeaderAutoCreateHeaderRolesWin(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Dangerous.AllowExternalAuthHeader = true
	dv.Dangerous.AllowExternalAuthHeaderAutoCreateUsers = true

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:         dv,
		ExternalAuthHeaderConfig: trustedExternalAuthHeaderConfig(t, []string{codersdk.RoleAuditor}),
	})
	_ = coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	resp := doExternalAuthHeaderRequest(ctx, t, client.URL.String(),
		"Basic UserEmail=role-override@example.com, Roles=user-admin")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	created, err := client.User(ctx, "role-override")
	require.NoError(t, err)
	roles := roleNames(created.Roles)
	require.Contains(t, roles, codersdk.RoleUserAdmin)
	require.NotContains(t, roles, codersdk.RoleAuditor)
}

// TestExternalAuthHeaderAutoCreateInvalidRoleRejected ensures the
// helper refuses unknown role names rather than silently dropping
// them, matching the conservative posture for a dangerous flag.
func TestExternalAuthHeaderAutoCreateInvalidRoleRejected(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Dangerous.AllowExternalAuthHeader = true
	dv.Dangerous.AllowExternalAuthHeaderAutoCreateUsers = true

	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:         dv,
		ExternalAuthHeaderConfig: trustedExternalAuthHeaderConfig(t, nil),
	})
	_ = coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	resp := doExternalAuthHeaderRequest(ctx, t, client.URL.String(),
		"Basic UserEmail=bogus-role@example.com, Roles=not-a-real-role")
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// trustedExternalAuthHeaderConfig returns a ready-to-use config that
// trusts the httptest loopback range so test requests are accepted.
func trustedExternalAuthHeaderConfig(t *testing.T, defaultRoles []string) httpmw.ExternalAuthHeaderConfig {
	t.Helper()
	_, trustedNet, err := net.ParseCIDR("127.0.0.0/8")
	require.NoError(t, err)
	return httpmw.ExternalAuthHeaderConfig{
		Enabled:                true,
		TrustedOrigins:         []*net.IPNet{trustedNet},
		AllowAutoCreateUsers:   true,
		AutoCreateDefaultRoles: defaultRoles,
	}
}

// doExternalAuthHeaderRequest issues a GET to /api/v2/users/me with
// only the Coder-Authorization header set. It bypasses the codersdk
// client because the client would attach a session cookie.
func doExternalAuthHeaderRequest(ctx context.Context, t *testing.T, baseURL, headerValue string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v2/users/me", nil)
	require.NoError(t, err)
	req.Header.Set(httpmw.ExternalAuthHeaderName, headerValue)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// roleNames flattens a list of codersdk.SlimRole into just the names
// for membership assertions.
func roleNames(roles []codersdk.SlimRole) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, r.Name)
	}
	return out
}
