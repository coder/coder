package coderdtest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/coder/coderd/database/databasefake"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/rbac/regosql"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func AGPLRoutes(a *AuthTester) (map[string]string, map[string]RouteCheck) {
	// For any route using SQL filters, we need to know if the database is an
	// in memory fake. This is because the in memory fake does not use SQL, and
	// still uses rego. So this boolean indicates how to assert the expected
	// behavior.
	_, isMemoryDB := a.api.Database.(databasefake.FakeDatabase)

	// Some quick reused objects
	workspaceRBACObj := rbac.ResourceWorkspace.WithID(a.Workspace.ID).InOrg(a.Organization.ID).WithOwner(a.Workspace.OwnerID.String())
	workspaceExecObj := rbac.ResourceWorkspaceExecution.WithID(a.Workspace.ID).InOrg(a.Organization.ID).WithOwner(a.Workspace.OwnerID.String())
	applicationConnectObj := rbac.ResourceWorkspaceApplicationConnect.WithID(a.Workspace.ID).InOrg(a.Organization.ID).WithOwner(a.Workspace.OwnerID.String())
	templateObj := rbac.ResourceTemplate.WithID(a.Template.ID).InOrg(a.Template.OrganizationID)

	// skipRoutes allows skipping routes from being checked.
	skipRoutes := map[string]string{
		"POST:/api/v2/users/logout": "Logging out deletes the API Key for other routes",
		"GET:/derp":                 "This requires a WebSocket upgrade!",
		"GET:/derp/latency-check":   "This always returns a 200!",
	}

	assertRoute := map[string]RouteCheck{
		// These endpoints do not require auth
		"GET:/healthz":                  {NoAuthorize: true},
		"GET:/api/v2":                   {NoAuthorize: true},
		"GET:/api/v2/buildinfo":         {NoAuthorize: true},
		"GET:/api/v2/experiments":       {NoAuthorize: true}, // This route requires AuthN, but not AuthZ.
		"GET:/api/v2/updatecheck":       {NoAuthorize: true},
		"GET:/api/v2/users/first":       {NoAuthorize: true},
		"POST:/api/v2/users/first":      {NoAuthorize: true},
		"POST:/api/v2/users/login":      {NoAuthorize: true},
		"GET:/api/v2/users/authmethods": {NoAuthorize: true},
		"POST:/api/v2/csp/reports":      {NoAuthorize: true},
		"POST:/api/v2/authcheck":        {NoAuthorize: true},
		"GET:/api/v2/applications/host": {NoAuthorize: true},

		// Has it's own auth
		"GET:/api/v2/users/oauth2/github/callback": {NoAuthorize: true},
		"GET:/api/v2/users/oidc/callback":          {NoAuthorize: true},

		// All workspaceagents endpoints do not use rbac
		"POST:/api/v2/workspaceagents/aws-instance-identity":    {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/azure-instance-identity":  {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/google-instance-identity": {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/gitauth":                {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/gitsshkey":              {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/metadata":               {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/coordinate":             {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/me/version":               {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/me/app-health":            {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/me/report-stats":          {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/me/report-lifecycle":      {NoAuthorize: true},

		// These endpoints have more assertions. This is good, add more endpoints to assert if you can!
		"GET:/api/v2/organizations/{organization}": {AssertObject: rbac.ResourceOrganization.WithID(a.Admin.OrganizationID).InOrg(a.Admin.OrganizationID)},
		"GET:/api/v2/users/{user}/organizations":   {StatusCode: http.StatusOK, AssertObject: rbac.ResourceOrganization},
		"GET:/api/v2/users/{user}/workspace/{workspacename}": {
			AssertObject: rbac.ResourceWorkspace,
			AssertAction: rbac.ActionRead,
		},
		"GET:/api/v2/users/{user}/workspace/{workspacename}/builds/{buildnumber}": {
			AssertObject: rbac.ResourceWorkspace,
			AssertAction: rbac.ActionRead,
		},
		"GET:/api/v2/users/{user}/keys/tokens": {
			AssertObject: rbac.ResourceAPIKey,
			AssertAction: rbac.ActionRead,
			StatusCode:   http.StatusOK,
		},
		"GET:/api/v2/users/{user}/keys/{keyid}": {
			AssertObject: rbac.ResourceAPIKey,
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
		"POST:/api/v2/organizations/{organization}/templates": {
			AssertAction: rbac.ActionCreate,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Organization.ID),
		},
		"DELETE:/api/v2/templates/{template}": {
			AssertAction: rbac.ActionDelete,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templates/{template}": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"POST:/api/v2/files": {AssertAction: rbac.ActionCreate, AssertObject: rbac.ResourceFile},
		"GET:/api/v2/files/{fileID}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceFile.WithOwner(a.Admin.UserID.String()),
		},
		"GET:/api/v2/templates/{template}/versions": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"PATCH:/api/v2/templates/{template}/versions": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templates/{template}/versions/{templateversionname}": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templateversions/{templateversion}": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"PATCH:/api/v2/templateversions/{templateversion}/cancel": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templateversions/{templateversion}/logs": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templateversions/{templateversion}/parameters": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templateversions/{templateversion}/rich-parameters": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templateversions/{templateversion}/resources": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templateversions/{templateversion}/schema": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"POST:/api/v2/templateversions/{templateversion}/dry-run": {
			// The first check is to read the template
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{jobID}": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{jobID}/resources": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{jobID}/logs": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
		},
		"PATCH:/api/v2/templateversions/{templateversion}/dry-run/{jobID}/cancel": {
			AssertAction: rbac.ActionRead,
			AssertObject: templateObj,
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
			AssertObject: templateObj,
		},
		"POST:/api/v2/organizations/{organization}/members/{user}/workspaces": {
			AssertAction: rbac.ActionCreate,
			// No ID when creating
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspaces/{workspace}/watch": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/users":                      {StatusCode: http.StatusOK, AssertObject: rbac.ResourceUser},
		"GET:/api/v2/applications/auth-redirect": {AssertAction: rbac.ActionCreate, AssertObject: rbac.ResourceAPIKey},

		// These endpoints need payloads to get to the auth part. Payloads will be required
		"PUT:/api/v2/users/{user}/roles":                                                           {StatusCode: http.StatusBadRequest, NoAuthorize: true},
		"PUT:/api/v2/organizations/{organization}/members/{user}/roles":                            {NoAuthorize: true},
		"POST:/api/v2/workspaces/{workspace}/builds":                                               {StatusCode: http.StatusBadRequest, NoAuthorize: true},
		"POST:/api/v2/organizations/{organization}/templateversions":                               {StatusCode: http.StatusBadRequest, NoAuthorize: true},
		"GET:/api/v2/organizations/{organization}/templateversions/{templateversionname}":          {StatusCode: http.StatusBadRequest, NoAuthorize: true},
		"GET:/api/v2/organizations/{organization}/templateversions/{templateversionname}/previous": {StatusCode: http.StatusBadRequest, NoAuthorize: true},

		// Endpoints that use the SQLQuery filter.
		"GET:/api/v2/workspaces/": {
			StatusCode:   http.StatusOK,
			NoAuthorize:  !isMemoryDB,
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceWorkspace,
		},
		"GET:/api/v2/organizations/{organization}/templates": {
			StatusCode:   http.StatusOK,
			NoAuthorize:  !isMemoryDB,
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate,
		},

		"GET:/api/v2/debug/coordinator": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceDebugInfo,
		},
	}

	// Routes like proxy routes support all HTTP methods. A helper func to expand
	// 1 url to all http methods.
	assertAllHTTPMethods := func(url string, check RouteCheck) {
		methods := []string{
			http.MethodGet, http.MethodHead, http.MethodPost,
			http.MethodPut, http.MethodPatch, http.MethodDelete,
			http.MethodConnect, http.MethodOptions, http.MethodTrace,
		}

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
	_, err := client.CreateToken(ctx, admin.UserID.String(), codersdk.CreateTokenRequest{
		Lifetime: time.Hour,
		Scope:    codersdk.APIKeyScopeAll,
	})
	require.NoError(t, err, "create token")

	apiKeys, err := client.GetTokens(ctx, admin.UserID.String())
	require.NoError(t, err, "get tokens")
	apiKey := apiKeys[0]

	organization, err := client.Organization(ctx, admin.OrganizationID)
	require.NoError(t, err, "fetch org")

	// Setup some data in the database.
	version := CreateTemplateVersion(t, client, admin.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionApply: []*proto.Provision_Response{{
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
								Slug:        "testapp",
								DisplayName: "testapp",
								Url:         "http://localhost:3000",
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
	workspace, err = client.Workspace(ctx, workspace.ID)
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
		"{workspaceagent}":      workspace.LatestBuild.Resources[0].Agents[0].ID.String(),
		"{buildnumber}":         strconv.FormatInt(int64(workspace.LatestBuild.BuildNumber), 10),
		"{template}":            template.ID.String(),
		"{fileID}":              file.ID.String(),
		"{workspaceresource}":   workspace.LatestBuild.Resources[0].ID.String(),
		"{workspaceapp}":        workspace.LatestBuild.Resources[0].Agents[0].Apps[0].Slug,
		"{templateversion}":     version.ID.String(),
		"{jobID}":               templateVersionDryRun.ID.String(),
		"{templatename}":        template.Name,
		"{workspace_and_agent}": workspace.Name + "." + workspace.LatestBuild.Resources[0].Agents[0].Name,
		"{keyid}":               apiKey.ID,
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
		WorkspaceResource:     workspace.LatestBuild.Resources[0],
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
	Roles     rbac.ExpandableRoles
	Groups    []string
	Scope     rbac.ScopeName
	Action    rbac.Action
	Object    rbac.Object
}

type RecordingAuthorizer struct {
	Called       *authCall
	AlwaysReturn error
}

var _ rbac.Authorizer = (*RecordingAuthorizer)(nil)

// ByRoleNameSQL does not record the call. This matches the postgres behavior
// of not calling Authorize()
func (r *RecordingAuthorizer) ByRoleNameSQL(_ context.Context, _ string, _ rbac.ExpandableRoles, _ rbac.ScopeName, _ []string, _ rbac.Action, _ rbac.Object) error {
	return r.AlwaysReturn
}

func (r *RecordingAuthorizer) ByRoleName(_ context.Context, subjectID string, roleNames rbac.ExpandableRoles, scope rbac.ScopeName, groups []string, action rbac.Action, object rbac.Object) error {
	r.Called = &authCall{
		SubjectID: subjectID,
		Roles:     roleNames,
		Groups:    groups,
		Scope:     scope,
		Action:    action,
		Object:    object,
	}
	return r.AlwaysReturn
}

func (r *RecordingAuthorizer) PrepareByRoleName(_ context.Context, subjectID string, roles rbac.ExpandableRoles, scope rbac.ScopeName, groups []string, action rbac.Action, _ string) (rbac.PreparedAuthorized, error) {
	return &fakePreparedAuthorizer{
		Original:           r,
		SubjectID:          subjectID,
		Roles:              roles,
		Scope:              scope,
		Action:             action,
		HardCodedSQLString: "true",
		Groups:             groups,
	}, nil
}

func (r *RecordingAuthorizer) reset() {
	r.Called = nil
}

type fakePreparedAuthorizer struct {
	Original            *RecordingAuthorizer
	SubjectID           string
	Roles               rbac.ExpandableRoles
	Scope               rbac.ScopeName
	Action              rbac.Action
	Groups              []string
	HardCodedSQLString  string
	HardCodedRegoString string
}

func (f *fakePreparedAuthorizer) Authorize(ctx context.Context, object rbac.Object) error {
	return f.Original.ByRoleName(ctx, f.SubjectID, f.Roles, f.Scope, f.Groups, f.Action, object)
}

// CompileToSQL returns a compiled version of the authorizer that will work for
// in memory databases. This fake version will not work against a SQL database.
func (fakePreparedAuthorizer) CompileToSQL(_ context.Context, _ regosql.ConvertConfig) (string, error) {
	return "", xerrors.New("not implemented")
}

func (f *fakePreparedAuthorizer) Eval(object rbac.Object) bool {
	return f.Original.ByRoleNameSQL(context.Background(), f.SubjectID, f.Roles, f.Scope, f.Groups, f.Action, object) == nil
}

func (f fakePreparedAuthorizer) RegoString() string {
	if f.HardCodedRegoString != "" {
		return f.HardCodedRegoString
	}
	panic("not implemented")
}
