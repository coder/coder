package coderd

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/x/agentsandbox"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/provisioner"
	"github.com/coder/websocket"
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
	// finishedWorkloadsListed bounds how many finished workloads a listing
	// returns.
	finishedWorkloadsListed = 50
	// maxWorkloadOutputTailBytes bounds how much output is kept for a
	// finished workload.
	maxWorkloadOutputTailBytes = 16 << 10
)

func (api *API) registerWorkspaceWorkloadRoutes(r chi.Router) {
	r.Route("/workloads", func(r chi.Router) {
		r.Use(httpmw.RequireExperiment(api.Experiments, codersdk.ExperimentRuntimeWorkloads))
		r.Post("/", api.postWorkspaceWorkload)
		r.Get("/", api.workspaceWorkloads)
		r.Get("/{workload}/logs", api.workspaceWorkloadLogs)
		r.Get("/{workload}/attach", api.workspaceWorkloadAttach)
		r.Delete("/{workload}", api.deleteWorkspaceWorkload)
	})
}

// registerWorkloadRoutes adds the workloads of the caller, whichever
// workspace they run in.
func (api *API) registerWorkloadRoutes(r chi.Router) {
	r.Route("/workloads", func(r chi.Router) {
		r.Use(httpmw.RequireExperiment(api.Experiments, codersdk.ExperimentRuntimeWorkloads))
		r.Get("/", api.sponsoredWorkloads)
		r.Post("/", api.postWorkload)
	})
}

// @Summary List the workloads you sponsor (experimental)
// @ID list-sponsored-workloads
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param finished query bool false "Also list the most recent finished workloads"
// @Success 200 {array} codersdk.Workload
// @Router /api/v2/workloads [get]
// @x-apidocgen {"skip": true}
func (api *API) sponsoredWorkloads(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// A workload learns nothing about its sponsor's other workloads.
	if isWorkloadCaller(r) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{Message: "A workload cannot list workloads."})
		return
	}
	sponsor := httpmw.APIKey(r).UserID
	//nolint:gocritic // The caller's own workloads; identities have no permission of their own yet.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	identities, err := api.Database.GetWorkloadIdentitiesBySponsorID(sysCtx, sponsor)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if r.URL.Query().Get("finished") == "true" {
		finished, err := api.Database.GetFinishedWorkloadIdentitiesBySponsorID(sysCtx, database.GetFinishedWorkloadIdentitiesBySponsorIDParams{
			SponsorID: sponsor,
			LimitOpt:  finishedWorkloadsListed,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		identities = append(identities, finished...)
	}
	username := ""
	if user, err := api.Database.GetUserByID(sysCtx, sponsor); err == nil {
		username = user.Username
	}
	workspaces := make(map[uuid.UUID]string)
	agents := make(map[uuid.UUID]string)
	workloads := make([]codersdk.Workload, 0, len(identities))
	for _, identity := range identities {
		if _, ok := workspaces[identity.WorkspaceID]; !ok {
			// Workspaces that are gone still have their name.
			name := ""
			if workspace, err := api.Database.GetWorkspaceByID(sysCtx, identity.WorkspaceID); err == nil {
				name = workspace.Name
			}
			workspaces[identity.WorkspaceID] = name
		}
		if _, ok := agents[identity.WorkspaceAgentID.UUID]; !ok {
			name := ""
			if agent, err := api.Database.GetWorkspaceAgentByID(sysCtx, identity.WorkspaceAgentID.UUID); err == nil {
				name = agent.Name
			}
			agents[identity.WorkspaceAgentID.UUID] = name
		}
		// What coderd has on record. Whether a daemon still runs a live
		// workload is found out when its workspace's workloads are listed.
		status := codersdk.WorkloadStatusStarting
		if identity.StartedAt.Valid {
			status = codersdk.WorkloadStatusRunning
		}
		workload := workloadFromIdentity(identity, agents[identity.WorkspaceAgentID.UUID], username, status)
		workload.WorkspaceName = workspaces[identity.WorkspaceID]
		workloads = append(workloads, workload)
	}
	httpapi.Write(ctx, rw, http.StatusOK, workloads)
}

// registerAgentWorkloadRoutes adds what a workspace daemon asks and tells
// coderd about the workloads it runs. The daemon authenticates as itself.
func (api *API) registerAgentWorkloadRoutes(r chi.Router) {
	r.Route("/workloads", func(r chi.Router) {
		r.Use(httpmw.RequireExperiment(api.Experiments, codersdk.ExperimentRuntimeWorkloads))
		r.Get("/pending", api.workspaceAgentPendingWorkloads)
		r.Post("/{workload}/started", api.postWorkspaceAgentWorkloadStarted)
		r.Post("/{workload}/exit", api.postWorkspaceAgentWorkloadExit)
	})
}

// workloadPendingChannel is where a daemon is told that it has been given
// a workload.
func workloadPendingChannel(agentID uuid.UUID) string {
	return "workload_pending:" + agentID.String()
}

// workloadStartedChannel is where it is announced whether a workload was
// started. The message is empty, or the reason it was not.
func workloadStartedChannel(workloadID uuid.UUID) string {
	return "workload_started:" + workloadID.String()
}

// workloadFinishedChannel is where it is announced that a workload is over.
func workloadFinishedChannel(workloadID uuid.UUID) string {
	return "workload_finished:" + workloadID.String()
}

// maxPendingWorkloadsWait bounds how long a daemon's request for pending
// workloads is held.
const maxPendingWorkloadsWait = 30 * time.Second

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
	_, ok := httpmw.WorkloadIdentityID(httpmw.APIKey(r))
	return ok
}

// finishedWorkloadStatus is the status of a workload that is over.
func finishedWorkloadStatus(reason string) codersdk.WorkloadStatus {
	switch codersdk.WorkloadEndReason(reason) {
	case codersdk.WorkloadEndReasonExited:
		return codersdk.WorkloadStatusExited
	case codersdk.WorkloadEndReasonRemoved:
		return codersdk.WorkloadStatusRemoved
	case codersdk.WorkloadEndReasonStopped:
		return codersdk.WorkloadStatusStopped
	case codersdk.WorkloadEndReasonFailed:
		return codersdk.WorkloadStatusFailed
	default:
		return codersdk.WorkloadStatusLost
	}
}

// workloadFromIdentity converts a workload's record. status is only used
// for a live workload: a finished one's status is its end reason.
func workloadFromIdentity(identity database.WorkloadIdentity, agentName, sponsorUsername string, status codersdk.WorkloadStatus) codersdk.Workload {
	workload := codersdk.Workload{
		ID:              identity.ID,
		Name:            identity.Name,
		WorkspaceID:     identity.WorkspaceID,
		AgentID:         identity.WorkspaceAgentID.UUID,
		AgentName:       agentName,
		Status:          status,
		Command:         identity.Command,
		CreatedAt:       identity.CreatedAt,
		Identity:        workloadIdentityName(identity.WorkspaceID, identity.ID),
		SponsorID:       identity.SponsorID,
		SponsorUsername: sponsorUsername,
		Scopes:          workloadScopes(identity.Scopes),
		Image:           identity.Image,
		Network:         codersdk.WorkloadNetwork(identity.Network),
		OwnWorkspace:    identity.RemoveWorkspace || identity.UserID.Valid,
	}
	if identity.UserID.Valid {
		workload.UserID = &identity.UserID.UUID
	}
	for _, mount := range identity.Mounts {
		workload.Mounts = append(workload.Mounts, codersdk.ParseWorkloadMount(mount))
	}
	if identity.SandboxID != "" {
		workload.Sandbox = &codersdk.WorkloadSandbox{Runtime: identity.Runtime, ID: identity.SandboxID}
	}
	if identity.FinishedAt.Valid {
		workload.Status = finishedWorkloadStatus(identity.EndReason)
		workload.FinishedAt = &identity.FinishedAt.Time
	}
	if identity.ExitCode.Valid {
		code := int(identity.ExitCode.Int32)
		workload.ExitCode = &code
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
	target := "http://workspace-daemon:" + strconv.Itoa(workspacesdk.AgentHTTPAPIServerPort) + "/api/v0/workloads" + path
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
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

// workloadEnd is how a workload ended.
type workloadEnd struct {
	Reason     codersdk.WorkloadEndReason
	ExitCode   *int
	OutputTail string
	// ActorID is who ended it. The zero ID means nobody did: the workload
	// ended by itself, and the entry is recorded under its sponsor.
	ActorID uuid.UUID
}

// workloadAuditFields adds what an auditor needs to place a workload
// without following IDs.
func (api *API) workloadAuditFields(ctx context.Context, identity database.WorkloadIdentity, extra map[string]any) json.RawMessage {
	fields := map[string]any{"workload_identity_id": identity.ID.String()}
	//nolint:gocritic // Names for the audit log, not shown to a caller.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	if workspace, err := api.Database.GetWorkspaceByID(sysCtx, identity.WorkspaceID); err == nil {
		fields["workspace_name"] = workspace.Name
		fields["workspace_owner"] = workspace.OwnerUsername
	}
	if agent, err := api.Database.GetWorkspaceAgentByID(sysCtx, identity.WorkspaceAgentID.UUID); err == nil {
		fields["workspace_agent"] = agent.Name
	}
	for key, value := range extra {
		fields[key] = value
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return json.RawMessage("{}")
	}
	return raw
}

// finishWorkload records that a workload is over. From then on its identity
// authorizes nothing. It reports whether this call ended the workload: a
// workload ends once, whoever notices first.
func (api *API) finishWorkload(ctx context.Context, identity database.WorkloadIdentity, end workloadEnd) (database.WorkloadIdentity, bool, error) {
	//nolint:gocritic // Identities have no user-facing permission of their own yet.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	exitCode := sql.NullInt32{}
	if end.ExitCode != nil {
		//nolint:gosec // An exit code fits.
		exitCode = sql.NullInt32{Valid: true, Int32: int32(*end.ExitCode)}
	}
	tail := end.OutputTail
	if len(tail) > maxWorkloadOutputTailBytes {
		tail = tail[len(tail)-maxWorkloadOutputTailBytes:]
	}
	// Postgres text cannot hold NUL bytes or invalid UTF-8.
	tail = strings.ToValidUTF8(strings.ReplaceAll(tail, "\x00", ""), "?")

	finished, err := api.Database.FinishWorkloadIdentity(sysCtx, database.FinishWorkloadIdentityParams{
		ID:         identity.ID,
		FinishedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
		EndReason:  string(end.Reason),
		ExitCode:   exitCode,
		OutputTail: tail,
	})
	if errors.Is(err, sql.ErrNoRows) {
		// Already over. Keep what the daemon knows about the outcome if
		// the first notice did not have it.
		if end.ExitCode != nil || tail != "" {
			if err := api.Database.UpdateWorkloadIdentityOutcome(sysCtx, database.UpdateWorkloadIdentityOutcomeParams{
				ID: identity.ID, ExitCode: exitCode, OutputTail: tail,
			}); err != nil {
				return identity, false, xerrors.Errorf("update workload outcome: %w", err)
			}
		}
		return identity, false, nil
	}
	if err != nil {
		return identity, false, xerrors.Errorf("finish workload: %w", err)
	}
	api.Logger.Info(ctx, "workload is over", slog.F("workload_id", identity.ID), slog.F("reason", end.Reason))

	actor := end.ActorID
	if actor == uuid.Nil {
		actor = identity.SponsorID
	}
	extra := map[string]any{"reason": string(end.Reason), "ended_by_user": end.ActorID != uuid.Nil}
	if end.ExitCode != nil {
		extra["exit_code"] = *end.ExitCode
	}
	audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.WorkloadIdentity]{
		Audit:            *api.Auditor.Load(),
		Log:              api.Logger,
		UserID:           actor,
		OrganizationID:   identity.OrganizationID,
		Action:           database.AuditActionStop,
		Old:              identity,
		New:              finished,
		Status:           http.StatusOK,
		AdditionalFields: api.workloadAuditFields(ctx, identity, extra),
	})
	// Whoever follows the workload can stop waiting for its daemon.
	if err := api.Pubsub.Publish(workloadFinishedChannel(identity.ID), []byte{}); err != nil {
		api.Logger.Warn(ctx, "announce that a workload is over", slog.Error(err))
	}
	if finished.RemoveWorkspace {
		go api.removeWorkloadWorkspace(finished)
	}
	return finished, true, nil
}

// workloadWorkspaceGrace is how long a workspace that was made for one
// workload is kept after the workload is over, so that whoever follows the
// workload can read the end of its output from the daemon.
const workloadWorkspaceGrace = 3 * time.Second

// removeWorkloadWorkspace deletes the workspace that was made for a
// workload, as the workspace's owner: the workload's own identity when it
// has one, its sponsor otherwise. An identity that owned the workspace is
// retired when the workspace is gone.
func (api *API) removeWorkloadWorkspace(identity database.WorkloadIdentity) {
	ctx, cancel := context.WithTimeout(api.ctx, time.Minute)
	defer cancel()
	logger := api.Logger.With(slog.F("workload_id", identity.ID), slog.F("workspace_id", identity.WorkspaceID))
	select {
	case <-ctx.Done():
		return
	case <-time.After(workloadWorkspaceGrace):
	}
	ownerID := identity.SponsorID
	if identity.UserID.Valid {
		ownerID = identity.UserID.UUID
	}
	//nolint:gocritic // The owner's permissions are looked up to act as the owner.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	owner, _, err := httpmw.UserRBACSubject(sysCtx, api.Database, ownerID, rbac.ScopeAll)
	if err != nil {
		logger.Warn(ctx, "delete the workspace of a finished workload: get owner", slog.Error(err))
		return
	}
	ownerCtx := dbauthz.As(ctx, owner)
	workspace, err := api.Database.GetWorkspaceByID(ownerCtx, identity.WorkspaceID)
	if err != nil {
		logger.Warn(ctx, "delete the workspace of a finished workload: get workspace", slog.Error(err))
		return
	}
	if workspace.Deleted {
		return
	}
	_, err = api.postWorkspaceBuildsInternal(ownerCtx, database.APIKey{UserID: identity.SponsorID}, workspace,
		codersdk.CreateWorkspaceBuildRequest{Transition: codersdk.WorkspaceTransitionDelete},
		func(action policy.Action, object rbac.Objecter) bool {
			return api.HTTPAuth.Authorizer.Authorize(ownerCtx, owner, action, object.RBACObject()) == nil
		},
		audit.WorkspaceBuildBaggage{},
	)
	if err != nil {
		logger.Warn(ctx, "delete the workspace of a finished workload", slog.Error(err))
	}
}

// listWorkspaceWorkloads returns the workspace's live workloads, oldest
// first. It asks the daemons what still runs, and ends workloads that are
// gone without the daemon having said so.
func (api *API) listWorkspaceWorkloads(ctx context.Context, workspaceID uuid.UUID) ([]codersdk.Workload, error) {
	//nolint:gocritic // The caller is authorized for the workspace.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	identities, err := api.Database.GetWorkloadIdentitiesByWorkspaceID(sysCtx, workspaceID)
	if err != nil {
		return nil, xerrors.Errorf("get workload identities: %w", err)
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
	sponsorName := func(id uuid.UUID) (string, error) {
		if name, ok := sponsors[id]; ok {
			return name, nil
		}
		sponsor, err := api.Database.GetUserByID(sysCtx, id)
		if err != nil {
			return "", xerrors.Errorf("get sponsor: %w", err)
		}
		sponsors[id] = sponsor.Username
		return sponsor.Username, nil
	}
	result := []codersdk.Workload{}
	states := make(map[uuid.UUID]map[uuid.UUID]codersdk.AgentWorkloadState)
	unreachable := make(map[uuid.UUID]bool)
	now := dbtime.Now()
	for _, identity := range identities {
		agent, ok := latest[identity.WorkspaceAgentID.UUID]
		if !ok {
			// The daemon that ran the workload is gone, and with it the
			// only thing that could vouch for the identity.
			if _, _, err := api.finishWorkload(ctx, identity, workloadEnd{Reason: codersdk.WorkloadEndReasonStopped}); err != nil {
				return nil, err
			}
			continue
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
			case known:
				// The command has exited and the daemon's own report has
				// not arrived.
				if _, _, err := api.finishWorkload(ctx, identity, workloadEnd{
					Reason: codersdk.WorkloadEndReasonExited, ExitCode: state.ExitCode,
				}); err != nil {
					return nil, err
				}
				continue
			default:
				if _, _, err := api.finishWorkload(ctx, identity, workloadEnd{Reason: codersdk.WorkloadEndReasonLost}); err != nil {
					return nil, err
				}
				continue
			}
		}
		sponsor, err := sponsorName(identity.SponsorID)
		if err != nil {
			return nil, err
		}
		result = append(result, workloadFromIdentity(identity, agent.Name, sponsor, status))
	}
	return result, nil
}

// listFinishedWorkspaceWorkloads returns the workspace's most recent
// workloads that are over, newest first.
func (api *API) listFinishedWorkspaceWorkloads(ctx context.Context, workspaceID uuid.UUID) ([]codersdk.Workload, error) {
	//nolint:gocritic // The caller is authorized for the workspace.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	finished, err := api.Database.GetFinishedWorkloadIdentitiesByWorkspaceID(sysCtx, database.GetFinishedWorkloadIdentitiesByWorkspaceIDParams{
		WorkspaceID: workspaceID,
		LimitOpt:    finishedWorkloadsListed,
	})
	if err != nil {
		return nil, xerrors.Errorf("get finished workload identities: %w", err)
	}
	// The daemon that ran a workload may belong to an earlier build.
	sponsors := make(map[uuid.UUID]string)
	agents := make(map[uuid.UUID]string)
	result := make([]codersdk.Workload, 0, len(finished))
	for _, identity := range finished {
		if _, ok := sponsors[identity.SponsorID]; !ok {
			sponsor, err := api.Database.GetUserByID(sysCtx, identity.SponsorID)
			if err != nil {
				return nil, xerrors.Errorf("get sponsor: %w", err)
			}
			sponsors[identity.SponsorID] = sponsor.Username
		}
		if _, ok := agents[identity.WorkspaceAgentID.UUID]; !ok {
			name := ""
			if agent, err := api.Database.GetWorkspaceAgentByID(sysCtx, identity.WorkspaceAgentID.UUID); err == nil {
				name = agent.Name
			}
			agents[identity.WorkspaceAgentID.UUID] = name
		}
		result = append(result, workloadFromIdentity(identity, agents[identity.WorkspaceAgentID.UUID], sponsors[identity.SponsorID], ""))
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
	scopes, refusal := workloadRequestScopes(req)
	if refusal != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *refusal)
		return
	}
	daemon, status, refusal := api.workloadDaemon(ctx, workspace.ID, req.Agent)
	if refusal != nil {
		httpapi.Write(ctx, rw, status, *refusal)
		return
	}

	//nolint:gocritic // The caller is the workspace owner, checked above.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	// Only what coderd knows is looked at here. Asking the daemon would
	// wait for a connection to it.
	existing, err := api.Database.GetWorkloadIdentitiesByWorkspaceID(sysCtx, workspace.ID)
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
	if slices.ContainsFunc(existing, func(identity database.WorkloadIdentity) bool { return identity.Name == name }) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("A workload named %q already runs in this workspace.", name),
		})
		return
	}

	sponsor, err := api.Database.GetUserByID(sysCtx, apiKey.UserID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	mounts := make([]string, 0, len(req.Mounts))
	for _, mount := range req.Mounts {
		mounts = append(mounts, mount.String())
	}
	// The identity exists before anything runs. It is assigned to the one
	// daemon that may vouch for it, and says what was asked for.
	identity, err := api.Database.InsertWorkloadIdentity(sysCtx, database.InsertWorkloadIdentityParams{
		ID:               id,
		CreatedAt:        dbtime.Now(),
		OrganizationID:   workspace.OrganizationID,
		SponsorID:        sponsor.ID,
		WorkspaceID:      workspace.ID,
		WorkspaceAgentID: uuid.NullUUID{UUID: daemon.ID, Valid: true},
		UserID:           uuid.NullUUID{},
		Name:             name,
		Command:          req.Command,
		Scopes:           scopes,
		Image:            req.Image,
		Network:          string(req.Network),
		Mounts:           mounts,
		Interactive:      req.Interactive,
		RemoveWorkspace:  req.RemoveWorkspace,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	api.awaitWorkloadStart(rw, r, identity, daemon, sponsor)
}

// workloadRequestScopes checks what a workload is asked to run and returns
// the scopes it asks for, by their canonical names.
func workloadRequestScopes(req codersdk.CreateWorkloadRequest) ([]string, *codersdk.Response) {
	if req.Command == "" || len(req.Command) > maxWorkloadCommandBytes || strings.ContainsRune(req.Command, 0) {
		return nil, &codersdk.Response{
			Message: "A workload needs a command of at most 64 KiB without NUL bytes.",
		}
	}
	scopes := make([]string, 0, len(req.Scopes))
	for _, scope := range req.Scopes {
		name := rbac.CanonicalScopeName(rbac.ScopeName(scope))
		if !rbac.IsExternalScope(name) {
			return nil, &codersdk.Response{
				Message: fmt.Sprintf("%q is not a scope a workload can be given.", scope),
				Detail:  "Use the scope names API tokens accept, for example workspace:read or template:read.",
			}
		}
		// A workload exists to have less than a user account.
		if name == rbac.ScopeAll {
			return nil, &codersdk.Response{
				Message: "A workload cannot be given every permission.",
				Detail:  "Name the scopes it needs, for example workspace:read or template:read.",
			}
		}
		if !slices.Contains(scopes, string(name)) {
			scopes = append(scopes, string(name))
		}
	}
	return scopes, nil
}

// workloadDaemon picks the daemon of a workspace that runs its workloads.
// When there is none it returns why, with the status to answer with.
func (api *API) workloadDaemon(ctx context.Context, workspaceID uuid.UUID, agentName string) (database.WorkspaceAgent, int, *codersdk.Response) {
	var none database.WorkspaceAgent
	// Workloads run inside a daemon the template declared: sub-agents are
	// themselves hosted by one.
	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return none, http.StatusInternalServerError, &codersdk.Response{
			Message: "Internal error fetching the daemons of the workspace.",
			Detail:  err.Error(),
		}
	}
	daemons := slices.DeleteFunc(agents, func(agent database.WorkspaceAgent) bool {
		return agent.ParentID.Valid || agent.Deleted || (agentName != "" && agent.Name != agentName)
	})
	if len(daemons) > 1 {
		// A template can keep workloads away from the human's daemon by
		// giving them a daemon of their own. Pick the one that runs them.
		daemons = slices.DeleteFunc(daemons, func(agent database.WorkspaceAgent) bool {
			if agent.Status(dbtime.Now(), api.AgentInactiveDisconnectTimeout).Status != database.WorkspaceAgentStatusConnected {
				return true
			}
			_, err := api.agentWorkloadStates(ctx, agent.ID)
			return err != nil
		})
		if len(daemons) == 0 {
			return none, http.StatusConflict, &codersdk.Response{
				Message: "No daemon of this workspace can run workloads.",
				Detail:  "The template must start an agent with CODER_AGENT_WORKLOAD_RUNTIME set and give it a sandbox runtime such as Docker.",
			}
		}
	}
	switch {
	case len(daemons) == 0:
		return none, http.StatusConflict, &codersdk.Response{
			Message: "This workspace has no daemon to run the workload in. Is it started?",
		}
	case len(daemons) > 1:
		names := make([]string, 0, len(daemons))
		for _, daemon := range daemons {
			names = append(names, daemon.Name)
		}
		return none, http.StatusBadRequest, &codersdk.Response{
			Message: fmt.Sprintf("Several daemons of this workspace can run workloads. Choose one of: %s.", strings.Join(names, ", ")),
		}
	}
	daemon := daemons[0]
	// A daemon that has just been claimed for a new workspace may still be
	// on its way. One that has gone away is not.
	switch daemon.Status(dbtime.Now(), api.AgentInactiveDisconnectTimeout).Status {
	case database.WorkspaceAgentStatusConnected, database.WorkspaceAgentStatusConnecting:
	default:
		return none, http.StatusConflict, &codersdk.Response{
			Message: fmt.Sprintf("The workspace daemon %q is not connected.", daemon.Name),
		}
	}
	return daemon, http.StatusOK, nil
}

// awaitWorkloadStart hands a workload to its daemon, waits until the daemon
// has started it and answers the request with the outcome.
func (api *API) awaitWorkloadStart(rw http.ResponseWriter, r *http.Request, identity database.WorkloadIdentity, daemon database.WorkspaceAgent, sponsor database.User) {
	ctx := r.Context()
	//nolint:gocritic // The caller was authorized to run the workload.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	// What ran, where, for whom and with which permissions is recorded
	// whether or not the sandbox starts.
	auditStart := func(status int, started database.WorkloadIdentity) {
		audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.WorkloadIdentity]{
			Audit:            *api.Auditor.Load(),
			Log:              api.Logger,
			UserID:           sponsor.ID,
			RequestID:        httpmw.RequestID(r),
			OrganizationID:   identity.OrganizationID,
			IP:               r.RemoteAddr,
			UserAgent:        r.UserAgent(),
			Action:           database.AuditActionStart,
			New:              started,
			Status:           status,
			AdditionalFields: api.workloadAuditFields(ctx, started, nil),
		})
	}
	failed := func(status int, message, detail string) {
		auditStart(http.StatusBadGateway, identity)
		//nolint:gocritic // The caller may be gone; the workload must still be ended.
		if _, _, finishErr := api.finishWorkload(context.WithoutCancel(ctx), identity, workloadEnd{
			Reason: codersdk.WorkloadEndReasonFailed, OutputTail: detail,
		}); finishErr != nil {
			api.Logger.Error(ctx, "end a workload that did not start", slog.Error(finishErr))
		}
		httpapi.Write(ctx, rw, status, codersdk.Response{Message: message, Detail: detail})
	}

	// The daemon fetches the workload: it asks coderd for what it has been
	// given. coderd does not wait for a connection to the daemon, which a
	// daemon that has just started takes a moment to offer.
	outcome := make(chan string, 1)
	unsubscribe, err := api.Pubsub.Subscribe(workloadStartedChannel(identity.ID), func(_ context.Context, message []byte) {
		select {
		case outcome <- string(message):
		default:
		}
	})
	if err != nil {
		failed(http.StatusInternalServerError, "The workload could not be handed to the workspace daemon.", err.Error())
		return
	}
	defer unsubscribe()
	if err := api.Pubsub.Publish(workloadPendingChannel(daemon.ID), []byte{}); err != nil {
		failed(http.StatusInternalServerError, "The workload could not be handed to the workspace daemon.", err.Error())
		return
	}
	// A daemon without a runtime never fetches anything. Find that out
	// meanwhile, over the connection coderd has to daemons.
	unsupported := make(chan struct{}, 1)
	probeCtx, cancelProbe := context.WithCancel(ctx)
	defer cancelProbe()
	go func() {
		if _, err := api.agentWorkloadStates(probeCtx, daemon.ID); xerrors.Is(err, errWorkloadsUnsupported) {
			unsupported <- struct{}{}
		}
	}()

	timeout := time.NewTimer(workloadStartTimeout)
	defer timeout.Stop()
	// Look now and then as well, in case an announcement is lost.
	look := time.NewTicker(2 * time.Second)
	defer look.Stop()
	for {
		select {
		case <-ctx.Done():
			// The caller is gone. The daemon may still start the
			// workload, which then runs unattended.
			return
		case <-timeout.C:
			failed(http.StatusBadGateway, "The workspace daemon did not start the workload in time.", "")
			return
		case <-unsupported:
			auditStart(http.StatusConflict, identity)
			//nolint:gocritic // The caller may be gone; the workload must still be ended.
			if _, _, finishErr := api.finishWorkload(context.WithoutCancel(ctx), identity, workloadEnd{Reason: codersdk.WorkloadEndReasonFailed}); finishErr != nil {
				api.Logger.Error(ctx, "end a workload that did not start", slog.Error(finishErr))
			}
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: fmt.Sprintf("The workspace daemon %q cannot run workloads.", daemon.Name),
				Detail:  "The template must start the agent with CODER_AGENT_WORKLOAD_RUNTIME set and give it a sandbox runtime such as Docker.",
			})
			return
		case reason := <-outcome:
			if reason != "" {
				failed(http.StatusBadGateway, "The workspace daemon could not start the workload.", reason)
				return
			}
		case <-look.C:
		}
		started, err := api.Database.GetWorkloadIdentityByID(sysCtx, identity.ID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		// A command can be over before anybody has looked.
		if started.StartedAt.Valid || started.FinishedAt.Valid {
			auditStart(http.StatusCreated, started)
			httpapi.Write(ctx, rw, http.StatusCreated,
				workloadFromIdentity(started, daemon.Name, sponsor.Username, codersdk.WorkloadStatusRunning))
			return
		}
	}
}

// @Summary List workloads (experimental)
// @ID list-workspace-workloads
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param finished query bool false "Also list the most recent finished workloads"
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
	if r.URL.Query().Get("finished") == "true" {
		finished, err := api.listFinishedWorkspaceWorkloads(ctx, workspace.ID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		workloads = append(workloads, finished...)
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

	started := false
	controller := http.NewResponseController(rw)
	write := func(line []byte) bool {
		if !started {
			rw.Header().Set("Content-Type", "application/x-ndjson")
			rw.WriteHeader(http.StatusOK)
			started = true
		}
		if _, err := rw.Write(append(line, '\n')); err != nil {
			return false
		}
		_ = controller.Flush()
		return true
	}
	// recorded replays what was kept when the workload ended.
	recorded := func(identity database.WorkloadIdentity) {
		for _, line := range strings.Split(strings.TrimRight(identity.OutputTail, "\n"), "\n") {
			if identity.OutputTail == "" {
				break
			}
			raw, _ := json.Marshal(codersdk.WorkloadLogEvent{Output: line})
			if !write(raw) {
				return
			}
		}
		// Clients need a final event. Without a known exit code, say so
		// with the conventional -1.
		code := -1
		if identity.ExitCode.Valid {
			code = int(identity.ExitCode.Int32)
		}
		raw, _ := json.Marshal(codersdk.WorkloadLogEvent{ExitCode: &code})
		write(raw)
	}

	//nolint:gocritic // The caller is authorized for the workspace.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	// The end of a finished workload's output is on record. That is all
	// of it, unless there was more than is kept.
	complete := func(identity database.WorkloadIdentity) bool {
		return identity.FinishedAt.Valid && len(identity.OutputTail) < maxWorkloadOutputTailBytes
	}
	if complete(identity) {
		recorded(identity)
		return
	}

	// The daemon has the live output. A daemon that has just started
	// takes a moment to be reachable, and a short command can be over
	// before that. What the daemon reported is served then.
	type daemonResponse struct {
		res     *http.Response
		release func()
		err     error
	}
	dialCtx, cancelDial := context.WithCancel(ctx)
	defer cancelDial()
	dialed := make(chan daemonResponse, 1)
	go func() {
		//nolint:bodyclose // Whoever takes the response from the channel closes it.
		res, release, err := api.workloadAgentRequest(dialCtx, identity.WorkspaceAgentID.UUID, http.MethodGet, "/"+identity.ID.String()+"/logs", nil)
		dialed <- daemonResponse{res: res, release: release, err: err}
	}()
	abandon := func() {
		cancelDial()
		go func() {
			if late := <-dialed; late.err == nil {
				_ = late.res.Body.Close()
				late.release()
			}
		}()
	}
	over := make(chan struct{}, 1)
	if unsubscribe, err := api.Pubsub.Subscribe(workloadFinishedChannel(identity.ID), func(context.Context, []byte) {
		select {
		case over <- struct{}{}:
		default:
		}
	}); err == nil {
		defer unsubscribe()
	}
	// Look now and then as well, in case an announcement is lost.
	look := time.NewTicker(2 * time.Second)
	defer look.Stop()
	var response daemonResponse
waiting:
	for {
		if latest, err := api.Database.GetWorkloadIdentityByID(sysCtx, identity.ID); err == nil && complete(latest) {
			abandon()
			recorded(latest)
			return
		}
		select {
		case response = <-dialed:
			break waiting
		case <-over:
		case <-look.C:
		case <-ctx.Done():
			abandon()
			return
		}
	}
	if response.err != nil {
		if latest, err := api.Database.GetWorkloadIdentityByID(sysCtx, identity.ID); err == nil && latest.FinishedAt.Valid {
			recorded(latest)
			return
		}
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "The workspace daemon could not be reached.",
			Detail:  response.err.Error(),
		})
		return
	}
	res, release := response.res, response.release
	defer release()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		// The daemon no longer has the sandbox. What it reported when the
		// workload ended is on record.
		if latest, err := api.Database.GetWorkloadIdentityByID(sysCtx, identity.ID); err == nil && latest.FinishedAt.Valid {
			recorded(latest)
			return
		}
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: "The workload's output is not available.",
			Detail:  readAgentWorkloadError(res).Error(),
		})
		return
	}

	var exitCode *int
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		var event codersdk.WorkloadLogEvent
		if json.Unmarshal(line, &event) == nil && event.ExitCode != nil {
			exitCode = event.ExitCode
		}
		if !write(line) {
			return
		}
	}
	// The command has finished, so nothing may act as the workload again.
	// The daemon reports this too; whichever notice arrives first counts.
	if exitCode != nil {
		//nolint:gocritic // The request may be canceled as soon as the stream ends.
		if _, _, err := api.finishWorkload(context.WithoutCancel(ctx), identity, workloadEnd{
			Reason: codersdk.WorkloadEndReasonExited, ExitCode: exitCode,
		}); err != nil {
			api.Logger.Error(ctx, "end an exited workload", slog.Error(err))
		}
	}
}

// @Summary Attach to a workload's terminal (experimental)
// @ID workspace-workload-attach
// @Security CoderSessionToken
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Param workload path string true "Workload ID" format(uuid)
// @Param height query int false "Terminal height"
// @Param width query int false "Terminal width"
// @Success 101
// @Router /api/v2/workspaces/{workspace}/workloads/{workload}/attach [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceWorkloadAttach(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, policy.ActionSSH, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	// Whoever types into a workload's terminal acts as the workload, so
	// only its sponsor may. The terminal is inside the sandbox: it gives
	// the human no more than the workload has, and the workload nothing of
	// the human's.
	identity, ok := api.workloadParam(rw, r, workspace.ID)
	if !ok {
		return
	}
	apiKey := httpmw.APIKey(r)
	if (apiKey.UserID != workspace.OwnerID && apiKey.UserID != identity.SponsorID) || isWorkloadCaller(r) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the sponsor of a workload can attach to it.",
		})
		return
	}
	if identity.FinishedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "The workload is over."})
		return
	}

	agentConn, release, err := api.agentProvider.AgentConn(ctx, identity.WorkspaceAgentID.UUID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "The workspace daemon could not be reached.",
			Detail:  err.Error(),
		})
		return
	}
	defer release()
	query := url.Values{}
	query.Set("height", r.URL.Query().Get("height"))
	query.Set("width", r.URL.Query().Get("width"))
	// The connection's HTTP client dials the daemon whatever the host is.
	target := "http://workspace-daemon:" + strconv.Itoa(workspacesdk.AgentHTTPAPIServerPort) +
		"/api/v0/workloads/" + identity.ID.String() + "/attach?" + query.Encode()
	//nolint:bodyclose // The websocket owns the response body.
	agentWS, res, err := websocket.Dial(ctx, target, &websocket.DialOptions{
		HTTPClient:      agentConn.AppHTTPClient(),
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		detail := err.Error()
		if res != nil {
			detail = readAgentWorkloadError(res).Error()
		}
		httpapi.Write(ctx, rw, http.StatusBadGateway, codersdk.Response{
			Message: "The workload's terminal could not be reached.",
			Detail:  detail,
		})
		return
	}
	defer agentWS.Close(websocket.StatusNormalClosure, "")

	clientWS, err := websocket.Accept(rw, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer clientWS.Close(websocket.StatusNormalClosure, "")

	// That a person had a terminal in the workload belongs in its record.
	audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.WorkloadIdentity]{
		Audit:            *api.Auditor.Load(),
		Log:              api.Logger,
		UserID:           apiKey.UserID,
		RequestID:        httpmw.RequestID(r),
		OrganizationID:   identity.OrganizationID,
		IP:               r.RemoteAddr,
		UserAgent:        r.UserAgent(),
		Action:           database.AuditActionConnect,
		Old:              identity,
		New:              identity,
		Status:           http.StatusSwitchingProtocols,
		AdditionalFields: api.workloadAuditFields(ctx, identity, nil),
	})

	ctx, clientConn := workspaceapps.WebsocketNetConn(ctx, clientWS, websocket.MessageBinary)
	defer clientConn.Close()
	agentNetConn := websocket.NetConn(ctx, agentWS, websocket.MessageBinary)
	defer agentNetConn.Close()
	agentssh.Bicopy(ctx, clientConn, agentNetConn)
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
	if !api.Authorize(r, policy.ActionRead, workspace) {
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
	// A sponsor answers for the workload and may end it, also where the
	// workload's identity owns the workspace and the sponsor only shares it.
	if httpmw.APIKey(r).UserID != identity.SponsorID && !api.Authorize(r, policy.ActionUpdate, workspace) {
		httpapi.ResourceNotFound(rw)
		return
	}
	// A workspace that an identity owns exists for that one workload. A
	// sponsor who removes the workload removes it, also when it was kept:
	// the sponsor shares the workspace and has no other way to delete it.
	removeKept := identity.UserID.Valid && !identity.RemoveWorkspace
	if identity.FinishedAt.Valid {
		if removeKept {
			go api.removeWorkloadWorkspace(identity)
		}
		rw.WriteHeader(http.StatusNoContent)
		return
	}
	// Ending the identity comes first: it holds even if the daemon cannot
	// be reached.
	if _, _, err := api.finishWorkload(ctx, identity, workloadEnd{
		Reason: codersdk.WorkloadEndReasonRemoved, ActorID: httpmw.APIKey(r).UserID,
	}); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if removeKept {
		go api.removeWorkloadWorkspace(identity)
	}
	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res, release, err := api.workloadAgentRequest(stopCtx, identity.WorkspaceAgentID.UUID, http.MethodDelete, "/"+identity.ID.String(), nil)
	if err != nil {
		api.Logger.Warn(ctx, "stop the sandbox of a removed workload", slog.F("workload_id", identity.ID), slog.Error(err))
	} else {
		_ = res.Body.Close()
		release()
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Get the workloads a daemon has been given (experimental)
// @ID get-workspace-agent-pending-workloads
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param wait_seconds query int false "How long to hold the request when there is none"
// @Success 200 {array} codersdk.AgentWorkloadSpec
// @Router /api/v2/workspaceagents/me/workloads/pending [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentPendingWorkloads(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent := httpmw.WorkspaceAgent(r)
	seconds, _ := strconv.Atoi(r.URL.Query().Get("wait_seconds"))
	wait := min(time.Duration(max(seconds, 0))*time.Second, maxPendingWorkloadsWait)

	given := make(chan struct{}, 1)
	// Listening comes before looking, so that nothing is missed in between.
	unsubscribe, err := api.Pubsub.Subscribe(workloadPendingChannel(agent.ID), func(context.Context, []byte) {
		select {
		case given <- struct{}{}:
		default:
		}
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	defer unsubscribe()
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	// Look now and then as well, in case an announcement is lost.
	look := time.NewTicker(5 * time.Second)
	defer look.Stop()
	var identities []database.WorkloadIdentity
waiting:
	for {
		//nolint:gocritic // The daemon is authenticated; identities have no permission of their own yet.
		identities, err = api.Database.GetPendingWorkloadIdentitiesByWorkspaceAgentID(dbauthz.AsSystemRestricted(ctx),
			database.GetPendingWorkloadIdentitiesByWorkspaceAgentIDParams{
				WorkspaceAgentID: uuid.NullUUID{UUID: agent.ID, Valid: true},
				CreatedAfter:     dbtime.Now().Add(-workloadStartTimeout),
			})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		if len(identities) > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			break waiting
		case <-given:
		case <-look.C:
		}
	}
	specs := make([]codersdk.AgentWorkloadSpec, 0, len(identities))
	for _, identity := range identities {
		spec := codersdk.AgentWorkloadSpec{
			ID:          identity.ID,
			Name:        identity.Name,
			Command:     identity.Command,
			Image:       identity.Image,
			Network:     codersdk.WorkloadNetwork(identity.Network),
			Interactive: identity.Interactive,
		}
		for _, mount := range identity.Mounts {
			spec.Mounts = append(spec.Mounts, codersdk.ParseWorkloadMount(mount))
		}
		specs = append(specs, spec)
	}
	httpapi.Write(ctx, rw, http.StatusOK, specs)
}

// @Summary Report whether a workload was started (experimental)
// @ID post-workspace-agent-workload-started
// @Security CoderSessionToken
// @Accept json
// @Tags Agents
// @Param workload path string true "Workload ID" format(uuid)
// @Param request body codersdk.AgentWorkloadStart true "How starting the workload went"
// @Success 204
// @Router /api/v2/workspaceagents/me/workloads/{workload}/started [post]
// @x-apidocgen {"skip": true}
func (api *API) postWorkspaceAgentWorkloadStarted(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent := httpmw.WorkspaceAgent(r)
	id, err := uuid.Parse(chi.URLParam(r, "workload"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "The workload ID must be a UUID."})
		return
	}
	var req codersdk.AgentWorkloadStart
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	//nolint:gocritic // The daemon is authenticated; identities have no permission of their own yet.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	// A daemon reports only on workloads coderd gave it.
	identity, err := api.Database.GetWorkloadIdentityByID(sysCtx, id)
	if err != nil || identity.WorkspaceAgentID.UUID != agent.ID {
		httpapi.ResourceNotFound(rw)
		return
	}
	reason := req.Error
	switch {
	case req.State != nil:
		reason = ""
		mounts := make([]string, 0, len(req.State.Mounts))
		for _, mount := range req.State.Mounts {
			mounts = append(mounts, mount.String())
		}
		// No row means the workload is over already, which is fine.
		_, err = api.Database.UpdateWorkloadIdentitySandbox(sysCtx, database.UpdateWorkloadIdentitySandboxParams{
			ID:        identity.ID,
			Runtime:   req.State.Sandbox.Runtime,
			SandboxID: req.State.Sandbox.ID,
			Image:     req.State.Image,
			Network:   string(req.State.Network),
			Mounts:    mounts,
			StartedAt: sql.NullTime{Valid: true, Time: dbtime.Now()},
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return
		}
	case reason == "":
		reason = "the workspace daemon did not say why"
	}
	// Whoever asked for the workload is waiting to hear, and records the
	// outcome.
	if err := api.Pubsub.Publish(workloadStartedChannel(identity.ID), []byte(reason)); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Report that a workload is over (experimental)
// @ID post-workspace-agent-workload-exit
// @Security CoderSessionToken
// @Accept json
// @Tags Agents
// @Param workload path string true "Workload ID" format(uuid)
// @Param request body codersdk.AgentWorkloadExit true "How the workload ended"
// @Success 204
// @Router /api/v2/workspaceagents/me/workloads/{workload}/exit [post]
// @x-apidocgen {"skip": true}
func (api *API) postWorkspaceAgentWorkloadExit(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent := httpmw.WorkspaceAgent(r)
	id, err := uuid.Parse(chi.URLParam(r, "workload"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "The workload ID must be a UUID."})
		return
	}
	var req codersdk.AgentWorkloadExit
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	// A daemon reports only on workloads coderd gave it.
	//nolint:gocritic // The daemon is authenticated; identities have no permission of their own yet.
	identity, err := api.Database.GetWorkloadIdentityByID(dbauthz.AsSystemRestricted(ctx), id)
	if err != nil || identity.WorkspaceAgentID.UUID != agent.ID {
		httpapi.ResourceNotFound(rw)
		return
	}
	reason := req.Reason
	switch reason {
	case codersdk.WorkloadEndReasonExited, codersdk.WorkloadEndReasonStopped, codersdk.WorkloadEndReasonLost:
	default:
		// Removal and failure to start are for coderd to say.
		reason = codersdk.WorkloadEndReasonLost
	}
	if _, _, err := api.finishWorkload(ctx, identity, workloadEnd{
		Reason: reason, ExitCode: req.ExitCode, OutputTail: req.OutputTail,
	}); err != nil {
		httpapi.InternalServerError(rw, err)
		return
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
// an identity that coderd assigned to it and whose workload is not over.
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
	if identity.FinishedAt.Valid {
		return noIdentity, noAgent, xerrors.New("the workload is over")
	}
	if identity.WorkspaceAgentID.UUID != row.WorkspaceAgent.ID {
		return noIdentity, noAgent, xerrors.New("the workload identity is not assigned to this daemon")
	}
	return identity, row.WorkspaceAgent, nil
}

// resolveWorkloadAPIKey lets a workload call the user API through its
// workspace daemon. A workload with a workspace to itself acts as its own
// service account, narrowed to the identity's scopes: it has nothing of its
// sponsor's. A workload in a sandbox inside its sponsor's workspace acts as
// its sponsor, narrowed to the identity's scopes. No API key exists in the
// database: the returned key only carries the account and the identity to
// the handlers.
func (api *API) resolveWorkloadAPIKey(ctx context.Context, agentToken, identityID string) (*httpmw.ValidateAPIKeyResult, bool) {
	identity, _, err := api.vouchedWorkloadIdentity(ctx, agentToken, identityID)
	if err != nil {
		return nil, false
	}
	userID := identity.SponsorID
	if identity.UserID.Valid {
		userID = identity.UserID.UUID
	}
	subject, status, err := httpmw.UserRBACSubject(ctx, api.Database, userID, workloadScope{
		identityID: identity.ID, scopes: identity.Scopes,
	})
	if err != nil || status != database.UserStatusActive {
		return nil, false
	}
	now := dbtime.Now()
	return &httpmw.ValidateAPIKeyResult{
		Key: database.APIKey{
			ID:        httpmw.WorkloadAPIKeyIDPrefix + identity.ID.String(),
			UserID:    userID,
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
	// A daemon in a pod from a pool was never given a token. It is the
	// daemon of this workspace because the cluster said which pod it is.
	attestedPod := strings.HasPrefix(agent.AuthInstanceID.String, agentsandbox.InstanceIDPrefix)
	if attestedPod {
		identity.Attestation = codersdk.WorkloadAttestationKubernetesPod
		identity.AttestationSubject = agent.AuthInstanceID.String
	}
	// A daemon that forwards a workload's request names the workload. The
	// answer must then describe the workload and never the daemon.
	if vouched := r.Header.Get(codersdk.WorkloadIdentityHeader); vouched != "" {
		record, _, err := api.vouchedWorkloadIdentity(ctx, httpmw.APITokenFromRequest(r), vouched)
		if err != nil || record.WorkspaceAgentID.UUID != agent.ID {
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
		// The daemon vouches for the workload. Where the cluster vouches
		// for the daemon's pod, and the workload has that pod to itself,
		// the workload's identity rests on the cluster's word too.
		if !attestedPod {
			identity.Attestation = codersdk.WorkloadAttestationWorkspaceDaemon
		}
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
