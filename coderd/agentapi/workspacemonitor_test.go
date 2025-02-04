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

func TestWorkspaceMemoryMonitorDebounce(t *testing.T) {
	t.Parallel()

	// This test is a bit of a long one. We're testing that
	// when a monitor goes into an alert state, it doesn't
	// allow another notification to occur until after the
	// debounce period.
	//
	// 1. OK -> NOK  |> sends a notification
	// 2. NOK -> OK  |> does nothing
	// 3. OK -> NOK  |> does nothing due to debounce period
	// 4. NOK -> OK  |> does nothing
	// 5. OK -> NOK  |> sends a notification as debounce period exceeded

	api, _, clock, notifyEnq := workspaceMonitorAPI(t)
	api.MinimumNOKs = 10
	api.ConsecutiveNOKs = 4
	api.MemoryMonitorEnabled = true
	api.MemoryUsageThreshold = 80
	api.Debounce = 1 * time.Minute

	// Given: A monitor in an OK state
	dbgen.WorkspaceMonitor(t, api.Database, database.WorkspaceMonitor{
		WorkspaceID: api.WorkspaceID,
		MonitorType: database.WorkspaceMonitorTypeMemory,
		State:       database.WorkspaceMonitorStateOK,
	})

	// When: The monitor is given a state that will trigger NOK
	api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
		Datapoints: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Memory: &agentproto.WorkspaceMonitorUpdateRequest_Datapoint_MemoryUsage{
					Used:  10,
					Total: 10,
				},
			},
		},
	})

	// Then: We expect there to be a notification sent
	sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
	require.Len(t, sent, 1)
	notifyEnq.Clear()

	// When: The monitor moves to an OK state from NOK
	clock.Advance(api.Debounce / 4)
	api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
		Datapoints: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Memory: &agentproto.WorkspaceMonitorUpdateRequest_Datapoint_MemoryUsage{
					Used:  1,
					Total: 10,
				},
			},
		},
	})

	// Then: We expect no new notifications
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When: The monitor moves back to a NOK state before the debounced time.
	clock.Advance(api.Debounce / 4)
	api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
		Datapoints: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Memory: &agentproto.WorkspaceMonitorUpdateRequest_Datapoint_MemoryUsage{
					Used:  10,
					Total: 10,
				},
			},
		},
	})

	// Then: We expect no new notifications (showing the debouncer working)
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When: The monitor moves back to an OK state from NOK
	clock.Advance(api.Debounce / 4)
	api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
		Datapoints: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Memory: &agentproto.WorkspaceMonitorUpdateRequest_Datapoint_MemoryUsage{
					Used:  1,
					Total: 10,
				},
			},
		},
	})

	// Then: We still expect no new notifications
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When: The monitor moves back to a NOK state after the debounce period.
	clock.Advance(api.Debounce/4 + 1*time.Second)
	api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
		Datapoints: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Memory: &agentproto.WorkspaceMonitorUpdateRequest_Datapoint_MemoryUsage{
					Used:  10,
					Total: 10,
				},
			},
		},
	})

	// Then: We expect a notification
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
	require.Len(t, sent, 1)
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

func TestWorkspaceVolumeMonitorDebounce(t *testing.T) {
	t.Parallel()

	// This test is a bit of a long one. We're testing that
	// when a monitor goes into an alert state, it doesn't
	// allow another notification to occur until after the
	// debounce period.
	//
	// 1. OK -> NOK  |> sends a notification
	// 2. NOK -> OK  |> does nothing
	// 3. OK -> NOK  |> does nothing due to debounce period
	// 4. NOK -> OK  |> does nothing
	// 5. OK -> NOK  |> sends a notification as debounce period exceeded

	volumePath := "/home/coder"

	api, _, clock, notifyEnq := workspaceMonitorAPI(t)
	api.MinimumNOKs = 10
	api.ConsecutiveNOKs = 4
	api.VolumeUsageThresholds = map[string]int32{
		volumePath: 80,
	}
	api.Debounce = 1 * time.Minute

	// Given: A monitor in an OK state
	dbgen.WorkspaceMonitor(t, api.Database, database.WorkspaceMonitor{
		WorkspaceID: api.WorkspaceID,
		MonitorType: database.WorkspaceMonitorTypeVolume,
		VolumePath:  sql.NullString{Valid: true, String: volumePath},
		State:       database.WorkspaceMonitorStateOK,
	})

	// When: The monitor is given a state that will trigger NOK
	api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
		Datapoints: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_VolumeUsage{
					{
						Path:  volumePath,
						Used:  10,
						Total: 10,
					},
				},
			},
		},
	})

	// Then: We expect there to be a notification sent
	sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 1)
	notifyEnq.Clear()

	// When: The monitor moves to an OK state from NOK
	clock.Advance(api.Debounce / 4)
	api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
		Datapoints: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_VolumeUsage{
					{
						Path:  volumePath,
						Used:  1,
						Total: 10,
					},
				},
			},
		},
	})

	// Then: We expect no new notifications
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When: The monitor moves back to a NOK state before the debounced time.
	clock.Advance(api.Debounce / 4)
	api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
		Datapoints: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_VolumeUsage{
					{
						Path:  volumePath,
						Used:  10,
						Total: 10,
					},
				},
			},
		},
	})

	// Then: We expect no new notifications (showing the debouncer working)
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When: The monitor moves back to an OK state from NOK
	clock.Advance(api.Debounce / 4)
	api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
		Datapoints: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_VolumeUsage{
					{
						Path:  volumePath,
						Used:  1,
						Total: 10,
					},
				},
			},
		},
	})

	// Then: We still expect no new notifications
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When: The monitor moves back to a NOK state after the debounce period.
	clock.Advance(api.Debounce/4 + 1*time.Second)
	api.UpdateWorkspaceMonitor(context.Background(), &agentproto.WorkspaceMonitorUpdateRequest{
		Datapoints: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.WorkspaceMonitorUpdateRequest_Datapoint_VolumeUsage{
					{
						Path:  volumePath,
						Used:  10,
						Total: 10,
					},
				},
			},
		},
	})

	// Then: We expect a notification
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 1)
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
