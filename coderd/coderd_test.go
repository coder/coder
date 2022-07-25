package coderd_test

import (
	"context"
	"crypto/x509"
	"database/sql"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/autobuild/executor"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/database/postgres"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/turnconn"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
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
	var (
		ctx        = context.Background()
		authorizer = &fakeAuthorizer{}
	)

	// This function was taken from coderdtest.newWithAPI. It is intentionally
	// copied to avoid exposing the API to other tests in coderd. Tests should
	// not need a reference to coderd.API...this test is an exception.
	newClient := func(authorizer rbac.Authorizer) (*codersdk.Client, *coderd.API) {
		// This can be hotswapped for a live database instance.
		db := databasefake.New()
		pubsub := database.NewPubsubInMemory()
		if os.Getenv("DB") != "" {
			connectionURL, closePg, err := postgres.Open()
			require.NoError(t, err)
			t.Cleanup(closePg)
			sqlDB, err := sql.Open("postgres", connectionURL)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = sqlDB.Close()
			})
			err = database.MigrateUp(sqlDB)
			require.NoError(t, err)
			db = database.New(sqlDB)

			pubsub, err = database.NewPubsub(context.Background(), sqlDB, connectionURL)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = pubsub.Close()
			})
		}

		tickerCh := make(chan time.Time)
		t.Cleanup(func() { close(tickerCh) })

		ctx, cancelFunc := context.WithCancel(context.Background())
		defer t.Cleanup(cancelFunc) // Defer to ensure cancelFunc is executed first.

		lifecycleExecutor := executor.New(
			ctx,
			db,
			slogtest.Make(t, nil).Named("autobuild.executor").Leveled(slog.LevelDebug),
			tickerCh,
		).WithStatsChannel(nil)
		lifecycleExecutor.Run()

		srv := httptest.NewUnstartedServer(nil)
		srv.Config.BaseContext = func(_ net.Listener) context.Context {
			return ctx
		}
		srv.Start()
		t.Cleanup(srv.Close)
		serverURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		turnServer, err := turnconn.New(nil)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = turnServer.Close()
		})

		validator, err := idtoken.NewValidator(ctx, option.WithoutAuthentication())
		require.NoError(t, err)

		// We set the handler after server creation for the access URL.
		coderAPI := coderd.New(&coderd.Options{
			AgentConnectionUpdateFrequency: 150 * time.Millisecond,
			AccessURL:                      serverURL,
			Logger:                         slogtest.Make(t, nil).Leveled(slog.LevelDebug),
			Database:                       db,
			Pubsub:                         pubsub,

			AWSCertificates:      nil,
			AzureCertificates:    x509.VerifyOptions{},
			GithubOAuth2Config:   nil,
			GoogleTokenValidator: validator,
			SSHKeygenAlgorithm:   gitsshkey.AlgorithmEd25519,
			TURNServer:           turnServer,
			APIRateLimit:         0,
			Authorizer:           authorizer,
			Telemetry:            telemetry.NewNoop(),
		})
		srv.Config.Handler = coderAPI.Handler

		_ = coderdtest.NewProvisionerDaemon(t, coderAPI)
		t.Cleanup(func() {
			_ = coderAPI.Close()
		})

		return codersdk.New(serverURL), coderAPI
	}

	client, api := newClient(authorizer)
	admin := coderdtest.CreateFirstUser(t, client)
	// The provisioner will call to coderd and register itself. This is async,
	// so we wait for it to occur.
	require.Eventually(t, func() bool {
		provisionerds, err := client.ProvisionerDaemons(ctx)
		return assert.NoError(t, err) && len(provisionerds) > 0
	}, time.Second*10, time.Second)

	provisionerds, err := client.ProvisionerDaemons(ctx)
	require.NoError(t, err, "fetch provisioners")
	require.Len(t, provisionerds, 1)

	organization, err := client.Organization(ctx, admin.OrganizationID)
	require.NoError(t, err, "fetch org")

	// Setup some data in the database.
	version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, &echo.Responses{
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
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, admin.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)
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

	// Some quick reused objects
	workspaceRBACObj := rbac.ResourceWorkspace.InOrg(organization.ID).WithID(workspace.ID.String()).WithOwner(workspace.OwnerID.String())

	// skipRoutes allows skipping routes from being checked.
	skipRoutes := map[string]string{
		"POST:/api/v2/users/logout": "Logging out deletes the API Key for other routes",
	}

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
		"GET:/api/v2/users/authmethods": {NoAuthorize: true},
		"POST:/api/v2/csp/reports":      {NoAuthorize: true},

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
		"GET:/api/v2/workspaceagents/{workspaceagent}/turn":       {NoAuthorize: true},
		"GET:/api/v2/workspaceagents/{workspaceagent}/derp":       {NoAuthorize: true},

		// These endpoints have more assertions. This is good, add more endpoints to assert if you can!
		"GET:/api/v2/organizations/{organization}": {AssertObject: rbac.ResourceOrganization.InOrg(admin.OrganizationID)},
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
			AssertAction: rbac.ActionUpdate,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspaceagents/{workspaceagent}/pty": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/workspaces/": {
			StatusCode:   http.StatusOK,
			AssertAction: rbac.ActionRead,
			AssertObject: workspaceRBACObj,
		},
		"GET:/api/v2/organizations/{organization}/templates": {
			StatusCode:   http.StatusOK,
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"POST:/api/v2/organizations/{organization}/templates": {
			AssertAction: rbac.ActionCreate,
			AssertObject: rbac.ResourceTemplate.InOrg(organization.ID),
		},
		"DELETE:/api/v2/templates/{template}": {
			AssertAction: rbac.ActionDelete,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/templates/{template}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"POST:/api/v2/files": {AssertAction: rbac.ActionCreate, AssertObject: rbac.ResourceFile},
		"GET:/api/v2/files/{fileHash}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceFile.WithOwner(admin.UserID.String()).WithID(file.Hash),
		},
		"GET:/api/v2/templates/{template}/versions": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"PATCH:/api/v2/templates/{template}/versions": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/templates/{template}/versions/{templateversionname}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/templateversions/{templateversion}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"PATCH:/api/v2/templateversions/{templateversion}/cancel": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/templateversions/{templateversion}/logs": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/templateversions/{templateversion}/parameters": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/templateversions/{templateversion}/resources": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/templateversions/{templateversion}/schema": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"POST:/api/v2/templateversions/{templateversion}/dry-run": {
			// The first check is to read the template
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(version.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{templateversiondryrun}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(version.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{templateversiondryrun}/resources": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(version.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/templateversions/{templateversion}/dry-run/{templateversiondryrun}/logs": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(version.OrganizationID).WithID(template.ID.String()),
		},
		"PATCH:/api/v2/templateversions/{templateversion}/dry-run/{templateversiondryrun}/cancel": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(version.OrganizationID).WithID(template.ID.String()),
		},
		"GET:/api/v2/provisionerdaemons": {
			StatusCode:   http.StatusOK,
			AssertObject: rbac.ResourceProvisionerDaemon.WithID(provisionerds[0].ID.String()),
		},

		"POST:/api/v2/parameters/{scope}/{id}": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: rbac.ResourceTemplate.WithID(template.ID.String()),
		},
		"GET:/api/v2/parameters/{scope}/{id}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.WithID(template.ID.String()),
		},
		"DELETE:/api/v2/parameters/{scope}/{id}/{name}": {
			AssertAction: rbac.ActionUpdate,
			AssertObject: rbac.ResourceTemplate.WithID(template.ID.String()),
		},
		"GET:/api/v2/organizations/{organization}/templates/{templatename}": {
			AssertAction: rbac.ActionRead,
			AssertObject: rbac.ResourceTemplate.InOrg(template.OrganizationID).WithID(template.ID.String()),
		},
		"POST:/api/v2/organizations/{organization}/workspaces": {
			AssertAction: rbac.ActionCreate,
			// No ID when creating
			AssertObject: workspaceRBACObj.WithID(""),
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

	for k, v := range assertRoute {
		noTrailSlash := strings.TrimRight(k, "/")
		if _, ok := assertRoute[noTrailSlash]; ok && noTrailSlash != k {
			t.Errorf("route %q & %q is declared twice", noTrailSlash, k)
			t.FailNow()
		}
		assertRoute[noTrailSlash] = v
	}

	for k, v := range skipRoutes {
		noTrailSlash := strings.TrimRight(k, "/")
		if _, ok := skipRoutes[noTrailSlash]; ok && noTrailSlash != k {
			t.Errorf("route %q & %q is declared twice", noTrailSlash, k)
			t.FailNow()
		}
		skipRoutes[noTrailSlash] = v
	}

	err = chi.Walk(api.Handler, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		name := method + ":" + route
		if _, ok := skipRoutes[strings.TrimRight(name, "/")]; ok {
			return nil
		}
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
			route = strings.ReplaceAll(route, "{organizationname}", organization.Name)
			route = strings.ReplaceAll(route, "{workspace}", workspace.ID.String())
			route = strings.ReplaceAll(route, "{workspacebuild}", workspace.LatestBuild.ID.String())
			route = strings.ReplaceAll(route, "{workspacename}", workspace.Name)
			route = strings.ReplaceAll(route, "{workspacebuildname}", workspace.LatestBuild.Name)
			route = strings.ReplaceAll(route, "{workspaceagent}", workspaceResources[0].Agents[0].ID.String())
			route = strings.ReplaceAll(route, "{buildnumber}", strconv.FormatInt(int64(workspace.LatestBuild.BuildNumber), 10))
			route = strings.ReplaceAll(route, "{template}", template.ID.String())
			route = strings.ReplaceAll(route, "{hash}", file.Hash)
			route = strings.ReplaceAll(route, "{workspaceresource}", workspaceResources[0].ID.String())
			route = strings.ReplaceAll(route, "{workspaceapp}", workspaceResources[0].Agents[0].Apps[0].Name)
			route = strings.ReplaceAll(route, "{templateversion}", version.ID.String())
			route = strings.ReplaceAll(route, "{templateversiondryrun}", templateVersionDryRun.ID.String())
			route = strings.ReplaceAll(route, "{templatename}", template.Name)
			// Only checking template scoped params here
			route = strings.ReplaceAll(route, "{scope}", string(templateParam.Scope))
			route = strings.ReplaceAll(route, "{id}", templateParam.ScopeID.String())

			resp, err := client.Request(context.Background(), method, route, nil)
			require.NoError(t, err, "do req")
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Response Body: %q", string(body))
			_ = resp.Body.Close()

			if !routeAssertions.NoAuthorize {
				assert.NotNil(t, authorizer.Called, "authorizer expected")
				if routeAssertions.StatusCode != 0 {
					assert.Equal(t, routeAssertions.StatusCode, resp.StatusCode, "expect unauthorized")
				} else {
					// It's either a 404 or 403.
					if resp.StatusCode != http.StatusNotFound {
						assert.Equal(t, http.StatusForbidden, resp.StatusCode, "expect unauthorized")
					}
				}
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
