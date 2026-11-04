package coderd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

func TestWorkloadHostReports(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	f.register(t)
	req := f.request()
	req.Command = "printf '%s\\n' 'private command'\nprintf done"
	f.reportHook = func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/api/v0/workloads/stats":
			_, _ = w.Write([]byte(`{"total":3,"running":1,"succeeded":2}`))
		case "/api/v0/workloads/history":
			assert.Equal(t, "offset=2&limit=1", r.URL.RawQuery)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total": 3, "next_offset": 3,
				"items": []any{map[string]any{
					"request":  codersdk.StartHostWorkloadRequest{CreateWorkloadRequest: req, OrganizationID: f.orgID, OwnerID: uuid.MustParse(f.actors["owner"].ID), GenerationID: f.info.Generation.ID},
					"workload": codersdk.Workload{ID: req.ID, OrganizationID: f.orgID, TemplateID: f.template.ID, OwnerID: uuid.MustParse(f.actors["owner"].ID)},
				}},
			})
		default:
			t.Errorf("unexpected report path %s", r.URL.Path)
		}
		return true
	}
	base := "/workload-hosts/" + f.agentID.String()
	stats := f.call(t, "admin", http.MethodGet, base+"/stats", nil)
	require.Equal(t, http.StatusOK, stats.Code, stats.Body.String())
	var counts codersdk.WorkloadHostStats
	require.NoError(t, json.Unmarshal(stats.Body.Bytes(), &counts))
	require.Equal(t, int64(3), counts.Total)
	require.Equal(t, int64(1), counts.Running)
	require.NotContains(t, stats.Body.String(), req.Command)
	require.Equal(t, "no-store", stats.Header().Get("Cache-Control"))
	history := f.call(t, "admin", http.MethodGet, base+"/history?offset=2&limit=1", nil)
	require.Equal(t, http.StatusOK, history.Code, history.Body.String())
	var result struct {
		Items []struct {
			Request codersdk.StartHostWorkloadRequest `json:"request"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(history.Body.Bytes(), &result))
	require.Len(t, result.Items, 1)
	require.Equal(t, req, result.Items[0].Request.CreateWorkloadRequest)
	require.NotContains(t, history.Body.String(), f.token)
	require.Equal(t, "no-store", history.Header().Get("Cache-Control"))

	requests := f.requests.Load()
	for _, actor := range []string{"owner", "other", "template-admin", "foreign-admin", "owner-restricted"} {
		for _, route := range []string{"/stats", "/history"} {
			denied := f.call(t, actor, http.MethodGet, base+route, nil)
			require.Equal(t, http.StatusNotFound, denied.Code, actor+route)
			require.NotContains(t, denied.Body.String(), "private command")
		}
	}
	for _, route := range []string{"/stats", "/history"} {
		require.Equal(t, http.StatusNotFound, f.callPath(t, "admin", http.MethodGet, "/organizations/"+uuid.NewString()+base+route, nil).Code)
		require.Equal(t, http.StatusNotFound, f.call(t, "admin", http.MethodGet, "/workload-hosts/"+uuid.NewString()+route, nil).Code)
	}
	for _, query := range []string{"offset=-1", "offset=abc", "limit=0", "limit=11", "limit=9999999999999999999999"} {
		require.Equal(t, http.StatusBadRequest, f.call(t, "admin", http.MethodGet, base+"/history?"+query, nil).Code, query)
	}
	require.Equal(t, requests, f.requests.Load(), "denied requests must not contact the supervisor")
}

func TestWorkloadHostReportRestrictedAdmin(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	f.register(t)
	f.actors["restricted-admin"] = rbac.Subject{
		ID: uuid.NewString(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAdmin(f.orgID)},
		Scope: rbac.Scope{Role: rbac.Role{
			Identifier: rbac.RoleIdentifier{Name: "organization-only"},
			Site:       rbac.Permissions(map[string][]policy.Action{rbac.ResourceOrganization.Type: {policy.ActionRead, policy.ActionUpdate}}),
		}, AllowIDList: []rbac.AllowListElement{rbac.AllowListAll()}},
	}
	requests := f.requests.Load()
	for _, path := range []string{"/stats", "/history"} {
		require.Equal(t, http.StatusNotFound, f.call(t, "restricted-admin", http.MethodGet, "/workload-hosts/"+f.agentID.String()+path, nil).Code)
	}
	require.Equal(t, requests, f.requests.Load())
}

func TestWorkloadAgentStatsIdentity(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	f.register(t)
	f.reportHook = func(w http.ResponseWriter, r *http.Request) bool {
		assert.Equal(t, "/api/v0/workloads/stats", r.URL.Path)
		_, _ = w.Write([]byte(`{"total":7,"running":2}`))
		return true
	}
	// The public route supplies these identities exclusively through agent-token
	// middleware. The helper accepts no caller-selected host or organization.
	host := f.api.workloadStatsHost(database.WorkspaceAgent{ID: f.agentID}, f.build)
	require.NotNil(t, host)
	w := httptest.NewRecorder()
	f.api.writeWorkloadHostStats(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), host)
	require.Equal(t, http.StatusOK, w.Code)
	var counts codersdk.WorkloadHostStats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &counts))
	require.Equal(t, int64(7), counts.Total)
	require.NotContains(t, w.Body.String(), f.token)
	require.Nil(t, f.api.workloadStatsHost(database.WorkspaceAgent{ID: uuid.New()}, f.build))
	build := f.build
	build.ID = uuid.New()
	require.Nil(t, f.api.workloadStatsHost(database.WorkspaceAgent{ID: f.agentID}, build))
}

func TestWorkloadHostReportFailures(t *testing.T) {
	t.Parallel()
	for _, invalidHistory := range []bool{false, true} {
		name := "Unavailable"
		if invalidHistory {
			name = "CrossOrganizationHistory"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newWorkloadTest(t)
			f.register(t)
			f.reportHook = func(w http.ResponseWriter, _ *http.Request) bool {
				if invalidHistory {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"total": 1, "next_offset": 1, "items": []any{map[string]any{
							"request": codersdk.StartHostWorkloadRequest{CreateWorkloadRequest: codersdk.CreateWorkloadRequest{Command: "foreign-command"}, OrganizationID: uuid.New()},
						}},
					})
				} else {
					http.Error(w, "private-supervisor-error", http.StatusServiceUnavailable)
				}
				return true
			}
			res := f.call(t, "admin", http.MethodGet, "/workload-hosts/"+f.agentID.String()+"/history", nil)
			require.Equal(t, http.StatusBadGateway, res.Code)
			require.NotContains(t, res.Body.String(), "foreign-command")
			require.NotContains(t, res.Body.String(), "private-supervisor-error")
			if !invalidHistory {
				res = f.call(t, "admin", http.MethodGet, "/workload-hosts/"+f.agentID.String()+"/stats", nil)
				require.Equal(t, http.StatusBadGateway, res.Code)
				require.NotContains(t, res.Body.String(), "private-supervisor-error")
			}
		})
	}
}
