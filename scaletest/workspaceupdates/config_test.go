package workspaceupdates_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
	"github.com/coder/coder/v2/scaletest/workspaceupdates"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	// validConfig returns an otherwise-valid config so each case can vary only
	// the reuse fields under test.
	validConfig := func() workspaceupdates.Config {
		return workspaceupdates.Config{
			User: createusers.Config{OrganizationID: uuid.New()},
			Workspace: workspacebuild.Config{
				Request: codersdk.CreateWorkspaceRequest{TemplateID: uuid.New()},
			},
			WorkspaceCount:          1,
			WorkspaceUpdatesTimeout: time.Minute,
			DialTimeout:             time.Minute,
			Metrics:                 workspaceupdates.NewMetrics(prometheus.NewRegistry()),
			DialBarrier:             &sync.WaitGroup{},
		}
	}

	t.Run("CreateUsers", func(t *testing.T) {
		t.Parallel()

		// No session token: the runner creates its own user.
		require.NoError(t, validConfig().Validate())
	})

	t.Run("ReuseValid", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.SessionToken = "a-token"
		cfg.PreCreatedUser = codersdk.User{
			ReducedUser: codersdk.ReducedUser{
				MinimalUser: codersdk.MinimalUser{ID: uuid.New()},
			},
		}
		require.NoError(t, cfg.Validate())
	})

	t.Run("ReuseMissingUser", func(t *testing.T) {
		t.Parallel()

		// A session token without a pre-created user is a misconfiguration: the
		// runner would have a token but no user identity to run as.
		cfg := validConfig()
		cfg.SessionToken = "a-token"
		require.ErrorContains(t, cfg.Validate(), "pre_created_user must be set")
	})
}
