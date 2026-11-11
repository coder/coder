package coderd

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/provisioner"
)

const (
	// maxWorkloadsPerWorkspace bounds how many workloads one workspace runs
	// at a time.
	maxWorkloadsPerWorkspace = 16
	// maxWorkloadCommandBytes bounds a workload's command.
	maxWorkloadCommandBytes = 64 << 10
	// workloadStartTimeout bounds how long a daemon may take to start a
	// sandbox. It allows for pulling an image.
	workloadStartTimeout = 3 * time.Minute
	// workloadStartGrace is how long an identity may exist without the
	// daemon knowing its sandbox before it is considered lost.
	workloadStartGrace = workloadStartTimeout + time.Minute
	// workloadAPIKeyIDPrefix starts the ID of the API key a workload
	// identity is presented as on user API routes. Real API key IDs never
	// contain a colon.
	workloadAPIKeyIDPrefix = "workload:"
)

func (api *API) registerWorkspaceWorkloadRoutes(r chi.Router) {
	r.Route("/workloads", func(r chi.Router) {
		r.Use(httpmw.RequireExperiment(api.Experiments, codersdk.ExperimentRuntimeWorkloads))
		r.Post("/", api.postWorkspaceWorkload)
		r.Get("/", api.workspaceWorkloads)
		r.Get("/{workload}/logs", api.workspaceWorkloadLogs)
		r.Delete("/{workload}", api.deleteWorkspaceWorkload)
	})
}

// workloadIdentityName is how a workload's identity is displayed.
func workloadIdentityName(workspaceID, identityID uuid.UUID) string {
	return workspaceID.String() + "/workload/" + identityID.String()
}

// daemonIdentityName is how a template-declared daemon's identity is
// displayed.
func daemonIdentityName(workspaceID, agentID uuid.UUID) string {
	return workspaceID.String() + "/agent/" + agentID.String()
}

// workloadScopes converts stored scope names to the SDK type. It never
// returns nil, so the API always serializes a list.
func workloadScopes(scopes []string) []codersdk.APIKeyScope {
	out := make([]codersdk.APIKeyScope, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, codersdk.APIKeyScope(scope))
	}
	return out
}

// isWorkloadCaller reports whether the request was made by a workload.
func isWorkloadCaller(r *http.Request) bool {
	return strings.HasPrefix(httpmw.APIKey(r).ID, workloadAPIKeyIDPrefix)
}

func workloadFromIdentity(identity database.WorkloadIdentity, agentName, sponsorUsername string, status codersdk.WorkloadStatus) codersdk.Workload {
	workload := codersdk.Workload{
		ID:              identity.ID,
		Name:            identity.Name,
		WorkspaceID:     identity.WorkspaceID,
		AgentID:         identity.WorkspaceAgentID,
		AgentName:       agentName,
		Status:          status,
		Command:         identity.Command,
		CreatedAt:       identity.CreatedAt,
		Identity:        workloadIdentityName(identity.WorkspaceID, identity.ID),
		SponsorID:       identity.SponsorID,
		SponsorUsername: sponsorUsername,
		Scopes:          workloadScopes(identity.Scopes),
	}
	if identity.SandboxID != "" {
		workload.Sandbox = &codersdk.WorkloadSandbox{Runtime: identity.Runtime, ID: identity.SandboxID}
	}
	return workload
}

// workloadAgentRequest calls a workspace daemon's workload API over the
// tailnet. The caller must close the response body and call release.
func (api *API) workloadAgentRequest(ctx context.Context, agentID uuid.UUID, method, path string, body any) (*http.Response, func(), error) {
	conn, release, err := api.agentProvider.AgentConn(ctx, agentID)
	if err != nil {
		return nil, nil, xerrors.Errorf("connect to the workspace daemon: %w", err)
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			release()
			return nil, nil, xerrors.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	// The connection's HTTP client dials the daemon whatever the host is.
	url := "http://workspace-daemon:" + strconv.Itoa(workspacesdk.AgentHTTPAPIServerPort) + "/api/v0/workloads" + path
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		release()
		return nil, nil, xerrors.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := conn.AppHTTPClient().Do(req)
	if err != nil {
		release()
		return nil, nil, xerrors.Errorf("call the workspace daemon: %w", err)
	}
	return res, release, nil
}

// errWorkloadsUnsupported means the daemon has no workload runtime.
var errWorkloadsUnsupported = xerrors.New("the workspace daemon cannot run workloads")

// readAgentWorkloadError turns a daemon's error response into an error.
func readAgentWorkloadError(res *http.Response) error {
	if res.StatusCode == http.StatusNotFound && res.Header.Get("Content-Type") != "application/json" {
		// The daemon does not serve the workload API at all.
		return errWorkloadsUnsupported
	}
	var resp codersdk.Response
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err := json.Unmarshal(raw, &resp); err != nil || resp.Message == "" {
		if res.StatusCode == http.StatusNotFound {
			return errWorkloadsUnsupported
		}
		return xerrors.Errorf("workspace daemon returned status %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	if resp.Detail != "" {
		return xerrors.Errorf("%s %s", resp.Message, resp.Detail)
	}
	return xerrors.New(resp.Message)
}

// agentWorkloadStates returns what a daemon knows about its workloads.
func (api *API) agentWorkloadStates(ctx context.Context, agentID uuid.UUID) (map[uuid.UUID]codersdk.AgentWorkloadState, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	res, release, err := api.workloadAgentRequest(ctx, agentID, http.MethodGet, "/", nil)
	if err != nil {
		return nil, err
	}
	defer release()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readAgentWorkloadError(res)
	}
	var states []codersdk.AgentWorkloadState
	if err := json.NewDecoder(res.Body).Decode(&states); err != nil {
		return nil, xerrors.Errorf("decode workload states: %w", err)
	}
	byID := make(map[uuid.UUID]codersdk.AgentWorkloadState, len(states))
	for _, state := range states {
		byID[state.ID] = state
	}
	return byID, nil
}

func (api *API) revokeWorkloadIdentity(ctx context.Context, id uuid.UUID, reason string) error {
	//nolint:gocritic // Identities have no user-facing permission of their own yet.
	err := api.Database.RevokeWorkloadIdentityByID(dbauthz.AsSystemRestricted(ctx), database.RevokeWorkloadIdentityByIDParams{
		ID:        id,
		RevokedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
	})
	if err != nil {
		return xerrors.Errorf("revoke workload identity: %w", err)
	}
	api.Logger.Info(ctx, "revoked workload identity", slog.F("workload_id", id), slog.F("reason", reason))
	return nil
}

// listWorkspaceWorkloads returns the workspace's live workloads. It asks
// the daemons what still runs, and revokes the identities of workloads that
// have exited or that belong to a daemon of an earlier build.
func (api *API) listWorkspaceWorkloads(ctx context.Context, workspaceID uuid.UUID) ([]codersdk.Workload, error) {
	//nolint:gocritic // The caller is authorized for the workspace.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	identities, err := api.Database.GetWorkloadIdentitiesByWorkspaceID(sysCtx, workspaceID)
	if err != nil {
		return nil, xerrors.Errorf("get workload identities: %w", err)
	}
	result := []codersdk.Workload{}
	if len(identities) == 0 {
		return result, nil
	}
	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(sysCtx, workspaceID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace agents: %w", err)
	}
	latest := make(map[uuid.UUID]database.WorkspaceAgent, len(agents))
	for _, agent := range agents {
		latest[agent.ID] = agent
	}

	sponsors := make(map[uuid.UUID]string)
	states := make(map[uuid.UUID]map[uuid.UUID]codersdk.AgentWorkloadState)
	unreachable := make(map[uuid.UUID]bool)
	now := dbtime.Now()
	for _, identity := range identities {
		agent, ok := latest[identity.WorkspaceAgentID]
		if !ok {
			// The daemon that ran the workload is gone, and with it the
			// only thing that could vouch for the identity.
			if err := api.revokeWorkloadIdentity(ctx, identity.ID, "its workspace daemon is gone"); err != nil {
				return nil, err
			}
			continue
		}
		if _, ok := sponsors[identity.SponsorID]; !ok {
			sponsor, err := api.Database.GetUserByID(sysCtx, identity.SponsorID)
			if err != nil {
				return nil, xerrors.Errorf("get sponsor: %w", err)
			}
			sponsors[identity.SponsorID] = sponsor.Username
		}
		if _, asked := states[agent.ID]; !asked && !unreachable[agent.ID] {
			connected := agent.Status(now, api.AgentInactiveDisconnectTimeout).Status == database.WorkspaceAgentStatusConnected
			var agentStates map[uuid.UUID]codersdk.AgentWorkloadState
			if connected {
				agentStates, err = api.agentWorkloadStates(ctx, agent.ID)
			}
			if !connected || err != nil {
				unreachable[agent.ID] = true
			} else {
				states[agent.ID] = agentStates
			}
		}

		status := codersdk.WorkloadStatusUnknown
		if !unreachable[agent.ID] {
			state, known := states[agent.ID][identity.ID]
			switch {
			case known && state.Running:
				status = codersdk.WorkloadStatusRunning
			case !known && !identity.StartedAt.Valid && now.Sub(identity.CreatedAt) < workloadStartGrace:
				status = codersdk.WorkloadStatusStarting
			default:
				// The command has exited, or the daemon lost the sandbox.
				if err := api.revokeWorkloadIdentity(ctx, identity.ID, "its command is no longer running"); err != nil {
					return nil, err
				}
				continue
			}
		}
		result = append(result, workloadFromIdentity(identity, agent.Name, sponsors[identity.SponsorID], status))
	}
	return result, nil
}

// @Summary Run a workload (experimental)
// @ID create-workspace-workload
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param request body codersdk.CreateWorkloadRequest true "Create workload request"
// @Success 201 {object} codersdk.Workload
// @Router /api/v2/workspaces/{workspace}/workloads [post]
// @x-apidocgen {"skip": true}
func (api *API) postWorkspaceWorkload(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, policy.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	// A workload acts with its sponsor's permissions, so only the workspace
	// owner may sponsor one in their workspace. Any user account can be a
	// sponsor, including a service account. A workload cannot sponsor
	// another: that would let it hand out permissions it was not given.
	apiKey := httpmw.APIKey(r)
	if apiKey.UserID != workspace.OwnerID || isWorkloadCaller(r) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the owner of a workspace can run workloads in it.",
		})
		return
	}

	var req codersdk.CreateWorkloadRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.Command == "" || len(req.Command) > maxWorkloadCommandBytes || strings.ContainsRune(req.Command, 0) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "A workload needs a command of at most 64 KiB without NUL bytes.",
		})
		return
	}
	scopes := make([]string, 0, len(req.Scopes))
	for _, scope := range req.Scopes {
		name := rbac.CanonicalScopeName(rbac.ScopeName(scope))
		if !rbac.IsExternalScope(name) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("%q is not a scope a workload can be given.", scope),
				Detail:  "Use the scope names API tokens accept, for example workspace:read or template:read.",
			})
			return
		}
		if !slices.Contains(scopes, string(name)) {
			scopes = append(scopes, string(name))
		}
	}

	// Workloads run inside a daemon the template declared: sub-agents are
	// themselves hosted by one.
	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	daemons := slices.DeleteFunc(agents, func(agent database.WorkspaceAgent) bool {
		return agent.ParentID.Valid || agent.Deleted || (req.Agent != "" && agent.Name != req.Agent)
	})
	switch {
	case len(daemons) == 0:
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "This workspace has no daemon to run the workload in. Is it started?",
		})
		return
	case len(daemons) > 1:
		names := make([]string, 0, len(daemons))
		for _, daemon := range daemons {
			names = append(names, daemon.Name)
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("This workspace has several daemons. Choose one of: %s.", strings.Join(names, ", ")),
		})
		return
	}
	daemon := daemons[0]
	if daemon.Status(dbtime.Now(), api.AgentInactiveDisconnectTimeout).Status != database.WorkspaceAgentStatusConnected {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("The workspace daemon %q is not connected.", daemon.Name),
		})
		return
	}

	existing, err := api.listWorkspaceWorkloads(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if len(existing) >= maxWorkloadsPerWorkspace {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("This workspace already runs %d workloads.", maxWorkloadsPerWorkspace),
		})
		return
	}
	id := uuid.New()
	name := req.Name
	if name == "" {
		name = "workload-" + id.String()[:8]
	}
	if !provisioner.AgentNameRegex.MatchString(name) || len(name) > 40 || name != strings.ToLower(name) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The workload name must be lowercase letters, digits and single hyphens, and at most 40 characters.",
		})
		return
	}
	if slices.ContainsFunc(existing, func(workload codersdk.Workload) bool { return workload.Name == name }) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("A workload named %q already runs in this workspace.", name),
		})
		return
	}

	//nolint:gocritic // The caller is the workspace owner, checked above.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	sponsor, err := api.Database.GetUserByID(sysCtx, apiKey.UserID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	// The identity exists before anything runs. It is assigned to the one
	// daemon that may vouch for it.
	identity, err := api.Database.InsertWorkloadIdentity(sysCtx, database.InsertWorkloadIdentityParams{
		ID:               id,
		CreatedAt:        dbtime.Now(),
		OrganizationID:   workspace.OrganizationID,
		SponsorID:        sponsor.ID,
		WorkspaceID:      workspace.ID,
		WorkspaceAgentID: daemon.ID,
		Name:             name,
		Command:          req.Command,
		Scopes:           scopes,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	state, err := api.startAgentWorkload(ctx, daemon.ID, codersdk.AgentWorkloadSpec{
		ID: identity.ID, Name: name, Command: req.Command, Image: req.Image,
	})
	if err != nil {
		if revokeErr := api.revokeWorkloadIdentity(ctx, identity.ID, "its sandbox did not start"); revokeErr != nil {
			api.Logger.Error(ctx, "revoke identity of a workload that did not start", slog.Error(revokeErr))
		}
		if xerrors.Is(err, errWorkloadsUnsupported) {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: fmt.Sprintf("The workspace daemon %q cannot run workloads.", daemon.Name),
				Detail:  "The template must start the agent with CODER_AGENT_WORKLOAD_RUNTIME set and give it a sandbox runtime such as Docker.",
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "The workspace daemon could not start the workload.",
			Detail:  err.Error(),
		})
		return
	}
	identity, err = api.Database.UpdateWorkloadIdentitySandbox(sysCtx, database.UpdateWorkloadIdentitySandboxParams{
		ID:        identity.ID,
		Runtime:   state.Sandbox.Runtime,
		SandboxID: state.Sandbox.ID,
		StartedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated,
		workloadFromIdentity(identity, daemon.Name, sponsor.Username, codersdk.WorkloadStatusRunning))
}

func (api *API) startAgentWorkload(ctx context.Context, agentID uuid.UUID, spec codersdk.AgentWorkloadSpec) (codersdk.AgentWorkloadState, error) {
	ctx, cancel := context.WithTimeout(ctx, workloadStartTimeout)
	defer cancel()
	res, release, err := api.workloadAgentRequest(ctx, agentID, http.MethodPost, "/", spec)
	if err != nil {
		return codersdk.AgentWorkloadState{}, err
	}
	defer release()
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return codersdk.AgentWorkloadState{}, readAgentWorkloadError(res)
	}
	var state codersdk.AgentWorkloadState
	if err := json.NewDecoder(res.Body).Decode(&state); err != nil {
		return codersdk.AgentWorkloadState{}, xerrors.Errorf("decode workload state: %w", err)
	}
	return state, nil
}

// @Summary List workloads (experimental)
// @ID list-workspace-workloads
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 200 {array} codersdk.Workload
// @Router /api/v2/workspaces/{workspace}/workloads [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceWorkloads(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, policy.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	workloads, err := api.listWorkspaceWorkloads(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, workloads)
}

// workloadParam returns the workload identity named in the URL, if it
// belongs to the workspace.
func (api *API) workloadParam(rw http.ResponseWriter, r *http.Request, workspaceID uuid.UUID) (database.WorkloadIdentity, bool) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "workload"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "The workload ID must be a UUID."})
		return database.WorkloadIdentity{}, false
	}
	//nolint:gocritic // The caller is authorized for the workspace.
	identity, err := api.Database.GetWorkloadIdentityByID(dbauthz.AsSystemRestricted(ctx), id)
	if err != nil || identity.WorkspaceID != workspaceID {
		httpapi.ResourceNotFound(rw)
		return database.WorkloadIdentity{}, false
	}
	return identity, true
}

// @Summary Follow a workload's output (experimental)
// @ID workspace-workload-logs
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param workload path string true "Workload ID" format(uuid)
// @Success 200 {object} codersdk.WorkloadLogEvent
// @Router /api/v2/workspaces/{workspace}/workloads/{workload}/logs [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceWorkloadLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, policy.ActionRead, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	identity, ok := api.workloadParam(rw, r, workspace.ID)
	if !ok {
		return
	}
	res, release, err := api.workloadAgentRequest(ctx, identity.WorkspaceAgentID, http.MethodGet, "/"+identity.ID.String()+"/logs", nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "The workspace daemon could not be reached.",
			Detail:  err.Error(),
		})
		return
	}
	defer release()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "The workload's output is no longer available.",
			Detail:  readAgentWorkloadError(res).Error(),
		})
		return
	}

	rw.Header().Set("Content-Type", "application/x-ndjson")
	rw.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(rw)
	exited := false
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if _, err := rw.Write(append(line, '\n')); err != nil {
			return
		}
		_ = controller.Flush()
		var event codersdk.WorkloadLogEvent
		if json.Unmarshal(line, &event) == nil && event.ExitCode != nil {
			exited = true
		}
	}
	// The command has finished, so nothing can act as the workload again.
	if exited && !identity.RevokedAt.Valid {
		//nolint:gocritic // The request may be canceled as soon as the stream ends.
		if err := api.revokeWorkloadIdentity(context.WithoutCancel(ctx), identity.ID, "its command exited"); err != nil {
			api.Logger.Error(ctx, "revoke identity of an exited workload", slog.Error(err))
		}
	}
}

// @Summary Remove a workload (experimental)
// @ID delete-workspace-workload
// @Security CoderSessionToken
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param workload path string true "Workload ID" format(uuid)
// @Success 204
// @Router /api/v2/workspaces/{workspace}/workloads/{workload} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteWorkspaceWorkload(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, policy.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	// Whatever its scopes, a workload cannot remove workloads.
	if isWorkloadCaller(r) {
		httpapi.Forbidden(rw)
		return
	}
	identity, ok := api.workloadParam(rw, r, workspace.ID)
	if !ok {
		return
	}
	// Revoking comes first: it holds even if the daemon cannot be reached.
	if err := api.revokeWorkloadIdentity(ctx, identity.ID, "it was removed"); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res, release, err := api.workloadAgentRequest(stopCtx, identity.WorkspaceAgentID, http.MethodDelete, "/"+identity.ID.String(), nil)
	if err != nil {
		api.Logger.Warn(ctx, "stop the sandbox of a removed workload", slog.F("workload_id", identity.ID), slog.Error(err))
	} else {
		_ = res.Body.Close()
		release()
	}
	rw.WriteHeader(http.StatusNoContent)
}

// workloadScope is the RBAC scope of a workload identity. Unlike an API
// key, an identity without scopes can do nothing.
type workloadScope struct {
	identityID uuid.UUID
	scopes     []string
}

func (s workloadScope) Name() rbac.RoleIdentifier {
	return rbac.RoleIdentifier{Name: "workload[" + s.identityID.String() + "]"}
}

func (s workloadScope) Expand() (rbac.Scope, error) {
	if len(s.scopes) == 0 {
		return rbac.Scope{
			Role:        rbac.Role{Identifier: s.Name()},
			AllowIDList: []rbac.AllowListElement{rbac.AllowListAll()},
		}, nil
	}
	scopes := make(database.APIKeyScopes, 0, len(s.scopes))
	for _, scope := range s.scopes {
		scopes = append(scopes, database.APIKeyScope(scope))
	}
	return scopes.WithAllowList(database.AllowList{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}}).Expand()
}

// vouchedWorkloadIdentity returns the workload identity a daemon vouches
// for. The daemon proves itself with its agent token. It can only vouch for
// an identity that coderd assigned to it and that has not been revoked.
// Nothing else selects the identity: not the workload, and not a user.
func (api *API) vouchedWorkloadIdentity(ctx context.Context, agentToken, identityID string) (database.WorkloadIdentity, database.WorkspaceAgent, error) {
	var noIdentity database.WorkloadIdentity
	var noAgent database.WorkspaceAgent
	if !api.Experiments.Enabled(codersdk.ExperimentRuntimeWorkloads) {
		return noIdentity, noAgent, xerrors.New("workloads are not enabled")
	}
	token, err := uuid.Parse(agentToken)
	if err != nil {
		return noIdentity, noAgent, xerrors.New("the credential is not an agent token")
	}
	id, err := uuid.Parse(identityID)
	if err != nil {
		return noIdentity, noAgent, xerrors.New("the workload identity is not a UUID")
	}
	//nolint:gocritic // Authentication happens before there is an actor.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	// This only returns agents of the latest build of their workspace.
	row, err := api.Database.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(sysCtx, token)
	if err != nil {
		return noIdentity, noAgent, xerrors.Errorf("authenticate workspace daemon: %w", err)
	}
	identity, err := api.Database.GetWorkloadIdentityByID(sysCtx, id)
	if err != nil {
		return noIdentity, noAgent, xerrors.Errorf("get workload identity: %w", err)
	}
	if identity.RevokedAt.Valid {
		return noIdentity, noAgent, xerrors.New("the workload identity is revoked")
	}
	if identity.WorkspaceAgentID != row.WorkspaceAgent.ID {
		return noIdentity, noAgent, xerrors.New("the workload identity is not assigned to this daemon")
	}
	return identity, row.WorkspaceAgent, nil
}

// resolveWorkloadAPIKey lets a workload call the user API through its
// workspace daemon. The workload acts as its sponsor, with the sponsor's
// permissions narrowed to the identity's scopes. No API key exists in the
// database: the returned key only carries the sponsor and the identity to
// the handlers.
func (api *API) resolveWorkloadAPIKey(ctx context.Context, agentToken, identityID string) (*httpmw.ValidateAPIKeyResult, bool) {
	identity, _, err := api.vouchedWorkloadIdentity(ctx, agentToken, identityID)
	if err != nil {
		return nil, false
	}
	subject, status, err := httpmw.UserRBACSubject(ctx, api.Database, identity.SponsorID, workloadScope{
		identityID: identity.ID, scopes: identity.Scopes,
	})
	if err != nil || status != database.UserStatusActive {
		return nil, false
	}
	now := dbtime.Now()
	return &httpmw.ValidateAPIKeyResult{
		Key: database.APIKey{
			ID:        workloadAPIKeyIDPrefix + identity.ID.String(),
			UserID:    identity.SponsorID,
			LoginType: database.LoginTypeToken,
			CreatedAt: identity.CreatedAt,
			UpdatedAt: now,
			LastUsed:  now,
			ExpiresAt: now.Add(time.Minute),
		},
		Subject:    subject,
		UserStatus: status,
	}, true
}

// @Summary Get the identity of the caller in a workspace (experimental)
// @ID get-workspace-agent-identity
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Success 200 {object} codersdk.WorkloadIdentity
// @Router /api/v2/workspaceagents/me/identity [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentIdentity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent := httpmw.WorkspaceAgent(r)

	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, agent.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	identity := codersdk.WorkloadIdentity{
		ID:            daemonIdentityName(workspace.ID, agent.ID),
		Kind:          codersdk.WorkloadIdentityKindDaemon,
		Attestation:   codersdk.WorkloadAttestationAgentToken,
		AgentID:       agent.ID,
		AgentName:     agent.Name,
		WorkspaceID:   workspace.ID,
		WorkspaceName: workspace.Name,
		OwnerID:       workspace.OwnerID,
		OwnerUsername: workspace.OwnerUsername,
	}
	// A daemon that forwards a workload's request names the workload. The
	// answer must then describe the workload and never the daemon.
	if vouched := r.Header.Get(codersdk.WorkloadIdentityHeader); vouched != "" {
		record, _, err := api.vouchedWorkloadIdentity(ctx, httpmw.APITokenFromRequest(r), vouched)
		if err != nil || record.WorkspaceAgentID != agent.ID {
			httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
				Message: "Workload identity not accepted.",
			})
			return
		}
		//nolint:gocritic // The workload reads its own identity.
		sponsor, err := api.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), record.SponsorID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		identity.ID = workloadIdentityName(workspace.ID, record.ID)
		identity.Kind = codersdk.WorkloadIdentityKindWorkload
		identity.Attestation = codersdk.WorkloadAttestationWorkspaceDaemon
		identity.IdentityID = record.ID
		identity.Name = record.Name
		identity.SponsorID = record.SponsorID
		identity.SponsorUsername = sponsor.Username
		identity.Scopes = workloadScopes(record.Scopes)
		if record.SandboxID != "" {
			identity.Sandbox = &codersdk.WorkloadSandbox{Runtime: record.Runtime, ID: record.SandboxID}
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, identity)
}
