// Command aiagentprobe is the executable run from a workspace startup script
// by the proof of concept acceptance tests.
//
// It stands in for the code that will eventually tell the control plane that
// an AI agent was created. It calls CreateAIAgent on the workspace_agent's
// local socket, which is the first hop of that eventual path. The rest of the
// path does not exist yet, so the handler it reaches is a stub.
//
// It reads four environment variables. The first three already exist in a real
// workspace:
//
//	CODER_AGENT_SOCKET_PATH  where the workspace_agent listens
//	CODER_WORKSPACE_ID       the workspace the AI agent belongs to
//	CODER_AGENT_TOKEN        the workspace_agent credential
//	CODER_POC_MARKER_PATH    where the handler records that it was called
//
// The workspace identifier and the credential are sent with the call. Neither
// is used for anything yet, since the handler is a stub, but the data path
// they travel is the one the real call will use.
//
// Exit status is meaningful: zero only if every step succeeded, including the
// socket call. Any failure exits non-zero.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
)

const timeout = 30 * time.Second

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "aiagentprobe: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	socketPath, err := requiredEnv("CODER_AGENT_SOCKET_PATH")
	if err != nil {
		return err
	}
	markerPath, err := requiredEnv("CODER_POC_MARKER_PATH")
	if err != nil {
		return err
	}

	rawWorkspaceID, err := requiredEnv("CODER_WORKSPACE_ID")
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(rawWorkspaceID)
	if err != nil {
		return xerrors.Errorf("CODER_WORKSPACE_ID is not a uuid: %w", err)
	}
	agentToken, err := requiredEnv("CODER_AGENT_TOKEN")
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(socketPath))
	if err != nil {
		return xerrors.Errorf("connect to agent socket %q: %w", socketPath, err)
	}
	defer client.Close()

	// The marker is not written here. The handler on the other side of the
	// socket writes it, so its presence proves the call arrived rather than
	// merely that this executable ran. The path travels in the request only
	// because that handler is still a stub.
	if err := client.CreateAIAgent(ctx, workspaceID, []byte(agentToken), markerPath); err != nil {
		return xerrors.Errorf("create AI agent: %w", err)
	}

	return nil
}

func requiredEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", xerrors.Errorf("%s is not set", name)
	}
	return value, nil
}
