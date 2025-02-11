package agent_test

import (
	"context"
	"os"
	"testing"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/quartz"
)

func TestPushResourcesMonitoringWithConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		configFetcher    agent.ResourcesMonitorConfigurationFetcher
		datapointsPusher agent.ResourcesMonitorDatapointsPusher
		expectedError    bool
		numTicks         int
		counterCalls     int
		expectedCalls    int
	}{
		{
			name: "SuccessfulMonitoring",
			configFetcher: func(_ context.Context, _ *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
				return &proto.GetResourcesMonitoringConfigurationResponse{
					Enabled: true,
					Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
						NumDatapoints: 20,
						TickInterval:  1,
					},
					MonitoredVolumes: []string{"/"},
				}, nil
			},
			datapointsPusher: func(_ context.Context, _ *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				return &proto.PushResourcesMonitoringUsageResponse{}, nil
			},
			expectedError: false,
			numTicks:      20,
			counterCalls:  0,
			expectedCalls: 1,
		},
		{
			name: "SuccessfulMonitoringLongRun",
			configFetcher: func(_ context.Context, _ *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
				return &proto.GetResourcesMonitoringConfigurationResponse{
					Enabled: true,
					Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
						NumDatapoints: 20,
						TickInterval:  1,
					},
					MonitoredVolumes: []string{"/"},
				}, nil
			},
			datapointsPusher: func(_ context.Context, _ *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				return &proto.PushResourcesMonitoringUsageResponse{}, nil
			},
			expectedError: false,
			numTicks:      60,
			counterCalls:  0,
			expectedCalls: 41,
		},
		{
			name: "FailedToFetchConfiguration",
			configFetcher: func(_ context.Context, _ *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
				return &proto.GetResourcesMonitoringConfigurationResponse{}, xerrors.New("failed to fetch configuration")
			},
			datapointsPusher: func(_ context.Context, _ *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				return &proto.PushResourcesMonitoringUsageResponse{}, nil
			},
			expectedError: true,
			numTicks:      10,
			counterCalls:  0,
			expectedCalls: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clk := quartz.NewMock(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			logger := slog.Make(sloghuman.Sink(os.Stdout))

			datapointsPusher := func(ctx context.Context, params *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				tt.counterCalls++
				return tt.datapointsPusher(ctx, params)
			}

			err := agent.PushResourcesMonitoringWithConfig(ctx, logger, clk, tt.configFetcher, datapointsPusher)
			if (err != nil) != tt.expectedError {
				t.Errorf("expected error: %v, got: %v", tt.expectedError, err)
			}

			if tt.expectedError {
				return
			}

			for i := 0; i < tt.numTicks; i++ {
				_, waiter := clk.AdvanceNext()
				waiter.Wait(ctx)
			}

			if tt.counterCalls != tt.expectedCalls {
				t.Errorf("expected call count: %v, got: %v", tt.expectedCalls, tt.counterCalls)
			}

			cancel()
		})
	}
}
