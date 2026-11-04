package coderd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
)

// workloadRegistry is deliberately process-local. Restarting coderd loses the
// registry, not the host processes. There is no replay, recovery, or autoscaling.
// One active host per template keeps prepared generations coherent in this POC.
// Ordinary workspace stop/delete can still interrupt commands; an unreachable
// supervisor yields an unknown outcome, never an automatic replacement run.
type workloadRegistry struct {
	mu    sync.Mutex
	hosts map[uuid.UUID]*workloadHost
	runs  map[uuid.UUID]*workloadRun

	transportOnce sync.Once
	transport     *http.Transport
}

type workloadHost struct {
	agentID, organizationID, templateID, buildID uuid.UUID
	token                                        string
	info                                         codersdk.WorkloadHostInfo
	draining                                     bool // Protected by workloadRegistry.mu.
}

type workloadRun struct {
	// Serialize observation/cancellation with dispatch, not with other runs.
	op           sync.Mutex
	request      codersdk.CreateWorkloadRequest
	host         *workloadHost
	value        codersdk.Workload // Protected by workloadRegistry.mu.
	pending      bool
	dispatchDone chan struct{} // Closed after dispatch publishes its result.
}

func (api *API) registerWorkloadRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpmw.RequireExperiment(api.Experiments, codersdk.ExperimentWorkloads), httpmw.NoStore)
		r.Post("/workload-hosts", api.postWorkloadHost)
		r.Delete("/workload-hosts/{agent}", api.deleteWorkloadHost)
		r.Get("/workload-hosts/{agent}/stats", api.getWorkloadHostStats)
		r.Get("/workload-hosts/{agent}/history", api.getWorkloadHostHistory)
		r.Post("/workloads", api.postWorkload)
		r.Get("/workloads/{workload}", api.getWorkload)
		r.Get("/workloads/{workload}/input", api.getWorkloadInput)
		r.Delete("/workloads/{workload}", api.deleteWorkload)
	})
}

// @Summary Register a managed workload host (experimental)
// @ID register-a-managed-workload-host-experimental
// @Security CoderSessionToken
// @Tags Workspaces
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.RegisterWorkloadHostRequest true "Host registration"
// @Success 201 {object} codersdk.RegisteredWorkloadHost
// @Router /api/v2/organizations/{organization}/workload-hosts [post]
// @x-apidocgen {"skip": true}
func (api *API) postWorkloadHost(rw http.ResponseWriter, r *http.Request) {
	org := httpmw.OrganizationParam(r)
	if !api.Authorize(r, policy.ActionUpdate, org) {
		httpapi.ResourceNotFound(rw)
		return
	}
	var req codersdk.RegisterWorkloadHostRequest
	if !readWorkloadRequest(rw, r, &req) {
		return
	}
	if req.AgentID == uuid.Nil || !validWorkloadToken(req.Token) {
		workloadError(rw, r, http.StatusBadRequest, "A dedicated supervisor token of 32 to 4096 non-whitespace ASCII characters is required.")
		return
	}
	// Explicit organization administration authorizes discovering managed host
	// metadata, but not registering a host from a different organization.
	//nolint:gocritic // Narrow read-only discovery after organization authorization.
	ctx := dbauthz.AsSystemRestricted(r.Context())
	host, agent, err := api.workloadHostMetadata(ctx, org.ID, req.AgentID)
	if err != nil {
		workloadError(rw, r, http.StatusBadRequest, "Host must belong to this organization and its latest running build must use the active template version.")
		return
	}
	if tokenID, err := uuid.Parse(req.Token); err == nil && tokenID == agent.AuthToken {
		workloadError(rw, r, http.StatusBadRequest, "A workspace agent token cannot be used as a supervisor token.")
		return
	}
	host.token = req.Token
	if _, err := api.workloadHostRequest(r.Context(), host, http.MethodGet, "/info", nil, &host.info); err != nil || !validWorkloadHostInfo(host.info) || host.info.Draining {
		workloadError(rw, r, http.StatusBadGateway, "Workload supervisor is unavailable or has invalid prepared capacity.")
		return
	}
	reg := &api.workloads
	reg.mu.Lock()
	if reg.hosts == nil {
		reg.hosts = make(map[uuid.UUID]*workloadHost)
	}
	if old := reg.hosts[host.agentID]; old != nil {
		if old.draining || old.token != host.token || old.buildID != host.buildID || !reflect.DeepEqual(old.info.Generation, host.info.Generation) {
			reg.mu.Unlock()
			workloadError(rw, r, http.StatusConflict, "This host is already registered; drained hosts cannot be re-enrolled in this server process.")
			return
		}
	} else {
		for _, old := range reg.hosts {
			if old.templateID == host.templateID && !old.draining {
				reg.mu.Unlock()
				workloadError(rw, r, http.StatusConflict, "This POC permits one active host per template. Drain the existing host first.")
				return
			}
		}
		reg.hosts[host.agentID] = host
	}
	reg.mu.Unlock()
	httpapi.Write(r.Context(), rw, http.StatusCreated, codersdk.RegisteredWorkloadHost{AgentID: host.agentID, TemplateID: host.templateID, Info: host.info})
}

// @Summary Drain a managed workload host (experimental)
// @ID drain-a-managed-workload-host-experimental
// @Security CoderSessionToken
// @Tags Workspaces
// @Param organization path string true "Organization ID" format(uuid)
// @Param agent path string true "Workspace agent ID" format(uuid)
// @Success 204
// @Router /api/v2/organizations/{organization}/workload-hosts/{agent} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteWorkloadHost(rw http.ResponseWriter, r *http.Request) {
	org := httpmw.OrganizationParam(r)
	if !api.Authorize(r, policy.ActionUpdate, org) {
		httpapi.ResourceNotFound(rw)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "agent"))
	reg := &api.workloads
	reg.mu.Lock()
	host := reg.hosts[id]
	if err != nil || host == nil || host.organizationID != org.ID {
		reg.mu.Unlock()
		httpapi.ResourceNotFound(rw)
		return
	}
	// Keep the local drain even when the supervisor is unreachable. Placements
	// reserved before this point may complete; draining never cancels commands.
	host.draining = true
	reg.mu.Unlock()
	if _, err := api.workloadHostRequest(r.Context(), host, http.MethodPost, "/drain", nil, nil); err != nil {
		workloadError(rw, r, http.StatusBadGateway, "Host is drained locally, but supervisor drain could not be confirmed.")
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Create a workload on prepared capacity (experimental)
// @ID create-a-workload-on-prepared-capacity-experimental
// @Security CoderSessionToken
// @Tags Workspaces
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateWorkloadRequest true "Workload request"
// @Success 201 {object} codersdk.Workload
// @Success 200 {object} codersdk.Workload
// @Router /api/v2/organizations/{organization}/workloads [post]
// @x-apidocgen {"skip": true}
func (api *API) postWorkload(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.CreateWorkloadRequest
	if !readWorkloadRequest(rw, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		workloadError(rw, r, http.StatusBadRequest, "Invalid workload resources, identity, command, or environment.")
		return
	}
	org := httpmw.OrganizationParam(r)
	actor := httpmw.UserAuthorization(r.Context())
	ownerID, err := uuid.Parse(actor.ID)
	if err != nil {
		httpapi.ResourceNotFound(rw)
		return
	}
	template, err := api.Database.GetTemplateByID(r.Context(), req.TemplateID)
	if err != nil || template.OrganizationID != org.ID || template.Deleted || template.Deprecated != "" ||
		!api.Authorize(r, policy.ActionUse, template) ||
		!api.Authorize(r, policy.ActionCreate, rbac.ResourceWorkspace.InOrg(org.ID).WithOwner(ownerID.String())) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if api.workloadBrowserOnly(rw) {
		return
	}
	reg := &api.workloads
	reg.mu.Lock()
	if previous := reg.runs[req.ID]; previous != nil {
		value := previous.value
		matches := reflect.DeepEqual(previous.request, req)
		reg.mu.Unlock()
		switch {
		case value.OrganizationID != org.ID || value.OwnerID != ownerID:
			httpapi.ResourceNotFound(rw)
		case !matches:
			workloadError(rw, r, http.StatusConflict, "Workload ID is already reserved for a different request.")
		default:
			httpapi.Write(r.Context(), rw, http.StatusOK, value)
		}
		return
	}
	// Never evict an idempotency key to make room for another command.
	if len(reg.runs) >= 1024 {
		reg.mu.Unlock()
		workloadError(rw, r, http.StatusServiceUnavailable, "Workload history is full; this POC retains all idempotency keys until server restart.")
		return
	}
	run := &workloadRun{request: req, pending: true, dispatchDone: make(chan struct{}), value: codersdk.Workload{
		ID: req.ID, OrganizationID: org.ID, OwnerID: ownerID, TemplateID: req.TemplateID,
		CPU: req.CPU, MemoryBytes: req.MemoryBytes, State: "pending", CreatedAt: time.Now(),
	}}
	run.op.Lock()
	defer run.op.Unlock()
	if reg.runs == nil {
		reg.runs = make(map[uuid.UUID]*workloadRun)
	}
	// Reserve the normalized request before any network operation. Even pending
	// requests cannot reuse an ID with a different command or resource request.
	reg.runs[req.ID] = run
	var host *workloadHost
	for _, candidate := range reg.hosts {
		if candidate.organizationID == org.ID && candidate.templateID == req.TemplateID && !candidate.draining {
			host = candidate
			break
		}
	}
	reg.mu.Unlock()

	value := run.value
	value.State, value.Failure = "rejected", "No eligible prepared capacity is available."
	if host != nil {
		var connectionTiming workloadConnectionTiming
		ctx := httptrace.WithClientTrace(r.Context(), connectionTiming.trace())
		value = api.dispatchWorkload(ctx, run, host, value)
		value.Timings.HostConnectNS, value.Timings.HostConnectionReused = connectionTiming.result()
	}
	reg.mu.Lock()
	run.value, run.pending = value, false
	close(run.dispatchDone)
	reg.mu.Unlock()
	httpapi.Write(r.Context(), rw, http.StatusCreated, value)
}

// dispatchWorkload is called with run.op held. A single host means there is no
// cross-host retry, including after an ambiguous timeout or connection loss.
func (api *API) dispatchWorkload(ctx context.Context, run *workloadRun, host *workloadHost, value codersdk.Workload) codersdk.Workload {
	// The caller has explicitly authorized template use and workspace create.
	// Only registered host metadata is read as system, never caller-chosen SSH.
	//nolint:gocritic // Managed capacity is not owned by the invoking user.
	current, _, err := api.workloadHostMetadata(dbauthz.AsSystemRestricted(ctx), value.OrganizationID, host.agentID)
	if err != nil || current.buildID != host.buildID || current.templateID != value.TemplateID {
		return value
	}
	var info codersdk.WorkloadHostInfo
	if _, err := api.workloadHostRequest(ctx, host, http.MethodGet, "/info", nil, &info); err != nil || !validWorkloadHostInfo(info) || info.Draining || !reflect.DeepEqual(info.Generation, host.info.Generation) || info.AvailableCPU < value.CPU || info.AvailableMemoryBytes < value.MemoryBytes {
		return value
	}
	reg := &api.workloads
	reg.mu.Lock()
	if host.draining {
		reg.mu.Unlock()
		return value
	}
	value.AgentID, value.Generation = host.agentID, host.info.Generation
	value.State, value.Failure = "pending", ""
	run.host, run.value = host, value
	reg.mu.Unlock()
	// Placement and generation are pinned before dispatch. The supervisor makes
	// the atomic CPU/RAM admission decision, including its configured overhead.
	var observed codersdk.Workload
	status, err := api.workloadHostRequest(ctx, host, http.MethodPost, "/workloads", codersdk.StartHostWorkloadRequest{
		CreateWorkloadRequest: run.request, OrganizationID: value.OrganizationID,
		OwnerID: value.OwnerID, GenerationID: value.Generation.ID,
	}, &observed)
	if err == nil && mergeWorkloadObservation(&value, observed) {
		return value
	}
	if status == http.StatusTooManyRequests {
		// A definitive rejection has no remote run to observe or cancel.
		reg.mu.Lock()
		run.host = nil
		reg.mu.Unlock()
		value.State, value.Failure = "rejected", "Host rejected admission because prepared capacity is unavailable."
		return value
	}
	value.State, value.Failure = "unknown", "Dispatch outcome is unknown; this workload will not be automatically retried."
	return value
}

// @Summary Read workload output (experimental)
// @Description Returns an output snapshot, or with follow=true streams bounded application/x-ndjson batches and 15-second blank-line heartbeats until a terminal batch with no events confirms all output is drained.
// @ID read-workload-output-experimental
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce json,application/x-ndjson
// @Param organization path string true "Organization ID" format(uuid)
// @Param workload path string true "Workload ID" format(uuid)
// @Param offset query integer false "Output event offset"
// @Param follow query boolean false "Stream output until the terminal empty batch"
// @Success 200 {object} codersdk.WorkloadOutput
// @Router /api/v2/organizations/{organization}/workloads/{workload} [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP GET handlers write responses rather than return values.
func (api *API) getWorkload(rw http.ResponseWriter, r *http.Request) {
	run := api.authorizedWorkload(rw, r)
	if run == nil {
		return
	}
	offset := int64(0)
	var err error
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.ParseInt(raw, 10, 64)
	}
	if err != nil || offset < 0 {
		workloadError(rw, r, http.StatusBadRequest, "Offset must be a nonnegative event index.")
		return
	}
	if r.URL.Query().Get("follow") == "true" {
		api.followWorkload(rw, r, run, offset)
		return
	}
	reg := &api.workloads
	reg.mu.Lock()
	value, pending := run.value, run.pending
	reg.mu.Unlock()
	if pending {
		httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.WorkloadOutput{Workload: value, NextOffset: offset})
		return
	}
	run.op.Lock()
	defer run.op.Unlock()
	reg.mu.Lock()
	value = run.value
	reg.mu.Unlock()
	output := codersdk.WorkloadOutput{Workload: value, NextOffset: offset}
	if run.host != nil {
		var observed codersdk.WorkloadOutput
		status, err := api.workloadHostRequest(r.Context(), run.host, http.MethodGet, "/workloads/"+value.ID.String()+"?offset="+strconv.FormatInt(offset, 10), nil, &observed)
		if status == http.StatusBadRequest {
			workloadError(rw, r, http.StatusBadRequest, "Offset is outside the retained output range.")
			return
		}
		if err != nil || !mergeWorkloadObservation(&value, observed.Workload) || observed.NextOffset < offset {
			if value.Terminal() && value.State != "unknown" {
				workloadError(rw, r, http.StatusBadGateway, "Host output is unavailable; the previously observed execution outcome is retained.")
				return
			}
			value.State, value.Failure = "unknown", "Host is unavailable; execution outcome is unknown and will not be automatically retried."
		} else {
			output = observed
		}
		output.Workload = value
		reg.mu.Lock()
		run.value = value
		reg.mu.Unlock()
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, output)
}

// @Summary Cancel a workload (experimental)
// @ID cancel-a-workload-experimental
// @Security CoderSessionToken
// @Tags Workspaces
// @Param organization path string true "Organization ID" format(uuid)
// @Param workload path string true "Workload ID" format(uuid)
// @Success 204
// @Router /api/v2/organizations/{organization}/workloads/{workload} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteWorkload(rw http.ResponseWriter, r *http.Request) {
	run := api.authorizedWorkload(rw, r)
	if run == nil {
		return
	}
	// Wait for dispatch so a cancellation cannot race ahead of the host POST.
	run.op.Lock()
	defer run.op.Unlock()
	if run.host != nil {
		if _, err := api.workloadHostRequest(r.Context(), run.host, http.MethodDelete, "/workloads/"+run.request.ID.String(), nil, nil); err != nil {
			workloadError(rw, r, http.StatusBadGateway, "Cancellation could not be confirmed; the command may still be running.")
			return
		}
	}
	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) authorizedWorkload(rw http.ResponseWriter, r *http.Request) *workloadRun {
	id, err := uuid.Parse(chi.URLParam(r, "workload"))
	org := httpmw.OrganizationParam(r)
	reg := &api.workloads
	reg.mu.Lock()
	run := reg.runs[id]
	var value codersdk.Workload
	if run != nil {
		value = run.value
	}
	reg.mu.Unlock()
	if err != nil || run == nil || value.OrganizationID != org.ID ||
		(value.OwnerID.String() != httpmw.UserAuthorization(r.Context()).ID && !api.Authorize(r, policy.ActionUpdate, org)) {
		httpapi.ResourceNotFound(rw)
		return nil
	}
	// Ownership and organization administration do not bypass workspace scopes.
	action := policy.ActionRead
	if r.Method == http.MethodDelete {
		action = policy.ActionUpdate
	}
	if !api.Authorize(r, action, rbac.ResourceWorkspace.InOrg(org.ID).WithOwner(value.OwnerID.String())) {
		httpapi.ResourceNotFound(rw)
		return nil
	}
	if api.workloadBrowserOnly(rw) {
		return nil
	}
	return run
}

func (api *API) workloadBrowserOnly(rw http.ResponseWriter) bool {
	if override := api.WorkspaceClientCoordinateOverride.Load(); override != nil && *override != nil {
		return (*override)(rw)
	}
	return false
}

func (api *API) workloadHostMetadata(ctx context.Context, orgID, agentID uuid.UUID) (*workloadHost, database.WorkspaceAgent, error) {
	invalid := xerrors.New("host is not eligible")
	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, agentID)
	if err != nil || workspace.OrganizationID != orgID || workspace.Deleted || workspace.DormantAt.Valid || workspace.DeletingAt.Valid {
		return nil, database.WorkspaceAgent{}, invalid
	}
	agent, err := api.Database.GetWorkspaceAgentByID(ctx, agentID)
	if err != nil || agent.OperatingSystem != "linux" {
		return nil, database.WorkspaceAgent{}, invalid
	}
	resource, err := api.Database.GetWorkspaceResourceByID(ctx, agent.ResourceID)
	if err != nil {
		return nil, database.WorkspaceAgent{}, invalid
	}
	build, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil || build.JobID != resource.JobID || build.Transition != database.WorkspaceTransitionStart {
		return nil, database.WorkspaceAgent{}, invalid
	}
	job, err := api.Database.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil || !job.CompletedAt.Valid || job.CanceledAt.Valid || job.Error.String != "" {
		return nil, database.WorkspaceAgent{}, invalid
	}
	template, err := api.Database.GetTemplateByID(ctx, workspace.TemplateID)
	if err != nil || template.OrganizationID != orgID || template.Deleted || template.Deprecated != "" || template.ActiveVersionID != build.TemplateVersionID {
		return nil, database.WorkspaceAgent{}, invalid
	}
	return &workloadHost{agentID: agentID, organizationID: orgID, templateID: template.ID, buildID: build.ID}, agent, nil
}

func validWorkloadToken(token string) bool {
	if len(token) < 32 || len(token) > 4096 {
		return false
	}
	for _, ch := range token {
		if ch < 33 || ch > 126 {
			return false
		}
	}
	return true
}

func validWorkloadHostInfo(info codersdk.WorkloadHostInfo) bool {
	return info.Generation.ID != "" && info.Generation.Workdir != "" && info.CPU > 0 && info.MemoryBytes > 0 &&
		info.AvailableCPU >= 0 && info.AvailableCPU <= info.CPU && info.AvailableMemoryBytes >= 0 && info.AvailableMemoryBytes <= info.MemoryBytes
}

// mergeWorkloadObservation never trusts a supervisor to change ownership,
// placement, or connection timings. Supervisor failure text is not exposed.
func mergeWorkloadObservation(value *codersdk.Workload, observed codersdk.Workload) bool {
	if observed.ID != value.ID || observed.OrganizationID != value.OrganizationID || observed.OwnerID != value.OwnerID || observed.TemplateID != value.TemplateID || observed.Generation.ID != value.Generation.ID || observed.CPU != value.CPU || observed.MemoryBytes != value.MemoryBytes {
		return false
	}
	switch observed.State {
	case "pending", "requested", "reserved", "starting", "running", "succeeded", "failed", "canceled", "rejected", "unknown":
	default:
		return false
	}
	value.State, value.StartedAt, value.FinishedAt = observed.State, observed.StartedAt, observed.FinishedAt
	value.ExitCode, value.CleanupPending = observed.ExitCode, observed.CleanupPending
	value.Timings.ProvisionNS = nonnegativeWorkloadTiming(observed.Timings.ProvisionNS)
	value.Timings.ExecNS = nonnegativeWorkloadTiming(observed.Timings.ExecNS)
	value.Failure = ""
	if observed.Failure != "" {
		value.Failure = "Workload supervisor reported an execution or cleanup failure."
	}
	return true
}

func nonnegativeWorkloadTiming(ns *int64) *int64 {
	if ns == nil || *ns < 0 {
		return nil
	}
	return new(*ns)
}

// workloadConnectionTiming sums completed GetConn/GotConn pairs across dispatch
// requests, excluding request writes, host processing, and response reads.
// An unsuccessful acquisition has no GotConn and contributes no measurement.
// Dispatch requests are sequential, but trace callbacks may run concurrently.
type workloadConnectionTiming struct {
	mu        sync.Mutex
	started   time.Time
	connectNS int64
	acquired  bool
	reused    bool
}

func (t *workloadConnectionTiming) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(string) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.started = time.Now()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			if t.started.IsZero() {
				return
			}
			t.connectNS += time.Since(t.started).Nanoseconds()
			t.reused = info.Reused && (!t.acquired || t.reused)
			t.acquired = true
			t.started = time.Time{}
		},
	}
}

func (t *workloadConnectionTiming) result() (*int64, *bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.acquired {
		return nil, nil
	}
	return new(t.connectNS), new(t.reused)
}

// workloadTransport pools sockets by agent address. Each socket retains its
// provider lease until closed, including while idle between HTTP requests.
func (api *API) workloadTransport() *http.Transport {
	reg := &api.workloads
	reg.transportOnce.Do(func() {
		reg.transport = &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   2,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			DialContext: func(ctx context.Context, network, target string) (net.Conn, error) {
				agentID, ok := ctx.Value(agentIDKey{}).(uuid.UUID)
				addr := netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(agentID), workspacesdk.AgentHTTPAPIServerPort).String()
				if !ok || network != "tcp" || target != addr {
					return nil, xerrors.New("invalid workload supervisor address")
				}
				// Transport may finish a dial after its initiating request ends
				// and reuse it for another request. Bound that work independently,
				// and stop it when the API shuts down.
				ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				stop := context.AfterFunc(api.ctx, cancel)
				defer stop()
				conn, release, err := api.agentProvider.AgentConn(ctx, agentID)
				if err != nil {
					return nil, err
				}
				nc, err := conn.DialContext(ctx, network, target)
				if err != nil {
					release()
					return nil, err
				}
				return &netConnCloser{Conn: nc, close: sync.OnceFunc(release)}, nil
			},
		}
		context.AfterFunc(api.ctx, reg.transport.CloseIdleConnections)
	})
	return reg.transport
}

// workloadHostRequest uses only the dedicated supervisor token. Errors from the
// transport or supervisor may contain secrets, so callers receive static errors.
func (api *API) workloadHostRequest(ctx context.Context, host *workloadHost, method, path string, body, result any) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(api.ctx, cancel)
	defer stop()
	ctx = context.WithValue(ctx, agentIDKey{}, host.agentID)
	unavailable := xerrors.New("workload supervisor request failed")
	addr := netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(host.agentID), workspacesdk.AgentHTTPAPIServerPort).String()
	client := &http.Client{Transport: api.workloadTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var input io.Reader
	// A nonempty JSON body, including null for drain, and no GetBody prevent
	// Transport from retrying POSTs even after a zero-byte write failure.
	if body != nil || method == http.MethodPost {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, unavailable
		}
		input = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+addr+"/api/v0/workloads"+path, input)
	if err != nil {
		return 0, unavailable
	}
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+host.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return 0, unavailable
	}
	defer res.Body.Close()
	// Read through EOF so the socket can return to the pool. Limit both
	// successful and error responses, including responses callers ignore.
	data, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 || res.StatusCode < 200 || res.StatusCode >= 300 {
		return res.StatusCode, unavailable
	}
	if result != nil {
		if err := json.Unmarshal(data, result); err != nil {
			return res.StatusCode, unavailable
		}
	}
	return res.StatusCode, nil
}

func readWorkloadRequest(rw http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 512<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		workloadError(rw, r, http.StatusBadRequest, "Invalid workload request body.")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		workloadError(rw, r, http.StatusBadRequest, "Invalid workload request body.")
		return false
	}
	return true
}

func workloadError(rw http.ResponseWriter, r *http.Request, status int, message string) {
	httpapi.Write(r.Context(), rw, status, codersdk.Response{Message: message})
}
