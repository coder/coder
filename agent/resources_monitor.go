package agent

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/cli/clistat"
	"github.com/coder/quartz"
)

func (a *agent) pushResourcesMonitoring(ctx context.Context, aAPI proto.DRPCAgentClient24) error {
	logger := a.logger.Named("resources_monitor")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	config, err := aAPI.GetResourcesMonitoringConfiguration(ctx, &proto.GetResourcesMonitoringConfigurationRequest{})
	if err != nil {
		return xerrors.Errorf("failed to get resources monitoring configuration: %w", err)
	}

	if !config.Enabled {
		logger.Info(ctx, "resources monitoring is disabled")
		return nil
	}

	clk := quartz.NewReal()

	resourcesFetcher, err := clistat.New()
	if err != nil {
		return xerrors.Errorf("failed to create resources fetcher: %w", err)
	}

	datapointsQueue := newResourcesMonitorQueue(int(config.Config.NumDatapoints))

	clk.TickerFunc(ctx, time.Duration(config.Config.TickInterval*int32(time.Second)), func() error {
		memTotal, memUsed, err := fetchResourceMonitoredMemory(resourcesFetcher)
		if err != nil {
			logger.Error(ctx, "failed to fetch memory", slog.Error(err))
			return nil
		}

		volumes := make([]*resourcesMonitor_VolumeDatapoint, 0, len(config.MonitoredVolumes))
		for _, volume := range config.MonitoredVolumes {
			volTotal, volUsed, err := fetchResourceMonitoredVolume(resourcesFetcher, volume)
			if err != nil {
				logger.Error(ctx, "failed to fetch volume", slog.Error(err))
				return nil
			}

			volumes = append(volumes, &resourcesMonitor_VolumeDatapoint{
				Path:  volume,
				Total: volTotal,
				Used:  volUsed,
			})
		}

		datapointsQueue.Push(resourcesMonitor_Datapoint{
			Memory: &resourcesMonitor_MemoryDatapoint{
				Total: memTotal,
				Used:  memUsed,
			},
			Volumes: volumes,
		})

		if datapointsQueue.IsFull() {
			_, err = aAPI.PushResourcesMonitoringUsage(ctx, &proto.PushResourcesMonitoringUsageRequest{
				Datapoints: datapointsQueue.ItemsAsProto(),
			})
			if err != nil {
				logger.Error(ctx, "failed to push resources monitoring usage", slog.Error(err))
			}
		}

		return nil
	}, "resources_monitor")

	return nil
}

func fetchResourceMonitoredMemory(fetcher *clistat.Statter) (int64, int64, error) {
	mem, err := fetcher.HostMemory(clistat.PrefixMebi)
	if err != nil {
		return 0, 0, err
	}

	var memTotal, memUsed int64
	if mem.Total == nil {
		return 0, 0, xerrors.New("memory total is nil - can not fetch memory")
	}

	memTotal = bytesToMegabytes(int64(*mem.Total))
	memUsed = bytesToMegabytes(int64(mem.Used))

	return memTotal, memUsed, nil
}

func fetchResourceMonitoredVolume(fetcher *clistat.Statter, volume string) (int64, int64, error) {
	vol, err := fetcher.Disk(clistat.PrefixMebi, volume)
	if err != nil {
		return 0, 0, err
	}

	var volTotal, volUsed int64
	if vol.Total == nil {
		return 0, 0, xerrors.New("volume total is nil - can not fetch volume")
	}

	volTotal = bytesToMegabytes(int64(*vol.Total))
	volUsed = bytesToMegabytes(int64(vol.Used))

	return volTotal, volUsed, nil
}

func bytesToMegabytes(bytes int64) int64 {
	return bytes / (1024 * 1024)
}
