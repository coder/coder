package coderd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

func TestWorkspaceCreationAttemptsMetricLogic(t *testing.T) {
	t.Parallel()

	// Test the logic conditions for incrementing the metric.
	testCases := []struct {
		name            string
		buildNumber     int32
		transition      database.WorkspaceTransition
		initiatorID     uuid.UUID
		hasPreset       bool
		presetName      string
		shouldIncrement bool
		description     string
	}{
		{
			name:            "FirstStartBuildByRegularUser",
			buildNumber:     1,
			transition:      database.WorkspaceTransitionStart,
			initiatorID:     uuid.New(), // Regular user
			hasPreset:       false,
			shouldIncrement: true,
			description:     "Should increment for first start build by regular user",
		},
		{
			name:            "SecondBuildShouldNotIncrement",
			buildNumber:     2,
			transition:      database.WorkspaceTransitionStart,
			initiatorID:     uuid.New(),
			hasPreset:       false,
			shouldIncrement: false,
			description:     "Should not increment for subsequent builds",
		},
		{
			name:            "StopTransitionShouldNotIncrement",
			buildNumber:     1,
			transition:      database.WorkspaceTransitionStop,
			initiatorID:     uuid.New(),
			hasPreset:       false,
			shouldIncrement: false,
			description:     "Should not increment for stop transitions",
		},
		{
			name:            "PrebuildsUserShouldNotIncrement",
			buildNumber:     1,
			transition:      database.WorkspaceTransitionStart,
			initiatorID:     database.PrebuildsSystemUserID,
			hasPreset:       false,
			shouldIncrement: false,
			description:     "Should not increment for prebuilds system user",
		},
		{
			name:            "WithPresetTracksPresetName",
			buildNumber:     1,
			transition:      database.WorkspaceTransitionStart,
			initiatorID:     uuid.New(),
			hasPreset:       true,
			presetName:      "test-preset",
			shouldIncrement: true,
			description:     "Should increment and track preset name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			db := dbmock.NewMockStore(ctrl)

			// Simulate the workspace and build data.
			orgName := "test-org"
			templateName := "test-template"
			presetID := uuid.New()

			workspace := database.Workspace{
				OrganizationName: orgName,
				TemplateName:     templateName,
			}

			workspaceBuild := database.WorkspaceBuild{
				BuildNumber: tc.buildNumber,
				Transition:  tc.transition,
				TemplateVersionPresetID: uuid.NullUUID{
					UUID:  presetID,
					Valid: tc.hasPreset,
				},
			}

			// Mock preset lookup if needed.
			if tc.shouldIncrement && tc.hasPreset {
				db.EXPECT().
					GetPresetByID(ctx, presetID).
					Return(database.GetPresetByIDRow{
						Name: tc.presetName,
					}, nil)
			}

			// Get initial metric value.
			expectedPresetName := ""
			if tc.shouldIncrement && tc.hasPreset {
				expectedPresetName = tc.presetName
			}
			initialValue := promtest.ToFloat64(WorkspaceCreationAttemptsTotal.WithLabelValues(
				orgName,
				templateName,
				expectedPresetName,
			))

			// Call the actual metric increment function.
			incrementWorkspaceCreationAttemptsMetric(ctx, db, workspace, workspaceBuild, tc.initiatorID)

			// Verify the metric.
			newValue := promtest.ToFloat64(WorkspaceCreationAttemptsTotal.WithLabelValues(
				orgName,
				templateName,
				expectedPresetName,
			))

			if tc.shouldIncrement {
				require.Equal(t, initialValue+1, newValue, tc.description)
			} else {
				require.Equal(t, initialValue, newValue, tc.description)
			}
		})
	}
}
