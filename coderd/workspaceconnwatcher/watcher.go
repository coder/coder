package workspaceconnwatcher

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
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
}

type event struct {
	sync    bool
	wsEvent *wspubsub.WorkspaceEvent
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

	filteredEvents := make(chan event, 1)
	filteredEvents <- event{sync: true} // init sync
	cancelWorkspaceSubscribe, err := w.sub.SubscribeWithErr(wspubsub.WorkspaceEventChannel(workspace.OwnerID),
		wspubsub.HandleWorkspaceEvent(
			func(ctx context.Context, payload wspubsub.WorkspaceEvent, err error) {
				if err != nil {
					// subscription error, resync
					select {
					case filteredEvents <- event{sync: true}:
					case <-ctx.Done():
					}
					return
				}
				if payload.WorkspaceID != workspace.ID {
					return
				}
				select {
				case filteredEvents <- event{wsEvent: &payload}:
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

	conn, err := websocket.Accept(rw, r, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to accept WebSocket.",
			Detail:  err.Error(),
		})
		return
	}

	// CloseRead starts a goroutine to read and discard messages from the client,
	// including Pong messages sent in response to our Ping heartbeats.
	_ = conn.CloseRead(ctx)

	ctx, cancel := context.WithCancel(ctx)
	go httpapi.HeartbeatClose(ctx, w.logger, cancel, conn)
	defer cancel()

	u := &updater{
		db:          w.db,
		watcherCtx:  w.ctx,
		connCtx:     ctx,
		conn:        conn,
		workspaceID: workspace.ID,
		events:      filteredEvents,
		agentName:   agentName,
	}
	u.run()
}

func (w *Watcher) Close() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()

	w.cancel()
	w.wg.Wait()
}

type updater struct {
	db          database.Store
	watcherCtx  context.Context
	connCtx     context.Context
	conn        *websocket.Conn
	enc         *wsjson.Encoder[workspacesdk.ConnectionWatchEvent]
	workspaceID uuid.UUID
	events      <-chan event
	agentName   string

	lastBuild database.GetLatestWorkspaceBuildWithStatusByWorkspaceIDRow
}

func (u *updater) run() {
	u.enc = wsjson.NewEncoder[workspacesdk.ConnectionWatchEvent](u.conn, websocket.MessageText)
	defer func() {
		// this is a no-op if we have already closed for some other reason.
		_ = u.enc.Close(websocket.StatusNormalClosure)
	}()

	for {
		select {
		case <-u.watcherCtx.Done():
			u.errorThenClose(workspacesdk.WatchError{
				Code:      workspacesdk.WatchErrorServerShutdown,
				Retryable: true,
				Message:   "server is shutting down",
			})
			return
		case <-u.connCtx.Done():
			return
		case e := <-u.events:
			if e.sync {
				// zero this out so we'll send a full update
				u.lastBuild = database.GetLatestWorkspaceBuildWithStatusByWorkspaceIDRow{}
				if !u.buildUpdate() {
					return
				}
			}
			if e.wsEvent != nil {
				switch e.wsEvent.Kind {
				case wspubsub.WorkspaceEventKindStateChange:
					if !u.buildUpdate() {
						return
					}
				case wspubsub.WorkspaceEventKindAgentLifecycleUpdate:
					if !u.maybeSendAgentUpdate() {
						return
					}
				}
			}
		}
	}
}

func (u *updater) buildUpdate() bool {
	build, err := u.db.GetLatestWorkspaceBuildWithStatusByWorkspaceID(u.connCtx, u.workspaceID)
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
		u.errorThenClose(workspacesdk.WatchError{
			Code:      workspacesdk.WatchErrorDatabase,
			Retryable: retryable,
			Message:   "failed to fetch latest workspace build",
			Details:   details,
		})
		return false
	}

	if build.BuildNumber != u.lastBuild.BuildNumber ||
		build.JobStatus != u.lastBuild.JobStatus ||
		build.Transition != u.lastBuild.Transition {
		u.lastBuild = build
		err = u.enc.Encode(workspacesdk.ConnectionWatchEvent{BuildUpdate: &workspacesdk.BuildUpdate{
			Transition: codersdk.WorkspaceTransition(build.Transition),
			JobStatus:  codersdk.ProvisionerJobStatus(build.JobStatus),
		}})
		if err != nil {
			// probably this is just that the connection is closed, but in case there is some actual JSON serialization
			// error, send a close frame.
			_ = u.conn.Close(websocket.StatusInternalError, "failed to encode build update")
			return false
		}
		return u.maybeSendAgentUpdate()
	}
	return true
}

func (u *updater) maybeSendAgentUpdate() (ok bool) {
	if u.lastBuild.Transition != database.WorkspaceTransitionStart ||
		u.lastBuild.JobStatus != database.ProvisionerJobStatusSucceeded {
		// only send agent updates for successfully started workspaces
		return true
	}

	agents, err := u.db.GetWorkspaceAgentsByWorkspaceAndBuildNumber(u.connCtx,
		database.GetWorkspaceAgentsByWorkspaceAndBuildNumberParams{
			WorkspaceID: u.workspaceID,
			BuildNumber: u.lastBuild.BuildNumber,
		})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		details := err.Error()
		retryable := true
		if dbauthz.IsNotAuthorizedError(err) {
			retryable = false
			details = "unauthorized"
		}
		u.errorThenClose(workspacesdk.WatchError{
			Code:      workspacesdk.WatchErrorDatabase,
			Retryable: retryable,
			Message:   "failed to fetch workspace agents",
			Details:   details,
		})
		return false
	}
	if len(agents) == 0 {
		u.errorThenClose(workspacesdk.WatchError{
			Code:      workspacesdk.WatchErrorNoAgents,
			Retryable: false,
			Message:   "no agents found for workspace",
		})
		return false
	}
	if len(agents) > 1 && u.agentName == "" {
		u.errorThenClose(workspacesdk.WatchError{
			Code:      workspacesdk.WatchErrorTooManyAgents,
			Retryable: false,
			Message:   "more than one agent on workspace and target not specified",
		})
		return false
	}
	var agent database.WorkspaceAgent
	if u.agentName == "" {
		agent = agents[0]
	} else {
		for _, a := range agents {
			if a.Name == u.agentName {
				agent = a
				break
			}
		}
		if agent.ID == uuid.Nil {
			u.errorThenClose(workspacesdk.WatchError{
				Code:      workspacesdk.WatchErrorNameNotFound,
				Retryable: false,
				Message:   "target agent not found by name",
			})
			return false
		}
	}

	err = u.enc.Encode(workspacesdk.ConnectionWatchEvent{AgentUpdate: &workspacesdk.AgentUpdate{
		Lifecycle: codersdk.WorkspaceAgentLifecycle(agent.LifecycleState),
		ID:        agent.ID,
	}})
	if err != nil {
		// probably this is just that the connection is closed, but in case there is some actual JSON serialization
		// error, send a close frame.
		_ = u.conn.Close(websocket.StatusInternalError, "failed to encode agent update")
		return false
	}
	return true
}

func (u *updater) errorThenClose(err workspacesdk.WatchError) {
	_ = u.enc.Encode(workspacesdk.ConnectionWatchEvent{Error: &err})
	// ignore encoding errors above because in any case, we are going to close the connection.
	_ = u.conn.Close(websocket.StatusNormalClosure, "error")
}
