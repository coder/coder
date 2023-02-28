package workspaceapps_test

import (
	"context"
	"net"
	"net/http/httptest"
	"net/http/httputil"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func Test_ResolveRequest(t *testing.T) {
	t.Parallel()

	const (
		agentName     = "agent"
		appNameOwner  = "app-owner"
		appNameAuthed = "app-authed"
		appNamePublic = "app-public"
		// This is not a valid URL we listen on in the test, but it needs to be
		// set to a value.
		appURL = "http://localhost:8080"
	)
	allApps := []string{appNameOwner, appNameAuthed, appNamePublic}

	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon:    true,
		AgentStatsRefreshInterval:   time.Millisecond * 100,
		MetricsCacheRefreshInterval: time.Millisecond * 100,
		RealIPConfig: &httpmw.RealIPConfig{
			TrustedOrigins: []*net.IPNet{{
				IP:   net.ParseIP("127.0.0.1"),
				Mask: net.CIDRMask(8, 32),
			}},
			TrustedHeaders: []string{
				"CF-Connecting-IP",
			},
		},
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	firstUser := coderdtest.CreateFirstUser(t, client)
	me, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	agentAuthToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, firstUser.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.ProvisionComplete,
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id:   uuid.NewString(),
							Name: agentName,
							Auth: &proto.Agent_Token{
								Token: agentAuthToken,
							},
							Apps: []*proto.App{
								{
									Slug:         appNameOwner,
									DisplayName:  appNameOwner,
									SharingLevel: proto.AppSharingLevel_OWNER,
									Url:          appURL,
								},
								{
									Slug:         appNameAuthed,
									DisplayName:  appNameAuthed,
									SharingLevel: proto.AppSharingLevel_AUTHENTICATED,
									Url:          appURL,
								},
								{
									Slug:         appNamePublic,
									DisplayName:  appNamePublic,
									SharingLevel: proto.AppSharingLevel_PUBLIC,
									Url:          appURL,
								},
							},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, firstUser.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, firstUser.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(agentAuthToken)
	agentCloser := agent.New(agent.Options{
		Client: agentClient,
		Logger: slogtest.Make(t, nil).Named("agent"),
	})
	t.Cleanup(func() {
		_ = agentCloser.Close()
	})
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	agentID := uuid.Nil
	for _, resource := range resources {
		for _, agnt := range resource.Agents {
			if agnt.Name == agentName {
				agentID = agnt.ID
			}
		}
	}
	require.NotEqual(t, uuid.Nil, agentID)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		// Try resolving a request for each app as the owner, without a ticket,
		// then use the ticket to resolve each app.
		for _, app := range allApps {
			req := workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      me.Username,
				WorkspaceNameOrID: workspace.Name,
				AgentNameOrID:     agentName,
				AppSlugOrPort:     app,
			}

			rw := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/app", nil)
			r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

			// Try resolving the request without a ticket.
			ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
			if !assert.True(t, ok) {
				dump, err := httputil.DumpResponse(rw.Result(), true)
				require.NoError(t, err, "error dumping failed response")
				t.Log(string(dump))
				return
			}

			require.Equal(t, &workspaceapps.Ticket{
				AccessMethod:      req.AccessMethod,
				UsernameOrID:      req.UsernameOrID,
				WorkspaceNameOrID: req.WorkspaceNameOrID,
				AgentNameOrID:     req.AgentNameOrID,
				AppSlugOrPort:     req.AppSlugOrPort,
				Expiry:            ticket.Expiry, // ignored to avoid flakiness
				UserID:            me.ID,
				WorkspaceID:       workspace.ID,
				AgentID:           agentID,
				AppURL:            appURL,
			}, ticket)
			require.NotZero(t, ticket.Expiry)
			require.InDelta(t, time.Now().Add(workspaceapps.TicketExpiry).Unix(), ticket.Expiry, time.Minute.Seconds())

			// Check that the ticket was set in the response and is valid.
			require.Len(t, rw.Result().Cookies(), 1)
			cookie := rw.Result().Cookies()[0]
			require.Equal(t, codersdk.DevURLSessionTicketCookie, cookie.Name)
			require.Equal(t, req.BasePath, cookie.Path)

			parsedTicket, err := api.WorkspaceAppsProvider.ParseTicket(cookie.Value)
			require.NoError(t, err)
			require.Equal(t, ticket, &parsedTicket)

			// Try resolving the request with the ticket only.
			rw = httptest.NewRecorder()
			r = httptest.NewRequest("GET", "/app", nil)
			r.AddCookie(cookie)

			secondTicket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
			require.True(t, ok)
			require.Equal(t, ticket, secondTicket)
		}
	})
}
