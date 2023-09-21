package agenttest

import (
	"context"
	"net/url"
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
// Returns the agent client and a function that will block until the agent is
// connected to the workspace.
// Closing the agent is handled by the test cleanup.
func New(t testing.TB, opts ...OptFunc) (agentClient *agentsdk.Client, awaitAgent func(*codersdk.Client)) {
	t.Helper()

	var o Options
	for _, opt := range opts {
		opt(&o)
	}

	if o.URL == nil {
		require.Fail(t, "must specify URL for agent")
	}
	agentClient = agentsdk.New(o.URL)

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

	awaitAgent = func(c *codersdk.Client) {
		if o.WorkspaceID == uuid.Nil {
			require.FailNow(t, "must specify workspace ID for agent in order to wait")
		}
		coderdtest.AwaitWorkspaceAgents(t, c, o.WorkspaceID)
	}

	return agentClient, awaitAgent
}
