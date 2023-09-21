package agenttest

import (
	"context"
	"net/url"
	"sync"
	"testing"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// Agent is a small wrapper around an agent for use in tests.
type Agent struct {
	agent       agent.Agent
	agentClient *agentsdk.Client
	resources   []codersdk.WorkspaceResource
	waiter      func(*codersdk.Client, uuid.UUID, ...string) []codersdk.WorkspaceResource
	waitOnce    sync.Once
}

// Wait waits for the agent to connect to the workspace and returns the
// resources for the connected workspace.
// Calls coderdtest.AwaitWorkspaceAgents under the hood.
// Multiple calls to Wait() are idempotent.
func (a *Agent) Wait(client *codersdk.Client, workspaceID uuid.UUID, agentNames ...string) []codersdk.WorkspaceResource {
	a.waitOnce.Do(func() {
		a.resources = a.waiter(client, workspaceID, agentNames...)
	})
	return a.resources
}

// Client returns the agent client.
func (a *Agent) Client() *agentsdk.Client {
	return a.agentClient
}

// Agent returns the agent itself.
func (a *Agent) Agent() agent.Agent {
	return a.agent
}

// New starts a new agent for use in tests.
// The agent will use the provided coder URL and session token.
// The options passed to agent.New() can be modified by passing an optional
// variadic func(*agent.Options).
// Returns a wrapper that can be used to wait for the agent to connect to the
// workspace by calling Wait(). The arguments to Wait() are passed to
// coderdtest.AwaitWorkspaceAgents.
// Closing the agent is handled by the test cleanup.
func New(t testing.TB, coderURL *url.URL, agentToken string, opts ...func(*agent.Options)) *Agent {
	t.Helper()

	var o agent.Options
	log := slogtest.Make(t, nil).Leveled(slog.LevelDebug).Named("agent")
	o.Logger = log

	for _, opt := range opts {
		opt(&o)
	}

	if o.Client == nil {
		agentClient := agentsdk.New(coderURL)
		agentClient.SetSessionToken(agentToken)
		o.Client = agentClient
	}

	if o.ExchangeToken == nil {
		o.ExchangeToken = func(_ context.Context) (string, error) {
			return agentToken, nil
		}
	}

	if o.LogDir == "" {
		o.LogDir = t.TempDir()
	}

	agentCloser := agent.New(o)
	t.Cleanup(func() {
		assert.NoError(t, agentCloser.Close(), "failed to close agent during cleanup")
	})

	return &Agent{
		agent:       agentCloser,
		agentClient: o.Client.(*agentsdk.Client), // nolint:forcetypeassert
		waiter: func(c *codersdk.Client, workspaceID uuid.UUID, agentNames ...string) []codersdk.WorkspaceResource {
			return coderdtest.AwaitWorkspaceAgents(t, c, workspaceID, agentNames...)
		},
	}
}
