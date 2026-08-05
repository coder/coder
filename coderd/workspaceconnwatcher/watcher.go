package workspaceconnwatcher

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sync"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/websocket"
)

type Watcher struct {
	logger slog.Logger
	sub    pubsub.Subscriber
	db     database.Store
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool

	connCtx         context.Context
	conn            *websocket.Conn
	enc             *wsjson.Encoder[workspacesdk.ConnectionWatchEvent]
	workspaceID     uuid.UUID
	events          chan event
	agentName       string
	lastBuild       database.GetLatestWorkspaceBuildWithStatusByWorkspaceIDRow
	jobLogSubCancel context.CancelFunc
	lastLogID       int64
}

type event struct {
	sync         bool
	wsEvent      *wspubsub.WorkspaceEvent
	jobLogNotify *provisionersdk.ProvisionerJobLogsNotifyMessage
}

func New(ctx context.Context, logger slog.Logger, sub pubsub.Subscriber, db database.Store) *Watcher {
	ctx, cancel := context.WithCancel(ctx)
	w := &Watcher{
		logger: logger.Named("wsconnwatcher"),
		ctx:    ctx,
		cancel: cancel,
		sub:    sub,
		db:     db,
	}
	go func() {
		<-ctx.Done()
		w.Close()
	}()
	return w
}

// @Summary Workspace Agent Connection Watch
// @ID workspace-agent-connection-watch
// @Security CoderSessionToken
// @Produce json
// @Tags Workspaces
// @Param workspace path string true "Workspace ID" format(uuid)
// @Success 101 {object} workspacesdk.ConnectionWatchEvent
// @Router /api/v2/workspaces/{workspace}/agent-connection-watch [get]
func (w *Watcher) WorkspaceAgentConnectionWatch(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspace := httpmw.WorkspaceParam(r)
	agentName := r.URL.Query().Get("agent_name")

	w.events = make(chan event, 1)
	w.events <- event{sync: true} // init sync
	cancelWorkspaceSubscribe, err := w.sub.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspace.OwnerID),
		wspubsub.HandleWorkspaceEvent(
			func(_ context.Context, payload wspubsub.WorkspaceEvent, err error) {
				if err != nil {
					// subscription error, resync
					select {
					case w.events <- event{sync: true}:
					case <-ctx.Done():
					}
					return
				}
				if payload.WorkspaceID != workspace.ID {
					return
				}
				select {
				case w.events <- event{wsEvent: &payload}:
				case <-ctx.Done():
				}
			}))
	if err != nil {
		w.logger.Error(ctx, "failed to subscribe to workspace events",
			slog.Error(err), slog.F("owner_id", workspace.OwnerID))
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error setting up workspace event subscription",
			// Don't include the error in case it leaks infra details about the pubsub
		})
		return
	}
	defer cancelWorkspaceSubscribe()

	closed := false
	w.mu.Lock()
	closed = w.closed
	if !closed {
		w.wg.Add(1)
	}
	w.mu.Unlock()
	if closed {
		w.logger.Debug(ctx, "server is closed, writing error")
		httpapi.Write(ctx, rw, http.StatusServiceUnavailable, codersdk.Response{
			Message: "Server instance is shutting down",
		})
		return
	}
	defer w.wg.Done()

	w.conn, err = websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept WebSocket.",
			Detail:  err.Error(),
		})
		return
	}

	// CloseRead starts a goroutine to read and discard messages from the client,
	// including Pong messages sent in response to our Ping heartbeats.
	_ = w.conn.CloseRead(ctx)

	var cancel context.CancelFunc
	w.connCtx, cancel = context.WithCancel(ctx)
	go httpapi.HeartbeatClose(ctx, w.logger, cancel, w.conn)
	defer cancel()
	w.workspaceID = workspace.ID
	w.agentName = agentName
	w.run()
}

func (w *Watcher) Close() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()

	w.cancel()
	w.wg.Wait()
}

func (w *Watcher) run() {
	w.enc = wsjson.NewEncoder[workspacesdk.ConnectionWatchEvent](w.conn, websocket.MessageText)
	defer func() {
		// this is a no-op if we have already closed for some other reason.
		_ = w.enc.Close(websocket.StatusNormalClosure)
		if w.jobLogSubCancel != nil {
			w.jobLogSubCancel()
			w.jobLogSubCancel = nil
		}
	}()

	for {
		select {
		case <-w.ctx.Done():
			w.errorThenClose(workspacesdk.WatchError{
				Code:      workspacesdk.WatchErrorServerShutdown,
				Retryable: true,
				Message:   "server is shutting down",
			})
			return
		case <-w.connCtx.Done():
			return
		case e := <-w.events:
			if e.sync {
				// zero this out so we'll send a full update
				w.lastBuild = database.GetLatestWorkspaceBuildWithStatusByWorkspaceIDRow{}
				if !w.buildUpdate() {
					return
				}
			}
			if e.wsEvent != nil {
				switch e.wsEvent.Kind {
				case wspubsub.WorkspaceEventKindStateChange:
					if !w.buildUpdate() {
						return
					}
				case wspubsub.WorkspaceEventKindAgentLifecycleUpdate:
					if !w.maybeSendAgentUpdate() {
						return
					}
				}
			}
			if e.jobLogNotify != nil {
				if e.jobLogNotify.EndOfLogs {
					// There aren't actually new logs, so we can ignore. Note that unlike a log stream, we should not
					// be tearing things down here, since the end of build logs just means that the client of the stream
					// will now be waiting additional build and agent updates.
					continue
				}
				if e.jobLogNotify.CreatedAfter >= w.lastLogID {
					// Newer logs are potentially available.
					if !w.queryJobLogs() {
						return
					}
				}
			}
		}
	}
}

func (w *Watcher) buildUpdate() bool {
	build, err := w.db.GetLatestWorkspaceBuildWithStatusByWorkspaceID(w.connCtx, w.workspaceID)
	if err != nil {
		retryable := true
		details := err.Error()
		if errors.Is(err, sql.ErrNoRows) {
			// There is no build (unlikely), or the workspace was deleted. In both cases, retrying won't help.
			retryable = false
		}
		if dbauthz.IsNotAuthorizedError(err) {
			retryable = false
			details = "unauthorized" // security: don't leak internal authz details
		}
		w.errorThenClose(workspacesdk.WatchError{
			Code:      workspacesdk.WatchErrorDatabase,
			Retryable: retryable,
			Message:   "failed to fetch latest workspace build",
			Details:   details,
		})
		return false
	}
	oldBuild := w.lastBuild
	w.lastBuild = build
	// We want to provide logs for builds we are waiting for. But, if the build job is already complete, don't bother
	// sending logs.
	if build.JobID != oldBuild.JobID && build.JobStatus != database.ProvisionerJobStatusSucceeded {
		err = w.subscribeToJobLogs()
		if err != nil {
			w.errorThenClose(workspacesdk.WatchError{
				Code:      workspacesdk.WatchErrorDatabase,
				Retryable: false,
				Message:   "failed to subscribe to build job logs",
				Details:   err.Error(),
			})
			return false
		}
	}

	if build.BuildNumber != oldBuild.BuildNumber ||
		build.JobStatus != oldBuild.JobStatus ||
		build.Transition != oldBuild.Transition {
		if build.Transition == database.WorkspaceTransitionStart &&
			build.JobStatus == database.ProvisionerJobStatusSucceeded &&
			// Only if we have previously subscribed.
			w.jobLogSubCancel != nil {
			// Before we send the update about a successful build, do one last query of the job log to ensure we haven't
			// missed any, since they logically precede the job completing.
			if !w.queryJobLogs() {
				return false
			}
		}

		err = w.enc.Encode(workspacesdk.ConnectionWatchEvent{BuildUpdate: &workspacesdk.BuildUpdate{
			Transition: codersdk.WorkspaceTransition(build.Transition),
			JobStatus:  codersdk.ProvisionerJobStatus(build.JobStatus),
		}})
		if err != nil {
			// probably this is just that the connection is closed, but in case there is some actual JSON serialization
			// error, send a close frame.
			_ = w.conn.Close(websocket.StatusInternalError, "failed to encode build update")
			return false
		}
		return w.maybeSendAgentUpdate()
	}
	return true
}

func (w *Watcher) maybeSendAgentUpdate() (ok bool) {
	if w.lastBuild.Transition != database.WorkspaceTransitionStart ||
		w.lastBuild.JobStatus != database.ProvisionerJobStatusSucceeded {
		// only send agent updates for successfully started workspaces
		return true
	}

	agents, err := w.db.GetWorkspaceAgentsByWorkspaceAndBuildNumber(w.connCtx,
		database.GetWorkspaceAgentsByWorkspaceAndBuildNumberParams{
			WorkspaceID: w.workspaceID,
			BuildNumber: w.lastBuild.BuildNumber,
		})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		details := err.Error()
		retryable := true
		if dbauthz.IsNotAuthorizedError(err) {
			retryable = false
			details = "unauthorized"
		}
		w.errorThenClose(workspacesdk.WatchError{
			Code:      workspacesdk.WatchErrorDatabase,
			Retryable: retryable,
			Message:   "failed to fetch workspace agents",
			Details:   details,
		})
		return false
	}
	if len(agents) == 0 {
		w.errorThenClose(workspacesdk.WatchError{
			Code:      workspacesdk.WatchErrorNoAgents,
			Retryable: false,
			Message:   "no agents found for workspace",
		})
		return false
	}
	if len(agents) > 1 && w.agentName == "" {
		w.errorThenClose(workspacesdk.WatchError{
			Code:      workspacesdk.WatchErrorTooManyAgents,
			Retryable: false,
			Message:   "more than one agent on workspace and target not specified",
		})
		return false
	}
	var agent database.WorkspaceAgent
	if w.agentName == "" {
		agent = agents[0]
	} else {
		for _, a := range agents {
			if a.Name == w.agentName {
				agent = a
				break
			}
		}
		if agent.ID == uuid.Nil {
			w.errorThenClose(workspacesdk.WatchError{
				Code:      workspacesdk.WatchErrorNameNotFound,
				Retryable: false,
				Message:   "target agent not found by name",
			})
			return false
		}
	}

	err = w.enc.Encode(workspacesdk.ConnectionWatchEvent{AgentUpdate: &workspacesdk.AgentUpdate{
		Lifecycle: codersdk.WorkspaceAgentLifecycle(agent.LifecycleState),
		ID:        agent.ID,
	}})
	if err != nil {
		// probably this is just that the connection is closed, but in case there is some actual JSON serialization
		// error, send a close frame.
		_ = w.conn.Close(websocket.StatusInternalError, "failed to encode agent update")
		return false
	}
	return true
}

func (w *Watcher) errorThenClose(err workspacesdk.WatchError) {
	_ = w.enc.Encode(workspacesdk.ConnectionWatchEvent{Error: &err})
	// ignore encoding errors above because in any case, we are going to close the connection.
	_ = w.conn.Close(websocket.StatusNormalClosure, "error")
	if w.jobLogSubCancel != nil {
		w.jobLogSubCancel()
		w.jobLogSubCancel = nil
	}
}

func (w *Watcher) subscribeToJobLogs() error {
	// restart the logID, since we are subscribing to a new job ID, and log IDs are scoped to the job.
	w.lastLogID = 0
	if w.jobLogSubCancel != nil {
		w.jobLogSubCancel()
	}
	var err error
	w.jobLogSubCancel, err = w.sub.SubscribeWithErr(
		provisionersdk.ProvisionerJobLogsNotifyChannel(w.lastBuild.JobID),
		w.jobLogListener)
	if err != nil {
		return err
	}
	return nil
}

func (w *Watcher) jobLogListener(_ context.Context, message []byte, err error) {
	n := new(provisionersdk.ProvisionerJobLogsNotifyMessage)
	if err == nil {
		err = json.Unmarshal(message, &n)
	}
	if err != nil {
		// This means there was a problem with the pubsub, like a disconnection, or with decoding. In either case we
		// don't know if some new logs have arrived during the disconnection, so send a notify that will trigger a query
		// of the latest.
		n.CreatedAfter = math.MaxInt64
		n.EndOfLogs = false
	}
	select {
	case w.events <- event{jobLogNotify: n}:
	case <-w.connCtx.Done():
		return
	}
}

func (w *Watcher) queryJobLogs() (ok bool) {
	w.logger.Debug(w.connCtx, "querying logs", slog.F("after", w.lastLogID))
	logs, err := w.db.GetProvisionerLogsAfterID(w.connCtx, database.GetProvisionerLogsAfterIDParams{
		JobID:        w.lastBuild.JobID,
		CreatedAfter: w.lastLogID,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		details := err.Error()
		retryable := true
		if dbauthz.IsNotAuthorizedError(err) {
			retryable = false
			details = "unauthorized"
		}
		w.errorThenClose(workspacesdk.WatchError{
			Code:      workspacesdk.WatchErrorDatabase,
			Retryable: retryable,
			Message:   "failed to fetch build logs",
			Details:   details,
		})
		return false
	}
	for _, log := range logs {
		sdkLog := db2sdk.ConvertProvisionerJobLog(log)
		err = w.enc.Encode(workspacesdk.ConnectionWatchEvent{JobLog: &sdkLog})
		if err != nil {
			// probably this is just that the connection is closed, but in case there is some actual JSON serialization
			// error, send a close frame.
			_ = w.conn.Close(websocket.StatusInternalError, "failed to encode agent update")
			return false
		}
		w.lastLogID = log.ID
		w.logger.Debug(w.connCtx, "wrote log to websocket", slog.F("id", log.ID))
	}
	return true
}
