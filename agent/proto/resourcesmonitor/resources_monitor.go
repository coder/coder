package resourcesmonitor

import (
	"context"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/quartz"
)

type monitor struct {
	logger           slog.Logger
	clock            quartz.Clock
	config           *proto.GetResourcesMonitoringConfigurationResponse
	resourcesFetcher Fetcher
	datapointsPusher datapointsPusher
	queue            *Queue
}

//nolint:revive
func NewResourcesMonitor(logger slog.Logger, clock quartz.Clock, config *proto.GetResourcesMonitoringConfigurationResponse, resourcesFetcher Fetcher, datapointsPusher datapointsPusher) *monitor {
	return &monitor{
		logger:           logger,
		clock:            clock,
		config:           config,
		resourcesFetcher: resourcesFetcher,
		datapointsPusher: datapointsPusher,
		queue:            NewQueue(int(config.Config.NumDatapoints)),
	}
}

type datapointsPusher interface {
	PushResourcesMonitoringUsage(ctx context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error)
}

func (m *monitor) Start(ctx context.Context) error {
	m.clock.TickerFunc(ctx, time.Duration(m.config.Config.CollectionIntervalSeconds)*time.Second, func() error {
		datapoint := Datapoint{
			CollectedAt: m.clock.Now(),
			Volumes:     make([]*VolumeDatapoint, 0, len(m.config.Volumes)),
		}

		if m.config.Memory != nil && m.config.Memory.Enabled {
			memTotal, memUsed, err := m.resourcesFetcher.FetchMemory()
			if err != nil {
				m.logger.Error(ctx, "failed to fetch memory", slog.Error(err))
			} else {
				datapoint.Memory = &MemoryDatapoint{
					Total: memTotal,
					Used:  memUsed,
				}
			}
		}

		for _, volume := range m.config.Volumes {
			if !volume.Enabled {
				continue
			}

			volTotal, volUsed, err := m.resourcesFetcher.FetchVolume(volume.Path)
			if err != nil {
				m.logger.Error(ctx, "failed to fetch volume", slog.Error(err))
				continue
			}

			datapoint.Volumes = append(datapoint.Volumes, &VolumeDatapoint{
				Path:  volume.Path,
				Total: volTotal,
				Used:  volUsed,
			})
		}

		m.queue.Push(datapoint)

		if m.queue.IsFull() {
			_, err := m.datapointsPusher.PushResourcesMonitoringUsage(ctx, &proto.PushResourcesMonitoringUsageRequest{
				Datapoints: m.queue.ItemsAsProto(),
			})
			if err != nil {
				// We don't want to stop the monitoring if we fail to push the datapoints
				// to the server. We just log the error and continue.
				// The queue will anyway remove the oldest datapoint and add the new one.
				m.logger.Error(ctx, "failed to push resources monitoring usage", slog.Error(err))
				return nil
			}
		}

		return nil
	}, "resources_monitor")

	return nil
}
