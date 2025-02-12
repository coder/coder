package resourcesmonitor_test

import (
	"context"
	"os"
	"testing"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/proto/resourcesmonitor"
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
		counterCalls     int
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
			counterCalls:  0,
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
			counterCalls:  0,
			expectedCalls: 41,
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

			pusher := &datapointsPusherMock{
				PushResourcesMonitoringUsageFunc: datapointsPusher,
			}

			monitor := resourcesmonitor.NewResourcesMonitor(logger, clk, tt.config, pusher)
			err := monitor.Start(ctx)
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
