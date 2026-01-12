package support

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/netcheck"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
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
	Deployment    Deployment   `json:"deployment"`
	Network       Network      `json:"network"`
	Workspace     Workspace    `json:"workspace"`
	Agent         Agent        `json:"agent"`
	Logs          []string     `json:"logs"`
	CLILogs       []byte       `json:"cli_logs"`
	NamedTemplate TemplateDump `json:"named_template"`
	Pprof         Pprof        `json:"pprof"`
}

type Deployment struct {
	BuildInfo      *codersdk.BuildInfoResponse  `json:"build"`
	Config         *codersdk.DeploymentConfig   `json:"config"`
	Experiments    codersdk.Experiments         `json:"experiments"`
	HealthReport   *healthsdk.HealthcheckReport `json:"health_report"`
	Licenses       []codersdk.License           `json:"licenses"`
	Stats          *codersdk.DeploymentStats    `json:"stats"`
	Entitlements   *codersdk.Entitlements       `json:"entitlements"`
	HealthSettings *healthsdk.HealthSettings    `json:"health_settings"`
	Workspaces     *codersdk.WorkspacesResponse `json:"workspaces"`
	Prometheus     []byte                       `json:"prometheus"`
}

type Network struct {
	ConnectionInfo   workspacesdk.AgentConnectionInfo
	CoordinatorDebug string                     `json:"coordinator_debug"`
	Netcheck         *derphealth.Report         `json:"netcheck"`
	TailnetDebug     string                     `json:"tailnet_debug"`
	Interfaces       healthsdk.InterfacesReport `json:"interfaces"`
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

type TemplateDump struct {
	Template           codersdk.Template        `json:"template"`
	TemplateVersion    codersdk.TemplateVersion `json:"template_version"`
	TemplateFileBase64 string                   `json:"template_file_base64"`
}

type Pprof struct {
	Server *PprofCollection `json:"server,omitempty"`
	Agent  *PprofCollection `json:"agent,omitempty"`
}

type PprofCollection struct {
	Heap         []byte    `json:"heap,omitempty"`
	Allocs       []byte    `json:"allocs,omitempty"`
	Profile      []byte    `json:"profile,omitempty"`
	Block        []byte    `json:"block,omitempty"`
	Mutex        []byte    `json:"mutex,omitempty"`
	Goroutine    []byte    `json:"goroutine,omitempty"`
	Threadcreate []byte    `json:"threadcreate,omitempty"`
	Trace        []byte    `json:"trace,omitempty"`
	Cmdline      string    `json:"cmdline,omitempty"`
	Symbol       string    `json:"symbol,omitempty"`
	CollectedAt  time.Time `json:"collected_at"`
	EndpointURL  string    `json:"endpoint_url"`
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
	// WorkspacesTotalCap limits the TOTAL number of workspaces aggregated into the bundle.
	// > 0  => cap at this number (default flag value should be 1000 via CLI).
	// <= 0 => no cap (fetch/keep all available workspaces).
	WorkspacesTotalCap int
	// TemplateID optionally specifies a template to capture (active version).
	TemplateID uuid.UUID
	// CollectPprof toggles server and agent pprof collection.
	CollectPprof bool
}

// ctxKeyWorkspacesCap is used to plumb the workspace cap into DeploymentInfo
// without changing its signature. This follows Go's context value pattern
// for request-scoped configuration.
type ctxKeyWorkspacesCap struct{}

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

	eg.Go(func() error {
		licenses, err := client.Licenses(ctx)
		if err != nil {
			// Ignore 404 because AGPL doesn't have this endpoint
			if cerr, ok := codersdk.AsError(err); ok && cerr.StatusCode() != http.StatusNotFound {
				return xerrors.Errorf("fetch license status: %w", err)
			}
		}
		if licenses == nil {
			licenses = make([]codersdk.License, 0)
		}
		d.Licenses = licenses
		return nil
	})

	// Deployment stats
	eg.Go(func() error {
		stats, err := client.DeploymentStats(ctx)
		if err != nil {
			// If unauthorized or forbidden, log and continue
			if cerr, ok := codersdk.AsError(err); ok && (cerr.StatusCode() == http.StatusForbidden || cerr.StatusCode() == http.StatusUnauthorized || cerr.StatusCode() == http.StatusBadRequest) {
				log.Warn(ctx, "unable to fetch deployment stats")
				return nil
			}
			return xerrors.Errorf("fetch deployment stats: %w", err)
		}
		d.Stats = &stats
		return nil
	})

	// Entitlements
	eg.Go(func() error {
		ents, err := client.Entitlements(ctx)
		if err != nil {
			// Ignore 404 or enterprise-not-enabled
			if cerr, ok := codersdk.AsError(err); ok && (cerr.StatusCode() == http.StatusNotFound || cerr.StatusCode() == http.StatusForbidden) {
				log.Warn(ctx, "unable to fetch entitlements")
				return nil
			}
			return xerrors.Errorf("fetch entitlements: %w", err)
		}
		d.Entitlements = &ents
		return nil
	})

	// Health settings
	eg.Go(func() error {
		settings, err := healthsdk.New(client).HealthSettings(ctx)
		if err != nil {
			// If not accessible, log and continue
			if cerr, ok := codersdk.AsError(err); ok && (cerr.StatusCode() == http.StatusForbidden || cerr.StatusCode() == http.StatusUnauthorized) {
				log.Warn(ctx, "unable to fetch health settings")
				return nil
			}
			return xerrors.Errorf("fetch health settings: %w", err)
		}
		d.HealthSettings = &settings
		return nil
	})

	// List workspaces (paginated)
	eg.Go(func() error {
		var (
			offset int
			limit  = 200
			all    []codersdk.Workspace
			count  int
		)
		// Early-exit cap (plumbed via context from Run; <=0 means no cap).
		capTotal := 0
		if v, ok := ctx.Value(ctxKeyWorkspacesCap{}).(int); ok {
			capTotal = v
		}
		for {
			resp, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{Offset: offset, Limit: limit})
			if err != nil {
				// Log and continue if forbidden; otherwise return error
				if cerr, ok := codersdk.AsError(err); ok && (cerr.StatusCode() == http.StatusForbidden || cerr.StatusCode() == http.StatusUnauthorized) {
					log.Warn(ctx, "unable to list workspaces")
					break
				}
				return xerrors.Errorf("list workspaces: %w", err)
			}
			if d.Workspaces == nil {
				d.Workspaces = &resp
			}
			// sanitize env vars on agents in each workspace before appending
			for i := range resp.Workspaces {
				ws := &resp.Workspaces[i]
				for _, res := range ws.LatestBuild.Resources {
					for _, agt := range res.Agents {
						// safe to call even if map is nil (range in sanitizeEnv would be empty)
						sanitizeEnv(agt.EnvironmentVariables)
					}
				}
			}
			all = append(all, resp.Workspaces...)
			count = resp.Count
			// Stop early once we've reached the cap; trim any overflow from the last page.
			if capTotal > 0 && len(all) >= capTotal {
				if len(all) > capTotal {
					all = all[:capTotal]
				}
				break
			}
			if offset+len(resp.Workspaces) >= count || len(resp.Workspaces) == 0 {
				break
			}
			offset += len(resp.Workspaces)
		}
		if d.Workspaces != nil {
			// Replace with aggregated list
			d.Workspaces.Workspaces = all
			// Preserve server-reported total so Run() can log accurate truncation.
			d.Workspaces.Count = count
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		log.Error(ctx, "fetch deployment information", slog.Error(err))
	}

	if d.Config != nil && d.Config.Values != nil {
		prometheusCfg := d.Config.Values.Prometheus
		if prometheusCfg.Enable.Value() {
			metrics, err := fetchPrometheusMetrics(ctx, client, log)
			if err != nil {
				log.Warn(ctx, "fetch coderd prometheus metrics", slog.Error(err))
			} else {
				d.Prometheus = metrics
			}
		}
	}

	return d
}

func fetchPrometheusMetrics(ctx context.Context, client *codersdk.Client, log slog.Logger) ([]byte, error) {
	if client == nil {
		return nil, xerrors.New("nil client")
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.Request(reqCtx, http.MethodGet, "/api/v2/debug/metrics", nil)
	if err != nil {
		return nil, xerrors.Errorf("request metrics: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, xerrors.Errorf("read metrics body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Debug(ctx, "coderd prometheus metrics fetch non-200",
			slog.F("status", resp.StatusCode), slog.F("body_len", len(body)))
		return nil, xerrors.Errorf("unexpected status code %d", resp.StatusCode)
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, xerrors.New("empty prometheus metrics response")
	}
	return append([]byte(nil), trimmed...), nil
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

	eg.Go(func() error {
		rpt, err := healthsdk.RunInterfacesReport()
		if err != nil {
			return xerrors.Errorf("run interfaces report: %w", err)
		}
		n.Interfaces = rpt
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
		for log := range buildLogCh {
			w.BuildLogs = append(w.BuildLogs, log)
		}
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
		<-conn.TailnetConn().Closed()
	}

	eg.Go(func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/", nil)
		if err != nil {
			return xerrors.Errorf("create request: %w", err)
		}
		rr := httptest.NewRecorder()
		conn.TailnetConn().MagicsockServeHTTPDebug(rr, req)
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

func PprofInfo(ctx context.Context, client *codersdk.Client, log slog.Logger) *PprofCollection {
	if client == nil {
		return nil
	}

	var (
		p  PprofCollection
		eg errgroup.Group
	)

	if client.URL != nil {
		if u, err := client.URL.Parse("/api/v2/debug/pprof"); err == nil {
			p.EndpointURL = u.String()
		}
	}
	if p.EndpointURL == "" {
		p.EndpointURL = "/api/v2/debug/pprof"
	}
	p.CollectedAt = time.Now()

	const basePath = "/api/v2/debug/pprof"
	endpoints := map[string]func([]byte){
		"/allocs": func(data []byte) {
			p.Allocs = compressData(data)
		},
		"/heap": func(data []byte) {
			p.Heap = compressData(data)
		},
		"/profile?seconds=30": func(data []byte) {
			p.Profile = compressData(data)
		},
		"/block": func(data []byte) {
			p.Block = compressData(data)
		},
		"/mutex": func(data []byte) {
			p.Mutex = compressData(data)
		},
		"/goroutine": func(data []byte) {
			p.Goroutine = compressData(data)
		},
		"/threadcreate": func(data []byte) {
			p.Threadcreate = compressData(data)
		},
		"/trace?seconds=30": func(data []byte) {
			p.Trace = compressData(data)
		},
		"/cmdline": func(data []byte) {
			p.Cmdline = string(data)
		},
		"/symbol": func(data []byte) {
			p.Symbol = string(data)
		},
	}

	for endpoint, setter := range endpoints {
		endpoint, setter := endpoint, setter
		eg.Go(func() error {
			timeout := 10 * time.Second
			if strings.Contains(endpoint, "seconds=30") {
				timeout = 45 * time.Second
			}

			reqCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			resp, err := client.Request(reqCtx, http.MethodGet, basePath+endpoint, nil)
			if err != nil {
				log.Warn(reqCtx, "failed to fetch pprof data", slog.F("endpoint", endpoint), slog.Error(err))
				return nil
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Warn(reqCtx, "pprof endpoint returned non-200 status",
					slog.F("endpoint", endpoint), slog.F("status", resp.StatusCode))
				return nil
			}

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Warn(reqCtx, "failed to read pprof response", slog.F("endpoint", endpoint), slog.Error(err))
				return nil
			}

			setter(data)
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		log.Error(ctx, "failed to collect some pprof data", slog.Error(err))
	}

	return &p
}

func compressData(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return data // Return uncompressed if compression fails
	}
	if err := gz.Close(); err != nil {
		return data
	}

	return buf.Bytes()
}

func PprofInfoFromAgent(ctx context.Context, conn workspacesdk.AgentConn, log slog.Logger) *PprofCollection {
	if conn == nil {
		return nil
	}

	var (
		p  PprofCollection
		eg errgroup.Group
	)

	p.EndpointURL = "agent"
	p.CollectedAt = time.Now()

	// Define agent pprof endpoints - these go through the agent connection
	endpoints := map[string]func([]byte){
		"/debug/pprof/allocs": func(data []byte) {
			p.Allocs = compressData(data)
		},
		"/debug/pprof/heap": func(data []byte) {
			p.Heap = compressData(data)
		},
		"/debug/pprof/profile?seconds=30": func(data []byte) {
			p.Profile = compressData(data)
		},
		"/debug/pprof/block": func(data []byte) {
			p.Block = compressData(data)
		},
		"/debug/pprof/mutex": func(data []byte) {
			p.Mutex = compressData(data)
		},
		"/debug/pprof/goroutine": func(data []byte) {
			p.Goroutine = compressData(data)
		},
		"/debug/pprof/threadcreate": func(data []byte) {
			p.Threadcreate = compressData(data)
		},
		"/debug/pprof/trace?seconds=30": func(data []byte) {
			p.Trace = compressData(data)
		},
		"/debug/pprof/cmdline": func(data []byte) {
			p.Cmdline = string(data)
		},
		"/debug/pprof/symbol": func(data []byte) {
			p.Symbol = string(data)
		},
	}

	// Collect each endpoint in parallel
	for endpoint, setter := range endpoints {
		endpoint, setter := endpoint, setter // capture loop variables
		eg.Go(func() error {
			// Set longer timeout for profile and trace endpoints (they take 30 seconds)
			timeout := 10 * time.Second
			if strings.Contains(endpoint, "seconds=30") {
				timeout = 45 * time.Second
			}

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Use the agent's direct HTTP capability
			// Agent pprof server runs on 127.0.0.1:6060 by default
			netConn, err := conn.DialContext(ctx, "tcp", "127.0.0.1:6060")
			if err != nil {
				log.Warn(ctx, "failed to dial agent pprof endpoint", slog.F("endpoint", endpoint), slog.Error(err))
				return nil
			}
			defer netConn.Close()

			// Create HTTP client using the connection
			client := &http.Client{
				Transport: &http.Transport{
					DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
						return netConn, nil
					},
				},
				Timeout: timeout,
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:6060"+endpoint, nil)
			if err != nil {
				log.Warn(ctx, "failed to create agent pprof request", slog.F("endpoint", endpoint), slog.Error(err))
				return nil
			}

			resp, err := client.Do(req)
			if err != nil {
				log.Warn(ctx, "failed to fetch agent pprof data", slog.F("endpoint", endpoint), slog.Error(err))
				return nil
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Warn(ctx, "agent pprof endpoint returned non-200 status", slog.F("endpoint", endpoint), slog.F("status", resp.StatusCode))
				return nil
			}

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Warn(ctx, "failed to read agent pprof response", slog.F("endpoint", endpoint), slog.Error(err))
				return nil
			}

			setter(data)
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		log.Error(ctx, "failed to collect some agent pprof data", slog.Error(err))
	}

	return &p
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

	totalCap := d.WorkspacesTotalCap
	// Make the cap available to DeploymentInfo without changing its signature.
	ctx = context.WithValue(ctx, ctxKeyWorkspacesCap{}, totalCap)

	var eg errgroup.Group
	eg.Go(func() error {
		di := DeploymentInfo(ctx, d.Client, d.Log)

		if di.Workspaces != nil && totalCap > 0 {
			origTotal := di.Workspaces.Count // server-reported total

			// Ensure at most 'totalCap' are returned (covers non-early-exit path).
			if len(di.Workspaces.Workspaces) > totalCap {
				di.Workspaces.Workspaces = di.Workspaces.Workspaces[:totalCap]
			}
			// If we returned fewer than the original total, log a truncation.
			if origTotal > len(di.Workspaces.Workspaces) {
				di.Workspaces.Count = len(di.Workspaces.Workspaces)
				d.Log.Warn(ctx, "workspace list truncated",
					slog.F("cap", totalCap),
					slog.F("original_total", origTotal),
				)
			}
		}
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

	// Optional: capture a template's active version and file if TemplateID is set.
	eg.Go(func() error {
		if d.TemplateID == uuid.Nil {
			return nil
		}
		var td TemplateDump
		tpl, err := d.Client.Template(ctx, d.TemplateID)
		if err != nil {
			d.Log.Error(ctx, "fetch template", slog.Error(err), slog.F("template_id", d.TemplateID))
			return nil
		}
		td.Template = tpl
		if tpl.ActiveVersionID == uuid.Nil {
			d.Log.Error(ctx, "template has nil active version id", slog.F("template_id", tpl.ID))
			b.NamedTemplate = td
			return nil
		}
		tv, err := d.Client.TemplateVersion(ctx, tpl.ActiveVersionID)
		if err != nil {
			d.Log.Error(ctx, "fetch active template version", slog.Error(err), slog.F("active_version_id", tpl.ActiveVersionID))
			b.NamedTemplate = td
			return nil
		}
		td.TemplateVersion = tv
		if tv.Job.FileID == uuid.Nil {
			d.Log.Error(ctx, "template file id is nil", slog.F("template_version_id", tv.ID))
			b.NamedTemplate = td
			return nil
		}
		raw, ctype, err := d.Client.DownloadWithFormat(ctx, tv.Job.FileID, codersdk.FormatZip)
		if err != nil || ctype != codersdk.ContentTypeZip {
			d.Log.Error(ctx, "download template file", slog.Error(err), slog.F("content_type", ctype))
			b.NamedTemplate = td
			return nil
		}
		td.TemplateFileBase64 = base64.StdEncoding.EncodeToString(raw)
		b.NamedTemplate = td
		return nil
	})

	_ = eg.Wait()

	// Collect pprof data after deployment info is available (need version check).
	// Pprof endpoints require Coder server version 2.28.0 or newer.
	if d.CollectPprof {
		b.Pprof = collectPprof(ctx, d, &b)
	}

	return &b, nil
}

// minPprofVersion is the minimum Coder server version that supports
// the /api/v2/debug/pprof endpoints.
const minPprofVersion = "v2.28.0"

// VersionSupportsPprof checks if the given version supports pprof endpoints.
func VersionSupportsPprof(version string) bool {
	if version == "" {
		return false
	}
	if version[0] != 'v' {
		version = "v" + version
	}
	// For prerelease versions like "v2.28.0-devel+abc123", we compare
	// the major.minor.patch portion since prereleases of 2.28.0 should
	// have the pprof feature.
	canonical := semver.Canonical(version)
	if idx := strings.Index(canonical, "-"); idx != -1 {
		canonical = canonical[:idx]
	}
	return semver.Compare(canonical, minPprofVersion) >= 0
}

func collectPprof(ctx context.Context, d *Deps, b *Bundle) Pprof {
	var pprof Pprof

	// Check server version before attempting pprof collection.
	if b.Deployment.BuildInfo == nil {
		d.Log.Warn(ctx, "skipping pprof collection: build info not available")
		return pprof
	}
	if !VersionSupportsPprof(b.Deployment.BuildInfo.Version) {
		d.Log.Warn(ctx, "skipping pprof collection: server version too old",
			slog.F("version", b.Deployment.BuildInfo.Version),
			slog.F("min_version", minPprofVersion))
		return pprof
	}

	serverPprof := PprofInfo(ctx, d.Client, d.Log)
	if serverPprof != nil {
		pprof.Server = serverPprof
	}

	if d.AgentID != uuid.Nil {
		conn, err := workspacesdk.New(d.Client).
			DialAgent(ctx, d.AgentID, &workspacesdk.DialAgentOptions{
				Logger:         d.Log.Named("dial-agent-pprof"),
				BlockEndpoints: false,
			})
		if err != nil {
			d.Log.Warn(ctx, "failed to dial agent for pprof collection", slog.Error(err))
		} else {
			defer func() {
				if err := conn.Close(); err != nil {
					d.Log.Error(ctx, "failed to close agent pprof connection", slog.Error(err))
				}
				<-conn.TailnetConn().Closed()
			}()

			if conn.AwaitReachable(ctx) {
				agentPprof := PprofInfoFromAgent(ctx, conn, d.Log)
				if agentPprof != nil {
					pprof.Agent = agentPprof
				}
			} else {
				d.Log.Warn(ctx, "agent not reachable for pprof collection")
			}
		}
	}

	return pprof
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
