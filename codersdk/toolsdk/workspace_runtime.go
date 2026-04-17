package toolsdk

import (
	"context"
	"io"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type workspaceRuntime struct {
	client *codersdk.Client
	dialer workspacesdk.AgentDialer
}

func newWorkspaceRuntime(deps Deps) workspaceRuntime {
	runtime := workspaceRuntime{
		client: deps.coderClient,
		dialer: deps.agentDialer,
	}
	if runtime.client != nil && runtime.dialer == nil {
		runtime.dialer = workspacesdk.New(runtime.client)
	}
	return runtime
}

func (r workspaceRuntime) openAgentConn(ctx context.Context, workspace string) (workspacesdk.AgentConn, error) {
	if r.client == nil {
		return nil, xerrors.New("workspace tools require an authenticated client")
	}
	if r.dialer == nil {
		return nil, xerrors.New("workspace tools require an agent dialer")
	}

	workspaceName := NormalizeWorkspaceInput(workspace)
	_, workspaceAgent, err := findWorkspaceAndAgent(ctx, r.client, workspaceName)
	if err != nil {
		return nil, xerrors.Errorf("failed to find workspace: %w", err)
	}

	if err := cliui.Agent(ctx, io.Discard, workspaceAgent.ID, cliui.AgentOptions{
		FetchInterval: 0,
		Fetch:         r.client.WorkspaceAgent,
		FetchLogs:     r.client.WorkspaceAgentLogsAfter,
		Wait:          true,
	}); err != nil {
		return nil, xerrors.Errorf("agent not ready: %w", err)
	}

	conn, err := r.dialer.DialAgent(ctx, workspaceAgent.ID, &workspacesdk.DialAgentOptions{
		BlockEndpoints: false,
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to dial agent: %w", err)
	}

	return conn, nil
}
