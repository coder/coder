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
	Start(conn workspacesdk.AgentConn)
}

// WorkspaceStarter is used to create a start build of the workspace. It is an interface here because the CLI has lots
// of complex logic for determining the build parameters including prompting and environment variables, which we don't
// want to burden the Tunneler with. Other users of the Tunneler like `scaletest` can have a much simpler
// implementation.
type WorkspaceStarter interface {
	StartWorkspace() error
}

const (
	stateInit state = iota
	exit
	waitToStart
	waitForWorkspaceStarted
	waitForAgent
	establishTailnet
	tailnetUp
	applicationUp
	shutdownApplication
	shutdownTailnet
	maxState // used for testing
)

type Tunneler struct {
	config    Config
	ctx       context.Context
	cancel    context.CancelFunc
	client    *workspacesdk.Client
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
	// TODO: commented out to appease linter
	// transition codersdk.WorkspaceTransition
	// id         uuid.UUID
}

type networkedApplicationUpdate struct {
	// up is true if the application is up. False if it is down.
	up bool
}

type tailnetUpdate struct {
	// up is true if the tailnet is up. False if it is down.
	up bool
}

func NewTunneler(client *workspacesdk.Client, config Config) *Tunneler {
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
	}
}

func (t *Tunneler) handleSignal() {
	switch t.state {
	case exit, shutdownTailnet, shutdownApplication:
		return
	case tailnetUp, applicationUp:
		t.wg.Add(1)
		go t.closeApp()
		t.state = shutdownApplication
	case establishTailnet:
		t.wg.Add(1)
		go t.shutdownTailnet()
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
		case establishTailnet:
			// new build after we're already connecting
			t.wg.Add(1)
			go t.shutdownTailnet()
			t.state = shutdownTailnet
		case applicationUp, tailnetUp:
			// new build after we have already connected
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
		case establishTailnet:
			// new build after we're already connecting
			t.wg.Add(1)
			go t.shutdownTailnet()
			t.state = shutdownTailnet
			return
		case applicationUp, tailnetUp:
			// new build after we have already connected
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

func (*Tunneler) handleAgentUpdate(*agentUpdate) {
}

func (*Tunneler) handleAgentLog(*codersdk.WorkspaceAgentLog) {
}

func (*Tunneler) handleAppUpdate(*networkedApplicationUpdate) {
}

func (*Tunneler) handleTailnetUpdate(*tailnetUpdate) {
}

func (t *Tunneler) closeApp() {
	defer t.wg.Done()
	err := t.config.App.Close()
	if err != nil {
		t.config.DebugLogger.Error(t.ctx, "failed to close networked application", slog.Error(err))
	}
	select {
	case <-t.ctx.Done():
		t.config.DebugLogger.Info(t.ctx, "context expired before sending app down")
	case t.events <- tunnelerEvent{appUpdate: &networkedApplicationUpdate{up: false}}:
	}
}

func (t *Tunneler) startWorkspace() {
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
		case t.events <- tunnelerEvent{shutdownSignal: &shutdownSignal{}}:
		}
	}
}

func (t *Tunneler) shutdownTailnet() {
	defer t.wg.Done()
	err := t.agentConn.Close()
	if err != nil {
		t.config.DebugLogger.Error(t.ctx, "failed to close agent connection", slog.Error(err))
	}
	select {
	case <-t.ctx.Done():
		t.config.DebugLogger.Debug(t.ctx, "context expired before sending event after shutting down tailnet")
	case t.events <- tunnelerEvent{tailnetUpdate: &tailnetUpdate{up: false}}:
	}
}
