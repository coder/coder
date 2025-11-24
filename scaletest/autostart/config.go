package autostart

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

	// WorkspaceJobTimeout is how long to wait for any one workspace job
	// (start or stop) to complete.
	WorkspaceJobTimeout time.Duration `json:"workspace_job_timeout"`

	// AutostartDelay is how long after all the workspaces have been stopped
	// to schedule them to be started again.
	AutostartDelay time.Duration `json:"autostart_delay"`

	// AutostartTimeout is how long to wait for the autostart build to be
	// initiated after the scheduled time.
	AutostartTimeout time.Duration `json:"autostart_timeout"`

	Metrics *Metrics `json:"-"`

	// SetupBarrier is used to ensure all runners own stopped workspaces
	// before setting the autostart schedule on each.
	SetupBarrier *sync.WaitGroup `json:"-"`
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

	if c.SetupBarrier == nil {
		return xerrors.New("setup barrier must be set")
	}

	if c.WorkspaceJobTimeout <= 0 {
		return xerrors.New("workspace_job_timeout must be greater than 0")
	}

	if c.AutostartDelay < time.Minute*2 {
		return xerrors.New("autostart_delay must be at least 2 minutes")
	}

	if c.AutostartTimeout <= 0 {
		return xerrors.New("autostart_timeout must be greater than 0")
	}

	if c.Metrics == nil {
		return xerrors.New("metrics must be set")
	}

	return nil
}
