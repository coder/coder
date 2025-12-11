package coderd_test

import (
	"database/sql"
	"io"
	"net/http"
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
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestAIBridgeListInterceptions(t *testing.T) {
	t.Parallel()

	t.Run("RequiresLicenseFeature", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				// No aibridge feature
				Features: license.Features{},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is irrelevant here.
		_, err := client.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		require.Equal(t, "AI Bridge is a Premium feature. Contact sales!", sdkErr.Message)
	})

	t.Run("EmptyDB", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is irrelevant here.
		res, err := client.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.Empty(t, res.Results)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		client, db, firstUser := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		user1, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		user1Visible := database.VisibleUser{
			ID:        user1.ID,
			Username:  user1.Username,
			Name:      user1.Name,
			AvatarURL: user1.AvatarURL,
		}

		_, user2 := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		user2Visible := database.VisibleUser{
			ID:        user2.ID,
			Username:  user2.Username,
			Name:      user2.Name,
			AvatarURL: user2.AvatarURL,
		}

		// Insert a bunch of test data.
		now := dbtime.Now()
		i1ApiKey := sql.NullString{String: "some-api-key", Valid: true}
		i1EndedAt := now.Add(-time.Hour + time.Minute)
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			APIKeyID:    i1ApiKey,
			InitiatorID: user1.ID,
			StartedAt:   now.Add(-time.Hour),
		}, &i1EndedAt)
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
			InitiatorID: user2.ID,
			StartedAt:   now,
		}, &now)

		// Convert to SDK types for response comparison.
		// You may notice that the ordering of the inner arrays are ASC, this is
		// intentional.
		i1SDK := db2sdk.AIBridgeInterception(i1, user1Visible, []database.AIBridgeTokenUsage{i1tok2, i1tok1}, []database.AIBridgeUserPrompt{i1up2, i1up1}, []database.AIBridgeToolUsage{i1tool2, i1tool1})
		i2SDK := db2sdk.AIBridgeInterception(i2, user2Visible, nil, nil, nil)

		res, err := client.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Results, 2)
		require.Equal(t, i2SDK.ID, res.Results[0].ID)
		require.Equal(t, i1SDK.ID, res.Results[1].ID)

		require.Equal(t, &i1ApiKey.String, i1SDK.APIKeyID)
		require.Nil(t, i2SDK.APIKeyID)

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

		// Time comparison
		require.Len(t, res.Results, 2)
		require.Equal(t, res.Results[0].ID, i2SDK.ID)
		require.NotNil(t, res.Results[0].EndedAt)
		require.WithinDuration(t, now, *res.Results[0].EndedAt, 5*time.Second)
		res.Results[0].EndedAt = i2SDK.EndedAt
		require.NotNil(t, res.Results[1].EndedAt)
		res.Results[1].EndedAt = i1SDK.EndedAt

		require.Equal(t, []codersdk.AIBridgeInterception{i2SDK, i1SDK}, res.Results)
	})

	t.Run("Pagination", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, db, firstUser := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		allInterceptionIDs := make([]uuid.UUID, 0, 20)

		// Create 10 interceptions with the same started_at time. The returned
		// order for these should still be deterministic.
		now := dbtime.Now()
		for i := range 10 {
			interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				ID:          uuid.UUID{byte(i)},
				InitiatorID: firstUser.UserID,
				StartedAt:   now,
			}, &now)
			allInterceptionIDs = append(allInterceptionIDs, interception.ID)
		}

		// Create 10 interceptions with a random started_at time.
		for i := range 10 {
			randomOffset, err := cryptorand.Intn(10000)
			require.NoError(t, err)
			randomOffsetDur := time.Duration(randomOffset) * time.Second
			endedAt := now.Add(randomOffsetDur + time.Minute)
			interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				ID:          uuid.UUID{byte(i + 10)},
				InitiatorID: firstUser.UserID,
				StartedAt:   now.Add(randomOffsetDur),
			}, &endedAt)
			allInterceptionIDs = append(allInterceptionIDs, interception.ID)
		}

		// Try to fetch with an invalid limit.
		res, err := client.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
			Pagination: codersdk.Pagination{
				Limit: 1001,
			},
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Contains(t, sdkErr.Message, "Invalid pagination limit value.")
		require.Empty(t, res.Results)

		// Try to fetch with both after_id and offset pagination.
		res, err = client.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
			Pagination: codersdk.Pagination{
				AfterID: allInterceptionIDs[0],
				Offset:  1,
			},
		})
		require.ErrorAs(t, err, &sdkErr)
		require.Contains(t, sdkErr.Message, "Query parameters have invalid values")
		require.Contains(t, sdkErr.Detail, "Cannot use both after_id and offset pagination in the same request.")

		// Iterate over all interceptions using both cursor and offset
		// pagination modes.
		for _, paginationMode := range []string{"after_id", "offset"} {
			t.Run(paginationMode, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitLong)

				// Get all interceptions one by one using the given pagination
				// mode.
				getAllInterceptionsOneByOne := func() []uuid.UUID {
					interceptionIDs := []uuid.UUID{}
					for {
						pagination := codersdk.Pagination{
							Limit: 1,
						}
						if paginationMode == "after_id" {
							if len(interceptionIDs) > 0 {
								pagination.AfterID = interceptionIDs[len(interceptionIDs)-1]
							}
						} else {
							pagination.Offset = len(interceptionIDs)
						}
						res, err := client.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
							Pagination: pagination,
						})
						require.NoError(t, err)
						if len(res.Results) == 0 {
							break
						}
						require.EqualValues(t, len(allInterceptionIDs), res.Count)
						require.Len(t, res.Results, 1)
						interceptionIDs = append(interceptionIDs, res.Results[0].ID)
					}
					return interceptionIDs
				}

				// First attempt: get all interceptions one by one.
				gotInterceptionIDs1 := getAllInterceptionsOneByOne()
				// We should have all of the interceptions returned:
				require.ElementsMatch(t, allInterceptionIDs, gotInterceptionIDs1)

				// Second attempt: get all interceptions one by one again.
				gotInterceptionIDs2 := getAllInterceptionsOneByOne()
				// They should be returned in the exact same order.
				require.Equal(t, gotInterceptionIDs1, gotInterceptionIDs2)
			})
		}
	})

	t.Run("InflightInterceptions", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		client, db, firstUser := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		i1EndedAt := now.Add(time.Minute)
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now,
		}, &i1EndedAt)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now.Add(-time.Hour),
		}, nil)

		res, err := client.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Len(t, res.Results, 1)
		require.Equal(t, i1.ID, res.Results[0].ID)
	})

	t.Run("Authorized", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		adminClient, db, firstUser := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		secondUserClient, secondUser := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)

		now := dbtime.Now()
		i1EndedAt := now.Add(time.Minute)
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now,
		}, &i1EndedAt)
		i2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: secondUser.ID,
			StartedAt:   now.Add(-time.Hour),
		}, &now)

		// Admin can see all interceptions.
		res, err := adminClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.EqualValues(t, 2, res.Count)
		require.Len(t, res.Results, 2)
		require.Equal(t, i1.ID, res.Results[0].ID)
		require.Equal(t, i2.ID, res.Results[1].ID)

		// Second user can only see their own interceptions.
		res, err = secondUserClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Len(t, res.Results, 1)
		require.Equal(t, i2.ID, res.Results[0].ID)
	})

	t.Run("Filter", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		client, db, firstUser := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		user1, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		user1Visible := database.VisibleUser{
			ID:        user1.ID,
			Username:  user1.Username,
			Name:      user1.Name,
			AvatarURL: user1.AvatarURL,
		}

		_, user2 := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		user2Visible := database.VisibleUser{
			ID:        user2.ID,
			Username:  user2.Username,
			Name:      user2.Name,
			AvatarURL: user2.AvatarURL,
		}

		// Insert a bunch of test data with varying filterable fields.
		now := dbtime.Now()
		i1EndedAt := now.Add(time.Minute)
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			InitiatorID: user1.ID,
			Provider:    "one",
			Model:       "one",
			StartedAt:   now,
		}, &i1EndedAt)
		i2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			InitiatorID: user1.ID,
			Provider:    "two",
			Model:       "two",
			StartedAt:   now.Add(-time.Hour),
		}, &now)
		i3 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			InitiatorID: user2.ID,
			Provider:    "three",
			Model:       "three",
			StartedAt:   now.Add(-2 * time.Hour),
		}, &now)

		// Convert to SDK types for response comparison. We don't care about the
		// inner arrays for this test.
		i1SDK := db2sdk.AIBridgeInterception(i1, user1Visible, nil, nil, nil)
		i2SDK := db2sdk.AIBridgeInterception(i2, user1Visible, nil, nil, nil)
		i3SDK := db2sdk.AIBridgeInterception(i3, user2Visible, nil, nil, nil)

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
				filter: codersdk.AIBridgeListInterceptionsFilter{Initiator: user2.ID.String()},
				want:   []codersdk.AIBridgeInterception{i3SDK},
			},
			{
				name:   "Initiator/Username",
				filter: codersdk.AIBridgeListInterceptionsFilter{Initiator: user2.Username},
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
				ctx := testutil.Context(t, testutil.WaitLong)
				res, err := client.AIBridgeListInterceptions(ctx, tc.filter)
				require.NoError(t, err)
				require.EqualValues(t, len(tc.want), res.Count)
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
		dv := coderdtest.DeploymentValues(t)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
			},
		})

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
				ctx := testutil.Context(t, testutil.WaitLong)
				res, err := client.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
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

func TestAIBridgeRouting(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	client, closer, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAIBridge: 1,
			},
		},
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})

	// Register a simple test handler that echoes back the request path.
	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(r.URL.Path))
	})
	api.RegisterInMemoryAIBridgedHTTPHandler(testHandler)

	cases := []struct {
		name         string
		path         string
		expectedPath string
	}{
		{
			name:         "StablePrefix",
			path:         "/api/v2/aibridge/openai/v1/chat/completions",
			expectedPath: "/openai/v1/chat/completions",
		},
		{
			name:         "ExperimentalPrefix",
			path:         "/api/experimental/aibridge/openai/v1/chat/completions",
			expectedPath: "/openai/v1/chat/completions",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.URL.String()+tc.path, nil)
			require.NoError(t, err)
			req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

			httpClient := &http.Client{}
			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Verify that the prefix was stripped correctly and the path was forwarded.
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.expectedPath, string(body))
		})
	}
}

func TestAIBridgeRateLimiting(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	// Set a low rate limit for testing.
	dv.AI.BridgeConfig.RateLimit = 2

	client, closer, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAIBridge: 1,
			},
		},
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})

	// Register a simple test handler.
	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})
	api.RegisterInMemoryAIBridgedHTTPHandler(testHandler)

	ctx := testutil.Context(t, testutil.WaitLong)
	httpClient := &http.Client{}
	url := client.URL.String() + "/api/v2/aibridge/test"

	// Make requests up to the limit - should succeed.
	for range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		require.NoError(t, err)
		req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// Next request should be rate limited.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	require.NoError(t, err)
	req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("Retry-After"))
}

func TestAIBridgeConcurrencyLimiting(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	// Set a low concurrency limit for testing.
	dv.AI.BridgeConfig.MaxConcurrency = 1

	client, closer, api, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAIBridge: 1,
			},
		},
	})
	t.Cleanup(func() {
		_ = closer.Close()
	})

	// Register a handler that blocks until signaled.
	started := make(chan struct{})
	unblock := make(chan struct{})
	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-unblock
		rw.WriteHeader(http.StatusOK)
	})
	api.RegisterInMemoryAIBridgedHTTPHandler(testHandler)

	ctx := testutil.Context(t, testutil.WaitLong)
	httpClient := &http.Client{}
	url := client.URL.String() + "/api/v2/aibridge/test"

	// Start a request that will block.
	done := make(chan struct{})
	go func() {
		defer close(done)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		require.NoError(t, err)
		req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

		resp, err := httpClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	// Wait for the first request to start processing.
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("timed out waiting for first request to start")
	}

	// Second request should be rejected with 503.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	require.NoError(t, err)
	req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	// Unblock the first request and wait for it to complete.
	close(unblock)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for first request to complete")
	}
}
