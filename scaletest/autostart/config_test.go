package autostart_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/autostart"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	baseWorkspace := workspacebuild.Config{
		Request: codersdk.CreateWorkspaceRequest{
			TemplateID: uuid.New(),
			Name:       "scaletest-0",
		},
	}
	base := func() autostart.Config {
		return autostart.Config{
			User:                  createusers.Config{OrganizationID: orgID},
			Workspace:             baseWorkspace,
			WorkspaceJobTimeout:   5 * time.Minute,
			AutostartDelay:        2 * time.Minute,
			AutostartBuildTimeout: 15 * time.Minute,
			SetupBarrier:          &sync.WaitGroup{},
			BuildUpdates:          make(chan codersdk.WorkspaceBuildUpdate),
		}
	}

	t.Run("CreateMode", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, base().Validate())
	})

	t.Run("ReuseMode", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.SessionToken = "token"
		cfg.PreCreatedUser = codersdk.User{ReducedUser: codersdk.ReducedUser{MinimalUser: codersdk.MinimalUser{ID: uuid.New()}}}
		require.NoError(t, cfg.Validate())
	})

	t.Run("ReuseModeMissingUser", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.SessionToken = "token"
		err := cfg.Validate()
		require.ErrorContains(t, err, "pre_created_user must be set")
	})
}
