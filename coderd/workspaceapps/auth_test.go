package workspaceapps_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
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
		agentName         = "agent"
		appNameOwner      = "app-owner"
		appNameAuthed     = "app-authed"
		appNamePublic     = "app-public"
		appNameInvalidURL = "app-invalid-url"

		// This is not a valid URL we listen on in the test, but it needs to be
		// set to a value.
		appURL = "http://localhost:8080"
	)
	allApps := []string{appNameOwner, appNameAuthed, appNamePublic}

	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.DisablePathApps = false
	deploymentValues.Dangerous.AllowPathAppSharing = true
	deploymentValues.Dangerous.AllowPathAppSiteOwnerAccess = true

	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:            deploymentValues,
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

	secondUserClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

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
								{
									Slug:         appNameInvalidURL,
									DisplayName:  appNameInvalidURL,
									SharingLevel: proto.AppSharingLevel_PUBLIC,
									Url:          "test:path/to/app",
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

		cases := []struct {
			name              string
			workspaceNameOrID string
			agentNameOrID     string
		}{
			{
				name:              "Names",
				workspaceNameOrID: workspace.Name,
				agentNameOrID:     agentName,
			},
			{
				name:              "IDs",
				workspaceNameOrID: workspace.ID.String(),
				agentNameOrID:     agentID.String(),
			},
		}

		for _, c := range cases {
			c := c

			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				// Try resolving a request for each app as the owner, without a ticket,
				// then use the ticket to resolve each app.
				for _, app := range allApps {
					req := workspaceapps.Request{
						AccessMethod:      workspaceapps.AccessMethodPath,
						BasePath:          "/app",
						UsernameOrID:      me.Username,
						WorkspaceNameOrID: c.workspaceNameOrID,
						AgentNameOrID:     c.agentNameOrID,
						AppSlugOrPort:     app,
					}

					t.Log("app", app)
					rw := httptest.NewRecorder()
					r := httptest.NewRequest("GET", "/app", nil)
					r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

					// Try resolving the request without a ticket.
					ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
					w := rw.Result()
					if !assert.True(t, ok) {
						dump, err := httputil.DumpResponse(w, true)
						require.NoError(t, err, "error dumping failed response")
						t.Log(string(dump))
						return
					}
					_ = w.Body.Close()

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
					require.Len(t, w.Cookies(), 1)
					cookie := w.Cookies()[0]
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
	})

	t.Run("AuthenticatedOtherUser", func(t *testing.T) {
		t.Parallel()

		for _, app := range allApps {
			req := workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      me.Username,
				WorkspaceNameOrID: workspace.Name,
				AgentNameOrID:     agentName,
				AppSlugOrPort:     app,
			}

			t.Log("app", app)
			rw := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/app", nil)
			r.Header.Set(codersdk.SessionTokenHeader, secondUserClient.SessionToken())

			ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
			w := rw.Result()
			_ = w.Body.Close()
			if app == appNameOwner {
				require.False(t, ok)
				require.Nil(t, ticket)
				require.NotZero(t, w.StatusCode)
				require.Equal(t, http.StatusNotFound, w.StatusCode)
				return
			}
			require.True(t, ok)
			require.NotNil(t, ticket)
			require.Zero(t, w.StatusCode)
		}
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		for _, app := range allApps {
			req := workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      me.Username,
				WorkspaceNameOrID: workspace.Name,
				AgentNameOrID:     agentName,
				AppSlugOrPort:     app,
			}

			t.Log("app", app)
			rw := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/app", nil)
			ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
			w := rw.Result()
			if app != appNamePublic {
				require.False(t, ok)
				require.Nil(t, ticket)
				require.NotZero(t, rw.Code)
				require.NotEqual(t, http.StatusOK, rw.Code)
			} else {
				if !assert.True(t, ok) {
					dump, err := httputil.DumpResponse(w, true)
					require.NoError(t, err, "error dumping failed response")
					t.Log(string(dump))
					return
				}
				require.NotNil(t, ticket)
				if rw.Code != 0 && rw.Code != http.StatusOK {
					t.Fatalf("expected 200 (or unset) response code, got %d", rw.Code)
				}
			}
			_ = w.Body.Close()
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()

		req := workspaceapps.Request{
			AccessMethod: "invalid",
		}
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
		require.False(t, ok)
		require.Nil(t, ticket)
	})

	t.Run("SplitWorkspaceAndAgent", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name              string
			workspaceAndAgent string
			workspace         string
			agent             string
			ok                bool
		}{
			{
				name:              "WorkspaecOnly",
				workspaceAndAgent: workspace.Name,
				workspace:         workspace.Name,
				agent:             "",
				ok:                true,
			},
			{
				name:              "WorkspaceAndAgent",
				workspaceAndAgent: fmt.Sprintf("%s.%s", workspace.Name, agentName),
				workspace:         workspace.Name,
				agent:             agentName,
				ok:                true,
			},
			{
				name:              "WorkspaceID",
				workspaceAndAgent: workspace.ID.String(),
				workspace:         workspace.ID.String(),
				agent:             "",
				ok:                true,
			},
			{
				name:              "WorkspaceIDAndAgentID",
				workspaceAndAgent: fmt.Sprintf("%s.%s", workspace.ID, agentID),
				workspace:         workspace.ID.String(),
				agent:             agentID.String(),
				ok:                true,
			},
			{
				name:              "Invalid1",
				workspaceAndAgent: "invalid",
				ok:                false,
			},
			{
				name:              "Invalid2",
				workspaceAndAgent: ".",
				ok:                false,
			},
			{
				name:              "Slash",
				workspaceAndAgent: fmt.Sprintf("%s/%s", workspace.Name, agentName),
				ok:                false,
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				req := workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodPath,
					BasePath:          "/app",
					UsernameOrID:      me.Username,
					WorkspaceAndAgent: c.workspaceAndAgent,
					AppSlugOrPort:     appNamePublic,
				}

				rw := httptest.NewRecorder()
				r := httptest.NewRequest("GET", "/app", nil)
				r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

				ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
				w := rw.Result()
				if !assert.Equal(t, c.ok, ok) {
					dump, err := httputil.DumpResponse(w, true)
					require.NoError(t, err, "error dumping failed response")
					t.Log(string(dump))
					return
				}
				if c.ok {
					require.NotNil(t, ticket)
					require.Equal(t, ticket.WorkspaceNameOrID, c.workspace)
					require.Equal(t, ticket.AgentNameOrID, c.agent)
					require.Equal(t, ticket.WorkspaceID, workspace.ID)
					require.Equal(t, ticket.AgentID, agentID)
				} else {
					require.Nil(t, ticket)
				}
				_ = w.Body.Close()
			})
		}
	})

	t.Run("TicketDoesNotMatchRequest", func(t *testing.T) {
		t.Parallel()

		badTicket := workspaceapps.Ticket{
			AccessMethod:      workspaceapps.AccessMethodPath,
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			// App name differs
			AppSlugOrPort: appNamePublic,
			Expiry:        time.Now().Add(time.Minute).Unix(),
			UserID:        me.ID,
			WorkspaceID:   workspace.ID,
			AgentID:       agentID,
			AppURL:        appURL,
		}
		badTicketStr, err := api.WorkspaceAppsProvider.GenerateTicket(badTicket)
		require.NoError(t, err)

		req := workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			// App name differs
			AppSlugOrPort: appNameOwner,
		}

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.AddCookie(&http.Cookie{
			Name:  codersdk.DevURLSessionTicketCookie,
			Value: badTicketStr,
		})

		// Even though the ticket is invalid, we should still perform request
		// resolution.
		ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
		require.True(t, ok)
		require.NotNil(t, ticket)
		require.Equal(t, appNameOwner, ticket.AppSlugOrPort)

		// Cookie should be set in response, and it should be a different
		// ticket.
		w := rw.Result()
		_ = w.Body.Close()
		cookies := w.Cookies()
		require.Len(t, cookies, 1)
		require.Equal(t, cookies[0].Name, codersdk.DevURLSessionTicketCookie)
		require.NotEqual(t, cookies[0].Value, badTicketStr)
		parsedTicket, err := api.WorkspaceAppsProvider.ParseTicket(cookies[0].Value)
		require.NoError(t, err)
		require.Equal(t, appNameOwner, parsedTicket.AppSlugOrPort)
	})

	t.Run("PortPathBlocked", func(t *testing.T) {
		t.Parallel()

		req := workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     "8080",
		}

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
		require.False(t, ok)
		require.Nil(t, ticket)
	})

	t.Run("PortSubdomain", func(t *testing.T) {
		t.Parallel()

		req := workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodSubdomain,
			BasePath:          "/",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     "9090",
		}

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
		require.True(t, ok)
		require.Equal(t, req.AppSlugOrPort, ticket.AppSlugOrPort)
		require.Equal(t, "http://127.0.0.1:9090", ticket.AppURL)
	})

	t.Run("InsufficientPermissions", func(t *testing.T) {
		t.Parallel()

		req := workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     appNameOwner,
		}

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, secondUserClient.SessionToken())

		ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
		require.False(t, ok)
		require.Nil(t, ticket)
	})

	t.Run("UserNotFound", func(t *testing.T) {
		t.Parallel()
		req := workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      "thisuserdoesnotexist",
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     appNameOwner,
		}

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
		require.False(t, ok)
		require.Nil(t, ticket)
	})

	t.Run("RedirectSubdomainAuth", func(t *testing.T) {
		t.Parallel()

		req := workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodSubdomain,
			BasePath:          "/",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     appNameOwner,
		}

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/some-path", nil)
		r.Host = "app.com"

		ticket, ok := api.WorkspaceAppsProvider.ResolveRequest(rw, r, req)
		require.False(t, ok)
		require.Nil(t, ticket)

		w := rw.Result()
		defer w.Body.Close()
		require.Equal(t, http.StatusTemporaryRedirect, w.StatusCode)

		loc, err := w.Location()
		require.NoError(t, err)

		require.Equal(t, api.AccessURL.Scheme, loc.Scheme)
		require.Equal(t, api.AccessURL.Host, loc.Host)
		require.Equal(t, "/api/v2/applications/auth-redirect", loc.Path)

		redirectURIStr := loc.Query().Get(workspaceapps.RedirectURIQueryParam)
		redirectURI, err := url.Parse(redirectURIStr)
		require.NoError(t, err)

		require.Equal(t, "http", redirectURI.Scheme)
		require.Equal(t, "app.com", redirectURI.Host)
		require.Equal(t, "/some-path", redirectURI.Path)
	})
}
