package workspaceapps_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/tracing"
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

	auditor := audit.NewMock()
	t.Cleanup(func() {
		if t.Failed() {
			return
		}
		assert.Len(t, auditor.AuditLogs(), 0, "one or more test cases produced unexpected audit logs, did you replace the auditor or forget to call ResetLogs?")
	})
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
		Auditor: auditor,
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})

	ctx := testutil.Context(t, testutil.WaitMedium)

	firstUser := coderdtest.CreateFirstUser(t, client)
	me, err := client.User(ctx, codersdk.Me)
	require.NoError(t, err)

	secondUserClient, secondUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

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
				break
			}
		}
	}
	require.NotEqual(t, uuid.Nil, agentID)

	//nolint:gocritic // This is a test, allow dbauthz.AsSystemRestricted.
	agent, err := api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agentID)
	require.NoError(t, err)

	//nolint:gocritic // This is a test, allow dbauthz.AsSystemRestricted.
	apps, err := api.Database.GetWorkspaceAppsByAgentID(dbauthz.AsSystemRestricted(ctx), agentID)
	require.NoError(t, err)
	appsBySlug := make(map[string]database.WorkspaceApp, len(apps))
	for _, app := range apps {
		appsBySlug[app.Slug] = app
	}

	// Reset audit logs so cleanup check can pass.
	auditor.ResetLogs()

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

					auditor := audit.NewMock()
					auditableIP := randomIPv6(t)
					auditableUA := "Tidua"

					t.Log("app", app)
					rw := httptest.NewRecorder()
					r := httptest.NewRequest("GET", "/app", nil)
					r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
					r.RemoteAddr = auditableIP
					r.Header.Set("User-Agent", auditableUA)

					// Try resolving the request without a token.
					token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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

					require.True(t, auditor.Contains(t, database.AuditLog{
						OrganizationID: workspace.OrganizationID,
						Action:         database.AuditActionOpen,
						ResourceType:   audit.ResourceType(appsBySlug[app]),
						ResourceID:     audit.ResourceID(appsBySlug[app]),
						ResourceTarget: audit.ResourceTarget(appsBySlug[app]),
						UserID:         me.ID,
						UserAgent:      sql.NullString{Valid: true, String: auditableUA},
						Ip:             audit.ParseIP(auditableIP),
						StatusCode:     int32(w.StatusCode), //nolint:gosec
					}), "audit log")
					require.Len(t, auditor.AuditLogs(), 1, "single audit log")

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
					r.RemoteAddr = auditableIP

					secondToken, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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
					require.Len(t, auditor.AuditLogs(), 1, "no new audit log, FromRequest returned the same token and is not audited")
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

			auditor := audit.NewMock()
			auditableIP := randomIPv6(t)

			t.Log("app", app)
			rw := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/app", nil)
			r.Header.Set(codersdk.SessionTokenHeader, secondUserClient.SessionToken())
			r.RemoteAddr = auditableIP

			token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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

			require.True(t, auditor.Contains(t, database.AuditLog{
				OrganizationID: workspace.OrganizationID,
				Action:         database.AuditActionOpen,
				ResourceType:   audit.ResourceType(appsBySlug[app]),
				ResourceID:     audit.ResourceID(appsBySlug[app]),
				ResourceTarget: audit.ResourceTarget(appsBySlug[app]),
				UserID:         secondUser.ID,
				Ip:             audit.ParseIP(auditableIP),
				StatusCode:     int32(w.StatusCode), //nolint:gosec
			}), "audit log")
			require.Len(t, auditor.AuditLogs(), 1, "single audit log")
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

			auditor := audit.NewMock()
			auditableIP := randomIPv6(t)

			t.Log("app", app)
			rw := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/app", nil)
			r.RemoteAddr = auditableIP
			token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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

				require.Len(t, auditor.AuditLogs(), 0, "no audit logs for unauthenticated requests")
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

				require.True(t, auditor.Contains(t, database.AuditLog{
					OrganizationID: workspace.OrganizationID,
					ResourceType:   audit.ResourceType(appsBySlug[app]),
					ResourceID:     audit.ResourceID(appsBySlug[app]),
					ResourceTarget: audit.ResourceTarget(appsBySlug[app]),
					UserID:         uuid.Nil, // Nil is not verified by Contains, see below.
					Ip:             audit.ParseIP(auditableIP),
					StatusCode:     int32(w.StatusCode), //nolint:gosec
				}), "audit log")
				require.Len(t, auditor.AuditLogs(), 1, "single audit log")
				require.Equal(t, uuid.Nil, auditor.AuditLogs()[0].UserID, "no user ID in audit log")
			}
			_ = w.Body.Close()
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod: "invalid",
		}).Normalize()
		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.RemoteAddr = auditableIP
		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.False(t, ok)
		require.Nil(t, token)
		require.Len(t, auditor.AuditLogs(), 0, "no audit logs for invalid requests")
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

				auditor := audit.NewMock()
				auditableIP := randomIPv6(t)

				rw := httptest.NewRecorder()
				r := httptest.NewRequest("GET", "/app", nil)
				r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
				r.RemoteAddr = auditableIP

				token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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
					require.True(t, auditor.Contains(t, database.AuditLog{
						OrganizationID: workspace.OrganizationID,
						ResourceType:   audit.ResourceType(appsBySlug[token.AppSlugOrPort]),
						ResourceID:     audit.ResourceID(appsBySlug[token.AppSlugOrPort]),
						ResourceTarget: audit.ResourceTarget(appsBySlug[token.AppSlugOrPort]),
						UserID:         me.ID,
						Ip:             audit.ParseIP(auditableIP),
						StatusCode:     int32(w.StatusCode), //nolint:gosec
					}), "audit log")
					require.Len(t, auditor.AuditLogs(), 1, "single audit log")
				} else {
					require.Nil(t, token)
					require.Len(t, auditor.AuditLogs(), 0, "no audit logs")
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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.AddCookie(&http.Cookie{
			Name:  codersdk.SignedAppTokenCookie,
			Value: badTokenStr,
		})
		r.RemoteAddr = auditableIP

		// Even though the token is invalid, we should still perform request
		// resolution without failure since we'll just ignore the bad token.
		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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

		require.True(t, auditor.Contains(t, database.AuditLog{
			OrganizationID: workspace.OrganizationID,
			ResourceType:   audit.ResourceType(appsBySlug[token.AppSlugOrPort]),
			ResourceID:     audit.ResourceID(appsBySlug[token.AppSlugOrPort]),
			ResourceTarget: audit.ResourceTarget(appsBySlug[token.AppSlugOrPort]),
			UserID:         me.ID,
			Ip:             audit.ParseIP(auditableIP),
			StatusCode:     int32(w.StatusCode), //nolint:gosec
		}), "audit log")
		require.Len(t, auditor.AuditLogs(), 1, "single audit log")
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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.RemoteAddr = auditableIP

		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.False(t, ok)
		require.Nil(t, token)

		w := rw.Result()
		_ = w.Body.Close()
		// TODO(mafredri): Verify this is the correct status code.
		require.Equal(t, http.StatusInternalServerError, w.StatusCode)
		require.Len(t, auditor.AuditLogs(), 0, "no audit logs for port path blocked requests")
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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.RemoteAddr = auditableIP

		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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

		w := rw.Result()
		_ = w.Body.Close()
		require.Equal(t, http.StatusOK, w.StatusCode)
		require.True(t, auditor.Contains(t, database.AuditLog{
			OrganizationID: workspace.OrganizationID,
			ResourceType:   audit.ResourceType(agent),
			ResourceID:     audit.ResourceID(agent),
			ResourceTarget: audit.ResourceTarget(agent),
			UserID:         me.ID,
			Ip:             audit.ParseIP(auditableIP),
			StatusCode:     int32(w.StatusCode), //nolint:gosec
		}), "audit log for agent, not app")
		require.Len(t, auditor.AuditLogs(), 1, "single audit log")
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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.RemoteAddr = auditableIP

		_, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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
		require.Equal(t, http.StatusNotFound, w.StatusCode)
		require.Len(t, auditor.AuditLogs(), 0, "no audit logs for invalid requests")
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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.RemoteAddr = auditableIP

		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.True(t, ok)
		require.Equal(t, req.AppSlugOrPort, token.AppSlugOrPort)
		w := rw.Result()
		_ = w.Body.Close()
		require.True(t, auditor.Contains(t, database.AuditLog{
			OrganizationID: workspace.OrganizationID,
			ResourceType:   audit.ResourceType(appsBySlug[token.AppSlugOrPort]),
			ResourceID:     audit.ResourceID(appsBySlug[token.AppSlugOrPort]),
			ResourceTarget: audit.ResourceTarget(appsBySlug[token.AppSlugOrPort]),
			UserID:         me.ID,
			Ip:             audit.ParseIP(auditableIP),
			StatusCode:     int32(w.StatusCode), //nolint:gosec
		}), "audit log")
		require.Len(t, auditor.AuditLogs(), 1, "single audit log")
	})

	t.Run("Terminal", func(t *testing.T) {
		t.Parallel()

		req := (workspaceapps.Request{
			AccessMethod:  workspaceapps.AccessMethodTerminal,
			BasePath:      "/app",
			AgentNameOrID: agentID.String(),
		}).Normalize()

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.RemoteAddr = auditableIP

		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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
		w := rw.Result()
		_ = w.Body.Close()
		require.True(t, auditor.Contains(t, database.AuditLog{
			OrganizationID: workspace.OrganizationID,
			ResourceType:   audit.ResourceType(agent),
			ResourceID:     audit.ResourceID(agent),
			ResourceTarget: audit.ResourceTarget(agent),
			UserID:         me.ID,
			Ip:             audit.ParseIP(auditableIP),
			StatusCode:     int32(w.StatusCode), //nolint:gosec
		}), "audit log for agent, not app")
		require.Len(t, auditor.AuditLogs(), 1, "single audit log")
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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, secondUserClient.SessionToken())
		r.RemoteAddr = auditableIP

		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.False(t, ok)
		require.Nil(t, token)
		w := rw.Result()
		_ = w.Body.Close()
		require.Equal(t, http.StatusNotFound, w.StatusCode)
		require.True(t, auditor.Contains(t, database.AuditLog{
			OrganizationID: workspace.OrganizationID,
			UserID:         secondUser.ID,
			Ip:             audit.ParseIP(auditableIP),
			StatusCode:     int32(w.StatusCode), //nolint:gosec
		}), "audit log insufficient permissions")
		require.Len(t, auditor.AuditLogs(), 1, "single audit log")
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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.RemoteAddr = auditableIP

		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.False(t, ok)
		require.Nil(t, token)
		require.Len(t, auditor.AuditLogs(), 0, "no audit logs for user not found")
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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/some-path", nil)
		// Should not be used as the hostname in the redirect URI.
		r.Host = "app.com"
		r.RemoteAddr = auditableIP

		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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
		require.Len(t, auditor.AuditLogs(), 0, "no audit logs for redirect requests")

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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.RemoteAddr = auditableIP

		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
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
		require.True(t, auditor.Contains(t, database.AuditLog{
			OrganizationID: workspace.OrganizationID,
			UserID:         me.ID,
			Ip:             audit.ParseIP(auditableIP),
			StatusCode:     int32(w.StatusCode), //nolint:gosec
		}), "audit log unhealthy agent")
		require.Len(t, auditor.AuditLogs(), 1, "single audit log")

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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.RemoteAddr = auditableIP

		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.True(t, ok, "ResolveRequest failed, should pass even though app is initializing")
		require.NotNil(t, token)
		w := rw.Result()
		_ = w.Body.Close()
		require.True(t, auditor.Contains(t, database.AuditLog{
			OrganizationID: workspace.OrganizationID,
			UserID:         me.ID,
			Ip:             audit.ParseIP(auditableIP),
			StatusCode:     int32(w.StatusCode), //nolint:gosec
		}), "audit log initializing app")
		require.Len(t, auditor.AuditLogs(), 1, "single audit log")
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

		auditor := audit.NewMock()
		auditableIP := randomIPv6(t)

		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/app", nil)
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		r.RemoteAddr = auditableIP

		token, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
			Logger:              api.Logger,
			SignedTokenProvider: api.WorkspaceAppsProvider,
			DashboardURL:        api.AccessURL,
			PathAppBaseURL:      api.AccessURL,
			AppHostname:         api.AppHostname,
			AppRequest:          req,
		})
		require.True(t, ok, "ResolveRequest failed, should pass even though app is unhealthy")
		require.NotNil(t, token)
		w := rw.Result()
		_ = w.Body.Close()
		require.True(t, auditor.Contains(t, database.AuditLog{
			OrganizationID: workspace.OrganizationID,
			UserID:         me.ID,
			Ip:             audit.ParseIP(auditableIP),
			StatusCode:     int32(w.StatusCode), //nolint:gosec
		}), "audit log unhealthy app")
		require.Len(t, auditor.AuditLogs(), 1, "single audit log")
	})

	t.Run("AuditLogging", func(t *testing.T) {
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

			auditor := audit.NewMock()
			auditableIP := randomIPv6(t)

			t.Log("app", app)

			// First request, new audit log.
			rw := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/app", nil)
			r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
			r.RemoteAddr = auditableIP

			_, ok := workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
				Logger:              api.Logger,
				SignedTokenProvider: api.WorkspaceAppsProvider,
				DashboardURL:        api.AccessURL,
				PathAppBaseURL:      api.AccessURL,
				AppHostname:         api.AppHostname,
				AppRequest:          req,
			})
			require.True(t, ok)
			w := rw.Result()
			_ = w.Body.Close()
			require.True(t, auditor.Contains(t, database.AuditLog{
				OrganizationID: workspace.OrganizationID,
				Action:         database.AuditActionOpen,
				ResourceType:   audit.ResourceType(appsBySlug[app]),
				ResourceID:     audit.ResourceID(appsBySlug[app]),
				ResourceTarget: audit.ResourceTarget(appsBySlug[app]),
				UserID:         me.ID,
				Ip:             audit.ParseIP(auditableIP),
				StatusCode:     int32(w.StatusCode), //nolint:gosec
			}), "audit log 1")
			require.Len(t, auditor.AuditLogs(), 1, "single audit log")

			// Second request, no audit log because the session is active.
			rw = httptest.NewRecorder()
			r = httptest.NewRequest("GET", "/app", nil)
			r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
			r.RemoteAddr = auditableIP

			_, ok = workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
				Logger:              api.Logger,
				SignedTokenProvider: api.WorkspaceAppsProvider,
				DashboardURL:        api.AccessURL,
				PathAppBaseURL:      api.AccessURL,
				AppHostname:         api.AppHostname,
				AppRequest:          req,
			})
			require.True(t, ok)
			w = rw.Result()
			_ = w.Body.Close()
			require.Len(t, auditor.AuditLogs(), 1, "single audit log, previous session active")

			// Third request, session timed out, new audit log.
			rw = httptest.NewRecorder()
			r = httptest.NewRequest("GET", "/app", nil)
			r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
			r.RemoteAddr = auditableIP

			sessionTimeoutTokenProvider := signedTokenProviderWithAuditor(t, api.WorkspaceAppsProvider, auditor, 0)
			_, ok = workspaceappsResolveRequest(t, nil, rw, r, workspaceapps.ResolveRequestOptions{
				Logger:              api.Logger,
				SignedTokenProvider: sessionTimeoutTokenProvider,
				DashboardURL:        api.AccessURL,
				PathAppBaseURL:      api.AccessURL,
				AppHostname:         api.AppHostname,
				AppRequest:          req,
			})
			require.True(t, ok)
			w = rw.Result()
			_ = w.Body.Close()
			require.True(t, auditor.Contains(t, database.AuditLog{
				OrganizationID: workspace.OrganizationID,
				Action:         database.AuditActionOpen,
				ResourceType:   audit.ResourceType(appsBySlug[app]),
				ResourceID:     audit.ResourceID(appsBySlug[app]),
				ResourceTarget: audit.ResourceTarget(appsBySlug[app]),
				UserID:         me.ID,
				Ip:             audit.ParseIP(auditableIP),
				StatusCode:     int32(w.StatusCode), //nolint:gosec
			}), "audit log 2")
			require.Len(t, auditor.AuditLogs(), 2, "two audit logs, session timed out")

			// Fourth request, new IP produces new audit log.
			auditableIP = randomIPv6(t)
			rw = httptest.NewRecorder()
			r = httptest.NewRequest("GET", "/app", nil)
			r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
			r.RemoteAddr = auditableIP

			_, ok = workspaceappsResolveRequest(t, auditor, rw, r, workspaceapps.ResolveRequestOptions{
				Logger:              api.Logger,
				SignedTokenProvider: api.WorkspaceAppsProvider,
				DashboardURL:        api.AccessURL,
				PathAppBaseURL:      api.AccessURL,
				AppHostname:         api.AppHostname,
				AppRequest:          req,
			})
			require.True(t, ok)
			w = rw.Result()
			_ = w.Body.Close()
			require.True(t, auditor.Contains(t, database.AuditLog{
				OrganizationID: workspace.OrganizationID,
				Action:         database.AuditActionOpen,
				ResourceType:   audit.ResourceType(appsBySlug[app]),
				ResourceID:     audit.ResourceID(appsBySlug[app]),
				ResourceTarget: audit.ResourceTarget(appsBySlug[app]),
				UserID:         me.ID,
				Ip:             audit.ParseIP(auditableIP),
				StatusCode:     int32(w.StatusCode), //nolint:gosec
			}), "audit log 3")
			require.Len(t, auditor.AuditLogs(), 3, "three audit logs, new IP")
		}
	})
}

func randomIPv6(t testing.TB) string {
	t.Helper()

	// 2001:db8::/32 is reserved for documentation and examples.
	buf := make([]byte, 16)
	_, err := rand.Read(buf)
	require.NoError(t, err, "error generating random IPv6 address")
	return fmt.Sprintf("2001:db8:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x",
		buf[0], buf[1], buf[2], buf[3], buf[4], buf[5],
		buf[6], buf[7], buf[8], buf[9], buf[10], buf[11])
}

func workspaceappsResolveRequest(t testing.TB, auditor audit.Auditor, w http.ResponseWriter, r *http.Request, opts workspaceapps.ResolveRequestOptions) (token *workspaceapps.SignedToken, ok bool) {
	t.Helper()
	if opts.SignedTokenProvider != nil && auditor != nil {
		opts.SignedTokenProvider = signedTokenProviderWithAuditor(t, opts.SignedTokenProvider, auditor, time.Hour)
	}

	tracing.StatusWriterMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok = workspaceapps.ResolveRequest(w, r, opts)
	})).ServeHTTP(w, r)

	return token, ok
}

func signedTokenProviderWithAuditor(t testing.TB, provider workspaceapps.SignedTokenProvider, auditor audit.Auditor, sessionTimeout time.Duration) workspaceapps.SignedTokenProvider {
	t.Helper()
	p, ok := provider.(*workspaceapps.DBTokenProvider)
	require.True(t, ok, "provider is not a DBTokenProvider")

	shallowCopy := *p
	shallowCopy.Auditor = &atomic.Pointer[audit.Auditor]{}
	shallowCopy.Auditor.Store(&auditor)
	shallowCopy.WorkspaceAppAuditSessionTimeout = sessionTimeout
	return &shallowCopy
}
