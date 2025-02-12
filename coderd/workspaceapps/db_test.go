package workspaceapps_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func Test_ResolveRequest(t *testing.T) {
	t.Parallel()

	const (
		agentName         = "agent"
		appNameOwner      = "app-owner"
		appNameAuthed     = "app-authed"
		appNamePublic     = "app-public"
		appNameInvalidURL = "app-invalid-url"
		// Users can access unhealthy and initializing apps (as of 2024-02).
		appNameUnhealthy    = "app-unhealthy"
		appNameInitializing = "app-initializing"
		appNameEndsInS      = "app-ends-in-s"

		// This agent will never connect, so it will never become "connected".
		// Users cannot access unhealthy agents.
		agentNameUnhealthy    = "agent-unhealthy"
		appNameAgentUnhealthy = "app-agent-unhealthy"

		// This is not a valid URL we listen on in the test, but it needs to be
		// set to a value.
		appURL = "http://localhost:8080"
	)
	allApps := []string{appNameOwner, appNameAuthed, appNamePublic}

	// Start a listener for a server that always responds with 500 for the
	// unhealthy app.
	unhealthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("unhealthy"))
	}))
	t.Cleanup(unhealthySrv.Close)

	// Start a listener for a server that never responds.
	initializingServer, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = initializingServer.Close()
	})
	initializingURL := fmt.Sprintf("http://%s", initializingServer.Addr().String())

	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.DisablePathApps = false
	deploymentValues.Dangerous.AllowPathAppSharing = true
	deploymentValues.Dangerous.AllowPathAppSiteOwnerAccess = true

	client, closer, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		AppHostname:                 "*.test.coder.com",
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

	ctx := testutil.Context(t, testutil.WaitMedium)

	firstUser := coderdtest.CreateFirstUser(t, client)
	me, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	secondUserClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

	agentAuthToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, firstUser.OrganizationID, &echo.Responses{
		Parse:         echo.ParseComplete,
		ProvisionPlan: echo.PlanComplete,
		ProvisionApply: []*proto.Response{{
			Type: &proto.Response_Apply{
				Apply: &proto.ApplyComplete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{
							{
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
									{
										Slug:         appNameUnhealthy,
										DisplayName:  appNameUnhealthy,
										SharingLevel: proto.AppSharingLevel_PUBLIC,
										Url:          appURL,
										Healthcheck: &proto.Healthcheck{
											Url:       unhealthySrv.URL,
											Interval:  1,
											Threshold: 1,
										},
									},
									{
										Slug:         appNameInitializing,
										DisplayName:  appNameInitializing,
										SharingLevel: proto.AppSharingLevel_PUBLIC,
										Url:          appURL,
										Healthcheck: &proto.Healthcheck{
											Url:       initializingURL,
											Interval:  30,
											Threshold: 1000,
										},
									},
									{
										Slug:         appNameEndsInS,
										DisplayName:  appNameEndsInS,
										SharingLevel: proto.AppSharingLevel_OWNER,
										Url:          appURL,
									},
								},
							},
							{
								Id:   uuid.NewString(),
								Name: agentNameUnhealthy,
								Auth: &proto.Agent_Token{
									Token: uuid.NewString(),
								},
								Apps: []*proto.App{
									{
										Slug:         appNameAgentUnhealthy,
										DisplayName:  appNameAgentUnhealthy,
										SharingLevel: proto.AppSharingLevel_PUBLIC,
										Url:          appURL,
									},
								},
							},
						},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, firstUser.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	_ = agenttest.New(t, client.URL, agentAuthToken)
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID, agentName)

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

				// Try resolving a request for each app as the owner, without a
				// token, then use the token to resolve each app.
				for _, app := range allApps {
					req := (workspaceapps.Request{
						AccessMethod:      workspaceapps.AccessMethodPath,
						BasePath:          "/app",
						UsernameOrID:      me.Username,
						WorkspaceNameOrID: c.workspaceNameOrID,
						AgentNameOrID:     c.agentNameOrID,
						AppSlugOrPort:     app,
					}).Normalize()

					t.Log("app", app)
					rw := httptest.NewRecorder()
					r := httptest.NewRequest("GET", "/app", nil)
					r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

					// Try resolving the request without a token.
					token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
						Logger:              api.Logger,
						SignedTokenProvider: api.WorkspaceAppsProvider,
						DashboardURL:        api.AccessURL,
						PathAppBaseURL:      api.AccessURL,
						AppHostname:         api.AppHostname,
						AppRequest:          req,
					})
					w := rw.Result()
					if !assert.True(t, ok) {
						dump, err := httputil.DumpResponse(w, true)
						require.NoError(t, err, "error dumping failed response")
						t.Log(string(dump))
						return
					}
					_ = w.Body.Close()

					require.Equal(t, &workspaceapps.SignedToken{
						RegisteredClaims: jwtutils.RegisteredClaims{
							Expiry: jwt.NewNumericDate(token.Expiry.Time()),
						},
						Request:     req,
						UserID:      me.ID,
						WorkspaceID: workspace.ID,
						AgentID:     agentID,
						AppURL:      appURL,
					}, token)
					require.NotZero(t, token.Expiry)
					require.WithinDuration(t, time.Now().Add(workspaceapps.DefaultTokenExpiry), token.Expiry.Time(), time.Minute)

					// Check that the token was set in the response and is valid.
					require.Len(t, w.Cookies(), 1)
					cookie := w.Cookies()[0]
					require.Equal(t, codersdk.SignedAppTokenCookie, cookie.Name)
					require.Equal(t, req.BasePath, cookie.Path)

					var parsedToken workspaceapps.SignedToken
					err := jwtutils.Verify(ctx, api.AppSigningKeyCache, cookie.Value, &parsedToken)
					require.NoError(t, err)
					// normalize expiry
					require.WithinDuration(t, token.Expiry.Time(), parsedToken.Expiry.Time(), 2*time.Second)
					parsedToken.Expiry = token.Expiry
					require.Equal(t, token, &parsedToken)

					// Try resolving the request with the token only.
					rw = httptest.NewRecorder()
					r = httptest.NewRequest("GET", "/app", nil)
					r.AddCookie(cookie)

					secondToken, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
						Logger:              api.Logger,
						SignedTokenProvider: api.WorkspaceAppsProvider,
						DashboardURL:        api.AccessURL,
						PathAppBaseURL:      api.AccessURL,
						AppHostname:         api.AppHostname,
						AppRequest:          req,
					})
					require.True(t, ok)
					// normalize expiry
					require.WithinDuration(t, token.Expiry.Time(), secondToken.Expiry.Time(), 2*time.Second)
					secondToken.Expiry = token.Expiry
					require.Equal(t, token, secondToken)
				}
			})
		}
	})

	t.Run("AuthenticatedOtherUser", func(t *testing.T) {
		t.Parallel()

		for _, app := range allApps {
			req := (workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      me.Username,
				WorkspaceNameOrID: workspace.Name,
				AgentNameOrID:     agentName,
				AppSlugOrPort:     app,
			}).Normalize()

			t.Log("app", app)
			rw := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/app", nil)
			r.Header.Set(codersdk.SessionTokenHeader, secondUserClient.SessionToken())

			token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
				Logger:              api.Logger,
				SignedTokenProvider: api.WorkspaceAppsProvider,
				DashboardURL:        api.AccessURL,
				PathAppBaseURL:      api.AccessURL,
				AppHostname:         api.AppHostname,
				AppRequest:          req,
			})
			w := rw.Result()
			_ = w.Body.Close()
			if app == appNameOwner {
				require.False(t, ok)
				require.Nil(t, token)
				require.NotZero(t, w.StatusCode)
				require.Equal(t, http.StatusNotFound, w.StatusCode)
				return
			}
			require.True(t, ok)
			require.NotNil(t, token)
			require.Zero(t, w.StatusCode)
		}
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		t.Parallel()

		for _, app := range allApps {
			req := (workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      me.Username,
				WorkspaceNameOrID: workspace.Name,
				AgentNameOrID:     agentName,
				AppSlugOrPort:     app,
			}).Normalize()

			t.Log("app", app)
			rw := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/app", nil)
			token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
				Logger:              api.Logger,
				SignedTokenProvider: api.WorkspaceAppsProvider,
				DashboardURL:        api.AccessURL,
				PathAppBaseURL:      api.AccessURL,
				AppHostname:         api.AppHostname,
				AppRequest:          req,
			})
			w := rw.Result()
			if app != appNamePublic {
				require.False(t, ok)
				require.Nil(t, token)
				require.NotZero(t, rw.Code)
				require.NotEqual(t, http.StatusOK, rw.Code)
			} else {
				if !assert.True(t, ok) {
					dump, err := httputil.DumpResponse(w, true)
					require.NoError(t, err, "error dumping failed response")
					t.Log(string(dump))
					return
				}
				require.NotNil(t, token)
				if rw.Code != 0 && rw.Code != http.StatusOK {
					t.Fatalf("expected 200 (or unset) response code, got %d", rw.Code)
				}
			}
			_ = w.Body.Close()
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod: "invalid",
		}).Normalize()
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.False(t, ok)
		require.Nil(t, token)
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
				name:              "WorkspaceOnly",
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
				req := (workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodPath,
					BasePath:          "/app",
					UsernameOrID:      me.Username,
					WorkspaceAndAgent: c.workspaceAndAgent,
					AppSlugOrPort:     appNamePublic,
				}).Normalize()

				rw := httptest.NewRecorder()
				r := httptest.NewRequest("GET", "/app", nil)
				r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

				token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
					Logger:              api.Logger,
					SignedTokenProvider: api.WorkspaceAppsProvider,
					DashboardURL:        api.AccessURL,
					PathAppBaseURL:      api.AccessURL,
					AppHostname:         api.AppHostname,
					AppRequest:          req,
				})
				w := rw.Result()
				if !assert.Equal(t, c.ok, ok) {
					dump, err := httputil.DumpResponse(w, true)
					require.NoError(t, err, "error dumping failed response")
					t.Log(string(dump))
					return
				}
				if c.ok {
					require.NotNil(t, token)
					require.Equal(t, token.WorkspaceNameOrID, c.workspace)
					require.Equal(t, token.AgentNameOrID, c.agent)
					require.Equal(t, token.WorkspaceID, workspace.ID)
					require.Equal(t, token.AgentID, agentID)
				} else {
					require.Nil(t, token)
				}
				_ = w.Body.Close()
			})
		}
	})

	t.Run("TokenDoesNotMatchRequest", func(t *testing.T) {
		t.Parallel()

		badToken := workspaceapps.SignedToken{
			Request: (workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      me.Username,
				WorkspaceNameOrID: workspace.Name,
				AgentNameOrID:     agentName,
				// App name differs
				AppSlugOrPort: appNamePublic,
			}).Normalize(),
			RegisteredClaims: jwtutils.RegisteredClaims{
				Expiry: jwt.NewNumericDate(time.Now().Add(time.Minute)),
			},
			UserID:      me.ID,
			WorkspaceID: workspace.ID,
			AgentID:     agentID,
			AppURL:      appURL,
		}

		badTokenStr, err := jwtutils.Sign(ctx, api.AppSigningKeyCache, badToken)
		require.NoError(t, err)

		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			// App name differs
			AppSlugOrPort: appNameOwner,
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.AddCookie(&http.Cookie{
			Name:  codersdk.SignedAppTokenCookie,
			Value: badTokenStr,
		})

		// Even though the token is invalid, we should still perform request
		// resolution without failure since we'll just ignore the bad token.
		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.True(t, ok)
		require.NotNil(t, token)
		require.Equal(t, appNameOwner, token.AppSlugOrPort)

		// Cookie should be set in response, and it should be a different
		// token.
		w := rw.Result()
		_ = w.Body.Close()
		cookies := w.Cookies()
		require.Len(t, cookies, 1)
		require.Equal(t, cookies[0].Name, codersdk.SignedAppTokenCookie)
		require.NotEqual(t, cookies[0].Value, badTokenStr)
		var parsedToken workspaceapps.SignedToken
		err = jwtutils.Verify(ctx, api.AppSigningKeyCache, cookies[0].Value, &parsedToken)
		require.NoError(t, err)
		require.Equal(t, appNameOwner, parsedToken.AppSlugOrPort)
	})

	t.Run("PortPathBlocked", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     "8080",
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.False(t, ok)
		require.Nil(t, token)
	})

	t.Run("PortSubdomain", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodSubdomain,
			BasePath:          "/",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     "9090",
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.True(t, ok)
		require.Equal(t, req.AppSlugOrPort, token.AppSlugOrPort)
		require.Equal(t, "http://127.0.0.1:9090", token.AppURL)
	})

	t.Run("PortSubdomainHTTPSS", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodSubdomain,
			BasePath:          "/",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     "9090ss",
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		_, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		// should parse as app and fail to find app "9090ss"
		require.False(t, ok)
		w := rw.Result()
		_ = w.Body.Close()
		b, err := io.ReadAll(w.Body)
		require.NoError(t, err)
		require.Contains(t, string(b), "404 - Application Not Found")
	})

	t.Run("SubdomainEndsInS", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodSubdomain,
			BasePath:          "/",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     appNameEndsInS,
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.True(t, ok)
		require.Equal(t, req.AppSlugOrPort, token.AppSlugOrPort)
	})

	t.Run("Terminal", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod:  workspaceapps.AccessMethodTerminal,
			BasePath:      "/app",
			AgentNameOrID: agentID.String(),
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.True(t, ok)
		require.Equal(t, req.AccessMethod, token.AccessMethod)
		require.Equal(t, req.BasePath, token.BasePath)
		require.Empty(t, token.UsernameOrID)
		require.Empty(t, token.WorkspaceNameOrID)
		require.Equal(t, req.AgentNameOrID, token.Request.AgentNameOrID)
		require.Empty(t, token.AppSlugOrPort)
		require.Empty(t, token.AppURL)
	})

	t.Run("InsufficientPermissions", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     appNameOwner,
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, secondUserClient.SessionToken())

		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.False(t, ok)
		require.Nil(t, token)
	})

	t.Run("UserNotFound", func(t *testing.T) {
		t.Parallel()
		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      "thisuserdoesnotexist",
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     appNameOwner,
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.False(t, ok)
		require.Nil(t, token)
	})

	t.Run("RedirectSubdomainAuth", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodSubdomain,
			BasePath:          "/",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     appNameOwner,
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/some-path", nil)
		// Should not be used as the hostname in the redirect URI.
		r.Host = "app.com"

		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
			AppPath:             "/some-path",
		})
		require.False(t, ok)
		require.Nil(t, token)

		w := rw.Result()
		defer w.Body.Close()
		require.Equal(t, http.StatusSeeOther, w.StatusCode)

		loc, err := w.Location()
		require.NoError(t, err)

		require.Equal(t, api.AccessURL.Scheme, loc.Scheme)
		require.Equal(t, api.AccessURL.Host, loc.Host)
		require.Equal(t, "/api/v2/applications/auth-redirect", loc.Path)

		redirectURIStr := loc.Query().Get(workspaceapps.RedirectURIQueryParam)
		redirectURI, err := url.Parse(redirectURIStr)
		require.NoError(t, err)

		appHost := appurl.ApplicationURL{
			Prefix:        "",
			AppSlugOrPort: req.AppSlugOrPort,
			AgentName:     req.AgentNameOrID,
			WorkspaceName: req.WorkspaceNameOrID,
			Username:      req.UsernameOrID,
		}
		host := strings.Replace(api.AppHostname, "*", appHost.String(), 1)

		require.Equal(t, "http", redirectURI.Scheme)
		require.Equal(t, host, redirectURI.Host)
		require.Equal(t, "/some-path", redirectURI.Path)
	})

	t.Run("UnhealthyAgent", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentNameUnhealthy,
			AppSlugOrPort:     appNameAgentUnhealthy,
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.False(t, ok, "request succeeded even though agent is not connected")
		require.Nil(t, token)

		w := rw.Result()
		defer w.Body.Close()
		require.Equal(t, http.StatusBadGateway, w.StatusCode)

		body, err := io.ReadAll(w.Body)
		require.NoError(t, err)
		bodyStr := string(body)
		bodyStr = strings.ReplaceAll(bodyStr, "&#34;", `"`)
		// It'll either be "connecting" or "disconnected". Both are OK for this
		// test.
		require.Contains(t, bodyStr, `Agent state is "`)
	})

	// Initializing apps are now permitted to connect anyways. This wasn't
	// always the case, but we're testing the behavior to ensure it doesn't
	// change back accidentally.
	t.Run("InitializingAppPermitted", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		agent, err := client.WorkspaceAgent(ctx, agentID)
		require.NoError(t, err)

		for _, app := range agent.Apps {
			if app.Slug == appNameInitializing {
				t.Log("app is", app.Health)
				require.Equal(t, codersdk.WorkspaceAppHealthInitializing, app.Health)
				break
			}
		}

		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     appNameInitializing,
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.True(t, ok, "ResolveRequest failed, should pass even though app is initializing")
		require.NotNil(t, token)
	})

	// Unhealthy apps are now permitted to connect anyways. This wasn't always
	// the case, but we're testing the behavior to ensure it doesn't change back
	// accidentally.
	t.Run("UnhealthyAppPermitted", func(t *testing.T) {
		t.Parallel()

		require.Eventually(t, func() bool {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancel()

			agent, err := client.WorkspaceAgent(ctx, agentID)
			if err != nil {
				t.Log("could not get agent", err)
				return false
			}

			for _, app := range agent.Apps {
				if app.Slug == appNameUnhealthy {
					t.Log("app is", app.Health)
					return app.Health == codersdk.WorkspaceAppHealthUnhealthy
				}
			}

			t.Log("could not find app")
			return false
		}, testutil.WaitLong, testutil.IntervalFast, "wait for app to become unhealthy")

		req := (workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodPath,
			BasePath:          "/app",
			UsernameOrID:      me.Username,
			WorkspaceNameOrID: workspace.Name,
			AgentNameOrID:     agentName,
			AppSlugOrPort:     appNameUnhealthy,
		}).Normalize()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		token, ok := workspaceapps.ResolveRequest(rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.True(t, ok, "ResolveRequest failed, should pass even though app is unhealthy")
		require.NotNil(t, token)
	})
}
