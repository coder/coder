package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	maputil "github.com/coder/coder/v2/coderd/util/maps"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/websocket"
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

	var (
		dbApps     []database.WorkspaceApp
		scripts    []database.WorkspaceAgentScript
		logSources []database.WorkspaceAgentLogSource
	)

	var eg errgroup.Group
	eg.Go(func() (err error) {
		dbApps, err = api.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
		return err
	})
	eg.Go(func() (err error) {
		//nolint:gocritic // TODO: can we make this not require system restricted?
		scripts, err = api.Database.GetWorkspaceAgentScriptsByAgentIDs(dbauthz.AsSystemRestricted(ctx), []uuid.UUID{workspaceAgent.ID})
		return err
	})
	eg.Go(func() (err error) {
		//nolint:gocritic // TODO: can we make this not require system restricted?
		logSources, err = api.Database.GetWorkspaceAgentLogSourcesByAgentIDs(dbauthz.AsSystemRestricted(ctx), []uuid.UUID{workspaceAgent.ID})
		return err
	})
	err := eg.Wait()
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent.",
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

	apiAgent, err := db2sdk.WorkspaceAgent(
		api.DERPMap(), *api.TailnetCoordinator.Load(), workspaceAgent, db2sdk.Apps(dbApps, workspaceAgent, owner.Username, workspace), convertScripts(scripts), convertLogSources(logSources), api.AgentInactiveDisconnectTimeout,
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

const AgentAPIVersionREST = "1.0"

// @Summary Patch workspace agent logs
// @ID patch-workspace-agent-logs
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.PatchLogs true "logs"
// @Success 200 {object} codersdk.Response
// @Router /workspaceagents/me/logs [patch]
func (api *API) patchWorkspaceAgentLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgent(r)

	var req agentsdk.PatchLogs
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if len(req.Logs) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "No logs provided.",
		})
		return
	}
	// This is to support the legacy API where the log source ID was
	// not provided in the request body. We default to the external
	// log source in this case.
	if req.LogSourceID == uuid.Nil {
		// Use the external log source
		externalSources, err := api.Database.InsertWorkspaceAgentLogSources(ctx, database.InsertWorkspaceAgentLogSourcesParams{
			WorkspaceAgentID: workspaceAgent.ID,
			CreatedAt:        dbtime.Now(),
			ID:               []uuid.UUID{agentsdk.ExternalLogSourceID},
			DisplayName:      []string{"External"},
			Icon:             []string{"/emojis/1f310.png"},
		})
		if database.IsUniqueViolation(err, database.UniqueWorkspaceAgentLogSourcesPkey) {
			err = nil
			req.LogSourceID = agentsdk.ExternalLogSourceID
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create external log source.",
				Detail:  err.Error(),
			})
			return
		}
		if len(externalSources) == 1 {
			req.LogSourceID = externalSources[0].ID
		}
	}
	output := make([]string, 0)
	level := make([]database.LogLevel, 0)
	outputLength := 0
	for _, logEntry := range req.Logs {
		output = append(output, logEntry.Output)
		outputLength += len(logEntry.Output)
		if logEntry.Level == "" {
			// Default to "info" to support older agents that didn't have the level field.
			logEntry.Level = codersdk.LogLevelInfo
		}
		parsedLevel := database.LogLevel(logEntry.Level)
		if !parsedLevel.Valid() {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid log level provided.",
				Detail:  fmt.Sprintf("invalid log level: %q", logEntry.Level),
			})
			return
		}
		level = append(level, parsedLevel)
	}

	logs, err := api.Database.InsertWorkspaceAgentLogs(ctx, database.InsertWorkspaceAgentLogsParams{
		AgentID:      workspaceAgent.ID,
		CreatedAt:    dbtime.Now(),
		Output:       output,
		Level:        level,
		LogSourceID:  req.LogSourceID,
		OutputLength: int32(outputLength),
	})
	if err != nil {
		if !database.IsWorkspaceAgentLogsLimitError(err) {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to upload logs",
				Detail:  err.Error(),
			})
			return
		}
		if workspaceAgent.LogsOverflowed {
			httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
				Message: "Logs limit exceeded",
				Detail:  err.Error(),
			})
			return
		}
		err := api.Database.UpdateWorkspaceAgentLogOverflowByID(ctx, database.UpdateWorkspaceAgentLogOverflowByIDParams{
			ID:             workspaceAgent.ID,
			LogsOverflowed: true,
		})
		if err != nil {
			// We don't want to return here, because the agent will retry
			// on failure and this isn't a huge deal. The overflow state
			// is just a hint to the user that the logs are incomplete.
			api.Logger.Warn(ctx, "failed to update workspace agent log overflow", slog.Error(err))
		}

		workspace, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to get workspace.",
				Detail:  err.Error(),
			})
			return
		}

		api.publishWorkspaceUpdate(ctx, workspace.OwnerID, wspubsub.WorkspaceEvent{
			Kind:        wspubsub.WorkspaceEventKindAgentLogsOverflow,
			WorkspaceID: workspace.ID,
			AgentID:     &workspaceAgent.ID,
		})

		httpapi.Write(ctx, rw, http.StatusRequestEntityTooLarge, codersdk.Response{
			Message: "Logs limit exceeded",
		})
		return
	}

	lowestLogID := logs[0].ID

	// Publish by the lowest log ID inserted so the
	// log stream will fetch everything from that point.
	api.publishWorkspaceAgentLogsUpdate(ctx, workspaceAgent.ID, agentsdk.LogsNotifyMessage{
		CreatedAfter: lowestLogID - 1,
	})

	if workspaceAgent.LogsLength == 0 {
		// If these are the first logs being appended, we publish a UI update
		// to notify the UI that logs are now available.
		workspace, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to get workspace.",
				Detail:  err.Error(),
			})
			return
		}

		api.publishWorkspaceUpdate(ctx, workspace.OwnerID, wspubsub.WorkspaceEvent{
			Kind:        wspubsub.WorkspaceEventKindAgentFirstLogs,
			WorkspaceID: workspace.ID,
			AgentID:     &workspaceAgent.ID,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, nil)
}

// workspaceAgentLogs returns the logs associated with a workspace agent
//
// @Summary Get logs by workspace agent
// @ID get-logs-by-workspace-agent
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Param before query int false "Before log id"
// @Param after query int false "After log id"
// @Param follow query bool false "Follow log stream"
// @Param no_compression query bool false "Disable compression for WebSocket connection"
// @Success 200 {array} codersdk.WorkspaceAgentLog
// @Router /workspaceagents/{workspaceagent}/logs [get]
func (api *API) workspaceAgentLogs(rw http.ResponseWriter, r *http.Request) {
	// This mostly copies how provisioner job logs are streamed!
	var (
		ctx            = r.Context()
		workspaceAgent = httpmw.WorkspaceAgentParam(r)
		logger         = api.Logger.With(slog.F("workspace_agent_id", workspaceAgent.ID))
		follow         = r.URL.Query().Has("follow")
		afterRaw       = r.URL.Query().Get("after")
		noCompression  = r.URL.Query().Has("no_compression")
	)

	var after int64
	// Only fetch logs created after the time provided.
	if afterRaw != "" {
		var err error
		after, err = strconv.ParseInt(afterRaw, 10, 64)
		if err != nil || after < 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Query param \"after\" must be an integer greater than or equal to zero.",
				Validations: []codersdk.ValidationError{
					{Field: "after", Detail: "Must be an integer greater than or equal to zero"},
				},
			})
			return
		}
	}

	logs, err := api.Database.GetWorkspaceAgentLogsAfter(ctx, database.GetWorkspaceAgentLogsAfterParams{
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
		logs = []database.WorkspaceAgentLog{}
	}

	if !follow {
		httpapi.Write(ctx, rw, http.StatusOK, convertWorkspaceAgentLogs(logs))
		return
	}

	workspace, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace by agent id.",
			Detail:  err.Error(),
		})
		return
	}

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()

	opts := &websocket.AcceptOptions{}

	// Allow client to request no compression. This is useful for buggy
	// clients or if there's a client/server incompatibility. This is
	// needed with e.g. coder/websocket and Safari (confirmed in 16.5).
	//
	// See:
	// * https://github.com/nhooyr/websocket/issues/218
	// * https://github.com/gobwas/ws/issues/169
	if noCompression {
		opts.CompressionMode = websocket.CompressionDisabled
	}

	conn, err := websocket.Accept(rw, r, opts)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	go httpapi.Heartbeat(ctx, conn)

	encoder := wsjson.NewEncoder[[]codersdk.WorkspaceAgentLog](conn, websocket.MessageText)
	defer encoder.Close(websocket.StatusNormalClosure)

	err = encoder.Encode(convertWorkspaceAgentLogs(logs))
	if err != nil {
		return
	}

	lastSentLogID := after
	if len(logs) > 0 {
		lastSentLogID = logs[len(logs)-1].ID
	}

	workspaceNotifyCh := make(chan struct{}, 1)
	notifyCh := make(chan struct{}, 1)
	// Allow us to immediately check if we missed any logs
	// between initial fetch and subscribe.
	notifyCh <- struct{}{}

	// Subscribe to workspace to detect new builds.
	closeSubscribeWorkspace, err := api.Pubsub.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspace.OwnerID),
		wspubsub.HandleWorkspaceEvent(
			func(_ context.Context, e wspubsub.WorkspaceEvent, err error) {
				if err != nil {
					return
				}
				if e.Kind == wspubsub.WorkspaceEventKindStateChange && e.WorkspaceID == workspace.ID {
					select {
					case workspaceNotifyCh <- struct{}{}:
					default:
					}
				}
			}))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to subscribe to workspace for log streaming.",
			Detail:  err.Error(),
		})
		return
	}
	defer closeSubscribeWorkspace()
	// Subscribe early to prevent missing log events.
	closeSubscribe, err := api.Pubsub.Subscribe(agentsdk.LogsNotifyChannel(workspaceAgent.ID), func(_ context.Context, _ []byte) {
		// The message is not important, we're tracking lastSentLogID manually.
		select {
		case notifyCh <- struct{}{}:
		default:
		}
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to subscribe to agent for log streaming.",
			Detail:  err.Error(),
		})
		return
	}
	defer closeSubscribe()

	// Buffer size controls the log prefetch capacity.
	bufferedLogs := make(chan []database.WorkspaceAgentLog, 8)
	// Check at least once per minute in case we didn't receive a pubsub message.
	recheckInterval := time.Minute
	t := time.NewTicker(recheckInterval)
	defer t.Stop()

	go func() {
		defer func() {
			logger.Debug(ctx, "end log streaming loop")
			close(bufferedLogs)
		}()
		logger.Debug(ctx, "start log streaming loop", slog.F("last_sent_log_id", lastSentLogID))

		keepGoing := true
		for keepGoing {
			var (
				debugTriggeredBy     string
				onlyCheckLatestBuild bool
			)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				debugTriggeredBy = "timer"
			case <-workspaceNotifyCh:
				debugTriggeredBy = "workspace"
				onlyCheckLatestBuild = true
			case <-notifyCh:
				debugTriggeredBy = "log"
				t.Reset(recheckInterval)
			}

			agents, err := api.Database.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
			if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
				if xerrors.Is(err, context.Canceled) {
					return
				}
				logger.Warn(ctx, "failed to get workspace agents in latest build", slog.Error(err))
				continue
			}
			// If the agent is no longer in the latest build, we can stop after
			// checking once.
			keepGoing = slices.ContainsFunc(agents, func(agent database.WorkspaceAgent) bool { return agent.ID == workspaceAgent.ID })

			logger.Debug(
				ctx,
				"checking for new logs",
				slog.F("triggered_by", debugTriggeredBy),
				slog.F("only_check_latest_build", onlyCheckLatestBuild),
				slog.F("keep_going", keepGoing),
				slog.F("last_sent_log_id", lastSentLogID),
				slog.F("workspace_has_agents", len(agents) > 0),
			)

			if onlyCheckLatestBuild && keepGoing {
				continue
			}

			logs, err := api.Database.GetWorkspaceAgentLogsAfter(ctx, database.GetWorkspaceAgentLogsAfterParams{
				AgentID:      workspaceAgent.ID,
				CreatedAfter: lastSentLogID,
			})
			if err != nil {
				if xerrors.Is(err, context.Canceled) {
					return
				}
				logger.Warn(ctx, "failed to get workspace agent logs after", slog.Error(err))
				continue
			}
			if len(logs) == 0 {
				// Just keep listening - more logs might come in the future!
				continue
			}

			select {
			case <-ctx.Done():
				return
			case bufferedLogs <- logs:
				lastSentLogID = logs[len(logs)-1].ID
			}
		}
	}()
	defer func() {
		// Ensure that we don't return until the goroutine has exited.
		//nolint:revive // Consume channel to wait until it's closed.
		for range bufferedLogs {
		}
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Debug(ctx, "job logs context canceled")
			return
		case logs, ok := <-bufferedLogs:
			if !ok {
				select {
				case <-ctx.Done():
					logger.Debug(ctx, "job logs context canceled")
				default:
					logger.Debug(ctx, "reached the end of published logs")
				}
				return
			}
			err = encoder.Encode(convertWorkspaceAgentLogs(logs))
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

	// If the agent is unreachable, the request will hang. Assume that if we
	// don't get a response after 30s that the agent is unreachable.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	apiAgent, err := db2sdk.WorkspaceAgent(
		api.DERPMap(), *api.TailnetCoordinator.Load(), workspaceAgent, nil, nil, nil, api.AgentInactiveDisconnectTimeout,
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

	agentConn, release, err := api.agentProvider.AgentConn(ctx, workspaceAgent.ID)
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
		if port.Port < workspacesdk.AgentMinimumListeningPort {
			continue
		}
		if _, ok := appPorts[port.Port]; ok {
			continue
		}
		if _, ok := workspacesdk.AgentIgnoredListeningPorts[port.Port]; ok {
			continue
		}
		filteredPorts = append(filteredPorts, port)
	}

	portsResponse.Ports = filteredPorts
	httpapi.Write(ctx, rw, http.StatusOK, portsResponse)
}

// @Summary Get running containers for workspace agent
// @ID get-running-containers-for-workspace-agent
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Param label query string true "Labels" format(key=value)
// @Success 200 {object} codersdk.WorkspaceAgentListContainersResponse
// @Router /workspaceagents/{workspaceagent}/containers [get]
func (api *API) workspaceAgentListContainers(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceAgent := httpmw.WorkspaceAgentParam(r)

	labelParam, ok := r.URL.Query()["label"]
	if !ok {
		labelParam = []string{}
	}
	labels := make(map[string]string, len(labelParam)/2)
	for _, label := range labelParam {
		kvs := strings.Split(label, "=")
		if len(kvs) != 2 {
			httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid label format",
				Detail:  "Labels must be in the format key=value",
			})
			return
		}
		labels[kvs[0]] = kvs[1]
	}

	// If the agent is unreachable, the request will hang. Assume that if we
	// don't get a response after 30s that the agent is unreachable.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	apiAgent, err := db2sdk.WorkspaceAgent(
		api.DERPMap(),
		*api.TailnetCoordinator.Load(),
		workspaceAgent,
		nil,
		nil,
		nil,
		api.AgentInactiveDisconnectTimeout,
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

	agentConn, release, err := api.agentProvider.AgentConn(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error dialing workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	defer release()

	// Get a list of containers that the agent is able to detect
	cts, err := agentConn.ListContainers(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			httpapi.Write(ctx, rw, http.StatusRequestTimeout, codersdk.Response{
				Message: "Failed to fetch containers from agent.",
				Detail:  "Request timed out.",
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching containers.",
			Detail:  err.Error(),
		})
		return
	}

	// Filter in-place by labels
	for idx, ct := range cts.Containers {
		if !maputil.Subset(labels, ct.Labels) {
			cts.Containers = append(cts.Containers[:idx], cts.Containers[idx+1:]...)
		}
	}
	// filtered := slices.DeleteFunc(cts.Containers, func(ct codersdk.WorkspaceAgentDevcontainer) bool {
	// return !maputil.Subset(labels, ct.Labels)
	// })

	httpapi.Write(ctx, rw, http.StatusOK, cts)
}

// @Summary Get connection info for workspace agent
// @ID get-connection-info-for-workspace-agent
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param workspaceagent path string true "Workspace agent ID" format(uuid)
// @Success 200 {object} workspacesdk.AgentConnectionInfo
// @Router /workspaceagents/{workspaceagent}/connection [get]
func (api *API) workspaceAgentConnection(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.AgentConnectionInfo{
		DERPMap:                  api.DERPMap(),
		DERPForceWebSockets:      api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		DisableDirectConnections: api.DeploymentValues.DERP.Config.BlockDirect.Value(),
	})
}

// workspaceAgentConnectionGeneric is the same as workspaceAgentConnection but
// without the workspaceagent path parameter.
//
// @Summary Get connection info for workspace agent generic
// @ID get-connection-info-for-workspace-agent-generic
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Success 200 {object} workspacesdk.AgentConnectionInfo
// @Router /workspaceagents/connection [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentConnectionGeneric(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.AgentConnectionInfo{
		DERPMap:                  api.DERPMap(),
		DERPForceWebSockets:      api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		DisableDirectConnections: api.DeploymentValues.DERP.Config.BlockDirect.Value(),
	})
}

// @Summary Get DERP map updates
// @ID get-derp-map-updates
// @Security CoderSessionToken
// @Tags Agents
// @Success 101
// @Router /derp-map [get]
func (api *API) derpMapUpdates(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()

	ws, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept websocket.",
			Detail:  err.Error(),
		})
		return
	}
	encoder := wsjson.NewEncoder[*tailcfg.DERPMap](ws, websocket.MessageBinary)
	defer encoder.Close(websocket.StatusGoingAway)

	go func(ctx context.Context) {
		// TODO(mafredri): Is this too frequent? Use separate ping disconnect timeout?
		t := time.NewTicker(api.AgentConnectionUpdateFrequency)
		defer t.Stop()

		for {
			select {
			case <-t.C:
			case <-ctx.Done():
				return
			}

			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := ws.Ping(ctx)
			cancel()
			if err != nil {
				_ = ws.Close(websocket.StatusGoingAway, "ping failed")
				return
			}
		}
	}(ctx)

	ticker := time.NewTicker(api.Options.DERPMapUpdateFrequency)
	defer ticker.Stop()

	var lastDERPMap *tailcfg.DERPMap
	for {
		derpMap := api.DERPMap()
		if lastDERPMap == nil || !tailnet.CompareDERPMaps(lastDERPMap, derpMap) {
			err := encoder.Encode(derpMap)
			if err != nil {
				return
			}
			lastDERPMap = derpMap
		}

		ticker.Reset(api.Options.DERPMapUpdateFrequency)
		select {
		case <-ctx.Done():
			return
		case <-api.ctx.Done():
			return
		case <-ticker.C:
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

	// This route accepts user API key auth and workspace proxy auth. The moon actor has
	// full permissions so should be able to pass this authz check.
	workspace := httpmw.WorkspaceParam(r)
	if !api.Authorize(r, policy.ActionSSH, workspace) {
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

	version := "1.0"
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

	peerID, err := api.handleResumeToken(ctx, rw, r)
	if err != nil {
		// handleResumeToken has already written the response.
		return
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
	ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()

	go httpapi.Heartbeat(ctx, conn)

	defer conn.Close(websocket.StatusNormalClosure, "")
	err = api.TailnetClientService.ServeClient(ctx, version, wsNetConn, tailnet.StreamID{
		Name: "client",
		ID:   peerID,
		Auth: tailnet.ClientCoordinateeAuth{
			AgentID: workspaceAgent.ID,
		},
	})
	if err != nil && !xerrors.Is(err, io.EOF) && !xerrors.Is(err, context.Canceled) {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
}

// handleResumeToken accepts a resume_token query parameter to use the same peer ID
func (api *API) handleResumeToken(ctx context.Context, rw http.ResponseWriter, r *http.Request) (peerID uuid.UUID, err error) {
	peerID = uuid.New()
	resumeToken := r.URL.Query().Get("resume_token")
	if resumeToken != "" {
		peerID, err = api.Options.CoordinatorResumeTokenProvider.VerifyResumeToken(ctx, resumeToken)
		// If the token is missing the key ID, it's probably an old token in which
		// case we just want to generate a new peer ID.
		if xerrors.Is(err, jwtutils.ErrMissingKeyID) {
			peerID = uuid.New()
			err = nil
		} else if err != nil {
			httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
				Message: workspacesdk.CoordinateAPIInvalidResumeToken,
				Detail:  err.Error(),
				Validations: []codersdk.ValidationError{
					{Field: "resume_token", Detail: workspacesdk.CoordinateAPIInvalidResumeToken},
				},
			})
			return peerID, err
		} else {
			api.Logger.Debug(ctx, "accepted coordinate resume token for peer",
				slog.F("peer_id", peerID.String()))
		}
	}
	return peerID, err
}

// @Summary Post workspace agent log source
// @ID post-workspace-agent-log-source
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.PostLogSourceRequest true "Log source request"
// @Success 200 {object} codersdk.WorkspaceAgentLogSource
// @Router /workspaceagents/me/log-source [post]
func (api *API) workspaceAgentPostLogSource(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req agentsdk.PostLogSourceRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	workspaceAgent := httpmw.WorkspaceAgent(r)

	sources, err := api.Database.InsertWorkspaceAgentLogSources(ctx, database.InsertWorkspaceAgentLogSourcesParams{
		WorkspaceAgentID: workspaceAgent.ID,
		CreatedAt:        dbtime.Now(),
		ID:               []uuid.UUID{req.ID},
		DisplayName:      []string{req.DisplayName},
		Icon:             []string{req.Icon},
	})
	if err != nil {
		if database.IsUniqueViolation(err, "workspace_agent_log_sources_pkey") {
			httpapi.Write(ctx, rw, http.StatusCreated, codersdk.WorkspaceAgentLogSource{
				WorkspaceAgentID: workspaceAgent.ID,
				CreatedAt:        dbtime.Now(),
				ID:               req.ID,
				DisplayName:      req.DisplayName,
				Icon:             req.Icon,
			})
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	if len(sources) != 1 {
		httpapi.InternalServerError(rw, xerrors.Errorf("database should've returned 1 row, got %d", len(sources)))
		return
	}

	apiSource := convertLogSources(sources)[0]

	httpapi.Write(ctx, rw, http.StatusCreated, apiSource)
}

// convertProvisionedApps converts applications that are in the middle of provisioning process.
// It means that they may not have an agent or workspace assigned (dry-run job).
func convertProvisionedApps(dbApps []database.WorkspaceApp) []codersdk.WorkspaceApp {
	return db2sdk.Apps(dbApps, database.WorkspaceAgent{}, "", database.Workspace{})
}

func convertLogSources(dbLogSources []database.WorkspaceAgentLogSource) []codersdk.WorkspaceAgentLogSource {
	logSources := make([]codersdk.WorkspaceAgentLogSource, 0)
	for _, dbLogSource := range dbLogSources {
		logSources = append(logSources, codersdk.WorkspaceAgentLogSource{
			ID:               dbLogSource.ID,
			DisplayName:      dbLogSource.DisplayName,
			WorkspaceAgentID: dbLogSource.WorkspaceAgentID,
			CreatedAt:        dbLogSource.CreatedAt,
			Icon:             dbLogSource.Icon,
		})
	}
	return logSources
}

func convertScripts(dbScripts []database.WorkspaceAgentScript) []codersdk.WorkspaceAgentScript {
	scripts := make([]codersdk.WorkspaceAgentScript, 0)
	for _, dbScript := range dbScripts {
		scripts = append(scripts, codersdk.WorkspaceAgentScript{
			ID:               dbScript.ID,
			LogPath:          dbScript.LogPath,
			LogSourceID:      dbScript.LogSourceID,
			Script:           dbScript.Script,
			Cron:             dbScript.Cron,
			RunOnStart:       dbScript.RunOnStart,
			RunOnStop:        dbScript.RunOnStop,
			StartBlocksLogin: dbScript.StartBlocksLogin,
			Timeout:          time.Duration(dbScript.TimeoutSeconds) * time.Second,
			DisplayName:      dbScript.DisplayName,
		})
	}
	return scripts
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
	// Allow us to interrupt watch via cancel.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	workspaceAgent := httpmw.WorkspaceAgentParam(r)
	log := api.Logger.Named("workspace_metadata_watcher").With(
		slog.F("workspace_agent_id", workspaceAgent.ID),
	)

	// Send metadata on updates, we must ensure subscription before sending
	// initial metadata to guarantee that events in-between are not missed.
	update := make(chan agentapi.WorkspaceAgentMetadataChannelPayload, 1)
	cancelSub, err := api.Pubsub.Subscribe(agentapi.WatchWorkspaceAgentMetadataChannel(workspaceAgent.ID), func(_ context.Context, byt []byte) {
		if ctx.Err() != nil {
			return
		}

		var payload agentapi.WorkspaceAgentMetadataChannelPayload
		err := json.Unmarshal(byt, &payload)
		if err != nil {
			log.Error(ctx, "failed to unmarshal pubsub message", slog.Error(err))
			return
		}

		log.Debug(ctx, "received metadata update", "payload", payload)

		select {
		case prev := <-update:
			payload.Keys = appendUnique(prev.Keys, payload.Keys)
		default:
		}
		// This can never block since we pop and merge beforehand.
		update <- payload
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	defer cancelSub()

	// We always use the original Request context because it contains
	// the RBAC actor.
	initialMD, err := api.Database.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
		WorkspaceAgentID: workspaceAgent.ID,
		Keys:             nil,
	})
	if err != nil {
		// If we can't successfully pull the initial metadata, pubsub
		// updates will be no-op so we may as well terminate the
		// connection early.
		httpapi.InternalServerError(rw, err)
		return
	}

	log.Debug(ctx, "got initial metadata", "num", len(initialMD))

	metadataMap := make(map[string]database.WorkspaceAgentMetadatum, len(initialMD))
	for _, datum := range initialMD {
		metadataMap[datum.Key] = datum
	}
	//nolint:ineffassign // Release memory.
	initialMD = nil

	sseSendEvent, sseSenderClosed, err := httpapi.ServerSentEventSender(rw, r)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error setting up server-sent events.",
			Detail:  err.Error(),
		})
		return
	}
	// Prevent handler from returning until the sender is closed.
	defer func() {
		cancel()
		<-sseSenderClosed
	}()
	// Synchronize cancellation from SSE -> context, this lets us simplify the
	// cancellation logic.
	go func() {
		select {
		case <-ctx.Done():
		case <-sseSenderClosed:
			cancel()
		}
	}()

	var lastSend time.Time
	sendMetadata := func() {
		lastSend = time.Now()
		values := maps.Values(metadataMap)

		log.Debug(ctx, "sending metadata", "num", len(values))

		_ = sseSendEvent(ctx, codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: convertWorkspaceAgentMetadata(values),
		})
	}

	// We send updates exactly every second.
	const sendInterval = time.Second * 1
	sendTicker := time.NewTicker(sendInterval)
	defer sendTicker.Stop()

	// Send initial metadata.
	sendMetadata()

	// Fetch updated metadata keys as they come in.
	fetchedMetadata := make(chan []database.WorkspaceAgentMetadatum)
	go func() {
		defer close(fetchedMetadata)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case payload := <-update:
				md, err := api.Database.GetWorkspaceAgentMetadata(ctx, database.GetWorkspaceAgentMetadataParams{
					WorkspaceAgentID: workspaceAgent.ID,
					Keys:             payload.Keys,
				})
				if err != nil {
					if !database.IsQueryCanceledError(err) {
						log.Error(ctx, "failed to get metadata", slog.Error(err))
						_ = sseSendEvent(ctx, codersdk.ServerSentEvent{
							Type: codersdk.ServerSentEventTypeError,
							Data: codersdk.Response{
								Message: "Failed to get metadata.",
								Detail:  err.Error(),
							},
						})
					}
					return
				}
				select {
				case <-ctx.Done():
					return
				// We want to block here to avoid constantly pinging the
				// database when the metadata isn't being processed.
				case fetchedMetadata <- md:
					log.Debug(ctx, "fetched metadata update for keys", "keys", payload.Keys, "num", len(md))
				}
			}
		}
	}()
	defer func() {
		<-fetchedMetadata
	}()

	pendingChanges := true
	for {
		select {
		case <-ctx.Done():
			return
		case md, ok := <-fetchedMetadata:
			if !ok {
				return
			}
			for _, datum := range md {
				metadataMap[datum.Key] = datum
			}
			pendingChanges = true
			continue
		case <-sendTicker.C:
			// We send an update even if there's no change every 5 seconds
			// to ensure that the frontend always has an accurate "Result.Age".
			if !pendingChanges && time.Since(lastSend) < 5*time.Second {
				continue
			}
			pendingChanges = false
		}

		sendMetadata()
	}
}

// appendUnique is like append and adds elements from src to dst,
// skipping any elements that already exist in dst.
func appendUnique[T comparable](dst, src []T) []T {
	exists := make(map[T]struct{}, len(dst))
	for _, key := range dst {
		exists[key] = struct{}{}
	}
	for _, key := range src {
		if _, ok := exists[key]; !ok {
			dst = append(dst, key)
		}
	}
	return dst
}

func convertWorkspaceAgentMetadata(db []database.WorkspaceAgentMetadatum) []codersdk.WorkspaceAgentMetadata {
	// Sort the input database slice by DisplayOrder and then by Key before processing
	sort.Slice(db, func(i, j int) bool {
		if db[i].DisplayOrder == db[j].DisplayOrder {
			return db[i].Key < db[j].Key
		}
		return db[i].DisplayOrder < db[j].DisplayOrder
	})

	// An empty array is easier for clients to handle than a null.
	result := make([]codersdk.WorkspaceAgentMetadata, len(db))
	for i, datum := range db {
		result[i] = codersdk.WorkspaceAgentMetadata{
			Result: codersdk.WorkspaceAgentMetadataResult{
				Value:       datum.Value,
				Error:       datum.Error,
				CollectedAt: datum.CollectedAt.UTC(),
				Age:         int64(time.Since(datum.CollectedAt).Seconds()),
			},
			Description: codersdk.WorkspaceAgentMetadataDescription{
				DisplayName: datum.DisplayName,
				Key:         datum.Key,
				Script:      datum.Script,
				Interval:    datum.Interval,
				Timeout:     datum.Timeout,
			},
		}
	}
	return result
}

// workspaceAgentsExternalAuth returns an access token for a given URL
// or finds a provider by ID.
//
// @Summary Get workspace agent external auth
// @ID get-workspace-agent-external-auth
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Param match query string true "Match"
// @Param id query string true "Provider ID"
// @Param listen query bool false "Wait for a new token to be issued"
// @Success 200 {object} agentsdk.ExternalAuthResponse
// @Router /workspaceagents/me/external-auth [get]
func (api *API) workspaceAgentsExternalAuth(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()
	// Either match or configID must be provided!
	match := query.Get("match")
	if match == "" {
		// Support legacy agents!
		match = query.Get("url")
	}
	id := query.Get("id")
	if match == "" && id == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "'url' or 'id' must be provided!",
		})
		return
	}
	if match != "" && id != "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "'url' and 'id' cannot be provided together!",
		})
		return
	}

	// listen determines if the request will wait for a
	// new token to be issued!
	listen := r.URL.Query().Has("listen")

	var externalAuthConfig *externalauth.Config
	for _, extAuth := range api.ExternalAuthConfigs {
		if extAuth.ID == id {
			externalAuthConfig = extAuth
			break
		}
		if match == "" || extAuth.Regex == nil {
			continue
		}
		matches := extAuth.Regex.MatchString(match)
		if !matches {
			continue
		}
		externalAuthConfig = extAuth
	}
	if externalAuthConfig == nil {
		detail := "External auth provider not found."
		if len(api.ExternalAuthConfigs) > 0 {
			regexURLs := make([]string, 0, len(api.ExternalAuthConfigs))
			for _, extAuth := range api.ExternalAuthConfigs {
				if extAuth.Regex == nil {
					continue
				}
				regexURLs = append(regexURLs, fmt.Sprintf("%s=%q", extAuth.ID, extAuth.Regex.String()))
			}
			detail = fmt.Sprintf("The configured external auth provider have regex filters that do not match the url. Provider url regex: %s", strings.Join(regexURLs, ","))
		}
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("No matching external auth provider found in Coder for the url %q.", match),
			Detail:  detail,
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

	var previousToken *database.ExternalAuthLink
	// handleRetrying will attempt to continually check for a new token
	// if listen is true. This is useful if an error is encountered in the
	// original single flow.
	//
	// By default, if no errors are encountered, then the single flow response
	// is returned.
	handleRetrying := func(code int, response any) {
		if !listen {
			httpapi.Write(ctx, rw, code, response)
			return
		}

		api.workspaceAgentsExternalAuthListen(ctx, rw, previousToken, externalAuthConfig, workspace)
	}

	// This is the URL that will redirect the user with a state token.
	redirectURL, err := api.AccessURL.Parse(fmt.Sprintf("/external-auth/%s", externalAuthConfig.ID))
	if err != nil {
		handleRetrying(http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to parse access URL.",
			Detail:  err.Error(),
		})
		return
	}

	externalAuthLink, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
		ProviderID: externalAuthConfig.ID,
		UserID:     workspace.OwnerID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			handleRetrying(http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get external auth link.",
				Detail:  err.Error(),
			})
			return
		}

		handleRetrying(http.StatusOK, agentsdk.ExternalAuthResponse{
			URL: redirectURL.String(),
		})
		return
	}

	refreshedLink, err := externalAuthConfig.RefreshToken(ctx, api.Database, externalAuthLink)
	if err != nil && !externalauth.IsInvalidTokenError(err) {
		handleRetrying(http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to refresh external auth token.",
			Detail:  err.Error(),
		})
		return
	}
	if err != nil {
		// Set the previous token so the retry logic will skip validating the
		// same token again. This should only be set if the token is invalid and there
		// was no error. If it is invalid because of an error, then we should recheck.
		previousToken = &refreshedLink
		handleRetrying(http.StatusOK, agentsdk.ExternalAuthResponse{
			URL: redirectURL.String(),
		})
		return
	}
	resp, err := createExternalAuthResponse(externalAuthConfig.Type, refreshedLink.OAuthAccessToken, refreshedLink.OAuthExtra)
	if err != nil {
		handleRetrying(http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create external auth response.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func (api *API) workspaceAgentsExternalAuthListen(ctx context.Context, rw http.ResponseWriter, previous *database.ExternalAuthLink, externalAuthConfig *externalauth.Config, workspace database.Workspace) {
	// Since we're ticking frequently and this sign-in operation is rare,
	// we are OK with polling to avoid the complexity of pubsub.
	ticker, done := api.NewTicker(time.Second)
	defer done()
	// If we have a previous token that is invalid, we should not check this again.
	// This serves to prevent doing excessive unauthorized requests to the external
	// auth provider. For github, this limit is 60 per hour, so saving a call
	// per invalid token can be significant.
	var previousToken database.ExternalAuthLink
	if previous != nil {
		previousToken = *previous
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker:
		}
		externalAuthLink, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
			ProviderID: externalAuthConfig.ID,
			UserID:     workspace.OwnerID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get external auth link.",
				Detail:  err.Error(),
			})
			return
		}

		// Expiry may be unset if the application doesn't configure tokens
		// to expire.
		// See
		// https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-user-access-token-for-a-github-app.
		if externalAuthLink.OAuthExpiry.Before(dbtime.Now()) && !externalAuthLink.OAuthExpiry.IsZero() {
			continue
		}

		// Only attempt to revalidate an oauth token if it has actually changed.
		// No point in trying to validate the same token over and over again.
		if previousToken.OAuthAccessToken == externalAuthLink.OAuthAccessToken &&
			previousToken.OAuthRefreshToken == externalAuthLink.OAuthRefreshToken &&
			previousToken.OAuthExpiry == externalAuthLink.OAuthExpiry {
			continue
		}

		valid, _, err := externalAuthConfig.ValidateToken(ctx, externalAuthLink.OAuthToken())
		if err != nil {
			api.Logger.Warn(ctx, "failed to validate external auth token",
				slog.F("workspace_owner_id", workspace.OwnerID.String()),
				slog.F("validate_url", externalAuthConfig.ValidateURL),
				slog.Error(err),
			)
		}
		previousToken = externalAuthLink
		if !valid {
			continue
		}
		resp, err := createExternalAuthResponse(externalAuthConfig.Type, externalAuthLink.OAuthAccessToken, externalAuthLink.OAuthExtra)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create external auth response.",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(ctx, rw, http.StatusOK, resp)
		return
	}
}

// @Summary User-scoped tailnet RPC connection
// @ID user-scoped-tailnet-rpc-connection
// @Security CoderSessionToken
// @Tags Agents
// @Success 101
// @Router /tailnet [get]
func (api *API) tailnetRPCConn(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	version := "2.0"
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

	peerID, err := api.handleResumeToken(ctx, rw, r)
	if err != nil {
		// handleResumeToken has already written the response.
		return
	}

	// Used to authorize tunnel request
	sshPrep, err := api.HTTPAuth.AuthorizeSQLFilter(r, policy.ActionSSH, rbac.ResourceWorkspace.Type)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error preparing sql filter.",
			Detail:  err.Error(),
		})
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
	ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()
	defer conn.Close(websocket.StatusNormalClosure, "")

	go httpapi.Heartbeat(ctx, conn)
	err = api.TailnetClientService.ServeClient(ctx, version, wsNetConn, tailnet.StreamID{
		Name: "client",
		ID:   peerID,
		Auth: tailnet.ClientUserCoordinateeAuth{
			Auth: &rbacAuthorizer{
				sshPrep: sshPrep,
				db:      api.Database,
			},
		},
	})
	if err != nil && !xerrors.Is(err, io.EOF) && !xerrors.Is(err, context.Canceled) {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
}

// createExternalAuthResponse creates an ExternalAuthResponse based on the
// provider type. This is to support legacy `/workspaceagents/me/gitauth`
// which uses `Username` and `Password`.
func createExternalAuthResponse(typ, token string, extra pqtype.NullRawMessage) (agentsdk.ExternalAuthResponse, error) {
	var resp agentsdk.ExternalAuthResponse
	switch typ {
	case string(codersdk.EnhancedExternalAuthProviderGitLab):
		// https://stackoverflow.com/questions/25409700/using-gitlab-token-to-clone-without-authentication
		resp = agentsdk.ExternalAuthResponse{
			Username: "oauth2",
			Password: token,
		}
	case string(codersdk.EnhancedExternalAuthProviderBitBucketCloud), string(codersdk.EnhancedExternalAuthProviderBitBucketServer):
		// The string "bitbucket" was a legacy parameter that needs to still be supported.
		// https://support.atlassian.com/bitbucket-cloud/docs/use-oauth-on-bitbucket-cloud/#Cloning-a-repository-with-an-access-token
		resp = agentsdk.ExternalAuthResponse{
			Username: "x-token-auth",
			Password: token,
		}
	default:
		resp = agentsdk.ExternalAuthResponse{
			Username: token,
		}
	}
	resp.AccessToken = token
	resp.Type = typ

	var err error
	if extra.Valid {
		err = json.Unmarshal(extra.RawMessage, &resp.TokenExtra)
	}
	return resp, err
}

func convertWorkspaceAgentLogs(logs []database.WorkspaceAgentLog) []codersdk.WorkspaceAgentLog {
	sdk := make([]codersdk.WorkspaceAgentLog, 0, len(logs))
	for _, logEntry := range logs {
		sdk = append(sdk, convertWorkspaceAgentLog(logEntry))
	}
	return sdk
}

func convertWorkspaceAgentLog(logEntry database.WorkspaceAgentLog) codersdk.WorkspaceAgentLog {
	return codersdk.WorkspaceAgentLog{
		ID:        logEntry.ID,
		CreatedAt: logEntry.CreatedAt,
		Output:    logEntry.Output,
		Level:     codersdk.LogLevel(logEntry.Level),
		SourceID:  logEntry.LogSourceID,
	}
}
