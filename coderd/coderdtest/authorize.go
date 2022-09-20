package coderdtest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func AGPLRoutes(a *AuthTester) (map[string]string, map[string]RouteCheck) {
	// Some quick reused objects
	workspaceRBACObj := rbac.ResourceWorkspace.InOrg(a.Organization.ID).WithOwner(a.Workspace.OwnerID.String())
	workspaceExecObj := rbac.ResourceWorkspaceExecution.InOrg(a.Organization.ID).WithOwner(a.Workspace.OwnerID.String())
	applicationConnectObj := rbac.ResourceWorkspaceApplicationConnect.InOrg(a.Organization.ID).WithOwner(a.Workspace.OwnerID.String())

	// skipRoutes allows skipping routes from being checked.
	skipRoutes := map[string]string{
		"POST:/api/v2/users/logout": "Logging out deletes the API Key for other routes",
		"GET:/derp":                 "This requires a WebSocket upgrade!",
		"GET:/derp/latency-check":   "This always returns a 200!",
	}

	assertRoute := map[string]RouteCheck{
		// These endpoints do not require auth
		"GET:/api/v2":                   {NoAuthorize: true},
		"GET:/api/v2/buildinfo":         {NoAuthorize: true},
		"GET:/api/v2/users/first":       {NoAuthorize: true},
		"POST:/api/v2/users/first":      {NoAuthorize: true},
		"POST:/api/v2/users/login":      {NoAuthorize: true},
		"GET:/api/v2/users/authmethods": {NoAuthorize: true},
		"POST:/api/v2/csp/reports":      {NoAuthorize: true},
		// This is a dummy endpoint for compatibility.
		"GET:/api/v2/workspaceagents/{workspaceagent}/dial": {NoAuthorize: true},

		// Has it's own auth
		"GET:/api/v2/users/oauth2/github/callback": {NoAuthorize: true},
		"GET:/api/v2/users/oidc/callback":          {NoAuthorize: true},

		// All workspaceagents endpoints do not use rbac
		"POST:/api/v2/workspaceagents/aws-instance-identity":    {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/azure-instance-identity":  {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/google-instance-identity": {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/gitsshkey":              {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/metadata":               {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/coordinate":             {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/me/version":               {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/me/app-health":            {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/apps":                   {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/report-stats":           {NoAuthorize: true},

		// These endpoints have more assertions. This is good, add more endpoints to assert if you can!
		"GET:/api/v2/organizations/{organization}": {AssertObject: rbac.ResourceOrganization.InOrg(a.Admin.OrganizationID)},
		"GET:/api/v2/users/{user}/organizations":   {StatusCode: http.StatusOK, AssertObject: rbac.ResourceOrganization},
		"GET:/api/v2/users/{user}/workspace/{workspacename}": {
			AssertObject: rbac.ResourceWorkspace,
			AssertAction: rbac.ActionRead,
		},
		"GET:/api/v2/users/{user}/workspace/{workspacename}/builds/{buildnumber}": {
			AssertObject: rbac.ResourceWorkspace,
			AssertAction: rbac.ActionRead,
		},
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
		"PUT:/api/v2/workspaces/{workspace}/ttl": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspaceresources/{workspaceresource}": {
			AssertAction: rbac.ActionRead,
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
		"GET:/api/v2/workspaceagents/{workspaceagent}": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspaceagents/{workspaceagent}/pty": {
			AssertAction: rbac.ActionCreate,
			AssertObject: workspaceExecObj,
		},
		"GET:/api/v2/workspaceagents/{workspaceagent}/coordinate": {
			AssertAction: rbac.ActionCreate,
			AssertObject: workspaceExecObj,
		},
		"GET:/api/v2/workspaces/": {
			StatusCode:   http.StatusOK,
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/organizations/{organization}/templates": {
			StatusCode:   http.StatusOK,
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"POST:/api/v2/organizations/{organization}/templates": {
			AssertAction: rbac.ActionCreate,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Organization.ID),
		},
		"DELETE:/api/v2/templates/{template}": {
			AssertAction: rbac.ActionDelete,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"GET:/api/v2/templates/{template}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"POST:/api/v2/files": {AssertAction: rbac.ActionCreate, AssertObject: rbac.ResourceFile},
		"GET:/api/v2/files/{hash}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceFile.WithOwner(a.Admin.UserID.String()),
		},
		"GET:/api/v2/templates/{template}/versions": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"PATCH:/api/v2/templates/{template}/versions": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"GET:/api/v2/templates/{template}/versions/{templateversionname}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"GET:/api/v2/templateversions/{templateversion}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"PATCH:/api/v2/templateversions/{templateversion}/cancel": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"GET:/api/v2/templateversions/{templateversion}/logs": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"GET:/api/v2/templateversions/{templateversion}/parameters": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"GET:/api/v2/templateversions/{templateversion}/resources": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"GET:/api/v2/templateversions/{templateversion}/schema": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"POST:/api/v2/templateversions/{templateversion}/dry-run": {
			// The first check is to read the template
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Version.OrganizationID),
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{jobID}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Version.OrganizationID),
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{jobID}/resources": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Version.OrganizationID),
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{jobID}/logs": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Version.OrganizationID),
		},
		"PATCH:/api/v2/templateversions/{templateversion}/dry-run/{jobID}/cancel": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Version.OrganizationID),
		},
		"GET:/api/v2/provisionerdaemons": {
			StatusCode:   http.StatusOK,
			AssertObject: rbac.ResourceProvisionerDaemon,
		},

		"POST:/api/v2/parameters/{scope}/{id}": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: rbac.ResourceTemplate,
		},
		"GET:/api/v2/parameters/{scope}/{id}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate,
		},
		"DELETE:/api/v2/parameters/{scope}/{id}/{name}": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: rbac.ResourceTemplate,
		},
		"GET:/api/v2/organizations/{organization}/templates/{templatename}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Template.OrganizationID),
		},
		"POST:/api/v2/organizations/{organization}/workspaces": {
			AssertAction: rbac.ActionCreate,
			// No ID when creating
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspaces/{workspace}/watch": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/users": {StatusCode: http.StatusOK, AssertObject: rbac.ResourceUser},

		// These endpoints need payloads to get to the auth part. Payloads will be required
		"PUT:/api/v2/users/{user}/roles":                                {StatusCode: http.StatusBadRequest, NoAuthorize: true},
		"PUT:/api/v2/organizations/{organization}/members/{user}/roles": {NoAuthorize: true},
		"POST:/api/v2/workspaces/{workspace}/builds":                    {StatusCode: http.StatusBadRequest, NoAuthorize: true},
		"POST:/api/v2/organizations/{organization}/templateversions":    {StatusCode: http.StatusBadRequest, NoAuthorize: true},
	}

	// Routes like proxy routes support all HTTP methods. A helper func to expand
	// 1 url to all http methods.
	assertAllHTTPMethods := func(url string, check RouteCheck) {
		methods := []string{http.MethodGet, http.MethodHead, http.MethodPost,
			http.MethodPut, http.MethodPatch, http.MethodDelete,
			http.MethodConnect, http.MethodOptions, http.MethodTrace}

		for _, method := range methods {
			route := method + ":" + url
			assertRoute[route] = check
		}
	}

	assertAllHTTPMethods("/%40{user}/{workspace_and_agent}/apps/{workspaceapp}/*", RouteCheck{
		AssertAction: rbac.ActionCreate,
		AssertObject: applicationConnectObj,
	})
	assertAllHTTPMethods("/@{user}/{workspace_and_agent}/apps/{workspaceapp}/*", RouteCheck{
		AssertAction: rbac.ActionCreate,
		AssertObject: applicationConnectObj,
	})

	return skipRoutes, assertRoute
}

type RouteCheck struct {
	NoAuthorize  bool
	AssertAction rbac.Action
	AssertObject rbac.Object
	StatusCode   int
}

type AuthTester struct {
	t          *testing.T
	api        *coderd.API
	authorizer *RecordingAuthorizer

	Client                *codersdk.Client
	Workspace             codersdk.Workspace
	Organization          codersdk.Organization
	Admin                 codersdk.CreateFirstUserResponse
	Template              codersdk.Template
	Version               codersdk.TemplateVersion
	WorkspaceResource     codersdk.WorkspaceResource
	File                  codersdk.UploadResponse
	TemplateVersionDryRun codersdk.ProvisionerJob
	TemplateParam         codersdk.Parameter
	URLParams             map[string]string
}

func NewAuthTester(ctx context.Context, t *testing.T, client *codersdk.Client, api *coderd.API, admin codersdk.CreateFirstUserResponse) *AuthTester {
	authorizer, ok := api.Authorizer.(*RecordingAuthorizer)
	if !ok {
		t.Fail()
	}
	// The provisioner will call to coderd and register itself. This is async,
	// so we wait for it to occur.
	require.Eventually(t, func() bool {
		provisionerds, err := client.ProvisionerDaemons(ctx)
		return assert.NoError(t, err) && len(provisionerds) > 0
	}, testutil.WaitLong, testutil.IntervalSlow)

	provisionerds, err := client.ProvisionerDaemons(ctx)
	require.NoError(t, err, "fetch provisioners")
	require.Len(t, provisionerds, 1)

	organization, err := client.Organization(ctx, admin.OrganizationID)
	require.NoError(t, err, "fetch org")

	// Setup some data in the database.
	version := CreateTemplateVersion(t, client, admin.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					// Return a workspace resource
					Resources: []*proto.Resource{{
						Name: "some",
						Type: "example",
						Agents: []*proto.Agent{{
							Name: "agent",
							Id:   "something",
							Auth: &proto.Agent_Token{},
							Apps: []*proto.App{{
								Name: "testapp",
								Url:  "http://localhost:3000",
							}},
						}},
					}},
				},
			},
		}},
	})
	AwaitTemplateVersionJob(t, client, version.ID)
	template := CreateTemplate(t, client, admin.OrganizationID, version.ID)
	workspace := CreateWorkspace(t, client, admin.OrganizationID, template.ID)
	AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
	file, err := client.Upload(ctx, codersdk.ContentTypeTar, make([]byte, 1024))
	require.NoError(t, err, "upload file")
	workspaceResources, err := client.WorkspaceResourcesByBuild(ctx, workspace.LatestBuild.ID)
	require.NoError(t, err, "workspace resources")
	templateVersionDryRun, err := client.CreateTemplateVersionDryRun(ctx, version.ID, codersdk.CreateTemplateVersionDryRunRequest{
		ParameterValues: []codersdk.CreateParameterRequest{},
	})
	require.NoError(t, err, "template version dry-run")

	templateParam, err := client.CreateParameter(ctx, codersdk.ParameterTemplate, template.ID, codersdk.CreateParameterRequest{
		Name:              "test-param",
		SourceValue:       "hello world",
		SourceScheme:      codersdk.ParameterSourceSchemeData,
		DestinationScheme: codersdk.ParameterDestinationSchemeProvisionerVariable,
	})
	require.NoError(t, err, "create template param")
	urlParameters := map[string]string{
		"{organization}":        admin.OrganizationID.String(),
		"{user}":                admin.UserID.String(),
		"{organizationname}":    organization.Name,
		"{workspace}":           workspace.ID.String(),
		"{workspacebuild}":      workspace.LatestBuild.ID.String(),
		"{workspacename}":       workspace.Name,
		"{workspaceagent}":      workspaceResources[0].Agents[0].ID.String(),
		"{buildnumber}":         strconv.FormatInt(int64(workspace.LatestBuild.BuildNumber), 10),
		"{template}":            template.ID.String(),
		"{hash}":                file.Hash,
		"{workspaceresource}":   workspaceResources[0].ID.String(),
		"{workspaceapp}":        workspaceResources[0].Agents[0].Apps[0].Name,
		"{templateversion}":     version.ID.String(),
		"{jobID}":               templateVersionDryRun.ID.String(),
		"{templatename}":        template.Name,
		"{workspace_and_agent}": workspace.Name + "." + workspaceResources[0].Agents[0].Name,
		// Only checking template scoped params here
		"parameters/{scope}/{id}": fmt.Sprintf("parameters/%s/%s",
			string(templateParam.Scope), templateParam.ScopeID.String()),
	}

	return &AuthTester{
		t:                     t,
		api:                   api,
		authorizer:            authorizer,
		Client:                client,
		Workspace:             workspace,
		Organization:          organization,
		Admin:                 admin,
		Template:              template,
		Version:               version,
		WorkspaceResource:     workspaceResources[0],
		File:                  file,
		TemplateVersionDryRun: templateVersionDryRun,
		TemplateParam:         templateParam,
		URLParams:             urlParameters,
	}
}

func (a *AuthTester) Test(ctx context.Context, assertRoute map[string]RouteCheck, skipRoutes map[string]string) {
	// Always fail auth from this point forward
	a.authorizer.AlwaysReturn = rbac.ForbiddenWithInternal(xerrors.New("fake implementation"), nil, nil)

	routeMissing := make(map[string]bool)
	for k, v := range assertRoute {
		noTrailSlash := strings.TrimRight(k, "/")
		if _, ok := assertRoute[noTrailSlash]; ok && noTrailSlash != k {
			a.t.Errorf("route %q & %q is declared twice", noTrailSlash, k)
			a.t.FailNow()
		}
		assertRoute[noTrailSlash] = v
		routeMissing[noTrailSlash] = true
	}

	for k, v := range skipRoutes {
		noTrailSlash := strings.TrimRight(k, "/")
		if _, ok := skipRoutes[noTrailSlash]; ok && noTrailSlash != k {
			a.t.Errorf("route %q & %q is declared twice", noTrailSlash, k)
			a.t.FailNow()
		}
		skipRoutes[noTrailSlash] = v
	}

	err := chi.Walk(
		a.api.RootHandler,
		func(
			method string,
			route string,
			handler http.Handler,
			middlewares ...func(http.Handler) http.Handler,
		) error {
			// work around chi's bugged handling of /*/*/ which can occur if we
			// r.Mount("/", someHandler()) in our tree
			for strings.Contains(route, "/*/") {
				route = strings.Replace(route, "/*/", "/", -1)
			}
			name := method + ":" + route
			if _, ok := skipRoutes[strings.TrimRight(name, "/")]; ok {
				return nil
			}
			a.t.Run(name, func(t *testing.T) {
				a.authorizer.reset()
				routeKey := strings.TrimRight(name, "/")

				routeAssertions, ok := assertRoute[routeKey]
				if !ok {
					// By default, all omitted routes check for just "authorize" called
					routeAssertions = RouteCheck{}
				}
				delete(routeMissing, routeKey)

				// Replace all url params with known values
				for k, v := range a.URLParams {
					route = strings.ReplaceAll(route, k, v)
				}

				resp, err := a.Client.Request(ctx, method, route, nil)
				require.NoError(t, err, "do req")
				body, _ := io.ReadAll(resp.Body)
				t.Logf("Response Body: %q", string(body))
				_ = resp.Body.Close()

				if !routeAssertions.NoAuthorize {
					assert.NotNil(t, a.authorizer.Called, "authorizer expected")
					if routeAssertions.StatusCode != 0 {
						assert.Equal(t, routeAssertions.StatusCode, resp.StatusCode, "expect unauthorized")
					} else {
						// It's either a 404 or 403.
						if resp.StatusCode != http.StatusNotFound {
							assert.Equal(t, http.StatusForbidden, resp.StatusCode, "expect unauthorized")
						}
					}
					if a.authorizer.Called != nil {
						if routeAssertions.AssertAction != "" {
							assert.Equal(t, routeAssertions.AssertAction, a.authorizer.Called.Action, "resource action")
						}
						if routeAssertions.AssertObject.Type != "" {
							assert.Equal(t, routeAssertions.AssertObject.Type, a.authorizer.Called.Object.Type, "resource type")
						}
						if routeAssertions.AssertObject.Owner != "" {
							assert.Equal(t, routeAssertions.AssertObject.Owner, a.authorizer.Called.Object.Owner, "resource owner")
						}
						if routeAssertions.AssertObject.OrgID != "" {
							assert.Equal(t, routeAssertions.AssertObject.OrgID, a.authorizer.Called.Object.OrgID, "resource org")
						}
					}
				} else {
					assert.Nil(t, a.authorizer.Called, "authorize not expected")
				}
			})
			return nil
		})
	require.NoError(a.t, err)
	require.Len(a.t, routeMissing, 0, "didn't walk some asserted routes: %v", routeMissing)
}

type authCall struct {
	SubjectID string
	Roles     []string
	Scope     rbac.Scope
	Action    rbac.Action
	Object    rbac.Object
}

type RecordingAuthorizer struct {
	Called       *authCall
	AlwaysReturn error
}

var _ rbac.Authorizer = (*RecordingAuthorizer)(nil)

func (r *RecordingAuthorizer) ByRoleName(_ context.Context, subjectID string, roleNames []string, scope rbac.Scope, action rbac.Action, object rbac.Object) error {
	r.Called = &authCall{
		SubjectID: subjectID,
		Roles:     roleNames,
		Scope:     scope,
		Action:    action,
		Object:    object,
	}
	return r.AlwaysReturn
}

func (r *RecordingAuthorizer) PrepareByRoleName(_ context.Context, subjectID string, roles []string, scope rbac.Scope, action rbac.Action, _ string) (rbac.PreparedAuthorized, error) {
	return &fakePreparedAuthorizer{
		Original:  r,
		SubjectID: subjectID,
		Roles:     roles,
		Scope:     scope,
		Action:    action,
	}, nil
}

func (r *RecordingAuthorizer) reset() {
	r.Called = nil
}

type fakePreparedAuthorizer struct {
	Original  *RecordingAuthorizer
	SubjectID string
	Roles     []string
	Scope     rbac.Scope
	Action    rbac.Action
}

func (f *fakePreparedAuthorizer) Authorize(ctx context.Context, object rbac.Object) error {
	return f.Original.ByRoleName(ctx, f.SubjectID, f.Roles, f.Scope, f.Action, object)
}
