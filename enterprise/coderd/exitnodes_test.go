package coderd_test

import (
	"cmp"
	"context"
	"database/sql"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/types/key"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/connectionlog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/tailnet"
	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestExitNodesLicenseGate(t *testing.T) {
	t.Parallel()

	client, _, api, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{DontAddLicense: true})
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := client.CreateExitNode(ctx, owner.OrganizationID, codersdk.CreateExitNodeRequest{Name: "licensed-only"})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	require.Equal(t, "Exit Nodes is a Premium feature. Contact sales!", sdkErr.Message)

	res, err := client.Request(ctx, http.MethodPost, "/api/v2/exitnodes/me/register", codersdk.RegisterExitNodeRequest{})
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)

	api.Entitlements.Modify(func(entitlements *codersdk.Entitlements) {
		entitlements.Features[codersdk.FeatureExitNodes] = codersdk.Feature{
			Entitlement: codersdk.EntitlementEntitled,
			Enabled:     true,
		}
	})
	created, err := client.CreateExitNode(ctx, owner.OrganizationID, codersdk.CreateExitNodeRequest{Name: "licensed"})
	require.NoError(t, err)
	require.NotEmpty(t, created.Token)
}

func TestExitNodes(t *testing.T) {
	t.Parallel()

	auditor := audit.NewMock()
	connLogger := connectionlog.NewFake()
	ps := pubsub.NewInMemory()
	client, _, enterpriseAPI, owner := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		AuditLogging:      true,
		ConnectionLogging: true,
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			Auditor:                  auditor,
			ConnectionLogger:         connLogger,
			Pubsub:                   ps,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAuditLog:      1,
				codersdk.FeatureConnectionLog: 1,
				codersdk.FeatureExitNodes:     1,
			},
		},
	})
	db := enterpriseAPI.Database
	orgID := owner.OrganizationID

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
		ExitNodeIDs: &[]uuid.UUID{boundNode.ID},
	})
	require.NoError(t, err)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	build := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	require.Len(t, build.Resources, 1)
	require.Len(t, build.Resources[0].Agents, 1)
	agent := build.Resources[0].Agents[0]

	request := func(t *testing.T, token, method, path string, body any) *http.Response {
		t.Helper()
		nodeClient := codersdk.New(client.URL)
		res, err := nodeClient.Request(testutil.Context(t, testutil.WaitLong), method, path, body, func(r *http.Request) {
			r.Header.Set(codersdk.ExitNodeTokenHeader, token)
		})
		require.NoError(t, err)
		return res
	}
	register := func(t *testing.T, token string, req codersdk.RegisterExitNodeRequest, response ...*codersdk.RegisterExitNodeResponse) int {
		t.Helper()
		res := request(t, token, http.MethodPost, "/api/v2/exitnodes/me/register", req)
		defer res.Body.Close()
		if len(response) > 0 && res.StatusCode == http.StatusCreated {
			require.NoError(t, codersdk.ReadBodyAsJSON(res, response[0]))
		}
		return res.StatusCode
	}
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
		require.Equal(t, codersdk.ExitNodeStatusUnregistered, created.Status)
		require.Empty(t, created.Replicas)
		require.True(t, auditor.Contains(t, database.AuditLog{
			Action:     database.AuditActionCreate,
			ResourceID: created.ID,
		}))

		_, err = client.CreateExitNode(ctx, orgID, codersdk.CreateExitNodeRequest{Name: name})
		requireStatus(t, err, http.StatusConflict)
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

		replicaID := uuid.New()
		require.Equal(t, http.StatusCreated, register(t, created.Token, codersdk.RegisterExitNodeRequest{
			ReplicaID: replicaID,
			Version:   "v0.0.0-test",
		}))
		_, err = client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			ExitNodeIDs: &[]uuid.UUID{boundNode.ID, created.ID},
		})
		require.NoError(t, err)
		events := make(chan string, 1)
		unsubscribe, err := ps.Subscribe(codersdk.ExitNodeReplicasPubsubChannel, func(_ context.Context, message []byte) {
			if string(message) == created.ID.String() {
				events <- string(message)
			}
		})
		require.NoError(t, err)
		defer unsubscribe()

		require.NoError(t, client.DeleteExitNode(ctx, orgID, name))
		require.Equal(t, created.ID.String(), testutil.TryReceive(ctx, t, events))
		replica, err := db.GetExitNodeReplicaByID(dbauthz.AsSystemRestricted(ctx), replicaID)
		require.NoError(t, err)
		require.True(t, replica.StoppedAt.Valid)
		boundNodes, err := db.GetTemplateExitNodes(dbauthz.AsSystemRestricted(ctx), template.ID)
		require.NoError(t, err)
		require.NotContains(t, slice.Convert(boundNodes, func(node database.ExitNode) uuid.UUID { return node.ID }), created.ID)
		require.True(t, auditor.Contains(t, database.AuditLog{
			Action:     database.AuditActionDelete,
			ResourceID: created.ID,
		}))
		_, err = client.ExitNodeByName(ctx, orgID, name)
		requireStatus(t, err, http.StatusNotFound)
		nodes, err = client.ExitNodes(ctx, orgID)
		require.NoError(t, err)
		require.False(t, contains(nodes, created.ID))

		require.Equal(t, http.StatusUnauthorized, register(t, created.Token, codersdk.RegisterExitNodeRequest{}))
	})

	t.Run("RBAC", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			role rbac.RoleIdentifier
			read bool
		}{
			{name: "Member"},
			{name: "TemplateAdmin", role: rbac.RoleTemplateAdmin(), read: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)
				var roles []rbac.RoleIdentifier
				if tc.role.Name != "" {
					roles = append(roles, tc.role)
				}
				user, _ := coderdtest.CreateAnotherUser(t, client, orgID, roles...)
				_, err := user.CreateExitNode(ctx, orgID, codersdk.CreateExitNodeRequest{Name: testutil.GetRandomNameHyphenated(t)})
				requireStatus(t, err, http.StatusNotFound)
				_, err = user.ExitNodeByName(ctx, orgID, boundNode.Name)
				if tc.read {
					require.NoError(t, err)
					return
				}
				requireStatus(t, err, http.StatusNotFound)
				nodes, err := user.ExitNodes(ctx, orgID)
				require.NoError(t, err)
				require.Empty(t, nodes)
				requireStatus(t, user.DeleteExitNode(ctx, orgID, boundNode.Name), http.StatusNotFound)
			})
		}
	})

	t.Run("Replicas", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		node, secret := dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: orgID})
		token := node.ID.String() + ":" + secret
		otherNode, otherSecret := dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: orgID})
		otherToken := otherNode.ID.String() + ":" + otherSecret

		events := make(chan string, 4)
		unsubscribe, err := ps.Subscribe(codersdk.ExitNodeReplicasPubsubChannel, func(_ context.Context, message []byte) {
			if string(message) == node.ID.String() {
				events <- string(message)
			}
		})
		require.NoError(t, err)
		defer unsubscribe()

		firstID := uuid.New()
		first := codersdk.RegisterExitNodeResponse{}
		require.Equal(t, http.StatusCreated, register(t, token, codersdk.RegisterExitNodeRequest{
			ReplicaID:  firstID,
			Hostname:   "first",
			Version:    "v1",
			PolicyHash: "policy-a",
		}, &first))
		require.Empty(t, first.SiblingReplicas)
		select {
		case event := <-events:
			require.Equal(t, node.ID.String(), event)
		case <-ctx.Done():
			t.Fatal("timed out waiting for new replica pubsub event")
		}

		secondID := uuid.New()
		second := codersdk.RegisterExitNodeResponse{}
		require.Equal(t, http.StatusCreated, register(t, token, codersdk.RegisterExitNodeRequest{
			ReplicaID:  secondID,
			Hostname:   "second",
			Version:    "v2",
			PolicyHash: "policy-b",
		}, &second))
		require.Len(t, second.SiblingReplicas, 1)
		require.Equal(t, firstID, second.SiblingReplicas[0].ID)
		select {
		case event := <-events:
			require.Equal(t, node.ID.String(), event)
		case <-ctx.Done():
			t.Fatal("timed out waiting for second replica pubsub event")
		}

		got, err := client.ExitNodeByName(ctx, orgID, node.ID.String())
		require.NoError(t, err)
		require.Equal(t, codersdk.ExitNodeStatusHealthy, got.Status)
		require.True(t, got.PolicyMismatch)
		require.Len(t, got.Replicas, 2)
		require.Equal(t, []uuid.UUID{firstID, secondID}, []uuid.UUID{got.Replicas[0].ID, got.Replicas[1].ID})
		require.Equal(t, tailnet.TailscaleServicePrefix.AddrFromUUID(codersdk.ExitNodeReplicaPeerID(node.ID, firstID)).String(), got.Replicas[0].TailnetAddress)

		for _, id := range []uuid.UUID{agent.ID, func() uuid.UUID {
			proxy, _ := dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
			return proxy.ID
		}()} {
			require.Equal(t, http.StatusBadRequest, register(t, token, codersdk.RegisterExitNodeRequest{ReplicaID: id}))
		}

		require.Equal(t, http.StatusBadRequest, register(t, token, codersdk.RegisterExitNodeRequest{}))
		require.Equal(t, http.StatusBadRequest, register(t, otherToken, codersdk.RegisterExitNodeRequest{ReplicaID: firstID}))

		deregister := func(replicaID uuid.UUID) int {
			res := request(t, token, http.MethodPost, "/api/v2/exitnodes/me/deregister", codersdk.DeregisterExitNodeRequest{ReplicaID: replicaID})
			defer res.Body.Close()
			return res.StatusCode
		}
		require.Equal(t, http.StatusNotFound, func() int {
			res := request(t, otherToken, http.MethodPost, "/api/v2/exitnodes/me/deregister", codersdk.DeregisterExitNodeRequest{ReplicaID: firstID})
			defer res.Body.Close()
			return res.StatusCode
		}())
		require.Equal(t, http.StatusNoContent, deregister(firstID))
		select {
		case event := <-events:
			require.Equal(t, node.ID.String(), event)
		case <-ctx.Done():
			t.Fatal("timed out waiting for deregister pubsub event")
		}
		require.Equal(t, http.StatusBadRequest, register(t, token, codersdk.RegisterExitNodeRequest{ReplicaID: firstID}))

		got, err = client.ExitNodeByName(ctx, orgID, node.ID.String())
		require.NoError(t, err)
		require.Equal(t, codersdk.ExitNodeStatusHealthy, got.Status)
		require.False(t, got.PolicyMismatch)
		require.Equal(t, codersdk.ExitNodeReplicaStatusStopped, got.Replicas[0].Status)
		require.NotNil(t, got.Replicas[0].StoppedAt)

		require.Equal(t, http.StatusNoContent, deregister(secondID))
		got, err = client.ExitNodeByName(ctx, orgID, node.ID.String())
		require.NoError(t, err)
		require.Equal(t, codersdk.ExitNodeStatusUnreachable, got.Status)

		unregistered, err := client.ExitNodeByName(ctx, orgID, otherNode.ID.String())
		require.NoError(t, err)
		require.Equal(t, codersdk.ExitNodeStatusUnregistered, unregistered.Status)
		require.Empty(t, unregistered.Replicas)
	})

	t.Run("CoordinateValidation", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		node, secret := dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: orgID})
		token := node.ID.String() + ":" + secret
		request := func(query string) int {
			res := request(t, token, http.MethodGet, "/api/v2/exitnodes/me/coordinate?version=2.0"+query, nil)
			defer res.Body.Close()
			return res.StatusCode
		}
		require.Equal(t, http.StatusBadRequest, request(""))
		require.Equal(t, http.StatusBadRequest, request("&replica_id=invalid"))
		require.Equal(t, http.StatusNotFound, request("&replica_id="+uuid.NewString()))

		staleID := uuid.New()
		staleNow := dbtime.Now().Add(-time.Hour)
		_, err := db.UpsertExitNodeReplica(dbauthz.AsSystemRestricted(ctx), database.UpsertExitNodeReplicaParams{
			ID: staleID, ExitNodeID: node.ID, WireguardEndpoints: []string{}, Now: staleNow,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusConflict, request("&replica_id="+staleID.String()))

		stoppedID := uuid.New()
		stoppedNow := dbtime.Now()
		_, err = db.UpsertExitNodeReplica(dbauthz.AsSystemRestricted(ctx), database.UpsertExitNodeReplicaParams{
			ID: stoppedID, ExitNodeID: node.ID, WireguardEndpoints: []string{}, Now: stoppedNow,
		})
		require.NoError(t, err)
		require.NoError(t, db.StopExitNodeReplica(dbauthz.AsSystemRestricted(ctx), database.StopExitNodeReplicaParams{ID: stoppedID, StoppedAt: stoppedNow}))
		require.Equal(t, http.StatusConflict, request("&replica_id="+stoppedID.String()))
	})

	t.Run("Register", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		replicaID := uuid.New()
		res := request(t, boundToken, http.MethodPost, "/api/v2/exitnodes/me/register", codersdk.RegisterExitNodeRequest{
			ReplicaID:          replicaID,
			Version:            "v0.0.0-test",
			Hostname:           "exit-1",
			WireguardEndpoints: []string{"203.0.113.10:41641"},
		})
		defer res.Body.Close()
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("register: %v", codersdk.ReadBodyAsError(res))
		}
		var resp codersdk.RegisterExitNodeResponse
		require.NoError(t, codersdk.ReadBodyAsJSON(res, &resp))
		require.NotNil(t, resp.DERPMap)
		require.Contains(t, resp.AgentIDs, agent.ID)

		got, err := client.ExitNodeByName(ctx, orgID, boundNode.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.ExitNodeStatusHealthy, got.Status)
		replicaIndex := slices.IndexFunc(got.Replicas, func(replica codersdk.ExitNodeReplica) bool {
			return replica.ID == replicaID
		})
		require.NotEqual(t, -1, replicaIndex)
		replica := got.Replicas[replicaIndex]
		require.Equal(t, "v0.0.0-test", replica.Version)
		require.Equal(t, []string{"203.0.113.10:41641"}, replica.WireguardEndpoints)
		require.Equal(t, codersdk.ExitNodeReplicaStatusLive, replica.Status)
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

		otherNode, otherSecret := dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: orgID})
		otherToken := otherNode.ID.String() + ":" + otherSecret

		connectTime := time.Now().Add(-time.Minute).UTC().Truncate(time.Millisecond)
		disconnectTime := connectTime.Add(30 * time.Second)
		allowedID := uuid.New()
		deniedID := uuid.New()
		udpID := uuid.New()
		dnsID := uuid.New()
		unboundID := uuid.New()

		report := func(token string, droppedReports int, flows ...codersdk.ExitNodeFlowReport) {
			for i := range flows {
				flows[i].AgentID = cmp.Or(flows[i].AgentID, agent.ID)
				if flows[i].ConnectTime.IsZero() {
					flows[i].ConnectTime = connectTime
				}
			}
			res := request(t, token, http.MethodPost, "/api/v2/exitnodes/me/flows", codersdk.ReportExitNodeFlowsRequest{
				Flows:          flows,
				DroppedReports: droppedReports,
			})
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

		invalidReport := func(flow codersdk.ExitNodeFlowReport) int {
			res := request(t, boundToken, http.MethodPost, "/api/v2/exitnodes/me/flows", codersdk.ReportExitNodeFlowsRequest{Flows: []codersdk.ExitNodeFlowReport{flow}})
			defer res.Body.Close()
			return res.StatusCode
		}
		validFlow := codersdk.ExitNodeFlowReport{
			FlowID: uuid.New(), AgentID: agent.ID, DestinationPort: 443,
			Decision: codersdk.ExitNodeFlowAllow, ConnectTime: connectTime,
		}
		for _, mutate := range []func(*codersdk.ExitNodeFlowReport){
			func(flow *codersdk.ExitNodeFlowReport) { flow.AgentID = uuid.Nil },
			func(flow *codersdk.ExitNodeFlowReport) { flow.Protocol = "invalid" },
			func(flow *codersdk.ExitNodeFlowReport) { flow.Decision = "invalid" },
			func(flow *codersdk.ExitNodeFlowReport) { flow.DestinationPort = 0 },
			func(flow *codersdk.ExitNodeFlowReport) { flow.BytesIn = -1 },
			func(flow *codersdk.ExitNodeFlowReport) { flow.Reason = strings.Repeat("x", 513) },
			func(flow *codersdk.ExitNodeFlowReport) { flow.ConnectTime = time.Time{} },
			func(flow *codersdk.ExitNodeFlowReport) { flow.ConnectTime = time.Now().Add(6 * time.Minute) },
		} {
			flow := validFlow
			mutate(&flow)
			require.Equal(t, http.StatusBadRequest, invalidReport(flow))
		}

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
			codersdk.ExitNodeFlowReport{
				FlowID:          uuid.New(),
				AgentID:         uuid.New(),
				DestinationIP:   "198.51.100.8",
				DestinationPort: 80,
				Decision:        codersdk.ExitNodeFlowAllow,
			},
		)
		allowed.BytesIn, allowed.BytesOut, allowed.DisconnectTime = 1234, 567, &disconnectTime
		report(boundToken, 0, allowed)
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
			clog.EgressInfo.DisconnectTime = nil
			assert.Equal(t, tt.want, *clog.EgressInfo, tt.slugOrPort)
		}
	})

	t.Run("Coordinate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		node, secret := dbgen.ExitNode(t, db, database.ExitNode{OrganizationID: orgID})
		token := node.ID.String() + ":" + secret
		replicaID := uuid.New()
		require.Equal(t, http.StatusCreated, register(t, token, codersdk.RegisterExitNodeRequest{ReplicaID: replicaID}))
		u, err := client.URL.Parse("/api/v2/exitnodes/me/coordinate?version=2.0&" + codersdk.ExitNodeCoordinateReplicaIDParam + "=" + replicaID.String())
		require.NoError(t, err)
		//nolint:bodyclose // The websocket package closes the response body.
		wsConn, res, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
			HTTPHeader: http.Header{codersdk.ExitNodeTokenHeader: []string{token}},
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
		peerID := codersdk.ExitNodeReplicaPeerID(node.ID, replicaID)
		protoNode, err := tailnet.NodeToProto(&tailnet.Node{
			Key:         key.NewNode().Public(),
			DiscoKey:    key.NewDisco().Public(),
			Addresses:   []netip.Prefix{tailnet.TailscaleServicePrefix.PrefixFromUUID(peerID)},
			AllowedIPs:  []netip.Prefix{tailnet.TailscaleServicePrefix.PrefixFromUUID(peerID)},
			DERPLatency: map[string]float64{},
		})
		require.NoError(t, err)
		err = stream.Send(&tailnetproto.CoordinateRequest{
			UpdateSelf: &tailnetproto.CoordinateRequest_UpdateSelf{Node: protoNode},
		})
		require.NoError(t, err)
		testutil.Eventually(ctx, t, func(context.Context) bool {
			coordinator := enterpriseAPI.AGPL.TailnetCoordinator.Load()
			return (*coordinator).Node(peerID) != nil && (*coordinator).Node(replicaID) == nil
		}, testutil.IntervalFast)
		require.NoError(t, stream.Close())

		//nolint:bodyclose // The websocket package closes the response body.
		_, res, err = websocket.Dial(ctx, u.String(), &websocket.DialOptions{})
		require.Error(t, err)
		require.NotNil(t, res)
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})
}
