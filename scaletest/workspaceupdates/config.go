package workspaceupdates

import (
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)

type Config struct {
	// User is the configuration for the user to create.
	User createusers.Config `json:"user"`

	// Workspace is the configuration for the workspace to create. The workspace
	// will be built using the new user.
	//
	// OrganizationID is ignored and set to the new user's organization ID.
	Workspace workspacebuild.Config `json:"workspace"`

	// WorkspaceCount is the number of workspaces to create.
	WorkspaceCount int64 `json:"power_user_workspaces"`

	// WorkspaceUpdatesTimeout is how long to wait for all expected workspace updates.
	WorkspaceUpdatesTimeout time.Duration `json:"workspace_updates_timeout"`

	// DialTimeout is how long to wait to successfully dial the Coder Connect
	// endpoint.
	DialTimeout time.Duration `json:"dial_timeout"`

	Metrics *Metrics `json:"-"`

	// DialBarrier is used to ensure all runners have dialed the Coder Connect
	// endpoint before creating their workspace(s).
	DialBarrier *sync.WaitGroup `json:"-"`
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

	if c.Workspace.Request.Name != "" {
		return xerrors.New("workspace name cannot be overridden")
	}

	if c.WorkspaceCount <= 0 {
		return xerrors.New("workspace_count must be greater than 0")
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
