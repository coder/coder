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
	"github.com/coder/coder/v2/coderd/alerts"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/quartz"
)

type ResourcesMonitoringAPI struct {
	AgentID     uuid.UUID
	WorkspaceID uuid.UUID

	Log                   slog.Logger
	Clock                 quartz.Clock
	Database              database.Store
	NotificationsEnqueuer alerts.Enqueuer

	Debounce time.Duration
	Config   resourcesmonitor.Config

	// Cache resource monitors on first call to avoid millions of DB queries per day.
	memoryMonitor  database.WorkspaceAgentMemoryResourceMonitor
	volumeMonitors []database.WorkspaceAgentVolumeResourceMonitor
	monitorsLock   sync.RWMutex
}

// InitMonitors fetches resource monitors from the database and caches them.
// This must be called once after creating a ResourcesMonitoringAPI, the context should be
// the agent per-RPC connection context. If fetching fails with a real error (not sql.ErrNoRows), the
// connection should be torn down.
func (a *ResourcesMonitoringAPI) InitMonitors(ctx context.Context) error {
	memMon, err := a.Database.FetchMemoryResourceMonitorsByAgentID(ctx, a.AgentID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("fetch memory resource monitor: %w", err)
	}
	// If sql.ErrNoRows, memoryMonitor stays as zero value (CreatedAt.IsZero() = true).
	// Otherwise, store the fetched monitor.
	if err == nil {
		a.memoryMonitor = memMon
	}

	volMons, err := a.Database.FetchVolumesResourceMonitorsByAgentID(ctx, a.AgentID)
	if err != nil {
		return xerrors.Errorf("fetch volume resource monitors: %w", err)
	}
	// 0 length is valid, indicating none configured, since the volume monitors in the DB can be many.
	a.volumeMonitors = volMons

	return nil
}

func (a *ResourcesMonitoringAPI) GetResourcesMonitoringConfiguration(_ context.Context, _ *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
	return &proto.GetResourcesMonitoringConfigurationResponse{
		Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
			CollectionIntervalSeconds: int32(a.Config.CollectionInterval.Seconds()),
			NumDatapoints:             a.Config.NumDatapoints,
		},
		Memory: func() *proto.GetResourcesMonitoringConfigurationResponse_Memory {
			if a.memoryMonitor.CreatedAt.IsZero() {
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

	// Lock for the entire push operation since calls are sequential from the agent
	a.monitorsLock.Lock()
	defer a.monitorsLock.Unlock()

	if memoryErr := a.monitorMemory(ctx, req.Datapoints); memoryErr != nil {
		err = errors.Join(err, xerrors.Errorf("monitor memory: %w", memoryErr))
	}

	if volumeErr := a.monitorVolumes(ctx, req.Datapoints); volumeErr != nil {
		err = errors.Join(err, xerrors.Errorf("monitor volume: %w", volumeErr))
	}

	return &proto.PushResourcesMonitoringUsageResponse{}, err
}

func (a *ResourcesMonitoringAPI) monitorMemory(ctx context.Context, datapoints []*proto.PushResourcesMonitoringUsageRequest_Datapoint) error {
	if !a.memoryMonitor.Enabled {
		return nil
	}

	usageDatapoints := make([]*proto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage, 0, len(datapoints))
	for _, datapoint := range datapoints {
		usageDatapoints = append(usageDatapoints, datapoint.Memory)
	}

	usageStates := resourcesmonitor.CalculateMemoryUsageStates(a.memoryMonitor, usageDatapoints)

	oldState := a.memoryMonitor.State
	newState := resourcesmonitor.NextState(a.Config, oldState, usageStates)

	debouncedUntil, shouldNotify := a.memoryMonitor.Debounce(a.Debounce, a.Clock.Now(), oldState, newState)

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
	a.memoryMonitor.State = newState
	a.memoryMonitor.DebouncedUntil = dbtime.Time(debouncedUntil)
	a.memoryMonitor.UpdatedAt = dbtime.Time(a.Clock.Now())

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
		alerts.TemplateWorkspaceOutOfMemory,
		map[string]string{
			"workspace": workspace.Name,
			"threshold": fmt.Sprintf("%d%%", a.memoryMonitor.Threshold),
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
	outOfDiskVolumes := make([]map[string]any, 0)

	for i, monitor := range a.volumeMonitors {
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
		a.volumeMonitors[i].State = newState
		a.volumeMonitors[i].DebouncedUntil = dbtime.Time(debouncedUntil)
		a.volumeMonitors[i].UpdatedAt = dbtime.Time(a.Clock.Now())
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
		alerts.TemplateWorkspaceOutOfDisk,
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
