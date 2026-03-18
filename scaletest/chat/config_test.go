package chat_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/chat"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	newConfig := func() chat.Config {
		reg := prometheus.NewRegistry()
		return chat.Config{
			RunID:             "run-123",
			WorkspaceID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			Prompt:            "Reply with one short sentence.",
			Turns:             2,
			FollowUpPrompt:    "Continue.",
			ReadyWaitGroup:    &sync.WaitGroup{},
			StartChan:         make(chan struct{}),
			Metrics:           chat.NewMetrics(reg, chat.MetricLabelNames()...),
			MetricLabelValues: chat.MetricLabelValues("run-123"),
		}
	}

	t.Run("ValidSharedWorkspace", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig()
		require.NoError(t, cfg.Validate())
	})

	t.Run("ValidTemplateWorkspace", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig()
		cfg.WorkspaceID = uuid.Nil
		cfg.Workspace = workspacebuild.Config{
			OrganizationID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			UserID:         codersdk.Me,
			Request: codersdk.CreateWorkspaceRequest{
				TemplateID: uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			},
		}
		require.NoError(t, cfg.Validate())
	})

	t.Run("WorkspaceSelectionIsMutuallyExclusive", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig()
		cfg.Workspace = workspacebuild.Config{
			OrganizationID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			UserID:         codersdk.Me,
			Request: codersdk.CreateWorkspaceRequest{
				TemplateID: uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			},
		}
		require.ErrorContains(t, cfg.Validate(), "exactly one of workspace_id or workspace config")
	})

	t.Run("DelayedFollowUpsRequireBarrier", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig()
		cfg.FollowUpStartDelay = 10 * time.Second
		require.ErrorContains(t, cfg.Validate(), "follow_up_ready_wait_group")

		cfg.FollowUpReadyWaitGroup = &sync.WaitGroup{}
		require.ErrorContains(t, cfg.Validate(), "start_follow_up_chan")

		cfg.StartFollowUpChan = make(chan struct{})
		require.NoError(t, cfg.Validate())
	})

	t.Run("NegativeFollowUpDelayRejected", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig()
		cfg.FollowUpStartDelay = -time.Second
		require.ErrorContains(t, cfg.Validate(), "must not be negative")
	})
}
