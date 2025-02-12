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
	datapointsPusher datapointsPusher
}

//nolint:revive
func NewResourcesMonitor(logger slog.Logger, clock quartz.Clock, config *proto.GetResourcesMonitoringConfigurationResponse, datapointsPusher datapointsPusher) *monitor {
	return &monitor{
		logger:           logger,
		clock:            clock,
		config:           config,
		datapointsPusher: datapointsPusher,
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

	resourcesFetcher, err := clistat.New()
	if err != nil {
		return xerrors.Errorf("failed to create resources fetcher: %w", err)
	}

	datapointsQueue := NewQueue(int(m.config.Config.NumDatapoints))

	m.clock.TickerFunc(ctx, time.Duration(m.config.Config.CollectionIntervalSeconds*int32(time.Second)), func() error {
		memTotal, memUsed, err := m.fetchResourceMonitoredMemory(resourcesFetcher)
		if err != nil {
			m.logger.Error(ctx, "failed to fetch memory", slog.Error(err))
			return nil
		}

		volumes := make([]*VolumeDatapoint, 0, len(m.config.MonitoredVolumes))
		for _, volume := range m.config.MonitoredVolumes {
			volTotal, volUsed, err := m.fetchResourceMonitoredVolume(resourcesFetcher, volume)
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

		datapointsQueue.Push(Datapoint{
			Memory: &MemoryDatapoint{
				Total: memTotal,
				Used:  memUsed,
			},
			Volumes: volumes,
		})

		if datapointsQueue.IsFull() {
			_, err = m.datapointsPusher.PushResourcesMonitoringUsage(ctx, &proto.PushResourcesMonitoringUsageRequest{
				Datapoints: datapointsQueue.ItemsAsProto(),
			})
			if err != nil {
				m.logger.Error(ctx, "failed to push resources monitoring usage", slog.Error(err))
			}
		}

		return nil
	}, "resources_monitor")

	return nil
}

func (m *monitor) fetchResourceMonitoredMemory(fetcher *clistat.Statter) (total int64, used int64, err error) {
	mem, err := fetcher.HostMemory(clistat.PrefixMebi)
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

func (m *monitor) fetchResourceMonitoredVolume(fetcher *clistat.Statter, volume string) (total int64, used int64, err error) {
	vol, err := fetcher.Disk(clistat.PrefixMebi, volume)
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
