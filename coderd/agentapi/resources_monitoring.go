package agentapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi/resourcesmonitor"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/quartz"
)

type VolumeNotFoundError struct {
	Volume string
}

func (e VolumeNotFoundError) Error() string {
	return fmt.Sprintf("volume not found: `%s`", e.Volume)
}

type ResourcesMonitoringAPI struct {
	AgentID     uuid.UUID
	WorkspaceID uuid.UUID

	Log                   slog.Logger
	Clock                 quartz.Clock
	Database              database.Store
	NotificationsEnqueuer notifications.Enqueuer

	Debounce time.Duration

	// How many datapoints in a row are required to
	// put the monitor in an alert state.
	ConsecutiveNOKsToAlert int

	// How many datapoints in total are required to
	// put the monitor in an alert state.
	MinimumNOKsToAlert int
}

func (a *ResourcesMonitoringAPI) GetResourcesMonitoringConfiguration(ctx context.Context, _ *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
	memoryMonitor, memoryErr := a.Database.FetchMemoryResourceMonitorsByAgentID(ctx, a.AgentID)
	if memoryErr != nil && !errors.Is(memoryErr, sql.ErrNoRows) {
		return nil, xerrors.Errorf("failed to fetch memory resource monitor: %w", memoryErr)
	}

	volumeMonitors, err := a.Database.FetchVolumesResourceMonitorsByAgentID(ctx, a.AgentID)
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch volume resource monitors: %w", err)
	}

	return &proto.GetResourcesMonitoringConfigurationResponse{
		Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
			CollectionIntervalSeconds: 10,
			NumDatapoints:             20,
		},
		Memory: func() *proto.GetResourcesMonitoringConfigurationResponse_Memory {
			if memoryErr != nil {
				return nil
			}

			return &proto.GetResourcesMonitoringConfigurationResponse_Memory{
				Enabled: memoryMonitor.Enabled,
			}
		}(),
		Volumes: func() []*proto.GetResourcesMonitoringConfigurationResponse_Volume {
			volumes := make([]*proto.GetResourcesMonitoringConfigurationResponse_Volume, 0, len(volumeMonitors))
			for _, monitor := range volumeMonitors {
				volumes = append(volumes, &proto.GetResourcesMonitoringConfigurationResponse_Volume{
					Enabled: monitor.Enabled,
					Path:    monitor.Path,
				})
			}

			return volumes
		}(),
	}, nil
}

func (a *ResourcesMonitoringAPI) PushResourcesMonitoringUsage(ctx context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
	if err := a.monitorMemory(ctx, req.Datapoints); err != nil {
		return nil, xerrors.Errorf("monitor memory: %w", err)
	}

	if err := a.monitorVolumes(ctx, req.Datapoints); err != nil {
		return nil, xerrors.Errorf("monitor volumes: %w", err)
	}

	return &proto.PushResourcesMonitoringUsageResponse{}, nil
}

func (a *ResourcesMonitoringAPI) monitorMemory(ctx context.Context, datapoints []*proto.PushResourcesMonitoringUsageRequest_Datapoint) error {
	monitor, err := a.Database.FetchMemoryResourceMonitorsByAgentID(ctx, a.AgentID)
	if err != nil {
		// It is valid for an agent to not have a memory monitor, so we
		// do not want to treat it as an error.
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}

		return xerrors.Errorf("fetch memory resource monitor: %w", err)
	}

	if !monitor.Enabled {
		return nil
	}

	usageDatapoints := make([]*proto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage, 0, len(datapoints))
	for _, datapoint := range datapoints {
		usageDatapoints = append(usageDatapoints, datapoint.Memory)
	}

	usageStates := resourcesmonitor.CalculateMemoryUsageStates(monitor, usageDatapoints)

	oldState := monitor.State
	newState := a.calculateNextState(oldState, usageStates)

	shouldNotify := a.Clock.Now().After(monitor.DebouncedUntil) &&
		oldState == database.WorkspaceAgentMonitorStateOK &&
		newState == database.WorkspaceAgentMonitorStateNOK

	debouncedUntil := monitor.DebouncedUntil
	if shouldNotify {
		debouncedUntil = a.Clock.Now().Add(a.Debounce)
	}

	err = a.Database.UpdateMemoryResourceMonitor(dbauthz.AsResourceMonitor(ctx), database.UpdateMemoryResourceMonitorParams{
		AgentID:        a.AgentID,
		State:          newState,
		UpdatedAt:      dbtime.Time(a.Clock.Now()),
		DebouncedUntil: dbtime.Time(debouncedUntil),
	})
	if err != nil {
		return xerrors.Errorf("update workspace monitor: %w", err)
	}

	if shouldNotify {
		workspace, err := a.Database.GetWorkspaceByID(ctx, a.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("get workspace by id: %w", err)
		}

		_, err = a.NotificationsEnqueuer.Enqueue(
			// nolint:gocritic // We need to be able to send the notification.
			dbauthz.AsNotifier(ctx),
			workspace.OwnerID,
			notifications.TemplateWorkspaceOutOfMemory,
			map[string]string{
				"workspace": workspace.Name,
				"threshold": fmt.Sprintf("%d%%", monitor.Threshold),
			},
			"workspace-monitor-memory",
		)
		if err != nil {
			return xerrors.Errorf("notify workspace OOM: %w", err)
		}
	}

	return nil
}

func (a *ResourcesMonitoringAPI) monitorVolumes(ctx context.Context, datapoints []*proto.PushResourcesMonitoringUsageRequest_Datapoint) error {
	volumeMonitors, err := a.Database.FetchVolumesResourceMonitorsByAgentID(ctx, a.AgentID)
	if err != nil {
		return xerrors.Errorf("get or insert volume monitor: %w", err)
	}

	volumes := make(map[string][]*proto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage)
	for _, datapoint := range datapoints {
		for _, volume := range datapoint.Volumes {
			volumeDatapoints := volumes[volume.Volume]
			volumeDatapoints = append(volumeDatapoints, volume)
			volumes[volume.Volume] = volumeDatapoints
		}
	}

	outOfDiskVolumes := make([]map[string]any, 0)

	for _, monitor := range volumeMonitors {
		if !monitor.Enabled {
			continue
		}

		datapoints, found := volumes[monitor.Path]
		if !found {
			return VolumeNotFoundError{Volume: monitor.Path}
		}

		usageStates := resourcesmonitor.CalculateVolumeUsageStates(monitor, datapoints)

		oldState := monitor.State
		newState := a.calculateNextState(oldState, usageStates)

		shouldNotify := a.Clock.Now().After(monitor.DebouncedUntil) &&
			oldState == database.WorkspaceAgentMonitorStateOK &&
			newState == database.WorkspaceAgentMonitorStateNOK

		debouncedUntil := monitor.DebouncedUntil
		if shouldNotify {
			debouncedUntil = a.Clock.Now().Add(a.Debounce)

			outOfDiskVolumes = append(outOfDiskVolumes, map[string]any{
				"path":      monitor.Path,
				"threshold": fmt.Sprintf("%d%%", monitor.Threshold),
			})
		}

		if err := a.Database.UpdateVolumeResourceMonitor(dbauthz.AsResourceMonitor(ctx), database.UpdateVolumeResourceMonitorParams{
			AgentID:        a.AgentID,
			Path:           monitor.Path,
			State:          newState,
			UpdatedAt:      dbtime.Time(a.Clock.Now()),
			DebouncedUntil: dbtime.Time(debouncedUntil),
		}); err != nil {
			return xerrors.Errorf("update workspace monitor: %w", err)
		}
	}

	if len(outOfDiskVolumes) != 0 {
		workspace, err := a.Database.GetWorkspaceByID(ctx, a.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("get workspace by id: %w", err)
		}

		if _, err := a.NotificationsEnqueuer.EnqueueWithData(
			// nolint:gocritic // We need to be able to send the notification.
			dbauthz.AsNotifier(ctx),
			workspace.OwnerID,
			notifications.TemplateWorkspaceOutOfDisk,
			map[string]string{
				"workspace": workspace.Name,
			},
			map[string]any{
				"volumes": outOfDiskVolumes,
			},
			"workspace-monitor-volumes",
		); err != nil {
			return xerrors.Errorf("notify workspace OOD: %w", err)
		}
	}

	return nil
}

func (a *ResourcesMonitoringAPI) calculateNextState(
	oldState database.WorkspaceAgentMonitorState,
	states []resourcesmonitor.State,
) database.WorkspaceAgentMonitorState {
	// If there are enough consecutive NOK states, we should be in an
	// alert state.
	consecutiveNOKs := resourcesmonitor.CalculateConsecutiveNOK(states)
	if consecutiveNOKs >= a.ConsecutiveNOKsToAlert {
		return database.WorkspaceAgentMonitorStateNOK
	}

	nokCount, okCount := 0, 0
	for _, state := range states {
		switch state {
		case resourcesmonitor.StateOK:
			okCount += 1
		case resourcesmonitor.StateNOK:
			nokCount += 1
		}
	}

	// If there are enough NOK datapoints, we should be in an alert state.
	if nokCount >= a.MinimumNOKsToAlert {
		return database.WorkspaceAgentMonitorStateNOK
	}

	// If all datapoints are OK, we should be in an OK state
	if okCount == len(states) {
		return database.WorkspaceAgentMonitorStateOK
	}

	// Otherwise we stay in the same state as last.
	return oldState
}
