package coderdtest

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/coder/coder/coderd"

	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
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

func NewAuthTester(t *testing.T, ctx context.Context) *AuthTester {
	authorizer := &recordingAuthorizer{}

	client, _, api := newWithAPI(t, &Options{Authorizer: authorizer, IncludeProvisionerD: true})
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

	err := chi.Walk(a.api.Handler, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
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
