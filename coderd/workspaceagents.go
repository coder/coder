package coderd

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bep/debounce"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/tailnet"
)

// @Summary Get workspace agent by ID
// @ID get-workspace-agent-by-id
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceAgent
// @Router /workspaceagents/{workspaceagent} [get]
func (api *API) workspaceAgent(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgentParam(r)

	dbApps, err := api.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent applications.",
			Detail:  err.Error(),
		})
		return
	}
	apiAgent, err := convertWorkspaceAgent(
		api.DERPMap, *api.TailnetCoordinator.Load(), workspaceAgent, convertApps(dbApps), api.AgentInactiveDisconnectTimeout,
		api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, apiAgent)
}

// @Summary Get authorized workspace agent manifest
// @ID get-authorized-workspace-agent-manifest
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Success 200 {object} agentsdk.Manifest
// @Router /workspaceagents/me/manifest [get]
func (api *API) workspaceAgentManifest(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgent(r)
	apiAgent, err := convertWorkspaceAgent(
		api.DERPMap, *api.TailnetCoordinator.Load(), workspaceAgent, nil, api.AgentInactiveDisconnectTimeout,
		api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	dbApps, err := api.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent applications.",
			Detail:  err.Error(),
		})
		return
	}

	metadata, err := api.Database.GetWorkspaceAgentMetadata(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent metadata.",
			Detail:  err.Error(),
		})
		return
	}

	resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resource.",
			Detail:  err.Error(),
		})
		return
	}
	build, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	workspace, err := api.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}
	owner, err := api.Database.GetUserByID(ctx, workspace.OwnerID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace owner.",
			Detail:  err.Error(),
		})
		return
	}

	vscodeProxyURI := strings.ReplaceAll(api.AppHostname, "*",
		fmt.Sprintf("%s://{{port}}--%s--%s--%s",
			api.AccessURL.Scheme,
			workspaceAgent.Name,
			workspace.Name,
			owner.Username,
		))
	if api.AccessURL.Port() != "" {
		vscodeProxyURI += fmt.Sprintf(":%s", api.AccessURL.Port())
	}

	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.Manifest{
		Apps:                  convertApps(dbApps),
		DERPMap:               api.DERPMap,
		GitAuthConfigs:        len(api.GitAuthConfigs),
		EnvironmentVariables:  apiAgent.EnvironmentVariables,
		StartupScript:         apiAgent.StartupScript,
		Directory:             apiAgent.Directory,
		VSCodePortProxyURI:    vscodeProxyURI,
		MOTDFile:              workspaceAgent.MOTDFile,
		StartupScriptTimeout:  time.Duration(apiAgent.StartupScriptTimeoutSeconds) * time.Second,
		ShutdownScript:        apiAgent.ShutdownScript,
		ShutdownScriptTimeout: time.Duration(apiAgent.ShutdownScriptTimeoutSeconds) * time.Second,
		Metadata:              convertWorkspaceAgentMetadataDesc(metadata),
	})
}

// @Summary Submit workspace agent startup
// @ID submit-workspace-agent-startup
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.PostStartupRequest true "Startup request"
// @Success 200
// @Router /workspaceagents/me/startup [post]
// @x-apidocgen {"skip": true}
func (api *API) postWorkspaceAgentStartup(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgent(r)
	apiAgent, err := convertWorkspaceAgent(
		api.DERPMap, *api.TailnetCoordinator.Load(), workspaceAgent, nil, api.AgentInactiveDisconnectTimeout,
		api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}

	var req agentsdk.PostStartupRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	api.Logger.Info(ctx, "post workspace agent version", slog.F("agent_id", apiAgent.ID), slog.F("agent_version", req.Version))

	if !semver.IsValid(req.Version) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid workspace agent version provided.",
			Detail:  fmt.Sprintf("invalid semver version: %q", req.Version),
		})
		return
	}

	if err := api.Database.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
		ID:                apiAgent.ID,
		Version:           req.Version,
		ExpandedDirectory: req.ExpandedDirectory,
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error setting agent version",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, nil)
}

// @Summary Patch workspace agent startup logs
// @ID patch-workspace-agent-startup-logs
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.PatchStartupLogs true "Startup logs"
// @Success 200 {object} codersdk.Response
// @Router /workspaceagents/me/startup-logs [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchWorkspaceAgentStartupLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgent(r)

	var req agentsdk.PatchStartupLogs
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if len(req.Logs) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "No logs provided.",
		})
		return
	}
	createdAt := make([]time.Time, 0)
	output := make([]string, 0)
	level := make([]database.LogLevel, 0)
	outputLength := 0
	for _, log := range req.Logs {
		createdAt = append(createdAt, log.CreatedAt)
		output = append(output, log.Output)
		outputLength += len(log.Output)
		if log.Level == "" {
			// Default to "info" to support older agents that didn't have the level field.
			log.Level = codersdk.LogLevelInfo
		}
		parsedLevel := database.LogLevel(log.Level)
		if !parsedLevel.Valid() {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid log level provided.",
				Detail:  fmt.Sprintf("invalid log level: %q", log.Level),
			})
			return
		}
		level = append(level, parsedLevel)
	}
	logs, err := api.Database.InsertWorkspaceAgentStartupLogs(ctx, database.InsertWorkspaceAgentStartupLogsParams{
		AgentID:      workspaceAgent.ID,
		CreatedAt:    createdAt,
		Output:       output,
		Level:        level,
		OutputLength: int32(outputLength),
	})
	if err != nil {
		if database.IsStartupLogsLimitError(err) {
			if !workspaceAgent.StartupLogsOverflowed {
				err := api.Database.UpdateWorkspaceAgentStartupLogOverflowByID(ctx, database.UpdateWorkspaceAgentStartupLogOverflowByIDParams{
					ID:                    workspaceAgent.ID,
					StartupLogsOverflowed: true,
				})
				if err != nil {
					// We don't want to return here, because the agent will retry
					// on failure and this isn't a huge deal. The overflow state
					// is just a hint to the user that the logs are incomplete.
					api.Logger.Warn(ctx, "failed to update workspace agent startup log overflow", slog.Error(err))
				}

				resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
				if err != nil {
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: "Failed to get workspace resource.",
						Detail:  err.Error(),
					})
					return
				}

				build, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
				if err != nil {
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: "Internal error fetching workspace build job.",
						Detail:  err.Error(),
					})
					return
				}

				api.publishWorkspaceUpdate(ctx, build.WorkspaceID)
			}

			httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
				Message: "Startup logs limit exceeded",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to upload startup logs",
			Detail:  err.Error(),
		})
		return
	}
	if workspaceAgent.StartupLogsLength == 0 {
		// If these are the first logs being appended, we publish a UI update
		// to notify the UI that logs are now available.
		resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to get workspace resource.",
				Detail:  err.Error(),
			})
			return
		}

		build, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Internal error fetching workspace build job.",
				Detail:  err.Error(),
			})
			return
		}

		api.publishWorkspaceUpdate(ctx, build.WorkspaceID)
	}

	lowestID := logs[0].ID
	// Publish by the lowest log ID inserted so the
	// log stream will fetch everything from that point.
	data, err := json.Marshal(agentsdk.StartupLogsNotifyMessage{
		CreatedAfter: lowestID - 1,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to marshal startup logs notify message",
			Detail:  err.Error(),
		})
		return
	}
	err = api.Pubsub.Publish(agentsdk.StartupLogsNotifyChannel(workspaceAgent.ID), data)
	if err != nil {
		// We don't want to return an error to the agent here,
		// otherwise it might try to reinsert the logs.
		api.Logger.Warn(ctx, "failed to publish startup logs notify message", slog.Error(err))
	}

	httpapi.Write(ctx, rw, http.StatusOK, nil)
}

// workspaceAgentStartupLogs returns the logs sent from a workspace agent
// during startup.
//
// @Summary Get startup logs by workspace agent
// @ID get-startup-logs-by-workspace-agent
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Param before query int false "Before log id"
// @Param after query int false "After log id"
// @Param follow query bool false "Follow log stream"
// @Success 200 {array} codersdk.WorkspaceAgentStartupLog
// @Router /workspaceagents/{workspaceagent}/startup-logs [get]
func (api *API) workspaceAgentStartupLogs(rw http.ResponseWriter, r *http.Request) {
	// This mostly copies how provisioner job logs are streamed!
	var (
		ctx            = r.Context()
		actor, _       = dbauthz.ActorFromContext(ctx)
		workspaceAgent = httpmw.WorkspaceAgentParam(r)
		logger         = api.Logger.With(slog.F("workspace_agent_id", workspaceAgent.ID))
		follow         = r.URL.Query().Has("follow")
		afterRaw       = r.URL.Query().Get("after")
	)

	var after int64
	// Only fetch logs created after the time provided.
	if afterRaw != "" {
		var err error
		after, err = strconv.ParseInt(afterRaw, 10, 64)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query param \"after\" must be an integer.",
				Validations: []codersdk.ValidationError{
					{Field: "after", Detail: "Must be an integer"},
				},
			})
			return
		}
	}

	logs, err := api.Database.GetWorkspaceAgentStartupLogsAfter(ctx, database.GetWorkspaceAgentStartupLogsAfterParams{
		AgentID:      workspaceAgent.ID,
		CreatedAfter: after,
	})
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner logs.",
			Detail:  err.Error(),
		})
		return
	}
	if logs == nil {
		logs = []database.WorkspaceAgentStartupLog{}
	}

	if !follow {
		logger.Debug(ctx, "Finished non-follow job logs")
		httpapi.Write(ctx, rw, http.StatusOK, convertWorkspaceAgentStartupLogs(logs))
		return
	}

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()
	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	go httpapi.Heartbeat(ctx, conn)

	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageText)
	defer wsNetConn.Close() // Also closes conn.

	// The Go stdlib JSON encoder appends a newline character after message write.
	encoder := json.NewEncoder(wsNetConn)
	err = encoder.Encode(convertWorkspaceAgentStartupLogs(logs))
	if err != nil {
		return
	}
	if workspaceAgent.LifecycleState == database.WorkspaceAgentLifecycleStateReady {
		// The startup script has finished running, so we can close the connection.
		return
	}

	var (
		bufferedLogs  = make(chan []database.WorkspaceAgentStartupLog, 128)
		endOfLogs     atomic.Bool
		lastSentLogID atomic.Int64
	)

	sendLogs := func(logs []database.WorkspaceAgentStartupLog) {
		select {
		case bufferedLogs <- logs:
			lastSentLogID.Store(logs[len(logs)-1].ID)
		default:
			logger.Warn(ctx, "workspace agent startup log overflowing channel")
		}
	}

	closeSubscribe, err := api.Pubsub.Subscribe(
		agentsdk.StartupLogsNotifyChannel(workspaceAgent.ID),
		func(ctx context.Context, message []byte) {
			if endOfLogs.Load() {
				return
			}
			jlMsg := agentsdk.StartupLogsNotifyMessage{}
			err := json.Unmarshal(message, &jlMsg)
			if err != nil {
				logger.Warn(ctx, "invalid startup logs notify message", slog.Error(err))
				return
			}

			if jlMsg.CreatedAfter != 0 {
				logs, err := api.Database.GetWorkspaceAgentStartupLogsAfter(dbauthz.As(ctx, actor), database.GetWorkspaceAgentStartupLogsAfterParams{
					AgentID:      workspaceAgent.ID,
					CreatedAfter: jlMsg.CreatedAfter,
				})
				if err != nil {
					logger.Warn(ctx, "failed to get workspace agent startup logs after", slog.Error(err))
					return
				}
				sendLogs(logs)
			}

			if jlMsg.EndOfLogs {
				endOfLogs.Store(true)
				logs, err := api.Database.GetWorkspaceAgentStartupLogsAfter(dbauthz.As(ctx, actor), database.GetWorkspaceAgentStartupLogsAfterParams{
					AgentID:      workspaceAgent.ID,
					CreatedAfter: lastSentLogID.Load(),
				})
				if err != nil {
					logger.Warn(ctx, "get workspace agent startup logs after", slog.Error(err))
					return
				}
				sendLogs(logs)
				bufferedLogs <- nil
			}
		},
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to subscribe to startup logs.",
			Detail:  err.Error(),
		})
		return
	}
	defer closeSubscribe()

	for {
		select {
		case <-ctx.Done():
			logger.Debug(context.Background(), "job logs context canceled")
			return
		case logs, ok := <-bufferedLogs:
			// A nil log is sent when complete!
			if !ok || logs == nil {
				logger.Debug(context.Background(), "reached the end of published logs")
				return
			}
			err = encoder.Encode(convertWorkspaceAgentStartupLogs(logs))
			if err != nil {
				return
			}
		}
	}
}

// @Summary Get listening ports for workspace agent
// @ID get-listening-ports-for-workspace-agent
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceAgentListeningPortsResponse
// @Router /workspaceagents/{workspaceagent}/listening-ports [get]
func (api *API) workspaceAgentListeningPorts(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgentParam(r)

	apiAgent, err := convertWorkspaceAgent(
		api.DERPMap, *api.TailnetCoordinator.Load(), workspaceAgent, nil, api.AgentInactiveDisconnectTimeout,
		api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
	)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error reading workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	if apiAgent.Status != codersdk.WorkspaceAgentConnected {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Agent state is %q, it must be in the %q state.", apiAgent.Status, codersdk.WorkspaceAgentConnected),
		})
		return
	}

	agentConn, release, err := api.workspaceAgentCache.Acquire(workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error dialing workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	defer release()

	portsResponse, err := agentConn.ListeningPorts(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching listening ports.",
			Detail:  err.Error(),
		})
		return
	}

	// Get a list of ports that are in-use by applications.
	apps, err := api.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
	if xerrors.Is(err, sql.ErrNoRows) {
		apps = []database.WorkspaceApp{}
		err = nil
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace apps.",
			Detail:  err.Error(),
		})
		return
	}
	appPorts := make(map[uint16]struct{}, len(apps))
	for _, app := range apps {
		if !app.Url.Valid || app.Url.String == "" {
			continue
		}
		u, err := url.Parse(app.Url.String)
		if err != nil {
			continue
		}
		port := u.Port()
		if port == "" {
			continue
		}
		portNum, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			continue
		}
		if portNum < 1 || portNum > 65535 {
			continue
		}
		appPorts[uint16(portNum)] = struct{}{}
	}

	// Filter out ports that are globally blocked, in-use by applications, or
	// common non-HTTP ports such as databases, FTP, SSH, etc.
	filteredPorts := make([]codersdk.WorkspaceAgentListeningPort, 0, len(portsResponse.Ports))
	for _, port := range portsResponse.Ports {
		if port.Port < codersdk.WorkspaceAgentMinimumListeningPort {
			continue
		}
		if _, ok := appPorts[port.Port]; ok {
			continue
		}
		if _, ok := codersdk.WorkspaceAgentIgnoredListeningPorts[port.Port]; ok {
			continue
		}
		filteredPorts = append(filteredPorts, port)
	}

	portsResponse.Ports = filteredPorts
	httpapi.Write(ctx, rw, http.StatusOK, portsResponse)
}

func (api *API) dialWorkspaceAgentTailnet(agentID uuid.UUID) (*codersdk.WorkspaceAgentConn, error) {
	clientConn, serverConn := net.Pipe()
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:   api.DERPMap,
		Logger:    api.Logger.Named("tailnet"),
	})
	if err != nil {
		_ = clientConn.Close()
		_ = serverConn.Close()
		return nil, xerrors.Errorf("create tailnet conn: %w", err)
	}
	ctx, cancel := context.WithCancel(api.ctx)
	conn.SetDERPRegionDialer(func(_ context.Context, region *tailcfg.DERPRegion) net.Conn {
		if !region.EmbeddedRelay {
			return nil
		}
		left, right := net.Pipe()
		go func() {
			defer left.Close()
			defer right.Close()
			brw := bufio.NewReadWriter(bufio.NewReader(right), bufio.NewWriter(right))
			api.DERPServer.Accept(ctx, right, brw, "internal")
		}()
		return left
	})

	sendNodes, _ := tailnet.ServeCoordinator(clientConn, func(node []*tailnet.Node) error {
		err = conn.UpdateNodes(node, true)
		if err != nil {
			return xerrors.Errorf("update nodes: %w", err)
		}
		return nil
	})
	conn.SetNodeCallback(sendNodes)
	agentConn := &codersdk.WorkspaceAgentConn{
		Conn: conn,
		CloseFunc: func() {
			cancel()
			_ = clientConn.Close()
			_ = serverConn.Close()
		},
	}
	go func() {
		err := (*api.TailnetCoordinator.Load()).ServeClient(serverConn, uuid.New(), agentID)
		if err != nil {
			// Sometimes, we get benign closed pipe errors when the server is
			// shutting down.
			if api.ctx.Err() == nil {
				api.Logger.Warn(ctx, "tailnet coordinator client error", slog.Error(err))
			}
			_ = agentConn.Close()
		}
	}()
	if !agentConn.AwaitReachable(ctx) {
		_ = agentConn.Close()
		return nil, xerrors.Errorf("agent not reachable")
	}
	return agentConn, nil
}

// @Summary Get connection info for workspace agent
// @ID get-connection-info-for-workspace-agent
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceAgentConnectionInfo
// @Router /workspaceagents/{workspaceagent}/connection [get]
func (api *API) workspaceAgentConnection(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceAgentConnectionInfo{
		DERPMap: api.DERPMap,
	})
}

// @Summary Coordinate workspace agent via Tailnet
// @Description It accepts a WebSocket connection to an agent that listens to
// @Description incoming connections and publishes node updates.
// @ID coordinate-workspace-agent-via-tailnet
// @Security CoderSessionToken
// @Tags Agents
// @Success 101
// @Router /workspaceagents/me/coordinate [get]
func (api *API) workspaceAgentCoordinate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()
	workspaceAgent := httpmw.WorkspaceAgent(r)
	resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	build, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching workspace build job.",
			Detail:  err.Error(),
		})
		return
	}

	workspace, err := api.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}

	owner, err := api.Database.GetUserByID(ctx, workspace.OwnerID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Internal error fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	// Ensure the resource is still valid!
	// We only accept agents for resources on the latest build.
	ensureLatestBuild := func() error {
		latestBuild, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, build.WorkspaceID)
		if err != nil {
			return err
		}
		if build.ID != latestBuild.ID {
			return xerrors.New("build is outdated")
		}
		return nil
	}

	err = ensureLatestBuild()
	if err != nil {
		api.Logger.Debug(ctx, "agent tried to connect from non-latest built",
			slog.F("resource", resource),
			slog.F("agent", workspaceAgent),
		)
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Agent trying to connect from non-latest build.",
			Detail:  err.Error(),
		})
		return
	}

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}

	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()

	// We use a custom heartbeat routine here instead of `httpapi.Heartbeat`
	// because we want to log the agent's last ping time.
	var lastPing atomic.Pointer[time.Time]
	lastPing.Store(ptr.Ref(time.Now())) // Since the agent initiated the request, assume it's alive.

	go pprof.Do(ctx, pprof.Labels("agent", workspaceAgent.ID.String()), func(ctx context.Context) {
		// TODO(mafredri): Is this too frequent? Use separate ping disconnect timeout?
		t := time.NewTicker(api.AgentConnectionUpdateFrequency)
		defer t.Stop()

		for {
			select {
			case <-t.C:
			case <-ctx.Done():
				return
			}

			// We don't need a context that times out here because the ping will
			// eventually go through. If the context times out, then other
			// websocket read operations will receive an error, obfuscating the
			// actual problem.
			err := conn.Ping(ctx)
			if err != nil {
				return
			}
			lastPing.Store(ptr.Ref(time.Now()))
		}
	})

	firstConnectedAt := workspaceAgent.FirstConnectedAt
	if !firstConnectedAt.Valid {
		firstConnectedAt = sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		}
	}
	lastConnectedAt := sql.NullTime{
		Time:  database.Now(),
		Valid: true,
	}
	disconnectedAt := workspaceAgent.DisconnectedAt
	updateConnectionTimes := func(ctx context.Context) error {
		//nolint:gocritic // We only update ourself.
		err = api.Database.UpdateWorkspaceAgentConnectionByID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAgentConnectionByIDParams{
			ID:               workspaceAgent.ID,
			FirstConnectedAt: firstConnectedAt,
			LastConnectedAt:  lastConnectedAt,
			DisconnectedAt:   disconnectedAt,
			UpdatedAt:        database.Now(),
			LastConnectedReplicaID: uuid.NullUUID{
				UUID:  api.ID,
				Valid: true,
			},
		})
		if err != nil {
			return err
		}
		return nil
	}

	defer func() {
		// If connection closed then context will be canceled, try to
		// ensure our final update is sent. By waiting at most the agent
		// inactive disconnect timeout we ensure that we don't block but
		// also guarantee that the agent will be considered disconnected
		// by normal status check.
		//
		// Use a system context as the agent has disconnected and that token
		// may no longer be valid.
		//nolint:gocritic
		ctx, cancel := context.WithTimeout(dbauthz.AsSystemRestricted(api.ctx), api.AgentInactiveDisconnectTimeout)
		defer cancel()

		// Only update timestamp if the disconnect is new.
		if !disconnectedAt.Valid {
			disconnectedAt = sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			}
		}
		err := updateConnectionTimes(ctx)
		if err != nil {
			// This is a bug with unit tests that cancel the app context and
			// cause this error log to be generated. We should fix the unit tests
			// as this is a valid log.
			//
			// The pq error occurs when the server is shutting down.
			if !xerrors.Is(err, context.Canceled) && !database.IsQueryCanceledError(err) {
				api.Logger.Error(ctx, "failed to update agent disconnect time",
					slog.Error(err),
					slog.F("workspace", build.WorkspaceID),
				)
			}
		}
		api.publishWorkspaceUpdate(ctx, build.WorkspaceID)
	}()

	err = updateConnectionTimes(ctx)
	if err != nil {
		_ = conn.Close(websocket.StatusGoingAway, err.Error())
		return
	}
	api.publishWorkspaceUpdate(ctx, build.WorkspaceID)

	api.Logger.Info(ctx, "accepting agent", slog.F("agent", workspaceAgent))

	defer conn.Close(websocket.StatusNormalClosure, "")

	closeChan := make(chan struct{})
	go func() {
		defer close(closeChan)
		err := (*api.TailnetCoordinator.Load()).ServeAgent(wsNetConn, workspaceAgent.ID,
			fmt.Sprintf("%s-%s-%s", owner.Username, workspace.Name, workspaceAgent.Name),
		)
		if err != nil {
			api.Logger.Warn(ctx, "tailnet coordinator agent error", slog.Error(err))
			_ = conn.Close(websocket.StatusInternalError, err.Error())
			return
		}
	}()
	ticker := time.NewTicker(api.AgentConnectionUpdateFrequency)
	defer ticker.Stop()
	for {
		select {
		case <-closeChan:
			return
		case <-ticker.C:
		}

		lastPing := *lastPing.Load()

		var connectionStatusChanged bool
		if time.Since(lastPing) > api.AgentInactiveDisconnectTimeout {
			if !disconnectedAt.Valid {
				connectionStatusChanged = true
				disconnectedAt = sql.NullTime{
					Time:  database.Now(),
					Valid: true,
				}
			}
		} else {
			connectionStatusChanged = disconnectedAt.Valid
			// TODO(mafredri): Should we update it here or allow lastConnectedAt to shadow it?
			disconnectedAt = sql.NullTime{}
			lastConnectedAt = sql.NullTime{
				Time:  database.Now(),
				Valid: true,
			}
		}
		err = updateConnectionTimes(ctx)
		if err != nil {
			_ = conn.Close(websocket.StatusGoingAway, err.Error())
			return
		}
		if connectionStatusChanged {
			api.publishWorkspaceUpdate(ctx, build.WorkspaceID)
		}
		err := ensureLatestBuild()
		if err != nil {
			// Disconnect agents that are no longer valid.
			_ = conn.Close(websocket.StatusGoingAway, "")
			return
		}
	}
}

// workspaceAgentClientCoordinate accepts a WebSocket that reads node network updates.
// After accept a PubSub starts listening for new connection node updates
// which are written to the WebSocket.
//
// @Summary Coordinate workspace agent
// @ID coordinate-workspace-agent
// @Security CoderSessionToken
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 101
// @Router /workspaceagents/{workspaceagent}/coordinate [get]
func (api *API) workspaceAgentClientCoordinate(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, rbac.ActionCreate, workspace.ExecutionRBAC()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	// This is used by Enterprise code to control the functionality of this route.
	override := api.WorkspaceClientCoordinateOverride.Load()
	if override != nil {
		overrideFunc := *override
		if overrideFunc != nil && overrideFunc(rw) {
			return
		}
	}

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()
	workspaceAgent := httpmw.WorkspaceAgentParam(r)

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()

	go httpapi.Heartbeat(ctx, conn)

	defer conn.Close(websocket.StatusNormalClosure, "")
	err = (*api.TailnetCoordinator.Load()).ServeClient(wsNetConn, uuid.New(), workspaceAgent.ID)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
}

func convertApps(dbApps []database.WorkspaceApp) []codersdk.WorkspaceApp {
	apps := make([]codersdk.WorkspaceApp, 0)
	for _, dbApp := range dbApps {
		apps = append(apps, codersdk.WorkspaceApp{
			ID:           dbApp.ID,
			URL:          dbApp.Url.String,
			External:     dbApp.External,
			Slug:         dbApp.Slug,
			DisplayName:  dbApp.DisplayName,
			Command:      dbApp.Command.String,
			Icon:         dbApp.Icon,
			Subdomain:    dbApp.Subdomain,
			SharingLevel: codersdk.WorkspaceAppSharingLevel(dbApp.SharingLevel),
			Healthcheck: codersdk.Healthcheck{
				URL:       dbApp.HealthcheckUrl,
				Interval:  dbApp.HealthcheckInterval,
				Threshold: dbApp.HealthcheckThreshold,
			},
			Health: codersdk.WorkspaceAppHealth(dbApp.Health),
		})
	}
	return apps
}

func convertWorkspaceAgentMetadataDesc(mds []database.WorkspaceAgentMetadatum) []codersdk.WorkspaceAgentMetadataDescription {
	metadata := make([]codersdk.WorkspaceAgentMetadataDescription, 0)
	for _, datum := range mds {
		metadata = append(metadata, codersdk.WorkspaceAgentMetadataDescription{
			DisplayName: datum.DisplayName,
			Key:         datum.Key,
			Script:      datum.Script,
			Interval:    datum.Interval,
			Timeout:     datum.Timeout,
		})
	}
	return metadata
}

func convertWorkspaceAgent(derpMap *tailcfg.DERPMap, coordinator tailnet.Coordinator, dbAgent database.WorkspaceAgent, apps []codersdk.WorkspaceApp, agentInactiveDisconnectTimeout time.Duration, agentFallbackTroubleshootingURL string) (codersdk.WorkspaceAgent, error) {
	var envs map[string]string
	if dbAgent.EnvironmentVariables.Valid {
		err := json.Unmarshal(dbAgent.EnvironmentVariables.RawMessage, &envs)
		if err != nil {
			return codersdk.WorkspaceAgent{}, xerrors.Errorf("unmarshal env vars: %w", err)
		}
	}
	troubleshootingURL := agentFallbackTroubleshootingURL
	if dbAgent.TroubleshootingURL != "" {
		troubleshootingURL = dbAgent.TroubleshootingURL
	}
	workspaceAgent := codersdk.WorkspaceAgent{
		ID:                           dbAgent.ID,
		CreatedAt:                    dbAgent.CreatedAt,
		UpdatedAt:                    dbAgent.UpdatedAt,
		ResourceID:                   dbAgent.ResourceID,
		InstanceID:                   dbAgent.AuthInstanceID.String,
		Name:                         dbAgent.Name,
		Architecture:                 dbAgent.Architecture,
		OperatingSystem:              dbAgent.OperatingSystem,
		StartupScript:                dbAgent.StartupScript.String,
		StartupLogsLength:            dbAgent.StartupLogsLength,
		StartupLogsOverflowed:        dbAgent.StartupLogsOverflowed,
		Version:                      dbAgent.Version,
		EnvironmentVariables:         envs,
		Directory:                    dbAgent.Directory,
		ExpandedDirectory:            dbAgent.ExpandedDirectory,
		Apps:                         apps,
		ConnectionTimeoutSeconds:     dbAgent.ConnectionTimeoutSeconds,
		TroubleshootingURL:           troubleshootingURL,
		LifecycleState:               codersdk.WorkspaceAgentLifecycle(dbAgent.LifecycleState),
		LoginBeforeReady:             dbAgent.LoginBeforeReady,
		StartupScriptTimeoutSeconds:  dbAgent.StartupScriptTimeoutSeconds,
		ShutdownScript:               dbAgent.ShutdownScript.String,
		ShutdownScriptTimeoutSeconds: dbAgent.ShutdownScriptTimeoutSeconds,
	}
	node := coordinator.Node(dbAgent.ID)
	if node != nil {
		workspaceAgent.DERPLatency = map[string]codersdk.DERPRegion{}
		for rawRegion, latency := range node.DERPLatency {
			regionParts := strings.SplitN(rawRegion, "-", 2)
			regionID, err := strconv.Atoi(regionParts[0])
			if err != nil {
				return codersdk.WorkspaceAgent{}, xerrors.Errorf("convert derp region id %q: %w", rawRegion, err)
			}
			region, found := derpMap.Regions[regionID]
			if !found {
				// It's possible that a workspace agent is using an old DERPMap
				// and reports regions that do not exist. If that's the case,
				// report the region as unknown!
				region = &tailcfg.DERPRegion{
					RegionID:   regionID,
					RegionName: fmt.Sprintf("Unnamed %d", regionID),
				}
			}
			workspaceAgent.DERPLatency[region.RegionName] = codersdk.DERPRegion{
				Preferred:           node.PreferredDERP == regionID,
				LatencyMilliseconds: latency * 1000,
			}
		}
	}

	status := dbAgent.Status(agentInactiveDisconnectTimeout)
	workspaceAgent.Status = codersdk.WorkspaceAgentStatus(status.Status)
	workspaceAgent.FirstConnectedAt = status.FirstConnectedAt
	workspaceAgent.LastConnectedAt = status.LastConnectedAt
	workspaceAgent.DisconnectedAt = status.DisconnectedAt

	return workspaceAgent, nil
}

// @Summary Submit workspace agent stats
// @ID submit-workspace-agent-stats
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.Stats true "Stats request"
// @Success 200 {object} agentsdk.StatsResponse
// @Router /workspaceagents/me/report-stats [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentReportStats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workspaceAgent := httpmw.WorkspaceAgent(r)
	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	var req agentsdk.Stats
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// An empty stat means it's just looking for the report interval.
	if req.ConnectionsByProto == nil {
		httpapi.Write(ctx, rw, http.StatusOK, agentsdk.StatsResponse{
			ReportInterval: api.AgentStatsRefreshInterval,
		})
		return
	}

	api.Logger.Debug(ctx, "read stats report",
		slog.F("interval", api.AgentStatsRefreshInterval),
		slog.F("agent", workspaceAgent.ID),
		slog.F("workspace", workspace.ID),
		slog.F("payload", req),
	)

	if req.ConnectionCount > 0 {
		activityBumpWorkspace(ctx, api.Logger.Named("activity_bump"), api.Database, workspace.ID)
	}

	payload, err := json.Marshal(req.ConnectionsByProto)
	if err != nil {
		api.Logger.Error(ctx, "marshal agent connections by proto", slog.F("workspace_agent", workspaceAgent.ID), slog.Error(err))
		payload = json.RawMessage("{}")
	}

	now := database.Now()
	_, err = api.Database.InsertWorkspaceAgentStat(ctx, database.InsertWorkspaceAgentStatParams{
		ID:                          uuid.New(),
		CreatedAt:                   now,
		AgentID:                     workspaceAgent.ID,
		WorkspaceID:                 workspace.ID,
		UserID:                      workspace.OwnerID,
		TemplateID:                  workspace.TemplateID,
		ConnectionsByProto:          payload,
		ConnectionCount:             req.ConnectionCount,
		RxPackets:                   req.RxPackets,
		RxBytes:                     req.RxBytes,
		TxPackets:                   req.TxPackets,
		TxBytes:                     req.TxBytes,
		SessionCountVSCode:          req.SessionCountVSCode,
		SessionCountJetBrains:       req.SessionCountJetBrains,
		SessionCountReconnectingPTY: req.SessionCountReconnectingPTY,
		SessionCountSSH:             req.SessionCountSSH,
		ConnectionMedianLatencyMS:   req.ConnectionMedianLatencyMS,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	if req.ConnectionCount > 0 {
		err = api.Database.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
			ID:         workspace.ID,
			LastUsedAt: now,
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.StatsResponse{
		ReportInterval: api.AgentStatsRefreshInterval,
	})
}

// @Summary Submit workspace agent metadata
// @ID submit-workspace-agent-metadata
// @Security CoderSessionToken
// @Accept json
// @Tags Agents
// @Param request body agentsdk.PostMetadataRequest true "Workspace agent metadata request"
// @Param key path string true "metadata key" format(string)
// @Success 204 "Success"
// @Router /workspaceagents/me/metadata/{key} [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentPostMetadata(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req agentsdk.PostMetadataRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	workspaceAgent := httpmw.WorkspaceAgent(r)

	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	key := chi.URLParam(r, "key")

	const (
		maxValueLen = 32 << 10
		maxErrorLen = maxValueLen
	)

	metadataError := req.Error

	// We overwrite the error if the provided payload is too long.
	if len(req.Value) > maxValueLen {
		metadataError = fmt.Sprintf("value of %d bytes exceeded %d bytes", len(req.Value), maxValueLen)
		req.Value = req.Value[:maxValueLen]
	}

	if len(req.Error) > maxErrorLen {
		metadataError = fmt.Sprintf("error of %d bytes exceeded %d bytes", len(req.Error), maxErrorLen)
		req.Error = req.Error[:maxErrorLen]
	}

	datum := database.UpdateWorkspaceAgentMetadataParams{
		WorkspaceAgentID: workspaceAgent.ID,
		// We don't want a misconfigured agent to fill the database.
		Key:   key,
		Value: req.Value,
		Error: metadataError,
		// We ignore the CollectedAt from the agent to avoid bugs caused by
		// clock skew.
		CollectedAt: time.Now(),
	}

	err = api.Database.UpdateWorkspaceAgentMetadata(ctx, datum)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	api.Logger.Debug(
		ctx, "accepted metadata report",
		slog.F("agent", workspaceAgent.ID),
		slog.F("workspace", workspace.ID),
		slog.F("collected_at", datum.CollectedAt),
		slog.F("key", datum.Key),
	)

	err = api.Pubsub.Publish(watchWorkspaceAgentMetadataChannel(workspaceAgent.ID), []byte(datum.Key))
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

// @Summary Watch for workspace agent metadata updates
// @ID watch-for-workspace-agent-metadata-updates
// @Security CoderSessionToken
// @Tags Agents
// @Success 200 "Success"
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Router /workspaceagents/{workspaceagent}/watch-metadata [get]
// @x-apidocgen {"skip": true}
func (api *API) watchWorkspaceAgentMetadata(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx            = r.Context()
		workspaceAgent = httpmw.WorkspaceAgentParam(r)
	)

	sendEvent, senderClosed, err := httpapi.ServerSentEventSender(rw, r)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error setting up server-sent events.",
			Detail:  err.Error(),
		})
		return
	}
	// Prevent handler from returning until the sender is closed.
	defer func() {
		<-senderClosed
	}()

	const refreshInterval = time.Second * 5
	refreshTicker := time.NewTicker(refreshInterval)
	defer refreshTicker.Stop()

	var (
		lastDBMetaMu sync.Mutex
		lastDBMeta   []database.WorkspaceAgentMetadatum
	)

	sendMetadata := func(pull bool) {
		lastDBMetaMu.Lock()
		defer lastDBMetaMu.Unlock()

		var err error
		if pull {
			// We always use the original Request context because it contains
			// the RBAC actor.
			lastDBMeta, err = api.Database.GetWorkspaceAgentMetadata(ctx, workspaceAgent.ID)
			if err != nil {
				_ = sendEvent(ctx, codersdk.ServerSentEvent{
					Type: codersdk.ServerSentEventTypeError,
					Data: codersdk.Response{
						Message: "Internal error getting metadata.",
						Detail:  err.Error(),
					},
				})
				return
			}
			slices.SortFunc(lastDBMeta, func(i, j database.WorkspaceAgentMetadatum) bool {
				return i.Key < j.Key
			})

			// Avoid sending refresh if the client is about to get a
			// fresh update.
			refreshTicker.Reset(refreshInterval)
		}

		_ = sendEvent(ctx, codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: convertWorkspaceAgentMetadata(lastDBMeta),
		})
	}

	// Send initial metadata.
	sendMetadata(true)

	// We debounce metadata updates to avoid overloading the frontend when
	// an agent is sending a lot of updates.
	pubsubDebounce := debounce.New(time.Second)
	if flag.Lookup("test.v") != nil {
		pubsubDebounce = debounce.New(time.Millisecond * 100)
	}

	// Send metadata on updates.
	cancelSub, err := api.Pubsub.Subscribe(watchWorkspaceAgentMetadataChannel(workspaceAgent.ID), func(_ context.Context, _ []byte) {
		pubsubDebounce(func() {
			sendMetadata(true)
		})
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	defer cancelSub()

	for {
		select {
		case <-senderClosed:
			return
		case <-refreshTicker.C:
			break
		}

		// Avoid spamming the DB with reads we know there are no updates. We want
		// to continue sending updates to the frontend so that "Result.Age"
		// is always accurate. This way, the frontend doesn't need to
		// sync its own clock with the backend.
		sendMetadata(false)
	}
}

func convertWorkspaceAgentMetadata(db []database.WorkspaceAgentMetadatum) []codersdk.WorkspaceAgentMetadata {
	// An empty array is easier for clients to handle than a null.
	result := []codersdk.WorkspaceAgentMetadata{}
	for _, datum := range db {
		result = append(result, codersdk.WorkspaceAgentMetadata{
			Result: codersdk.WorkspaceAgentMetadataResult{
				Value:       datum.Value,
				Error:       datum.Error,
				CollectedAt: datum.CollectedAt,
				Age:         int64(time.Since(datum.CollectedAt).Seconds()),
			},
			Description: codersdk.WorkspaceAgentMetadataDescription{
				DisplayName: datum.DisplayName,
				Key:         datum.Key,
				Script:      datum.Script,
				Interval:    datum.Interval,
				Timeout:     datum.Timeout,
			},
		})
	}
	return result
}

func watchWorkspaceAgentMetadataChannel(id uuid.UUID) string {
	return "workspace_agent_metadata:" + id.String()
}

// @Summary Submit workspace agent lifecycle state
// @ID submit-workspace-agent-lifecycle-state
// @Security CoderSessionToken
// @Accept json
// @Tags Agents
// @Param request body agentsdk.PostLifecycleRequest true "Workspace agent lifecycle request"
// @Success 204 "Success"
// @Router /workspaceagents/me/report-lifecycle [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentReportLifecycle(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workspaceAgent := httpmw.WorkspaceAgent(r)
	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	var req agentsdk.PostLifecycleRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	api.Logger.Debug(ctx, "workspace agent state report",
		slog.F("agent", workspaceAgent.ID),
		slog.F("workspace", workspace.ID),
		slog.F("payload", req),
	)

	lifecycleState := database.WorkspaceAgentLifecycleState(req.State)
	if !lifecycleState.Valid() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid lifecycle state.",
			Detail:  fmt.Sprintf("Invalid lifecycle state %q, must be be one of %q.", req.State, database.AllWorkspaceAgentLifecycleStateValues()),
		})
		return
	}

	err = api.Database.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
		ID:             workspaceAgent.ID,
		LifecycleState: lifecycleState,
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	api.publishWorkspaceUpdate(ctx, workspace.ID)

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

// @Summary Submit workspace agent application health
// @ID submit-workspace-agent-application-health
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.PostAppHealthsRequest true "Application health request"
// @Success 200
// @Router /workspaceagents/me/app-health [post]
func (api *API) postWorkspaceAppHealth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgent(r)
	var req agentsdk.PostAppHealthsRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.Healths == nil || len(req.Healths) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Health field is empty",
		})
		return
	}

	apps, err := api.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error getting agent apps",
			Detail:  err.Error(),
		})
		return
	}

	var newApps []database.WorkspaceApp
	for id, newHealth := range req.Healths {
		old := func() *database.WorkspaceApp {
			for _, app := range apps {
				if app.ID == id {
					return &app
				}
			}

			return nil
		}()
		if old == nil {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Error setting workspace app health",
				Detail:  xerrors.Errorf("workspace app name %s not found", id).Error(),
			})
			return
		}

		if old.HealthcheckUrl == "" {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Error setting workspace app health",
				Detail:  xerrors.Errorf("health checking is disabled for workspace app %s", id).Error(),
			})
			return
		}

		switch newHealth {
		case codersdk.WorkspaceAppHealthInitializing:
		case codersdk.WorkspaceAppHealthHealthy:
		case codersdk.WorkspaceAppHealthUnhealthy:
		default:
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Error setting workspace app health",
				Detail:  xerrors.Errorf("workspace app health %s is not a valid value", newHealth).Error(),
			})
			return
		}

		// don't save if the value hasn't changed
		if old.Health == database.WorkspaceAppHealth(newHealth) {
			continue
		}
		old.Health = database.WorkspaceAppHealth(newHealth)

		newApps = append(newApps, *old)
	}

	for _, app := range newApps {
		err = api.Database.UpdateWorkspaceAppHealthByID(ctx, database.UpdateWorkspaceAppHealthByIDParams{
			ID:     app.ID,
			Health: app.Health,
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Error setting workspace app health",
				Detail:  err.Error(),
			})
			return
		}
	}

	resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace resource.",
			Detail:  err.Error(),
		})
		return
	}
	job, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	workspace, err := api.Database.GetWorkspaceByID(ctx, job.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace.",
			Detail:  err.Error(),
		})
		return
	}
	api.publishWorkspaceUpdate(ctx, workspace.ID)

	httpapi.Write(ctx, rw, http.StatusOK, nil)
}

// workspaceAgentsGitAuth returns a username and password for use
// with GIT_ASKPASS.
//
// @Summary Get workspace agent Git auth
// @ID get-workspace-agent-git-auth
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param url query string true "Git URL" format(uri)
// @Param listen query bool false "Wait for a new token to be issued"
// @Success 200 {object} agentsdk.GitAuthResponse
// @Router /workspaceagents/me/gitauth [get]
func (api *API) workspaceAgentsGitAuth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gitURL := r.URL.Query().Get("url")
	if gitURL == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing 'url' query parameter!",
		})
		return
	}
	// listen determines if the request will wait for a
	// new token to be issued!
	listen := r.URL.Query().Has("listen")

	var gitAuthConfig *gitauth.Config
	for _, gitAuth := range api.GitAuthConfigs {
		matches := gitAuth.Regex.MatchString(gitURL)
		if !matches {
			continue
		}
		gitAuthConfig = gitAuth
	}
	if gitAuthConfig == nil {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("No git provider found for URL %q", gitURL),
		})
		return
	}
	workspaceAgent := httpmw.WorkspaceAgent(r)
	// We must get the workspace to get the owner ID!
	resource, err := api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get workspace resource.",
			Detail:  err.Error(),
		})
		return
	}
	build, err := api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get build.",
			Detail:  err.Error(),
		})
		return
	}
	workspace, err := api.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}

	if listen {
		// If listening we await a new token...
		authChan := make(chan struct{}, 1)
		cancelFunc, err := api.Pubsub.Subscribe("gitauth", func(ctx context.Context, message []byte) {
			ids := strings.Split(string(message), "|")
			if len(ids) != 2 {
				return
			}
			if ids[0] != gitAuthConfig.ID {
				return
			}
			if ids[1] != workspace.OwnerID.String() {
				return
			}
			select {
			case authChan <- struct{}{}:
			default:
			}
		})
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to listen for git auth token.",
				Detail:  err.Error(),
			})
			return
		}
		defer cancelFunc()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-authChan:
			}
			gitAuthLink, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
				ProviderID: gitAuthConfig.ID,
				UserID:     workspace.OwnerID,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to get git auth link.",
					Detail:  err.Error(),
				})
				return
			}
			if gitAuthLink.OAuthExpiry.Before(database.Now()) {
				continue
			}
			if gitAuthConfig.ValidateURL != "" {
				valid, err := gitAuthConfig.ValidateToken(ctx, gitAuthLink.OAuthAccessToken)
				if err != nil {
					api.Logger.Warn(ctx, "failed to validate git auth token",
						slog.F("workspace_owner_id", workspace.OwnerID.String()),
						slog.F("validate_url", gitAuthConfig.ValidateURL),
						slog.Error(err),
					)
				}
				if !valid {
					continue
				}
			}
			httpapi.Write(ctx, rw, http.StatusOK, formatGitAuthAccessToken(gitAuthConfig.Type, gitAuthLink.OAuthAccessToken))
			return
		}
	}

	// This is the URL that will redirect the user with a state token.
	redirectURL, err := api.AccessURL.Parse(fmt.Sprintf("/gitauth/%s", gitAuthConfig.ID))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to parse access URL.",
			Detail:  err.Error(),
		})
		return
	}

	gitAuthLink, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
		ProviderID: gitAuthConfig.ID,
		UserID:     workspace.OwnerID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get git auth link.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(ctx, rw, http.StatusOK, agentsdk.GitAuthResponse{
			URL: redirectURL.String(),
		})
		return
	}

	gitAuthLink, updated, err := gitAuthConfig.RefreshToken(ctx, api.Database, gitAuthLink)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to refresh git auth token.",
			Detail:  err.Error(),
		})
		return
	}
	if !updated {
		httpapi.Write(ctx, rw, http.StatusOK, agentsdk.GitAuthResponse{
			URL: redirectURL.String(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, formatGitAuthAccessToken(gitAuthConfig.Type, gitAuthLink.OAuthAccessToken))
}

// Provider types have different username/password formats.
func formatGitAuthAccessToken(typ codersdk.GitProvider, token string) agentsdk.GitAuthResponse {
	var resp agentsdk.GitAuthResponse
	switch typ {
	case codersdk.GitProviderGitLab:
		// https://stackoverflow.com/questions/25409700/using-gitlab-token-to-clone-without-authentication
		resp = agentsdk.GitAuthResponse{
			Username: "oauth2",
			Password: token,
		}
	case codersdk.GitProviderBitBucket:
		// https://support.atlassian.com/bitbucket-cloud/docs/use-oauth-on-bitbucket-cloud/#Cloning-a-repository-with-an-access-token
		resp = agentsdk.GitAuthResponse{
			Username: "x-token-auth",
			Password: token,
		}
	default:
		resp = agentsdk.GitAuthResponse{
			Username: token,
		}
	}
	return resp
}

func (api *API) gitAuthCallback(gitAuthConfig *gitauth.Config) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx    = r.Context()
			state  = httpmw.OAuth2(r)
			apiKey = httpmw.APIKey(r)
		)

		_, err := api.Database.GetGitAuthLink(ctx, database.GetGitAuthLinkParams{
			ProviderID: gitAuthConfig.ID,
			UserID:     apiKey.UserID,
		})
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to get git auth link.",
					Detail:  err.Error(),
				})
				return
			}

			_, err = api.Database.InsertGitAuthLink(ctx, database.InsertGitAuthLinkParams{
				ProviderID:        gitAuthConfig.ID,
				UserID:            apiKey.UserID,
				CreatedAt:         database.Now(),
				UpdatedAt:         database.Now(),
				OAuthAccessToken:  state.Token.AccessToken,
				OAuthRefreshToken: state.Token.RefreshToken,
				OAuthExpiry:       state.Token.Expiry,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to insert git auth link.",
					Detail:  err.Error(),
				})
				return
			}
		} else {
			_, err = api.Database.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
				ProviderID:        gitAuthConfig.ID,
				UserID:            apiKey.UserID,
				UpdatedAt:         database.Now(),
				OAuthAccessToken:  state.Token.AccessToken,
				OAuthRefreshToken: state.Token.RefreshToken,
				OAuthExpiry:       state.Token.Expiry,
			})
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Failed to update git auth link.",
					Detail:  err.Error(),
				})
				return
			}
		}

		err = api.Pubsub.Publish("gitauth", []byte(fmt.Sprintf("%s|%s", gitAuthConfig.ID, apiKey.UserID)))
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to publish auth update.",
				Detail:  err.Error(),
			})
			return
		}

		redirect := state.Redirect
		if redirect == "" {
			redirect = "/gitauth"
		}
		// This is a nicely rendered screen on the frontend
		http.Redirect(rw, r, redirect, http.StatusTemporaryRedirect)
	}
}

// wsNetConn wraps net.Conn created by websocket.NetConn(). Cancel func
// is called if a read or write error is encountered.
type wsNetConn struct {
	cancel context.CancelFunc
	net.Conn
}

func (c *wsNetConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c *wsNetConn) Close() error {
	defer c.cancel()
	return c.Conn.Close()
}

// websocketNetConn wraps websocket.NetConn and returns a context that
// is tied to the parent context and the lifetime of the conn. Any error
// during read or write will cancel the context, but not close the
// conn. Close should be called to release context resources.
func websocketNetConn(ctx context.Context, conn *websocket.Conn, msgType websocket.MessageType) (context.Context, net.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	nc := websocket.NetConn(ctx, conn, msgType)
	return ctx, &wsNetConn{
		cancel: cancel,
		Conn:   nc,
	}
}

func convertWorkspaceAgentStartupLogs(logs []database.WorkspaceAgentStartupLog) []codersdk.WorkspaceAgentStartupLog {
	sdk := make([]codersdk.WorkspaceAgentStartupLog, 0, len(logs))
	for _, log := range logs {
		sdk = append(sdk, convertWorkspaceAgentStartupLog(log))
	}
	return sdk
}

func convertWorkspaceAgentStartupLog(log database.WorkspaceAgentStartupLog) codersdk.WorkspaceAgentStartupLog {
	return codersdk.WorkspaceAgentStartupLog{
		ID:        log.ID,
		CreatedAt: log.CreatedAt,
		Output:    log.Output,
		Level:     codersdk.LogLevel(log.Level),
	}
}
