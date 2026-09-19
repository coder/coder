package coderd_test

import (
	"cmp"
	"context"
	"database/sql"
	"net/http"
	"slices"
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
		ExitNodeIDs: []uuid.UUID{boundNode.ID},
	})
	require.NoError(t, err)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Len(t, build.Resources, 1)
	require.Len(t, build.Resources[0].Agents, 1)
	agent := build.Resources[0].Agents[0]

	// exitNodeRequest issues a request authenticated with an exit node
	// token instead of a session token and returns the status code.
	exitNodeRequest := func(ctx context.Context, token, method, path string, body any) (*http.Response, error) {
		nodeClient := codersdk.New(client.URL)
		return nodeClient.Request(ctx, method, path, body, func(r *http.Request) {
			r.Header.Set(codersdk.ExitNodeTokenHeader, token)
		})
	}
	// register registers with the given token and returns the status code.
	register := func(t *testing.T, token string, req codersdk.RegisterExitNodeRequest) int {
		t.Helper()
		res, err := exitNodeRequest(testutil.Context(t, testutil.WaitLong), token, http.MethodPost, "/api/v2/exitnodes/me/register", req)
		require.NoError(t, err)
		_ = res.Body.Close()
		return res.StatusCode
	}
	// requireStatus asserts that err is an API error with the given status.
	requireStatus := func(t *testing.T, err error, status int) {
		t.Helper()
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, status, sdkErr.StatusCode())
	}

	t.Run("CRUD", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		contains := func(nodes []codersdk.ExitNode, id uuid.UUID) bool {
			return slices.ContainsFunc(nodes, func(n codersdk.ExitNode) bool { return n.ID == id })
		}

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
		requireStatus(t, err, http.StatusConflict)
		// Invalid names are rejected.
		_, err = client.CreateExitNode(ctx, orgID, codersdk.CreateExitNodeRequest{Name: "not valid!"})
		requireStatus(t, err, http.StatusBadRequest)

		nodes, err := client.ExitNodes(ctx, orgID)
		require.NoError(t, err)
		require.True(t, contains(nodes, created.ID))
		byName, err := client.ExitNodeByName(ctx, orgID, name)
		require.NoError(t, err)
		require.Equal(t, created.ID, byName.ID)
		byID, err := client.ExitNodeByName(ctx, orgID, created.ID.String())
		require.NoError(t, err)
		require.Equal(t, created.ID, byID.ID)

		// The token authenticates the new node.
		require.Equal(t, http.StatusCreated, register(t, created.Token, codersdk.RegisterExitNodeRequest{Version: "v0.0.0-test"}))

		require.NoError(t, client.DeleteExitNode(ctx, orgID, name))
		require.True(t, auditor.Contains(t, database.AuditLog{
			Action:     database.AuditActionDelete,
			ResourceID: created.ID,
		}))
		_, err = client.ExitNodeByName(ctx, orgID, name)
		requireStatus(t, err, http.StatusNotFound)
		nodes, err = client.ExitNodes(ctx, orgID)
		require.NoError(t, err)
		require.False(t, contains(nodes, created.ID))

		// A deleted node's token no longer authenticates.
		require.Equal(t, http.StatusUnauthorized, register(t, created.Token, codersdk.RegisterExitNodeRequest{}))
	})

	t.Run("MemberForbidden", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		member, _ := coderdtest.CreateAnotherUser(t, client, orgID)

		_, err := member.CreateExitNode(ctx, orgID, codersdk.CreateExitNodeRequest{Name: testutil.GetRandomNameHyphenated(t)})
		requireStatus(t, err, http.StatusNotFound)
		nodes, err := member.ExitNodes(ctx, orgID)
		require.NoError(t, err)
		require.Empty(t, nodes)
		_, err = member.ExitNodeByName(ctx, orgID, boundNode.Name)
		requireStatus(t, err, http.StatusNotFound)
		requireStatus(t, member.DeleteExitNode(ctx, orgID, boundNode.Name), http.StatusNotFound)
	})

	t.Run("TemplateAdminReadOnly", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, orgID, rbac.RoleTemplateAdmin())

		got, err := templateAdmin.ExitNodeByName(ctx, orgID, boundNode.Name)
		require.NoError(t, err)
		require.Equal(t, boundNode.ID, got.ID)
		_, err = templateAdmin.CreateExitNode(ctx, orgID, codersdk.CreateExitNodeRequest{Name: testutil.GetRandomNameHyphenated(t)})
		requireStatus(t, err, http.StatusNotFound)
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
		for _, token := range []string{
			"",
			"garbage",
			boundNode.ID.String() + ":wrong",
			uuid.NewString() + ":" + boundSecret,
		} {
			require.Equal(t, http.StatusUnauthorized, register(t, token, codersdk.RegisterExitNodeRequest{}), "token %q", token)
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
		udpID := uuid.New()
		dnsID := uuid.New()
		unboundID := uuid.New()

		// report sends flows for the shared agent at connectTime unless a
		// flow overrides them.
		report := func(token string, droppedReports int, flows ...codersdk.ExitNodeFlowReport) {
			for i := range flows {
				flows[i].AgentID = cmp.Or(flows[i].AgentID, agent.ID)
				if flows[i].ConnectTime.IsZero() {
					flows[i].ConnectTime = connectTime
				}
			}
			res, err := exitNodeRequest(ctx, token, http.MethodPost, "/api/v2/exitnodes/me/flows", codersdk.ReportExitNodeFlowsRequest{
				Flows:          flows,
				DroppedReports: droppedReports,
			})
			require.NoError(t, err)
			defer res.Body.Close()
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("report flows: %v", codersdk.ReadBodyAsError(res))
			}
		}
		allowed := codersdk.ExitNodeFlowReport{
			FlowID:          allowedID,
			DestinationIP:   "93.184.216.34",
			DestinationPort: 443,
			Host:            "example.com",
			Decision:        codersdk.ExitNodeFlowAllow,
			RuleID:          "rule-1",
			Reason:          "allowed host",
		}

		// Connect report for an allowed flow, plus a denied flow.
		report(boundToken, 7,
			allowed,
			codersdk.ExitNodeFlowReport{
				FlowID:          deniedID,
				DestinationIP:   "198.51.100.7",
				DestinationPort: 22,
				Decision:        codersdk.ExitNodeFlowDeny,
				RuleID:          "rule-3",
				Reason:          "denied host",
			},
			// udp and dns flows carry a protocol prefix in the destination.
			codersdk.ExitNodeFlowReport{
				FlowID:          udpID,
				Protocol:        codersdk.ExitNodeProtocolUDP,
				DestinationIP:   "198.51.100.10",
				DestinationPort: 443,
				Host:            "quic.example",
				Decision:        codersdk.ExitNodeFlowDeny,
				RuleID:          "no-quic",
				Reason:          "matched rule no-quic",
			},
			codersdk.ExitNodeFlowReport{
				FlowID:          dnsID,
				Protocol:        codersdk.ExitNodeProtocolDNS,
				DestinationIP:   "10.0.0.53",
				DestinationPort: 53,
				Host:            "api.example.com",
				Decision:        codersdk.ExitNodeFlowAllow,
				Reason:          "no dns rule matched (TXT query)",
				BytesIn:         120,
				BytesOut:        45,
				DisconnectTime:  &disconnectTime,
			},
			// Unknown agents are skipped without failing the report.
			codersdk.ExitNodeFlowReport{
				FlowID:          uuid.New(),
				AgentID:         uuid.New(),
				DestinationIP:   "198.51.100.8",
				DestinationPort: 80,
				Decision:        codersdk.ExitNodeFlowAllow,
			},
		)
		// Disconnect report for the allowed flow.
		allowed.BytesIn, allowed.BytesOut, allowed.DisconnectTime = 1234, 567, &disconnectTime
		report(boundToken, 0, allowed)
		// A flow from a node the template is not bound to.
		report(otherToken, 0, codersdk.ExitNodeFlowReport{
			FlowID:          unboundID,
			DestinationIP:   "198.51.100.9",
			DestinationPort: 80,
			Decision:        codersdk.ExitNodeFlowAllow,
		})

		byFlow := make(map[uuid.UUID][]database.UpsertConnectionLogParams)
		var gapLogs []database.UpsertConnectionLogParams
		for _, clog := range connLogger.ConnectionLogs() {
			if clog.SlugOrPort.String == "exit-node-report-gap" {
				gapLogs = append(gapLogs, clog)
			}
			byFlow[clog.ID] = append(byFlow[clog.ID], clog)
		}
		require.Len(t, gapLogs, 1)
		assert.Equal(t, database.ConnectionTypeEgress, gapLogs[0].Type)
		assert.Equal(t, database.ConnectionStatusDisconnected, gapLogs[0].ConnectionStatus)
		assert.Equal(t, "dropped 7 exit node flow reports", gapLogs[0].DisconnectReason.String)
		require.Empty(t, byFlow[unboundID], "unbound exit node must not log flows")

		// Connect, then connect+disconnect from the second report.
		allowedLogs := byFlow[allowedID]
		require.Len(t, allowedLogs, 3)
		for _, clog := range allowedLogs {
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
		assert.Equal(t, database.ConnectionStatusConnected, allowedLogs[0].ConnectionStatus)
		assert.True(t, connectTime.Equal(allowedLogs[0].Time))
		assert.False(t, allowedLogs[0].DisconnectReason.Valid, "allowed connect leaves the reason for the disconnect")
		assert.Equal(t, database.ConnectionStatusDisconnected, allowedLogs[2].ConnectionStatus)
		assert.True(t, disconnectTime.Equal(allowedLogs[2].Time))
		assert.Equal(t, "rule-1: allowed host (in=1234 out=567)", allowedLogs[2].DisconnectReason.String)

		denied := byFlow[deniedID]
		require.Len(t, denied, 1)
		assert.Equal(t, database.ConnectionStatusConnected, denied[0].ConnectionStatus)
		assert.EqualValues(t, http.StatusForbidden, denied[0].Code.Int32)
		assert.Equal(t, "198.51.100.7:22", denied[0].SlugOrPort.String)
		assert.Equal(t, "rule-3: denied host", denied[0].DisconnectReason.String)

		udp := byFlow[udpID]
		require.Len(t, udp, 1)
		assert.EqualValues(t, http.StatusForbidden, udp[0].Code.Int32)
		assert.Equal(t, "udp quic.example:443", udp[0].SlugOrPort.String)
		assert.Equal(t, "198.51.100.10", udp[0].IP.IPNet.IP.String())
		assert.Equal(t, "no-quic: matched rule no-quic", udp[0].DisconnectReason.String)

		// A dns report carries its disconnect, so it yields both rows.
		dns := byFlow[dnsID]
		require.Len(t, dns, 2)
		assert.Equal(t, "dns api.example.com", dns[0].SlugOrPort.String)
		assert.Equal(t, "10.0.0.53", dns[0].IP.IPNet.IP.String())
		assert.EqualValues(t, 0, dns[0].Code.Int32)
		assert.Equal(t, database.ConnectionStatusDisconnected, dns[1].ConnectionStatus)
		assert.Equal(t, "no dns rule matched (TXT query) (in=120 out=45)", dns[1].DisconnectReason.String)
	})

	t.Run("EgressInfoDecode", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Rows written straight to the database in the flow encoding must
		// decode into typed egress info on the way out. dbgen writes every row
		// from 127.0.0.1.
		disconnectTime := time.Now().UTC().Truncate(time.Millisecond)
		for _, tt := range []struct {
			slugOrPort string
			code       int32
			reason     string
			want       codersdk.ConnectionLogEgressInfo
		}{
			{
				slugOrPort: "example.com:443",
				reason:     "rule-1: allowed host (in=1 out=2)",
				want: codersdk.ConnectionLogEgressInfo{
					Protocol:      codersdk.ExitNodeProtocolTCP,
					Destination:   "example.com:443",
					DestinationIP: "127.0.0.1",
					Decision:      codersdk.ExitNodeFlowAllow,
					RuleID:        "rule-1",
					Reason:        "allowed host (in=1 out=2)",
				},
			},
			{
				slugOrPort: "udp quic.example:443",
				code:       http.StatusForbidden,
				reason:     "no-quic: matched rule no-quic",
				want: codersdk.ConnectionLogEgressInfo{
					Protocol:      codersdk.ExitNodeProtocolUDP,
					Destination:   "quic.example:443",
					DestinationIP: "127.0.0.1",
					Decision:      codersdk.ExitNodeFlowDeny,
					RuleID:        "no-quic",
					Reason:        "matched rule no-quic",
				},
			},
			{
				slugOrPort: "dns api.example.com",
				reason:     "no dns rule matched (TXT query) (in=120 out=45)",
				want: codersdk.ConnectionLogEgressInfo{
					Protocol:      codersdk.ExitNodeProtocolDNS,
					Destination:   "api.example.com",
					DestinationIP: "127.0.0.1",
					Decision:      codersdk.ExitNodeFlowAllow,
					Reason:        "no dns rule matched (TXT query) (in=120 out=45)",
				},
			},
		} {
			connectionID := uuid.New()
			dbgen.ConnectionLog(t, db, database.UpsertConnectionLogParams{
				ID:               connectionID,
				OrganizationID:   orgID,
				WorkspaceOwnerID: workspace.OwnerID,
				WorkspaceID:      workspace.ID,
				WorkspaceName:    workspace.Name,
				AgentName:        agent.Name,
				Type:             database.ConnectionTypeEgress,
				Code:             sql.NullInt32{Int32: tt.code, Valid: true},
				SlugOrPort:       sql.NullString{String: tt.slugOrPort, Valid: true},
				ConnectionID:     uuid.NullUUID{UUID: connectionID, Valid: true},
				DisconnectReason: sql.NullString{String: tt.reason, Valid: true},
				Time:             disconnectTime,
				ConnectionStatus: database.ConnectionStatusDisconnected,
			})

			logs, err := client.ConnectionLogs(ctx, codersdk.ConnectionLogsRequest{
				SearchQuery: "connection_id:" + connectionID.String(),
			})
			require.NoError(t, err, tt.slugOrPort)
			require.Len(t, logs.ConnectionLogs, 1, tt.slugOrPort)
			clog := logs.ConnectionLogs[0]
			require.Equal(t, codersdk.ConnectionTypeEgress, clog.Type, tt.slugOrPort)
			require.NotNil(t, clog.EgressInfo, tt.slugOrPort)
			// DisconnectTime is not part of the encoding under test.
			clog.EgressInfo.DisconnectTime = nil
			assert.Equal(t, tt.want, *clog.EgressInfo, tt.slugOrPort)
		}
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
