package support

import (
	"context"
	"io"
	"net/http"
	"strings"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// Bundle is a set of information discovered about a deployment.
// Even though we do attempt to sanitize data, it may still contain
// sensitive information and should thus be treated as secret.
type Bundle struct {
	Deployment Deployment `json:"deployment"`
	Network    Network    `json:"network"`
	Workspace  Workspace  `json:"workspace"`
	Logs       []string   `json:"logs"`
}

type Deployment struct {
	BuildInfo    *codersdk.BuildInfoResponse `json:"build"`
	Config       *codersdk.DeploymentConfig  `json:"config"`
	Experiments  codersdk.Experiments        `json:"experiments"`
	HealthReport *codersdk.HealthcheckReport `json:"health_report"`
}

type Network struct {
	CoordinatorDebug string                                 `json:"coordinator_debug"`
	TailnetDebug     string                                 `json:"tailnet_debug"`
	NetcheckLocal    *codersdk.WorkspaceAgentConnectionInfo `json:"netcheck_local"`
	NetcheckRemote   *codersdk.WorkspaceAgentConnectionInfo `json:"netcheck_remote"`
}

type Workspace struct {
	Workspace        codersdk.Workspace           `json:"workspace"`
	BuildLogs        []codersdk.ProvisionerJobLog `json:"build_logs"`
	Agent            codersdk.WorkspaceAgent      `json:"agent"`
	AgentStartupLogs []codersdk.WorkspaceAgentLog `json:"startup_logs"`
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
		hr, err := client.DebugHealth(ctx)
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

func NetworkInfo(ctx context.Context, client *codersdk.Client, log slog.Logger, agentID uuid.UUID) Network {
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
		if agentID == uuid.Nil {
			log.Warn(ctx, "agent id required for agent connection info")
			return nil
		}
		connInfo, err := client.WorkspaceAgentConnectionInfo(ctx, agentID)
		if err != nil {
			return xerrors.Errorf("fetch agent conn info: %w", err)
		}
		n.NetcheckLocal = &connInfo
		return nil
	})

	if err := eg.Wait(); err != nil {
		log.Error(ctx, "fetch network information", slog.Error(err))
	}

	return n
}

func WorkspaceInfo(ctx context.Context, client *codersdk.Client, log slog.Logger, workspaceID, agentID uuid.UUID) Workspace {
	var (
		w  Workspace
		eg errgroup.Group
	)

	if workspaceID == uuid.Nil {
		log.Error(ctx, "no workspace id specified")
		return w
	}

	if agentID == uuid.Nil {
		log.Error(ctx, "no agent id specified")
	}

	// dependency, cannot fetch concurrently
	ws, err := client.Workspace(ctx, workspaceID)
	if err != nil {
		log.Error(ctx, "fetch workspace", slog.Error(err), slog.F("workspace_id", workspaceID))
		return w
	}
	w.Workspace = ws

	eg.Go(func() error {
		agt, err := client.WorkspaceAgent(ctx, agentID)
		if err != nil {
			return xerrors.Errorf("fetch workspace agent: %w", err)
		}
		w.Agent = agt
		return nil
	})

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
		if len(w.Workspace.LatestBuild.Resources) == 0 {
			log.Warn(ctx, "workspace build has no resources")
			return nil
		}
		agentLogCh, closer, err := client.WorkspaceAgentLogsAfter(ctx, agentID, 0, false)
		if err != nil {
			return xerrors.Errorf("fetch agent startup logs: %w", err)
		}
		defer closer.Close()
		var logs []codersdk.WorkspaceAgentLog
		for logChunk := range agentLogCh {
			logs = append(w.AgentStartupLogs, logChunk...)
		}
		w.AgentStartupLogs = logs
		return nil
	})

	if err := eg.Wait(); err != nil {
		log.Error(ctx, "fetch workspace information", slog.Error(err))
	}

	return w
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
				ResourceType: codersdk.ResourceDeploymentValues,
			},
			Action: string(rbac.ActionRead),
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
		wi := WorkspaceInfo(ctx, d.Client, d.Log, d.WorkspaceID, d.AgentID)
		b.Workspace = wi
		return nil
	})
	eg.Go(func() error {
		ni := NetworkInfo(ctx, d.Client, d.Log, d.AgentID)
		b.Network = ni
		return nil
	})

	_ = eg.Wait()

	return &b, nil
}
