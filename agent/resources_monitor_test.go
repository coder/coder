package agent

import (
	"context"
	"os"
	"testing"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/quartz"
)

func TestPushResourcesMonitoringWithConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		configFetcher    resourcesMonitorConfigurationFetcher
		datapointsPusher resourcesMonitorDatapointsPusher
		expectedError    bool
		numTicks         int
		counterCalls     int
		expectedCalls    int
	}{
		{
			name: "Successful monitoring",
			configFetcher: func(_ context.Context, params *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
				return &proto.GetResourcesMonitoringConfigurationResponse{
					Enabled: true,
					Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
						NumDatapoints: 20,
						TickInterval:  1,
					},
					MonitoredVolumes: []string{"/"},
				}, nil
			},
			datapointsPusher: func(_ context.Context, params *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				return &proto.PushResourcesMonitoringUsageResponse{}, nil
			},
			expectedError: false,
			numTicks:      20,
			counterCalls:  0,
			expectedCalls: 1,
		},
		// {
		// 	name: "Disabled monitoring",
		// 	configFetcher: func(ctx context.Context, params *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
		// 		return &proto.GetResourcesMonitoringConfigurationResponse{
		// 			Enabled: false,
		// 		}, nil
		// 	},
		// 	datapointsPusher: func(ctx context.Context, params *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
		// 		return &proto.PushResourcesMonitoringUsageResponse{}, nil
		// 	},
		// 	expectedError: false,
		// 	expectedCalls: 0,
		// },
		// {
		// 	name: "Failed to fetch configuration",
		// 	configFetcher: func(ctx context.Context, params *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
		// 		return nil, xerrors.New("failed to fetch configuration")
		// 	},
		// 	datapointsPusher: func(ctx context.Context, params *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
		// 		return &proto.PushResourcesMonitoringUsageResponse{}, nil
		// 	},
		// 	expectedError: true,
		// 	expectedCalls: 0,
		// },
		// {
		// 	name: "Failed to push datapoints",
		// 	configFetcher: func(ctx context.Context, params *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
		// 		return &proto.GetResourcesMonitoringConfigurationResponse{
		// 			Enabled: true,
		// 			Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
		// 				NumDatapoints: 1,
		// 				TickInterval:  1,
		// 			},
		// 			MonitoredVolumes: []string{"/"},
		// 		}, nil
		// 	},
		// 	datapointsPusher: func(ctx context.Context, params *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
		// 		return nil, xerrors.New("failed to push datapoints")
		// 	},
		// 	expectedError: true,
		// 	expectedCalls: 1,
		// },
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
				t.Logf("pushing datapoints: %v", tt.counterCalls)
				return tt.datapointsPusher(ctx, params)
			}

			err := pushResourcesMonitoringWithConfig(ctx, logger, clk, tt.configFetcher, datapointsPusher)
			if (err != nil) != tt.expectedError {
				t.Errorf("expected error: %v, got: %v", tt.expectedError, err)
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
