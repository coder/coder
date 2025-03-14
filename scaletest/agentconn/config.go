package agentconn
import (
	"fmt"
	"errors"
	"net/url"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/httpapi"
)
type ConnectionMode string
const (
	ConnectionModeDirect ConnectionMode = "direct"
	ConnectionModeDerp   ConnectionMode = "derp"
)
type Config struct {
	// AgentID is the ID of the agent to connect to.
	AgentID uuid.UUID `json:"agent_id"`
	// ConnectionMode is the strategy to use when connecting to the agent.
	ConnectionMode ConnectionMode `json:"connection_mode"`
	// HoldDuration is the duration to hold the connection open for. If set to
	// 0, the connection will be closed immediately after making each request
	// once.
	HoldDuration httpapi.Duration `json:"hold_duration"`
	// Connections is the list of connections to make to services running
	// inside the workspace. Only HTTP connections are supported.
	Connections []Connection `json:"connections"`
}
type Connection struct {
	// URL is the address to connect to (e.g. "http://127.0.0.1:8080/path"). The
	// endpoint must respond with a any response within timeout. The IP address
	// is ignored and the connection is made to the agent's WireGuard IP
	// instead.
	URL string `json:"url"`
	// Interval is the duration to wait between connections to this endpoint. If
	// set to 0, the connection will only be made once. Must be set to 0 if
	// the parent config's hold_duration is set to 0.
	Interval httpapi.Duration `json:"interval"`
	// Timeout is the duration to wait for a connection to this endpoint to
	// succeed. If set to 0, the default timeout will be used.
	Timeout httpapi.Duration `json:"timeout"`
}
func (c Config) Validate() error {
	if c.AgentID == uuid.Nil {
		return errors.New("agent_id must be set")
	}
	if c.ConnectionMode == "" {
		return errors.New("connection_mode must be set")
	}
	switch c.ConnectionMode {
	case ConnectionModeDirect:
	case ConnectionModeDerp:
	default:
		return fmt.Errorf("invalid connection_mode: %q", c.ConnectionMode)
	}
	if c.HoldDuration < 0 {
		return errors.New("hold_duration must be a positive value")
	}
	for i, conn := range c.Connections {
		if conn.URL == "" {
			return fmt.Errorf("connections[%d].url must be set", i)
		}
		u, err := url.Parse(conn.URL)
		if err != nil {
			return fmt.Errorf("connections[%d].url is not a valid URL: %w", i, err)
		}
		if u.Scheme != "http" {
			return fmt.Errorf("connections[%d].url has an unsupported scheme %q, only http is supported", i, u.Scheme)
		}
		if conn.Interval < 0 {
			return fmt.Errorf("connections[%d].interval must be a positive value", i)
		}
		if conn.Interval > 0 && c.HoldDuration == 0 {
			return fmt.Errorf("connections[%d].interval must be 0 if hold_duration is 0", i)
		}
		if conn.Timeout < 0 {
			return fmt.Errorf("connections[%d].timeout must be a positive value", i)
		}
	}
	return nil
}
