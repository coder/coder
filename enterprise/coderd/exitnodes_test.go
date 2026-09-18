package coderd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestExitNodes(t *testing.T) {
	t.Parallel()

	auditor := audit.NewMock()
	connLogger := connectionlog.NewFake()
	client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		AuditLogging:      true,
		ConnectionLogging: true,
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			Auditor:                  auditor,
			ConnectionLogger:         connLogger,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAuditLog:      1,
				codersdk.FeatureConnectionLog: 1,
			},
		},
	})
	orgID := owner.OrganizationID

	// A running workspace whose template is bound to boundNode. Subtests
	// only read from it, so it is shared.
	boundNode, boundSecret := dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: orgID})
	boundToken := boundNode.ID.String() + ":" + boundSecret
	version := coderdtest.CreateTemplateVersion(t, client, orgID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(uuid.NewString()),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, orgID, version.ID)
	setupCtx := testutil.Context(t, testutil.WaitLong)
	_, err := client.UpdateTemplateMeta(setupCtx, template.ID, codersdk.UpdateTemplateMeta{
		ExitNodeID: &boundNode.ID,
	})
	require.NoError(t, err)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Len(t, build.Resources, 1)
	require.Len(t, build.Resources[0].Agents, 1)
	agent := build.Resources[0].Agents[0]

	// exitNodeRequest issues a request authenticated with an exit node
	// token instead of a session token.
	exitNodeRequest := func(ctx context.Context, token, method, path string, body any) (*http.Response, error) {
		nodeClient := codersdk.New(client.URL)
		return nodeClient.Request(ctx, method, path, body, func(r *http.Request) {
			r.Header.Set(codersdk.ExitNodeTokenHeader, token)
		})
	}

	t.Run("CRUD", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		name := testutil.GetRandomNameHyphenated(t)
		created, err := client.CreateExitNode(ctx, orgID, codersdk.CreateExitNodeRequest{
			Name:        name,
			DisplayName: "Exit " + name,
		})
		require.NoError(t, err)
		require.NotEmpty(t, created.Token)
		require.Equal(t, orgID, created.OrganizationID)
		require.Equal(t, name, created.Name)
		require.Equal(t, tailnet.TailscaleServicePrefix.AddrFromUUID(created.ID).String(), created.TailnetAddress)
		require.NotNil(t, created.WireguardEndpoints)
		require.True(t, auditor.Contains(t, database.AuditLog{
			Action:     database.AuditActionCreate,
			ResourceID: created.ID,
		}))

		// Duplicate names conflict, case-insensitively.
		_, err = client.CreateExitNode(ctx, orgID, codersdk.CreateExitNodeRequest{Name: name})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())

		// Invalid names are rejected.
		_, err = client.CreateExitNode(ctx, orgID, codersdk.CreateExitNodeRequest{Name: "not valid!"})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

		nodes, err := client.ExitNodes(ctx, orgID)
		require.NoError(t, err)
		require.Contains(t, mapIDs(nodes), created.ID)

		byName, err := client.ExitNodeByName(ctx, orgID, name)
		require.NoError(t, err)
		require.Equal(t, created.ID, byName.ID)
		byID, err := client.ExitNodeByName(ctx, orgID, created.ID.String())
		require.NoError(t, err)
		require.Equal(t, created.ID, byID.ID)

		// The token authenticates the new node.
		res, err := exitNodeRequest(ctx, created.Token, http.MethodPost, "/api/v2/exitnodes/me/register", codersdk.RegisterExitNodeRequest{
			Version: "v0.0.0-test",
		})
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusCreated, res.StatusCode)

		err = client.DeleteExitNode(ctx, orgID, name)
		require.NoError(t, err)
		require.True(t, auditor.Contains(t, database.AuditLog{
			Action:     database.AuditActionDelete,
			ResourceID: created.ID,
		}))

		_, err = client.ExitNodeByName(ctx, orgID, name)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
		nodes, err = client.ExitNodes(ctx, orgID)
		require.NoError(t, err)
		require.NotContains(t, mapIDs(nodes), created.ID)

		// A deleted node's token no longer authenticates.
		res, err = exitNodeRequest(ctx, created.Token, http.MethodPost, "/api/v2/exitnodes/me/register", codersdk.RegisterExitNodeRequest{})
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("MemberForbidden", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		member, _ := coderdtest.CreateAnotherUser(t, client, orgID)

		_, err := member.CreateExitNode(ctx, orgID, codersdk.CreateExitNodeRequest{Name: testutil.GetRandomNameHyphenated(t)})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		nodes, err := member.ExitNodes(ctx, orgID)
		require.NoError(t, err)
		require.Empty(t, nodes)

		_, err = member.ExitNodeByName(ctx, orgID, boundNode.Name)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		err = member.DeleteExitNode(ctx, orgID, boundNode.Name)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("TemplateAdminReadOnly", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, orgID, rbac.RoleTemplateAdmin())

		got, err := templateAdmin.ExitNodeByName(ctx, orgID, boundNode.Name)
		require.NoError(t, err)
		require.Equal(t, boundNode.ID, got.ID)

		_, err = templateAdmin.CreateExitNode(ctx, orgID, codersdk.CreateExitNodeRequest{Name: testutil.GetRandomNameHyphenated(t)})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("Register", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		res, err := exitNodeRequest(ctx, boundToken, http.MethodPost, "/api/v2/exitnodes/me/register", codersdk.RegisterExitNodeRequest{
			Version:            "v0.0.0-test",
			Hostname:           "exit-1",
			WireguardEndpoints: []string{"203.0.113.10:41641"},
		})
		require.NoError(t, err)
		defer res.Body.Close()
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("register: %v", codersdk.ReadBodyAsError(res))
		}

		var resp codersdk.RegisterExitNodeResponse
		require.NoError(t, codersdk.ReadBodyAsJSON(res, &resp))
		require.NotNil(t, resp.DERPMap)
		require.Contains(t, resp.AgentIDs, agent.ID)

		// Registration is reflected on the node.
		got, err := client.ExitNodeByName(ctx, orgID, boundNode.Name)
		require.NoError(t, err)
		require.Equal(t, "v0.0.0-test", got.Version)
		require.Equal(t, []string{"203.0.113.10:41641"}, got.WireguardEndpoints)
		require.NotNil(t, got.LastSeenAt)
	})

	t.Run("InvalidToken", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		for _, token := range []string{
			"",
			"garbage",
			boundNode.ID.String() + ":wrong",
			uuid.NewString() + ":" + boundSecret,
		} {
			res, err := exitNodeRequest(ctx, token, http.MethodPost, "/api/v2/exitnodes/me/register", codersdk.RegisterExitNodeRequest{})
			require.NoError(t, err)
			_ = res.Body.Close()
			require.Equal(t, http.StatusUnauthorized, res.StatusCode, "token %q", token)
		}
	})

	t.Run("Flows", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// An exit node the workspace's template is not bound to must not
		// be able to log flows for its agent.
		otherNode, otherSecret := dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: orgID})
		otherToken := otherNode.ID.String() + ":" + otherSecret

		connectTime := time.Now().Add(-time.Minute).UTC().Truncate(time.Millisecond)
		disconnectTime := connectTime.Add(30 * time.Second)
		allowedID := uuid.New()
		deniedID := uuid.New()
		unboundID := uuid.New()
		unknownAgentID := uuid.New()

		report := func(token string, flows ...codersdk.ExitNodeFlowReport) {
			res, err := exitNodeRequest(ctx, token, http.MethodPost, "/api/v2/exitnodes/me/flows", codersdk.ReportExitNodeFlowsRequest{Flows: flows})
			require.NoError(t, err)
			defer res.Body.Close()
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("report flows: %v", codersdk.ReadBodyAsError(res))
			}
		}

		// Connect report for an allowed flow, plus a denied flow.
		report(boundToken,
			codersdk.ExitNodeFlowReport{
				FlowID:          allowedID,
				AgentID:         agent.ID,
				DestinationIP:   "93.184.216.34",
				DestinationPort: 443,
				Host:            "example.com",
				Decision:        codersdk.ExitNodeFlowAllow,
				RuleID:          "rule-1",
				Reason:          "allowed host",
				ConnectTime:     connectTime,
			},
			codersdk.ExitNodeFlowReport{
				FlowID:          deniedID,
				AgentID:         agent.ID,
				DestinationIP:   "198.51.100.7",
				DestinationPort: 22,
				Decision:        codersdk.ExitNodeFlowDeny,
				RuleID:          "rule-3",
				Reason:          "denied host",
				ConnectTime:     connectTime,
			},
			// Unknown agents are skipped without failing the report.
			codersdk.ExitNodeFlowReport{
				FlowID:          uuid.New(),
				AgentID:         unknownAgentID,
				DestinationIP:   "198.51.100.8",
				DestinationPort: 80,
				Decision:        codersdk.ExitNodeFlowAllow,
				ConnectTime:     connectTime,
			},
		)
		// Disconnect report for the allowed flow.
		report(boundToken, codersdk.ExitNodeFlowReport{
			FlowID:          allowedID,
			AgentID:         agent.ID,
			DestinationIP:   "93.184.216.34",
			DestinationPort: 443,
			Host:            "example.com",
			Decision:        codersdk.ExitNodeFlowAllow,
			RuleID:          "rule-1",
			Reason:          "allowed host",
			BytesIn:         1234,
			BytesOut:        567,
			ConnectTime:     connectTime,
			DisconnectTime:  &disconnectTime,
		})
		// A flow from a node the template is not bound to.
		report(otherToken, codersdk.ExitNodeFlowReport{
			FlowID:          unboundID,
			AgentID:         agent.ID,
			DestinationIP:   "198.51.100.9",
			DestinationPort: 80,
			Decision:        codersdk.ExitNodeFlowAllow,
			ConnectTime:     connectTime,
		})

		var allowed, denied, unbound []database.UpsertConnectionLogParams
		for _, clog := range connLogger.ConnectionLogs() {
			switch clog.ID {
			case allowedID:
				allowed = append(allowed, clog)
			case deniedID:
				denied = append(denied, clog)
			case unboundID:
				unbound = append(unbound, clog)
			}
		}
		require.Empty(t, unbound, "unbound exit node must not log flows")

		// Connect, then connect+disconnect from the second report.
		require.Len(t, allowed, 3)
		for _, clog := range allowed {
			assert.Equal(t, database.ConnectionTypeEgress, clog.Type)
			assert.Equal(t, workspace.ID, clog.WorkspaceID)
			assert.Equal(t, workspace.OwnerID, clog.WorkspaceOwnerID)
			assert.Equal(t, orgID, clog.OrganizationID)
			assert.Equal(t, workspace.Name, clog.WorkspaceName)
			assert.Equal(t, agent.Name, clog.AgentName)
			assert.Equal(t, "example.com:443", clog.SlugOrPort.String)
			assert.Equal(t, "93.184.216.34", clog.IP.IPNet.IP.String())
			assert.Equal(t, allowedID, clog.ConnectionID.UUID)
			assert.True(t, clog.Code.Valid)
			assert.EqualValues(t, 0, clog.Code.Int32)
		}
		assert.Equal(t, database.ConnectionStatusConnected, allowed[0].ConnectionStatus)
		assert.True(t, connectTime.Equal(allowed[0].Time))
		assert.False(t, allowed[0].DisconnectReason.Valid, "allowed connect leaves the reason for the disconnect")
		assert.Equal(t, database.ConnectionStatusDisconnected, allowed[2].ConnectionStatus)
		assert.True(t, disconnectTime.Equal(allowed[2].Time))
		assert.Equal(t, "rule-1: allowed host (in=1234 out=567)", allowed[2].DisconnectReason.String)

		require.Len(t, denied, 1)
		assert.Equal(t, database.ConnectionStatusConnected, denied[0].ConnectionStatus)
		assert.EqualValues(t, http.StatusForbidden, denied[0].Code.Int32)
		assert.Equal(t, "198.51.100.7:22", denied[0].SlugOrPort.String)
		assert.Equal(t, "rule-3: denied host", denied[0].DisconnectReason.String)
	})

	t.Run("Coordinate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		u, err := client.URL.Parse("/api/v2/exitnodes/me/coordinate?version=2.0")
		require.NoError(t, err)
		//nolint:bodyclose // The websocket package closes the response body.
		wsConn, res, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
			HTTPHeader: http.Header{codersdk.ExitNodeTokenHeader: []string{boundToken}},
		})
		if err != nil && res != nil && res.StatusCode != http.StatusSwitchingProtocols {
			err = codersdk.ReadBodyAsError(res)
		}
		require.NoError(t, err)
		defer wsConn.Close(websocket.StatusNormalClosure, "done")

		rpcClient, err := tailnet.NewDRPCClient(websocket.NetConn(ctx, wsConn, websocket.MessageBinary), logger)
		require.NoError(t, err)
		stream, err := rpcClient.Coordinate(ctx)
		require.NoError(t, err)
		err = stream.Send(&tailnetproto.CoordinateRequest{
			UpdateSelf: &tailnetproto.CoordinateRequest_UpdateSelf{
				Node: &tailnetproto.Node{PreferredDerp: 1},
			},
		})
		require.NoError(t, err)
		require.NoError(t, stream.Close())

		// Without a token the upgrade is refused.
		//nolint:bodyclose // The websocket package closes the response body.
		_, res, err = websocket.Dial(ctx, u.String(), &websocket.DialOptions{})
		require.Error(t, err)
		require.NotNil(t, res)
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
}

func mapIDs(nodes []codersdk.ExitNode) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}
