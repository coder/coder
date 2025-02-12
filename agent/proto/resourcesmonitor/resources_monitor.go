package resourcesmonitor

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/cli/clistat"
	"github.com/coder/quartz"
)

type monitor struct {
	logger           slog.Logger
	clock            quartz.Clock
	config           *proto.GetResourcesMonitoringConfigurationResponse
	resourcesFetcher *clistat.Statter
	datapointsPusher datapointsPusher
	queue            *Queue
}

//nolint:revive
func NewResourcesMonitor(logger slog.Logger, clock quartz.Clock, config *proto.GetResourcesMonitoringConfigurationResponse, resourcesFetcher *clistat.Statter, datapointsPusher datapointsPusher) *monitor {
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
	if !m.config.Enabled {
		m.logger.Info(ctx, "resources monitoring is disabled - skipping")
		return nil
	}

	m.clock.TickerFunc(ctx, time.Duration(m.config.Config.CollectionIntervalSeconds)*time.Second, func() error {
		memTotal, memUsed, err := m.fetchResourceMonitoredMemory()
		if err != nil {
			m.logger.Error(ctx, "failed to fetch memory", slog.Error(err))
			return nil
		}

		volumes := make([]*VolumeDatapoint, 0, len(m.config.MonitoredVolumes))
		for _, volume := range m.config.MonitoredVolumes {
			volTotal, volUsed, err := m.fetchResourceMonitoredVolume(volume)
			if err != nil {
				m.logger.Error(ctx, "failed to fetch volume", slog.Error(err))
				continue
			}

			volumes = append(volumes, &VolumeDatapoint{
				Path:  volume,
				Total: volTotal,
				Used:  volUsed,
			})
		}

		m.queue.Push(Datapoint{
			Memory: &MemoryDatapoint{
				Total: memTotal,
				Used:  memUsed,
			},
			Volumes: volumes,
		})

		if m.queue.IsFull() {
			_, err = m.datapointsPusher.PushResourcesMonitoringUsage(ctx, &proto.PushResourcesMonitoringUsageRequest{
				Datapoints: m.queue.ItemsAsProto(),
			})
			if err != nil {
				// We don't want to stop the monitoring if we fail to push the datapoints
				// to the server. We just log the error and continue.
				// The queue will anyway remove the oldest datapoint and add the new one.
				m.logger.Error(ctx, "failed to push resources monitoring usage", slog.Error(err))
			}
		}

		return nil
	}, "resources_monitor")

	return nil
}

func (m *monitor) fetchResourceMonitoredMemory() (total int64, used int64, err error) {
	mem, err := m.resourcesFetcher.HostMemory(clistat.PrefixMebi)
	if err != nil {
		return 0, 0, err
	}

	var memTotal, memUsed int64
	if mem.Total == nil {
		return 0, 0, xerrors.New("memory total is nil - can not fetch memory")
	}

	memTotal = m.bytesToMegabytes(int64(*mem.Total))
	memUsed = m.bytesToMegabytes(int64(mem.Used))

	return memTotal, memUsed, nil
}

func (m *monitor) fetchResourceMonitoredVolume(volume string) (total int64, used int64, err error) {
	vol, err := m.resourcesFetcher.Disk(clistat.PrefixMebi, volume)
	if err != nil {
		return 0, 0, err
	}

	var volTotal, volUsed int64
	if vol.Total == nil {
		return 0, 0, xerrors.New("volume total is nil - can not fetch volume")
	}

	volTotal = m.bytesToMegabytes(int64(*vol.Total))
	volUsed = m.bytesToMegabytes(int64(vol.Used))

	return volTotal, volUsed, nil
}

func (*monitor) bytesToMegabytes(bytes int64) int64 {
	return bytes / (1024 * 1024)
}
