package reconnectingpty_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/scaletest/reconnectingpty"
)

func Test_Config(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	cases := []struct {
		name        string
		config      reconnectingpty.Config
		errContains string
	}{
		{
			name: "OKBasic",
			config: reconnectingpty.Config{
				AgentID: id,
			},
		},
		{
			name: "OKFull",
			config: reconnectingpty.Config{
				AgentID: id,
				Init: workspacesdk.AgentReconnectingPTYInit{
					ID:      id,
					Width:   80,
					Height:  24,
					Command: "echo 'hello world'",
				},
				Timeout:       httpapi.Duration(time.Minute),
				ExpectTimeout: false,
				ExpectOutput:  "hello world",
				LogOutput:     true,
			},
		},
		{
			name: "NoAgentID",
			config: reconnectingpty.Config{
				AgentID: uuid.Nil,
			},
			errContains: "agent_id must be set",
		},
		{
			name: "NegativeTimeout",
			config: reconnectingpty.Config{
				AgentID: id,
				Timeout: httpapi.Duration(-time.Minute),
			},
			errContains: "timeout must be a positive value",
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			err := c.config.Validate()
			if c.errContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), c.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
