package coderd_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

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
	srv, client := coderdtest.NewWithServer(t, &coderdtest.Options{
		Authorizer: authorizer,
	})
	admin := coderdtest.CreateFirstUser(t, client)
	organization, err := client.Organization(context.Background(), admin.OrganizationID)
	require.NoError(t, err, "fetch org")

	// Setup some data in the database.
	coderdtest.NewProvisionerDaemon(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, admin.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	// Always fail auth from this point forward
	authorizer.AlwaysReturn = rbac.ForbiddenWithInternal(xerrors.New("fake implementation"), nil, nil)

	// Some quick reused objects
	workspaceRBACObj := rbac.ResourceWorkspace.InOrg(organization.ID).WithID(workspace.ID.String()).WithOwner(workspace.OwnerID.String())

	// skipRoutes allows skipping routes from being checked.
	type routeCheck struct {
		NoAuthorize  bool
		AssertAction rbac.Action
		AssertObject rbac.Object
		StatusCode   int
	}
	assertRoute := map[string]routeCheck{
		// These endpoints do not require auth
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
		"GET:/api/v2/workspaceagents/{workspaceagent}/dial":       {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/iceservers": {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/pty":        {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/turn":       {NoAuthorize: true},

		// TODO: @emyrk these need to be fixed by adding authorize calls
		"GET:/api/v2/workspaceresources/{workspaceresource}": {NoAuthorize: true},

		"GET:/api/v2/users/oauth2/github/callback": {NoAuthorize: true},

		"PUT:/api/v2/organizations/{organization}/members/{user}/roles":     {NoAuthorize: true},
		"GET:/api/v2/organizations/{organization}/provisionerdaemons":       {NoAuthorize: true},
		"POST:/api/v2/organizations/{organization}/templates":               {NoAuthorize: true},
		"GET:/api/v2/organizations/{organization}/templates":                {NoAuthorize: true},
		"GET:/api/v2/organizations/{organization}/templates/{templatename}": {NoAuthorize: true},
		"POST:/api/v2/organizations/{organization}/templateversions":        {NoAuthorize: true},
		"POST:/api/v2/organizations/{organization}/workspaces":              {NoAuthorize: true},

		"POST:/api/v2/parameters/{scope}/{id}":          {NoAuthorize: true},
		"GET:/api/v2/parameters/{scope}/{id}":           {NoAuthorize: true},
		"DELETE:/api/v2/parameters/{scope}/{id}/{name}": {NoAuthorize: true},

		"GET:/api/v2/provisionerdaemons/me/listen": {NoAuthorize: true},

		"DELETE:/api/v2/templates/{template}":                             {NoAuthorize: true},
		"GET:/api/v2/templates/{template}":                                {NoAuthorize: true},
		"GET:/api/v2/templates/{template}/versions":                       {NoAuthorize: true},
		"PATCH:/api/v2/templates/{template}/versions":                     {NoAuthorize: true},
		"GET:/api/v2/templates/{template}/versions/{templateversionname}": {NoAuthorize: true},

		"GET:/api/v2/templateversions/{templateversion}":            {NoAuthorize: true},
		"PATCH:/api/v2/templateversions/{templateversion}/cancel":   {NoAuthorize: true},
		"GET:/api/v2/templateversions/{templateversion}/logs":       {NoAuthorize: true},
		"GET:/api/v2/templateversions/{templateversion}/parameters": {NoAuthorize: true},
		"GET:/api/v2/templateversions/{templateversion}/resources":  {NoAuthorize: true},
		"GET:/api/v2/templateversions/{templateversion}/schema":     {NoAuthorize: true},

		"POST:/api/v2/users/{user}/organizations": {NoAuthorize: true},

		"POST:/api/v2/files":                       {NoAuthorize: true},
		"GET:/api/v2/files/{hash}":                 {NoAuthorize: true},
		"GET:/api/v2/workspaces/{workspace}/watch": {NoAuthorize: true},

		// These endpoints have more assertions. This is good, add more endpoints to assert if you can!
		"GET:/api/v2/organizations/{organization}":                   {AssertObject: rbac.ResourceOrganization.InOrg(admin.OrganizationID)},
		"GET:/api/v2/users/{user}/organizations":                     {StatusCode: http.StatusOK, AssertObject: rbac.ResourceOrganization},
		"GET:/api/v2/users/{user}/workspaces":                        {StatusCode: http.StatusOK, AssertObject: rbac.ResourceWorkspace},
		"GET:/api/v2/organizations/{organization}/workspaces/{user}": {StatusCode: http.StatusOK, AssertObject: rbac.ResourceWorkspace},
		"GET:/api/v2/organizations/{organization}/workspaces/{user}/{workspace}": {
			AssertObject: rbac.ResourceWorkspace.InOrg(organization.ID).WithID(workspace.ID.String()).WithOwner(workspace.OwnerID.String()),
		},
		"GET:/api/v2/workspaces/{workspace}/builds/{workspacebuildname}": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/organizations/{organization}/workspaces/{user}/{workspacename}": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/organizations/{organization}/workspaces": {StatusCode: http.StatusOK, AssertObject: rbac.ResourceWorkspace},
		"GET:/api/v2/workspacebuilds/{workspacebuild}": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspacebuilds/{workspacebuild}/logs": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspaces/{workspace}/builds": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspaces/{workspace}": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"PUT:/api/v2/workspaces/{workspace}/autostart": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: workspaceRBACObj,
		},
		"PUT:/api/v2/workspaces/{workspace}/autostop": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: workspaceRBACObj,
		},
		"PATCH:/api/v2/workspacebuilds/{workspacebuild}/cancel": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspacebuilds/{workspacebuild}/resources": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspacebuilds/{workspacebuild}/state": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspaces/": {
			StatusCode:   http.StatusOK,
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},

		// These endpoints need payloads to get to the auth part. Payloads will be required
		"PUT:/api/v2/users/{user}/roles":             {StatusCode: http.StatusBadRequest, NoAuthorize: true},
		"POST:/api/v2/workspaces/{workspace}/builds": {StatusCode: http.StatusBadRequest, NoAuthorize: true},
	}

	for k, v := range assertRoute {
		noTrailSlash := strings.TrimRight(k, "/")
		if _, ok := assertRoute[noTrailSlash]; ok && noTrailSlash != k {
			t.Errorf("route %q & %q is declared twice", noTrailSlash, k)
			t.FailNow()
		}
		assertRoute[noTrailSlash] = v
	}

	c, _ := srv.Config.Handler.(*chi.Mux)
	err = chi.Walk(c, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		name := method + ":" + route
		t.Run(name, func(t *testing.T) {
			authorizer.reset()
			routeAssertions, ok := assertRoute[strings.TrimRight(name, "/")]
			if !ok {
				// By default, all omitted routes check for just "authorize" called
				routeAssertions = routeCheck{}
			}
			if routeAssertions.StatusCode == 0 {
				routeAssertions.StatusCode = http.StatusForbidden
			}

			// Replace all url params with known values
			route = strings.ReplaceAll(route, "{organization}", admin.OrganizationID.String())
			route = strings.ReplaceAll(route, "{user}", admin.UserID.String())
			route = strings.ReplaceAll(route, "{organizationname}", organization.Name)
			route = strings.ReplaceAll(route, "{workspace}", workspace.ID.String())
			route = strings.ReplaceAll(route, "{workspacebuild}", workspace.LatestBuild.ID.String())
			route = strings.ReplaceAll(route, "{workspacename}", workspace.Name)
			route = strings.ReplaceAll(route, "{workspacebuildname}", workspace.LatestBuild.Name)

			resp, err := client.Request(context.Background(), method, route, nil)
			require.NoError(t, err, "do req")
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Response Body: %q", string(body))
			_ = resp.Body.Close()

			if !routeAssertions.NoAuthorize {
				assert.NotNil(t, authorizer.Called, "authorizer expected")
				assert.Equal(t, routeAssertions.StatusCode, resp.StatusCode, "expect unauthorized")
				if authorizer.Called != nil {
					if routeAssertions.AssertAction != "" {
						assert.Equal(t, routeAssertions.AssertAction, authorizer.Called.Action, "resource action")
					}
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
	Called       *authCall
	AlwaysReturn error
}

func (f *fakeAuthorizer) ByRoleName(_ context.Context, subjectID string, roleNames []string, action rbac.Action, object rbac.Object) error {
	f.Called = &authCall{
		SubjectID: subjectID,
		Roles:     roleNames,
		Action:    action,
		Object:    object,
	}
	return f.AlwaysReturn
}

func (f *fakeAuthorizer) reset() {
	f.Called = nil
}
