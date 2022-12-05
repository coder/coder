package vscodeipc

import (
	"context"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

// It's a lil http server that listens for messages from the VS Code extension.
// It can listen on macOS and Windows, and explicitly blocks any web requests.

type Dial struct {
	//
}

type PortForward struct {
}

type Exec struct {
}

type ExecRequest struct {
	Command string
}

type PortForwardRequest struct {
	Port string
}

type NetworkStats struct {
	P2P           bool
	Latency       float64
	PreferredDERP int
	DERPLatency   map[string]float64
}

func New(ctx context.Context, client *codersdk.Client, agentID uuid.UUID, options *codersdk.DialWorkspaceAgentOptions) (http.Handler, io.Closer, error) {
	r := chi.NewRouter()
	agentConn, err := client.DialWorkspaceAgent(ctx, agentID, &codersdk.DialWorkspaceAgentOptions{})
	if err != nil {
		return nil, nil, err
	}
	reachable := agentConn.AwaitReachable(ctx)
	if !reachable {
		return nil, nil, xerrors.New("we weren't reachable")
	}
	r.Get("/network", func(w http.ResponseWriter, r *http.Request) {
		// This returns tracked network information!
	})
	r.Get("/port/{port}", func(w http.ResponseWriter, r *http.Request) {
		// This transforms into a port forward!
	})
	r.Post("/exec", func(w http.ResponseWriter, r *http.Request) {
		// This
	})
	r.Post("/stop", func(w http.ResponseWriter, r *http.Request) {

	})
	return r, agentConn, nil
}
