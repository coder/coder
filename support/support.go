package support

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/netcheck"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
)

// Bundle is a set of information discovered about a deployment.
// Even though we do attempt to sanitize data, it may still contain
// sensitive information and should thus be treated as secret.
type Bundle struct {
	Deployment Deployment `json:"deployment"`
	Network    Network    `json:"network"`
	Workspace  Workspace  `json:"workspace"`
	Agent      Agent      `json:"agent"`
	Logs       []string   `json:"logs"`
	CLILogs    []byte     `json:"cli_logs"`
}

type Deployment struct {
	BuildInfo    *codersdk.BuildInfoResponse  `json:"build"`
	Config       *codersdk.DeploymentConfig   `json:"config"`
	Experiments  codersdk.Experiments         `json:"experiments"`
	HealthReport *healthsdk.HealthcheckReport `json:"health_report"`
}

type Network struct {
	ConnectionInfo   workspacesdk.AgentConnectionInfo
	CoordinatorDebug string             `json:"coordinator_debug"`
	Netcheck         *derphealth.Report `json:"netcheck"`
	TailnetDebug     string             `json:"tailnet_debug"`
}

type Netcheck struct {
	Report *netcheck.Report `json:"report"`
	Error  string           `json:"error"`
	Logs   []string         `json:"logs"`
}

type Workspace struct {
	Workspace          codersdk.Workspace                 `json:"workspace"`
	Parameters         []codersdk.WorkspaceBuildParameter `json:"parameters"`
	Template           codersdk.Template                  `json:"template"`
	TemplateVersion    codersdk.TemplateVersion           `json:"template_version"`
	TemplateFileBase64 string                             `json:"template_file_base64"`
	BuildLogs          []codersdk.ProvisionerJobLog       `json:"build_logs"`
}

type Agent struct {
	Agent               *codersdk.WorkspaceAgent                       `json:"agent"`
	ConnectionInfo      *workspacesdk.AgentConnectionInfo              `json:"connection_info"`
	ListeningPorts      *codersdk.WorkspaceAgentListeningPortsResponse `json:"listening_ports"`
	Logs                []byte                                         `json:"logs"`
	ClientMagicsockHTML []byte                                         `json:"client_magicsock_html"`
	AgentMagicsockHTML  []byte                                         `json:"agent_magicsock_html"`
	Manifest            *agentsdk.Manifest                             `json:"manifest"`
	PeerDiagnostics     *tailnet.PeerDiagnostics                       `json:"peer_diagnostics"`
	PingResult          *ipnstate.PingResult                           `json:"ping_result"`
	Prometheus          []byte                                         `json:"prometheus"`
	StartupLogs         []codersdk.WorkspaceAgentLog                   `json:"startup_logs"`
}

// Deps is a set of dependencies for discovering information
type Deps struct {
	// Source from which to obtain information.
	Client *codersdk.Client
	// Log is where to log any informational or warning messages.
	Log slog.Logger
	// WorkspaceID is the optional workspace against which to run connection tests.
	WorkspaceID uuid.UUID
	// AgentID is the optional agent ID against which to run connection tests.
	// Defaults to the first agent of the workspace, if not specified.
	AgentID uuid.UUID
}

func DeploymentInfo(ctx context.Context, client *codersdk.Client, log slog.Logger) Deployment {
	// Note: each goroutine assigns to a different struct field, hence no mutex.
	var (
		d  Deployment
		eg errgroup.Group
	)

	eg.Go(func() error {
		bi, err := client.BuildInfo(ctx)
		if err != nil {
			return xerrors.Errorf("fetch build info: %w", err)
		}
		d.BuildInfo = &bi
		return nil
	})

	eg.Go(func() error {
		dc, err := client.DeploymentConfig(ctx)
		if err != nil {
			return xerrors.Errorf("fetch deployment config: %w", err)
		}
		d.Config = dc
		return nil
	})

	eg.Go(func() error {
		hr, err := healthsdk.New(client).DebugHealth(ctx)
		if err != nil {
			return xerrors.Errorf("fetch health report: %w", err)
		}
		d.HealthReport = &hr
		return nil
	})

	eg.Go(func() error {
		exp, err := client.Experiments(ctx)
		if err != nil {
			return xerrors.Errorf("fetch experiments: %w", err)
		}
		d.Experiments = exp
		return nil
	})

	if err := eg.Wait(); err != nil {
		log.Error(ctx, "fetch deployment information", slog.Error(err))
	}

	return d
}

func NetworkInfo(ctx context.Context, client *codersdk.Client, log slog.Logger) Network {
	var (
		n  Network
		eg errgroup.Group
	)

	eg.Go(func() error {
		coordResp, err := client.Request(ctx, http.MethodGet, "/api/v2/debug/coordinator", nil)
		if err != nil {
			return xerrors.Errorf("fetch coordinator debug page: %w", err)
		}
		defer coordResp.Body.Close()
		bs, err := io.ReadAll(coordResp.Body)
		if err != nil {
			return xerrors.Errorf("read coordinator debug page: %w", err)
		}
		n.CoordinatorDebug = string(bs)
		return nil
	})

	eg.Go(func() error {
		tailResp, err := client.Request(ctx, http.MethodGet, "/api/v2/debug/tailnet", nil)
		if err != nil {
			return xerrors.Errorf("fetch tailnet debug page: %w", err)
		}
		defer tailResp.Body.Close()
		bs, err := io.ReadAll(tailResp.Body)
		if err != nil {
			return xerrors.Errorf("read tailnet debug page: %w", err)
		}
		n.TailnetDebug = string(bs)
		return nil
	})

	eg.Go(func() error {
		// Need connection info to get DERP map for netcheck
		connInfo, err := workspacesdk.New(client).AgentConnectionInfoGeneric(ctx)
		if err != nil {
			log.Warn(ctx, "unable to fetch generic agent connection info")
			return nil
		}
		n.ConnectionInfo = connInfo
		var rpt derphealth.Report
		rpt.Run(ctx, &derphealth.ReportOptions{
			DERPMap: connInfo.DERPMap,
		})
		n.Netcheck = &rpt
		return nil
	})

	if err := eg.Wait(); err != nil {
		log.Error(ctx, "fetch network information", slog.Error(err))
	}

	return n
}

func WorkspaceInfo(ctx context.Context, client *codersdk.Client, log slog.Logger, workspaceID uuid.UUID) Workspace {
	var (
		w  Workspace
		eg errgroup.Group
	)

	if workspaceID == uuid.Nil {
		log.Error(ctx, "no workspace id specified")
		return w
	}

	// dependency, cannot fetch concurrently
	ws, err := client.Workspace(ctx, workspaceID)
	if err != nil {
		log.Error(ctx, "fetch workspace", slog.Error(err), slog.F("workspace_id", workspaceID))
		return w
	}
	for _, res := range ws.LatestBuild.Resources {
		for _, agt := range res.Agents {
			sanitizeEnv(agt.EnvironmentVariables)
		}
	}
	w.Workspace = ws

	eg.Go(func() error {
		buildLogCh, closer, err := client.WorkspaceBuildLogsAfter(ctx, ws.LatestBuild.ID, 0)
		if err != nil {
			return xerrors.Errorf("fetch provisioner job logs: %w", err)
		}
		defer closer.Close()
		var logs []codersdk.ProvisionerJobLog
		for log := range buildLogCh {
			logs = append(w.BuildLogs, log)
		}
		w.BuildLogs = logs
		return nil
	})

	eg.Go(func() error {
		if w.Workspace.TemplateActiveVersionID == uuid.Nil {
			return xerrors.Errorf("workspace has nil template active version id")
		}
		tv, err := client.TemplateVersion(ctx, w.Workspace.TemplateActiveVersionID)
		if err != nil {
			return xerrors.Errorf("fetch template active version id")
		}
		w.TemplateVersion = tv

		if tv.Job.FileID == uuid.Nil {
			return xerrors.Errorf("template file id is nil")
		}
		raw, ctype, err := client.DownloadWithFormat(ctx, tv.Job.FileID, codersdk.FormatZip)
		if err != nil {
			return err
		}
		if ctype != codersdk.ContentTypeZip {
			return xerrors.Errorf("expected content-type %s, got %s", codersdk.ContentTypeZip, ctype)
		}

		b64encoded := base64.StdEncoding.EncodeToString(raw)
		w.TemplateFileBase64 = b64encoded
		return nil
	})

	eg.Go(func() error {
		if w.Workspace.TemplateID == uuid.Nil {
			return xerrors.Errorf("workspace has nil version id")
		}
		tpl, err := client.Template(ctx, w.Workspace.TemplateID)
		if err != nil {
			return xerrors.Errorf("fetch template")
		}
		w.Template = tpl
		return nil
	})

	eg.Go(func() error {
		if ws.LatestBuild.ID == uuid.Nil {
			return xerrors.Errorf("workspace has nil latest build id")
		}
		params, err := client.WorkspaceBuildParameters(ctx, ws.LatestBuild.ID)
		if err != nil {
			return xerrors.Errorf("fetch workspace build parameters: %w", err)
		}
		w.Parameters = params
		return nil
	})

	if err := eg.Wait(); err != nil {
		log.Error(ctx, "fetch workspace information", slog.Error(err))
	}

	return w
}

func AgentInfo(ctx context.Context, client *codersdk.Client, log slog.Logger, agentID uuid.UUID) Agent {
	var (
		a  Agent
		eg errgroup.Group
	)

	if agentID == uuid.Nil {
		log.Error(ctx, "no agent id specified")
		return a
	}

	eg.Go(func() error {
		agt, err := client.WorkspaceAgent(ctx, agentID)
		if err != nil {
			return xerrors.Errorf("fetch workspace agent: %w", err)
		}
		sanitizeEnv(agt.EnvironmentVariables)
		a.Agent = &agt
		return nil
	})

	eg.Go(func() error {
		agentLogCh, closer, err := client.WorkspaceAgentLogsAfter(ctx, agentID, 0, false)
		if err != nil {
			return xerrors.Errorf("fetch agent startup logs: %w", err)
		}
		defer closer.Close()
		var logs []codersdk.WorkspaceAgentLog
		for logChunk := range agentLogCh {
			logs = append(logs, logChunk...)
		}
		a.StartupLogs = logs
		return nil
	})

	// to simplify control flow, fetching information directly from
	// the agent is handled in a separate function
	closer := connectedAgentInfo(ctx, client, log, agentID, &eg, &a)
	defer closer()

	if err := eg.Wait(); err != nil {
		log.Error(ctx, "fetch agent information", slog.Error(err))
	}

	return a
}

func connectedAgentInfo(ctx context.Context, client *codersdk.Client, log slog.Logger, agentID uuid.UUID, eg *errgroup.Group, a *Agent) (closer func()) {
	conn, err := workspacesdk.New(client).
		DialAgent(ctx, agentID, &workspacesdk.DialAgentOptions{
			Logger:         log.Named("dial-agent"),
			BlockEndpoints: false,
		})

	closer = func() {}

	if err != nil {
		log.Error(ctx, "dial agent", slog.Error(err))
		return closer
	}

	if !conn.AwaitReachable(ctx) {
		log.Error(ctx, "timed out waiting for agent")
		return closer
	}

	closer = func() {
		if err := conn.Close(); err != nil {
			log.Error(ctx, "failed to close agent connection", slog.Error(err))
		}
		<-conn.Closed()
	}

	eg.Go(func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/", nil)
		if err != nil {
			return xerrors.Errorf("create request: %w", err)
		}
		rr := httptest.NewRecorder()
		conn.MagicsockServeHTTPDebug(rr, req)
		a.ClientMagicsockHTML = rr.Body.Bytes()
		return nil
	})

	eg.Go(func() error {
		promRes, err := conn.PrometheusMetrics(ctx)
		if err != nil {
			return xerrors.Errorf("fetch agent prometheus metrics: %w", err)
		}
		a.Prometheus = promRes
		return nil
	})

	eg.Go(func() error {
		_, _, pingRes, err := conn.Ping(ctx)
		if err != nil {
			return xerrors.Errorf("ping agent: %w", err)
		}
		a.PingResult = pingRes
		return nil
	})

	eg.Go(func() error {
		pds := conn.GetPeerDiagnostics()
		a.PeerDiagnostics = &pds
		return nil
	})

	eg.Go(func() error {
		msBytes, err := conn.DebugMagicsock(ctx)
		if err != nil {
			return xerrors.Errorf("get agent magicsock page: %w", err)
		}
		a.AgentMagicsockHTML = msBytes
		return nil
	})

	eg.Go(func() error {
		manifestRes, err := conn.DebugManifest(ctx)
		if err != nil {
			return xerrors.Errorf("fetch manifest: %w", err)
		}
		if err := json.NewDecoder(bytes.NewReader(manifestRes)).Decode(&a.Manifest); err != nil {
			return xerrors.Errorf("decode agent manifest: %w", err)
		}
		sanitizeEnv(a.Manifest.EnvironmentVariables)

		return nil
	})

	eg.Go(func() error {
		logBytes, err := conn.DebugLogs(ctx)
		if err != nil {
			return xerrors.Errorf("fetch coder agent logs: %w", err)
		}
		a.Logs = logBytes
		return nil
	})

	eg.Go(func() error {
		lps, err := conn.ListeningPorts(ctx)
		if err != nil {
			return xerrors.Errorf("get listening ports: %w", err)
		}
		a.ListeningPorts = &lps
		return nil
	})

	return closer
}

// Run generates a support bundle with the given dependencies.
func Run(ctx context.Context, d *Deps) (*Bundle, error) {
	var b Bundle
	if d.Client == nil {
		return nil, xerrors.Errorf("developer error: missing client!")
	}

	authChecks := map[string]codersdk.AuthorizationCheck{
		"Read DeploymentValues": {
			Object: codersdk.AuthorizationObject{
				ResourceType: codersdk.ResourceDeploymentConfig,
			},
			Action: codersdk.ActionRead,
		},
	}

	// Ensure we capture logs from the client.
	var logw strings.Builder
	d.Log = d.Log.AppendSinks(sloghuman.Sink(&logw))
	d.Client.SetLogger(d.Log)
	defer func() {
		b.Logs = strings.Split(logw.String(), "\n")
	}()

	authResp, err := d.Client.AuthCheck(ctx, codersdk.AuthorizationRequest{Checks: authChecks})
	if err != nil {
		return &b, xerrors.Errorf("check authorization: %w", err)
	}
	for k, v := range authResp {
		if !v {
			return &b, xerrors.Errorf("failed authorization check: cannot %s", k)
		}
	}

	var eg errgroup.Group
	eg.Go(func() error {
		di := DeploymentInfo(ctx, d.Client, d.Log)
		b.Deployment = di
		return nil
	})
	eg.Go(func() error {
		wi := WorkspaceInfo(ctx, d.Client, d.Log, d.WorkspaceID)
		b.Workspace = wi
		return nil
	})
	eg.Go(func() error {
		ni := NetworkInfo(ctx, d.Client, d.Log)
		b.Network = ni
		return nil
	})
	eg.Go(func() error {
		ai := AgentInfo(ctx, d.Client, d.Log, d.AgentID)
		b.Agent = ai
		return nil
	})

	_ = eg.Wait()

	return &b, nil
}

// sanitizeEnv modifies kvs in place and replaces the values all non-empty keys
// with the string ***REDACTED***
func sanitizeEnv(kvs map[string]string) {
	for k, v := range kvs {
		if v != "" {
			kvs[k] = "***REDACTED***"
		}
	}
}
