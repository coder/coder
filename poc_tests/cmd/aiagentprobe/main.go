// Command aiagentprobe is the executable run from a workspace startup script
// by the proof of concept acceptance tests.
//
// It stands in for the code that will eventually tell the control plane that
// an AI agent was created. For now it only proves the plumbing: that a startup
// script can reach the workspace_agent over its local socket, and that it is
// given the identifiers a real create call would need.
//
// It reads three environment variables, all of which already exist in a real
// workspace:
//
//	CODER_AGENT_SOCKET_PATH  where the workspace_agent listens
//	CODER_WORKSPACE_ID       the workspace the AI agent would belong to
//	CODER_AGENT_TOKEN        the workspace_agent credential
//	CODER_POC_MARKER_PATH    where to record that this ran
//
// The workspace identifier and credential are not used yet. Ping takes an
// empty request. They are read and validated here so that the increment which
// adds the real call changes only the call and not the wiring.
//
// Exit status is meaningful: zero only if every step succeeded, including
// writing the marker. Any failure exits non-zero and leaves no marker.
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

	// Read but not yet used. Validated so that a missing or malformed value
	// fails here rather than in the increment that starts depending on it.
	rawWorkspaceID, err := requiredEnv("CODER_WORKSPACE_ID")
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(rawWorkspaceID)
	if err != nil {
		return xerrors.Errorf("CODER_WORKSPACE_ID is not a uuid: %w", err)
	}
	if _, err := requiredEnv("CODER_AGENT_TOKEN"); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(socketPath))
	if err != nil {
		return xerrors.Errorf("connect to agent socket %q: %w", socketPath, err)
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		return xerrors.Errorf("ping workspace_agent: %w", err)
	}

	// The marker is written last, so its presence means every preceding step
	// succeeded. The workspace identifier goes in the contents so the test can
	// tell the marker came from this run rather than a previous one.
	if err := os.WriteFile(markerPath, []byte(workspaceID.String()+"\n"), 0o600); err != nil {
		return xerrors.Errorf("write marker %q: %w", markerPath, err)
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
