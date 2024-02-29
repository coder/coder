package support

import (
	"context"
	"io"
	"net/http"
	"strings"

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
	var d Deployment

	bi, err := client.BuildInfo(ctx)
	if err != nil {
		log.Error(ctx, "fetch build info", slog.Error(err))
	} else {
		d.BuildInfo = &bi
	}

	dc, err := client.DeploymentConfig(ctx)
	if err != nil {
		log.Error(ctx, "fetch deployment config", slog.Error(err))
	} else {
		d.Config = dc
	}

	hr, err := client.DebugHealth(ctx)
	if err != nil {
		log.Error(ctx, "fetch health report", slog.Error(err))
	} else {
		d.HealthReport = &hr
	}

	exp, err := client.Experiments(ctx)
	if err != nil {
		log.Error(ctx, "fetch experiments", slog.Error(err))
	} else {
		d.Experiments = exp
	}

	return d
}

func NetworkInfo(ctx context.Context, client *codersdk.Client, log slog.Logger, agentID uuid.UUID) Network {
	var n Network

	coordResp, err := client.Request(ctx, http.MethodGet, "/api/v2/debug/coordinator", nil)
	if err != nil {
		log.Error(ctx, "fetch coordinator debug page", slog.Error(err))
	} else {
		defer coordResp.Body.Close()
		bs, err := io.ReadAll(coordResp.Body)
		if err != nil {
			log.Error(ctx, "read coordinator debug page", slog.Error(err))
		} else {
			n.CoordinatorDebug = string(bs)
		}
	}

	tailResp, err := client.Request(ctx, http.MethodGet, "/api/v2/debug/tailnet", nil)
	if err != nil {
		log.Error(ctx, "fetch tailnet debug page", slog.Error(err))
	} else {
		defer tailResp.Body.Close()
		bs, err := io.ReadAll(tailResp.Body)
		if err != nil {
			log.Error(ctx, "read tailnet debug page", slog.Error(err))
		} else {
			n.TailnetDebug = string(bs)
		}
	}

	if agentID != uuid.Nil {
		connInfo, err := client.WorkspaceAgentConnectionInfo(ctx, agentID)
		if err != nil {
			log.Error(ctx, "fetch agent conn info", slog.Error(err), slog.F("agent_id", agentID.String()))
		} else {
			n.NetcheckLocal = &connInfo
		}
	} else {
		log.Warn(ctx, "agent id required for agent connection info")
	}

	return n
}

func WorkspaceInfo(ctx context.Context, client *codersdk.Client, log slog.Logger, workspaceID, agentID uuid.UUID) Workspace {
	var w Workspace

	if workspaceID == uuid.Nil {
		log.Error(ctx, "no workspace id specified")
		return w
	}

	if agentID == uuid.Nil {
		log.Error(ctx, "no agent id specified")
	}

	ws, err := client.Workspace(ctx, workspaceID)
	if err != nil {
		log.Error(ctx, "fetch workspace", slog.Error(err), slog.F("workspace_id", workspaceID))
		return w
	}

	w.Workspace = ws

	buildLogCh, closer, err := client.WorkspaceBuildLogsAfter(ctx, ws.LatestBuild.ID, 0)
	if err != nil {
		log.Error(ctx, "fetch provisioner job logs", slog.Error(err), slog.F("job_id", ws.LatestBuild.Job.ID.String()))
	} else {
		defer closer.Close()
		for log := range buildLogCh {
			w.BuildLogs = append(w.BuildLogs, log)
		}
	}

	if len(w.Workspace.LatestBuild.Resources) == 0 {
		log.Warn(ctx, "workspace build has no resources")
		return w
	}

	agentLogCh, closer, err := client.WorkspaceAgentLogsAfter(ctx, agentID, 0, false)
	if err != nil {
		log.Error(ctx, "fetch agent startup logs", slog.Error(err), slog.F("agent_id", agentID.String()))
	} else {
		defer closer.Close()
		for logChunk := range agentLogCh {
			w.AgentStartupLogs = append(w.AgentStartupLogs, logChunk...)
		}
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
	d.Log.AppendSinks(sloghuman.Sink(&logw))
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

	b.Deployment = DeploymentInfo(ctx, d.Client, d.Log)
	b.Workspace = WorkspaceInfo(ctx, d.Client, d.Log, d.WorkspaceID, d.AgentID)
	b.Network = NetworkInfo(ctx, d.Client, d.Log, d.AgentID)

	return &b, nil
}
