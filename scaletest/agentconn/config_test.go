package agentconn_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/scaletest/agentconn"
)

func Test_Config(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	cases := []struct {
		name        string
		config      agentconn.Config
		errContains string
	}{
		{
			name: "OK",
			config: agentconn.Config{
				AgentID:        id,
				ConnectionMode: agentconn.ConnectionModeDirect,
				HoldDuration:   httpapi.Duration(time.Minute),
				Connections: []agentconn.Connection{
					{
						URL:      "http://localhost:8080/path",
						Interval: httpapi.Duration(time.Second),
						Timeout:  httpapi.Duration(time.Second),
					},
					{
						URL:      "http://localhost:8000/differentpath",
						Interval: httpapi.Duration(2 * time.Second),
						Timeout:  httpapi.Duration(2 * time.Second),
					},
				},
			},
		},
		{
			name: "NoAgentID",
			config: agentconn.Config{
				AgentID:        uuid.Nil,
				ConnectionMode: agentconn.ConnectionModeDirect,
				HoldDuration:   0,
				Connections:    nil,
			},
			errContains: "agent_id must be set",
		},
		{
			name: "NoConnectionMode",
			config: agentconn.Config{
				AgentID:        id,
				ConnectionMode: "",
				HoldDuration:   0,
				Connections:    nil,
			},
			errContains: "connection_mode must be set",
		},
		{
			name: "InvalidConnectionMode",
			config: agentconn.Config{
				AgentID:        id,
				ConnectionMode: "blah",
				HoldDuration:   0,
				Connections:    nil,
			},
			errContains: "invalid connection_mode",
		},
		{
			name: "NegativeHoldDuration",
			config: agentconn.Config{
				AgentID:        id,
				ConnectionMode: agentconn.ConnectionModeDerp,
				HoldDuration:   -1,
				Connections:    nil,
			},
			errContains: "hold_duration must be a positive value",
		},
		{
			name: "ConnectionNoURL",
			config: agentconn.Config{
				AgentID:        id,
				ConnectionMode: agentconn.ConnectionModeDirect,
				HoldDuration:   1,
				Connections: []agentconn.Connection{{
					URL:      "",
					Interval: 0,
					Timeout:  0,
				}},
			},
			errContains: "connections[0].url must be set",
		},
		{
			name: "ConnectionInvalidURL",
			config: agentconn.Config{
				AgentID:        id,
				ConnectionMode: agentconn.ConnectionModeDirect,
				HoldDuration:   1,
				Connections: []agentconn.Connection{{
					URL:      string([]byte{0x7f}),
					Interval: 0,
					Timeout:  0,
				}},
			},
			errContains: "connections[0].url is not a valid URL",
		},
		{
			name: "ConnectionInvalidURLScheme",
			config: agentconn.Config{
				AgentID:        id,
				ConnectionMode: agentconn.ConnectionModeDirect,
				HoldDuration:   1,
				Connections: []agentconn.Connection{{
					URL:      "blah://localhost:8080",
					Interval: 0,
					Timeout:  0,
				}},
			},
			errContains: "connections[0].url has an unsupported scheme",
		},
		{
			name: "ConnectionNegativeInterval",
			config: agentconn.Config{
				AgentID:        id,
				ConnectionMode: agentconn.ConnectionModeDirect,
				HoldDuration:   1,
				Connections: []agentconn.Connection{{
					URL:      "http://localhost:8080",
					Interval: -1,
					Timeout:  0,
				}},
			},
			errContains: "connections[0].interval must be a positive value",
		},
		{
			name: "ConnectionIntervalMustBeZero",
			config: agentconn.Config{
				AgentID:        id,
				ConnectionMode: agentconn.ConnectionModeDirect,
				HoldDuration:   0,
				Connections: []agentconn.Connection{{
					URL:      "http://localhost:8080",
					Interval: 1,
					Timeout:  0,
				}},
			},
			errContains: "connections[0].interval must be 0 if hold_duration is 0",
		},
		{
			name: "ConnectionNegativeTimeout",
			config: agentconn.Config{
				AgentID:        id,
				ConnectionMode: agentconn.ConnectionModeDirect,
				HoldDuration:   1,
				Connections: []agentconn.Connection{{
					URL:      "http://localhost:8080",
					Interval: 0,
					Timeout:  -1,
				}},
			},
			errContains: "connections[0].timeout must be a positive value",
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
