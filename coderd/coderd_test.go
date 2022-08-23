package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestBuildInfo(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	buildInfo, err := client.BuildInfo(ctx)
	require.NoError(t, err)
	require.Equal(t, buildinfo.ExternalURL(), buildInfo.ExternalURL, "external URL")
	require.Equal(t, buildinfo.Version(), buildInfo.Version, "version")
}

// TestAuthorizeAllEndpoints will check `authorize` is called on every endpoint registered.
func TestAuthorizeAllEndpoints(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	a := coderdtest.NewAuthTester(t, ctx)
	// Some quick reused objects
	workspaceRBACObj := rbac.ResourceWorkspace.InOrg(a.Organization.ID).WithOwner(a.Workspace.OwnerID.String())
	workspaceExecObj := rbac.ResourceWorkspaceExecution.InOrg(a.Organization.ID).WithOwner(a.Workspace.OwnerID.String())
	// skipRoutes allows skipping routes from being checked.
	skipRoutes := map[string]string{
		"POST:/api/v2/users/logout": "Logging out deletes the API Key for other routes",
	}

	assertRoute := map[string]coderdtest.RouteCheck{
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
	a.Test(ctx, assertRoute, skipRoutes)
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
