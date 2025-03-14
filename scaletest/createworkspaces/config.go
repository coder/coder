package createworkspaces
import (
	"fmt"
	"errors"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/agentconn"
	"github.com/coder/coder/v2/scaletest/reconnectingpty"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)
type UserConfig struct {
	// OrganizationID is the ID of the organization to add the user to.
	OrganizationID uuid.UUID `json:"organization_id"`
	// Username is the username of the new user.
	Username string `json:"username"`
	// Email is the email of the new user.
	Email string `json:"email"`
	// SessionToken is the session token of an already existing user. If set, no
	// user will be created.
	SessionToken string `json:"session_token"`
}
func (c UserConfig) Validate() error {
	if c.OrganizationID == uuid.Nil {
		return errors.New("organization_id must not be a nil UUID")
	}
	if c.SessionToken == "" {
		if c.Username == "" {
			return errors.New("username must be set")
		}
		if c.Email == "" {
			return errors.New("email must be set")
		}
	}
	return nil
}
type Config struct {
	// User is the configuration for the user to create.
	User UserConfig `json:"user"`
	// Workspace is the configuration for the workspace to create. The workspace
	// will be built using the new user.
	//
	// OrganizationID is ignored and set to the new user's organization ID.
	Workspace workspacebuild.Config `json:"workspace"`
	// ReconnectingPTY is the configuration for web terminal connections to the
	// new workspace. If nil, no web terminal connections will be made. Runs in
	// parallel to agent connections if specified.
	//
	// AgentID is ignored and set to the new workspace's agent ID.
	ReconnectingPTY *reconnectingpty.Config `json:"reconnecting_pty"`
	// AgentConn is the configuration for connections made to the agent. If nil,
	// no agent connections will be made. Runs in parallel to reconnecting pty
	// connections if specified.
	//
	// AgentID is ignored and set to the new workspace's agent ID.
	AgentConn *agentconn.Config `json:"agent_conn"`
	// NoCleanup determines whether the user and workspace should be left as is
	// and not deleted or stopped in any way.
	NoCleanup bool `json:"no_cleanup"`
}
func (c Config) Validate() error {
	if err := c.User.Validate(); err != nil {
		return fmt.Errorf("validate user: %w", err)
	}
	c.Workspace.OrganizationID = c.User.OrganizationID
	// This value will be overwritten during the test.
	c.Workspace.UserID = codersdk.Me
	if err := c.Workspace.Validate(); err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	if c.ReconnectingPTY != nil {
		// This value will be overwritten during the test.
		c.ReconnectingPTY.AgentID = uuid.New()
		if err := c.ReconnectingPTY.Validate(); err != nil {
			return fmt.Errorf("validate reconnecting pty: %w", err)
		}
	}
	if c.AgentConn != nil {
		// This value will be overwritten during the test.
		c.AgentConn.AgentID = uuid.New()
		if err := c.AgentConn.Validate(); err != nil {
			return fmt.Errorf("validate agent conn: %w", err)
		}
	}
	return nil
}
