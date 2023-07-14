package workspacetraffic

import (
	"context"
	"github.com/coder/coder/codersdk"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

func connectPTY(ctx context.Context, client *codersdk.Client, agentID, reconnect uuid.UUID) (*countReadWriteCloser, error) {
	conn, err := client.WorkspaceAgentReconnectingPTY(ctx, codersdk.WorkspaceAgentReconnectingPTYOpts{
		AgentID:   agentID,
		Reconnect: reconnect,
		Height:    25,
		Width:     80,
		Command:   "/bin/sh",
	})
	if err != nil {
		return nil, xerrors.Errorf("connect pty: %w", err)
	}

	//Wrap the conn in a countReadWriteCloser so we can monitor bytes sent/rcvd.
	crw := countReadWriteCloser{ctx: ctx, rwc: conn}
	return &crw, nil
}
