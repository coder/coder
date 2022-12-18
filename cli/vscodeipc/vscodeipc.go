package vscodeipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

const AuthHeader = "Coder-IPC-Token"

// New creates a VS Code IPC client that can be used to communicate with workspaces.
//
// Creating this IPC was required instead of using SSH, because we're unable to get
// connection information to display in the bottom-bar when using SSH. It's possible
// we could jank around this (maybe by using a temporary SSH host), but that's not
// ideal.
//
// This persists a single workspace connection, and lets you execute commands, check
// for network information, and forward ports.
//
// The VS Code extension is located at https://github.com/coder/vscode-coder. The
// extension downloads the slim binary from `/bin/*` and executes `coder vscodeipc`
// which calls this function. This API must maintain backward compatibility with
// the extension to support prior versions of Coder.
func New(ctx context.Context, client *codersdk.Client, agentID uuid.UUID, options *codersdk.DialWorkspaceAgentOptions) (http.Handler, io.Closer, error) {
	if options == nil {
		options = &codersdk.DialWorkspaceAgentOptions{}
	}
	// We need this to track upload and download!
	options.EnableTrafficStats = true

	agentConn, err := client.DialWorkspaceAgent(ctx, agentID, options)
	if err != nil {
		return nil, nil, err
	}
	api := &api{
		agentConn: agentConn,
	}
	r := chi.NewRouter()
	// This is to prevent unauthorized clients on the same machine from executing
	// requests on behalf of the workspace.
	r.Use(sessionTokenMiddleware(client.SessionToken()))
	r.Route("/v1", func(r chi.Router) {
		r.Get("/port/{port}", api.port)
		r.Get("/network", api.network)
		r.Post("/execute", api.execute)
	})
	return r, api, nil
}

type api struct {
	agentConn     *codersdk.AgentConn
	sshClient     *ssh.Client
	sshClientErr  error
	sshClientOnce sync.Once

	lastNetwork time.Time
}

func (api *api) Close() error {
	if api.sshClient != nil {
		api.sshClient.Close()
	}
	return api.agentConn.Close()
}

type NetworkResponse struct {
	P2P              bool               `json:"p2p"`
	Latency          float64            `json:"latency"`
	PreferredDERP    string             `json:"preferred_derp"`
	DERPLatency      map[string]float64 `json:"derp_latency"`
	UploadBytesSec   int64              `json:"upload_bytes_sec"`
	DownloadBytesSec int64              `json:"download_bytes_sec"`
}

// port accepts an HTTP request to dial a port on the workspace agent.
// It uses an HTTP connection upgrade to transfer the connection to TCP.
func (api *api) port(w http.ResponseWriter, r *http.Request) {
	port, err := strconv.Atoi(chi.URLParam(r, "port"))
	if err != nil {
		httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{
			Message: "Port must be an integer!",
		})
		return
	}
	remoteConn, err := api.agentConn.DialContext(r.Context(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		httpapi.InternalServerError(w, err)
		return
	}
	defer remoteConn.Close()

	// Upgrade an switch to TCP!
	w.Header().Set("Connection", "Upgrade")
	w.Header().Set("Upgrade", "tcp")
	w.WriteHeader(http.StatusSwitchingProtocols)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		httpapi.InternalServerError(w, xerrors.Errorf("unable to hijack connection: %T", w))
		return
	}

	localConn, brw, err := hijacker.Hijack()
	if err != nil {
		httpapi.InternalServerError(w, err)
		return
	}
	defer localConn.Close()

	_ = brw.Flush()
	agent.Bicopy(r.Context(), localConn, remoteConn)
}

// network returns network information about the workspace.
func (api *api) network(w http.ResponseWriter, r *http.Request) {
	// Ping the workspace agent to get the latency.
	latency, p2p, err := api.agentConn.Ping(r.Context())
	if err != nil {
		httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to ping the workspace agent.",
			Detail:  err.Error(),
		})
		return
	}

	node := api.agentConn.Node()
	derpMap := api.agentConn.DERPMap()
	derpLatency := map[string]float64{}

	// Convert DERP region IDs to friendly names for display in the UI.
	for rawRegion, latency := range node.DERPLatency {
		regionParts := strings.SplitN(rawRegion, "-", 2)
		regionID, err := strconv.Atoi(regionParts[0])
		if err != nil {
			continue
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
		// Convert the microseconds to milliseconds.
		derpLatency[region.RegionName] = latency * 1000
	}

	totalRx := uint64(0)
	totalTx := uint64(0)
	for _, stat := range api.agentConn.ExtractTrafficStats() {
		totalRx += stat.RxBytes
		totalTx += stat.TxBytes
	}
	// Tracking the time since last request is required because
	// ExtractTrafficStats() resets its counters after each call.
	dur := time.Since(api.lastNetwork)
	uploadSecs := float64(totalTx) / dur.Seconds()
	downloadSecs := float64(totalRx) / dur.Seconds()

	api.lastNetwork = time.Now()

	httpapi.Write(r.Context(), w, http.StatusOK, NetworkResponse{
		P2P:              p2p,
		Latency:          float64(latency.Microseconds()) / 1000,
		PreferredDERP:    derpMap.Regions[node.PreferredDERP].RegionName,
		DERPLatency:      derpLatency,
		UploadBytesSec:   int64(uploadSecs),
		DownloadBytesSec: int64(downloadSecs),
	})
}

type ExecuteRequest struct {
	Command string `json:"command"`
	Stdin   string `json:"stdin"`
}

type ExecuteResponse struct {
	Data     string `json:"data"`
	ExitCode *int   `json:"exit_code"`
}

// execute runs the command provided, streams the output back, and returns the exit code.
func (api *api) execute(w http.ResponseWriter, r *http.Request) {
	var req ExecuteRequest
	if !httpapi.Read(r.Context(), w, r, &req) {
		return
	}
	api.sshClientOnce.Do(func() {
		// The SSH client is lazily created because it's not needed for
		// all requests. It's only needed for the execute endpoint.
		//
		// It's alright if this fails on the first execution, because
		// a new instance of this API is created for each remote SSH request.
		api.sshClient, api.sshClientErr = api.agentConn.SSHClient(context.Background())
	})
	if api.sshClientErr != nil {
		httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create SSH client.",
			Detail:  api.sshClientErr.Error(),
		})
		return
	}
	session, err := api.sshClient.NewSession()
	if err != nil {
		httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create SSH session.",
			Detail:  err.Error(),
		})
		return
	}
	defer session.Close()
	f, ok := w.(http.Flusher)
	if !ok {
		httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
			Message: fmt.Sprintf("http.ResponseWriter is not http.Flusher: %T", w),
		})
		return
	}

	execWriter := &execWriter{w, f}
	session.Stdout = execWriter
	session.Stderr = execWriter
	session.Stdin = strings.NewReader(req.Stdin)
	err = session.Start(req.Command)
	if err != nil {
		httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to start SSH session.",
			Detail:  err.Error(),
		})
		return
	}
	err = session.Wait()

	writeExit := func(exitCode int) {
		data, _ := json.Marshal(&ExecuteResponse{
			ExitCode: &exitCode,
		})
		_, _ = w.Write(data)
		f.Flush()
	}

	if err != nil {
		var exitError *ssh.ExitError
		if errors.As(err, &exitError) {
			writeExit(exitError.ExitStatus())
			return
		}
	}
	writeExit(0)
}

type execWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (e *execWriter) Write(data []byte) (int, error) {
	js, err := json.Marshal(&ExecuteResponse{
		Data: string(data),
	})
	if err != nil {
		return 0, err
	}
	_, err = e.w.Write(js)
	if err != nil {
		return 0, err
	}
	e.f.Flush()
	return len(data), nil
}

func sessionTokenMiddleware(sessionToken string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get(AuthHeader)
			if token == "" {
				httpapi.Write(r.Context(), w, http.StatusUnauthorized, codersdk.Response{
					Message: fmt.Sprintf("A session token must be provided in the `%s` header.", AuthHeader),
				})
				return
			}
			if token != sessionToken {
				httpapi.Write(r.Context(), w, http.StatusUnauthorized, codersdk.Response{
					Message: "The session token provided doesn't match the one used to create the client.",
				})
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", "*")
			h.ServeHTTP(w, r)
		})
	}
}
