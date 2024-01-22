package coderd

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/mod/semver"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
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

	// As this API becomes deprecated, use the new protobuf API and convert the
	// types back to the SDK types.
	manifestAPI := &agentapi.ManifestAPI{
		AccessURL:                       api.AccessURL,
		AppHostname:                     api.AppHostname,
		AgentInactiveDisconnectTimeout:  api.AgentInactiveDisconnectTimeout,
		AgentFallbackTroubleshootingURL: api.DeploymentValues.AgentFallbackTroubleshootingURL.String(),
		ExternalAuthConfigs:             api.ExternalAuthConfigs,
		DisableDirectConnections:        api.DeploymentValues.DERP.Config.BlockDirect.Value(),
		DerpForceWebSockets:             api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),

		AgentFn:            func(_ context.Context) (database.WorkspaceAgent, error) { return workspaceAgent, nil },
		Database:           api.Database,
		DerpMapFn:          api.DERPMap,
		TailnetCoordinator: &api.TailnetCoordinator,
	}
	manifest, err := manifestAPI.GetManifest(ctx, &agentproto.GetManifestRequest{})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent manifest.",
			Detail:  err.Error(),
		})
		return
	}

	apps, err := agentproto.SDKAppsFromProto(manifest.Apps)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace agent apps.",
			Detail:  err.Error(),
		})
		return
	}

	scripts, err := agentproto.SDKAgentScriptsFromProto(manifest.Scripts)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace agent scripts.",
			Detail:  err.Error(),
		})
		return
	}

	agentID, err := uuid.FromBytes(manifest.AgentId)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace agent ID.",
			Detail:  err.Error(),
		})
		return
	}
	workspaceID, err := uuid.FromBytes(manifest.WorkspaceId)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error converting workspace ID.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.Manifest{
		AgentID:                  agentID,
		AgentName:                manifest.AgentName,
		OwnerName:                manifest.OwnerUsername,
		WorkspaceID:              workspaceID,
		WorkspaceName:            manifest.WorkspaceName,
		Apps:                     apps,
		Scripts:                  scripts,
		DERPMap:                  tailnet.DERPMapFromProto(manifest.DerpMap),
		DERPForceWebSockets:      manifest.DerpForceWebsockets,
		GitAuthConfigs:           int(manifest.GitAuthConfigs),
		EnvironmentVariables:     manifest.EnvironmentVariables,
		Directory:                manifest.Directory,
		VSCodePortProxyURI:       manifest.VsCodePortProxyUri,
		MOTDFile:                 manifest.MotdPath,
		DisableDirectConnections: manifest.DisableDirectConnections,
		Metadata:                 agentproto.SDKAgentMetadataDescriptionsFromProto(manifest.Metadata),
	})
}

const AgentAPIVersionREST = "1.0"

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

	var req agentsdk.PostStartupRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	api.Logger.Debug(
		ctx,
		"post workspace agent version",
		slog.F("agent_id", apiAgent.ID),
		slog.F("agent_version", req.Version),
		slog.F("remote_addr", r.RemoteAddr),
	)

	if !semver.IsValid(req.Version) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid workspace agent version provided.",
			Detail:  fmt.Sprintf("invalid semver version: %q", req.Version),
		})
		return
	}

	// Validate subsystems.
	seen := make(map[codersdk.AgentSubsystem]bool)
	for _, s := range req.Subsystems {
		if !s.Valid() {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid workspace agent subsystem provided.",
				Detail:  fmt.Sprintf("invalid subsystem: %q", s),
			})
			return
		}
		if seen[s] {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid workspace agent subsystem provided.",
				Detail:  fmt.Sprintf("duplicate subsystem: %q", s),
			})
			return
		}
		seen[s] = true
	}

	if err := api.Database.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
		ID:                apiAgent.ID,
		Version:           req.Version,
		ExpandedDirectory: req.ExpandedDirectory,
		Subsystems:        convertWorkspaceAgentSubsystems(req.Subsystems),
		APIVersion:        AgentAPIVersionREST,
	}); err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Error setting agent version",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, nil)
}

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

	row, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace by agent id.",
			Detail:  err.Error(),
		})
		return
	}
	workspace := row.Workspace

	api.WebsocketWaitMutex.Lock()
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()

	opts := &websocket.AcceptOptions{}

	// Allow client to request no compression. This is useful for buggy
	// clients or if there's a client/server incompatibility. This is
	// needed with e.g. nhooyr/websocket and Safari (confirmed in 16.5).
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

	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageText)
	defer wsNetConn.Close() // Also closes conn.

	// The Go stdlib JSON encoder appends a newline character after message write.
	encoder := json.NewEncoder(wsNetConn)
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
	closeSubscribeWorkspace, err := api.Pubsub.Subscribe(codersdk.WorkspaceNotifyChannel(workspace.ID), func(_ context.Context, _ []byte) {
		select {
		case workspaceNotifyCh <- struct{}{}:
		default:
		}
	})
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

// Deprecated: use api.tailnet.AgentConn instead.
// See: https://github.com/coder/coder/issues/8218
func (api *API) _dialWorkspaceAgentTailnet(agentID uuid.UUID) (*codersdk.WorkspaceAgentConn, error) {
	derpMap := api.DERPMap()
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:           []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:             api.DERPMap(),
		DERPForceWebSockets: api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		Logger:              api.Logger.Named("net.tailnet"),
		BlockEndpoints:      api.DeploymentValues.DERP.Config.BlockDirect.Value(),
	})
	if err != nil {
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

	clientID := uuid.New()
	coordination := tailnet.NewInMemoryCoordination(ctx, api.Logger,
		clientID, agentID,
		*(api.TailnetCoordinator.Load()), conn)

	// Check for updated DERP map every 5 seconds.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			lastDERPMap := derpMap
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}

				derpMap := api.DERPMap()
				if lastDERPMap == nil || tailnet.CompareDERPMaps(lastDERPMap, derpMap) {
					conn.SetDERPMap(derpMap)
					lastDERPMap = derpMap
				}
				ticker.Reset(5 * time.Second)
			}
		}
	}()

	agentConn := codersdk.NewWorkspaceAgentConn(conn, codersdk.WorkspaceAgentConnOptions{
		AgentID: agentID,
		AgentIP: codersdk.WorkspaceAgentIP,
		CloseFunc: func() error {
			_ = coordination.Close()
			cancel()
			return nil
		},
	})
	if !agentConn.AwaitReachable(ctx) {
		_ = agentConn.Close()
		cancel()
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
// @Success 200 {object} codersdk.WorkspaceAgentConnectionInfo
// @Router /workspaceagents/connection [get]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentConnectionGeneric(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.WorkspaceAgentConnectionInfo{
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
	ctx, nconn := websocketNetConn(ctx, ws, websocket.MessageBinary)
	defer nconn.Close()

	// Slurp all packets from the connection into io.Discard so pongs get sent
	// by the websocket package. We don't do any reads ourselves so this is
	// necessary.
	go func() {
		_, _ = io.Copy(io.Discard, nconn)
		_ = nconn.Close()
	}()

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
				_ = nconn.Close()
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
			err := json.NewEncoder(nconn).Encode(derpMap)
			if err != nil {
				_ = nconn.Close()
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
	// Ensure the resource is still valid!
	// We only accept agents for resources on the latest build.
	build, ok := ensureLatestBuild(ctx, api.Database, api.Logger, rw, workspaceAgent)
	if !ok {
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

	closeCtx, closeCtxCancel := context.WithCancel(ctx)
	defer closeCtxCancel()
	monitor := api.startAgentWebsocketMonitor(closeCtx, workspaceAgent, build, conn)
	defer monitor.close()

	api.Logger.Debug(ctx, "accepting agent",
		slog.F("owner", owner.Username),
		slog.F("workspace", workspace.Name),
		slog.F("name", workspaceAgent.Name),
	)
	api.Logger.Debug(ctx, "accepting agent details", slog.F("agent", workspaceAgent))

	defer conn.Close(websocket.StatusNormalClosure, "")

	err = (*api.TailnetCoordinator.Load()).ServeAgent(wsNetConn, workspaceAgent.ID,
		fmt.Sprintf("%s-%s-%s", owner.Username, workspace.Name, workspaceAgent.Name),
	)
	if err != nil {
		api.Logger.Warn(ctx, "tailnet coordinator agent error", slog.Error(err))
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
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

	version := "1.0"
	qv := r.URL.Query().Get("version")
	if qv != "" {
		version = qv
	}
	if err := tailnet.CurrentVersion.Validate(version); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Unknown or unsupported API version",
			Validations: []codersdk.ValidationError{
				{Field: "version", Detail: err.Error()},
			},
		})
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
	ctx, wsNetConn := websocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()

	go httpapi.Heartbeat(ctx, conn)

	defer conn.Close(websocket.StatusNormalClosure, "")
	err = api.TailnetClientService.ServeClient(ctx, version, wsNetConn, uuid.New(), workspaceAgent.ID)
	if err != nil && !xerrors.Is(err, io.EOF) && !xerrors.Is(err, context.Canceled) {
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
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
			LogPath:          dbScript.LogPath,
			LogSourceID:      dbScript.LogSourceID,
			Script:           dbScript.Script,
			Cron:             dbScript.Cron,
			RunOnStart:       dbScript.RunOnStart,
			RunOnStop:        dbScript.RunOnStop,
			StartBlocksLogin: dbScript.StartBlocksLogin,
			Timeout:          time.Duration(dbScript.TimeoutSeconds) * time.Second,
		})
	}
	return scripts
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
func (api *API) workspaceAgentReportStats(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workspaceAgent := httpmw.WorkspaceAgent(r)
	row, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}
	workspace := row.Workspace

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
		slog.F("workspace_agent_id", workspaceAgent.ID),
		slog.F("workspace_id", workspace.ID),
		slog.F("payload", req),
	)

	if req.ConnectionCount > 0 {
		var nextAutostart time.Time
		if workspace.AutostartSchedule.String != "" {
			templateSchedule, err := (*(api.TemplateScheduleStore.Load())).Get(ctx, api.Database, workspace.TemplateID)
			// If the template schedule fails to load, just default to bumping without the next transition and log it.
			if err != nil {
				api.Logger.Error(ctx, "failed to load template schedule bumping activity, defaulting to bumping by 60min",
					slog.F("workspace_id", workspace.ID),
					slog.F("template_id", workspace.TemplateID),
					slog.Error(err),
				)
			} else {
				next, allowed := autobuild.NextAutostartSchedule(time.Now(), workspace.AutostartSchedule.String, templateSchedule)
				if allowed {
					nextAutostart = next
				}
			}
		}
		agentapi.ActivityBumpWorkspace(ctx, api.Logger.Named("activity_bump"), api.Database, workspace.ID, nextAutostart)
	}

	now := dbtime.Now()
	protoStats := &agentproto.Stats{
		ConnectionsByProto:          req.ConnectionsByProto,
		ConnectionCount:             req.ConnectionCount,
		ConnectionMedianLatencyMs:   req.ConnectionMedianLatencyMS,
		RxPackets:                   req.RxPackets,
		RxBytes:                     req.RxBytes,
		TxPackets:                   req.TxPackets,
		TxBytes:                     req.TxBytes,
		SessionCountVscode:          req.SessionCountVSCode,
		SessionCountJetbrains:       req.SessionCountJetBrains,
		SessionCountReconnectingPty: req.SessionCountReconnectingPTY,
		SessionCountSsh:             req.SessionCountSSH,
		Metrics:                     make([]*agentproto.Stats_Metric, len(req.Metrics)),
	}
	for i, metric := range req.Metrics {
		metricType := agentproto.Stats_Metric_TYPE_UNSPECIFIED
		switch metric.Type {
		case agentsdk.AgentMetricTypeCounter:
			metricType = agentproto.Stats_Metric_COUNTER
		case agentsdk.AgentMetricTypeGauge:
			metricType = agentproto.Stats_Metric_GAUGE
		}

		protoStats.Metrics[i] = &agentproto.Stats_Metric{
			Name:   metric.Name,
			Type:   metricType,
			Value:  metric.Value,
			Labels: make([]*agentproto.Stats_Metric_Label, len(metric.Labels)),
		}
		for j, label := range metric.Labels {
			protoStats.Metrics[i].Labels[j] = &agentproto.Stats_Metric_Label{
				Name:  label.Name,
				Value: label.Value,
			}
		}
	}

	var errGroup errgroup.Group
	errGroup.Go(func() error {
		err := api.statsBatcher.Add(time.Now(), workspaceAgent.ID, workspace.TemplateID, workspace.OwnerID, workspace.ID, protoStats)
		if err != nil {
			api.Logger.Error(ctx, "failed to add stats to batcher", slog.Error(err))
			return xerrors.Errorf("can't insert workspace agent stat: %w", err)
		}
		return nil
	})
	if req.SessionCount() > 0 {
		errGroup.Go(func() error {
			err := api.Database.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
				ID:         workspace.ID,
				LastUsedAt: now,
			})
			if err != nil {
				return xerrors.Errorf("can't update workspace LastUsedAt: %w", err)
			}
			return nil
		})
	}
	if api.Options.UpdateAgentMetrics != nil {
		errGroup.Go(func() error {
			user, err := api.Database.GetUserByID(ctx, workspace.OwnerID)
			if err != nil {
				return xerrors.Errorf("can't get user: %w", err)
			}

			api.Options.UpdateAgentMetrics(ctx, prometheusmetrics.AgentMetricLabels{
				Username:      user.Username,
				WorkspaceName: workspace.Name,
				AgentName:     workspaceAgent.Name,
				TemplateName:  row.TemplateName,
			}, protoStats.Metrics)
			return nil
		})
	}
	err = errGroup.Wait()
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.StatsResponse{
		ReportInterval: api.AgentStatsRefreshInterval,
	})
}

func ellipse(v string, n int) string {
	if len(v) > n {
		return v[:n] + "..."
	}
	return v
}

// @Summary Submit workspace agent metadata
// @ID submit-workspace-agent-metadata
// @Security CoderSessionToken
// @Accept json
// @Tags Agents
// @Param request body []agentsdk.PostMetadataRequest true "Workspace agent metadata request"
// @Success 204 "Success"
// @Router /workspaceagents/me/metadata [post]
// @x-apidocgen {"skip": true}
func (api *API) workspaceAgentPostMetadata(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req agentsdk.PostMetadataRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	workspaceAgent := httpmw.WorkspaceAgent(r)

	// Split into function to allow call by deprecated handler.
	err := api.workspaceAgentUpdateMetadata(ctx, workspaceAgent, req)
	if err != nil {
		api.Logger.Error(ctx, "failed to handle metadata request", slog.Error(err))
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusNoContent, nil)
}

func (api *API) workspaceAgentUpdateMetadata(ctx context.Context, workspaceAgent database.WorkspaceAgent, req agentsdk.PostMetadataRequest) error {
	const (
		// maxValueLen is set to 2048 to stay under the 8000 byte Postgres
		// NOTIFY limit. Since both value and error can be set, the real
		// payload limit is 2 * 2048 * 4/3 <base64 expansion> = 5461 bytes + a few hundred bytes for JSON
		// syntax, key names, and metadata.
		maxValueLen = 2048
		maxErrorLen = maxValueLen
	)

	collectedAt := time.Now()

	datum := database.UpdateWorkspaceAgentMetadataParams{
		WorkspaceAgentID: workspaceAgent.ID,
		Key:              make([]string, 0, len(req.Metadata)),
		Value:            make([]string, 0, len(req.Metadata)),
		Error:            make([]string, 0, len(req.Metadata)),
		CollectedAt:      make([]time.Time, 0, len(req.Metadata)),
	}

	for _, md := range req.Metadata {
		metadataError := md.Error

		// We overwrite the error if the provided payload is too long.
		if len(md.Value) > maxValueLen {
			metadataError = fmt.Sprintf("value of %d bytes exceeded %d bytes", len(md.Value), maxValueLen)
			md.Value = md.Value[:maxValueLen]
		}

		if len(md.Error) > maxErrorLen {
			metadataError = fmt.Sprintf("error of %d bytes exceeded %d bytes", len(md.Error), maxErrorLen)
			md.Error = md.Error[:maxErrorLen]
		}

		// We don't want a misconfigured agent to fill the database.
		datum.Key = append(datum.Key, md.Key)
		datum.Value = append(datum.Value, md.Value)
		datum.Error = append(datum.Error, metadataError)
		// We ignore the CollectedAt from the agent to avoid bugs caused by
		// clock skew.
		datum.CollectedAt = append(datum.CollectedAt, collectedAt)

		api.Logger.Debug(
			ctx, "accepted metadata report",
			slog.F("workspace_agent_id", workspaceAgent.ID),
			slog.F("collected_at", collectedAt),
			slog.F("original_collected_at", md.CollectedAt),
			slog.F("key", md.Key),
			slog.F("value", ellipse(md.Value, 16)),
		)
	}

	payload, err := json.Marshal(agentapi.WorkspaceAgentMetadataChannelPayload{
		CollectedAt: collectedAt,
		Keys:        datum.Key,
	})
	if err != nil {
		return err
	}

	err = api.Database.UpdateWorkspaceAgentMetadata(ctx, datum)
	if err != nil {
		return err
	}

	err = api.Pubsub.Publish(agentapi.WatchWorkspaceAgentMetadataChannel(workspaceAgent.ID), payload)
	if err != nil {
		return err
	}

	return nil
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
	// An empty array is easier for clients to handle than a null.
	result := make([]codersdk.WorkspaceAgentMetadata, 0, len(db))
	for _, datum := range db {
		result = append(result, codersdk.WorkspaceAgentMetadata{
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
		})
	}
	// Sorting prevents the metadata from jumping around in the frontend.
	sort.Slice(result, func(i, j int) bool {
		return result[i].Description.Key < result[j].Description.Key
	})
	return result
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
	row, err := api.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to get workspace.",
			Detail:  err.Error(),
		})
		return
	}
	workspace := row.Workspace

	var req agentsdk.PostLifecycleRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	logger := api.Logger.With(
		slog.F("workspace_agent_id", workspaceAgent.ID),
		slog.F("workspace_id", workspace.ID),
		slog.F("payload", req),
	)
	logger.Debug(ctx, "workspace agent state report")

	lifecycleState := req.State
	dbLifecycleState := database.WorkspaceAgentLifecycleState(lifecycleState)
	if !dbLifecycleState.Valid() {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid lifecycle state.",
			Detail:  fmt.Sprintf("Invalid lifecycle state %q, must be be one of %q.", lifecycleState, database.AllWorkspaceAgentLifecycleStateValues()),
		})
		return
	}

	if req.ChangedAt.IsZero() {
		// Backwards compatibility with older agents.
		req.ChangedAt = dbtime.Now()
	}
	changedAt := sql.NullTime{Time: req.ChangedAt, Valid: true}

	startedAt := workspaceAgent.StartedAt
	readyAt := workspaceAgent.ReadyAt
	switch lifecycleState {
	case codersdk.WorkspaceAgentLifecycleStarting:
		startedAt = changedAt
		readyAt.Valid = false // This agent is re-starting, so it's not ready yet.
	case codersdk.WorkspaceAgentLifecycleReady, codersdk.WorkspaceAgentLifecycleStartError:
		readyAt = changedAt
	}

	err = api.Database.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
		ID:             workspaceAgent.ID,
		LifecycleState: dbLifecycleState,
		StartedAt:      startedAt,
		ReadyAt:        readyAt,
	})
	if err != nil {
		if !xerrors.Is(err, context.Canceled) {
			// not an error if we are canceled
			logger.Error(ctx, "failed to update lifecycle state", slog.Error(err))
		}
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

	if listen {
		// Since we're ticking frequently and this sign-in operation is rare,
		// we are OK with polling to avoid the complexity of pubsub.
		ticker, done := api.NewTicker(time.Second)
		defer done()
		var previousToken database.ExternalAuthLink
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

			valid, _, err := externalAuthConfig.ValidateToken(ctx, externalAuthLink.OAuthAccessToken)
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

	// This is the URL that will redirect the user with a state token.
	redirectURL, err := api.AccessURL.Parse(fmt.Sprintf("/external-auth/%s", externalAuthConfig.ID))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
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
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get external auth link.",
				Detail:  err.Error(),
			})
			return
		}

		httpapi.Write(ctx, rw, http.StatusOK, agentsdk.ExternalAuthResponse{
			URL: redirectURL.String(),
		})
		return
	}

	externalAuthLink, updated, err := externalAuthConfig.RefreshToken(ctx, api.Database, externalAuthLink)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to refresh external auth token.",
			Detail:  err.Error(),
		})
		return
	}
	if !updated {
		httpapi.Write(ctx, rw, http.StatusOK, agentsdk.ExternalAuthResponse{
			URL: redirectURL.String(),
		})
		return
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

func convertWorkspaceAgentSubsystems(ss []codersdk.AgentSubsystem) []database.WorkspaceAgentSubsystem {
	out := make([]database.WorkspaceAgentSubsystem, 0, len(ss))
	for _, s := range ss {
		switch s {
		case codersdk.AgentSubsystemEnvbox:
			out = append(out, database.WorkspaceAgentSubsystemEnvbox)
		case codersdk.AgentSubsystemEnvbuilder:
			out = append(out, database.WorkspaceAgentSubsystemEnvbuilder)
		case codersdk.AgentSubsystemExectrace:
			out = append(out, database.WorkspaceAgentSubsystemExectrace)
		default:
			// Invalid, drop it.
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}
