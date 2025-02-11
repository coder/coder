package agentapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
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

func resourceMonitorAPI(t *testing.T) (*agentapi.ResourcesMonitoringAPI, database.User, *quartz.Mock, *notificationstest.FakeEnqueuer) {
	t.Helper()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{Valid: true, UUID: template.ID},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		OwnerID:        user.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		Type: database.ProvisionerJobTypeWorkspaceBuild,
	})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		JobID:             job.ID,
		WorkspaceID:       workspace.ID,
		TemplateVersionID: templateVersion.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: build.JobID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})

	notifyEnq := &notificationstest.FakeEnqueuer{}
	clock := quartz.NewMock(t)

	return &agentapi.ResourcesMonitoringAPI{
		AgentID:                agent.ID,
		WorkspaceID:            workspace.ID,
		Clock:                  clock,
		Database:               db,
		NotificationsEnqueuer:  notifyEnq,
		MinimumNOKsToAlert:     4,
		ConsecutiveNOKsToAlert: 10,
		Debounce:               1 * time.Minute,
	}, user, clock, notifyEnq
}

func TestMemoryResourceMonitorDebounce(t *testing.T) {
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

	api, user, clock, notifyEnq := resourceMonitorAPI(t)

	// Given: A monitor in an OK state
	dbgen.WorkspaceAgentMemoryResourceMonitor(t, api.Database, database.WorkspaceAgentMemoryResourceMonitor{
		AgentID:   api.AgentID,
		State:     database.WorkspaceAgentMonitorStateOK,
		Threshold: 80,
	})

	// When: The monitor is given a state that will trigger NOK
	_, err := api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Memory: &agentproto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage{
					Used:  10,
					Total: 10,
				},
			},
		},
	})
	require.NoError(t, err)

	// Then: We expect there to be a notification sent
	sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
	require.Len(t, sent, 1)
	require.Equal(t, user.ID, sent[0].UserID)
	notifyEnq.Clear()

	// When: The monitor moves to an OK state from NOK
	clock.Advance(api.Debounce / 4)
	_, err = api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Memory: &agentproto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage{
					Used:  1,
					Total: 10,
				},
			},
		},
	})
	require.NoError(t, err)

	// Then: We expect no new notifications
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When: The monitor moves back to a NOK state before the debounced time.
	clock.Advance(api.Debounce / 4)
	_, err = api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Memory: &agentproto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage{
					Used:  10,
					Total: 10,
				},
			},
		},
	})
	require.NoError(t, err)

	// Then: We expect no new notifications (showing the debouncer working)
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When: The monitor moves back to an OK state from NOK
	clock.Advance(api.Debounce / 4)
	_, err = api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Memory: &agentproto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage{
					Used:  1,
					Total: 10,
				},
			},
		},
	})
	require.NoError(t, err)

	// Then: We still expect no new notifications
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When: The monitor moves back to a NOK state after the debounce period.
	clock.Advance(api.Debounce/4 + 1*time.Second)
	_, err = api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Memory: &agentproto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage{
					Used:  10,
					Total: 10,
				},
			},
		},
	})
	require.NoError(t, err)

	// Then: We expect a notification
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfMemory))
	require.Len(t, sent, 1)
	require.Equal(t, user.ID, sent[0].UserID)
}

func TestMemoryResourceMonitor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		memoryUsage      []int64
		memoryTotal      int64
		thresholdPercent int32
		minimumNOKs      int
		consecutiveNOKs  int
		previousState    database.WorkspaceAgentMonitorState
		expectState      database.WorkspaceAgentMonitorState
		shouldNotify     bool
	}{
		{
			name:             "WhenOK/NeverExceedsThreshold",
			memoryUsage:      []int64{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 2, 3, 1, 2},
			memoryTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceAgentMonitorStateOK,
			expectState:      database.WorkspaceAgentMonitorStateOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenOK/ConsecutiveExceedsThreshold",
			memoryUsage:      []int64{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 8, 9, 8, 9},
			memoryTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceAgentMonitorStateOK,
			expectState:      database.WorkspaceAgentMonitorStateNOK,
			shouldNotify:     true,
		},
		{
			name:             "WhenOK/MinimumExceedsThreshold",
			memoryUsage:      []int64{2, 8, 2, 9, 2, 8, 2, 9, 2, 8, 4, 9, 1, 8, 2, 8, 9},
			memoryTotal:      10,
			thresholdPercent: 80,
			minimumNOKs:      4,
			consecutiveNOKs:  10,
			previousState:    database.WorkspaceAgentMonitorStateOK,
			expectState:      database.WorkspaceAgentMonitorStateNOK,
			shouldNotify:     true,
		},
		{
			name:             "WhenNOK/NeverExceedsThreshold",
			memoryUsage:      []int64{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 2, 3, 1, 2},
			memoryTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceAgentMonitorStateNOK,
			expectState:      database.WorkspaceAgentMonitorStateOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenNOK/ConsecutiveExceedsThreshold",
			memoryUsage:      []int64{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 8, 9, 8, 9},
			memoryTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceAgentMonitorStateNOK,
			expectState:      database.WorkspaceAgentMonitorStateNOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenNOK/MinimumExceedsThreshold",
			memoryUsage:      []int64{2, 8, 2, 9, 2, 8, 2, 9, 2, 8, 4, 9, 1, 8, 2, 8, 9},
			memoryTotal:      10,
			thresholdPercent: 80,
			minimumNOKs:      4,
			consecutiveNOKs:  10,
			previousState:    database.WorkspaceAgentMonitorStateNOK,
			expectState:      database.WorkspaceAgentMonitorStateNOK,
			shouldNotify:     false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api, user, clock, notifyEnq := resourceMonitorAPI(t)
			api.MinimumNOKsToAlert = tt.minimumNOKs
			api.ConsecutiveNOKsToAlert = tt.consecutiveNOKs

			datapoints := make([]*agentproto.PushResourcesMonitoringUsageRequest_Datapoint, 0, len(tt.memoryUsage))
			collectedAt := clock.Now()
			for _, usage := range tt.memoryUsage {
				collectedAt = collectedAt.Add(15 * time.Second)
				datapoints = append(datapoints, &agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
					CollectedAt: timestamppb.New(collectedAt),
					Memory: &agentproto.PushResourcesMonitoringUsageRequest_Datapoint_MemoryUsage{
						Used:  usage,
						Total: tt.memoryTotal,
					},
				})
			}

			dbgen.WorkspaceAgentMemoryResourceMonitor(t, api.Database, database.WorkspaceAgentMemoryResourceMonitor{
				AgentID:   api.AgentID,
				State:     tt.previousState,
				Threshold: tt.thresholdPercent,
			})

			clock.Set(collectedAt)
			_, err := api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
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

func TestVolumeResourceMonitorDebounce(t *testing.T) {
	t.Parallel()

	// This test is an even longer one. We're testing
	// that the debounce logic is independent per
	// volume monitor. We interleave the triggering
	// of each monitor to ensure the debounce logic
	// is monitor independent.
	//
	// First Monitor:
	//   1. OK -> NOK  |> sends a notification
	//   2. NOK -> OK  |> does nothing
	//   3. OK -> NOK  |> does nothing due to debounce period
	//   4. NOK -> OK  |> does nothing
	//   5. OK -> NOK  |> sends a notification as debounce period exceeded
	//   6. NOK -> OK  |> does nothing
	//
	// Second Monitor:
	//   1. OK -> OK  |> does nothing
	//   2. OK -> NOK |> sends a notification
	//   3. NOK -> OK |> does nothing
	//   4. OK -> NOK |> does nothing due to debounce period
	//   5. NOK -> OK |> does nothing
	//   6. OK -> NOK |> sends a notification as debounce period exceeded
	//

	firstVolumePath := "/home/coder"
	secondVolumePath := "/dev/coder"

	api, _, clock, notifyEnq := resourceMonitorAPI(t)

	// Given:
	//  - First monitor in an OK state
	//  - Second monitor in an OK state
	dbgen.WorkspaceAgentVolumeResourceMonitor(t, api.Database, database.WorkspaceAgentVolumeResourceMonitor{
		AgentID:   api.AgentID,
		Path:      firstVolumePath,
		State:     database.WorkspaceAgentMonitorStateOK,
		Threshold: 80,
	})
	dbgen.WorkspaceAgentVolumeResourceMonitor(t, api.Database, database.WorkspaceAgentVolumeResourceMonitor{
		AgentID:   api.AgentID,
		Path:      secondVolumePath,
		State:     database.WorkspaceAgentMonitorStateNOK,
		Threshold: 80,
	})

	// When:
	//  - First monitor is in a NOK state
	//  - Second monitor is in an OK state
	_, err := api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage{
					{Path: firstVolumePath, SpaceUsed: 10, SpaceTotal: 10},
					{Path: secondVolumePath, SpaceUsed: 1, SpaceTotal: 10},
				},
			},
		},
	})
	require.NoError(t, err)

	// Then:
	//  - We expect a notification from only the first monitor
	sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 1)
	volumes := requireVolumeData(t, sent[0])
	require.Len(t, volumes, 1)
	require.Equal(t, firstVolumePath, volumes[0]["path"])
	notifyEnq.Clear()

	// When:
	//  - First monitor moves back to OK
	//  - Second monitor moves to NOK
	clock.Advance(api.Debounce / 4)
	_, err = api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage{
					{Path: firstVolumePath, SpaceUsed: 1, SpaceTotal: 10},
					{Path: secondVolumePath, SpaceUsed: 10, SpaceTotal: 10},
				},
			},
		},
	})
	require.NoError(t, err)

	// Then:
	//  - We expect a notification from only the second monitor
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 1)
	volumes = requireVolumeData(t, sent[0])
	require.Len(t, volumes, 1)
	require.Equal(t, secondVolumePath, volumes[0]["path"])
	notifyEnq.Clear()

	// When:
	//  - First monitor moves back to NOK before debounce period has ended
	//  - Second monitor moves back to OK
	clock.Advance(api.Debounce / 4)
	_, err = api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage{
					{Path: firstVolumePath, SpaceUsed: 10, SpaceTotal: 10},
					{Path: secondVolumePath, SpaceUsed: 1, SpaceTotal: 10},
				},
			},
		},
	})
	require.NoError(t, err)

	// Then:
	//  - We expect no new notifications
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When:
	//  - First monitor moves back to OK
	//  - Second monitor moves back to NOK
	clock.Advance(api.Debounce / 4)
	_, err = api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage{
					{Path: firstVolumePath, SpaceUsed: 1, SpaceTotal: 10},
					{Path: secondVolumePath, SpaceUsed: 10, SpaceTotal: 10},
				},
			},
		},
	})
	require.NoError(t, err)

	// Then:
	//  - We expect no new notifications.
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 0)
	notifyEnq.Clear()

	// When:
	//  - First monitor moves back to a NOK state after the debounce period
	//  - Second monitor moves back to OK
	clock.Advance(api.Debounce/4 + 1*time.Second)
	_, err = api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage{
					{Path: firstVolumePath, SpaceUsed: 10, SpaceTotal: 10},
					{Path: secondVolumePath, SpaceUsed: 1, SpaceTotal: 10},
				},
			},
		},
	})
	require.NoError(t, err)

	// Then:
	//  - We expect a notification from only the first monitor
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 1)
	volumes = requireVolumeData(t, sent[0])
	require.Len(t, volumes, 1)
	require.Equal(t, firstVolumePath, volumes[0]["path"])
	notifyEnq.Clear()

	// When:
	//  - First montior moves back to OK
	//  - Second monitor moves back to NOK after the debounce period
	clock.Advance(api.Debounce/4 + 1*time.Second)
	_, err = api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage{
					{Path: firstVolumePath, SpaceUsed: 1, SpaceTotal: 10},
					{Path: secondVolumePath, SpaceUsed: 10, SpaceTotal: 10},
				},
			},
		},
	})
	require.NoError(t, err)

	// Then:
	//  - We expect a notification from only the second monitor
	sent = notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 1)
	volumes = requireVolumeData(t, sent[0])
	require.Len(t, volumes, 1)
	require.Equal(t, secondVolumePath, volumes[0]["path"])
}

func TestVolumeResourceMonitor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		volumePath       string
		volumeUsage      []int64
		volumeTotal      int64
		thresholdPercent int32
		previousState    database.WorkspaceAgentMonitorState
		expectState      database.WorkspaceAgentMonitorState
		shouldNotify     bool
		minimumNOKs      int
		consecutiveNOKs  int
	}{
		{
			name:             "WhenOK/NeverExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int64{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 2, 3, 1, 2},
			volumeTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceAgentMonitorStateOK,
			expectState:      database.WorkspaceAgentMonitorStateOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenOK/ConsecutiveExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int64{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 8, 9, 8, 9},
			volumeTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceAgentMonitorStateOK,
			expectState:      database.WorkspaceAgentMonitorStateNOK,
			shouldNotify:     true,
		},
		{
			name:             "WhenOK/MinimumExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int64{2, 8, 2, 9, 2, 8, 2, 9, 2, 8, 4, 9, 1, 8, 2, 8, 9},
			volumeTotal:      10,
			thresholdPercent: 80,
			minimumNOKs:      4,
			consecutiveNOKs:  10,
			previousState:    database.WorkspaceAgentMonitorStateOK,
			expectState:      database.WorkspaceAgentMonitorStateNOK,
			shouldNotify:     true,
		},
		{
			name:             "WhenNOK/NeverExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int64{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 2, 3, 1, 2},
			volumeTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceAgentMonitorStateNOK,
			expectState:      database.WorkspaceAgentMonitorStateOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenNOK/ConsecutiveExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int64{2, 3, 2, 4, 2, 3, 2, 1, 2, 3, 4, 4, 1, 8, 9, 8, 9},
			volumeTotal:      10,
			thresholdPercent: 80,
			consecutiveNOKs:  4,
			minimumNOKs:      10,
			previousState:    database.WorkspaceAgentMonitorStateNOK,
			expectState:      database.WorkspaceAgentMonitorStateNOK,
			shouldNotify:     false,
		},
		{
			name:             "WhenNOK/MinimumExceedsThreshold",
			volumePath:       "/home/coder",
			volumeUsage:      []int64{2, 8, 2, 9, 2, 8, 2, 9, 2, 8, 4, 9, 1, 8, 2, 8, 9},
			volumeTotal:      10,
			thresholdPercent: 80,
			minimumNOKs:      4,
			consecutiveNOKs:  10,
			previousState:    database.WorkspaceAgentMonitorStateNOK,
			expectState:      database.WorkspaceAgentMonitorStateNOK,
			shouldNotify:     false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api, user, clock, notifyEnq := resourceMonitorAPI(t)
			api.MinimumNOKsToAlert = tt.minimumNOKs
			api.ConsecutiveNOKsToAlert = tt.consecutiveNOKs

			datapoints := make([]*agentproto.PushResourcesMonitoringUsageRequest_Datapoint, 0, len(tt.volumeUsage))
			collectedAt := clock.Now()
			for _, volumeUsage := range tt.volumeUsage {
				collectedAt = collectedAt.Add(15 * time.Second)

				volumeDatapoints := []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage{
					{
						Path:       tt.volumePath,
						SpaceUsed:  volumeUsage,
						SpaceTotal: tt.volumeTotal,
					},
				}

				datapoints = append(datapoints, &agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
					CollectedAt: timestamppb.New(collectedAt),
					Volume:      volumeDatapoints,
				})
			}

			dbgen.WorkspaceAgentVolumeResourceMonitor(t, api.Database, database.WorkspaceAgentVolumeResourceMonitor{
				AgentID:   api.AgentID,
				Path:      tt.volumePath,
				State:     tt.previousState,
				Threshold: tt.thresholdPercent,
			})

			clock.Set(collectedAt)
			_, err := api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
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

func TestVolumeResourceMonitorMultiple(t *testing.T) {
	t.Parallel()

	api, _, clock, notifyEnq := resourceMonitorAPI(t)

	// Given: two different volume resource monitors
	dbgen.WorkspaceAgentVolumeResourceMonitor(t, api.Database, database.WorkspaceAgentVolumeResourceMonitor{
		AgentID:   api.AgentID,
		Path:      "/home/coder",
		State:     database.WorkspaceAgentMonitorStateOK,
		Threshold: 80,
	})

	dbgen.WorkspaceAgentVolumeResourceMonitor(t, api.Database, database.WorkspaceAgentVolumeResourceMonitor{
		AgentID:   api.AgentID,
		Path:      "/dev/coder",
		State:     database.WorkspaceAgentMonitorStateOK,
		Threshold: 80,
	})

	// When: both of them move to a NOK state
	_, err := api.PushResourcesMonitoringUsage(context.Background(), &agentproto.PushResourcesMonitoringUsageRequest{
		Datapoints: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint{
			{
				CollectedAt: timestamppb.New(clock.Now()),
				Volume: []*agentproto.PushResourcesMonitoringUsageRequest_Datapoint_VolumeUsage{
					{
						Path:       "/home/coder",
						SpaceUsed:  10,
						SpaceTotal: 10,
					},
					{
						Path:       "/dev/coder",
						SpaceUsed:  10,
						SpaceTotal: 10,
					},
				},
			},
		},
	})
	require.NoError(t, err)

	// Then: We expect a notification to alert with information about both
	sent := notifyEnq.Sent(notificationstest.WithTemplateID(notifications.TemplateWorkspaceOutOfDisk))
	require.Len(t, sent, 1)

	volumes := requireVolumeData(t, sent[0])
	require.Len(t, volumes, 2)
	require.Equal(t, "/home/coder", volumes[0]["path"])
	require.Equal(t, "/dev/coder", volumes[1]["path"])
}

func requireVolumeData(t *testing.T, notif *notificationstest.FakeNotification) []map[string]any {
	t.Helper()

	volumesData := notif.Data["volumes"]
	require.IsType(t, []map[string]any{}, volumesData)

	return volumesData.([]map[string]any)
}
