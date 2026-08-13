// Command aiagentprobe is the executable run from a workspace startup script
// by the proof of concept acceptance tests.
//
// It stands in for the code that tells the control plane an AI agent was
// created. It calls CreateAIAgent on the workspace_agent's local socket, which
// forwards to coderd, which mints an identity and journals the creation.
//
// It reads three environment variables, all of which exist in a real
// workspace:
//
//	CODER_AGENT_SOCKET_PATH  where the workspace_agent listens
//	CODER_WORKSPACE_ID       the workspace the AI agent belongs to
//	CODER_AGENT_TOKEN        the credential issued to the workspace
//
// The workspace identifier and the credential are sent over the socket, which
// authenticates nobody. They travel no further: coderd resolves both from the
// connection the workspace_agent forwards over. The credential is read as
// bytes and passed on untouched.
//
// The minted identifier is printed to stdout, which the workspace_agent
// captures into the startup script's log and ships to the control plane. The
// credential it receives is never printed. A digest of it is, so that a test
// can show it arrived intact without the value being stored anywhere.
//
// Exit status is meaningful: zero only if every step succeeded, including the
// socket call. Any failure exits non-zero.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	rawWorkspaceID, err := requiredEnv("CODER_WORKSPACE_ID")
	if err != nil {
		return err
	}
	workspaceID, err := uuid.Parse(rawWorkspaceID)
	if err != nil {
		return xerrors.Errorf("CODER_WORKSPACE_ID is not a uuid: %w", err)
	}
	workspaceCredential, err := requiredEnv("CODER_AGENT_TOKEN")
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

	id, credential, err := client.CreateAIAgent(ctx, workspaceID, []byte(workspaceCredential))
	if err != nil {
		return xerrors.Errorf("create AI agent: %w", err)
	}

	// Standard output becomes a startup script log, which the workspace_agent
	// ships to the control plane and which is stored there. So the identifier
	// is printed and the credential never is. What goes out instead is a digest
	// of it, which is enough for a test to show the credential arrived intact
	// and useless to anyone who reads the log.
	digest := sha256.Sum256(credential)
	_, _ = fmt.Printf("created AI agent %s\n", id)
	_, _ = fmt.Printf("credential sha256 %s\n", hex.EncodeToString(digest[:]))

	return nil
}

func requiredEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", xerrors.Errorf("%s is not set", name)
	}
	return value, nil
}
