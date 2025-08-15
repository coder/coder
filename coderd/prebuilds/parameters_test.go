package prebuilds_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
)

func TestFindMatchingPresetID(t *testing.T) {
	t.Parallel()

	presetIDs := []uuid.UUID{
		uuid.New(),
		uuid.New(),
	}
	// Give each preset a meaningful name in alphabetical order
	presetNames := map[uuid.UUID]string{
		presetIDs[0]: "development",
		presetIDs[1]: "production",
	}
	tests := []struct {
		name             string
		parameterNames   []string
		parameterValues  []string
		presetParameters []database.TemplateVersionPresetParameter
		expectedPresetID uuid.UUID
		expectError      bool
		errorContains    string
	}{
		{
			name:            "exact match",
			parameterNames:  []string{"region", "instance_type"},
			parameterValues: []string{"us-west-2", "t3.medium"},
			presetParameters: []database.TemplateVersionPresetParameter{
				{TemplateVersionPresetID: presetIDs[0], Name: "region", Value: "us-west-2"},
				{TemplateVersionPresetID: presetIDs[0], Name: "instance_type", Value: "t3.medium"},
				// antagonist:
				{TemplateVersionPresetID: presetIDs[1], Name: "region", Value: "us-west-2"},
				{TemplateVersionPresetID: presetIDs[1], Name: "instance_type", Value: "t3.large"},
			},
			expectedPresetID: presetIDs[0],
			expectError:      false,
		},
		{
			name:            "no match - different values",
			parameterNames:  []string{"region", "instance_type"},
			parameterValues: []string{"us-east-1", "t3.medium"},
			presetParameters: []database.TemplateVersionPresetParameter{
				{TemplateVersionPresetID: presetIDs[0], Name: "region", Value: "us-west-2"},
				{TemplateVersionPresetID: presetIDs[0], Name: "instance_type", Value: "t3.medium"},
				// antagonist:
				{TemplateVersionPresetID: presetIDs[1], Name: "region", Value: "us-west-2"},
				{TemplateVersionPresetID: presetIDs[1], Name: "instance_type", Value: "t3.large"},
			},
			expectedPresetID: uuid.Nil,
			expectError:      false,
		},
		{
			name:            "no match - fewer provided parameters",
			parameterNames:  []string{"region"},
			parameterValues: []string{"us-west-2"},
			presetParameters: []database.TemplateVersionPresetParameter{
				{TemplateVersionPresetID: presetIDs[0], Name: "region", Value: "us-west-2"},
				{TemplateVersionPresetID: presetIDs[0], Name: "instance_type", Value: "t3.medium"},
				// antagonist:
				{TemplateVersionPresetID: presetIDs[1], Name: "region", Value: "us-west-2"},
				{TemplateVersionPresetID: presetIDs[1], Name: "instance_type", Value: "t3.large"},
			},
			expectedPresetID: uuid.Nil,
			expectError:      false,
		},
		{
			name:            "subset match - extra provided parameter",
			parameterNames:  []string{"region", "instance_type", "extra_param"},
			parameterValues: []string{"us-west-2", "t3.medium", "extra_value"},
			presetParameters: []database.TemplateVersionPresetParameter{
				{TemplateVersionPresetID: presetIDs[0], Name: "region", Value: "us-west-2"},
				{TemplateVersionPresetID: presetIDs[0], Name: "instance_type", Value: "t3.medium"},
				// antagonist:
				{TemplateVersionPresetID: presetIDs[1], Name: "region", Value: "us-west-2"},
				{TemplateVersionPresetID: presetIDs[1], Name: "instance_type", Value: "t3.large"},
			},
			expectedPresetID: presetIDs[0], // Should match because all preset parameters are present
			expectError:      false,
		},
		{
			name:             "mismatched parameter names vs values",
			parameterNames:   []string{"region", "instance_type"},
			parameterValues:  []string{"us-west-2"},
			presetParameters: []database.TemplateVersionPresetParameter{},
			expectedPresetID: uuid.Nil,
			expectError:      true,
			errorContains:    "parameter names and values must have the same length",
		},
		{
			name:            "multiple presets - match first",
			parameterNames:  []string{"region", "instance_type"},
			parameterValues: []string{"us-west-2", "t3.medium"},
			presetParameters: []database.TemplateVersionPresetParameter{
				{TemplateVersionPresetID: presetIDs[0], Name: "region", Value: "us-west-2"},
				{TemplateVersionPresetID: presetIDs[0], Name: "instance_type", Value: "t3.medium"},
				{TemplateVersionPresetID: presetIDs[1], Name: "region", Value: "us-east-1"},
				{TemplateVersionPresetID: presetIDs[1], Name: "instance_type", Value: "t3.large"},
			},
			expectedPresetID: presetIDs[0],
			expectError:      false,
		},
		{
			name:            "largest subset match",
			parameterNames:  []string{"region", "instance_type", "storage_size"},
			parameterValues: []string{"us-west-2", "t3.medium", "100gb"},
			presetParameters: []database.TemplateVersionPresetParameter{
				{TemplateVersionPresetID: presetIDs[0], Name: "region", Value: "us-west-2"},
				{TemplateVersionPresetID: presetIDs[0], Name: "instance_type", Value: "t3.medium"},
				{TemplateVersionPresetID: presetIDs[1], Name: "region", Value: "us-west-2"},
			},
			expectedPresetID: presetIDs[0], // Should match the larger subset (2 params vs 1 param)
			expectError:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			db, _ := dbtestutil.NewDB(t)
			org := dbgen.Organization(t, db, database.Organization{})
			user := dbgen.User(t, db, database.User{})
			templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				OrganizationID: org.ID,
				CreatedBy:      user.ID,
				JobID:          uuid.New(),
			})

			// Group parameters by preset ID and create presets
			presetMap := make(map[uuid.UUID][]database.TemplateVersionPresetParameter)
			for _, param := range tt.presetParameters {
				presetMap[param.TemplateVersionPresetID] = append(presetMap[param.TemplateVersionPresetID], param)
			}

			// Create presets and insert their parameters
			for presetID, params := range presetMap {
				// Create the preset
				_, err := db.InsertPreset(ctx, database.InsertPresetParams{
					ID:                presetID,
					TemplateVersionID: templateVersion.ID,
					Name:              presetNames[presetID],
					CreatedAt:         dbtestutil.NowInDefaultTimezone(),
				})
				require.NoError(t, err)

				// Insert parameters for this preset
				names := make([]string, len(params))
				values := make([]string, len(params))
				for i, param := range params {
					names[i] = param.Name
					values[i] = param.Value
				}

				_, err = db.InsertPresetParameters(ctx, database.InsertPresetParametersParams{
					TemplateVersionPresetID: presetID,
					Names:                   names,
					Values:                  values,
				})
				require.NoError(t, err)
			}

			result, err := prebuilds.FindMatchingPresetID(
				ctx,
				db,
				templateVersion.ID,
				tt.parameterNames,
				tt.parameterValues,
			)

			// Assert results
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPresetID, result)
			}
		})
	}
}
