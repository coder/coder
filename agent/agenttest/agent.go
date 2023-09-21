package agenttest

import (
	"context"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// Options are options for creating a new test agent.
type Options struct {
	// AgentOptions are the options to use for the agent.
	AgentOptions agent.Options

	// AgentToken is the token to use for the agent.
	AgentToken string
	// URL is the URL to which the agent should connect.
	URL *url.URL
	// WorkspaceID is the ID of the workspace to which the agent should connect.
	WorkspaceID uuid.UUID
	// Logger is the logger to use for the agent.
	// Defaults to a new test logger if not specified.
	Logger *slog.Logger
}

// Agent is a small wrapper around an agent for use in tests.
type Agent struct {
	waitOnce    sync.Once
	agent       agent.Agent
	agentClient *agentsdk.Client
	resources   []codersdk.WorkspaceResource
	waiter      func(*codersdk.Client) []codersdk.WorkspaceResource
}

// Wait waits for the agent to connect to the workspace and returns the
// resources for the connected workspace.
func (a *Agent) Wait(client *codersdk.Client) []codersdk.WorkspaceResource {
	a.waitOnce.Do(func() {
		a.resources = a.waiter(client)
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

// OptFunc is a function that modifies the given options.
type OptFunc func(*Options)

func WithAgentToken(token string) OptFunc {
	return func(o *Options) {
		o.AgentToken = token
	}
}

func WithURL(u *url.URL) OptFunc {
	return func(o *Options) {
		o.URL = u
	}
}

func WithWorkspaceID(id uuid.UUID) OptFunc {
	return func(o *Options) {
		o.WorkspaceID = id
	}
}

// New starts a new agent for use in tests.
// Returns a wrapper around the agent that can be used to wait for the agent to
// connect to the workspace.
// Closing the agent is handled by the test cleanup.
func New(t testing.TB, opts ...OptFunc) *Agent {
	t.Helper()

	var o Options
	for _, opt := range opts {
		opt(&o)
	}

	if o.URL == nil {
		require.Fail(t, "must specify URL for agent")
	}
	agentClient := agentsdk.New(o.URL)

	if o.AgentToken == "" {
		o.AgentToken = uuid.NewString()
	}
	agentClient.SetSessionToken(o.AgentToken)

	if o.AgentOptions.Client == nil {
		o.AgentOptions.Client = agentClient
	}

	if o.AgentOptions.ExchangeToken == nil {
		o.AgentOptions.ExchangeToken = func(_ context.Context) (string, error) {
			return o.AgentToken, nil
		}
	}

	if o.AgentOptions.LogDir == "" {
		o.AgentOptions.LogDir = t.TempDir()
	}

	if o.Logger == nil {
		log := slogtest.Make(t, nil).Leveled(slog.LevelDebug).Named("agent")
		o.Logger = &log
	}

	o.AgentOptions.Logger = *o.Logger

	agentCloser := agent.New(o.AgentOptions)
	t.Cleanup(func() {
		assert.NoError(t, agentCloser.Close(), "failed to close agent during cleanup")
	})

	return &Agent{
		agent:       agentCloser,
		agentClient: agentClient,
		waiter: func(c *codersdk.Client) []codersdk.WorkspaceResource {
			if o.WorkspaceID == uuid.Nil {
				require.FailNow(t, "must specify workspace ID for agent in order to wait")
				return nil // unreachable
			}
			return coderdtest.AwaitWorkspaceAgents(t, c, o.WorkspaceID)
		},
	}
}
