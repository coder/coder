package agentapi

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/quartz"
)

type WorkspaceMonitorAPI struct {
	WorkspaceID uuid.UUID

	Clock                 quartz.Clock
	Database              database.Store
	NotificationsEnqueuer notifications.Enqueuer

	MemoryMonitorEnabled  bool
	MemoryUsageThreshold  int32
	VolumeUsageThresholds map[string]int32

	Debounce time.Duration

	// How many datapoints in a row are required to
	// put the monitor in an alert state.
	ConsecutiveNOKs int

	// How many datapoints in total are required to
	// put the monitor in an alert state.
	MinimumNOKs int
}

func (m *WorkspaceMonitorAPI) UpdateWorkspaceMonitor(ctx context.Context, req *agentproto.WorkspaceMonitorUpdateRequest) (*agentproto.WorkspaceMonitorUpdateResponse, error) {
	res := &agentproto.WorkspaceMonitorUpdateResponse{}

	if m.MemoryMonitorEnabled {
		if err := m.monitorMemory(ctx, req.Datapoints); err != nil {
			return nil, xerrors.Errorf("monitor memory: %w", err)
		}
	}

	if err := m.monitorVolumes(ctx, req.Datapoints); err != nil {
		return nil, xerrors.Errorf("monitor volumes: %w", err)
	}

	return res, nil
}

func (m *WorkspaceMonitorAPI) monitorMemory(ctx context.Context, datapoints []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint) error {
	memoryMonitor, err := m.getOrInsertMemoryMonitor(ctx)
	if err != nil {
		return xerrors.Errorf("get or insert memory monitor: %w", err)
	}

	memoryUsageDatapoints := make([]*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_MemoryUsage, 0, len(datapoints))
	for _, datapoint := range datapoints {
		memoryUsageDatapoints = append(memoryUsageDatapoints, datapoint.Memory)
	}

	memoryUsageStates := m.calculateMemoryUsageStates(memoryUsageDatapoints)

	oldState := memoryMonitor.State
	newState := m.nextState(oldState, memoryUsageStates)
	shouldNotify := oldState == database.WorkspaceMonitorStateOK && newState == database.WorkspaceMonitorStateNOK

	var debouncedUntil = m.Clock.Now()
	if shouldNotify {
		debouncedUntil = debouncedUntil.Add(m.Debounce)
	}

	err = m.Database.UpdateWorkspaceMonitor(ctx, database.UpdateWorkspaceMonitorParams{
		WorkspaceID:    m.WorkspaceID,
		MonitorType:    database.WorkspaceMonitorTypeMemory,
		VolumePath:     sql.NullString{Valid: false},
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
				"threshold": fmt.Sprintf("%d%%", m.MemoryUsageThreshold),
			},
			"workspace-monitor-memory",
		)
		if err != nil {
			return xerrors.Errorf("notify workspace OOM: %w", err)
		}
	}

	return nil
}

func (m *WorkspaceMonitorAPI) getOrInsertMemoryMonitor(ctx context.Context) (database.WorkspaceMonitor, error) {
	memoryMonitor, err := m.Database.GetWorkspaceMonitor(ctx, database.GetWorkspaceMonitorParams{
		WorkspaceID: m.WorkspaceID,
		MonitorType: database.WorkspaceMonitorTypeMemory,
	})
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return m.Database.InsertWorkspaceMonitor(
				ctx,
				database.InsertWorkspaceMonitorParams{
					WorkspaceID:    m.WorkspaceID,
					MonitorType:    database.WorkspaceMonitorTypeMemory,
					VolumePath:     sql.NullString{Valid: false},
					State:          database.WorkspaceMonitorStateOK,
					CreatedAt:      dbtime.Now(),
					UpdatedAt:      dbtime.Now(),
					DebouncedUntil: dbtime.Now(),
				},
			)
		}

		return database.WorkspaceMonitor{}, err
	}

	return memoryMonitor, nil
}

func (m *WorkspaceMonitorAPI) monitorVolumes(ctx context.Context, datapoints []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint) error {
	volumes := make(map[string][]*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_VolumeUsage)

	for _, datapoint := range datapoints {
		for _, volume := range datapoint.Volume {
			volumeDatapoints := volumes[volume.Path]
			volumeDatapoints = append(volumeDatapoints, volume)
			volumes[volume.Path] = volumeDatapoints
		}
	}

	for path, volume := range volumes {
		if err := m.monitorVolume(ctx, path, volume); err != nil {
			return xerrors.Errorf("monitor volume: %w", err)
		}
	}

	return nil
}

func (m *WorkspaceMonitorAPI) monitorVolume(ctx context.Context, path string, datapoints []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_VolumeUsage) error {
	volumeMonitor, err := m.getOrInsertVolumeMonitor(ctx, path)
	if err != nil {
		return xerrors.Errorf("get or insert volume monitor: %w", err)
	}

	volumeUsageStates := m.calculateVolumeUsageStates(path, datapoints)

	oldState := volumeMonitor.State
	newState := m.nextState(oldState, volumeUsageStates)
	shouldNotify := oldState == database.WorkspaceMonitorStateOK && newState == database.WorkspaceMonitorStateNOK

	var debouncedUntil = m.Clock.Now()
	if shouldNotify {
		debouncedUntil = debouncedUntil.Add(m.Debounce)
	}

	err = m.Database.UpdateWorkspaceMonitor(ctx, database.UpdateWorkspaceMonitorParams{
		WorkspaceID:    m.WorkspaceID,
		MonitorType:    database.WorkspaceMonitorTypeVolume,
		VolumePath:     sql.NullString{Valid: true, String: path},
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
			notifications.TemplateWorkspaceOutOfDisk,
			map[string]string{
				"workspace": workspace.Name,
				"threshold": fmt.Sprintf("%d%%", m.VolumeUsageThresholds[path]),
				"volume":    path,
			},
			"workspace-monitor-memory",
		)
		if err != nil {
			return xerrors.Errorf("notify workspace OOM: %w", err)
		}
	}

	return nil
}

func (m *WorkspaceMonitorAPI) getOrInsertVolumeMonitor(ctx context.Context, path string) (database.WorkspaceMonitor, error) {
	memoryMonitor, err := m.Database.GetWorkspaceMonitor(ctx, database.GetWorkspaceMonitorParams{
		WorkspaceID: m.WorkspaceID,
		MonitorType: database.WorkspaceMonitorTypeVolume,
		VolumePath:  sql.NullString{Valid: true, String: path},
	})
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return m.Database.InsertWorkspaceMonitor(
				ctx,
				database.InsertWorkspaceMonitorParams{
					WorkspaceID:    m.WorkspaceID,
					MonitorType:    database.WorkspaceMonitorTypeVolume,
					VolumePath:     sql.NullString{Valid: true, String: path},
					State:          database.WorkspaceMonitorStateOK,
					CreatedAt:      dbtime.Now(),
					UpdatedAt:      dbtime.Now(),
					DebouncedUntil: dbtime.Now(),
				},
			)
		}

		return database.WorkspaceMonitor{}, err
	}

	return memoryMonitor, nil
}

func (m *WorkspaceMonitorAPI) nextState(oldState database.WorkspaceMonitorState, states []database.WorkspaceMonitorState) database.WorkspaceMonitorState {
	// If we do not have an OK in the last `X` datapoints, then we are
	// in an alert state.
	lastXStates := states[len(states)-m.ConsecutiveNOKs:]
	if !slices.Contains(lastXStates, database.WorkspaceMonitorStateOK) {
		return database.WorkspaceMonitorStateNOK
	}

	nokCount := 0
	for _, state := range states {
		if state == database.WorkspaceMonitorStateNOK {
			nokCount++
		}
	}

	// If there are enough NOK datapoints, we should be in an alert state.
	if nokCount >= m.MinimumNOKs {
		return database.WorkspaceMonitorStateNOK
	}

	// If there are no NOK datapoints, we should be in an OK state.
	if nokCount == 0 {
		return database.WorkspaceMonitorStateOK
	}

	// Otherwise we stay in the same state as last.
	return oldState
}

func (m *WorkspaceMonitorAPI) calculateMemoryUsageStates(datapoints []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_MemoryUsage) []database.WorkspaceMonitorState {
	states := make([]database.WorkspaceMonitorState, 0, len(datapoints))

	for _, datapoint := range datapoints {
		percent := int32(float64(datapoint.Used) / float64(datapoint.Total) * 100)

		state := database.WorkspaceMonitorStateOK
		if percent >= m.MemoryUsageThreshold {
			state = database.WorkspaceMonitorStateNOK
		}

		states = append(states, state)
	}

	return states
}

func (m *WorkspaceMonitorAPI) calculateVolumeUsageStates(path string, datapoints []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_VolumeUsage) []database.WorkspaceMonitorState {
	states := make([]database.WorkspaceMonitorState, 0, len(datapoints))

	for _, datapoint := range datapoints {
		percent := int32(float64(datapoint.Used) / float64(datapoint.Total) * 100)

		state := database.WorkspaceMonitorStateOK
		if percent >= m.VolumeUsageThresholds[path] {
			state = database.WorkspaceMonitorStateNOK
		}

		states = append(states, state)
	}

	return states
}
