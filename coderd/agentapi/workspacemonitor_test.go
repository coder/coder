package agentapi_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/quartz"
)

func workspaceMonitorAPI(t *testing.T) (*agentapi.WorkspaceMonitorAPI, database.User, *quartz.Mock, *notificationstest.FakeEnqueuer) {
	t.Helper()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		OwnerID:        user.ID,
	})

	notifyEnq := &notificationstest.FakeEnqueuer{}
	clock := quartz.NewMock(t)

	return &agentapi.WorkspaceMonitorAPI{
		WorkspaceID:           workspace.ID,
		Clock:                 clock,
		Database:              db,
		NotificationsEnqueuer: notifyEnq,
	}, user, clock, notifyEnq
}

func TestWorkspaceMemoryMonitor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		memoryUsage      []int32
		memoryTotal      int32
		thresholdPercent int32
		minimumNOKs      int
		consecutiveNOKs  int
		previousState    database.WorkspaceMonitorState
		expectState      database.WorkspaceMonitorState
		shouldNotify     bool
	}{
		{
			name:             "WhenOK/NeverExceedsThreshold",
			memoryUsage:      []int32{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 2, 3, 1, 2},
			memoryTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceMonitorStateOK,
			expectState:      database.WorkspaceMonitorStateOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenOK/ConsecutiveExceedsThreshold",
			memoryUsage:      []int32{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 8, 9, 8, 9},
			memoryTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceMonitorStateOK,
			expectState:      database.WorkspaceMonitorStateNOK,
			shouldNotify:     true,
		},
		{
			name:             "WhenOK/MinimumExceedsThreshold",
			memoryUsage:      []int32{2, 8, 2, 9, 2, 8, 2, 9, 2, 8, 4, 9, 1, 8, 2, 8, 9},
			memoryTotal:      10,
			thresholdPercent: 80,
			minimumNOKs:      4,
			consecutiveNOKs:  10,
			previousState:    database.WorkspaceMonitorStateOK,
			expectState:      database.WorkspaceMonitorStateNOK,
			shouldNotify:     true,
		},
		{
			name:             "WhenNOK/NeverExceedsThreshold",
			memoryUsage:      []int32{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 2, 3, 1, 2},
			memoryTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceMonitorStateNOK,
			expectState:      database.WorkspaceMonitorStateOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenNOK/ConsecutiveExceedsThreshold",
			memoryUsage:      []int32{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 8, 9, 8, 9},
			memoryTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceMonitorStateNOK,
			expectState:      database.WorkspaceMonitorStateNOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenNOK/MinimumExceedsThreshold",
			memoryUsage:      []int32{2, 8, 2, 9, 2, 8, 2, 9, 2, 8, 4, 9, 1, 8, 2, 8, 9},
			memoryTotal:      10,
			thresholdPercent: 80,
			minimumNOKs:      4,
			consecutiveNOKs:  10,
			previousState:    database.WorkspaceMonitorStateNOK,
			expectState:      database.WorkspaceMonitorStateNOK,
			shouldNotify:     false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api, user, clock, notifyEnq := workspaceMonitorAPI(t)
			api.MinimumNOKs = tt.minimumNOKs
			api.ConsecutiveNOKs = tt.consecutiveNOKs
			api.MemoryMonitorEnabled = true
			api.MemoryUsageThreshold = tt.thresholdPercent

			datapoints := make([]*agentproto.WorkspaceMonitorUpdateRequest_Datapoint, 0, len(tt.memoryUsage))
			collectedAt := clock.Now()
			for _, usage := range tt.memoryUsage {
				collectedAt = collectedAt.Add(15 * time.Second)
				datapoints = append(datapoints, &agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
					CollectedAt: timestamppb.New(collectedAt),
					Memory: &agentproto.WorkspaceMonitorUpdateRequest_Datapoint_MemoryUsage{
						Used:  usage,
						Total: tt.memoryTotal,
					},
				})
			}

			dbgen.WorkspaceMonitor(t, api.Database, database.WorkspaceMonitor{
				WorkspaceID: api.WorkspaceID,
				MonitorType: database.WorkspaceMonitorTypeMemory,
				State:       tt.previousState,
			})

			clock.Set(collectedAt)
			_, err := api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
				Datapoints: datapoints,
			})
			require.NoError(t, err)

			sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
			if tt.shouldNotify {
				require.Len(t, sent, 1)
				require.Equal(t, user.ID, sent[0].UserID)
			} else {
				require.Len(t, sent, 0)
			}
		})
	}
}

func TestWorkspaceVolumeMonitor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		volumePath       string
		volumeUsage      []int32
		volumeTotal      int32
		thresholdPercent int32
		previousState    database.WorkspaceMonitorState
		expectState      database.WorkspaceMonitorState
		shouldNotify     bool
		minimumNOKs      int
		consecutiveNOKs  int
	}{
		{
			name:             "WhenOK/NeverExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int32{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 2, 3, 1, 2},
			volumeTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceMonitorStateOK,
			expectState:      database.WorkspaceMonitorStateOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenOK/ConsecutiveExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int32{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 8, 9, 8, 9},
			volumeTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceMonitorStateOK,
			expectState:      database.WorkspaceMonitorStateNOK,
			shouldNotify:     true,
		},
		{
			name:             "WhenOK/MinimumExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int32{2, 8, 2, 9, 2, 8, 2, 9, 2, 8, 4, 9, 1, 8, 2, 8, 9},
			volumeTotal:      10,
			thresholdPercent: 80,
			minimumNOKs:      4,
			consecutiveNOKs:  10,
			previousState:    database.WorkspaceMonitorStateOK,
			expectState:      database.WorkspaceMonitorStateNOK,
			shouldNotify:     true,
		},
		{
			name:             "WhenNOK/NeverExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int32{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 2, 3, 1, 2},
			volumeTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceMonitorStateNOK,
			expectState:      database.WorkspaceMonitorStateOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenNOK/ConsecutiveExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int32{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 8, 9, 8, 9},
			volumeTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceMonitorStateNOK,
			expectState:      database.WorkspaceMonitorStateNOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenNOK/MinimumExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int32{2, 8, 2, 9, 2, 8, 2, 9, 2, 8, 4, 9, 1, 8, 2, 8, 9},
			volumeTotal:      10,
			thresholdPercent: 80,
			minimumNOKs:      4,
			consecutiveNOKs:  10,
			previousState:    database.WorkspaceMonitorStateNOK,
			expectState:      database.WorkspaceMonitorStateNOK,
			shouldNotify:     false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api, user, clock, notifyEnq := workspaceMonitorAPI(t)
			api.MinimumNOKs = tt.minimumNOKs
			api.ConsecutiveNOKs = tt.consecutiveNOKs
			api.VolumeUsageThresholds = map[string]int32{
				tt.volumePath: tt.thresholdPercent,
			}

			datapoints := make([]*agentproto.WorkspaceMonitorUpdateRequest_Datapoint, 0, len(tt.volumeUsage))
			collectedAt := clock.Now()
			for _, volumeUsage := range tt.volumeUsage {
				collectedAt = collectedAt.Add(15 * time.Second)

				volumeDatapoints := []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_VolumeUsage{
					{
						Path:  tt.volumePath,
						Used:  volumeUsage,
						Total: tt.volumeTotal,
					},
				}

				datapoints = append(datapoints, &agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
					CollectedAt: timestamppb.New(collectedAt),
					Volume:      volumeDatapoints,
				})
			}

			dbgen.WorkspaceMonitor(t, api.Database, database.WorkspaceMonitor{
				WorkspaceID: api.WorkspaceID,
				MonitorType: database.WorkspaceMonitorTypeVolume,
				VolumePath:  sql.NullString{Valid: true, String: tt.volumePath},
				State:       tt.previousState,
			})

			clock.Set(collectedAt)
			_, err := api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
				Datapoints: datapoints,
			})
			require.NoError(t, err)

			sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
			if tt.shouldNotify {
				require.Len(t, sent, 1)
				require.Equal(t, user.ID, sent[0].UserID)
			} else {
				require.Len(t, sent, 0)
			}
		})
	}
}
