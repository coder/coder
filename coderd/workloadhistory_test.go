package coderd_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkloadHistoryPublicRoutes(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentWorkloads)}
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
	admin := coderdtest.CreateFirstUser(t, client)
	member, memberUser := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	host := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        admin.UserID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].OperatingSystem = "linux"
		return agents
	}).Do()
	unregistered := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        memberUser.ID,
	}).WithAgent().Do()

	const supervisorToken = "workload-history-test-supervisor-token"
	const privateCommand = "printf 'private command history'"
	const privateEnv = "private-environment-value"
	const statsJSON = `{"total":7,"pending":1,"running":2,"succeeded":3,"failed":1,"canceled":0,"unknown":0}`
	workloadID := uuid.New()
	history := codersdk.WorkloadHostHistory{
		Items: []codersdk.WorkloadHistoryEntry{{
			Request: codersdk.StartHostWorkloadRequest{
				CreateWorkloadRequest: codersdk.CreateWorkloadRequest{
					ID: workloadID, TemplateID: host.Workspace.TemplateID, Command: privateCommand,
					Env: map[string]string{"TOKEN": privateEnv},
				},
				OrganizationID: admin.OrganizationID,
				OwnerID:        admin.UserID,
			},
			Workload: codersdk.Workload{
				ID: workloadID, OrganizationID: admin.OrganizationID,
				TemplateID: host.Workspace.TemplateID, OwnerID: admin.UserID,
			},
		}},
		Total: 1, NextOffset: 1,
	}
	agenttest.New(t, client.URL, host.AgentToken, func(o *agent.Options) {
		o.WorkloadHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !assert.Equal(t, "Bearer "+supervisorToken, r.Header.Get("Authorization")) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			assert.Equal(t, http.MethodGet, r.Method)
			switch r.URL.Path {
			case "/info":
				assert.NoError(t, json.NewEncoder(w).Encode(codersdk.WorkloadHostInfo{
					Generation: codersdk.WorkloadGeneration{ID: "test-generation", Workdir: "/work"},
					CPU:        4, MemoryBytes: 1 << 30, AvailableCPU: 4, AvailableMemoryBytes: 1 << 30,
				}))
			case "/stats":
				assert.Empty(t, r.URL.RawQuery, "caller-selected hosts and history queries must not reach the supervisor")
				// Extra supervisor fields must not expose command history to agents.
				assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"total": 7, "pending": 1, "running": 2, "succeeded": 3, "failed": 1,
					"items": history.Items,
				}))
			case "/history":
				assert.NoError(t, json.NewEncoder(w).Encode(history))
			default:
				t.Errorf("unexpected supervisor path: %s", r.URL.Path)
				http.NotFound(w, r)
			}
		})
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	coderdtest.NewWorkspaceAgentWaiter(t, client, host.Workspace.ID).WithContext(ctx).Wait()
	_, err := client.RegisterWorkloadHost(ctx, admin.OrganizationID, codersdk.RegisterWorkloadHostRequest{
		AgentID: host.Agents[0].ID, Token: supervisorToken,
	})
	require.NoError(t, err)

	newClient := func(token string) *codersdk.Client {
		c := codersdk.New(client.URL, codersdk.WithHTTPClient(client.HTTPClient))
		c.SetSessionToken(token)
		return c
	}
	anonymous := newClient("")
	hostClient := newClient(host.AgentToken)
	unregisteredClient := newClient(unregistered.AgentToken)
	agentStatsPath := "/api/v2/workspaceagents/me/workload-stats"
	hostPath := "/api/v2/organizations/" + admin.OrganizationID.String() + "/workload-hosts/" + host.Agents[0].ID.String()
	missingHostPath := "/api/v2/organizations/" + admin.OrganizationID.String() + "/workload-hosts/" + unregistered.Agents[0].ID.String()

	for _, tc := range []struct {
		name        string
		client      *codersdk.Client
		path        string
		wantStatus  int
		wantStats   bool
		wantHistory bool
	}{
		{name: "AgentAnonymous", client: anonymous, path: agentStatsPath, wantStatus: http.StatusUnauthorized},
		{name: "AgentRejectsAdminUserToken", client: client, path: agentStatsPath, wantStatus: http.StatusUnauthorized},
		{name: "AgentMissingRegistration", client: unregisteredClient, path: agentStatsPath, wantStatus: http.StatusNotFound},
		{name: "AgentCannotSelectAnotherHost", client: unregisteredClient, path: agentStatsPath + "?agent=" + host.Agents[0].ID.String() + "&agent_id=" + host.Agents[0].ID.String() + "&host=" + host.Agents[0].ID.String(), wantStatus: http.StatusNotFound},
		{name: "AgentOwnStats", client: hostClient, path: agentStatsPath, wantStatus: http.StatusOK, wantStats: true},
		{name: "AgentIgnoresHostAndHistoryQuery", client: hostClient, path: agentStatsPath + "?agent_id=" + unregistered.Agents[0].ID.String() + "&history=true&offset=1&limit=1", wantStatus: http.StatusOK, wantStats: true},
		{name: "AgentHasNoHistoryRoute", client: hostClient, path: "/api/v2/workspaceagents/me/workload-history", wantStatus: http.StatusNotFound},
		{name: "StatsAnonymous", client: anonymous, path: hostPath + "/stats", wantStatus: http.StatusUnauthorized},
		{name: "HistoryAnonymous", client: anonymous, path: hostPath + "/history", wantStatus: http.StatusUnauthorized},
		{name: "StatsRejectsAgentToken", client: hostClient, path: hostPath + "/stats", wantStatus: http.StatusUnauthorized},
		{name: "HistoryRejectsAgentToken", client: hostClient, path: hostPath + "/history", wantStatus: http.StatusUnauthorized},
		{name: "StatsMember", client: member, path: hostPath + "/stats", wantStatus: http.StatusNotFound},
		{name: "HistoryMember", client: member, path: hostPath + "/history", wantStatus: http.StatusNotFound},
		{name: "StatsAdmin", client: client, path: hostPath + "/stats", wantStatus: http.StatusOK, wantStats: true},
		{name: "HistoryAdmin", client: client, path: hostPath + "/history", wantStatus: http.StatusOK, wantHistory: true},
		{name: "StatsMissingRegistration", client: client, path: missingHostPath + "/stats", wantStatus: http.StatusNotFound},
		{name: "HistoryMissingRegistration", client: client, path: missingHostPath + "/history", wantStatus: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := tc.client.Request(testutil.Context(t, testutil.WaitLong), http.MethodGet, tc.path, nil)
			require.NoError(t, err)
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, res.StatusCode, string(body))
			require.NotContains(t, string(body), supervisorToken)
			if tc.wantStatus == http.StatusOK {
				require.Equal(t, "no-store", res.Header.Get("Cache-Control"))
			}
			if tc.wantStats {
				require.JSONEq(t, statsJSON, string(body))
			}
			if tc.wantHistory {
				var got codersdk.WorkloadHostHistory
				require.NoError(t, json.Unmarshal(body, &got))
				require.Len(t, got.Items, 1)
				require.Equal(t, privateCommand, got.Items[0].Request.Command)
				require.Equal(t, map[string]string{"TOKEN": privateEnv}, got.Items[0].Request.Env)
				require.Equal(t, host.Agents[0].ID, got.Items[0].Workload.AgentID)
			} else {
				require.NotContains(t, string(body), privateCommand)
				require.NotContains(t, string(body), privateEnv)
			}
		})
	}
}

func TestWorkloadHistoryPublicExperiment(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		experiments []string
	}{
		{name: "Disabled"},
		{name: "WildcardExcluded", experiments: []string{"*"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dv := coderdtest.DeploymentValues(t)
			dv.Experiments = tc.experiments
			client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
			admin := coderdtest.CreateFirstUser(t, client)
			workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: admin.OrganizationID,
				OwnerID:        admin.UserID,
			}).WithAgent().Do()
			agentClient := codersdk.New(client.URL, codersdk.WithHTTPClient(client.HTTPClient))
			agentClient.SetSessionToken(workspace.AgentToken)
			hostPath := "/api/v2/organizations/" + admin.OrganizationID.String() + "/workload-hosts/" + workspace.Agents[0].ID.String()
			for _, route := range []struct {
				client *codersdk.Client
				path   string
			}{
				{agentClient, "/api/v2/workspaceagents/me/workload-stats"},
				{client, hostPath + "/stats"},
				{client, hostPath + "/history"},
			} {
				t.Run(route.path, func(t *testing.T) {
					t.Parallel()
					res, err := route.client.Request(testutil.Context(t, testutil.WaitLong), http.MethodGet, route.path, nil)
					require.NoError(t, err)
					defer res.Body.Close()
					require.Equal(t, http.StatusForbidden, res.StatusCode)
				})
			}
		})
	}
}
