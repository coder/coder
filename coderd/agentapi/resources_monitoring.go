package agentapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"cdr.dev/slog"
	"golang.org/x/xerrors"

	"github.com/google/uuid"

	agentproto "github.com/coder/coder/v2/agent/proto"
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

func (a *ResourcesMonitoringAPI) PushResourcesMonitoringUsage(ctx context.Context, req *agentproto.PushResourcesMonitoringUsageRequest) (*agentproto.PushResourcesMonitoringUsageResponse, error) {
	if err := a.monitorMemory(ctx, req.Datapoints); err != nil {
		return nil, xerrors.Errorf("monitor memory: %w", err)
	}

	if err := a.monitorVolumes(ctx, req.Datapoints); err != nil {
		return nil, xerrors.Errorf("monitor volumes: %w", err)
	}

	return &agentproto.PushResourcesMonitoringUsageResponse{}, nil
}

func (a *ResourcesMonitoringAPI) monitorMemory(ctx context.Context, datapoints []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint) error {
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

	usageDatapoints := make([]*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage, 0, len(datapoints))
	for _, datapoint := range datapoints {
		usageDatapoints = append(usageDatapoints, datapoint.Memory)
	}

	usageStates := calculateMemoryUsageStates(monitor, usageDatapoints)

	oldState := monitor.State
	newState := a.calculateNextState(oldState, usageStates)

	shouldNotify := a.Clock.Now().After(monitor.DebouncedUntil) &&
		oldState == database.WorkspaceAgentMonitorStateOK &&
		newState == database.WorkspaceAgentMonitorStateNOK

	debouncedUntil := monitor.DebouncedUntil
	if shouldNotify {
		debouncedUntil = a.Clock.Now().Add(a.Debounce)
	}

	err = a.Database.UpdateMemoryResourceMonitor(ctx, database.UpdateMemoryResourceMonitorParams{
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

func (a *ResourcesMonitoringAPI) monitorVolumes(ctx context.Context, datapoints []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint) error {
	volumeMonitors, err := a.Database.FetchVolumesResourceMonitorsByAgentID(ctx, a.AgentID)
	if err != nil {
		return xerrors.Errorf("get or insert volume monitor: %w", err)
	}

	volumes := make(map[string][]*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage)
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

		usageStates := calculateVolumeUsageStates(monitor, datapoints)

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

		if err := a.Database.UpdateVolumeResourceMonitor(ctx, database.UpdateVolumeResourceMonitorParams{
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

func (m *ResourcesMonitoringAPI) calculateNextState(
	oldState database.WorkspaceAgentMonitorState,
	states []database.WorkspaceAgentMonitorState,
) database.WorkspaceAgentMonitorState {
	// If we do not have an OK in the last `X` datapoints, then we are
	// in an alert state.
	lastXStates := states[max(len(states)-m.ConsecutiveNOKsToAlert, 0):]
	if !slices.Contains(lastXStates, database.WorkspaceAgentMonitorStateOK) {
		return database.WorkspaceAgentMonitorStateNOK
	}

	nokCount := 0
	for _, state := range states {
		if state == database.WorkspaceAgentMonitorStateNOK {
			nokCount++
		}
	}

	// If there are enough NOK datapoints, we should be in an alert state.
	if nokCount >= m.MinimumNOKsToAlert {
		return database.WorkspaceAgentMonitorStateNOK
	}

	// If there are no NOK datapoints, we should be in an OK state.
	if nokCount == 0 {
		return database.WorkspaceAgentMonitorStateOK
	}

	// Otherwise we stay in the same state as last.
	return oldState
}

func calculateMemoryUsageStates(
	monitor database.WorkspaceAgentMemoryResourceMonitor,
	datapoints []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage,
) []database.WorkspaceAgentMonitorState {
	states := make([]database.WorkspaceAgentMonitorState, 0, len(datapoints))

	for _, datapoint := range datapoints {
		percent := int32(float64(datapoint.Used) / float64(datapoint.Total) * 100)

		state := database.WorkspaceAgentMonitorStateOK
		if percent >= monitor.Threshold {
			state = database.WorkspaceAgentMonitorStateNOK
		}

		states = append(states, state)
	}

	return states
}

func calculateVolumeUsageStates(
	monitor database.WorkspaceAgentVolumeResourceMonitor,
	datapoints []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage,
) []database.WorkspaceAgentMonitorState {
	states := make([]database.WorkspaceAgentMonitorState, 0, len(datapoints))

	for _, datapoint := range datapoints {
		percent := int32(float64(datapoint.Used) / float64(datapoint.Total) * 100)

		state := database.WorkspaceAgentMonitorStateOK
		if percent >= monitor.Threshold {
			state = database.WorkspaceAgentMonitorStateNOK
		}

		states = append(states, state)
	}

	return states
}
