package chat_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scaletest/chat"
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

	t.Run("ValidImmediateFollowUps", func(t *testing.T) {
		t.Parallel()
		cfg := newConfig()
		require.NoError(t, cfg.Validate())
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
