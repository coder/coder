package coderd_test

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/provisionersdk/proto"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

// updateGoldenFiles is a flag that can be set to update golden files.
var updateGoldenFiles = flag.Bool("update", false, "Update golden files")

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

func TestDERP(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	derpPort, err := strconv.Atoi(client.URL.Port())
	require.NoError(t, err)
	derpMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			1: {
				RegionID:   1,
				RegionCode: "cdr",
				RegionName: "Coder",
				Nodes: []*tailcfg.DERPNode{{
					Name:      "1a",
					RegionID:  1,
					HostName:  client.URL.Hostname(),
					DERPPort:  derpPort,
					STUNPort:  -1,
					ForceHTTP: true,
				}},
			},
		},
	}
	w1IP := tailnet.IP()
	w1, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(w1IP, 128)},
		Logger:    logger.Named("w1"),
		DERPMap:   derpMap,
	})
	require.NoError(t, err)

	w2, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		Logger:    logger.Named("w2"),
		DERPMap:   derpMap,
	})
	require.NoError(t, err)

	w2Ready := make(chan struct{})
	w2ReadyOnce := sync.Once{}
	w1.SetNodeCallback(func(node *tailnet.Node) {
		w2.UpdateNodes([]*tailnet.Node{node}, false)
		w2ReadyOnce.Do(func() {
			close(w2Ready)
		})
	})
	w2.SetNodeCallback(func(node *tailnet.Node) {
		w1.UpdateNodes([]*tailnet.Node{node}, false)
	})

	conn := make(chan struct{})
	go func() {
		listener, err := w1.Listen("tcp", ":35565")
		assert.NoError(t, err)
		defer listener.Close()
		conn <- struct{}{}
		nc, err := listener.Accept()
		assert.NoError(t, err)
		_ = nc.Close()
		conn <- struct{}{}
	}()

	<-conn
	<-w2Ready
	nc, err := w2.DialContextTCP(context.Background(), netip.AddrPortFrom(w1IP, 35565))
	require.NoError(t, err)
	_ = nc.Close()
	<-conn

	w1.Close()
	w2.Close()
}

func TestDERPForceWebSockets(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.DERP.Config.ForceWebSockets = true
	dv.DERP.Config.BlockDirect = true // to ensure the test always uses DERP

	// Manually create a server so we can influence the HTTP handler.
	options := &coderdtest.Options{
		DeploymentValues: dv,
	}
	setHandler, cancelFunc, serverURL, newOptions := coderdtest.NewOptions(t, options)
	coderAPI := coderd.New(newOptions)
	t.Cleanup(func() {
		cancelFunc()
		_ = coderAPI.Close()
	})

	// Set the HTTP handler to a custom one that ensures all /derp calls are
	// WebSockets and not `Upgrade: derp`.
	var upgradeCount int64
	setHandler(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/derp") {
			up := r.Header.Get("Upgrade")
			if up != "" && up != "websocket" {
				t.Errorf("expected Upgrade: websocket, got %q", up)
			} else {
				atomic.AddInt64(&upgradeCount, 1)
			}
		}

		coderAPI.RootHandler.ServeHTTP(rw, r)
	}))

	// Start a provisioner daemon.
	provisionerCloser := coderdtest.NewProvisionerDaemon(t, coderAPI)
	t.Cleanup(func() {
		_ = provisionerCloser.Close()
	})

	client := codersdk.New(serverURL)
	t.Cleanup(func() {
		client.HTTPClient.CloseIdleConnections()
	})
	user := coderdtest.CreateFirstUser(t, client)

	gen, err := client.WorkspaceAgentConnectionInfoGeneric(context.Background())
	require.NoError(t, err)
	t.Log(spew.Sdump(gen))

	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	_ = agenttest.New(t, client.URL, authToken)
	_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, nil)
	require.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	conn.AwaitReachable(ctx)

	require.GreaterOrEqual(t, atomic.LoadInt64(&upgradeCount), int64(1), "expected at least one /derp call")
}

func TestDERPLatencyCheck(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	res, err := client.Request(context.Background(), http.MethodGet, "/derp/latency-check", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}

func TestFastLatencyCheck(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	res, err := client.Request(context.Background(), http.MethodGet, "/latency-check", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}

func TestHealthz(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)

	res, err := client.Request(context.Background(), http.MethodGet, "/healthz", nil)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, "OK", string(body))
}

func TestSwagger(t *testing.T) {
	t.Parallel()

	const swaggerEndpoint = "/swagger"
	t.Run("endpoint enabled", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			SwaggerEndpoint: true,
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, swaggerEndpoint, nil)
		require.NoError(t, err)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Contains(t, string(body), "Swagger UI")
	})
	t.Run("doc.json exposed", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{
			SwaggerEndpoint: true,
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, swaggerEndpoint+"/doc.json", nil)
		require.NoError(t, err)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Contains(t, string(body), `"swagger": "2.0"`)
	})
	t.Run("endpoint disabled by default", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, swaggerEndpoint, nil)
		require.NoError(t, err)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, "<pre>\n</pre>\n", string(body))
	})
	t.Run("doc.json disabled by default", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		resp, err := requestWithRetries(ctx, t, client, http.MethodGet, swaggerEndpoint+"/doc.json", nil)
		require.NoError(t, err)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, "<pre>\n</pre>\n", string(body))
	})
}

func TestCSRFExempt(t *testing.T) {
	t.Parallel()

	// This test build a workspace with an agent and an app. The app is not
	// a real http server, so it will fail to serve requests. We just want
	// to make sure the failure is not a CSRF failure, as path based
	// apps should be exempt.
	t.Run("PathBasedApp", func(t *testing.T) {
		t.Parallel()

		client, _, api := coderdtest.NewWithAPI(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		owner, err := client.User(context.Background(), "me")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
		defer cancel()

		// Create a workspace.
		const agentSlug = "james"
		const appSlug = "web"
		wrk := dbfake.WorkspaceBuild(t, api.Database, database.Workspace{
			OwnerID:        owner.ID,
			OrganizationID: first.OrganizationID,
		}).
			WithAgent(func(agents []*proto.Agent) []*proto.Agent {
				agents[0].Name = agentSlug
				agents[0].Apps = []*proto.App{{
					Slug:        appSlug,
					DisplayName: appSlug,
					Subdomain:   false,
					Url:         "/",
				}}

				return agents
			}).
			Do()

		u := client.URL.JoinPath(fmt.Sprintf("/@%s/%s.%s/apps/%s", owner.Username, wrk.Workspace.Name, agentSlug, appSlug)).String()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
		req.AddCookie(&http.Cookie{
			Name:   codersdk.SessionTokenCookie,
			Value:  client.SessionToken(),
			Path:   "/",
			Domain: client.URL.String(),
		})
		require.NoError(t, err)

		resp, err := client.HTTPClient.Do(req)
		require.NoError(t, err)
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		// A StatusBadGateway means Coderd tried to proxy to the agent and failed because the agent
		// was not there. This means CSRF did not block the app request, which is what we want.
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "status code 500 is CSRF failure")
		require.NotContains(t, string(data), "CSRF")
	})
}
