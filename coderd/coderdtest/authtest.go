package coderdtest

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/coder/coder/coderd"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

type RouteCheck struct {
	NoAuthorize  bool
	AssertAction rbac.Action
	AssertObject rbac.Object
	StatusCode   int
}

type AuthTester struct {
	t          *testing.T
	api        *coderd.API
	authorizer *recordingAuthorizer
	client     *codersdk.Client

	Workspace             codersdk.Workspace
	Organization          codersdk.Organization
	Admin                 codersdk.CreateFirstUserResponse
	Template              codersdk.Template
	Version               codersdk.TemplateVersion
	WorkspaceResource     codersdk.WorkspaceResource
	File                  codersdk.UploadResponse
	TemplateVersionDryRun codersdk.ProvisionerJob
	TemplateParam         codersdk.Parameter
}

func NewAuthTester(ctx context.Context, t *testing.T, options *Options) *AuthTester {
	authorizer := &recordingAuthorizer{}
	if options == nil {
		options = &Options{}
	}
	if options.Authorizer != nil {
		t.Error("NewAuthTester cannot be called with custom Authorizer")
	}
	options.Authorizer = authorizer
	options.IncludeProvisionerD = true

	client, _, api := newWithAPI(t, options)
	admin := CreateFirstUser(t, client)
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
							Id:   "something",
							Auth: &proto.Agent_Token{},
							Apps: []*proto.App{{
								Name: "app",
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

	// Always fail auth from this point forward
	authorizer.AlwaysReturn = rbac.ForbiddenWithInternal(xerrors.New("fake implementation"), nil, nil)

	return &AuthTester{
		t:                     t,
		api:                   api,
		authorizer:            authorizer,
		client:                client,
		Workspace:             workspace,
		Organization:          organization,
		Admin:                 admin,
		Template:              template,
		Version:               version,
		WorkspaceResource:     workspaceResources[0],
		File:                  file,
		TemplateVersionDryRun: templateVersionDryRun,
		TemplateParam:         templateParam,
	}
}

func AGPLRoutes(a *AuthTester) (map[string]string, map[string]RouteCheck) {
	// Some quick reused objects
	workspaceRBACObj := rbac.ResourceWorkspace.InOrg(a.Organization.ID).WithOwner(a.Workspace.OwnerID.String())
	workspaceExecObj := rbac.ResourceWorkspaceExecution.InOrg(a.Organization.ID).WithOwner(a.Workspace.OwnerID.String())
	// skipRoutes allows skipping routes from being checked.
	skipRoutes := map[string]string{
		"POST:/api/v2/users/logout": "Logging out deletes the API Key for other routes",
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
		"GET:/api/v2/entitlements":      {NoAuthorize: true},

		"GET:/%40{user}/{workspacename}/apps/{application}/*": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/@{user}/{workspacename}/apps/{application}/*": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},

		// Has it's own auth
		"GET:/api/v2/users/oauth2/github/callback": {NoAuthorize: true},
		"GET:/api/v2/users/oidc/callback":          {NoAuthorize: true},

		// All workspaceagents endpoints do not use rbac
		"POST:/api/v2/workspaceagents/aws-instance-identity":      {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/azure-instance-identity":    {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/google-instance-identity":   {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/gitsshkey":                {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/iceservers":               {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/listen":                   {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/metadata":                 {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/turn":                     {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/derp":                     {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/me/wireguardlisten":          {NoAuthorize: true},
		"POST:/api/v2/workspaceagents/me/keys":                    {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/iceservers": {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/derp":       {NoAuthorize: true},

		// These endpoints have more assertions. This is good, add more endpoints to assert if you can!
		"GET:/api/v2/organizations/{organization}": {AssertObject: rbac.ResourceOrganization.InOrg(a.Admin.OrganizationID)},
		"GET:/api/v2/users/{user}/organizations":   {StatusCode: http.StatusOK, AssertObject: rbac.ResourceOrganization},
		"GET:/api/v2/users/{user}/workspace/{workspacename}": {
			AssertObject: rbac.ResourceWorkspace,
			AssertAction: rbac.ActionRead,
		},
		"GET:/api/v2/users/me/workspace/{workspacename}/builds/{buildnumber}": {
			AssertObject: rbac.ResourceWorkspace,
			AssertAction: rbac.ActionRead,
		},
		"GET:/api/v2/workspaces/{workspace}/builds/{workspacebuildname}": {
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
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
		"PUT:/api/v2/workspaces/{workspace}/autostop": {
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
		"GET:/api/v2/workspaceagents/{workspaceagent}/dial": {
			AssertAction: rbac.ActionCreate,
			AssertObject: workspaceExecObj,
		},
		"GET:/api/v2/workspaceagents/{workspaceagent}/turn": {
			AssertAction: rbac.ActionCreate,
			AssertObject: workspaceExecObj,
		},
		"GET:/api/v2/workspaceagents/{workspaceagent}/pty": {
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
		"GET:/api/v2/files/{fileHash}": {
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
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{templateversiondryrun}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Version.OrganizationID),
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{templateversiondryrun}/resources": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Version.OrganizationID),
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{templateversiondryrun}/logs": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(a.Version.OrganizationID),
		},
		"PATCH:/api/v2/templateversions/{templateversion}/dry-run/{templateversiondryrun}/cancel": {
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
		"POST:/api/v2/users/{user}/organizations": {
			AssertAction: rbac.ActionCreate,
			AssertObject: rbac.ResourceOrganization,
		},
		"GET:/api/v2/users": {StatusCode: http.StatusOK, AssertObject: rbac.ResourceUser},

		// These endpoints need payloads to get to the auth part. Payloads will be required
		"PUT:/api/v2/users/{user}/roles":                                {StatusCode: http.StatusBadRequest, NoAuthorize: true},
		"PUT:/api/v2/organizations/{organization}/members/{user}/roles": {NoAuthorize: true},
		"POST:/api/v2/workspaces/{workspace}/builds":                    {StatusCode: http.StatusBadRequest, NoAuthorize: true},
		"POST:/api/v2/organizations/{organization}/templateversions":    {StatusCode: http.StatusBadRequest, NoAuthorize: true},
	}
	return skipRoutes, assertRoute
}

func (a *AuthTester) Test(ctx context.Context, assertRoute map[string]RouteCheck, skipRoutes map[string]string) {
	for k, v := range assertRoute {
		noTrailSlash := strings.TrimRight(k, "/")
		if _, ok := assertRoute[noTrailSlash]; ok && noTrailSlash != k {
			a.t.Errorf("route %q & %q is declared twice", noTrailSlash, k)
			a.t.FailNow()
		}
		assertRoute[noTrailSlash] = v
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
		a.api.Handler,
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
				routeAssertions, ok := assertRoute[strings.TrimRight(name, "/")]
				if !ok {
					// By default, all omitted routes check for just "authorize" called
					routeAssertions = RouteCheck{}
				}

				// Replace all url params with known values
				route = strings.ReplaceAll(route, "{organization}", a.Admin.OrganizationID.String())
				route = strings.ReplaceAll(route, "{user}", a.Admin.UserID.String())
				route = strings.ReplaceAll(route, "{organizationname}", a.Organization.Name)
				route = strings.ReplaceAll(route, "{workspace}", a.Workspace.ID.String())
				route = strings.ReplaceAll(route, "{workspacebuild}", a.Workspace.LatestBuild.ID.String())
				route = strings.ReplaceAll(route, "{workspacename}", a.Workspace.Name)
				route = strings.ReplaceAll(route, "{workspacebuildname}", a.Workspace.LatestBuild.Name)
				route = strings.ReplaceAll(route, "{workspaceagent}", a.WorkspaceResource.Agents[0].ID.String())
				route = strings.ReplaceAll(route, "{buildnumber}", strconv.FormatInt(int64(a.Workspace.LatestBuild.BuildNumber), 10))
				route = strings.ReplaceAll(route, "{template}", a.Template.ID.String())
				route = strings.ReplaceAll(route, "{hash}", a.File.Hash)
				route = strings.ReplaceAll(route, "{workspaceresource}", a.WorkspaceResource.ID.String())
				route = strings.ReplaceAll(route, "{workspaceapp}", a.WorkspaceResource.Agents[0].Apps[0].Name)
				route = strings.ReplaceAll(route, "{templateversion}", a.Version.ID.String())
				route = strings.ReplaceAll(route, "{templateversiondryrun}", a.TemplateVersionDryRun.ID.String())
				route = strings.ReplaceAll(route, "{templatename}", a.Template.Name)
				// Only checking template scoped params here
				route = strings.ReplaceAll(route, "{scope}", string(a.TemplateParam.Scope))
				route = strings.ReplaceAll(route, "{id}", a.TemplateParam.ScopeID.String())

				resp, err := a.client.Request(ctx, method, route, nil)
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
}

type authCall struct {
	SubjectID string
	Roles     []string
	Action    rbac.Action
	Object    rbac.Object
}

type recordingAuthorizer struct {
	Called       *authCall
	AlwaysReturn error
}

func (r *recordingAuthorizer) ByRoleName(_ context.Context, subjectID string, roleNames []string, action rbac.Action, object rbac.Object) error {
	r.Called = &authCall{
		SubjectID: subjectID,
		Roles:     roleNames,
		Action:    action,
		Object:    object,
	}
	return r.AlwaysReturn
}

func (r *recordingAuthorizer) PrepareByRoleName(_ context.Context, subjectID string, roles []string, action rbac.Action, _ string) (rbac.PreparedAuthorized, error) {
	return &fakePreparedAuthorizer{
		Original:  r,
		SubjectID: subjectID,
		Roles:     roles,
		Action:    action,
	}, nil
}

func (r *recordingAuthorizer) reset() {
	r.Called = nil
}

type fakePreparedAuthorizer struct {
	Original  *recordingAuthorizer
	SubjectID string
	Roles     []string
	Action    rbac.Action
}

func (f *fakePreparedAuthorizer) Authorize(ctx context.Context, object rbac.Object) error {
	return f.Original.ByRoleName(ctx, f.SubjectID, f.Roles, f.Action, object)
}
