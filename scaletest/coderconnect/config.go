package coderconnect

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
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
		return xerrors.New("organization_id must not be a nil UUID")
	}
	if c.SessionToken != "" {
		if c.Username != "" {
			return xerrors.New("username must be empty when session_token is set")
		}
		if c.Email != "" {
			return xerrors.New("email must be empty when session_token is set")
		}
	}

	return nil
}

type Config struct {
	// User is the configuration for the user to create or use.
	User UserConfig `json:"user"`

	// Workspace is the configuration for the workspace to create. The workspace
	// will be built using the new user.
	//
	// OrganizationID is ignored and set to the new user's organization ID.
	Workspace workspacebuild.Config `json:"workspace"`

	// WorkspaceCount is the number of workspaces to create.
	WorkspaceCount int64 `json:"power_user_workspaces"`

	// WorkspaceUpdatesTimeout is how long to wait for all expected workspace updates.
	WorkspaceUpdatesTimeout time.Duration `json:"workspace_updates_timeout"`

	// DialTimeout is how long to wait for the Coder Connect endpoint to be
	// reachable.
	DialTimeout time.Duration `json:"dial_timeout"`

	// NoCleanup determines whether users and workspaces should be left after the test.
	NoCleanup bool `json:"no_cleanup"`

	Metrics *Metrics `json:"-"`

	MetricLabelValues []string `json:"metric_label_values"`

	// DialBarrier is used to ensure all runners have dialed the Coder Connect
	// endpoint before creating their workspace(s).
	DialBarrier *harness.Barrier `json:"-"`
}

func (c Config) Validate() error {
	if err := c.User.Validate(); err != nil {
		return xerrors.Errorf("user config: %w", err)
	}
	c.Workspace.OrganizationID = c.User.OrganizationID
	// This value will be overwritten during the test.
	c.Workspace.UserID = codersdk.Me
	if err := c.Workspace.Validate(); err != nil {
		return xerrors.Errorf("workspace config: %w", err)
	}

	if c.DialBarrier == nil {
		return xerrors.New("dial barrier must be set")
	}

	if c.WorkspaceUpdatesTimeout <= 0 {
		return xerrors.New("workspace_updates_timeout must be greater than 0")
	}

	if c.DialTimeout <= 0 {
		return xerrors.New("dial_timeout must be greater than 0")
	}

	if c.Metrics == nil {
		return xerrors.New("metrics must be set")
	}

	return nil
}
