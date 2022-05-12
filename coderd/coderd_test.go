package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestBuildInfo(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	buildInfo, err := client.BuildInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, buildinfo.ExternalURL(), buildInfo.ExternalURL, "external URL")
	require.Equal(t, buildinfo.Version(), buildInfo.Version, "version")
}

// TestAuthorizeAllEndpoints will check `authorize` is called on every endpoint registered.
func TestAuthorizeAllEndpoints(t *testing.T) {
	t.Parallel()

	authorizer := &fakeAuthorizer{}
	srv, client := coderdtest.NewMemoryCoderd(t, &coderdtest.Options{
		Authorizer: authorizer,
	})
	admin := coderdtest.CreateFirstUser(t, client)
	var _ = admin

	// skipRoutes allows skipping routes from being checked.
	type routeCheck struct {
		NoAuthorize  bool
		AssertObject rbac.Object
	}
	assertRoute := map[string]routeCheck{
		"GET:/api/v2":                   {NoAuthorize: true},
		"GET:/api/v2/buildinfo":         {NoAuthorize: true},
		"GET:/api/v2/users/first":       {NoAuthorize: true},
		"POST:/api/v2/users/first":      {NoAuthorize: true},
		"POST:/api/v2/users/login":      {NoAuthorize: true},
		"POST:/api/v2/users/logout":     {NoAuthorize: true},
		"GET:/api/v2/users/authmethods": {NoAuthorize: true},

		// All workspaceagents endpoints do not use rbac
		"POST:/api/v2/workspaceagents/aws-instance-identity":      {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/azure-instance-identity":    {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/google-instance-identity":   {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/gitsshkey":                {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/iceservers":               {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/listen":                   {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/metadata":                 {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/turn":                     {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}":            {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/":           {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/dial":       {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/iceservers": {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/pty":        {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/turn":       {NoAuthorize: true},

		// TODO: @emyrk these need to be fixed by adding authorize calls
		"/api/v2/organizations/{organization}/provisionerdaemons": {NoAuthorize: true},
		"GET:/api/v2/organizations/{organization}":                {AssertObject: rbac.ResourceOrganization.InOrg(admin.OrganizationID)},
	}

	c := srv.Config.Handler.(*chi.Mux)
	err := chi.Walk(c, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		name := method + ":" + route
		t.Run(name, func(t *testing.T) {
			authorizer.reset()
			routeAssertions, ok := assertRoute[strings.TrimRight(name, "/")]
			if !ok {
				// By default, all omitted routes check for just "authorize" called
				routeAssertions = routeCheck{}
			}

			// Replace all url params with known values
			route = strings.ReplaceAll(route, "{organization}", admin.OrganizationID.String())
			route = strings.ReplaceAll(route, "{user}", admin.UserID.String())

			resp, err := client.Request(context.Background(), method, route, nil)
			require.NoError(t, err, "do req")
			_ = resp.Body.Close()

			if !routeAssertions.NoAuthorize {
				assert.NotNil(t, authorizer.Called, "authorizer expected")
				assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "expect unauthorized")
				if routeAssertions.AssertObject.Type != "" {
					assert.Equal(t, routeAssertions.AssertObject.Type, authorizer.Called.Object.Type, "resource type")
				}
				if routeAssertions.AssertObject.Owner != "" {
					assert.Equal(t, routeAssertions.AssertObject.Owner, authorizer.Called.Object.Owner, "resource owner")
				}
				if routeAssertions.AssertObject.OrgID != "" {
					assert.Equal(t, routeAssertions.AssertObject.OrgID, authorizer.Called.Object.OrgID, "resource org")
				}
				if routeAssertions.AssertObject.ResourceID != "" {
					assert.Equal(t, routeAssertions.AssertObject.ResourceID, authorizer.Called.Object.ResourceID, "resource ID")
				}
			} else {
				assert.Nil(t, authorizer.Called, "authorize not expected")
			}
		})
		return nil
	})
	require.NoError(t, err)
}

type authCall struct {
	SubjectID string
	Roles     []string
	Action    rbac.Action
	Object    rbac.Object
}

type fakeAuthorizer struct {
	Called *authCall
}

func (f *fakeAuthorizer) AuthorizeByRoleName(ctx context.Context, subjectID string, roleNames []string, action rbac.Action, object rbac.Object) error {
	f.Called = &authCall{
		SubjectID: subjectID,
		Roles:     roleNames,
		Action:    action,
		Object:    object,
	}
	return rbac.ForbiddenWithInternal(fmt.Errorf("fake implementation"), nil, nil)
}

func (f *fakeAuthorizer) reset() {
	f.Called = nil
}
