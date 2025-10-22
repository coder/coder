package agentapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
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

type ResourcesMonitoringAPI struct {
	AgentID     uuid.UUID
	WorkspaceID uuid.UUID

	Log                   slog.Logger
	Clock                 quartz.Clock
	Database              database.Store
	NotificationsEnqueuer notifications.Enqueuer

	Debounce time.Duration
	Config   resourcesmonitor.Config

	// Cache resource monitors on first call to avoid millions of DB queries per day.
	memOnce sync.Once
	volOnce sync.Once
	memoryMonitor *database.WorkspaceAgentMemoryResourceMonitor
	volumeMonitors []database.WorkspaceAgentVolumeResourceMonitor
	monitorsLock sync.RWMutex
}

// fetchMemoryMonitor fetches the memory monitor from the database and caches it.
// Returns an error if the fetch fails (except for sql.ErrNoRows which is expected).
func (a *ResourcesMonitoringAPI) fetchMemoryMonitor(ctx context.Context) error {
    memMon, err := a.Database.FetchMemoryResourceMonitorsByAgentID(ctx, a.AgentID)
    if err != nil && !errors.Is(err, sql.ErrNoRows) {
        return xerrors.Errorf("fetch memory resource monitor: %w", err)
    }
    if err == nil {
        a.memoryMonitor = &memMon
    }
    return nil
}

// fetchVolumeMonitors fetches the volume monitors from the database and caches them.
func (a *ResourcesMonitoringAPI) fetchVolumeMonitors(ctx context.Context) error {
    volMons, err := a.Database.FetchVolumesResourceMonitorsByAgentID(ctx, a.AgentID)
    if err != nil {
        return xerrors.Errorf("fetch volume resource monitors: %w", err)
    }
    a.volumeMonitors = volMons
    return nil
}

func (a *ResourcesMonitoringAPI) GetResourcesMonitoringConfiguration(ctx context.Context, _ *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
    // Load memory monitor once
    var memoryErr error
    a.memOnce.Do(func() {
        memoryErr = a.fetchMemoryMonitor(ctx)
    })
    if memoryErr != nil {
        return nil, memoryErr
    }

    // Load volume monitors once
    var volumeErr error
    a.volOnce.Do(func() {
        volumeErr = a.fetchVolumeMonitors(ctx)
    })
    if volumeErr != nil {
        return nil, volumeErr
    }

    a.monitorsLock.RLock()
    defer a.monitorsLock.RUnlock()

    return &proto.GetResourcesMonitoringConfigurationResponse{
        Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
            CollectionIntervalSeconds: int32(a.Config.CollectionInterval.Seconds()),
            NumDatapoints:             a.Config.NumDatapoints,
        },
        Memory: func() *proto.GetResourcesMonitoringConfigurationResponse_Memory {
            if a.memoryMonitor == nil {
                return nil
            }
            return &proto.GetResourcesMonitoringConfigurationResponse_Memory{
                Enabled: a.memoryMonitor.Enabled,
            }
        }(),
        Volumes: func() []*proto.GetResourcesMonitoringConfigurationResponse_Volume {
            volumes := make([]*proto.GetResourcesMonitoringConfigurationResponse_Volume, 0, len(a.volumeMonitors))
            for _, monitor := range a.volumeMonitors {
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
	var err error

	if memoryErr := a.monitorMemory(ctx, req.Datapoints); memoryErr != nil {
		err = errors.Join(err, xerrors.Errorf("monitor memory: %w", memoryErr))
	}

	if volumeErr := a.monitorVolumes(ctx, req.Datapoints); volumeErr != nil {
		err = errors.Join(err, xerrors.Errorf("monitor volume: %w", volumeErr))
	}

	return &proto.PushResourcesMonitoringUsageResponse{}, err
}

func (a *ResourcesMonitoringAPI) monitorMemory(ctx context.Context, datapoints []*proto.PushResourcesMonitoringUsageRequest_Datapoint) error {
	// Load monitor once
	var fetchErr error
	a.memOnce.Do(func() {
			fetchErr = a.fetchMemoryMonitor(ctx)
	})
	if fetchErr != nil {
			return fetchErr
	}

	a.monitorsLock.RLock()
	monitor := a.memoryMonitor
	a.monitorsLock.RUnlock()

	// No memory monitor configured
	if monitor == nil {
			return nil
	}

	if !monitor.Enabled {
		return nil
	}

	usageDatapoints := make([]*proto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage, 0, len(datapoints))
	for _, datapoint := range datapoints {
		usageDatapoints = append(usageDatapoints, datapoint.Memory)
	}

	usageStates := resourcesmonitor.CalculateMemoryUsageStates(*monitor, usageDatapoints)

	oldState := monitor.State
	newState := resourcesmonitor.NextState(a.Config, oldState, usageStates)

	debouncedUntil, shouldNotify := monitor.Debounce(a.Debounce, a.Clock.Now(), oldState, newState)

	//nolint:gocritic // We need to be able to update the resource monitor here.
	err := a.Database.UpdateMemoryResourceMonitor(dbauthz.AsResourceMonitor(ctx), database.UpdateMemoryResourceMonitorParams{
		AgentID:        a.AgentID,
		State:          newState,
		UpdatedAt:      dbtime.Time(a.Clock.Now()),
		DebouncedUntil: dbtime.Time(debouncedUntil),
	})
	if err != nil {
		return xerrors.Errorf("update workspace monitor: %w", err)
	}

	// Update cached state
	a.monitorsLock.Lock()
	a.memoryMonitor.State = newState
	a.memoryMonitor.DebouncedUntil = dbtime.Time(debouncedUntil)
	a.memoryMonitor.UpdatedAt = dbtime.Time(a.Clock.Now())
	a.monitorsLock.Unlock()

	if !shouldNotify {
		return nil
	}

	workspace, err := a.Database.GetWorkspaceByID(ctx, a.WorkspaceID)
	if err != nil {
		return xerrors.Errorf("get workspace by id: %w", err)
	}

	_, err = a.NotificationsEnqueuer.EnqueueWithData(
		// nolint:gocritic // We need to be able to send the notification.
		dbauthz.AsNotifier(ctx),
		workspace.OwnerID,
		notifications.TemplateWorkspaceOutOfMemory,
		map[string]string{
			"workspace": workspace.Name,
			"threshold": fmt.Sprintf("%d%%", monitor.Threshold),
		},
		map[string]any{
			// NOTE(DanielleMaywood):
			// When notifications are enqueued, they are checked to be
			// unique within a single day. This means that if we attempt
			// to send two OOM notifications for the same workspace on
			// the same day, the enqueuer will prevent us from sending
			// a second one. We are inject a timestamp to make the
			// notifications appear different enough to circumvent this
			// deduplication logic.
			"timestamp": a.Clock.Now(),
		},
		"workspace-monitor-memory",
		workspace.ID,
		workspace.OwnerID,
		workspace.OrganizationID,
	)
	if err != nil {
		return xerrors.Errorf("notify workspace OOM: %w", err)
	}

	return nil
}

func (a *ResourcesMonitoringAPI) monitorVolumes(ctx context.Context, datapoints []*proto.PushResourcesMonitoringUsageRequest_Datapoint) error {
	// Load monitors once
	var fetchErr error
	a.volOnce.Do(func() {
			fetchErr = a.fetchVolumeMonitors(ctx)
	})
	if fetchErr != nil {
			return fetchErr
	}

	a.monitorsLock.RLock()
	volumeMonitors := a.volumeMonitors
	a.monitorsLock.RUnlock()

	outOfDiskVolumes := make([]map[string]any, 0)

	for i, monitor := range volumeMonitors {
		if !monitor.Enabled {
			continue
		}

		usageDatapoints := make([]*proto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage, 0, len(datapoints))
		for _, datapoint := range datapoints {
			var usage *proto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage

			for _, volume := range datapoint.Volumes {
				if volume.Volume == monitor.Path {
					usage = volume
					break
				}
			}

			usageDatapoints = append(usageDatapoints, usage)
		}

		usageStates := resourcesmonitor.CalculateVolumeUsageStates(monitor, usageDatapoints)

		oldState := monitor.State
		newState := resourcesmonitor.NextState(a.Config, oldState, usageStates)

		debouncedUntil, shouldNotify := monitor.Debounce(a.Debounce, a.Clock.Now(), oldState, newState)

		if shouldNotify {
			outOfDiskVolumes = append(outOfDiskVolumes, map[string]any{
				"path":      monitor.Path,
				"threshold": fmt.Sprintf("%d%%", monitor.Threshold),
			})
		}

		//nolint:gocritic // We need to be able to update the resource monitor here.
		if err := a.Database.UpdateVolumeResourceMonitor(dbauthz.AsResourceMonitor(ctx), database.UpdateVolumeResourceMonitorParams{
			AgentID:        a.AgentID,
			Path:           monitor.Path,
			State:          newState,
			UpdatedAt:      dbtime.Time(a.Clock.Now()),
			DebouncedUntil: dbtime.Time(debouncedUntil),
		}); err != nil {
			return xerrors.Errorf("update workspace monitor: %w", err)
		}

		// Update cached state
		a.monitorsLock.Lock()
		a.volumeMonitors[i].State = newState
		a.volumeMonitors[i].DebouncedUntil = dbtime.Time(debouncedUntil)
		a.volumeMonitors[i].UpdatedAt = dbtime.Time(a.Clock.Now())
		a.monitorsLock.Unlock()
	}

	if len(outOfDiskVolumes) == 0 {
		return nil
	}

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
			// NOTE(DanielleMaywood):
			// When notifications are enqueued, they are checked to be
			// unique within a single day. This means that if we attempt
			// to send two OOM notifications for the same workspace on
			// the same day, the enqueuer will prevent us from sending
			// a second one. We are inject a timestamp to make the
			// notifications appear different enough to circumvent this
			// deduplication logic.
			"timestamp": a.Clock.Now(),
		},
		"workspace-monitor-volumes",
		workspace.ID,
		workspace.OwnerID,
		workspace.OrganizationID,
	); err != nil {
		return xerrors.Errorf("notify workspace OOD: %w", err)
	}

	return nil
}
