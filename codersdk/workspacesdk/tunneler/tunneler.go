package tunneler

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type state int

// NetworkedApplication is the application that runs on top of the tailnet tunnel.
type NetworkedApplication interface {
	// Closer is used to gracefully tear down the application prior to stopping the tunnel.
	io.Closer
	// Start the NetworkedApplication, using the provided AgentConn to connect.
	Start(conn workspacesdk.AgentConn) error
}

// WorkspaceStarter is used to create a start build of the workspace. It is an interface here because the CLI has lots
// of complex logic for determining the build parameters including prompting and environment variables, which we don't
// want to burden the Tunneler with. Other users of the Tunneler like `scaletest` can have a much simpler
// implementation.
type WorkspaceStarter interface {
	StartWorkspace() error
}

type Client interface {
	DialAgent(dialCtx context.Context, agentID uuid.UUID, options *workspacesdk.DialAgentOptions) (workspacesdk.AgentConn, error)
}

const (
	// stateInit is the initial state of the FSM.
	stateInit state = iota
	// exit is the final state of the FSM, and implies that everything is closed or closing.
	exit
	// waitToStart means the workspace is in a state where we have to wait before we can create a new start build
	waitToStart
	// waitForWorkspaceStarted means the workspace is starting, or we have kicked off a goroutine to start it
	waitForWorkspaceStarted
	// waitForAgent means the workspace has started and we are waiting for the agent to connect or be ready
	waitForAgent
	// establishTailnet means we have kicked off a goroutine to dial the agent and are waiting for its results
	establishTailnet
	// tailnetUp means the tailnet connection came up and we kicked off a goroutine to start the NetworkedApplication.
	tailnetUp
	// applicationUp means the NetworkedApplication is up.
	applicationUp
	// shutdownApplication means we are in graceful shut down and waiting for the NetworkedApplication. It could be
	// starting or closing, and we expect to get a networkedApplicationUpdate event when it does.
	shutdownApplication
	// shutdownTailnet means that we are in graceful shut down and waiting for the tailnet. This implies the
	// NetworkedApplication is status is down. E.g. closed or was never started.
	shutdownTailnet
	// maxState is not a valid state for the FSM, and must be last in this list. It allows tests to iterate over all
	// valid states using `range maxState`.
	maxState // used for testing
)

func (s state) String() string {
	switch s {
	case stateInit:
		return "init"
	case exit:
		return "exit"
	case waitToStart:
		return "waitToStart"
	case waitForWorkspaceStarted:
		return "waitForWorkspaceStarted"
	case waitForAgent:
		return "waitForAgent"
	case establishTailnet:
		return "establishTailnet"
	case tailnetUp:
		return "tailnetUp"
	case applicationUp:
		return "applicationUp"
	case shutdownApplication:
		return "shutdownApplication"
	case shutdownTailnet:
		return "shutdownTailnet"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

type Tunneler struct {
	config    Config
	ctx       context.Context
	cancel    context.CancelFunc
	client    Client
	state     state
	agentConn workspacesdk.AgentConn
	events    chan tunnelerEvent
	wg        sync.WaitGroup
}

type Config struct {
	// Required
	WorkspaceID      uuid.UUID
	App              NetworkedApplication
	WorkspaceStarter WorkspaceStarter

	// Optional:

	// AgentName is the name of the agent to tunnel to. If blank, assumes workspace has only one agent and will cause
	// an error if that is not the case.
	AgentName string
	// NoAutostart can be set to true to prevent the tunneler from automatically starting the workspace.
	NoAutostart bool
	// NoWaitForScripts can be set to true to cause the tunneler to dial as soon as the agent is up, not waiting for
	// nominally blocking startup scripts.
	NoWaitForScripts bool
	// LogWriter is used to write progress logs (build, scripts, etc) if non-nil.
	LogWriter io.Writer
	// DebugLogger is used for logging internal messages and errors for debugging (e.g. in tests)
	DebugLogger slog.Logger
}

// tunnelerEvent is an event relevant to setting up a tunnel. ONE of the fields is non-null per event to allow explicit
// ordering.
type tunnelerEvent struct {
	shutdownSignal    *shutdownSignal
	buildUpdate       *buildUpdate
	provisionerJobLog *codersdk.ProvisionerJobLog
	agentUpdate       *agentUpdate
	agentLog          *codersdk.WorkspaceAgentLog
	appUpdate         *networkedApplicationUpdate
	tailnetUpdate     *tailnetUpdate
}

type shutdownSignal struct{}

type buildUpdate struct {
	transition codersdk.WorkspaceTransition
	jobStatus  codersdk.ProvisionerJobStatus
}

type agentUpdate struct {
	lifecycle codersdk.WorkspaceAgentLifecycle
	id        uuid.UUID
}

type networkedApplicationUpdate struct {
	// up is true if the application is up. False if it is down.
	up  bool
	err error
}

type tailnetUpdate struct {
	// up is true if the tailnet is up. False if it is down.
	up   bool
	conn workspacesdk.AgentConn
	err  error
}

func NewTunneler(client Client, config Config) *Tunneler {
	t := &Tunneler{
		config: config,
		client: client,
		events: make(chan tunnelerEvent),
	}
	// this context ends when we successfully gracefully shut down or are forced closed.
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.wg.Add(2)
	go t.start()
	go t.eventLoop()
	return t
}

func (t *Tunneler) start() {
	defer t.wg.Done()
	// here we would subscribe to updates.
	// t.client.AgentConnectionWatch(t.config.WorkspaceID, t.config.AgentName)
}

func (t *Tunneler) eventLoop() {
	defer t.wg.Done()
	for t.state != exit {
		var e tunnelerEvent
		select {
		case <-t.ctx.Done():
			t.state = exit
			return
		case e = <-t.events:
		}
		switch {
		case e.shutdownSignal != nil:
			t.handleSignal()
		case e.buildUpdate != nil:
			t.handleBuildUpdate(e.buildUpdate)
		case e.provisionerJobLog != nil:
			t.handleProvisionerJobLog(e.provisionerJobLog)
		case e.agentUpdate != nil:
			t.handleAgentUpdate(e.agentUpdate)
		case e.agentLog != nil:
			t.handleAgentLog(e.agentLog)
		case e.appUpdate != nil:
			t.handleAppUpdate(e.appUpdate)
		case e.tailnetUpdate != nil:
			t.handleTailnetUpdate(e.tailnetUpdate)
		}
		t.config.DebugLogger.Debug(t.ctx, "handled event", slog.F("state", t.state))
	}
}

func (t *Tunneler) handleSignal() {
	t.config.DebugLogger.Debug(t.ctx, "got shutdown signal")
	switch t.state {
	case exit, shutdownTailnet, shutdownApplication:
		return
	case applicationUp:
		t.wg.Add(1)
		go t.closeApp()
		t.state = shutdownApplication
	case tailnetUp:
		// waiting for app to start; setting state here will cause us to tear it down when the app start goroutine
		// event comes in.
		t.state = shutdownApplication
	case establishTailnet:
		// waiting for tailnet to start; setting state here will cause us to tear it down when the tailnet dial
		// goroutine event comes in.
		t.state = shutdownTailnet
	case stateInit, waitToStart, waitForWorkspaceStarted, waitForAgent:
		t.cancel() // stops the watch
		t.state = exit
	default:
		t.config.DebugLogger.Critical(t.ctx, "missing case in handleSignal()", slog.F("state", t.state))
	}
}

func (t *Tunneler) handleBuildUpdate(update *buildUpdate) {
	if t.state == shutdownTailnet || t.state == shutdownApplication || t.state == exit {
		return // no-op
	}

	var canMakeProgress, jobUnhealthy bool
	switch update.jobStatus {
	case codersdk.ProvisionerJobPending, codersdk.ProvisionerJobRunning:
		canMakeProgress = true
	case codersdk.ProvisionerJobSucceeded:
	default:
		jobUnhealthy = true
	}

	if update.transition == codersdk.WorkspaceTransitionDelete {
		t.config.DebugLogger.Info(t.ctx, "workspace is being deleted", slog.F("job_status", update.jobStatus))
		// treat same as signal
		t.handleSignal()
		return
	}
	if jobUnhealthy {
		t.config.DebugLogger.Info(t.ctx, "build job is in unhealthy state", slog.F("job_status", update.jobStatus))
		// treat same as signal
		t.handleSignal()
		return
	}

	if update.transition == codersdk.WorkspaceTransitionStart && canMakeProgress {
		t.config.DebugLogger.Debug(t.ctx, "workspace is starting", slog.F("job_status", update.jobStatus))
		switch t.state {
		// new build after we have already connected
		case establishTailnet: // we are starting the tailnet
			t.state = shutdownTailnet
		case tailnetUp: // we are starting the application
			t.state = shutdownApplication
		case applicationUp:
			t.wg.Add(1)
			go t.closeApp()
			t.state = shutdownApplication
		default:
			t.state = waitForWorkspaceStarted
		}
		return
	}
	if update.transition == codersdk.WorkspaceTransitionStart && update.jobStatus == codersdk.ProvisionerJobSucceeded {
		t.config.DebugLogger.Debug(t.ctx, "workspace is started", slog.F("job_status", update.jobStatus))
		switch t.state {
		case establishTailnet, applicationUp, tailnetUp:
			// no-op. Later agent updates will tell us whether the tailnet connection is current.
		default:
			t.state = waitForAgent
		}
		return
	}

	if update.transition == codersdk.WorkspaceTransitionStop {
		// these cases take effect regardless of whether the transition is complete or not
		switch t.state {
		// all 3 of these mean a new build after we have already started connecting
		case establishTailnet: // waiting for tailnet to start
			t.state = shutdownTailnet
			return
		case tailnetUp: // waiting for application to start
			t.state = shutdownApplication
			return
		case applicationUp:
			t.wg.Add(1)
			go t.closeApp()
			t.state = shutdownApplication
			return
		}
		if t.config.NoAutostart {
			// we are stopped/stopping and configured not to automatically start. Nothing more to do.
			t.cancel()
			t.state = exit
			return
		}
		if update.jobStatus == codersdk.ProvisionerJobSucceeded {
			switch t.state {
			case stateInit, waitToStart, waitForAgent:
				t.wg.Add(1)
				go t.startWorkspace()
				t.state = waitForWorkspaceStarted
				return
			case waitForWorkspaceStarted:
				return
			default:
				// unhittable because all the states where we have started already or are shutting down are handled
				// earlier
				t.config.DebugLogger.Critical(t.ctx, "unhandled build update while stopped", slog.F("state", t.state))
				return
			}
		}
		if canMakeProgress {
			t.state = waitToStart
			return
		}
	}
	// unhittable
	t.config.DebugLogger.Critical(t.ctx, "unhandled build update",
		slog.F("job_status", update.jobStatus), slog.F("transition", update.transition), slog.F("state", t.state))
}

func (*Tunneler) handleProvisionerJobLog(*codersdk.ProvisionerJobLog) {
}

func (t *Tunneler) handleAgentUpdate(update *agentUpdate) {
	t.config.DebugLogger.Debug(t.ctx, "handling agent update",
		slog.F("state", t.state),
		slog.F("lifecycle", update.lifecycle),
		slog.F("agent_id", update.id))
	if t.state != waitForAgent {
		return
	}
	doConnect := func() {
		t.wg.Add(1)
		t.state = establishTailnet
		go t.connectTailnet(update.id)
	}
	// consequence of ignoring updates if we are not waiting for the agent is that we MUST receive
	// the start build succeeded update BEFORE we get the Agent connected / ready update.  We should keep this
	// in mind when implementing the watch in Coderd.
	switch update.lifecycle {
	case codersdk.WorkspaceAgentLifecycleReady:
		doConnect()
		return
	case codersdk.WorkspaceAgentLifecycleStarting,
		codersdk.WorkspaceAgentLifecycleStartError,
		codersdk.WorkspaceAgentLifecycleStartTimeout:
		if t.config.NoWaitForScripts {
			doConnect()
			return
		}
	case codersdk.WorkspaceAgentLifecycleShuttingDown:
	case codersdk.WorkspaceAgentLifecycleShutdownError:
	case codersdk.WorkspaceAgentLifecycleShutdownTimeout:
	case codersdk.WorkspaceAgentLifecycleOff:
	case codersdk.WorkspaceAgentLifecycleCreated: // initial state, so it hasn't connected yet
	default:
		// unhittable, unless new states are added. We structure this with the switch and all cases covered to ensure
		// we cover all cases.
		t.config.DebugLogger.Critical(t.ctx, "unhandled agent update", slog.F("lifecycle", update.lifecycle))
	}
}

func (*Tunneler) handleAgentLog(*codersdk.WorkspaceAgentLog) {
}

func (t *Tunneler) handleAppUpdate(update *networkedApplicationUpdate) {
	if update.up {
		t.config.DebugLogger.Debug(t.ctx, "networked application up")
	} else {
		// we already logged any error, so this is just debug to track the state change
		t.config.DebugLogger.Debug(t.ctx, "networked application down", slog.Error(update.err))
	}
	switch t.state {
	case exit:
		return
	case stateInit, waitToStart, waitForAgent, waitForWorkspaceStarted, establishTailnet:
		t.config.DebugLogger.Error(t.ctx, "unexpected: application update before we started it",
			slog.F("state", t.state), slog.F("app_up", update.up), slog.Error(update.err))
		return
	}
	if update.up {
		switch t.state {
		case tailnetUp:
			t.state = applicationUp
			return
		case applicationUp:
			t.config.DebugLogger.Error(t.ctx, "unexpected: application 'up' update when it is already up")
			return
		case shutdownApplication:
			// this means that we started shutting down while we were waiting for the goroutine that starts the
			// application to complete. We need to tear down the app.
			t.config.DebugLogger.Debug(t.ctx, "gracefully shutting down application after it started")
			t.wg.Add(1)
			go t.closeApp()
			return
		case shutdownTailnet:
			t.config.DebugLogger.Error(t.ctx, "unexpected: application 'up' update when we were tearing down tailnet")
			return
		}
	}
	switch t.state {
	case tailnetUp, applicationUp, shutdownApplication:
		t.state = shutdownTailnet
		t.wg.Add(1)
		go t.shutdownTailnet()
		return
	case shutdownTailnet:
		t.config.DebugLogger.Error(t.ctx, "unexpected: application 'down' update when we were tearing down tailnet")
		return
	}
	t.config.DebugLogger.Critical(t.ctx, "unhandled application update",
		slog.F("state", t.state), slog.F("app_up", update.up))
}

func (t *Tunneler) handleTailnetUpdate(update *tailnetUpdate) {
	switch t.state {
	case exit:
		return
	case stateInit, waitToStart, waitForAgent, waitForWorkspaceStarted:
		t.config.DebugLogger.Error(t.ctx, "unexpected: tailnet update before we started it",
			slog.F("state", t.state), slog.F("app_up", update.up), slog.Error(update.err))
		return
	}
	if update.up {
		t.config.DebugLogger.Debug(t.ctx, "got tailnet 'up' update", slog.F("state", t.state))
		switch t.state {
		case establishTailnet:
			t.agentConn = update.conn
			t.state = tailnetUp
			t.wg.Add(1)
			go t.startApp()
			return
		case shutdownTailnet:
			// this means we were notified to shut down while we were starting the tailnet. We need to tear it down.
			t.config.DebugLogger.Debug(t.ctx, "gracefully shutting down tailnet after it started")
			t.agentConn = update.conn
			t.wg.Add(1)
			go t.shutdownTailnet()
			return
		case tailnetUp:
			t.config.DebugLogger.Error(t.ctx, "unexpected: got tailnet 'up' update when it is already up")
			if update.conn != nil && update.conn != t.agentConn {
				// somehow we have two updates with different connections. Something very bad has happened so we are
				// going to just bail, rather than try to gracefully tear them both down.
				t.config.DebugLogger.Fatal(t.ctx, "unexpected: got two different connections")
			}
			return
		case shutdownApplication:
			t.config.DebugLogger.Error(t.ctx, "unexpected: got tailnet 'up' update when we expected application update")
			return
		}
	}
	t.config.DebugLogger.Debug(t.ctx, "got tailnet 'down' update", slog.F("state", t.state))
	switch t.state {
	case establishTailnet, shutdownTailnet:
		// Either we failed to establish, or we successfully shut down. In the former case, the error has already been
		// logged. Nothing else to do now that tailnet is down, since it implies the application is also down.
		t.cancel()
		t.state = exit
		return
	case tailnetUp:
		t.config.DebugLogger.Error(t.ctx,
			"unexpected: got tailnet 'down' update when we were starting the application")
		return
	case shutdownApplication:
		t.config.DebugLogger.Error(t.ctx,
			"unexpected: got tailnet 'down' update when we were stopping the application")
		return
	}
	t.config.DebugLogger.Critical(t.ctx, "unhandled tailnet update",
		slog.F("state", t.state), slog.F("app_up", update.up))
}

func (t *Tunneler) startApp() {
	t.config.DebugLogger.Debug(t.ctx, "starting networked application")
	defer t.wg.Done()
	err := t.config.App.Start(t.agentConn)
	if err != nil {
		t.config.DebugLogger.Error(t.ctx, "failed to start application", slog.Error(err))
		if t.config.LogWriter != nil {
			_, _ = fmt.Fprintf(t.config.LogWriter, "failed to start: %s", err.Error())
		}
		select {
		case <-t.ctx.Done():
			t.config.DebugLogger.Info(t.ctx,
				"context expired before sending event after failed network application start")
		case t.events <- tunnelerEvent{appUpdate: &networkedApplicationUpdate{up: false, err: err}}:
		}
		return
	}
	select {
	case <-t.ctx.Done():
		t.config.DebugLogger.Info(t.ctx, "context expired before sending network application start update")
	case t.events <- tunnelerEvent{appUpdate: &networkedApplicationUpdate{up: true}}:
	}
}

func (t *Tunneler) closeApp() {
	t.config.DebugLogger.Info(t.ctx, "closing networked application")
	defer t.wg.Done()
	err := t.config.App.Close()
	if err != nil {
		t.config.DebugLogger.Error(t.ctx, "failed to close networked application", slog.Error(err))
	}
	select {
	case <-t.ctx.Done():
		t.config.DebugLogger.Info(t.ctx, "context expired before sending app down")
	case t.events <- tunnelerEvent{appUpdate: &networkedApplicationUpdate{up: false, err: err}}:
	}
}

func (t *Tunneler) startWorkspace() {
	t.config.DebugLogger.Info(t.ctx, "starting workspace")
	defer t.wg.Done()
	err := t.config.WorkspaceStarter.StartWorkspace()
	if err != nil {
		t.config.DebugLogger.Error(t.ctx, "failed to start workspace", slog.Error(err))
		if t.config.LogWriter != nil {
			_, _ = fmt.Fprintf(t.config.LogWriter, "failed to start workspace: %s", err.Error())
		}
		select {
		case <-t.ctx.Done():
			t.config.DebugLogger.Info(t.ctx, "context expired before sending signal after failed workspace start")
		case t.events <- tunnelerEvent{appUpdate: &networkedApplicationUpdate{up: false}}:
		}
		return
	}
}

func (t *Tunneler) connectTailnet(id uuid.UUID) {
	t.config.DebugLogger.Info(t.ctx, "connecting tailnet")
	defer t.wg.Done()
	conn, err := t.client.DialAgent(t.ctx, id, &workspacesdk.DialAgentOptions{
		Logger: t.config.DebugLogger.Named("dialer"),
	})
	if err != nil {
		t.config.DebugLogger.Error(t.ctx, "failed to connect agent", slog.Error(err))
		if t.config.LogWriter != nil {
			_, _ = fmt.Fprintf(t.config.LogWriter, "failed to dial workspace agent: %s", err.Error())
		}
		select {
		case <-t.ctx.Done():
			t.config.DebugLogger.Info(t.ctx, "context expired before sending event after failed agent dial")
		case t.events <- tunnelerEvent{tailnetUpdate: &tailnetUpdate{up: false, err: err}}:
		}
		return
	}
	select {
	case <-t.ctx.Done():
		t.config.DebugLogger.Info(t.ctx, "context expired before sending tailnet conn")
	case t.events <- tunnelerEvent{tailnetUpdate: &tailnetUpdate{up: true, conn: conn}}:
	}
}

func (t *Tunneler) shutdownTailnet() {
	t.config.DebugLogger.Info(t.ctx, "shutting down tailnet")
	defer t.wg.Done()
	err := t.agentConn.Close()
	if err != nil {
		t.config.DebugLogger.Error(t.ctx, "failed to close agent connection", slog.Error(err))
	}
	select {
	case <-t.ctx.Done():
		t.config.DebugLogger.Debug(t.ctx, "context expired before sending event after shutting down tailnet")
	case t.events <- tunnelerEvent{tailnetUpdate: &tailnetUpdate{up: false, err: err}}:
	}
}
