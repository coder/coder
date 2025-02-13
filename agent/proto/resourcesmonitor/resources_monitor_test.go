package resourcesmonitor_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		name                              string
		config                            *proto.GetResourcesMonitoringConfigurationResponse
		datapointsPusher                  func(ctx context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error)
		fetchResourcesMonitoredMemoryFunc func() (int64, int64, error)
		fetchResourcesMonitoredVolumeFunc func(volume string) (int64, int64, error)
		numTicks                          int
	}{
		{
			name: "SuccessfulMonitoring",
			config: &proto.GetResourcesMonitoringConfigurationResponse{
				Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
					NumDatapoints:             20,
					CollectionIntervalSeconds: 1,
				},
				Volumes: []*proto.GetResourcesMonitoringConfigurationResponse_Volume{
					{
						Enabled: true,
						Path:    "/",
					},
				},
			},
			datapointsPusher: func(_ context.Context, _ *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				return &proto.PushResourcesMonitoringUsageResponse{}, nil
			},
			fetchResourcesMonitoredMemoryFunc: func() (int64, int64, error) {
				return 1000, 8000, nil
			},
			fetchResourcesMonitoredVolumeFunc: func(_ string) (int64, int64, error) {
				return 50000, 100000, nil
			},
			numTicks: 20,
		},
		{
			name: "SuccessfulMonitoringLongRun",
			config: &proto.GetResourcesMonitoringConfigurationResponse{
				Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
					NumDatapoints:             20,
					CollectionIntervalSeconds: 1,
				},
				Volumes: []*proto.GetResourcesMonitoringConfigurationResponse_Volume{
					{
						Enabled: true,
						Path:    "/",
					},
				},
			},
			datapointsPusher: func(_ context.Context, _ *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				return &proto.PushResourcesMonitoringUsageResponse{}, nil
			},
			fetchResourcesMonitoredMemoryFunc: func() (int64, int64, error) {
				return 1000, 8000, nil
			},
			fetchResourcesMonitoredVolumeFunc: func(_ string) (int64, int64, error) {
				return 50000, 100000, nil
			},
			numTicks: 60,
		},
		{
			// We want to make sure that even if the datapointsPusher fails, the monitoring continues.
			name: "ErrorPushingDatapoints",
			config: &proto.GetResourcesMonitoringConfigurationResponse{
				Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
					NumDatapoints:             20,
					CollectionIntervalSeconds: 1,
				},
				Volumes: []*proto.GetResourcesMonitoringConfigurationResponse_Volume{
					{
						Enabled: true,
						Path:    "/",
					},
				},
			},
			datapointsPusher: func(_ context.Context, _ *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				return nil, assert.AnError
			},
			fetchResourcesMonitoredMemoryFunc: func() (int64, int64, error) {
				return 1000, 8000, nil
			},
			fetchResourcesMonitoredVolumeFunc: func(_ string) (int64, int64, error) {
				return 50000, 100000, nil
			},
			numTicks: 60,
		},
		{
			// If one of the resources fails to be fetched, the datapoints still should be pushed with the other resources.
			name: "ErrorFetchingMemory",
			config: &proto.GetResourcesMonitoringConfigurationResponse{
				Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
					NumDatapoints:             20,
					CollectionIntervalSeconds: 1,
				},
				Volumes: []*proto.GetResourcesMonitoringConfigurationResponse_Volume{
					{
						Enabled: true,
						Path:    "/",
					},
				},
			},
			datapointsPusher: func(_ context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				require.Len(t, req.Datapoints, 20)
				require.Nil(t, req.Datapoints[0].Memory)
				require.NotNil(t, req.Datapoints[0].Volumes)
				require.Equal(t, &proto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage{
					Volume: "/",
					Total:  100000,
					Used:   50000,
				}, req.Datapoints[0].Volumes[0])

				return &proto.PushResourcesMonitoringUsageResponse{}, nil
			},
			fetchResourcesMonitoredMemoryFunc: func() (int64, int64, error) {
				return 0, 0, assert.AnError
			},
			fetchResourcesMonitoredVolumeFunc: func(_ string) (int64, int64, error) {
				return 100000, 50000, nil
			},
			numTicks: 20,
		},
		{
			// If one of the resources fails to be fetched, the datapoints still should be pushed with the other resources.
			name: "ErrorFetchingVolume",
			config: &proto.GetResourcesMonitoringConfigurationResponse{
				Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
					NumDatapoints:             20,
					CollectionIntervalSeconds: 1,
				},
				Volumes: []*proto.GetResourcesMonitoringConfigurationResponse_Volume{
					{
						Enabled: true,
						Path:    "/",
					},
				},
			},
			datapointsPusher: func(_ context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				require.Len(t, req.Datapoints, 20)
				require.Len(t, req.Datapoints[0].Volumes, 0)

				return &proto.PushResourcesMonitoringUsageResponse{}, nil
			},
			fetchResourcesMonitoredMemoryFunc: func() (int64, int64, error) {
				return 1000, 8000, nil
			},
			fetchResourcesMonitoredVolumeFunc: func(_ string) (int64, int64, error) {
				return 0, 0, assert.AnError
			},
			numTicks: 20,
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

			datapointsPusher := func(ctx context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
				counterCalls++
				return tt.datapointsPusher(ctx, req)
			}

			pusher := &datapointsPusherMock{
				PushResourcesMonitoringUsageFunc: datapointsPusher,
			}

			resourcesFetcher := &resourcesFetcherMock{
				fetchResourcesMonitoredMemoryFunc: tt.fetchResourcesMonitoredMemoryFunc,
				fetchResourcesMonitoredVolumeFunc: tt.fetchResourcesMonitoredVolumeFunc,
			}

			monitor := resourcesmonitor.NewResourcesMonitor(logger, clk, tt.config, resourcesFetcher, pusher)
			require.NoError(t, monitor.Start(ctx))

			for i := 0; i < tt.numTicks; i++ {
				_, waiter := clk.AdvanceNext()
				require.NoError(t, waiter.Wait(ctx))
			}

			// expectedCalls is computed with the following logic :
			// We have one call per tick, once reached the ${config.NumDatapoints}.
			expectedCalls := tt.numTicks - int(tt.config.Config.NumDatapoints) + 1
			require.Equal(t, expectedCalls, counterCalls)
			cancel()
		})
	}
}
