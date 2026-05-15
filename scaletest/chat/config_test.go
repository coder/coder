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

func newConfig(t *testing.T) chat.Config {
	t.Helper()
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

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*chat.Config)
		wantErr string
	}{
		{
			name: "ValidSharedWorkspace",
		},
		{
			name: "TurnsMustBePositive",
			mutate: func(cfg *chat.Config) {
				cfg.Turns = 0
			},
			wantErr: "validate turns: must be at least 1",
		},
		{
			name: "DelayedFollowUpsRequireBarrier",
			mutate: func(cfg *chat.Config) {
				cfg.FollowUpStartDelay = 10 * time.Second
			},
			wantErr: "validate follow_up_ready_wait_group: must not be nil when follow-up delay is enabled",
		},
		{
			name: "DelayedFollowUpsRequireStartChan",
			mutate: func(cfg *chat.Config) {
				cfg.FollowUpStartDelay = 10 * time.Second
				cfg.FollowUpReadyWaitGroup = &sync.WaitGroup{}
			},
			wantErr: "validate start_follow_up_chan: must not be nil when follow-up delay is enabled",
		},
		{
			name: "MetricsRequired",
			mutate: func(cfg *chat.Config) {
				cfg.Metrics = nil
			},
			wantErr: "validate metrics: must not be nil",
		},
		{
			name: "MetricLabelValuesCardinalityMismatch",
			mutate: func(cfg *chat.Config) {
				cfg.MetricLabelValues = nil
			},
			wantErr: "validate metric_label_values: got 0 values, want 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := newConfig(t)
			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			err := cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.EqualError(t, err, tt.wantErr)
		})
	}
}
