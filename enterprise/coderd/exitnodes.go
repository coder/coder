package coderd

import (
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
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/websocket"
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
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.ExitNode](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionCreate,
			OrganizationID: organization.ID,
		})
	)
	defer commitAudit()

	var req codersdk.CreateExitNodeRequest
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
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = node
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.CreateExitNodeResponse{
		ExitNode: convertExitNode(node),
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
	var (
		ctx          = r.Context()
		organization = httpmw.OrganizationParam(r)
	)

	nodes, err := api.Database.GetExitNodesByOrganization(ctx, organization.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	resp := make([]codersdk.ExitNode, 0, len(nodes))
	for _, node := range nodes {
		resp = append(resp, convertExitNode(node))
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
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
	ctx := r.Context()
	node, ok := api.exitNodeParam(rw, r)
	if !ok {
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, convertExitNode(node))
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
	var (
		ctx               = r.Context()
		organization      = httpmw.OrganizationParam(r)
		auditor           = api.AGPL.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.ExitNode](rw, &audit.RequestParams{
			Audit:          *auditor,
			Log:            api.Logger,
			Request:        r,
			Action:         database.AuditActionDelete,
			OrganizationID: organization.ID,
		})
	)
	defer commitAudit()

	node, ok := api.exitNodeParam(rw, r)
	if !ok {
		return
	}
	aReq.Old = node

	err := api.Database.DeleteExitNodeByID(ctx, node.ID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	aReq.New = database.ExitNode{}
	rw.WriteHeader(http.StatusNoContent)
}

// exitNodeParam resolves the {exitnode} URL parameter, which may be an ID or
// a name, within the organization from ExtractOrganizationParam. Soft-deleted
// nodes and nodes from other organizations are reported as not found.
func (api *API) exitNodeParam(rw http.ResponseWriter, r *http.Request) (database.ExitNode, bool) {
	var (
		ctx          = r.Context()
		organization = httpmw.OrganizationParam(r)
		param        = chi.URLParam(r, "exitnode")
	)
	if param == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "\"exitnode\" must be provided.",
		})
		return database.ExitNode{}, false
	}

	var (
		node database.ExitNode
		err  error
	)
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
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return database.ExitNode{}, false
	}
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return database.ExitNode{}, false
	}
	return node, true
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
	)

	var req codersdk.RegisterExitNodeRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	endpoints := req.WireguardEndpoints
	if endpoints == nil {
		endpoints = []string{}
	}
	_, err := api.Database.UpdateExitNodeRegistration(ctx, database.UpdateExitNodeRegistrationParams{
		ID:                 node.ID,
		Version:            req.Version,
		LastSeenAt:         dbtime.Now(),
		WireguardEndpoints: endpoints,
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("update exit node registration: %w", err))
		return
	}

	// The middleware already runs the handler as system, which the query
	// requires because the agents span workspaces owned by many users.
	agentIDs, err := api.Database.GetWorkspaceAgentIDsByExitNode(ctx, node.ID)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("get bound agents: %w", err))
		return
	}
	if agentIDs == nil {
		agentIDs = []uuid.UUID{}
	}

	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.RegisterExitNodeResponse{
		DERPMap:             api.AGPL.DERPMap(),
		DERPForceWebSockets: api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		AgentIDs:            agentIDs,
	})
}

// @Summary Exit node coordinate
// @ID exit-node-coordinate
// @Security CoderSessionToken
// @Tags Enterprise
// @Success 101
// @Router /api/v2/exitnodes/me/coordinate [get]
// @x-apidocgen {"skip": true}
func (api *API) exitNodeCoordinate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		node = httpmw.ExitNode(r)
	)

	version := "1.0"
	msgType := websocket.MessageText
	qv := r.URL.Query().Get("version")
	if qv != "" {
		version = qv
	}
	if err := proto.CurrentVersion.Validate(version); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unknown or unsupported API version",
			Validations: []codersdk.ValidationError{
				{Field: "version", Detail: err.Error()},
			},
		})
		return
	}
	maj, _, _ := apiversion.Parse(version)
	if maj >= 2 {
		// Versions 2+ use dRPC over a binary connection.
		msgType = websocket.MessageBinary
	}

	api.AGPL.WebsocketWaitMutex.Lock()
	api.AGPL.WebsocketWaitGroup.Add(1)
	api.AGPL.WebsocketWaitMutex.Unlock()
	defer api.AGPL.WebsocketWaitGroup.Done()

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, nc := codersdk.WebsocketNetConn(ctx, conn, msgType)
	defer nc.Close()

	// The exit node ID doubles as its tailnet peer ID so that agents can
	// derive its tailnet address from the ID alone.
	err = api.tailnetService.ServeMultiAgentClient(ctx, version, nc, node.ID)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
	} else {
		_ = conn.Close(websocket.StatusGoingAway, "")
	}
}

// exitNodeFlowAgent caches the workspace context of an agent for one flow
// report so that repeated flows from the same agent cost one lookup.
type exitNodeFlowAgent struct {
	agent     database.WorkspaceAgent
	workspace database.Workspace
	// bound is false when the agent's template does not route egress
	// through the reporting exit node.
	bound bool
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
	)

	var req codersdk.ReportExitNodeFlowsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	connLogger := api.AGPL.ConnectionLogger.Load()
	if connLogger == nil {
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	agents := make(map[uuid.UUID]exitNodeFlowAgent)
	templates := make(map[uuid.UUID]database.Template)
	for _, flow := range req.Flows {
		info, ok := agents[flow.AgentID]
		if !ok {
			var err error
			info, err = api.resolveExitNodeFlowAgent(ctx, node, flow.AgentID, templates)
			if err != nil {
				httpapi.InternalServerError(rw, xerrors.Errorf("resolve agent %s: %w", flow.AgentID, err))
				return
			}
			agents[flow.AgentID] = info
		}
		if !info.bound {
			api.Logger.Warn(ctx, "ignoring flow for agent not bound to exit node",
				slog.F("exit_node_id", node.ID),
				slog.F("agent_id", flow.AgentID),
				slog.F("flow_id", flow.FlowID),
			)
			continue
		}

		for _, params := range exitNodeFlowConnectionLogs(info, flow) {
			if err := (*connLogger).Upsert(ctx, params); err != nil {
				httpapi.InternalServerError(rw, xerrors.Errorf("upsert connection log: %w", err))
				return
			}
		}
	}

	rw.WriteHeader(http.StatusNoContent)
}

// resolveExitNodeFlowAgent loads the agent and workspace behind a flow and
// checks that the workspace's template is bound to the reporting exit node.
// A missing agent is reported as unbound rather than an error so one stale
// flow cannot fail the whole report.
func (api *API) resolveExitNodeFlowAgent(ctx context.Context, node database.ExitNode, agentID uuid.UUID, templates map[uuid.UUID]database.Template) (exitNodeFlowAgent, error) {
	agent, err := api.Database.GetWorkspaceAgentByID(ctx, agentID)
	if httpapi.Is404Error(err) {
		return exitNodeFlowAgent{}, nil
	}
	if err != nil {
		return exitNodeFlowAgent{}, xerrors.Errorf("get workspace agent: %w", err)
	}
	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, agentID)
	if httpapi.Is404Error(err) {
		return exitNodeFlowAgent{}, nil
	}
	if err != nil {
		return exitNodeFlowAgent{}, xerrors.Errorf("get workspace by agent: %w", err)
	}
	template, ok := templates[workspace.TemplateID]
	if !ok {
		template, err = api.Database.GetTemplateByID(ctx, workspace.TemplateID)
		if err != nil {
			return exitNodeFlowAgent{}, xerrors.Errorf("get template: %w", err)
		}
		templates[workspace.TemplateID] = template
	}
	return exitNodeFlowAgent{
		agent:     agent,
		workspace: workspace,
		bound:     template.ExitNodeID.Valid && template.ExitNodeID.UUID == node.ID,
	}, nil
}

// exitNodeFlowConnectionLogs encodes one flow report as connection log
// upserts. The connection_logs schema is reused without changes:
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
func exitNodeFlowConnectionLogs(info exitNodeFlowAgent, flow codersdk.ExitNodeFlowReport) []database.UpsertConnectionLogParams {
	deny := flow.Decision == codersdk.ExitNodeFlowDeny

	var code int32
	if deny {
		code = http.StatusForbidden
	}

	destination := exitNodeFlowDestination(flow)

	base := database.UpsertConnectionLogParams{
		ID:               flow.FlowID,
		OrganizationID:   info.workspace.OrganizationID,
		WorkspaceOwnerID: info.workspace.OwnerID,
		WorkspaceID:      info.workspace.ID,
		WorkspaceName:    info.workspace.Name,
		AgentName:        info.agent.Name,
		Type:             database.ConnectionTypeEgress,
		Code:             sql.NullInt32{Int32: code, Valid: true},
		IP:               database.ParseIP(flow.DestinationIP),
		SlugOrPort:       sql.NullString{String: destination, Valid: true},
		ConnectionID:     uuid.NullUUID{UUID: flow.FlowID, Valid: true},
		// Exit nodes report agent traffic, so no user or user agent is known.
		UserAgent:        sql.NullString{},
		UserID:           uuid.NullUUID{},
		DisconnectReason: sql.NullString{},
		Time:             time.Time{},
		ConnectionStatus: "",
	}
	reason := exitNodeFlowReason(flow)

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

// exitNodeFlowDestination builds the slug_or_port encoding described on
// exitNodeFlowConnectionLogs. decodeEgressDestination reverses it.
func exitNodeFlowDestination(flow codersdk.ExitNodeFlowReport) string {
	switch flow.Protocol {
	case codersdk.ExitNodeProtocolDNS:
		return string(codersdk.ExitNodeProtocolDNS) + " " + flow.Host
	case codersdk.ExitNodeProtocolUDP:
		return string(codersdk.ExitNodeProtocolUDP) + " " + exitNodeFlowHostPort(flow)
	default:
		return exitNodeFlowHostPort(flow)
	}
}

func exitNodeFlowHostPort(flow codersdk.ExitNodeFlowReport) string {
	host := flow.Host
	if host == "" {
		host = flow.DestinationIP
	}
	return net.JoinHostPort(host, strconv.Itoa(flow.DestinationPort))
}

// exitNodeFlowReason builds the "<rule id>: <reason>" prefix of the
// disconnect_reason encoding described on exitNodeFlowConnectionLogs.
func exitNodeFlowReason(flow codersdk.ExitNodeFlowReport) string {
	reason := flow.Reason
	if reason == "" {
		reason = string(flow.Decision)
	}
	if flow.RuleID != "" {
		reason = flow.RuleID + ": " + reason
	}
	return reason
}

func convertExitNode(node database.ExitNode) codersdk.ExitNode {
	var lastSeenAt *time.Time
	if node.LastSeenAt.Valid {
		lastSeenAt = &node.LastSeenAt.Time
	}
	endpoints := node.WireguardEndpoints
	if endpoints == nil {
		endpoints = []string{}
	}
	return codersdk.ExitNode{
		ID:                 node.ID,
		OrganizationID:     node.OrganizationID,
		Name:               node.Name,
		DisplayName:        node.DisplayName,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
		LastSeenAt:         lastSeenAt,
		Version:            node.Version,
		WireguardEndpoints: endpoints,
		TailnetAddress:     tailnet.TailscaleServicePrefix.AddrFromUUID(node.ID).String(),
	}
}
