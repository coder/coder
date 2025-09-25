package coderd_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAIBridgeListInterceptions(t *testing.T) {
	t.Parallel()

	t.Run("EmptyDB", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)
		experimentalClient := codersdk.NewExperimentalClient(client)
		ctx := testutil.Context(t, testutil.WaitLong)
		res, err := experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.Empty(t, res.Results)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)
		experimentalClient := codersdk.NewExperimentalClient(client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Insert a bunch of test data.
		now := dbtime.Now()
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			StartedAt: now.Add(-time.Hour),
		})
		i1tok1 := dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: i1.ID,
			CreatedAt:      now,
		})
		i1tok2 := dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: i1.ID,
			CreatedAt:      now.Add(-time.Minute),
		})
		i1up1 := dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: i1.ID,
			CreatedAt:      now,
		})
		i1up2 := dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: i1.ID,
			CreatedAt:      now.Add(-time.Minute),
		})
		i1tool1 := dbgen.AIBridgeToolUsage(t, db, database.InsertAIBridgeToolUsageParams{
			InterceptionID: i1.ID,
			CreatedAt:      now,
		})
		i1tool2 := dbgen.AIBridgeToolUsage(t, db, database.InsertAIBridgeToolUsageParams{
			InterceptionID: i1.ID,
			CreatedAt:      now.Add(-time.Minute),
		})
		i2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			StartedAt: now,
		})

		// Convert to SDK types for response comparison.
		// You may notice that the ordering of the inner arrays are ASC, this is
		// intentional.
		i1SDK := db2sdk.AIBridgeInterception(i1, []database.AIBridgeTokenUsage{i1tok2, i1tok1}, []database.AIBridgeUserPrompt{i1up2, i1up1}, []database.AIBridgeToolUsage{i1tool2, i1tool1})
		i2SDK := db2sdk.AIBridgeInterception(i2, nil, nil, nil)

		res, err := experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Results, 2)
		require.Equal(t, i2SDK.ID, res.Results[0].ID)
		require.Equal(t, i1SDK.ID, res.Results[1].ID)

		// Normalize timestamps in the response so we can compare the whole
		// thing easily.
		res.Results[0].StartedAt = i2SDK.StartedAt
		res.Results[1].StartedAt = i1SDK.StartedAt
		require.Len(t, res.Results[1].TokenUsages, 2)
		require.Equal(t, i1SDK.TokenUsages[0].ID, res.Results[1].TokenUsages[0].ID)
		require.Equal(t, i1SDK.TokenUsages[1].ID, res.Results[1].TokenUsages[1].ID)
		res.Results[1].TokenUsages[0].CreatedAt = i1SDK.TokenUsages[0].CreatedAt
		res.Results[1].TokenUsages[1].CreatedAt = i1SDK.TokenUsages[1].CreatedAt
		require.Len(t, res.Results[1].UserPrompts, 2)
		require.Equal(t, i1SDK.UserPrompts[0].ID, res.Results[1].UserPrompts[0].ID)
		require.Equal(t, i1SDK.UserPrompts[1].ID, res.Results[1].UserPrompts[1].ID)
		res.Results[1].UserPrompts[0].CreatedAt = i1SDK.UserPrompts[0].CreatedAt
		res.Results[1].UserPrompts[1].CreatedAt = i1SDK.UserPrompts[1].CreatedAt
		require.Len(t, res.Results[1].ToolUsages, 2)
		require.Equal(t, i1SDK.ToolUsages[0].ID, res.Results[1].ToolUsages[0].ID)
		require.Equal(t, i1SDK.ToolUsages[1].ID, res.Results[1].ToolUsages[1].ID)
		res.Results[1].ToolUsages[0].CreatedAt = i1SDK.ToolUsages[0].CreatedAt
		res.Results[1].ToolUsages[1].CreatedAt = i1SDK.ToolUsages[1].CreatedAt

		require.Equal(t, []codersdk.AIBridgeInterception{i2SDK, i1SDK}, res.Results)
	})

	t.Run("Pagination", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)
		experimentalClient := codersdk.NewExperimentalClient(client)
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		interception1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			StartedAt: now.Add(-time.Hour),
		})
		interception2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			StartedAt: now,
		})

		// Try to get an invalid limit.
		res, err := experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
			Pagination: codersdk.Pagination{
				Limit: 1001,
			},
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Contains(t, sdkErr.Message, "Invalid pagination limit value.")
		require.Empty(t, res.Results)

		// Get first page. The newest interception should be the first one.
		res, err = experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
			Pagination: codersdk.Pagination{
				Limit: 1,
			},
		})
		require.NoError(t, err)
		require.Len(t, res.Results, 1)
		require.Equal(t, interception2.ID, res.Results[0].ID)

		// Get the second page. The oldest interception should be the first one.
		res, err = experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
			Pagination: codersdk.Pagination{
				Limit:  1,
				Offset: 1,
			},
		})
		require.NoError(t, err)
		require.Len(t, res.Results, 1)
		require.Equal(t, interception1.ID, res.Results[0].ID)

		// Get a non-existent page.
		res, err = experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
			Pagination: codersdk.Pagination{
				Limit:  1,
				Offset: 2,
			},
		})
		require.NoError(t, err)
		require.Empty(t, res.Results)
	})

	t.Run("Authorized", func(t *testing.T) {
		t.Parallel()
		adminClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		adminExperimentalClient := codersdk.NewExperimentalClient(adminClient)
		firstUser := coderdtest.CreateFirstUser(t, adminClient)
		ctx := testutil.Context(t, testutil.WaitLong)

		secondUserClient, secondUser := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
		secondUserExperimentalClient := codersdk.NewExperimentalClient(secondUserClient)

		now := dbtime.Now()
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now,
		})
		i2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: secondUser.ID,
			StartedAt:   now.Add(-time.Hour),
		})

		// Admin can see all interceptions.
		res, err := adminExperimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Results, 2)
		require.Equal(t, i1.ID, res.Results[0].ID)
		require.Equal(t, i2.ID, res.Results[1].ID)

		// Second user can only see their own interceptions.
		res, err = secondUserExperimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Results, 1)
		require.Equal(t, i2.ID, res.Results[0].ID)
	})

	t.Run("Filter", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		firstUser := coderdtest.CreateFirstUser(t, client)
		experimentalClient := codersdk.NewExperimentalClient(client)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, secondUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		// Insert a bunch of test data with varying filterable fields.
		now := dbtime.Now()
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			InitiatorID: firstUser.UserID,
			Provider:    "one",
			Model:       "one",
			StartedAt:   now,
		})
		i2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			InitiatorID: firstUser.UserID,
			Provider:    "two",
			Model:       "two",
			StartedAt:   now.Add(-time.Hour),
		})
		i3 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			InitiatorID: secondUser.ID,
			Provider:    "three",
			Model:       "three",
			StartedAt:   now.Add(-2 * time.Hour),
		})

		// Convert to SDK types for response comparison. We don't care about the
		// inner arrays for this test.
		i1SDK := db2sdk.AIBridgeInterception(i1, nil, nil, nil)
		i2SDK := db2sdk.AIBridgeInterception(i2, nil, nil, nil)
		i3SDK := db2sdk.AIBridgeInterception(i3, nil, nil, nil)

		cases := []struct {
			name   string
			filter codersdk.AIBridgeListInterceptionsFilter
			want   []codersdk.AIBridgeInterception
		}{
			{
				name:   "NoFilter",
				filter: codersdk.AIBridgeListInterceptionsFilter{},
				want:   []codersdk.AIBridgeInterception{i1SDK, i2SDK, i3SDK},
			},
			{
				name:   "Initiator/NoMatch",
				filter: codersdk.AIBridgeListInterceptionsFilter{Initiator: uuid.New().String()},
				want:   []codersdk.AIBridgeInterception{},
			},
			{
				name:   "Initiator/Me",
				filter: codersdk.AIBridgeListInterceptionsFilter{Initiator: codersdk.Me},
				want:   []codersdk.AIBridgeInterception{i1SDK, i2SDK},
			},
			{
				name:   "Initiator/UserID",
				filter: codersdk.AIBridgeListInterceptionsFilter{Initiator: secondUser.ID.String()},
				want:   []codersdk.AIBridgeInterception{i3SDK},
			},
			{
				name:   "Initiator/Username",
				filter: codersdk.AIBridgeListInterceptionsFilter{Initiator: secondUser.Username},
				want:   []codersdk.AIBridgeInterception{i3SDK},
			},
			{
				name:   "Provider/NoMatch",
				filter: codersdk.AIBridgeListInterceptionsFilter{Provider: "nonsense"},
				want:   []codersdk.AIBridgeInterception{},
			},
			{
				name:   "Provider/OK",
				filter: codersdk.AIBridgeListInterceptionsFilter{Provider: "two"},
				want:   []codersdk.AIBridgeInterception{i2SDK},
			},
			{
				name:   "Model/NoMatch",
				filter: codersdk.AIBridgeListInterceptionsFilter{Model: "nonsense"},
				want:   []codersdk.AIBridgeInterception{},
			},
			{
				name:   "Model/OK",
				filter: codersdk.AIBridgeListInterceptionsFilter{Model: "three"},
				want:   []codersdk.AIBridgeInterception{i3SDK},
			},
			{
				name: "StartedAfter/NoMatch",
				filter: codersdk.AIBridgeListInterceptionsFilter{
					StartedAfter: i1.StartedAt.Add(10 * time.Minute),
				},
				want: []codersdk.AIBridgeInterception{},
			},
			{
				name: "StartedAfter/OK",
				filter: codersdk.AIBridgeListInterceptionsFilter{
					StartedAfter: i2.StartedAt.Add(-10 * time.Minute),
				},
				want: []codersdk.AIBridgeInterception{i1SDK, i2SDK},
			},
			{
				name: "StartedBefore/NoMatch",
				filter: codersdk.AIBridgeListInterceptionsFilter{
					StartedBefore: i3.StartedAt.Add(-10 * time.Minute),
				},
				want: []codersdk.AIBridgeInterception{},
			},
			{
				name: "StartedBefore/OK",
				filter: codersdk.AIBridgeListInterceptionsFilter{
					StartedBefore: i3.StartedAt.Add(10 * time.Minute),
				},
				want: []codersdk.AIBridgeInterception{i3SDK},
			},
			{
				name: "BothBeforeAndAfter/NoMatch",
				filter: codersdk.AIBridgeListInterceptionsFilter{
					StartedAfter:  i1.StartedAt.Add(10 * time.Minute),
					StartedBefore: i1.StartedAt.Add(20 * time.Minute),
				},
				want: []codersdk.AIBridgeInterception{},
			},
			{
				name: "BothBeforeAndAfter/OK",
				filter: codersdk.AIBridgeListInterceptionsFilter{
					StartedAfter:  i2.StartedAt.Add(-10 * time.Minute),
					StartedBefore: i2.StartedAt.Add(10 * time.Minute),
				},
				want: []codersdk.AIBridgeInterception{i2SDK},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				res, err := experimentalClient.AIBridgeListInterceptions(ctx, tc.filter)
				require.NoError(t, err)
				// We just compare UUID strings for the sake of this test.
				wantIDs := make([]string, len(tc.want))
				for i, r := range tc.want {
					wantIDs[i] = r.ID.String()
				}
				gotIDs := make([]string, len(res.Results))
				for i, r := range res.Results {
					gotIDs[i] = r.ID.String()
				}
				require.Equal(t, wantIDs, gotIDs)
			})
		}
	})

	t.Run("FilterErrors", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)
		experimentalClient := codersdk.NewExperimentalClient(client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// No need to insert any test data, we're just testing the filter
		// errors.

		cases := []struct {
			name string
			q    string
			want []codersdk.ValidationError
		}{
			{
				name: "UnknownUsername",
				q:    "initiator:unknown",
				want: []codersdk.ValidationError{
					{
						Field:  "initiator",
						Detail: `Query param "initiator" has invalid value: user "unknown" either does not exist, or you are unauthorized to view them`,
					},
				},
			},
			{
				name: "InvalidStartedAfter",
				q:    "started_after:invalid",
				want: []codersdk.ValidationError{
					{
						Field:  "started_after",
						Detail: `Query param "started_after" must be a valid date format (2006-01-02T15:04:05.999999999Z07:00): parsing time "INVALID" as "2006-01-02T15:04:05.999999999Z07:00": cannot parse "INVALID" as "2006"`,
					},
				},
			},
			{
				name: "InvalidStartedBefore",
				q:    "started_before:invalid",
				want: []codersdk.ValidationError{
					{
						Field:  "started_before",
						Detail: `Query param "started_before" must be a valid date format (2006-01-02T15:04:05.999999999Z07:00): parsing time "INVALID" as "2006-01-02T15:04:05.999999999Z07:00": cannot parse "INVALID" as "2006"`,
					},
				},
			},
			{
				name: "InvalidBeforeAfterRange",
				// Before MUST be after After if both are set
				q: `started_after:"2025-01-01T00:00:00Z" started_before:"2024-01-01T00:00:00Z"`,
				want: []codersdk.ValidationError{
					{
						Field:  "started_before",
						Detail: `Query param "started_before" has invalid value: "started_before" must be after "started_after" if set`,
					},
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				res, err := experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
					FilterQuery: tc.q,
				})
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, tc.want, sdkErr.Validations)
				require.Empty(t, res.Results)
			})
		}
	})
}
