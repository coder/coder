package agentdesktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"sync"

	"github.com/go-chi/chi/v5"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

// API exposes the desktop streaming HTTP routes for the agent.
type API struct {
	logger slog.Logger

	mu      sync.Mutex
	session *desktopSession // nil until first connection
	closed  bool
}

type desktopSession struct {
	cmd     *exec.Cmd
	vncPort int
	cancel  context.CancelFunc
}

// portableDesktopOutput is the JSON output from `portabledesktop up --json`.
type portableDesktopOutput struct {
	Port int `json:"port"`
}

// NewAPI creates a new desktop streaming API.
func NewAPI(logger slog.Logger) *API {
	return &API{
		logger: logger,
	}
}

// Routes returns the chi router for mounting at /api/v0/desktop.
func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", a.handleDesktop)
	return r
}

func (a *API) handleDesktop(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check for the binary.
	if _, err := exec.LookPath("portabledesktop"); err != nil {
		httpapi.Write(ctx, rw, http.StatusFailedDependency, codersdk.Response{
			Message: "portabledesktop binary not found.",
			Detail:  "The portabledesktop binary must be installed in PATH to use desktop streaming.",
		})
		return
	}

	// Get or create the singleton session.
	session, err := a.getOrCreateSession(ctx)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to start desktop session.",
			Detail:  err.Error(),
		})
		return
	}

	// Accept WebSocket from coderd.
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		a.logger.Error(ctx, "failed to accept websocket", slog.Error(err))
		return
	}

	// No read limit — RFB framebuffer updates can be large.
	conn.SetReadLimit(-1)

	wsCtx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageBinary)
	defer wsNetConn.Close()

	// Dial Xvnc over local TCP.
	tcp, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", session.vncPort))
	if err != nil {
		a.logger.Error(ctx, "failed to dial VNC server", slog.Error(err))
		_ = conn.Close(websocket.StatusInternalError, "Failed to connect to VNC server.")
		return
	}
	defer tcp.Close()

	// Bicopy raw bytes between WebSocket and VNC TCP.
	agentssh.Bicopy(wsCtx, wsNetConn, tcp)
}

// getOrCreateSession returns the existing desktop session or creates a
// new one by running `portabledesktop up --json`.
func (a *API) getOrCreateSession(ctx context.Context) (*desktopSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil, xerrors.New("desktop API is closed")
	}

	// If we have an existing session, check if it's still alive.
	if a.session != nil {
		if a.session.cmd.ProcessState != nil && a.session.cmd.ProcessState.Exited() {
			// Process died, clean up and recreate.
			a.logger.Warn(ctx, "portabledesktop process died, recreating session")
			a.session.cancel()
			a.session = nil
		} else {
			return a.session, nil
		}
	}

	// Spawn portabledesktop up --json.
	sessionCtx, sessionCancel := context.WithCancel(context.Background())

	//nolint:gosec // portabledesktop is a trusted binary looked up via PATH.
	cmd := exec.CommandContext(sessionCtx, "portabledesktop", "up", "--json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sessionCancel()
		return nil, xerrors.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		sessionCancel()
		return nil, xerrors.Errorf("start portabledesktop: %w", err)
	}

	// Parse the JSON output to get the VNC port.
	var output portableDesktopOutput
	if err := json.NewDecoder(stdout).Decode(&output); err != nil {
		sessionCancel()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, xerrors.Errorf("parse portabledesktop output: %w", err)
	}

	if output.Port == 0 {
		sessionCancel()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, xerrors.New("portabledesktop returned port 0")
	}

	a.logger.Info(ctx, "started portabledesktop session",
		slog.F("vnc_port", output.Port),
		slog.F("pid", cmd.Process.Pid),
	)

	a.session = &desktopSession{
		cmd:     cmd,
		vncPort: output.Port,
		cancel:  sessionCancel,
	}

	return a.session, nil
}

// Close shuts down the desktop session if one is running.
func (a *API) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	if a.session != nil {
		a.session.cancel()
		// Xvnc is a child process — killing it cleans up the X
		// session.
		_ = a.session.cmd.Process.Kill()
		_ = a.session.cmd.Wait()
		a.session = nil
	}
	return nil
}
