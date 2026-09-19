package coderd

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
)

// @Summary Create exit node
// @ID create-exit-node
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateExitNodeRequest true "Create exit node request"
// @Success 201 {object} codersdk.CreateExitNodeResponse
// @Router /api/v2/organizations/{organization}/exitnodes [post]
func (api *API) postExitNode(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		organization      = httpmw.OrganizationParam(r)
		aReq, commitAudit = api.exitNodeAudit(rw, r, database.AuditActionCreate)
		req               codersdk.CreateExitNodeRequest
	)
	defer commitAudit()
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	id := uuid.New()
	fullToken, hashedSecret, err := generateWorkspaceProxyToken(id)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	now := dbtime.Now()
	node, err := api.Database.InsertExitNode(ctx, database.InsertExitNodeParams{
		ID:                id,
		OrganizationID:    organization.ID,
		Name:              req.Name,
		DisplayName:       req.DisplayName,
		TokenHashedSecret: hashedSecret,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if database.IsUniqueViolation(err, database.UniqueExitNodesOrganizationIDLowerNameIndex) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Exit node with name %q already exists in organization.", req.Name),
		})
		return
	}
	if writeExitNodeError(rw, err) {
		return
	}

	aReq.New = node
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateExitNodeResponse{
		ExitNode: convertExitNode(node, nil, now),
		Token:    fullToken,
	})
}

// @Summary List exit nodes
// @ID list-exit-nodes
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {array} codersdk.ExitNode
// @Router /api/v2/organizations/{organization}/exitnodes [get]
func (api *API) exitNodes(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nodes, err := api.Database.GetExitNodesByOrganization(ctx, httpmw.OrganizationParam(r).ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	now := dbtime.Now()
	converted := make([]codersdk.ExitNode, 0, len(nodes))
	// This is intentionally an N+1 query until the database exposes a bulk
	// replica lookup that preserves exit node ordering.
	for _, node := range nodes {
		replicas, err := api.Database.GetExitNodeReplicasByExitNode(ctx, node.ID)
		if err != nil {
			httpapi.InternalServerError(rw, xerrors.Errorf("get exit node replicas: %w", err))
			return
		}
		converted = append(converted, convertExitNode(node, replicas, now))
	}
	httpapi.Write(ctx, rw, http.StatusOK, converted)
}

// @Summary Get exit node
// @ID get-exit-node
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param exitnode path string true "Exit node ID or name"
// @Success 200 {object} codersdk.ExitNode
// @Router /api/v2/organizations/{organization}/exitnodes/{exitnode} [get]
func (api *API) exitNode(rw http.ResponseWriter, r *http.Request) {
	node, ok := api.exitNodeParam(rw, r)
	if !ok {
		return
	}
	replicas, err := api.Database.GetExitNodeReplicasByExitNode(r.Context(), node.ID)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get exit node replicas: %w", err))
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, convertExitNode(node, replicas, dbtime.Now()))
}

// @Summary Delete exit node
// @ID delete-exit-node
// @Security CoderSessionToken
// @Tags Enterprise
// @Param organization path string true "Organization ID" format(uuid)
// @Param exitnode path string true "Exit node ID or name"
// @Success 204
// @Router /api/v2/organizations/{organization}/exitnodes/{exitnode} [delete]
func (api *API) deleteExitNode(rw http.ResponseWriter, r *http.Request) {
	aReq, commitAudit := api.exitNodeAudit(rw, r, database.AuditActionDelete)
	defer commitAudit()

	node, ok := api.exitNodeParam(rw, r)
	if !ok {
		return
	}
	aReq.Old = node
	if writeExitNodeError(rw, api.Database.DeleteExitNodeByID(r.Context(), node.ID)) {
		return
	}
	aReq.New = database.ExitNode{}
	rw.WriteHeader(http.StatusNoContent)
}

// exitNodeAudit starts an audit request for an exit node mutation in the
// organization from ExtractOrganizationParam.
func (api *API) exitNodeAudit(rw http.ResponseWriter, r *http.Request, action database.AuditAction) (*audit.Request[database.ExitNode], func()) {
	return audit.InitRequest[database.ExitNode](rw, &audit.RequestParams{
		Audit:          *api.AGPL.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         action,
		OrganizationID: httpmw.OrganizationParam(r).ID,
	})
}

// writeExitNodeError writes a 404 for missing or unauthorized rows and a 500
// for any other error. It reports whether a response was written.
func writeExitNodeError(rw http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case httpapi.Is404Error(err):
		httpapi.ResourceNotFound(rw)
	default:
		httpapi.InternalServerError(rw, err)
	}
	return true
}

// exitNodeParam resolves the {exitnode} URL parameter, which may be an ID or
// a name, within the organization from ExtractOrganizationParam. Soft-deleted
// nodes and nodes from other organizations are reported as not found.
func (api *API) exitNodeParam(rw http.ResponseWriter, r *http.Request) (database.ExitNode, bool) {
	var (
		ctx          = r.Context()
		organization = httpmw.OrganizationParam(r)
		param        = chi.URLParam(r, "exitnode")
		node         database.ExitNode
		err          error
	)
	if param == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "\"exitnode\" must be provided.",
		})
		return node, false
	}
	if id, parseErr := uuid.Parse(param); parseErr == nil {
		node, err = api.Database.GetExitNodeByID(ctx, id)
		if err == nil && (node.Deleted || node.OrganizationID != organization.ID) {
			err = sql.ErrNoRows
		}
	} else {
		node, err = api.Database.GetExitNodeByOrgAndName(ctx, database.GetExitNodeByOrgAndNameParams{
			OrganizationID: organization.ID,
			Name:           param,
		})
	}
	return node, !writeExitNodeError(rw, err)
}

// @Summary Register exit node
// @ID register-exit-node
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.RegisterExitNodeRequest true "Register exit node request"
// @Success 201 {object} codersdk.RegisterExitNodeResponse
// @Router /api/v2/exitnodes/me/register [post]
// @x-apidocgen {"skip": true}
func (api *API) registerExitNode(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		node = httpmw.ExitNode(r)
		req  codersdk.RegisterExitNodeRequest
	)
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.ReplicaID == uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Replica ID is invalid."})
		return
	}

	now := dbtime.Now()
	isNew := false
	var registrationErr error
	err := api.Database.InTx(func(db database.Store) error {
		replica, err := db.GetExitNodeReplicaByID(ctx, req.ReplicaID)
		switch {
		case err == nil:
			if replica.StoppedAt.Valid {
				registrationErr = xerrors.New("replica is stopped; restart with a new replica id")
				return registrationErr
			}
			if replica.ExitNodeID != node.ID {
				registrationErr = xerrors.New("replica belongs to a different exit node")
				return registrationErr
			}
		case xerrors.Is(err, sql.ErrNoRows):
			isNew = true
		case err != nil:
			return xerrors.Errorf("get exit node replica: %w", err)
		}
		_, err = db.UpsertExitNodeReplica(ctx, database.UpsertExitNodeReplicaParams{
			ID:                 req.ReplicaID,
			ExitNodeID:         node.ID,
			Hostname:           req.Hostname,
			Version:            req.Version,
			WireguardEndpoints: nonNil(req.WireguardEndpoints),
			PolicyHash:         req.PolicyHash,
			Now:                now,
		})
		return err
	}, nil)
	if registrationErr != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: registrationErr.Error()})
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("upsert exit node replica: %w", err))
		return
	}
	if isNew {
		if err := api.Pubsub.Publish(codersdk.ExitNodeReplicasPubsubChannel, []byte(node.ID.String())); err != nil {
			httpapi.InternalServerError(rw, xerrors.Errorf("publish exit node replica update: %w", err))
			return
		}
	}
	if err := api.Database.DeleteStaleExitNodeReplicas(ctx, now.Add(-24*time.Hour)); err != nil {
		api.Logger.Warn(ctx, "failed to delete stale exit node replicas", slog.Error(err))
	}

	// The middleware already runs the handler as system, which the query
	// requires because the agents span workspaces owned by many users.
	agentIDs, err := api.Database.GetWorkspaceAgentIDsByExitNode(ctx, node.ID)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get bound agents: %w", err))
		return
	}
	liveReplicas, err := api.Database.GetLiveExitNodeReplicas(ctx, database.GetLiveExitNodeReplicasParams{
		ExitNodeIds:  []uuid.UUID{node.ID},
		UpdatedAfter: now.Add(-codersdk.ExitNodeReplicaStaleAfter),
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get sibling replicas: %w", err))
		return
	}
	siblings := make([]codersdk.ExitNodeReplica, 0, len(liveReplicas))
	for _, replica := range liveReplicas {
		if replica.ID != req.ReplicaID {
			siblings = append(siblings, convertExitNodeReplica(replica, now))
		}
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.RegisterExitNodeResponse{
		DERPMap:             api.AGPL.DERPMap(),
		DERPForceWebSockets: api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		AgentIDs:            nonNil(agentIDs),
		SiblingReplicas:     siblings,
	})
}

// @Summary Deregister exit node
// @ID deregister-exit-node
// @Security CoderSessionToken
// @Accept json
// @Tags Enterprise
// @Param request body codersdk.DeregisterExitNodeRequest true "Deregister exit node request"
// @Success 204
// @Router /api/v2/exitnodes/me/deregister [post]
// @x-apidocgen {"skip": true}
func (api *API) deregisterExitNode(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	node := httpmw.ExitNode(r)
	var req codersdk.DeregisterExitNodeRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	replica, err := api.Database.GetExitNodeReplicaByID(ctx, req.ReplicaID)
	if xerrors.Is(err, sql.ErrNoRows) || err == nil && replica.ExitNodeID != node.ID {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get exit node replica: %w", err))
		return
	}
	if err := api.Database.StopExitNodeReplica(ctx, database.StopExitNodeReplicaParams{
		ID: req.ReplicaID, StoppedAt: dbtime.Now(),
	}); err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("stop exit node replica: %w", err))
		return
	}
	if err := api.Pubsub.Publish(codersdk.ExitNodeReplicasPubsubChannel, []byte(node.ID.String())); err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("publish exit node replica update: %w", err))
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Exit node coordinate
// @ID exit-node-coordinate
// @Security CoderSessionToken
// @Tags Enterprise
// @Success 101
// @Router /api/v2/exitnodes/me/coordinate [get]
// @x-apidocgen {"skip": true}
func (api *API) exitNodeCoordinate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	node := httpmw.ExitNode(r)
	replicaID, err := uuid.Parse(r.URL.Query().Get(codersdk.ExitNodeCoordinateReplicaIDParam))
	if err != nil || replicaID == uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{Message: "Replica ID is missing or invalid."})
		return
	}
	replica, err := api.Database.GetExitNodeReplicaByID(ctx, replicaID)
	if xerrors.Is(err, sql.ErrNoRows) || err == nil && replica.ExitNodeID != node.ID {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get exit node replica: %w", err))
		return
	}
	if replica.StoppedAt.Valid {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "Replica is stopped."})
		return
	}
	if !replica.UpdatedAt.After(dbtime.Now().Add(-codersdk.ExitNodeReplicaStaleAfter)) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{Message: "Replica is stale; replicas must register before coordinating."})
		return
	}
	api.serveMultiAgentCoordinate(rw, r, replicaID)
}

// @Summary Report exit node flows
// @ID report-exit-node-flows
// @Security CoderSessionToken
// @Accept json
// @Tags Enterprise
// @Param request body codersdk.ReportExitNodeFlowsRequest true "Flow reports"
// @Success 204
// @Router /api/v2/exitnodes/me/flows [post]
// @x-apidocgen {"skip": true}
func (api *API) reportExitNodeFlows(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		node = httpmw.ExitNode(r)
		req  codersdk.ReportExitNodeFlowsRequest
	)
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if replicaID := r.URL.Query().Get(codersdk.ExitNodeCoordinateReplicaIDParam); replicaID != "" {
		api.Logger.Debug(ctx, "received exit node flow report", slog.F("replica_id", replicaID))
	}
	connLogger := api.AGPL.ConnectionLogger.Load()
	if connLogger == nil {
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	// Connection log fields shared by every flow from one agent are resolved
	// once per report. A nil entry marks an agent that is unknown or whose
	// template does not route egress through this exit node.
	agents := make(map[uuid.UUID]*database.UpsertConnectionLogParams)
	templates := make(map[uuid.UUID]database.Template)
	gapPending := req.DroppedReports > 0
	for _, flow := range req.Flows {
		base, ok := agents[flow.AgentID]
		if !ok {
			var err error
			base, err = api.exitNodeFlowBase(ctx, node.ID, flow.AgentID, templates)
			if err != nil {
				httpapi.InternalServerError(rw, xerrors.Errorf("resolve agent %s: %w", flow.AgentID, err))
				return
			}
			agents[flow.AgentID] = base
		}
		if base == nil {
			api.Logger.Warn(ctx, "ignoring flow for agent not bound to exit node",
				slog.F("exit_node_id", node.ID),
				slog.F("agent_id", flow.AgentID),
				slog.F("flow_id", flow.FlowID),
			)
			continue
		}
		if gapPending {
			gap := *base
			gap.ID = uuid.NewSHA1(flow.FlowID, []byte("dropped-reports"))
			gap.ConnectionID = uuid.NullUUID{UUID: gap.ID, Valid: true}
			gap.Time = flow.ConnectTime
			gap.ConnectionStatus = database.ConnectionStatusDisconnected
			gap.Code = sql.NullInt32{Valid: true}
			gap.SlugOrPort = sql.NullString{String: "exit-node-report-gap", Valid: true}
			gap.DisconnectReason = sql.NullString{
				String: fmt.Sprintf("dropped %d exit node flow reports", req.DroppedReports),
				Valid:  true,
			}
			if err := (*connLogger).Upsert(ctx, gap); err != nil {
				httpapi.InternalServerError(rw, xerrors.Errorf("upsert connection log gap marker: %w", err))
				return
			}
			gapPending = false
		}
		for _, params := range exitNodeFlowConnectionLogs(*base, flow) {
			if err := (*connLogger).Upsert(ctx, params); err != nil {
				httpapi.InternalServerError(rw, xerrors.Errorf("upsert connection log: %w", err))
				return
			}
		}
	}
	rw.WriteHeader(http.StatusNoContent)
}

// exitNodeFlowBase loads the agent and workspace behind a flow and returns
// the connection log fields they contribute, or nil when the agent is missing
// or its template is not bound to exitNodeID. A missing agent is not an error
// so one stale flow cannot fail the whole report. templates caches lookups
// across agents of one report.
func (api *API) exitNodeFlowBase(ctx context.Context, exitNodeID, agentID uuid.UUID, templates map[uuid.UUID]database.Template) (*database.UpsertConnectionLogParams, error) {
	agent, err := api.Database.GetWorkspaceAgentByID(ctx, agentID)
	if httpapi.Is404Error(err) {
		return nil, nil //nolint:nilnil // Unknown agents are skipped.
	}
	if err != nil {
		return nil, xerrors.Errorf("get workspace agent: %w", err)
	}
	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, agentID)
	if httpapi.Is404Error(err) {
		return nil, nil //nolint:nilnil // Unknown workspaces are skipped.
	}
	if err != nil {
		return nil, xerrors.Errorf("get workspace by agent: %w", err)
	}
	template, ok := templates[workspace.TemplateID]
	if !ok {
		if template, err = api.Database.GetTemplateByID(ctx, workspace.TemplateID); err != nil {
			return nil, xerrors.Errorf("get template: %w", err)
		}
		templates[workspace.TemplateID] = template
	}
	if !slice.Contains(template.ExitNodeIds, exitNodeID) {
		return nil, nil //nolint:nilnil // Unbound agents are skipped.
	}
	// Exit nodes report agent traffic, so no user or user agent is known.
	// The per-flow fields are set by exitNodeFlowConnectionLogs.
	//nolint:exhaustruct // See above.
	return &database.UpsertConnectionLogParams{
		OrganizationID:   workspace.OrganizationID,
		WorkspaceOwnerID: workspace.OwnerID,
		WorkspaceID:      workspace.ID,
		WorkspaceName:    workspace.Name,
		AgentName:        agent.Name,
		Type:             database.ConnectionTypeEgress,
	}, nil
}

// exitNodeFlowConnectionLogs encodes one flow report as connection log
// upserts on top of the agent fields in base. The connection_logs schema is
// reused without changes:
//
//   - id and connection_id are the flow ID, so connect and disconnect
//     reports for one flow collapse into a single row.
//   - type is "egress" and ip is the destination address.
//   - slug_or_port is the destination as dialed: "<host or ip>:<port>" for
//     tcp, "udp <host or ip>:<port>" for udp, and "dns <query name>" for
//     dns. The protocol prefix is separated by a space, which cannot occur
//     in a host, so unprefixed values decode as tcp.
//   - code is 0 for allowed flows and 403 for denied flows.
//   - disconnect_reason is "<rule id>: <reason>". Completed flows append
//     " (in=<bytes in> out=<bytes out>)". The database keeps the first
//     non-null reason, so for allowed flows the reason is only written
//     with the disconnect report; denied flows are terminal and write it
//     immediately.
//
// A report carrying a disconnect time yields a connect upsert followed by a
// disconnect upsert so that connect_time is preserved even when the exit
// node reports the whole flow at once.
func exitNodeFlowConnectionLogs(base database.UpsertConnectionLogParams, flow codersdk.ExitNodeFlowReport) []database.UpsertConnectionLogParams {
	deny := flow.Decision == codersdk.ExitNodeFlowDeny
	base.ID = flow.FlowID
	base.ConnectionID = uuid.NullUUID{UUID: flow.FlowID, Valid: true}
	base.IP = database.ParseIP(flow.DestinationIP)
	base.Code = sql.NullInt32{Valid: true}
	if deny {
		base.Code.Int32 = http.StatusForbidden
	}
	hostPort := net.JoinHostPort(cmp.Or(flow.Host, flow.DestinationIP), strconv.Itoa(flow.DestinationPort))
	base.SlugOrPort = sql.NullString{String: hostPort, Valid: true}
	switch flow.Protocol {
	case codersdk.ExitNodeProtocolDNS:
		base.SlugOrPort.String = string(codersdk.ExitNodeProtocolDNS) + " " + flow.Host
	case codersdk.ExitNodeProtocolUDP:
		base.SlugOrPort.String = string(codersdk.ExitNodeProtocolUDP) + " " + hostPort
	}
	reason := cmp.Or(flow.Reason, string(flow.Decision))
	if flow.RuleID != "" {
		reason = flow.RuleID + ": " + reason
	}

	connect := base
	connect.Time = flow.ConnectTime
	connect.ConnectionStatus = database.ConnectionStatusConnected
	if deny {
		connect.DisconnectReason = sql.NullString{String: reason, Valid: true}
	}
	if flow.DisconnectTime == nil {
		return []database.UpsertConnectionLogParams{connect}
	}
	disconnect := base
	disconnect.Time = *flow.DisconnectTime
	disconnect.ConnectionStatus = database.ConnectionStatusDisconnected
	disconnect.DisconnectReason = sql.NullString{
		String: fmt.Sprintf("%s (in=%d out=%d)", reason, flow.BytesIn, flow.BytesOut),
		Valid:  true,
	}
	return []database.UpsertConnectionLogParams{connect, disconnect}
}

func convertExitNode(node database.ExitNode, replicas []database.ExitNodeReplica, now time.Time) codersdk.ExitNode {
	convertedReplicas := make([]codersdk.ExitNodeReplica, 0, len(replicas))
	status := codersdk.ExitNodeStatusUnregistered
	policyHashes := make(map[string]struct{})
	for _, replica := range replicas {
		converted := convertExitNodeReplica(replica, now)
		convertedReplicas = append(convertedReplicas, converted)
		if converted.Status == codersdk.ExitNodeReplicaStatusLive {
			status = codersdk.ExitNodeStatusHealthy
			if converted.PolicyHash != "" {
				policyHashes[converted.PolicyHash] = struct{}{}
			}
		}
	}
	if len(replicas) > 0 && status != codersdk.ExitNodeStatusHealthy {
		status = codersdk.ExitNodeStatusUnreachable
	}
	return codersdk.ExitNode{
		ID:             node.ID,
		OrganizationID: node.OrganizationID,
		Name:           node.Name,
		DisplayName:    node.DisplayName,
		CreatedAt:      node.CreatedAt,
		UpdatedAt:      node.UpdatedAt,
		Status:         status,
		PolicyMismatch: len(policyHashes) > 1,
		Replicas:       convertedReplicas,
	}
}

func convertExitNodeReplica(replica database.ExitNodeReplica, now time.Time) codersdk.ExitNodeReplica {
	status := codersdk.ExitNodeReplicaStatusLive
	var stoppedAt *time.Time
	if replica.StoppedAt.Valid {
		status = codersdk.ExitNodeReplicaStatusStopped
		stoppedAt = &replica.StoppedAt.Time
	} else if !replica.UpdatedAt.After(now.Add(-codersdk.ExitNodeReplicaStaleAfter)) {
		status = codersdk.ExitNodeReplicaStatusStale
	}
	return codersdk.ExitNodeReplica{
		ID:                 replica.ID,
		ExitNodeID:         replica.ExitNodeID,
		Hostname:           replica.Hostname,
		Version:            replica.Version,
		WireguardEndpoints: nonNil(replica.WireguardEndpoints),
		PolicyHash:         replica.PolicyHash,
		TailnetAddress:     tailnet.TailscaleServicePrefix.AddrFromUUID(replica.ID).String(),
		Status:             status,
		StartedAt:          replica.StartedAt,
		UpdatedAt:          replica.UpdatedAt,
		StoppedAt:          stoppedAt,
	}
}

// nonNil replaces a nil slice with an empty one so it encodes as a JSON
// array and as a non-null database array.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
