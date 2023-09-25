package agenttest

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// New starts a new agent for use in tests.
// The agent will use the provided coder URL and session token.
// The options passed to agent.New() can be modified by passing an optional
// variadic func(*agent.Options).
// Returns the agent. Closing the agent is handled by the test cleanup.
// It is the responsibility of the caller to call coderdtest.AwaitWorkspaceAgents
// to ensure agent is connected.
func New(t testing.TB, coderURL *url.URL, agentToken string, opts ...func(*agent.Options)) agent.Agent {
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
		agentClient.SDK.SetLogger(log)
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

	agt := agent.New(o)
	t.Cleanup(func() {
		assert.NoError(t, agt.Close(), "failed to close agent during cleanup")
	})

	return agt
}
