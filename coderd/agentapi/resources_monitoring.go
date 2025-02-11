package agentapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/quartz"
)

type ResourcesMonitoringAPI struct {
	AgentID     uuid.UUID
	WorkspaceID uuid.UUID

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

func (m *ResourcesMonitoringAPI) PushResourcesMonitoringUsage(ctx context.Context, req *agentproto.PushResourcesMonitoringUsageRequest) (*agentproto.PushResourcesMonitoringUsageResponse, error) {
	if err := m.monitorMemory(ctx, req.Datapoints); err != nil {
		return nil, xerrors.Errorf("monitor memory: %w", err)
	}

	if err := m.monitorVolumes(ctx, req.Datapoints); err != nil {
		return nil, xerrors.Errorf("monitor volumes: %w", err)
	}

	return &agentproto.PushResourcesMonitoringUsageResponse{}, nil
}

func (m *ResourcesMonitoringAPI) monitorMemory(ctx context.Context, datapoints []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint) error {
	monitor, err := m.Database.FetchMemoryResourceMonitorsByAgentID(ctx, m.AgentID)
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
	newState := m.calculateNextState(oldState, usageStates)

	shouldNotify := m.Clock.Now().After(monitor.DebouncedUntil) &&
		oldState == database.WorkspaceAgentMonitorStateOK &&
		newState == database.WorkspaceAgentMonitorStateNOK

	debouncedUntil := monitor.DebouncedUntil
	if shouldNotify {
		debouncedUntil = m.Clock.Now().Add(m.Debounce)
	}

	err = m.Database.UpdateMemoryResourceMonitor(ctx, database.UpdateMemoryResourceMonitorParams{
		AgentID:        m.AgentID,
		State:          newState,
		UpdatedAt:      dbtime.Time(m.Clock.Now()),
		DebouncedUntil: dbtime.Time(debouncedUntil),
	})
	if err != nil {
		return xerrors.Errorf("update workspace monitor: %w", err)
	}

	if shouldNotify {
		workspace, err := m.Database.GetWorkspaceByID(ctx, m.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("get workspace by id: %w", err)
		}

		_, err = m.NotificationsEnqueuer.Enqueue(
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

func (m *ResourcesMonitoringAPI) monitorVolumes(ctx context.Context, datapoints []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint) error {
	volumeMonitors, err := m.Database.FetchVolumesResourceMonitorsByAgentID(ctx, m.AgentID)
	if err != nil {
		return xerrors.Errorf("get or insert volume monitor: %w", err)
	}

	volumes := make(map[string][]*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage)
	for _, datapoint := range datapoints {
		for _, volume := range datapoint.Volume {
			volumeDatapoints := volumes[volume.Path]
			volumeDatapoints = append(volumeDatapoints, volume)
			volumes[volume.Path] = volumeDatapoints
		}
	}

	outOfDiskVolumes := make([]map[string]any, 0)

	for _, monitor := range volumeMonitors {
		if !monitor.Enabled {
			continue
		}

		usageStates := calculateVolumeUsageStates(monitor, volumes[monitor.Path])

		oldState := monitor.State
		newState := m.calculateNextState(oldState, usageStates)

		shouldNotify := m.Clock.Now().After(monitor.DebouncedUntil) &&
			oldState == database.WorkspaceAgentMonitorStateOK &&
			newState == database.WorkspaceAgentMonitorStateNOK

		debouncedUntil := monitor.DebouncedUntil
		if shouldNotify {
			debouncedUntil = m.Clock.Now().Add(m.Debounce)

			outOfDiskVolumes = append(outOfDiskVolumes, map[string]any{
				"path":      monitor.Path,
				"threshold": fmt.Sprintf("%d%%", monitor.Threshold),
			})
		}

		if err := m.Database.UpdateVolumeResourceMonitor(ctx, database.UpdateVolumeResourceMonitorParams{
			AgentID:        m.AgentID,
			Path:           monitor.Path,
			State:          newState,
			UpdatedAt:      dbtime.Time(m.Clock.Now()),
			DebouncedUntil: dbtime.Time(debouncedUntil),
		}); err != nil {
			return xerrors.Errorf("update workspace monitor: %w", err)
		}
	}

	if len(outOfDiskVolumes) != 0 {
		workspace, err := m.Database.GetWorkspaceByID(ctx, m.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("get workspace by id: %w", err)
		}

		if _, err := m.NotificationsEnqueuer.EnqueueWithData(
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
		percent := int32(float64(datapoint.SpaceUsed) / float64(datapoint.SpaceTotal) * 100)

		state := database.WorkspaceAgentMonitorStateOK
		if percent >= monitor.Threshold {
			state = database.WorkspaceAgentMonitorStateNOK
		}

		states = append(states, state)
	}

	return states
}
