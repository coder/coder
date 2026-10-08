package agent

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
)

func (a *agent) lookupWorkspaceShutdown(client proto.DRPCAgentClient213) {
	if client == nil {
		return
	}
	// This is best-effort logging. Use the existing connection once, before
	// closing SSH, without delaying shutdown for an unavailable control plane.
	ctx, cancel := context.WithCancel(a.hardCtx)
	defer cancel()
	timer := a.clock.AfterFunc(time.Second, cancel, "workspace-shutdown-lookup")
	defer timer.Stop()
	shutdown, err := client.GetWorkspaceShutdown(ctx, &emptypb.Empty{})
	if err != nil {
		a.logger.Debug(ctx, "workspace shutdown reason unavailable", slog.Error(err))
		return
	}
	if shutdown.GetBuildId() == "" ||
		(shutdown.Transition != string(codersdk.WorkspaceTransitionStop) && shutdown.Transition != string(codersdk.WorkspaceTransitionDelete)) {
		return
	}
	a.shutdownCause.Store(&agentssh.ShutdownCause{
		Reason:  codersdk.DisconnectReasonWorkspaceStopped,
		BuildID: shutdown.BuildId, BuildReason: shutdown.BuildReason, Transition: shutdown.Transition,
	})
}

func (a *agent) sshShutdownCause() agentssh.ShutdownCause {
	if cause := a.shutdownCause.Load(); cause != nil {
		return *cause
	}
	return agentssh.ShutdownCause{Reason: codersdk.DisconnectReasonServerShutdown}
}
