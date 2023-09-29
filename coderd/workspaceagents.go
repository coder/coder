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
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/mod/semver"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/gitauth"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
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
		scripts, err = api.Database.GetWorkspaceAgentScriptsByAgentIDs(ctx, []uuid.UUID{workspaceAgent.ID})
		return err
	})
	eg.Go(func() (err error) {
		logSources, err = api.Database.GetWorkspaceAgentLogSourcesByAgentIDs(ctx, []uuid.UUID{workspaceAgent.ID})
		return err
	})
	err := eg.Wait()
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

	apiAgent, err := convertWorkspaceAgent(
		api.DERPMap(), *api.TailnetCoordinator.Load(), workspaceAgent, convertApps(dbApps, workspaceAgent, owner, workspace), convertScripts(scripts), convertLogSources(logSources), api.AgentInactiveDisconnectTimeout,
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

	var (
		dbApps    []database.WorkspaceApp
		scripts   []database.WorkspaceAgentScript
		metadata  []database.WorkspaceAgentMetadatum
		resource  database.WorkspaceResource
		build     database.WorkspaceBuild
		workspace database.Workspace
		owner     database.User
	)

	var eg errgroup.Group
	eg.Go(func() (err error) {
		dbApps, err = api.Database.GetWorkspaceAppsByAgentID(ctx, workspaceAgent.ID)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	eg.Go(func() (err error) {
		// nolint:gocritic // This is necessary to fetch agent scripts!
		scripts, err = api.Database.GetWorkspaceAgentScriptsByAgentIDs(dbauthz.AsSystemRestricted(ctx), []uuid.UUID{workspaceAgent.ID})
		return err
	})
	eg.Go(func() (err error) {
		metadata, err = api.Database.GetWorkspaceAgentMetadata(ctx, workspaceAgent.ID)
		return err
	})
	eg.Go(func() (err error) {
		resource, err = api.Database.GetWorkspaceResourceByID(ctx, workspaceAgent.ResourceID)
		if err != nil {
			return xerrors.Errorf("getting resource by id: %w", err)
		}
		build, err = api.Database.GetWorkspaceBuildByJobID(ctx, resource.JobID)
		if err != nil {
			return xerrors.Errorf("getting workspace build by job id: %w", err)
		}
		workspace, err = api.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("getting workspace by id: %w", err)
		}
		owner, err = api.Database.GetUserByID(ctx, workspace.OwnerID)
		if err != nil {
			return xerrors.Errorf("getting workspace owner by id: %w", err)
		}
		return err
	})
	err = eg.Wait()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent manifest.",
			Detail:  err.Error(),
		})
		return
	}

	appHost := httpapi.ApplicationURL{
		AppSlugOrPort: "{{port}}",
		AgentName:     workspaceAgent.Name,
		WorkspaceName: workspace.Name,
		Username:      owner.Username,
	}
	vscodeProxyURI := api.AccessURL.Scheme + "://" + strings.ReplaceAll(api.AppHostname, "*", appHost.String())
	if api.AppHostname == "" {
		vscodeProxyURI += api.AccessURL.Hostname()
	}
	if api.AccessURL.Port() != "" {
		vscodeProxyURI += fmt.Sprintf(":%s", api.AccessURL.Port())
	}

	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.Manifest{
		AgentID:                  apiAgent.ID,
		Apps:                     convertApps(dbApps, workspaceAgent, owner, workspace),
		Scripts:                  convertScripts(scripts),
		DERPMap:                  api.DERPMap(),
		DERPForceWebSockets:      api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		GitAuthConfigs:           len(api.ExternalAuthConfigs),
		EnvironmentVariables:     apiAgent.EnvironmentVariables,
		Directory:                apiAgent.Directory,
		VSCodePortProxyURI:       vscodeProxyURI,
		MOTDFile:                 workspaceAgent.MOTDFile,
		DisableDirectConnections: api.DeploymentValues.DERP.Config.BlockDirect.Value(),
		Metadata:                 convertWorkspaceAgentMetadataDesc(metadata),
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

	apiAgent, err := convertWorkspaceAgent(
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
	clientConn, serverConn := net.Pipe()

	derpMap := api.DERPMap()
	conn, err := tailnet.NewConn(&tailnet.Options{
		Addresses:           []netip.Prefix{netip.PrefixFrom(tailnet.IP(), 128)},
		DERPMap:             api.DERPMap(),
		DERPForceWebSockets: api.DeploymentValues.DERP.Config.ForceWebSockets.Value(),
		Logger:              api.Logger.Named("net.tailnet"),
		BlockEndpoints:      api.DeploymentValues.DERP.Config.BlockDirect.Value(),
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

	sendNodes, _ := tailnet.ServeCoordinator(clientConn, func(nodes []*tailnet.Node) error {
		return conn.UpdateNodes(nodes, true)
	})
	conn.SetNodeCallback(sendNodes)

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
			cancel()
			_ = clientConn.Close()
			_ = serverConn.Close()
			return nil
		},
	})
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
		_ = serverConn.Close()
		_ = clientConn.Close()
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
			Time:  dbtime.Now(),
			Valid: true,
		}
	}
	lastConnectedAt := sql.NullTime{
		Time:  dbtime.Now(),
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
			UpdatedAt:        dbtime.Now(),
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
				Time:  dbtime.Now(),
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
					slog.F("workspace_id", build.WorkspaceID),
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

	api.Logger.Debug(ctx, "accepting agent",
		slog.F("owner", owner.Username),
		slog.F("workspace", workspace.Name),
		slog.F("name", workspaceAgent.Name),
	)
	api.Logger.Debug(ctx, "accepting agent details", slog.F("agent", workspaceAgent))

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
					Time:  dbtime.Now(),
					Valid: true,
				}
			}
		} else {
			connectionStatusChanged = disconnectedAt.Valid
			// TODO(mafredri): Should we update it here or allow lastConnectedAt to shadow it?
			disconnectedAt = sql.NullTime{}
			lastConnectedAt = sql.NullTime{
				Time:  dbtime.Now(),
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

// convertProvisionedApps converts applications that are in the middle of provisioning process.
// It means that they may not have an agent or workspace assigned (dry-run job).
func convertProvisionedApps(dbApps []database.WorkspaceApp) []codersdk.WorkspaceApp {
	return convertApps(dbApps, database.WorkspaceAgent{}, database.User{}, database.Workspace{})
}

func convertApps(dbApps []database.WorkspaceApp, agent database.WorkspaceAgent, owner database.User, workspace database.Workspace) []codersdk.WorkspaceApp {
	apps := make([]codersdk.WorkspaceApp, 0)
	for _, dbApp := range dbApps {
		var subdomainName string
		if dbApp.Subdomain && agent.Name != "" && owner.Username != "" && workspace.Name != "" {
			appSlug := dbApp.Slug
			if appSlug == "" {
				appSlug = dbApp.DisplayName
			}
			subdomainName = httpapi.ApplicationURL{
				AppSlugOrPort: appSlug,
				AgentName:     agent.Name,
				WorkspaceName: workspace.Name,
				Username:      owner.Username,
			}.String()
		}

		apps = append(apps, codersdk.WorkspaceApp{
			ID:            dbApp.ID,
			URL:           dbApp.Url.String,
			External:      dbApp.External,
			Slug:          dbApp.Slug,
			DisplayName:   dbApp.DisplayName,
			Command:       dbApp.Command.String,
			Icon:          dbApp.Icon,
			Subdomain:     dbApp.Subdomain,
			SubdomainName: subdomainName,
			SharingLevel:  codersdk.WorkspaceAppSharingLevel(dbApp.SharingLevel),
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

func convertWorkspaceAgent(derpMap *tailcfg.DERPMap, coordinator tailnet.Coordinator,
	dbAgent database.WorkspaceAgent, apps []codersdk.WorkspaceApp, scripts []codersdk.WorkspaceAgentScript, logSources []codersdk.WorkspaceAgentLogSource,
	agentInactiveDisconnectTimeout time.Duration, agentFallbackTroubleshootingURL string,
) (codersdk.WorkspaceAgent, error) {
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
	subsystems := make([]codersdk.AgentSubsystem, len(dbAgent.Subsystems))
	for i, subsystem := range dbAgent.Subsystems {
		subsystems[i] = codersdk.AgentSubsystem(subsystem)
	}

	legacyStartupScriptBehavior := codersdk.WorkspaceAgentStartupScriptBehaviorNonBlocking
	for _, script := range scripts {
		if !script.RunOnStart {
			continue
		}
		if !script.StartBlocksLogin {
			continue
		}
		legacyStartupScriptBehavior = codersdk.WorkspaceAgentStartupScriptBehaviorBlocking
	}

	workspaceAgent := codersdk.WorkspaceAgent{
		ID:                       dbAgent.ID,
		CreatedAt:                dbAgent.CreatedAt,
		UpdatedAt:                dbAgent.UpdatedAt,
		ResourceID:               dbAgent.ResourceID,
		InstanceID:               dbAgent.AuthInstanceID.String,
		Name:                     dbAgent.Name,
		Architecture:             dbAgent.Architecture,
		OperatingSystem:          dbAgent.OperatingSystem,
		Scripts:                  scripts,
		StartupScriptBehavior:    legacyStartupScriptBehavior,
		LogsLength:               dbAgent.LogsLength,
		LogsOverflowed:           dbAgent.LogsOverflowed,
		LogSources:               logSources,
		Version:                  dbAgent.Version,
		EnvironmentVariables:     envs,
		Directory:                dbAgent.Directory,
		ExpandedDirectory:        dbAgent.ExpandedDirectory,
		Apps:                     apps,
		ConnectionTimeoutSeconds: dbAgent.ConnectionTimeoutSeconds,
		TroubleshootingURL:       troubleshootingURL,
		LifecycleState:           codersdk.WorkspaceAgentLifecycle(dbAgent.LifecycleState),
		Subsystems:               subsystems,
		DisplayApps:              convertDisplayApps(dbAgent.DisplayApps),
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

	if dbAgent.StartedAt.Valid {
		workspaceAgent.StartedAt = &dbAgent.StartedAt.Time
	}
	if dbAgent.ReadyAt.Valid {
		workspaceAgent.ReadyAt = &dbAgent.ReadyAt.Time
	}

	switch {
	case workspaceAgent.Status != codersdk.WorkspaceAgentConnected && workspaceAgent.LifecycleState == codersdk.WorkspaceAgentLifecycleOff:
		workspaceAgent.Health.Reason = "agent is not running"
	case workspaceAgent.Status == codersdk.WorkspaceAgentTimeout:
		workspaceAgent.Health.Reason = "agent is taking too long to connect"
	case workspaceAgent.Status == codersdk.WorkspaceAgentDisconnected:
		workspaceAgent.Health.Reason = "agent has lost connection"
	// Note: We could also handle codersdk.WorkspaceAgentLifecycleStartTimeout
	// here, but it's more of a soft issue, so we don't want to mark the agent
	// as unhealthy.
	case workspaceAgent.LifecycleState == codersdk.WorkspaceAgentLifecycleStartError:
		workspaceAgent.Health.Reason = "agent startup script exited with an error"
	case workspaceAgent.LifecycleState.ShuttingDown():
		workspaceAgent.Health.Reason = "agent is shutting down"
	default:
		workspaceAgent.Health.Healthy = true
	}

	return workspaceAgent, nil
}

func convertDisplayApps(apps []database.DisplayApp) []codersdk.DisplayApp {
	dapps := make([]codersdk.DisplayApp, 0, len(apps))
	for _, app := range apps {
		switch codersdk.DisplayApp(app) {
		case codersdk.DisplayAppVSCodeDesktop, codersdk.DisplayAppVSCodeInsiders, codersdk.DisplayAppPortForward, codersdk.DisplayAppWebTerminal, codersdk.DisplayAppSSH:
			dapps = append(dapps, codersdk.DisplayApp(app))
		}
	}

	return dapps
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
		slog.F("workspace_agent_id", workspaceAgent.ID),
		slog.F("workspace_id", workspace.ID),
		slog.F("payload", req),
	)

	if req.ConnectionCount > 0 {
		activityBumpWorkspace(ctx, api.Logger.Named("activity_bump"), api.Database, workspace.ID)
	}

	now := dbtime.Now()

	var errGroup errgroup.Group
	errGroup.Go(func() error {
		if err := api.statsBatcher.Add(time.Now(), workspaceAgent.ID, workspace.TemplateID, workspace.OwnerID, workspace.ID, req); err != nil {
			api.Logger.Error(ctx, "failed to add stats to batcher", slog.Error(err))
			return xerrors.Errorf("can't insert workspace agent stat: %w", err)
		}
		return nil
	})
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
	if api.Options.UpdateAgentMetrics != nil {
		errGroup.Go(func() error {
			user, err := api.Database.GetUserByID(ctx, workspace.OwnerID)
			if err != nil {
				return xerrors.Errorf("can't get user: %w", err)
			}

			api.Options.UpdateAgentMetrics(ctx, user.Username, workspace.Name, workspaceAgent.Name, req.Metrics)
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
		// maxValueLen is set to 2048 to stay under the 8000 byte Postgres
		// NOTIFY limit. Since both value and error can be set, the real
		// payload limit is 2 * 2048 * 4/3 <base64 expansion> = 5461 bytes + a few hundred bytes for JSON
		// syntax, key names, and metadata.
		maxValueLen = 2048
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
		slog.F("workspace_agent_id", workspaceAgent.ID),
		slog.F("workspace_id", workspace.ID),
		slog.F("collected_at", datum.CollectedAt),
		slog.F("key", datum.Key),
		slog.F("value", ellipse(datum.Value, 16)),
	)

	datumJSON, err := json.Marshal(datum)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	err = api.Pubsub.Publish(watchWorkspaceAgentMetadataChannel(workspaceAgent.ID), datumJSON)
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
		log            = api.Logger.Named("workspace_metadata_watcher").With(
			slog.F("workspace_agent_id", workspaceAgent.ID),
		)
	)

	// We avoid channel-based synchronization here to avoid backpressure problems.
	var (
		metadataMapMu sync.Mutex
		metadataMap   = make(map[string]database.WorkspaceAgentMetadatum)
		// pendingChanges must only be mutated when metadataMapMu is held.
		pendingChanges atomic.Bool
	)

	// Send metadata on updates, we must ensure subscription before sending
	// initial metadata to guarantee that events in-between are not missed.
	cancelSub, err := api.Pubsub.Subscribe(watchWorkspaceAgentMetadataChannel(workspaceAgent.ID), func(_ context.Context, byt []byte) {
		var update database.UpdateWorkspaceAgentMetadataParams
		err := json.Unmarshal(byt, &update)
		if err != nil {
			api.Logger.Error(ctx, "failed to unmarshal pubsub message", slog.Error(err))
			return
		}

		log.Debug(ctx, "received metadata update", "key", update.Key)

		metadataMapMu.Lock()
		defer metadataMapMu.Unlock()
		md := metadataMap[update.Key]
		md.Value = update.Value
		md.Error = update.Error
		md.CollectedAt = update.CollectedAt
		metadataMap[update.Key] = md
		pendingChanges.Store(true)
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	defer cancelSub()

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
		<-sseSenderClosed
	}()

	// We send updates exactly every second.
	const sendInterval = time.Second * 1
	sendTicker := time.NewTicker(sendInterval)
	defer sendTicker.Stop()

	// We always use the original Request context because it contains
	// the RBAC actor.
	md, err := api.Database.GetWorkspaceAgentMetadata(ctx, workspaceAgent.ID)
	if err != nil {
		// If we can't successfully pull the initial metadata, pubsub
		// updates will be no-op so we may as well terminate the
		// connection early.
		httpapi.InternalServerError(rw, err)
		return
	}

	metadataMapMu.Lock()
	for _, datum := range md {
		metadataMap[datum.Key] = datum
	}
	metadataMapMu.Unlock()

	// Send initial metadata.

	var lastSend time.Time
	sendMetadata := func() {
		metadataMapMu.Lock()
		values := maps.Values(metadataMap)
		pendingChanges.Store(false)
		metadataMapMu.Unlock()

		lastSend = time.Now()
		_ = sseSendEvent(ctx, codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: convertWorkspaceAgentMetadata(values),
		})
	}

	sendMetadata()

	for {
		select {
		case <-sendTicker.C:
			// We send an update even if there's no change every 5 seconds
			// to ensure that the frontend always has an accurate "Result.Age".
			if !pendingChanges.Load() && time.Since(lastSend) < time.Second*5 {
				continue
			}
			sendMetadata()
		case <-sseSenderClosed:
			return
		}
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
	for _, gitAuth := range api.ExternalAuthConfigs {
		matches := gitAuth.Regex.MatchString(gitURL)
		if !matches {
			continue
		}
		gitAuthConfig = gitAuth
	}
	if gitAuthConfig == nil {
		detail := "No git providers are configured."
		if len(api.ExternalAuthConfigs) > 0 {
			regexURLs := make([]string, 0, len(api.ExternalAuthConfigs))
			for _, gitAuth := range api.ExternalAuthConfigs {
				regexURLs = append(regexURLs, fmt.Sprintf("%s=%q", gitAuth.ID, gitAuth.Regex.String()))
			}
			detail = fmt.Sprintf("The configured git provider have regex filters that do not match the git url. Provider url regexs: %s", strings.Join(regexURLs, ","))
		}
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("No matching git provider found in Coder for the url %q.", gitURL),
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
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			gitAuthLink, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
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

			// Expiry may be unset if the application doesn't configure tokens
			// to expire.
			// See
			// https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-user-access-token-for-a-github-app.
			if gitAuthLink.OAuthExpiry.Before(dbtime.Now()) && !gitAuthLink.OAuthExpiry.IsZero() {
				continue
			}
			valid, _, err := gitAuthConfig.ValidateToken(ctx, gitAuthLink.OAuthAccessToken)
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

	gitAuthLink, err := api.Database.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
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
func formatGitAuthAccessToken(typ codersdk.ExternalAuthProvider, token string) agentsdk.GitAuthResponse {
	var resp agentsdk.GitAuthResponse
	switch typ {
	case codersdk.ExternalAuthProviderGitLab:
		// https://stackoverflow.com/questions/25409700/using-gitlab-token-to-clone-without-authentication
		resp = agentsdk.GitAuthResponse{
			Username: "oauth2",
			Password: token,
		}
	case codersdk.ExternalAuthProviderBitBucket:
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
