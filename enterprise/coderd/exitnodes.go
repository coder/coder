package coderd

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
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
	"github.com/coder/coder/v2/codersdk"
	enttailnet "github.com/coder/coder/v2/enterprise/tailnet"
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

func (api *API) exitNodeResponse(ctx context.Context, rw http.ResponseWriter, node database.ExitNode, now time.Time) (codersdk.ExitNode, bool) {
	replicas, err := api.Database.GetExitNodeReplicasByExitNode(ctx, node.ID)
	return convertExitNode(node, replicas, now), !writeExitNodeError(rw, err, "get exit node replicas")
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
	for _, node := range nodes {
		convertedNode, ok := api.exitNodeResponse(ctx, rw, node, now)
		if !ok {
			return
		}
		converted = append(converted, convertedNode)
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
	converted, ok := api.exitNodeResponse(r.Context(), rw, node, dbtime.Now())
	if !ok {
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, converted)
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
	now := dbtime.Now()
	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.DeleteExitNodeByID(r.Context(), node.ID); err != nil {
			return xerrors.Errorf("delete exit node: %w", err)
		}
		if err := tx.DeleteTemplateExitNodesByExitNode(r.Context(), node.ID); err != nil {
			return xerrors.Errorf("delete template exit node bindings: %w", err)
		}
		if err := tx.StopExitNodeReplicasByExitNode(r.Context(), database.StopExitNodeReplicasByExitNodeParams{
			ExitNodeID: node.ID,
			StoppedAt:  now,
		}); err != nil {
			return xerrors.Errorf("stop exit node replicas: %w", err)
		}
		return nil
	}, nil)
	if writeExitNodeError(rw, err) {
		return
	}
	api.exitNodeReplicaSessions.stopExitNode(node.ID)
	if err := api.Pubsub.Publish(codersdk.ExitNodeReplicasPubsubChannel, []byte(node.ID.String())); err != nil {
		api.Logger.Error(r.Context(), "failed to publish deleted exit node", slog.Error(err))
	}
	aReq.New = database.ExitNode{}
	rw.WriteHeader(http.StatusNoContent)
}

func (api *API) exitNodeAudit(rw http.ResponseWriter, r *http.Request, action database.AuditAction) (*audit.Request[database.ExitNode], func()) {
	return audit.InitRequest[database.ExitNode](rw, &audit.RequestParams{
		Audit:          *api.AGPL.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         action,
		OrganizationID: httpmw.OrganizationParam(r).ID,
	})
}

func writeExitNodeError(rw http.ResponseWriter, err error, message ...string) bool {
	if err == nil {
		return false
	}
	if len(message) > 0 {
		err = xerrors.Errorf("%s: %w", message[0], err)
	}
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
	} else {
		httpapi.InternalServerError(rw, err)
	}
	return true
}

func writeExitNodeResponse(ctx context.Context, rw http.ResponseWriter, status int, message string) {
	httpapi.Write(ctx, rw, status, codersdk.Response{Message: message})
}

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
		writeExitNodeResponse(ctx, rw, http.StatusBadRequest, "Replica ID is invalid.")
		return
	}
	if _, err := api.Database.GetWorkspaceAgentByID(ctx, req.ReplicaID); err == nil {
		writeExitNodeResponse(ctx, rw, http.StatusBadRequest, "Replica ID conflicts with a workspace agent.")
		return
	} else if !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("check workspace agent replica id: %w", err))
		return
	}
	if _, err := api.Database.GetWorkspaceProxyByID(ctx, req.ReplicaID); err == nil {
		writeExitNodeResponse(ctx, rw, http.StatusBadRequest, "Replica ID conflicts with a workspace proxy.")
		return
	} else if !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, xerrors.Errorf("check workspace proxy replica id: %w", err))
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
		writeExitNodeResponse(ctx, rw, http.StatusBadRequest, registrationErr.Error())
		return
	}
	if writeExitNodeError(rw, err, "upsert exit node replica") {
		return
	}
	api.exitNodeReplicaSessions.markLive(req.ReplicaID, node.ID)
	if isNew {
		if err := api.Pubsub.Publish(codersdk.ExitNodeReplicasPubsubChannel, []byte(node.ID.String())); err != nil {
			httpapi.InternalServerError(rw, xerrors.Errorf("publish exit node replica update: %w", err))
			return
		}
	}
	if err := api.Database.DeleteStaleExitNodeReplicas(ctx, now.Add(-24*time.Hour)); err != nil {
		api.Logger.Warn(ctx, "failed to delete stale exit node replicas", slog.Error(err))
	}

	agentIDs, err := api.Database.GetWorkspaceAgentIDsByExitNode(ctx, node.ID)
	if writeExitNodeError(rw, err, "get bound agents") {
		return
	}
	liveReplicas, err := api.Database.GetLiveExitNodeReplicas(ctx, database.GetLiveExitNodeReplicasParams{
		ExitNodeIds:  []uuid.UUID{node.ID},
		UpdatedAfter: now.Add(-codersdk.ExitNodeReplicaStaleAfter),
	})
	if writeExitNodeError(rw, err, "get sibling replicas") {
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

func (api *API) exitNodeReplica(ctx context.Context, rw http.ResponseWriter, nodeID, replicaID uuid.UUID) (database.ExitNodeReplica, bool) {
	replica, err := api.Database.GetExitNodeReplicaByID(ctx, replicaID)
	if err == nil && replica.ExitNodeID != nodeID {
		err = sql.ErrNoRows
	}
	return replica, !writeExitNodeError(rw, err, "get exit node replica")
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
	if _, ok := api.exitNodeReplica(ctx, rw, node.ID, req.ReplicaID); !ok {
		return
	}
	if err := api.Database.StopExitNodeReplica(ctx, database.StopExitNodeReplicaParams{
		ID: req.ReplicaID, StoppedAt: dbtime.Now(),
	}); err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("stop exit node replica: %w", err))
		return
	}
	api.exitNodeReplicaSessions.stop(req.ReplicaID)
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
		writeExitNodeResponse(ctx, rw, http.StatusBadRequest, "Replica ID is missing or invalid.")
		return
	}
	peerID := codersdk.ExitNodeReplicaPeerID(node.ID, replicaID)
	coordinateCtx, cancel := context.WithCancel(ctx)
	unregister := api.exitNodeReplicaSessions.registerExitNode(replicaID, node.ID, cancel)
	defer unregister()
	defer cancel()

	replica, ok := api.exitNodeReplica(coordinateCtx, rw, node.ID, replicaID)
	if !ok {
		return
	}
	if replica.StoppedAt.Valid {
		writeExitNodeResponse(ctx, rw, http.StatusConflict, "Replica is stopped.")
		return
	}
	if !replica.UpdatedAt.After(dbtime.Now().Add(-codersdk.ExitNodeReplicaStaleAfter)) {
		writeExitNodeResponse(ctx, rw, http.StatusConflict, "Replica is stale; replicas must register before coordinating.")
		return
	}
	auth := &enttailnet.ExitNodeCoordinateeAuth{
		Database:   api.Database,
		Clock:      api.Clock,
		ExitNodeID: node.ID,
		PeerID:     peerID,
	}
	api.serveMultiAgentCoordinate(rw, r.WithContext(coordinateCtx), peerID, auth)
}

const (
	maxExitNodeFlowReportBytes = 1 << 20
	maxExitNodeFlowReports     = 1000
	maxExitNodeFlowReasonLen   = 512
	maxExitNodeFlowRuleIDLen   = 128
	maxExitNodeFlowHostLen     = 253
)

func validateExitNodeFlow(flow *codersdk.ExitNodeFlowReport, now time.Time) string {
	if flow.AgentID == uuid.Nil {
		return "agent_id must be a valid UUID"
	}
	if flow.FlowID == uuid.Nil {
		return "flow_id must be a valid UUID"
	}
	if flow.Protocol == "" {
		flow.Protocol = codersdk.ExitNodeProtocolTCP
	}
	switch flow.Protocol {
	case codersdk.ExitNodeProtocolTCP, codersdk.ExitNodeProtocolUDP, codersdk.ExitNodeProtocolDNS:
		if flow.DestinationPort < 1 || flow.DestinationPort > 65535 {
			return "destination_port must be between 1 and 65535"
		}
	case codersdk.ExitNodeProtocolICMP, codersdk.ExitNodeProtocolNone:
		if flow.DestinationPort < 0 || flow.DestinationPort > 65535 {
			return "destination_port must be between 0 and 65535"
		}
	default:
		return "protocol is invalid"
	}
	if flow.Decision != codersdk.ExitNodeFlowAllow && flow.Decision != codersdk.ExitNodeFlowDeny {
		return "decision is invalid"
	}
	if flow.BytesIn < 0 || flow.BytesOut < 0 {
		return "byte counters must be non-negative"
	}
	if len(flow.Reason) > maxExitNodeFlowReasonLen {
		return fmt.Sprintf("reason must not exceed %d characters", maxExitNodeFlowReasonLen)
	}
	if len(flow.RuleID) > maxExitNodeFlowRuleIDLen {
		return fmt.Sprintf("rule_id must not exceed %d characters", maxExitNodeFlowRuleIDLen)
	}
	if len(flow.Host) > maxExitNodeFlowHostLen {
		return fmt.Sprintf("host must not exceed %d characters", maxExitNodeFlowHostLen)
	}
	if flow.ConnectTime.IsZero() || flow.ConnectTime.After(now.Add(5*time.Minute)) {
		return "connect_time must be set and not more than 5 minutes in the future"
	}
	if flow.DisconnectTime != nil && (flow.DisconnectTime.IsZero() || flow.DisconnectTime.After(now.Add(5*time.Minute))) {
		return "disconnect_time must be set and not more than 5 minutes in the future"
	}
	return ""
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
	r.Body = http.MaxBytesReader(rw, r.Body, maxExitNodeFlowReportBytes)
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if len(req.Flows) > maxExitNodeFlowReports {
		writeExitNodeResponse(ctx, rw, http.StatusBadRequest, fmt.Sprintf("No more than %d flows may be reported at once.", maxExitNodeFlowReports))
		return
	}
	if req.DroppedReports < 0 {
		writeExitNodeResponse(ctx, rw, http.StatusBadRequest, "Dropped reports must be non-negative.")
		return
	}
	now := dbtime.Now()
	agentIDs := make([]uuid.UUID, 0, len(req.Flows))
	seenAgentIDs := make(map[uuid.UUID]struct{}, len(req.Flows))
	for i := range req.Flows {
		if detail := validateExitNodeFlow(&req.Flows[i], now); detail != "" {
			writeExitNodeResponse(ctx, rw, http.StatusBadRequest, fmt.Sprintf("Flow %d is invalid: %s.", i, strings.TrimSuffix(detail, ".")))
			return
		}
		if _, ok := seenAgentIDs[req.Flows[i].AgentID]; !ok {
			seenAgentIDs[req.Flows[i].AgentID] = struct{}{}
			agentIDs = append(agentIDs, req.Flows[i].AgentID)
		}
	}
	if replicaID := r.URL.Query().Get(codersdk.ExitNodeCoordinateReplicaIDParam); replicaID != "" {
		api.Logger.Debug(ctx, "received exit node flow report", slog.F("replica_id", replicaID))
	}
	connLogger := api.AGPL.ConnectionLogger.Load()
	if connLogger == nil {
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	rows, err := api.Database.GetExitNodeFlowAgents(ctx, database.GetExitNodeFlowAgentsParams{
		AgentIds:   agentIDs,
		ExitNodeID: node.ID,
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("resolve flow agents: %w", err))
		return
	}
	agents := make(map[uuid.UUID]*database.UpsertConnectionLogParams, len(rows))
	for _, row := range rows {
		//nolint:exhaustruct // Per-flow fields are added by exitNodeFlowConnectionLogs.
		agents[row.AgentID] = &database.UpsertConnectionLogParams{
			OrganizationID:   row.OrganizationID,
			WorkspaceOwnerID: row.WorkspaceOwnerID,
			WorkspaceID:      row.WorkspaceID,
			WorkspaceName:    row.WorkspaceName,
			AgentName:        row.AgentName,
			Type:             database.ConnectionTypeEgress,
		}
	}
	unknownAgents := 0
	gapPending := req.DroppedReports > 0
	for _, flow := range req.Flows {
		base := agents[flow.AgentID]
		if base == nil {
			unknownAgents++
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
	if unknownAgents > 0 {
		api.Logger.Warn(ctx, "ignored exit node flows for unknown or unbound agents",
			slog.F("exit_node_id", node.ID),
			slog.F("flow_count", unknownAgents),
		)
	}
	rw.WriteHeader(http.StatusNoContent)
}

// exitNodeFlowConnectionLogs maps a flow to the existing connection log
// schema. A completed flow emits connect and disconnect upserts so the latter
// preserves the original connect time.
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
		TailnetAddress:     tailnet.TailscaleServicePrefix.AddrFromUUID(codersdk.ExitNodeReplicaPeerID(replica.ExitNodeID, replica.ID)).String(),
		Status:             status,
		StartedAt:          replica.StartedAt,
		UpdatedAt:          replica.UpdatedAt,
		StoppedAt:          stoppedAt,
	}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
