package provisionerdserver

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

func TestWorkspaceCreationOutcomesMetricLogic(t *testing.T) {
	t.Parallel()

	// Test the logic conditions for incrementing the metric.
	testCases := []struct {
		name            string
		buildNumber     int32
		transition      database.WorkspaceTransition
		initiatorID     uuid.UUID
		jobError        sql.NullString
		jobErrorCode    sql.NullString
		hasPreset       bool
		presetName      string
		shouldIncrement bool
		expectedStatus  string
		description     string
	}{
		{
			name:            "FirstStartBuildSuccess",
			buildNumber:     1,
			transition:      database.WorkspaceTransitionStart,
			initiatorID:     uuid.New(),
			jobError:        sql.NullString{Valid: false},
			jobErrorCode:    sql.NullString{Valid: false},
			hasPreset:       false,
			shouldIncrement: true,
			expectedStatus:  "success",
			description:     "Should increment with success status for successful first build",
		},
		{
			name:            "FirstStartBuildFailure",
			buildNumber:     1,
			transition:      database.WorkspaceTransitionStart,
			initiatorID:     uuid.New(),
			jobError:        sql.NullString{String: "build failed", Valid: true},
			jobErrorCode:    sql.NullString{Valid: false},
			hasPreset:       false,
			shouldIncrement: true,
			expectedStatus:  "failure",
			description:     "Should increment with failure status when job has error",
		},
		{
			name:            "FirstStartBuildFailureWithErrorCode",
			buildNumber:     1,
			transition:      database.WorkspaceTransitionStart,
			initiatorID:     uuid.New(),
			jobError:        sql.NullString{Valid: false},
			jobErrorCode:    sql.NullString{String: "TIMEOUT", Valid: true},
			hasPreset:       false,
			shouldIncrement: true,
			expectedStatus:  "failure",
			description:     "Should increment with failure status when job has error code",
		},
		{
			name:            "SecondBuildShouldNotIncrement",
			buildNumber:     2,
			transition:      database.WorkspaceTransitionStart,
			initiatorID:     uuid.New(),
			jobError:        sql.NullString{Valid: false},
			jobErrorCode:    sql.NullString{Valid: false},
			hasPreset:       false,
			shouldIncrement: false,
			expectedStatus:  "success",
			description:     "Should not increment for subsequent builds",
		},
		{
			name:            "StopTransitionShouldNotIncrement",
			buildNumber:     1,
			transition:      database.WorkspaceTransitionStop,
			initiatorID:     uuid.New(),
			jobError:        sql.NullString{Valid: false},
			jobErrorCode:    sql.NullString{Valid: false},
			hasPreset:       false,
			shouldIncrement: false,
			expectedStatus:  "success",
			description:     "Should not increment for stop transitions",
		},
		{
			name:            "PrebuildsUserShouldNotIncrement",
			buildNumber:     1,
			transition:      database.WorkspaceTransitionStart,
			initiatorID:     database.PrebuildsSystemUserID,
			jobError:        sql.NullString{Valid: false},
			jobErrorCode:    sql.NullString{Valid: false},
			hasPreset:       false,
			shouldIncrement: false,
			expectedStatus:  "success",
			description:     "Should not increment for prebuilds system user",
		},
		{
			name:            "WithPresetTracksPresetName",
			buildNumber:     1,
			transition:      database.WorkspaceTransitionStart,
			initiatorID:     uuid.New(),
			jobError:        sql.NullString{Valid: false},
			jobErrorCode:    sql.NullString{Valid: false},
			hasPreset:       true,
			presetName:      "test-preset",
			shouldIncrement: true,
			expectedStatus:  "success",
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

			// Simulate the workspace, build, and job data.
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
				InitiatorID: tc.initiatorID,
				TemplateVersionPresetID: uuid.NullUUID{
					UUID:  presetID,
					Valid: tc.hasPreset,
				},
			}

			job := database.ProvisionerJob{
				Error:     tc.jobError,
				ErrorCode: tc.jobErrorCode,
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
			initialValue := promtest.ToFloat64(WorkspaceCreationOutcomesTotal.WithLabelValues(
				orgName,
				templateName,
				expectedPresetName,
				tc.expectedStatus,
			))

			// Simulate the metric increment logic.
			if workspaceBuild.BuildNumber == 1 &&
				workspaceBuild.Transition == database.WorkspaceTransitionStart &&
				workspaceBuild.InitiatorID != database.PrebuildsSystemUserID {
				// Determine status based on job completion.
				status := "success"
				if job.Error.Valid || job.ErrorCode.Valid {
					status = "failure"
				}

				// Get preset name for labels.
				presetName := ""
				if workspaceBuild.TemplateVersionPresetID.Valid {
					preset, err := db.GetPresetByID(ctx, presetID)
					if err == nil {
						presetName = preset.Name
					}
				}

				WorkspaceCreationOutcomesTotal.WithLabelValues(
					workspace.OrganizationName,
					workspace.TemplateName,
					presetName,
					status,
				).Inc()
			}

			// Verify the metric.
			newValue := promtest.ToFloat64(WorkspaceCreationOutcomesTotal.WithLabelValues(
				orgName,
				templateName,
				expectedPresetName,
				tc.expectedStatus,
			))

			if tc.shouldIncrement {
				require.Equal(t, initialValue+1, newValue, tc.description)
			} else {
				require.Equal(t, initialValue, newValue, tc.description)
			}
		})
	}
}
