package resourcesmonitor_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/proto/resourcesmonitor"
	"github.com/coder/coder/v2/cli/clistat"
	"github.com/coder/quartz"
)

type datapointsPusherMock struct {
	PushResourcesMonitoringUsageFunc func(ctx context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error)
}

func (d *datapointsPusherMock) PushResourcesMonitoringUsage(ctx context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
	return d.PushResourcesMonitoringUsageFunc(ctx, req)
}

func TestPushResourcesMonitoringWithConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		config           *proto.GetResourcesMonitoringConfigurationResponse
		datapointsPusher func(ctx context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error)
		expectedError    bool
		numTicks         int
		expectedCalls    int
	}{
		{
			name: "SuccessfulMonitoring",
			config: &proto.GetResourcesMonitoringConfigurationResponse{
				Enabled: true,
				Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
					NumDatapoints:             20,
					CollectionIntervalSeconds: 1,
				},
				MonitoredVolumes: []string{"/"},
			},
			datapointsPusher: func(_ context.Context, _ *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				return &proto.PushResourcesMonitoringUsageResponse{}, nil
			},
			expectedError: false,
			numTicks:      20,
			expectedCalls: 1,
		},
		{
			name: "SuccessfulMonitoringLongRun",
			config: &proto.GetResourcesMonitoringConfigurationResponse{
				Enabled: true,
				Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
					NumDatapoints:             20,
					CollectionIntervalSeconds: 1,
				},
				MonitoredVolumes: []string{"/"},
			},
			datapointsPusher: func(_ context.Context, _ *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				return &proto.PushResourcesMonitoringUsageResponse{}, nil
			},
			expectedError: false,
			numTicks:      60,
			expectedCalls: 41,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var (
				logger       = slog.Make(sloghuman.Sink(os.Stdout))
				clk          = quartz.NewMock(t)
				counterCalls = 0
			)

			datapointsPusher := func(ctx context.Context, params *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				counterCalls++
				return tt.datapointsPusher(ctx, params)
			}

			pusher := &datapointsPusherMock{
				PushResourcesMonitoringUsageFunc: datapointsPusher,
			}

			resourcesFetcher, err := clistat.New()
			require.NoError(t, err)

			monitor := resourcesmonitor.NewResourcesMonitor(logger, clk, tt.config, resourcesFetcher, pusher)
			err = monitor.Start(ctx)
			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			for i := 0; i < tt.numTicks; i++ {
				_, waiter := clk.AdvanceNext()
				require.NoError(t, waiter.Wait(ctx))
			}

			require.Equal(t, tt.expectedCalls, counterCalls)
			cancel()
		})
	}
}
