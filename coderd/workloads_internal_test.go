package coderd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkloadFlow(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	f.register(t)
	// Managed host execution is not an SSH grant on the service workspace.
	require.False(t, f.api.HTTPAuth.AuthorizeContext(dbauthz.As(t.Context(), f.actors["owner"]), policy.ActionSSH, f.workspace))
	req := f.request()
	created := f.call(t, "owner", http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var workload codersdk.Workload
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &workload))
	require.Equal(t, "running", workload.State)
	require.Equal(t, f.agentID, workload.AgentID)
	require.Equal(t, f.info.Generation, workload.Generation)
	require.NotContains(t, created.Body.String(), req.Command)
	duplicate := f.call(t, "owner", http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusOK, duplicate.Code)
	require.JSONEq(t, created.Body.String(), duplicate.Body.String())
	require.EqualValues(t, 1, f.starts.Load())

	output := f.call(t, "owner", http.MethodGet, "/workloads/"+req.ID.String()+"?offset=0", nil)
	require.Equal(t, http.StatusOK, output.Code)
	var batch codersdk.WorkloadOutput
	require.NoError(t, json.Unmarshal(output.Body.Bytes(), &batch))
	require.Equal(t, []byte("hello\n"), batch.Events[0].Data)
	require.EqualValues(t, 1, batch.NextOffset)
	output = f.call(t, "owner", http.MethodGet, "/workloads/"+req.ID.String()+"?offset=1", nil)
	require.NoError(t, json.Unmarshal(output.Body.Bytes(), &batch))
	require.Empty(t, batch.Events)

	canceled := f.call(t, "owner", http.MethodDelete, "/workloads/"+req.ID.String(), nil)
	require.Equal(t, http.StatusNoContent, canceled.Code, canceled.Body.String())
	output = f.call(t, "owner", http.MethodGet, "/workloads/"+req.ID.String(), nil)
	require.NoError(t, json.Unmarshal(output.Body.Bytes(), &batch))
	require.Equal(t, "canceled", batch.Workload.State)

	drained := f.call(t, "admin", http.MethodDelete, "/workload-hosts/"+f.agentID.String(), nil)
	require.Equal(t, http.StatusNoContent, drained.Code)
	created = f.call(t, "owner", http.MethodPost, "/workloads", f.request())
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &workload))
	require.Equal(t, "rejected", workload.State)
	require.EqualValues(t, 1, f.starts.Load())
	require.EqualValues(t, 1, f.connections.Load())
	require.Zero(t, f.releases.Load())
	f.api.workloadTransport().CloseIdleConnections()
	require.Equal(t, f.connections.Load(), f.releases.Load())
}

func TestWorkloadPendingIdempotency(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	started, proceed := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(proceed) })
	defer unblock()
	f.startHook = func(http.ResponseWriter, *http.Request) bool {
		close(started)
		<-proceed
		return false
	}
	f.register(t)
	req := f.request()
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- f.call(t, "owner", http.MethodPost, "/workloads", req) }()
	ctx := testutil.Context(t, testutil.WaitLong)
	testutil.TryReceive(ctx, t, started)
	duplicate := f.call(t, "owner", http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusOK, duplicate.Code)
	var pending codersdk.Workload
	require.NoError(t, json.Unmarshal(duplicate.Body.Bytes(), &pending))
	require.Equal(t, "pending", pending.State)
	require.Equal(t, f.agentID, pending.AgentID)
	require.Equal(t, f.info.Generation.ID, pending.Generation.ID)
	require.Nil(t, pending.Timings.HostConnectNS, "timings are published only after dispatch completes")
	require.Nil(t, pending.Timings.HostConnectionReused)
	changed := req
	changed.Command = "different sensitive command"
	conflict := f.call(t, "owner", http.MethodPost, "/workloads", changed)
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.NotContains(t, conflict.Body.String(), changed.Command)
	unblock()
	created := testutil.TryReceive(ctx, t, result)
	require.Equal(t, http.StatusCreated, created.Code)
	var completed codersdk.Workload
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &completed))
	require.NotNil(t, completed.Timings.HostConnectNS)
	require.Equal(t, new(true), completed.Timings.HostConnectionReused)
	duplicate = f.call(t, "owner", http.MethodPost, "/workloads", req)
	require.Equal(t, http.StatusOK, duplicate.Code)
	require.JSONEq(t, created.Body.String(), duplicate.Body.String())
	require.EqualValues(t, 1, f.starts.Load())
}

func TestWorkloadUnknownDispatchNeverReplays(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	f.startHook = func(w http.ResponseWriter, _ *http.Request) bool {
		conn, _, err := w.(http.Hijacker).Hijack()
		assert.NoError(t, err)
		if err == nil {
			_ = conn.Close()
		}
		return true
	}
	f.register(t)
	req := f.request()
	created := f.call(t, "owner", http.MethodPost, "/workloads", req)
	var value codersdk.Workload
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &value))
	require.Equal(t, "unknown", value.State)
	require.Equal(t, f.agentID, value.AgentID)
	duplicate := f.call(t, "owner", http.MethodPost, "/workloads", req)
	require.JSONEq(t, created.Body.String(), duplicate.Body.String())
	require.EqualValues(t, 1, f.starts.Load())
}

func TestWorkloadDefinitiveCapacityRejection(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	f.startHook = func(w http.ResponseWriter, _ *http.Request) bool {
		http.Error(w, "private supervisor failure", http.StatusTooManyRequests)
		return true
	}
	f.register(t)
	req := f.request()
	res := f.call(t, "owner", http.MethodPost, "/workloads", req)
	var value codersdk.Workload
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &value))
	require.Equal(t, "rejected", value.State)
	require.NotContains(t, value.Failure, "private supervisor failure")
	requests := f.requests.Load()
	res = f.call(t, "owner", http.MethodGet, "/workloads/"+req.ID.String(), nil)
	var output codersdk.WorkloadOutput
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &output))
	require.Equal(t, "rejected", output.Workload.State)
	require.Equal(t, requests, f.requests.Load())
}

func TestWorkloadAuthorization(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	for _, actor := range []string{"owner", "other", "template-admin", "foreign-admin"} {
		denied := f.call(t, actor, http.MethodPost, "/workload-hosts", codersdk.RegisterWorkloadHostRequest{AgentID: f.agentID, Token: f.token})
		require.Equal(t, http.StatusNotFound, denied.Code, actor)
		denied = f.call(t, actor, http.MethodDelete, "/workload-hosts/"+f.agentID.String(), nil)
		require.Equal(t, http.StatusNotFound, denied.Code, actor)
	}
	require.Zero(t, f.connections.Load())
	// Template administration alone grants use, but not workspace creation.
	require.True(t, f.api.HTTPAuth.AuthorizeContext(dbauthz.As(t.Context(), f.actors["no-workspace-create"]), policy.ActionUse, f.template))
	for _, actor := range []string{"no-template-use", "no-workspace-create", "foreign-admin"} {
		denied := f.call(t, actor, http.MethodPost, "/workloads", f.request())
		require.Equal(t, http.StatusNotFound, denied.Code, actor+": "+denied.Body.String())
	}
	require.Zero(t, f.connections.Load())
	f.register(t)
	req := f.request()
	require.Equal(t, http.StatusCreated, f.call(t, "owner", http.MethodPost, "/workloads", req).Code)
	for _, actor := range []string{"other", "template-admin", "foreign-admin", "owner-restricted"} {
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			denied := f.call(t, actor, method, "/workloads/"+req.ID.String(), nil)
			require.Equal(t, http.StatusNotFound, denied.Code, actor+" "+method)
		}
	}
	require.Equal(t, http.StatusNotFound, f.call(t, "other", http.MethodPost, "/workloads", req).Code)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		require.Less(t, f.call(t, "admin", method, "/workloads/"+req.ID.String(), nil).Code, 300)
	}

	// Organization path isolation holds even for an administrator of both orgs.
	path := "/organizations/" + uuid.NewString() + "/workloads/" + req.ID.String()
	require.Equal(t, http.StatusNotFound, f.callPath(t, "admin", http.MethodGet, path, nil).Code)
}

func TestWorkloadRestrictedOrganizationAdmin(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	f.register(t)
	req := f.request()
	require.Equal(t, http.StatusCreated, f.call(t, "owner", http.MethodPost, "/workloads", req).Code)
	f.actors["restricted-org-admin"] = rbac.Subject{
		ID:    uuid.NewString(),
		Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAdmin(f.orgID)},
		Scope: rbac.Scope{
			Role: rbac.Role{
				Identifier: rbac.RoleIdentifier{Name: "organization-only"},
				Site: rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceOrganization.Type: {policy.ActionRead, policy.ActionUpdate},
				}),
				User:    []rbac.Permission{},
				ByOrgID: map[string]rbac.OrgPermissions{},
			},
			AllowIDList: []rbac.AllowListElement{rbac.AllowListAll()},
		},
	}
	ctx := dbauthz.As(t.Context(), f.actors["restricted-org-admin"])
	org := database.Organization{ID: f.orgID}
	require.True(t, f.api.HTTPAuth.AuthorizeContext(ctx, policy.ActionRead, org))
	require.True(t, f.api.HTTPAuth.AuthorizeContext(ctx, policy.ActionUpdate, org))
	requests := f.requests.Load()
	for _, route := range []struct{ method, query string }{
		{http.MethodGet, ""},
		{http.MethodGet, "?follow=true"},
		{http.MethodDelete, ""},
	} {
		t.Run(route.method+route.query, func(t *testing.T) {
			t.Parallel()
			denied := f.call(t, "restricted-org-admin", route.method, "/workloads/"+req.ID.String()+route.query, nil)
			require.Equal(t, http.StatusNotFound, denied.Code)
			require.Equal(t, requests, f.requests.Load(), "scope denial must occur before contacting the supervisor")
		})
	}
}

func TestWorkloadHostEligibility(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*workloadTest)
	}{
		{"Stopped", func(f *workloadTest) { f.build.Transition = database.WorkspaceTransitionStop }},
		{"OldAgent", func(f *workloadTest) { f.build.JobID = uuid.New() }},
		{"InactiveTemplateVersion", func(f *workloadTest) { f.template.ActiveVersionID = uuid.New() }},
		{"OtherOrganization", func(f *workloadTest) { f.workspace.OrganizationID = uuid.New() }},
		{"DeletedWorkspace", func(f *workloadTest) { f.workspace.Deleted = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newWorkloadTest(t)
			tc.change(f)
			res := f.call(t, "admin", http.MethodPost, "/workload-hosts", codersdk.RegisterWorkloadHostRequest{AgentID: f.agentID, Token: f.token})
			require.Equal(t, http.StatusBadRequest, res.Code)
			require.Zero(t, f.connections.Load())
		})
	}
}

func TestWorkloadAdmission(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*workloadTest)
	}{
		{"CPU", func(f *workloadTest) { f.info.AvailableCPU = 0 }},
		{"RAM", func(f *workloadTest) { f.info.AvailableMemoryBytes = 0 }},
		{"Generation", func(f *workloadTest) { f.info.Generation.ID = "changed" }},
		{"StoppedAfterRegistration", func(f *workloadTest) { f.build.Transition = database.WorkspaceTransitionStop }},
		{"VersionChangedAfterRegistration", func(f *workloadTest) { f.template.ActiveVersionID = uuid.New() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newWorkloadTest(t)
			f.register(t)
			tc.change(f)
			res := f.call(t, "owner", http.MethodPost, "/workloads", f.request())
			var value codersdk.Workload
			require.NoError(t, json.Unmarshal(res.Body.Bytes(), &value))
			require.Equal(t, "rejected", value.State)
			require.Zero(t, f.starts.Load())
		})
	}
}

func TestWorkloadTokenAndRedirectSecurity(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	for _, token := range []string{"short", f.agentToken.String(), strings.ReplaceAll(f.agentToken.String(), "-", ""), strings.Repeat("x", 31) + "\n"} {
		res := f.call(t, "admin", http.MethodPost, "/workload-hosts", codersdk.RegisterWorkloadHostRequest{AgentID: f.agentID, Token: token})
		require.Equal(t, http.StatusBadRequest, res.Code)
		require.NotContains(t, res.Body.String(), token)
	}
	require.Zero(t, f.connections.Load())
	f.infoHook = func(w http.ResponseWriter, r *http.Request) bool {
		http.Redirect(w, r, "http://example.invalid/stolen-token", http.StatusTemporaryRedirect)
		return true
	}
	res := f.call(t, "admin", http.MethodPost, "/workload-hosts", codersdk.RegisterWorkloadHostRequest{AgentID: f.agentID, Token: f.token})
	require.Equal(t, http.StatusBadGateway, res.Code)
	require.EqualValues(t, 1, f.connections.Load())
	f.api.workloadTransport().CloseIdleConnections()
	require.Equal(t, f.connections.Load(), f.releases.Load())
	require.NotContains(t, res.Body.String(), f.token)
}

func TestWorkloadObservationPreservesIdentity(t *testing.T) {
	t.Parallel()
	value := codersdk.Workload{ID: uuid.New(), OrganizationID: uuid.New(), OwnerID: uuid.New(), TemplateID: uuid.New(), AgentID: uuid.New(), Generation: codersdk.WorkloadGeneration{ID: "pinned"}, CPU: 1, MemoryBytes: 1 << 30, State: "pending"}
	observed := value
	observed.State = "requested"
	observed.AgentID = uuid.New()
	observed.Failure = "sensitive command or supervisor credential"
	originalAgent := value.AgentID
	require.True(t, mergeWorkloadObservation(&value, observed))
	require.Equal(t, "requested", value.State)
	require.Equal(t, originalAgent, value.AgentID)
	require.NotContains(t, value.Failure, observed.Failure)
	for _, state := range []string{"reserved", "starting", "running"} {
		observed.State = state
		require.True(t, mergeWorkloadObservation(&value, observed))
		require.Equal(t, state, value.State)
	}
	observed.OwnerID = uuid.New()
	require.False(t, mergeWorkloadObservation(&value, observed))
	require.NotEqual(t, observed.OwnerID, value.OwnerID)
}

func TestWorkloadSingleHostPerTemplate(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	existing := &workloadHost{agentID: uuid.New(), organizationID: f.orgID, templateID: f.template.ID}
	f.api.workloads.hosts = map[uuid.UUID]*workloadHost{existing.agentID: existing}
	res := f.call(t, "admin", http.MethodPost, "/workload-hosts", codersdk.RegisterWorkloadHostRequest{AgentID: f.agentID, Token: f.token})
	require.Equal(t, http.StatusConflict, res.Code)
	existing.draining = true
	f.register(t)
	// Organization administrators, not only deployment owners, may register.
	f.actors["org-admin"] = rbac.Subject{ID: uuid.NewString(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAdmin(f.orgID)}, Scope: rbac.ScopeAll}
	res = f.call(t, "org-admin", http.MethodPost, "/workload-hosts", codersdk.RegisterWorkloadHostRequest{AgentID: f.agentID, Token: f.token})
	require.Equal(t, http.StatusCreated, res.Code)
}

func TestWorkloadBrowserOnlyAndExperiment(t *testing.T) {
	t.Parallel()
	f := newWorkloadTest(t)
	blocked := func(w http.ResponseWriter) bool {
		w.WriteHeader(http.StatusConflict)
		return true
	}
	f.api.WorkspaceClientCoordinateOverride.Store(&blocked)
	require.Equal(t, http.StatusConflict, f.call(t, "owner", http.MethodPost, "/workloads", f.request()).Code)
	require.Zero(t, f.connections.Load())
	f.api.Experiments = nil
	f.router = f.newRouter()
	require.Equal(t, http.StatusForbidden, f.call(t, "admin", http.MethodPost, "/workload-hosts", nil).Code)
	require.Equal(t, http.StatusForbidden, f.call(t, "owner", http.MethodPost, "/workloads", nil).Code)
}

type workloadTest struct {
	api         *API
	db          *dbmock.MockStore
	router      http.Handler
	agentID     uuid.UUID
	agentToken  uuid.UUID
	orgID       uuid.UUID
	token       string
	template    database.Template
	workspace   database.Workspace
	build       database.WorkspaceBuild
	info        codersdk.WorkloadHostInfo
	actors      map[string]rbac.Subject
	connections atomic.Int64
	requests    atomic.Int64
	releases    atomic.Int64
	starts      atomic.Int64
	startHook   func(http.ResponseWriter, *http.Request) bool
	infoHook    func(http.ResponseWriter, *http.Request) bool
	reportHook  func(http.ResponseWriter, *http.Request) bool
	outputHook  func(http.ResponseWriter, *http.Request) bool
	inputHook   func(http.ResponseWriter, *http.Request)
}

func newWorkloadTest(t *testing.T) *workloadTest {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &workloadTest{db: dbmock.NewMockStore(ctrl), agentID: uuid.New(), agentToken: uuid.New(), orgID: uuid.New(), token: strings.Repeat("supervisor-secret-", 3)}
	ownerID, otherID, noUseID, noCreateID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.template = database.Template{ID: uuid.New(), OrganizationID: f.orgID, ActiveVersionID: uuid.New(), UserACL: database.TemplateACL{
		ownerID.String(): {"read", "use"}, otherID.String(): {"read", "use"}, noCreateID.String(): {"read", "use"},
	}}
	f.workspace = database.Workspace{ID: uuid.New(), OwnerID: uuid.New(), OrganizationID: f.orgID, TemplateID: f.template.ID}
	f.build = database.WorkspaceBuild{ID: uuid.New(), WorkspaceID: f.workspace.ID, JobID: uuid.New(), TemplateVersionID: f.template.ActiveVersionID, Transition: database.WorkspaceTransitionStart}
	f.info = codersdk.WorkloadHostInfo{Generation: codersdk.WorkloadGeneration{ID: "prepared-commit", Repositories: map[string]string{"project": strings.Repeat("a", 40)}, Workdir: "/project"}, Runtime: "docker", CPU: 8, MemoryBytes: 8 << 30, AvailableCPU: 7, AvailableMemoryBytes: 7 << 30}
	f.actors = make(map[string]rbac.Subject)
	member := rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgWorkspaceAccess(f.orgID)}
	for name, id := range map[string]uuid.UUID{"owner": ownerID, "other": otherID, "no-template-use": noUseID} {
		f.actors[name] = rbac.Subject{ID: id.String(), Type: rbac.SubjectTypeUser, Roles: member, Scope: rbac.ScopeAll}
	}
	f.actors["admin"] = rbac.Subject{ID: uuid.NewString(), Roles: rbac.RoleIdentifiers{rbac.RoleOwner(), rbac.RoleMember()}, Scope: rbac.ScopeAll}
	f.actors["foreign-admin"] = rbac.Subject{ID: uuid.NewString(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAdmin(uuid.New())}, Scope: rbac.ScopeAll}
	f.actors["template-admin"] = rbac.Subject{ID: uuid.NewString(), Roles: rbac.RoleIdentifiers{rbac.RoleMember(), rbac.RoleTemplateAdmin(), rbac.ScopedRoleOrgWorkspaceAccess(f.orgID)}, Scope: rbac.ScopeAll}
	f.actors["no-workspace-create"] = rbac.Subject{ID: noCreateID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleTemplateAdmin()}, Scope: rbac.ScopeAll}
	restricted := f.actors["owner"]
	restricted.Scope = rbac.ScopeApplicationConnect
	f.actors["owner-restricted"] = restricted
	logger := testutil.Logger(t)
	f.api = &API{ctx: t.Context(), Options: &Options{Database: f.db, Logger: logger}, HTTPAuth: &HTTPAuthorizer{Authorizer: rbac.NewAuthorizer(prometheus.NewRegistry()), Logger: logger}, Experiments: codersdk.Experiments{codersdk.ExperimentWorkloads}}
	f.db.EXPECT().GetOrganizationByID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id uuid.UUID) (database.Organization, error) {
		return database.Organization{ID: id}, nil
	}).AnyTimes()
	f.db.EXPECT().GetTemplateByID(gomock.Any(), f.template.ID).DoAndReturn(func(context.Context, uuid.UUID) (database.Template, error) { return f.template, nil }).AnyTimes()
	f.db.EXPECT().GetWorkspaceByAgentID(gomock.Any(), f.agentID).DoAndReturn(func(ctx context.Context, _ uuid.UUID) (database.Workspace, error) {
		actor, ok := dbauthz.ActorFromContext(ctx)
		assert.True(t, ok)
		assert.Equal(t, rbac.SubjectTypeSystemRestricted, actor.Type)
		return f.workspace, nil
	}).AnyTimes()
	resourceID := uuid.New()
	f.db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), f.agentID).Return(database.WorkspaceAgent{ID: f.agentID, ResourceID: resourceID, AuthToken: f.agentToken, OperatingSystem: "linux"}, nil).AnyTimes()
	f.db.EXPECT().GetWorkspaceResourceByID(gomock.Any(), resourceID).Return(database.WorkspaceResource{ID: resourceID, JobID: f.build.JobID}, nil).AnyTimes()
	f.db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), f.workspace.ID).DoAndReturn(func(context.Context, uuid.UUID) (database.WorkspaceBuild, error) { return f.build, nil }).AnyTimes()
	f.db.EXPECT().GetProvisionerJobByID(gomock.Any(), f.build.JobID).Return(database.ProvisionerJob{CompletedAt: sql.NullTime{Valid: true, Time: time.Now()}}, nil).AnyTimes()

	var mu sync.Mutex
	runs := make(map[uuid.UUID]codersdk.Workload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		assert.Equal(t, "Bearer "+f.token, r.Header.Get("Authorization"))
		path := strings.TrimPrefix(r.URL.Path, "/api/v0/workloads")
		if (path == "/stats" || path == "/history") && f.reportHook != nil && f.reportHook(w, r) {
			return
		}
		if path == "/info" {
			if f.infoHook != nil && f.infoHook(w, r) {
				return
			}
			_ = json.NewEncoder(w).Encode(f.info)
			return
		}
		if path == "/drain" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if path == "/workloads" && r.Method == http.MethodPost {
			f.starts.Add(1)
			if f.startHook != nil && f.startHook(w, r) {
				return
			}
			var req codersdk.StartHostWorkloadRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&req)) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			value := codersdk.Workload{ID: req.ID, OrganizationID: req.OrganizationID, OwnerID: req.OwnerID, TemplateID: req.TemplateID, CPU: req.CPU, MemoryBytes: req.MemoryBytes, Generation: f.info.Generation, State: "running"}
			assert.Equal(t, f.info.Generation.ID, req.GenerationID)
			mu.Lock()
			runs[value.ID] = value
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(value)
			return
		}
		if strings.HasSuffix(path, "/input") && f.inputHook != nil {
			f.inputHook(w, r)
			return
		}
		id, err := uuid.Parse(strings.TrimPrefix(path, "/workloads/"))
		if !assert.NoError(t, err) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if f.outputHook != nil && f.outputHook(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		value := runs[id]
		if r.Method == http.MethodDelete {
			value.State = "canceled"
			runs[id] = value
			w.WriteHeader(http.StatusNoContent)
			return
		}
		output := codersdk.WorkloadOutput{Workload: value, NextOffset: 1}
		if offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64); offset == 0 {
			output.Events = []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("hello\n")}}
		}
		_ = json.NewEncoder(w).Encode(output)
	}))
	t.Cleanup(server.Close)
	agent := agentconnmock.NewMockAgentConn(ctrl)
	addr := netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(f.agentID), workspacesdk.AgentHTTPAPIServerPort).String()
	agent.EXPECT().DialContext(gomock.Any(), "tcp", addr).DoAndReturn(func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}).AnyTimes()
	f.api.agentProvider = fakeAgentProvider{agentConn: func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		assert.Equal(t, f.agentID, id)
		f.connections.Add(1)
		return agent, func() { f.releases.Add(1) }, nil
	}}
	f.router = f.newRouter()
	t.Cleanup(func() { f.api.workloadTransport().CloseIdleConnections() })
	return f
}

func (f *workloadTest) newRouter() http.Handler {
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := f.actors[r.Header.Get("Test-Actor")]
			next.ServeHTTP(w, r.WithContext(dbauthz.As(r.Context(), actor)))
		})
	})
	router.Route("/organizations/{organization}", func(r chi.Router) {
		r.Use(httpmw.ExtractOrganizationParam(f.db))
		f.api.registerWorkloadRoutes(r)
	})
	return router
}

func (f *workloadTest) request() codersdk.CreateWorkloadRequest {
	return codersdk.CreateWorkloadRequest{ID: uuid.New(), TemplateID: f.template.ID, CPU: 2, MemoryBytes: 1 << 30, Command: "printf sensitive-command"}
}

func (f *workloadTest) register(t *testing.T) {
	t.Helper()
	res := f.call(t, "admin", http.MethodPost, "/workload-hosts", codersdk.RegisterWorkloadHostRequest{AgentID: f.agentID, Token: f.token})
	require.Equal(t, http.StatusCreated, res.Code, res.Body.String())
	require.NotContains(t, res.Body.String(), f.token)
}

func (f *workloadTest) call(t *testing.T, actor, method, path string, value any) *httptest.ResponseRecorder {
	t.Helper()
	return f.callPath(t, actor, method, "/organizations/"+f.orgID.String()+path, value)
}

func (f *workloadTest) callPath(t *testing.T, actor, method, path string, value any) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if value != nil {
		data, err := json.Marshal(value)
		require.NoError(t, err)
		body = bytes.NewReader(data)
	}
	r := httptest.NewRequestWithContext(t.Context(), method, path, body)
	r.Header.Set("Test-Actor", actor)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}
