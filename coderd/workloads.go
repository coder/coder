package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/workloads"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner"
)

// maxWorkloadsPerWorkspace bounds the runtime daemons of one workspace.
const maxWorkloadsPerWorkspace = 16

// maxWorkloadCommandBytes bounds the command a workload runs at start.
const maxWorkloadCommandBytes = 64 << 10

func (api *API) registerWorkspaceWorkloadRoutes(r chi.Router) {
	r.Route("/workloads", func(r chi.Router) {
		r.Use(httpmw.RequireExperiment(api.Experiments, codersdk.ExperimentRuntimeWorkloads))
		r.Post("/", api.postWorkspaceWorkload)
		r.Get("/", api.workspaceWorkloads)
		r.Delete("/{workload}", api.deleteWorkspaceWorkload)
	})
}

func (api *API) registerWorkloadOperatorRoutes(r chi.Router) {
	r.Route("/workload-operators/{operator}", func(r chi.Router) {
		r.Use(httpmw.RequireExperiment(api.Experiments, codersdk.ExperimentRuntimeWorkloads))
		r.Post("/sync", api.postWorkloadOperatorSync)
	})
}

// isRuntimeWorkloadAgent reports whether an agent row is a runtime daemon
// bound to a pod.
func isRuntimeWorkloadAgent(agent database.WorkspaceAgent) bool {
	return agent.ParentID.Valid && agent.AuthInstanceID.Valid &&
		strings.HasPrefix(agent.AuthInstanceID.String, workloads.PodInstanceIDPrefix)
}

func workloadPodMetadata(agent database.WorkspaceAgent) workloads.PodMetadata {
	var meta workloads.PodMetadata
	if agent.InstanceMetadata.Valid {
		// A row without readable metadata still has its pod UID in the
		// instance ID, so a decode failure only loses descriptive fields.
		_ = json.Unmarshal(agent.InstanceMetadata.RawMessage, &meta)
	}
	if meta.PodUID == "" {
		meta.PodUID = strings.TrimPrefix(agent.AuthInstanceID.String, workloads.PodInstanceIDPrefix)
	}
	return meta
}

// workloadIdentityID is the identity of a runtime daemon: its workspace and
// the pod it runs in.
func workloadIdentityID(workspaceID uuid.UUID, podUID string) string {
	return fmt.Sprintf("%s/pod/%s", workspaceID, podUID)
}

// declaredIdentityID is the identity of a template-declared daemon.
func declaredIdentityID(workspaceID, agentID uuid.UUID) string {
	return fmt.Sprintf("%s/agent/%s", workspaceID, agentID)
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

func (api *API) workloadFromAgent(agent database.WorkspaceAgent, identity database.WorkloadIdentity) codersdk.Workload {
	meta := workloadPodMetadata(agent)
	workspaceID := identity.WorkspaceID
	status := codersdk.WorkloadStatusDisconnected
	switch agent.Status(dbtime.Now(), api.AgentInactiveDisconnectTimeout).Status {
	case database.WorkspaceAgentStatusConnecting:
		status = codersdk.WorkloadStatusStarting
	case database.WorkspaceAgentStatusConnected:
		status = codersdk.WorkloadStatusRunning
	case database.WorkspaceAgentStatusDisconnected, database.WorkspaceAgentStatusTimeout:
	}
	createdAt := meta.RequestedAt
	if createdAt.IsZero() {
		createdAt = agent.CreatedAt
	}
	return codersdk.Workload{
		ID:                  agent.ID,
		Name:                agent.Name,
		WorkspaceID:         workspaceID,
		Shape:               meta.Shape,
		Status:              status,
		Command:             meta.Command,
		CreatedAt:           createdAt,
		RequestedByID:       meta.RequestedByID,
		RequestedByUsername: meta.RequestedByUsername,
		Identity:            workloadIdentityID(workspaceID, meta.PodUID),
		Pod: &codersdk.WorkloadPod{
			Namespace:      meta.Namespace,
			Name:           meta.PodName,
			UID:            meta.PodUID,
			ServiceAccount: meta.ServiceAccount,
		},
		IdentityID:      identity.ID,
		SponsorID:       identity.SponsorID,
		SponsorUsername: meta.RequestedByUsername,
		Scopes:          workloadScopes(identity.Scopes),
	}
}

func workloadFromRequest(req workloads.Request, shapeName string) codersdk.Workload {
	return codersdk.Workload{
		ID:                  req.ID,
		Name:                req.Name,
		WorkspaceID:         req.WorkspaceID,
		Shape:               shapeName,
		Status:              codersdk.WorkloadStatusPending,
		Command:             req.Command,
		CreatedAt:           req.CreatedAt,
		RequestedByID:       req.RequestedByID,
		RequestedByUsername: req.RequestedByUsername,
		IdentityID:          req.IdentityID,
		SponsorID:           req.RequestedByID,
		SponsorUsername:     req.RequestedByUsername,
		Scopes:              workloadScopes(req.Scopes),
	}
}

// listWorkspaceWorkloads returns the workspace's runtime daemons and its
// pending requests.
func (api *API) listWorkspaceWorkloads(ctx context.Context, workspaceID uuid.UUID) ([]codersdk.Workload, error) {
	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace agents: %w", err)
	}
	names := make(map[uuid.UUID]string, len(agents))
	for _, agent := range agents {
		names[agent.ID] = agent.Name
	}

	// The caller is authorized for the workspace. Identities have no
	// user-facing permission of their own yet.
	//nolint:gocritic
	identities, err := api.Database.GetWorkloadIdentitiesByWorkspaceID(dbauthz.AsSystemRestricted(ctx), workspaceID)
	if err != nil {
		return nil, xerrors.Errorf("get workload identities: %w", err)
	}
	byAgent := make(map[uuid.UUID]database.WorkloadIdentity, len(identities))
	for _, identity := range identities {
		if identity.WorkspaceAgentID.Valid {
			byAgent[identity.WorkspaceAgentID.UUID] = identity
		}
	}

	result := []codersdk.Workload{}
	for _, agent := range agents {
		// A runtime daemon without a live identity has been revoked.
		if identity, ok := byAgent[agent.ID]; ok && isRuntimeWorkloadAgent(agent) {
			result = append(result, api.workloadFromAgent(agent, identity))
		}
	}
	for _, req := range api.workloads.Requests(func(req workloads.Request) bool {
		return req.WorkspaceID == workspaceID
	}) {
		result = append(result, workloadFromRequest(req, names[req.ShapeAgentID]))
	}
	slices.SortFunc(result, func(a, b codersdk.Workload) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})
	return result, nil
}

// @Summary Create a runtime workload (experimental)
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
	if apiKey.UserID != workspace.OwnerID || strings.HasPrefix(apiKey.ID, workloads.APIKeyIDPrefix) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Only the owner of a workspace can run workloads in it.",
		})
		return
	}

	var req codersdk.CreateWorkloadRequest
	if !httpapi.Read(ctx, rw, r, &req) {
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
	if len(req.Command) > maxWorkloadCommandBytes || strings.ContainsRune(req.Command, 0) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The command must be at most 64 KiB and contain no NUL bytes.",
		})
		return
	}

	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	latest := make(map[uuid.UUID]database.WorkspaceAgent, len(agents))
	for _, agent := range agents {
		latest[agent.ID] = agent
	}

	// Only shapes of the latest build can be cloned. A shape from an older
	// build belongs to compute that no longer exists.
	var shapes []workloads.Shape
	for _, shape := range api.workloads.ShapesForWorkspace(workspace.ID) {
		if _, ok := latest[shape.AgentID]; ok {
			shapes = append(shapes, shape)
		}
	}
	if req.Shape != "" {
		shapes = slices.DeleteFunc(shapes, func(shape workloads.Shape) bool {
			return shape.AgentName != req.Shape
		})
	}
	switch {
	case len(shapes) == 0:
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "This workspace has no workload shape served by a workload operator.",
			Detail:  "The template must declare a shape, the workspace must be started, and an operator must be running in its cluster.",
		})
		return
	case len(shapes) > 1:
		names := make([]string, 0, len(shapes))
		for _, shape := range shapes {
			names = append(names, shape.AgentName)
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("This workspace has several workload shapes. Choose one of: %s.", strings.Join(names, ", ")),
		})
		return
	}
	shape := shapes[0]

	existing, err := api.listWorkspaceWorkloads(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if len(existing) >= maxWorkloadsPerWorkspace {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("This workspace already has %d runtime workloads.", maxWorkloadsPerWorkspace),
		})
		return
	}

	id := uuid.New()
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s-%s", shape.AgentName, id.String()[:8])
	}
	// Pod names are derived from the name, so it must also be a DNS label.
	if !provisioner.AgentNameRegex.MatchString(name) || len(name) > 40 || name != strings.ToLower(name) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The workload name must be lowercase letters, digits and single hyphens, and at most 40 characters.",
		})
		return
	}
	taken := slices.ContainsFunc(agents, func(agent database.WorkspaceAgent) bool { return agent.Name == name }) ||
		slices.ContainsFunc(existing, func(workload codersdk.Workload) bool { return workload.Name == name })
	if taken {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("An agent named %q already exists in this workspace.", name),
		})
		return
	}

	//nolint:gocritic // The caller is the workspace owner, checked above.
	requester, err := api.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), apiKey.UserID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// The identity exists before anything runs. The operator's pod is bound
	// to it later.
	//nolint:gocritic // Identities have no user-facing permission of their own yet.
	identity, err := api.Database.InsertWorkloadIdentity(dbauthz.AsSystemRestricted(ctx), database.InsertWorkloadIdentityParams{
		ID:             uuid.New(),
		CreatedAt:      dbtime.Now(),
		OrganizationID: workspace.OrganizationID,
		SponsorID:      requester.ID,
		WorkspaceID:    workspace.ID,
		Name:           name,
		Scopes:         scopes,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	workloadReq := workloads.Request{
		ID:                  id,
		Name:                name,
		WorkspaceID:         workspace.ID,
		ShapeAgentID:        shape.AgentID,
		Command:             req.Command,
		RequestedByID:       requester.ID,
		RequestedByUsername: requester.Username,
		CreatedAt:           identity.CreatedAt,
		IdentityID:          identity.ID,
		Scopes:              identity.Scopes,
	}
	api.workloads.AddRequest(workloadReq)
	httpapi.Write(ctx, rw, http.StatusCreated, workloadFromRequest(workloadReq, shape.AgentName))
}

// @Summary List runtime workloads (experimental)
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
	result, err := api.listWorkspaceWorkloads(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, result)
}

// @Summary Delete a runtime workload (experimental)
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
	if strings.HasPrefix(httpmw.APIKey(r).ID, workloads.APIKeyIDPrefix) {
		httpapi.Forbidden(rw)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "workload"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "The workload ID must be a UUID."})
		return
	}

	//nolint:gocritic // The caller is authorized to update the workspace above.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	revokedAt := sql.NullTime{Valid: true, Time: dbtime.Now()}
	if req, ok := api.workloads.Request(id); ok && req.WorkspaceID == workspace.ID {
		if err := api.Database.RevokeWorkloadIdentityByID(sysCtx, database.RevokeWorkloadIdentityByIDParams{
			ID: req.IdentityID, RevokedAt: revokedAt,
		}); err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		api.workloads.TakeRequest(id)
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	idx := slices.IndexFunc(agents, func(agent database.WorkspaceAgent) bool {
		return agent.ID == id && isRuntimeWorkloadAgent(agent)
	})
	if idx < 0 {
		httpapi.ResourceNotFound(rw)
		return
	}
	// Revoking the identity and deleting the agent row stops the daemon
	// from authenticating; the operator removes its pod on the next sync.
	if err := api.Database.RevokeWorkloadIdentityByWorkspaceAgentID(sysCtx, database.RevokeWorkloadIdentityByWorkspaceAgentIDParams{
		WorkspaceAgentID: uuid.NullUUID{Valid: true, UUID: id}, RevokedAt: revokedAt,
	}); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if err := api.Database.DeleteWorkspaceSubAgentByID(sysCtx, id); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Sync a workload operator (experimental)
// @ID sync-workload-operator
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Organizations
// @Param organization path string true "Organization ID" format(uuid)
// @Param operator path string true "Operator name"
// @Param request body codersdk.WorkloadOperatorSyncRequest true "Operator state"
// @Success 200 {object} codersdk.WorkloadOperatorSyncResponse
// @Router /api/v2/organizations/{organization}/workload-operators/{operator}/sync [post]
// @x-apidocgen {"skip": true}
func (api *API) postWorkloadOperatorSync(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := httpmw.OrganizationParam(r)
	// An operator can vouch for the identity of every workload it runs, so
	// only principals that administer the organization may act as one.
	if !api.Authorize(r, policy.ActionUpdate, org) {
		httpapi.Forbidden(rw)
		return
	}
	name := chi.URLParam(r, "operator")
	if !provisioner.AgentNameRegex.MatchString(name) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "The operator name is invalid."})
		return
	}

	var req codersdk.WorkloadOperatorSyncRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	var keys jose.JSONWebKeySet
	if err := json.Unmarshal(req.JWKS, &keys); err != nil || len(keys.Keys) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "The cluster's token signing keys are missing or unreadable."})
		return
	}
	if req.Namespace == "" || req.Issuer == "" || req.Audience == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "The namespace, issuer and audience are required."})
		return
	}

	operator := workloads.Operator{
		OrganizationID: org.ID,
		Name:           name,
		Namespace:      req.Namespace,
		Issuer:         req.Issuer,
		Audience:       req.Audience,
		Keys:           keys,
	}

	// Syncs are serialized so that two overlapping syncs cannot both create
	// the agent for one pod.
	api.workloadSync.Lock()
	defer api.workloadSync.Unlock()

	resp, err := api.syncWorkloadOperator(ctx, operator, req)
	if err != nil {
		api.Logger.Error(ctx, "sync workload operator", slog.F("operator", name), slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func (api *API) syncWorkloadOperator(ctx context.Context, operator workloads.Operator, req codersdk.WorkloadOperatorSyncRequest) (codersdk.WorkloadOperatorSyncResponse, error) {
	// The operator is authorized for its organization. Its sync reads and
	// writes agents of workspaces it does not own.
	//nolint:gocritic // Operators act on workspaces across owners.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	resp := codersdk.WorkloadOperatorSyncResponse{
		Shapes:    []codersdk.WorkloadOperatorShapeStatus{},
		Workloads: []codersdk.WorkloadOperatorWorkload{},
	}

	var shapes []workloads.Shape
	for _, reported := range req.Shapes {
		shape, err := api.resolveWorkloadShape(sysCtx, operator, reported)
		var rejected *shapeRejectedError
		if errors.As(err, &rejected) {
			resp.Shapes = append(resp.Shapes, codersdk.WorkloadOperatorShapeStatus{
				Deployment: reported.Deployment,
				Error:      rejected.reason,
			})
			continue
		}
		if err != nil {
			// The operator deletes the pods of shapes that are missing
			// from the response, so a transient failure must fail the
			// whole sync instead of dropping a shape.
			return resp, err
		}
		shapes = append(shapes, shape)
		resp.Shapes = append(resp.Shapes, codersdk.WorkloadOperatorShapeStatus{
			Deployment:  shape.Deployment,
			AgentID:     shape.AgentID,
			AgentName:   shape.AgentName,
			WorkspaceID: shape.WorkspaceID,
		})
	}
	api.workloads.SyncOperator(operator, shapes)

	// live holds every pod the operator sees. created holds the pods it
	// created itself, by workload: only those can receive an identity.
	live := make(map[string]struct{}, len(req.Pods))
	created := make(map[uuid.UUID]codersdk.WorkloadOperatorPod, len(req.Pods))
	for _, pod := range req.Pods {
		if pod.UID == "" {
			continue
		}
		live[pod.UID] = struct{}{}
		if pod.Created {
			created[pod.WorkloadID] = pod
		}
	}

	bound := make(map[string]uuid.UUID)
	for _, shape := range shapes {
		shapeAgent, err := api.Database.GetWorkspaceAgentByID(sysCtx, shape.AgentID)
		if err != nil {
			return resp, xerrors.Errorf("get shape agent: %w", err)
		}
		children, err := api.Database.GetWorkspaceAgentsByParentID(sysCtx, shape.AgentID)
		if err != nil {
			return resp, xerrors.Errorf("get runtime agents: %w", err)
		}
		for _, child := range children {
			if !isRuntimeWorkloadAgent(child) {
				continue
			}
			uid := strings.TrimPrefix(child.AuthInstanceID.String, workloads.PodInstanceIDPrefix)
			if _, ok := live[uid]; !ok {
				// The pod this daemon was bound to is gone. Its identity
				// must not outlive it.
				if err := api.Database.RevokeWorkloadIdentityByWorkspaceAgentID(sysCtx, database.RevokeWorkloadIdentityByWorkspaceAgentIDParams{
					WorkspaceAgentID: uuid.NullUUID{Valid: true, UUID: child.ID},
					RevokedAt:        sql.NullTime{Valid: true, Time: dbtime.Now()},
				}); err != nil {
					return resp, xerrors.Errorf("revoke workload identity: %w", err)
				}
				if err := api.Database.DeleteWorkspaceSubAgentByID(sysCtx, child.ID); err != nil {
					return resp, xerrors.Errorf("delete runtime agent: %w", err)
				}
				api.Logger.Info(ctx, "removed runtime workload whose pod is gone",
					slog.F("workload_id", child.ID), slog.F("pod_uid", uid))
				continue
			}
			bound[uid] = child.ID
			resp.Workloads = append(resp.Workloads, codersdk.WorkloadOperatorWorkload{
				ID: child.ID, Name: child.Name, Deployment: shape.Deployment, WorkspaceID: shape.WorkspaceID, PodUID: uid,
			})
		}

		for _, pending := range api.workloads.Requests(func(r workloads.Request) bool {
			return r.ShapeAgentID == shape.AgentID
		}) {
			podUID := ""
			if pod, ok := created[pending.ID]; ok {
				if _, taken := bound[pod.UID]; taken {
					return resp, xerrors.Errorf("pod %s is already bound to another workload", pod.UID)
				}
				if err := api.insertWorkloadAgent(sysCtx, shapeAgent, shape, operator, pending, pod); err != nil {
					return resp, xerrors.Errorf("create runtime agent: %w", err)
				}
				api.workloads.TakeRequest(pending.ID)
				bound[pod.UID] = pending.ID
				podUID = pod.UID
				api.Logger.Info(ctx, "bound runtime workload to pod",
					slog.F("workload_id", pending.ID), slog.F("pod_uid", pod.UID), slog.F("pod", pod.Name))
			}
			resp.Workloads = append(resp.Workloads, codersdk.WorkloadOperatorWorkload{
				ID: pending.ID, Name: pending.Name, Deployment: shape.Deployment, WorkspaceID: shape.WorkspaceID, PodUID: podUID,
			})
		}
	}
	api.workloads.SetPods(operator.Key(), bound)
	return resp, nil
}

// shapeRejectedError means a Deployment is not a usable shape. Its workloads
// must not keep running.
type shapeRejectedError struct{ reason string }

func (e *shapeRejectedError) Error() string { return e.reason }

// resolveWorkloadShape checks that a Deployment the operator found is the
// shape of a workspace in the operator's organization. The shape agent's
// token, which only that workspace build's pod template holds, is the proof.
func (api *API) resolveWorkloadShape(sysCtx context.Context, operator workloads.Operator, reported codersdk.WorkloadOperatorShape) (workloads.Shape, error) {
	token, err := uuid.Parse(reported.AgentToken)
	if err != nil {
		return workloads.Shape{}, &shapeRejectedError{"the pod template has no valid agent token"}
	}
	row, err := api.Database.GetAuthenticatedWorkspaceAgentAndBuildByAuthToken(sysCtx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workloads.Shape{}, &shapeRejectedError{"the agent token does not belong to an agent of a current workspace build"}
		}
		return workloads.Shape{}, xerrors.Errorf("look up shape agent: %w", err)
	}
	if row.WorkspaceTable.OrganizationID != operator.OrganizationID {
		return workloads.Shape{}, &shapeRejectedError{"the workspace belongs to another organization"}
	}
	agent := row.WorkspaceAgent
	if agent.ParentID.Valid {
		return workloads.Shape{}, &shapeRejectedError{"a runtime agent cannot be a shape"}
	}
	var env map[string]string
	if agent.EnvironmentVariables.Valid {
		_ = json.Unmarshal(agent.EnvironmentVariables.RawMessage, &env)
	}
	if env[codersdk.WorkloadShapeEnv] != "true" {
		return workloads.Shape{}, &shapeRejectedError{fmt.Sprintf("the agent %q does not set %s=true", agent.Name, codersdk.WorkloadShapeEnv)}
	}
	return workloads.Shape{
		AgentID:     agent.ID,
		AgentName:   agent.Name,
		WorkspaceID: row.WorkspaceTable.ID,
		Deployment:  reported.Deployment,
	}, nil
}

// insertWorkloadAgent creates the workspace agent of a runtime daemon as a
// child of its shape and binds it to the pod the operator created. The
// daemon gets the shape's environment and scripts, plus its own command.
func (api *API) insertWorkloadAgent(
	sysCtx context.Context,
	shapeAgent database.WorkspaceAgent,
	shape workloads.Shape,
	operator workloads.Operator,
	req workloads.Request,
	pod codersdk.WorkloadOperatorPod,
) error {
	meta, err := json.Marshal(workloads.PodMetadata{
		Namespace:           operator.Namespace,
		PodName:             pod.Name,
		PodUID:              pod.UID,
		ServiceAccount:      pod.ServiceAccount,
		Operator:            operator.Name,
		Shape:               shape.AgentName,
		Command:             req.Command,
		RequestedByID:       req.RequestedByID,
		RequestedByUsername: req.RequestedByUsername,
		RequestedAt:         req.CreatedAt,
	})
	if err != nil {
		return xerrors.Errorf("marshal pod metadata: %w", err)
	}

	return api.Database.InTx(func(tx database.Store) error {
		now := dbtime.Now()
		_, err := tx.InsertWorkspaceAgent(sysCtx, database.InsertWorkspaceAgentParams{
			ID:         req.ID,
			ParentID:   uuid.NullUUID{Valid: true, UUID: shapeAgent.ID},
			CreatedAt:  now,
			UpdatedAt:  now,
			Name:       req.Name,
			ResourceID: shapeAgent.ResourceID,
			// The auth token is never handed out. A runtime daemon
			// authenticates with its pod token, which resolves to this row.
			AuthToken:                uuid.New(),
			AuthInstanceID:           sql.NullString{Valid: true, String: workloads.PodInstanceIDPrefix + pod.UID},
			Architecture:             shapeAgent.Architecture,
			EnvironmentVariables:     shapeAgent.EnvironmentVariables,
			OperatingSystem:          shapeAgent.OperatingSystem,
			Directory:                shapeAgent.Directory,
			InstanceMetadata:         pqtype.NullRawMessage{Valid: true, RawMessage: meta},
			ResourceMetadata:         pqtype.NullRawMessage{},
			ConnectionTimeoutSeconds: shapeAgent.ConnectionTimeoutSeconds,
			TroubleshootingURL:       shapeAgent.TroubleshootingURL,
			MOTDFile:                 shapeAgent.MOTDFile,
			DisplayApps:              shapeAgent.DisplayApps,
			DisplayOrder:             shapeAgent.DisplayOrder,
			// A workload gets none of its sponsor's data through the agent
			// API: no Git SSH key and no external auth tokens. What it may
			// do on the sponsor's behalf comes from its identity's scopes.
			APIKeyScope: database.AgentKeyScopeEnumNoUserData,
		})
		if err != nil {
			return xerrors.Errorf("insert agent: %w", err)
		}
		if _, err := tx.BindWorkloadIdentity(sysCtx, database.BindWorkloadIdentityParams{
			ID:                 req.IdentityID,
			WorkspaceAgentID:   uuid.NullUUID{Valid: true, UUID: req.ID},
			AttestationSubject: sql.NullString{Valid: true, String: workloads.PodInstanceIDPrefix + pod.UID},
			BoundAt:            sql.NullTime{Valid: true, Time: now},
		}); err != nil {
			return xerrors.Errorf("bind workload identity: %w", err)
		}

		sources, err := tx.GetWorkspaceAgentLogSourcesByAgentIDs(sysCtx, []uuid.UUID{shapeAgent.ID})
		if err != nil {
			return xerrors.Errorf("get shape log sources: %w", err)
		}
		scripts, err := tx.GetWorkspaceAgentScriptsByAgentIDs(sysCtx, []uuid.UUID{shapeAgent.ID})
		if err != nil {
			return xerrors.Errorf("get shape scripts: %w", err)
		}

		sourceParams := database.InsertWorkspaceAgentLogSourcesParams{
			WorkspaceAgentID: req.ID,
			CreatedAt:        now,
			ID:               nil,
			DisplayName:      nil,
			Icon:             nil,
		}
		// Scripts reference log sources by ID, so each copied script needs
		// the ID of its copied source.
		sourceIDs := make(map[uuid.UUID]uuid.UUID, len(sources))
		for _, source := range sources {
			id := uuid.New()
			sourceIDs[source.ID] = id
			sourceParams.ID = append(sourceParams.ID, id)
			sourceParams.DisplayName = append(sourceParams.DisplayName, source.DisplayName)
			sourceParams.Icon = append(sourceParams.Icon, source.Icon)
		}

		scriptParams := database.InsertWorkspaceAgentScriptsParams{
			WorkspaceAgentID: req.ID,
			CreatedAt:        now,
			LogSourceID:      nil,
			LogPath:          nil,
			Script:           nil,
			Cron:             nil,
			StartBlocksLogin: nil,
			RunOnStart:       nil,
			RunOnStop:        nil,
			TimeoutSeconds:   nil,
			DisplayName:      nil,
			ID:               nil,
			ResourceAddress:  nil,
			Dependencies:     nil,
		}
		addScript := func(logSourceID uuid.UUID, script database.GetWorkspaceAgentScriptsByAgentIDsRow) {
			scriptParams.ID = append(scriptParams.ID, uuid.New())
			scriptParams.LogSourceID = append(scriptParams.LogSourceID, logSourceID)
			scriptParams.LogPath = append(scriptParams.LogPath, script.LogPath)
			scriptParams.Script = append(scriptParams.Script, script.Script)
			scriptParams.Cron = append(scriptParams.Cron, script.Cron)
			scriptParams.StartBlocksLogin = append(scriptParams.StartBlocksLogin, script.StartBlocksLogin)
			scriptParams.RunOnStart = append(scriptParams.RunOnStart, script.RunOnStart)
			scriptParams.RunOnStop = append(scriptParams.RunOnStop, script.RunOnStop)
			scriptParams.TimeoutSeconds = append(scriptParams.TimeoutSeconds, script.TimeoutSeconds)
			scriptParams.DisplayName = append(scriptParams.DisplayName, script.DisplayName)
			scriptParams.ResourceAddress = append(scriptParams.ResourceAddress, script.ResourceAddress)
			dependencies := string(script.Dependencies)
			if dependencies == "" {
				dependencies = "[]"
			}
			scriptParams.Dependencies = append(scriptParams.Dependencies, dependencies)
		}
		for _, script := range scripts {
			logSourceID, ok := sourceIDs[script.LogSourceID]
			if !ok {
				return xerrors.Errorf("script %q has no log source", script.DisplayName)
			}
			addScript(logSourceID, script)
		}
		if req.Command != "" {
			commandSource := uuid.New()
			sourceParams.ID = append(sourceParams.ID, commandSource)
			sourceParams.DisplayName = append(sourceParams.DisplayName, "Workload command")
			sourceParams.Icon = append(sourceParams.Icon, "")
			addScript(commandSource, database.GetWorkspaceAgentScriptsByAgentIDsRow{
				Script:      req.Command,
				RunOnStart:  true,
				DisplayName: "Workload command",
			})
		}

		if len(sourceParams.ID) > 0 {
			if _, err := tx.InsertWorkspaceAgentLogSources(sysCtx, sourceParams); err != nil {
				return xerrors.Errorf("insert log sources: %w", err)
			}
		}
		if len(scriptParams.ID) > 0 {
			if _, err := tx.InsertWorkspaceAgentScripts(sysCtx, scriptParams); err != nil {
				return xerrors.Errorf("insert scripts: %w", err)
			}
		}
		return nil
	}, nil)
}

// resolveWorkload maps a pod token to the runtime agent bound to that pod
// and to its workload identity. Nothing the caller supplies besides the
// signed token selects either.
func (api *API) resolveWorkload(ctx context.Context, token string) (database.WorkspaceAgent, database.WorkloadIdentity, error) {
	var noAgent database.WorkspaceAgent
	var noIdentity database.WorkloadIdentity
	if !api.Experiments.Enabled(codersdk.ExperimentRuntimeWorkloads) {
		return noAgent, noIdentity, xerrors.New("runtime workloads are not enabled")
	}
	claims, operator, err := api.workloads.VerifyPodToken(token)
	if err != nil {
		return noAgent, noIdentity, err
	}
	agentID, operatorKey, ok := api.workloads.AgentForPod(claims.PodUID)
	if !ok || operatorKey != operator.Key() {
		return noAgent, noIdentity, xerrors.Errorf("pod %s/%s is not a workload this operator reported", claims.Namespace, claims.PodName)
	}

	//nolint:gocritic // Pod identity auth happens before any other auth.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	agent, err := api.Database.GetWorkspaceAgentByID(sysCtx, agentID)
	if err != nil {
		return noAgent, noIdentity, xerrors.Errorf("get runtime agent: %w", err)
	}
	if !isRuntimeWorkloadAgent(agent) || agent.AuthInstanceID.String != claims.InstanceID() {
		return noAgent, noIdentity, xerrors.New("the pod is not bound to this agent")
	}
	shape, shapeOperator, ok := api.workloads.Shape(agent.ParentID.UUID)
	if !ok || shapeOperator.Key() != operator.Key() || shape.AgentID != agent.ParentID.UUID {
		return noAgent, noIdentity, xerrors.New("the pod's shape is not served by the operator that vouches for it")
	}
	// The query skips revoked identities, so revoking one stops the daemon.
	identity, err := api.Database.GetWorkloadIdentityByWorkspaceAgentID(sysCtx, uuid.NullUUID{Valid: true, UUID: agent.ID})
	if err != nil {
		return noAgent, noIdentity, xerrors.Errorf("get workload identity: %w", err)
	}
	if identity.AttestationSubject.String != claims.InstanceID() {
		return noAgent, noIdentity, xerrors.New("the pod is not bound to this identity")
	}
	return agent, identity, nil
}

// resolveAgentPodToken maps a pod token to the auth token of the runtime
// agent bound to that pod. It is the only way a runtime daemon authenticates.
func (api *API) resolveAgentPodToken(ctx context.Context, token string) (uuid.UUID, error) {
	agent, _, err := api.resolveWorkload(ctx, token)
	if err != nil {
		return uuid.Nil, err
	}
	return agent.AuthToken, nil
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

// resolveWorkloadAPIKey lets a workload call the user API with its pod
// token. The workload acts as its sponsor, with the sponsor's permissions
// narrowed to the identity's scopes. No API key exists in the database: the
// returned key only carries the sponsor and the identity to the handlers.
func (api *API) resolveWorkloadAPIKey(ctx context.Context, token string) (*httpmw.ValidateAPIKeyResult, bool) {
	_, identity, err := api.resolveWorkload(ctx, token)
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
			ID:        workloads.APIKeyIDPrefix + identity.ID.String(),
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

// @Summary Get the workload identity of the calling agent (experimental)
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
		ID:            declaredIdentityID(workspace.ID, agent.ID),
		Kind:          codersdk.WorkloadIdentityKindDeclared,
		Attestation:   codersdk.WorkloadAttestationAgentToken,
		AgentID:       agent.ID,
		AgentName:     agent.Name,
		WorkspaceID:   workspace.ID,
		WorkspaceName: workspace.Name,
		OwnerID:       workspace.OwnerID,
		OwnerUsername: workspace.OwnerUsername,
	}
	if isRuntimeWorkloadAgent(agent) {
		meta := workloadPodMetadata(agent)
		identity.ID = workloadIdentityID(workspace.ID, meta.PodUID)
		identity.Kind = codersdk.WorkloadIdentityKindRuntime
		identity.Attestation = codersdk.WorkloadAttestationKubernetesPod
		identity.Shape = meta.Shape
		identity.RequestedByID = meta.RequestedByID
		identity.RequestedByUsername = meta.RequestedByUsername
		//nolint:gocritic // The agent reads its own identity.
		record, err := api.Database.GetWorkloadIdentityByWorkspaceAgentID(dbauthz.AsSystemRestricted(ctx), uuid.NullUUID{Valid: true, UUID: agent.ID})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		identity.IdentityID = record.ID
		identity.SponsorID = record.SponsorID
		identity.SponsorUsername = meta.RequestedByUsername
		identity.Scopes = workloadScopes(record.Scopes)
		identity.Pod = &codersdk.WorkloadPod{
			Namespace:      meta.Namespace,
			Name:           meta.PodName,
			UID:            meta.PodUID,
			ServiceAccount: meta.ServiceAccount,
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, identity)
}
