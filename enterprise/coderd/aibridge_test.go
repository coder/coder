package coderd_test

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	aiblib "github.com/coder/aibridge"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
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
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is irrelevant here.
		res, err := client.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.Empty(t, res.Results)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
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

		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
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
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
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
		adminClient, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
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

		// Members cannot read AIBridge interceptions, not even their
		// own (i2 is owned by secondUser).
		res, err := secondUserClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Count)
		require.Empty(t, res.Results)

		// Owner can see all interceptions, including secondUser's,
		// proving the data exists and the member was filtered out.
		res, err = adminClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{})
		require.NoError(t, err)
		require.EqualValues(t, 2, res.Count)
		require.Len(t, res.Results, 2)
		require.Equal(t, i1.ID, res.Results[0].ID)
		require.Equal(t, i2.ID, res.Results[1].ID)
	})

	t.Run("Filter", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
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
			Client:      sql.NullString{String: string(aiblib.ClientCursor), Valid: true},
		}, &now)
		i3 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			ID:          uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			InitiatorID: user2.ID,
			Provider:    "three",
			Model:       "three",
			StartedAt:   now.Add(-2 * time.Hour),
			Client:      sql.NullString{String: string(aiblib.ClientClaudeCode), Valid: true},
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
				name:   "Client/Unknown",
				filter: codersdk.AIBridgeListInterceptionsFilter{Client: "Unknown"},
				want:   []codersdk.AIBridgeInterception{i1SDK},
			},
			{
				name:   "Client/Match",
				filter: codersdk.AIBridgeListInterceptionsFilter{Client: string(aiblib.ClientCursor)},
				want:   []codersdk.AIBridgeInterception{i2SDK},
			},
			{
				name:   "Client/NoMatch",
				filter: codersdk.AIBridgeListInterceptionsFilter{Client: "nonsense"},
				want:   []codersdk.AIBridgeInterception{},
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

	t.Run("FilterByMe/MemberCannotReadOwn", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		ownerClient, db, firstUser := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
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

		memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, firstUser.OrganizationID)

		now := dbtime.Now()
		// Create an interception initiated by the member.
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: member.ID,
			StartedAt:   now,
		}, nil)

		// Member cannot read their own interceptions, even when
		// filtering by "me".
		res, err := memberClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
			Initiator: codersdk.Me,
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Count)
		require.Empty(t, res.Results)
	})

	t.Run("FilterErrors", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))

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

	t.Run("InvalidCursor", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		// Using a nonexistent UUID as after_id should return 400,
		// not silently return an empty page.
		//nolint:gocritic // Owner role is irrelevant here.
		_, err := client.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsFilter{
			Pagination: codersdk.Pagination{
				AfterID: uuid.New(),
			},
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid pagination cursor")
	})
}

func aibridgeOpts(t *testing.T) *coderdenttest.Options {
	t.Helper()
	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	return &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAIBridge: 1,
			},
		},
	}
}

func TestAIBridgeListSessions(t *testing.T) {
	t.Parallel()

	t.Run("EmptyDB", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is irrelevant here.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.Empty(t, res.Sessions)
		require.EqualValues(t, 0, res.Count)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Session 1: Two interceptions sharing client_session_id "session-A".
		s1i1EndedAt := now.Add(time.Minute)
		s1i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "anthropic",
			Model:           "claude-4",
			StartedAt:       now,
			Client:          sql.NullString{String: "claude-code", Valid: true},
			ClientSessionID: sql.NullString{String: "session-A", Valid: true},
		}, &s1i1EndedAt)
		s1i2EndedAt := now.Add(2 * time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:                firstUser.UserID,
			Provider:                   "anthropic",
			Model:                      "claude-4-haiku",
			StartedAt:                  now.Add(time.Minute),
			Client:                     sql.NullString{String: "claude-code", Valid: true},
			ClientSessionID:            sql.NullString{String: "session-A", Valid: true},
			ThreadRootInterceptionID:   uuid.NullUUID{UUID: s1i1.ID, Valid: true},
			ThreadParentInterceptionID: uuid.NullUUID{UUID: s1i1.ID, Valid: true},
		}, &s1i2EndedAt)

		// Add token usages to session 1 interceptions.
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: s1i1.ID,
			InputTokens:    100,
			OutputTokens:   50,
			CreatedAt:      now,
		})
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: s1i1.ID,
			InputTokens:    200,
			OutputTokens:   75,
			CreatedAt:      now.Add(time.Second),
		})

		// Add user prompts to session 1.
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: s1i1.ID,
			Prompt:         "first prompt",
			CreatedAt:      now,
		})
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: s1i1.ID,
			Prompt:         "last prompt in session",
			CreatedAt:      now.Add(time.Minute),
		})

		// Session 2: Thread-based session (no client_session_id, shared thread_root_id).
		s2i1EndedAt := now.Add(-time.Hour + time.Minute)
		s2i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			Provider:    "openai",
			Model:       "gpt-4",
			StartedAt:   now.Add(-time.Hour),
		}, &s2i1EndedAt)
		s2i2EndedAt := now.Add(-time.Hour + 2*time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:                firstUser.UserID,
			Provider:                   "openai",
			Model:                      "gpt-4",
			StartedAt:                  now.Add(-time.Hour + time.Minute),
			ThreadRootInterceptionID:   uuid.NullUUID{UUID: s2i1.ID, Valid: true},
			ThreadParentInterceptionID: uuid.NullUUID{UUID: s2i1.ID, Valid: true},
		}, &s2i2EndedAt)

		// Session 3: Standalone interception (no client_session_id, no thread_root_id).
		s3EndedAt := now.Add(-2*time.Hour + time.Minute)
		s3i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			Provider:    "anthropic",
			Model:       "claude-4",
			StartedAt:   now.Add(-2 * time.Hour),
		}, &s3EndedAt)

		// Session 4: Two distinct thread roots in one client_session_id.
		s4i1EndedAt := now.Add(-3*time.Hour + time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "anthropic",
			Model:           "claude-4",
			StartedAt:       now.Add(-3 * time.Hour),
			ClientSessionID: sql.NullString{String: "session-multi", Valid: true},
		}, &s4i1EndedAt)
		s4i2EndedAt := now.Add(-3*time.Hour + 2*time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "openai",
			Model:           "gpt-4",
			StartedAt:       now.Add(-3*time.Hour + time.Minute),
			ClientSessionID: sql.NullString{String: "session-multi", Valid: true},
		}, &s4i2EndedAt)

		//nolint:gocritic // Owner role is irrelevant here.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.EqualValues(t, 4, res.Count)
		require.Len(t, res.Sessions, 4)

		// Sessions ordered by started_at DESC: session-A (now), then
		// thread-based (now-1h), then standalone (now-2h), then
		// multi-thread (now-3h).
		require.Equal(t, "session-A", res.Sessions[0].ID)
		require.Equal(t, s2i1.ID.String(), res.Sessions[1].ID)
		require.Equal(t, s3i1.ID.String(), res.Sessions[2].ID)
		require.Equal(t, "session-multi", res.Sessions[3].ID)

		// Verify session 1 aggregations.
		s1 := res.Sessions[0]
		require.ElementsMatch(t, []string{"anthropic"}, s1.Providers)
		require.ElementsMatch(t, []string{"claude-4", "claude-4-haiku"}, s1.Models)
		require.NotNil(t, s1.Client)
		require.Equal(t, "claude-code", *s1.Client)
		require.EqualValues(t, 300, s1.TokenUsageSummary.InputTokens)
		require.EqualValues(t, 125, s1.TokenUsageSummary.OutputTokens)
		require.NotNil(t, s1.LastPrompt)
		require.Equal(t, "last prompt in session", *s1.LastPrompt)
		// Two interceptions in session-A, but they share a thread root,
		// so thread count is 1.
		require.EqualValues(t, 1, s1.Threads)

		// Verify session 2 (thread-based).
		s2 := res.Sessions[1]
		require.ElementsMatch(t, []string{"openai"}, s2.Providers)
		// Thread count: the root interception and its child share the same
		// thread root, so count is 1.
		require.EqualValues(t, 1, s2.Threads)

		// Verify session 3 (standalone).
		s3 := res.Sessions[2]
		require.EqualValues(t, 1, s3.Threads)
		require.Nil(t, s3.LastPrompt)

		// Verify session 4 (multiple threads). Thread A has a root +
		// child (1 thread), thread B is a standalone root (1 thread),
		// so total is 2.
		s4 := res.Sessions[3]
		require.EqualValues(t, 2, s4.Threads)
		require.ElementsMatch(t, []string{"anthropic", "openai"}, s4.Providers)
		require.ElementsMatch(t, []string{"claude-4", "gpt-4"}, s4.Models)
	})

	t.Run("Pagination", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		// Create 5 standalone sessions with different start times.
		allSessionIDs := make([]string, 5)
		for i := range 5 {
			endedAt := now.Add(-time.Duration(i)*time.Hour + time.Minute)
			intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID: firstUser.UserID,
				StartedAt:   now.Add(-time.Duration(i) * time.Hour),
			}, &endedAt)
			// Standalone session: ID = interception UUID string.
			allSessionIDs[i] = intc.ID.String()
		}

		// Test offset pagination.
		//nolint:gocritic // Owner role is irrelevant here.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Pagination: codersdk.Pagination{Limit: 2},
		})
		require.NoError(t, err)
		require.Len(t, res.Sessions, 2)
		require.EqualValues(t, 5, res.Count)
		require.Equal(t, allSessionIDs[0], res.Sessions[0].ID)
		require.Equal(t, allSessionIDs[1], res.Sessions[1].ID)

		// Second page with offset.
		res, err = client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Pagination: codersdk.Pagination{Limit: 2, Offset: 2},
		})
		require.NoError(t, err)
		require.Len(t, res.Sessions, 2)
		require.Equal(t, allSessionIDs[2], res.Sessions[0].ID)
		require.Equal(t, allSessionIDs[3], res.Sessions[1].ID)

		// Test cursor pagination.
		res, err = client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Pagination:     codersdk.Pagination{Limit: 2},
			AfterSessionID: allSessionIDs[1],
		})
		require.NoError(t, err)
		require.Len(t, res.Sessions, 2)
		require.Equal(t, allSessionIDs[2], res.Sessions[0].ID)
		require.Equal(t, allSessionIDs[3], res.Sessions[1].ID)

		// Test mutual exclusion of cursor and offset.
		_, err = client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Pagination:     codersdk.Pagination{Limit: 2, Offset: 1},
			AfterSessionID: allSessionIDs[0],
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Contains(t, sdkErr.Detail, "Cannot use both after_session_id and offset pagination")
	})

	t.Run("AfterSessionIDNotFound", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is irrelevant here.
		_, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Pagination:     codersdk.Pagination{Limit: 10},
			AfterSessionID: "nonexistent-session-id",
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Equal(t, `after_session_id: session "nonexistent-session-id" not found`, sdkErr.Detail)
	})

	t.Run("Filters", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		_, user2 := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		now := dbtime.Now()

		// Session from user1 with provider "anthropic" and client "claude-code".
		s1EndedAt := now.Add(time.Minute)
		s1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			Provider:    "anthropic",
			Model:       "claude-4",
			StartedAt:   now,
			Client:      sql.NullString{String: "claude-code", Valid: true},
		}, &s1EndedAt)

		// Session from user2 with provider "openai".
		s2EndedAt := now.Add(-time.Hour + time.Minute)
		s2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user2.ID,
			Provider:    "openai",
			Model:       "gpt-4",
			StartedAt:   now.Add(-time.Hour),
		}, &s2EndedAt)

		// Filter by initiator.
		//nolint:gocritic // Owner role is irrelevant; testing filter behavior.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Initiator: user2.Username,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Equal(t, s2.ID.String(), res.Sessions[0].ID)

		// Filter by provider.
		res, err = client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Provider: "anthropic",
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Equal(t, s1.ID.String(), res.Sessions[0].ID)

		// Filter by model.
		res, err = client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Model: "gpt-4",
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Equal(t, s2.ID.String(), res.Sessions[0].ID)

		// Filter by client.
		res, err = client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Client: "claude-code",
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Equal(t, s1.ID.String(), res.Sessions[0].ID)

		// Filter by time range.
		res, err = client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			StartedAfter: now.Add(-30 * time.Minute),
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Equal(t, s1.ID.String(), res.Sessions[0].ID)

		// Filter by session_id.
		res, err = client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			SessionID: s2.ID.String(),
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Len(t, res.Sessions, 1)
		require.Equal(t, s2.ID.String(), res.Sessions[0].ID)

		// Filter by session_id with no match.
		res, err = client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			SessionID: "nonexistent-session-id",
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Count)
		require.Empty(t, res.Sessions)
	})

	t.Run("FilterByMe/MemberCannotReadOwn", func(t *testing.T) {
		t.Parallel()
		ownerClient, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, firstUser.OrganizationID)

		now := dbtime.Now()
		// Create an interception (session) initiated by the member.
		_ = dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: member.ID,
			StartedAt:   now,
		}, nil)

		// Member cannot read their own sessions, even when
		// filtering by "me".
		res, err := memberClient.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Initiator: codersdk.Me,
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Count)
		require.Empty(t, res.Sessions)
	})

	t.Run("Authorized", func(t *testing.T) {
		t.Parallel()
		adminClient, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		auditorClient, auditorUser := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID, rbac.RoleAuditor())

		now := dbtime.Now()
		i1EndedAt := now.Add(time.Minute)
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now,
		}, &i1EndedAt)
		i2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: auditorUser.ID,
			StartedAt:   now.Add(-time.Hour),
		}, &now)

		// Site-level auditors can see all sessions.
		res, err := auditorClient.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.EqualValues(t, 2, res.Count)
		require.Len(t, res.Sessions, 2)
		require.Equal(t, i1.ID.String(), res.Sessions[0].ID)
		require.Equal(t, i2.ID.String(), res.Sessions[1].ID)
	})

	t.Run("SessionIDCollisionAcrossUsers", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		_, user2 := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		now := dbtime.Now()

		// Two users share the same client_session_id. They must be
		// treated as distinct sessions.
		sharedSessionID := "shared-session-id"
		u1EndedAt := now.Add(time.Minute)
		u1Interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "anthropic",
			Model:           "claude-4",
			StartedAt:       now,
			Client:          sql.NullString{String: "claude-code", Valid: true},
			ClientSessionID: sql.NullString{String: sharedSessionID, Valid: true},
		}, &u1EndedAt)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: u1Interception.ID,
			InputTokens:    100,
			OutputTokens:   50,
			CreatedAt:      now,
		})

		u2EndedAt := now.Add(-time.Hour + time.Minute)
		u2Interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     user2.ID,
			Provider:        "openai",
			Model:           "gpt-4",
			StartedAt:       now.Add(-time.Hour),
			Client:          sql.NullString{String: "cursor", Valid: true},
			ClientSessionID: sql.NullString{String: sharedSessionID, Valid: true},
		}, &u2EndedAt)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: u2Interception.ID,
			InputTokens:    200,
			OutputTokens:   75,
			CreatedAt:      now.Add(-time.Hour),
		})

		// Admin should see two distinct sessions despite the shared
		// session_id, each with the correct user and token counts.
		//nolint:gocritic // Owner role is irrelevant; testing collision behavior.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.EqualValues(t, 2, res.Count)
		require.Len(t, res.Sessions, 2)

		// Both sessions share the same ID string but belong to
		// different users.
		require.Equal(t, sharedSessionID, res.Sessions[0].ID)
		require.Equal(t, sharedSessionID, res.Sessions[1].ID)
		require.NotEqual(t, res.Sessions[0].Initiator.ID, res.Sessions[1].Initiator.ID)

		// Verify token counts are not merged across users.
		for _, s := range res.Sessions {
			if s.Initiator.ID == firstUser.UserID {
				require.EqualValues(t, 100, s.TokenUsageSummary.InputTokens)
				require.EqualValues(t, 50, s.TokenUsageSummary.OutputTokens)
			} else {
				require.EqualValues(t, 200, s.TokenUsageSummary.InputTokens)
				require.EqualValues(t, 75, s.TokenUsageSummary.OutputTokens)
			}
		}
	})

	t.Run("InflightSessions", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		i1EndedAt := now.Add(time.Minute)
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now,
		}, &i1EndedAt)
		// Inflight interception (no ended_at) should not appear as a session.
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now.Add(-time.Hour),
		}, nil)

		//nolint:gocritic // Owner role is irrelevant; testing inflight filtering.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Len(t, res.Sessions, 1)
		require.Equal(t, i1.ID.String(), res.Sessions[0].ID)
	})

	t.Run("FilterErrors", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))

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
				q:    `started_after:"2025-01-01T00:00:00Z" started_before:"2024-01-01T00:00:00Z"`,
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
				res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
					FilterQuery: tc.q,
				})
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, tc.want, sdkErr.Validations)
				require.Empty(t, res.Sessions)
			})
		}
	})

	t.Run("PaginationLimitValidation", func(t *testing.T) {
		t.Parallel()
		client, _ := coderdenttest.New(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is irrelevant; testing pagination validation.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Pagination: codersdk.Pagination{
				Limit: 1001,
			},
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Contains(t, sdkErr.Message, "Invalid pagination limit value.")
		require.Empty(t, res.Sessions)
	})
}

func TestAIBridgeListClients(t *testing.T) {
	t.Parallel()

	t.Run("RequiresLicenseFeature", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{},
			},
		})

		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:gocritic // Owner role is irrelevant here.
		_, err := client.AIBridgeListClients(ctx)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
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

	now := dbtime.Now()
	endedAt := now.Add(time.Minute)

	// Completed interception with an explicit client.
	dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: firstUser.UserID,
		StartedAt:   now,
		Client:      sql.NullString{String: string(aiblib.ClientCursor), Valid: true},
	}, &endedAt)

	// Completed interception with a different client.
	dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: firstUser.UserID,
		StartedAt:   now,
		Client:      sql.NullString{String: string(aiblib.ClientClaudeCode), Valid: true},
	}, &endedAt)

	// Completed interception with no client — should appear as "Unknown".
	dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: firstUser.UserID,
		StartedAt:   now,
	}, &endedAt)

	// Duplicate client — should be deduplicated in results.
	dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: firstUser.UserID,
		StartedAt:   now,
		Client:      sql.NullString{String: string(aiblib.ClientCursor), Valid: true},
	}, &endedAt)

	// In-flight interception (no ended_at) — must NOT appear in results.
	dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: firstUser.UserID,
		StartedAt:   now,
		Client:      sql.NullString{String: string(aiblib.ClientCopilotCLI), Valid: true},
	}, nil)

	ctx := testutil.Context(t, testutil.WaitLong)
	clients, err := client.AIBridgeListClients(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		string(aiblib.ClientCursor),
		string(aiblib.ClientClaudeCode),
		"Unknown",
	}, clients)
}

func TestAIBridgeRouting(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
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
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
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
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
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
		if err != nil {
			return
		}
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

func TestAIBridgeGetSessionThreads(t *testing.T) {
	t.Parallel()

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		ownerClient, firstUser := coderdenttest.New(t, aibridgeOpts(t))
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, firstUser.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		_, err := memberClient.AIBridgeGetSessionThreads(ctx, "nonexistent-session-id", uuid.Nil, uuid.Nil, 0)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("LookupByClientSessionID", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		endedAt := now.Add(time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "anthropic",
			Model:           "claude-4",
			StartedAt:       now,
			ClientSessionID: sql.NullString{String: "my-session", Valid: true},
		}, &endedAt)

		res, err := client.AIBridgeGetSessionThreads(ctx, "my-session", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)
		require.Equal(t, "my-session", res.ID)
		require.Len(t, res.Threads, 1)
		require.Equal(t, "claude-4", res.Threads[0].Model)
		require.Equal(t, "anthropic", res.Threads[0].Provider)
	})

	t.Run("LookupByInterceptionUUID", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		endedAt := now.Add(time.Minute)
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			Provider:    "openai",
			Model:       "gpt-4",
			StartedAt:   now,
		}, &endedAt)

		// When no client session ID is set, the interception ID becomes the session identifier.
		res, err := client.AIBridgeGetSessionThreads(ctx, i1.ID.String(), uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)
		require.Equal(t, i1.ID.String(), res.ID)
		require.Len(t, res.Threads, 1)
	})

	t.Run("ThreadsWithAgenticActions", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Create a session with one thread. Root interception + child
		// interception sharing thread_root_id.
		rootEndedAt := now.Add(time.Minute)
		root := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "anthropic",
			Model:           "claude-4",
			StartedAt:       now,
			ClientSessionID: sql.NullString{String: "thread-session", Valid: true},
		}, &rootEndedAt)

		childEndedAt := now.Add(2 * time.Minute)
		child := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:                firstUser.UserID,
			Provider:                   "anthropic",
			Model:                      "claude-4",
			StartedAt:                  now.Add(time.Minute),
			ClientSessionID:            sql.NullString{String: "thread-session", Valid: true},
			ThreadRootInterceptionID:   uuid.NullUUID{UUID: root.ID, Valid: true},
			ThreadParentInterceptionID: uuid.NullUUID{UUID: root.ID, Valid: true},
		}, &childEndedAt)

		// Add a user prompt on the root.
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: root.ID,
			Prompt:         "implement login feature",
			CreatedAt:      now,
		})

		// Add token usage on root with metadata.
		providerRespID := "resp-1"
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID:     root.ID,
			ProviderResponseID: providerRespID,
			InputTokens:        100,
			OutputTokens:       50,
			Metadata:           json.RawMessage(`{"cache_read_input": 20, "cache_creation_input": 10}`),
			CreatedAt:          now,
		})

		// Add two tool usages on root (demonstrates multiple tools per action).
		dbgen.AIBridgeToolUsage(t, db, database.InsertAIBridgeToolUsageParams{
			InterceptionID:     root.ID,
			ProviderResponseID: providerRespID,
			Tool:               "read_file",
			Input:              `{"path": "/main.go"}`,
			CreatedAt:          now.Add(time.Second),
		})
		dbgen.AIBridgeToolUsage(t, db, database.InsertAIBridgeToolUsageParams{
			InterceptionID:     root.ID,
			ProviderResponseID: providerRespID,
			Tool:               "list_dir",
			Input:              `{"path": "/"}`,
			CreatedAt:          now.Add(2 * time.Second),
		})

		// Add model thought for the root interception.
		dbgen.AIBridgeModelThought(t, db, database.InsertAIBridgeModelThoughtParams{
			InterceptionID: root.ID,
			Content:        "Let me read the main file first.",
			CreatedAt:      now.Add(time.Second),
		})

		// Add token usage on child.
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID:     child.ID,
			ProviderResponseID: "resp-2",
			InputTokens:        200,
			OutputTokens:       100,
			Metadata:           json.RawMessage(`{"cache_read_input": 30}`),
			CreatedAt:          now.Add(time.Minute),
		})

		// Add another tool usage on child.
		dbgen.AIBridgeToolUsage(t, db, database.InsertAIBridgeToolUsageParams{
			InterceptionID:     child.ID,
			ProviderResponseID: "resp-2",
			Tool:               "write_file",
			Input:              `{"path": "/login.go"}`,
			CreatedAt:          now.Add(time.Minute + time.Second),
		})

		res, err := client.AIBridgeGetSessionThreads(ctx, "thread-session", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)
		require.Equal(t, "thread-session", res.ID)
		require.Len(t, res.Threads, 1)

		// PageStartedAt/PageEndedAt bracket the visible threads.
		require.NotNil(t, res.PageStartedAt)
		require.NotNil(t, res.PageEndedAt)
		require.True(t, res.PageStartedAt.Equal(now), "PageStartedAt should equal root started_at")
		require.True(t, res.PageEndedAt.Equal(childEndedAt), "PageEndedAt should equal child ended_at")

		thread := res.Threads[0]
		require.Equal(t, root.ID, thread.ID)
		require.NotNil(t, thread.Prompt)
		require.Equal(t, "implement login feature", *thread.Prompt)
		require.Equal(t, "claude-4", thread.Model)
		require.Equal(t, "anthropic", thread.Provider)

		// Thread-level token aggregation.
		require.EqualValues(t, 300, thread.TokenUsage.InputTokens)
		require.EqualValues(t, 150, thread.TokenUsage.OutputTokens)
		require.NotEmpty(t, thread.TokenUsage.Metadata)
		require.EqualValues(t, int64(50), thread.TokenUsage.Metadata["cache_read_input"])
		require.EqualValues(t, int64(10), thread.TokenUsage.Metadata["cache_creation_input"])

		// Two agentic actions (one per interception with tool calls).
		require.Len(t, thread.AgenticActions, 2)

		action1 := thread.AgenticActions[0]
		// Root interception has two tool calls.
		require.Len(t, action1.ToolCalls, 2)
		require.Equal(t, "read_file", action1.ToolCalls[0].Tool)
		require.Equal(t, "list_dir", action1.ToolCalls[1].Tool)
		require.Len(t, action1.Thinking, 1)
		require.Equal(t, "Let me read the main file first.", action1.Thinking[0].Text)
		// Token usage for root interception.
		require.EqualValues(t, 100, action1.TokenUsage.InputTokens)
		require.EqualValues(t, 50, action1.TokenUsage.OutputTokens)

		action2 := thread.AgenticActions[1]
		require.Len(t, action2.ToolCalls, 1)
		require.Equal(t, "write_file", action2.ToolCalls[0].Tool)
		require.Empty(t, action2.Thinking)

		// Session-level token aggregation.
		require.EqualValues(t, 300, res.TokenUsageSummary.InputTokens)
		require.EqualValues(t, 150, res.TokenUsageSummary.OutputTokens)
	})

	t.Run("MultiThreadPagination", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Create a session with 3 threads. Each thread is a standalone
		// interception sharing client_session_id.
		startedAt := func(i int) time.Time { return now.Add(time.Duration(i) * time.Hour) }
		endedAt := func(i int) time.Time { return now.Add(time.Duration(i)*time.Hour + time.Minute) }
		threadIDs := make([]uuid.UUID, 3)
		for i := range 3 {
			ea := endedAt(i)
			intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID:     firstUser.UserID,
				Provider:        "anthropic",
				Model:           "claude-4",
				StartedAt:       startedAt(i),
				ClientSessionID: sql.NullString{String: "multi-thread-session", Valid: true},
			}, &ea)
			threadIDs[i] = intc.ID
		}

		// Get all threads (no pagination).
		res, err := client.AIBridgeGetSessionThreads(ctx, "multi-thread-session", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)
		require.Len(t, res.Threads, 3)

		// Threads are ordered by started_at ASC (chronological).
		require.Equal(t, threadIDs[0], res.Threads[0].ID)
		require.Equal(t, threadIDs[1], res.Threads[1].ID)
		require.Equal(t, threadIDs[2], res.Threads[2].ID)

		// Page bounds span all 3 threads.
		require.NotNil(t, res.PageStartedAt)
		require.NotNil(t, res.PageEndedAt)
		require.True(t, res.PageStartedAt.Equal(startedAt(0)), "all threads: PageStartedAt = thread 0 started_at")
		require.True(t, res.PageEndedAt.Equal(endedAt(2)), "all threads: PageEndedAt = thread 2 ended_at")

		// Page with limit 1: should get only the oldest thread.
		res, err = client.AIBridgeGetSessionThreads(ctx, "multi-thread-session", uuid.Nil, uuid.Nil, 1)
		require.NoError(t, err)
		require.Len(t, res.Threads, 1)
		require.Equal(t, threadIDs[0], res.Threads[0].ID)
		require.NotNil(t, res.PageStartedAt)
		require.NotNil(t, res.PageEndedAt)
		require.True(t, res.PageStartedAt.Equal(startedAt(0)), "page 1: PageStartedAt = thread 0 started_at")
		require.True(t, res.PageEndedAt.Equal(endedAt(0)), "page 1: PageEndedAt = thread 0 ended_at")

		// Page forward using after_id: get next thread.
		res, err = client.AIBridgeGetSessionThreads(ctx, "multi-thread-session", threadIDs[0], uuid.Nil, 1)
		require.NoError(t, err)
		require.Len(t, res.Threads, 1)
		require.Equal(t, threadIDs[1], res.Threads[0].ID)
		require.NotNil(t, res.PageStartedAt)
		require.NotNil(t, res.PageEndedAt)
		require.True(t, res.PageStartedAt.Equal(startedAt(1)), "page 2: PageStartedAt = thread 1 started_at")
		require.True(t, res.PageEndedAt.Equal(endedAt(1)), "page 2: PageEndedAt = thread 1 ended_at")

		// Page forward again.
		res, err = client.AIBridgeGetSessionThreads(ctx, "multi-thread-session", threadIDs[1], uuid.Nil, 1)
		require.NoError(t, err)
		require.Len(t, res.Threads, 1)
		require.Equal(t, threadIDs[2], res.Threads[0].ID)
		require.NotNil(t, res.PageStartedAt)
		require.NotNil(t, res.PageEndedAt)
		require.True(t, res.PageStartedAt.Equal(startedAt(2)), "page 3: PageStartedAt = thread 2 started_at")
		require.True(t, res.PageEndedAt.Equal(endedAt(2)), "page 3: PageEndedAt = thread 2 ended_at")

		// No more threads.
		res, err = client.AIBridgeGetSessionThreads(ctx, "multi-thread-session", threadIDs[2], uuid.Nil, 1)
		require.NoError(t, err)
		require.Empty(t, res.Threads)
		require.Nil(t, res.PageStartedAt, "empty page: PageStartedAt is nil")
		require.Nil(t, res.PageEndedAt, "empty page: PageEndedAt is nil")

		// before_id filters to threads older than the given ID.
		// before_id=newest → returns both older threads, ASC.
		res, err = client.AIBridgeGetSessionThreads(ctx, "multi-thread-session", uuid.Nil, threadIDs[2], 0)
		require.NoError(t, err)
		require.Len(t, res.Threads, 2)
		require.Equal(t, threadIDs[0], res.Threads[0].ID)
		require.Equal(t, threadIDs[1], res.Threads[1].ID)
		require.NotNil(t, res.PageStartedAt)
		require.NotNil(t, res.PageEndedAt)
		require.True(t, res.PageStartedAt.Equal(startedAt(0)), "before_id=newest: PageStartedAt = thread 0 started_at")
		require.True(t, res.PageEndedAt.Equal(endedAt(1)), "before_id=newest: PageEndedAt = thread 1 ended_at")

		// before_id=middle → returns only the oldest thread.
		res, err = client.AIBridgeGetSessionThreads(ctx, "multi-thread-session", uuid.Nil, threadIDs[1], 0)
		require.NoError(t, err)
		require.Len(t, res.Threads, 1)
		require.Equal(t, threadIDs[0], res.Threads[0].ID)
		require.NotNil(t, res.PageStartedAt)
		require.NotNil(t, res.PageEndedAt)
		require.True(t, res.PageStartedAt.Equal(startedAt(0)), "before_id=middle: PageStartedAt = thread 0 started_at")
		require.True(t, res.PageEndedAt.Equal(endedAt(0)), "before_id=middle: PageEndedAt = thread 0 ended_at")

		// before_id=oldest → no older threads exist.
		res, err = client.AIBridgeGetSessionThreads(ctx, "multi-thread-session", uuid.Nil, threadIDs[0], 0)
		require.NoError(t, err)
		require.Empty(t, res.Threads)

		// Combining after_id and before_id is rejected.
		_, err = client.AIBridgeGetSessionThreads(ctx, "multi-thread-session", threadIDs[2], threadIDs[0], 0)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	// Verify that session-level token metadata aggregates tokens from ALL
	// threads, not just the ones visible in the current page.
	t.Run("SessionTokenAggregationAcrossPages", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Create 3 threads, each with token usage on both root and child
		// interceptions to ensure child tokens are counted too.
		var firstThreadID uuid.UUID
		for i := range 3 {
			offset := time.Duration(i) * time.Hour
			rootEndedAt := now.Add(offset + 30*time.Minute)
			root := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID:     firstUser.UserID,
				Provider:        "anthropic",
				Model:           "claude-4",
				StartedAt:       now.Add(offset),
				ClientSessionID: sql.NullString{String: "token-agg-session", Valid: true},
			}, &rootEndedAt)
			if i == 0 {
				firstThreadID = root.ID
			}

			// Token usage on root: 100 input, 50 output, with cache metadata.
			dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				InterceptionID:     root.ID,
				ProviderResponseID: "resp-root",
				InputTokens:        100,
				OutputTokens:       50,
				Metadata:           json.RawMessage(`{"cache_read_input": 20, "cache_creation_input": 5}`),
				CreatedAt:          now.Add(offset),
			})

			// Add a child interception with its own token usage.
			childEndedAt := now.Add(offset + 45*time.Minute)
			child := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID:                firstUser.UserID,
				Provider:                   "anthropic",
				Model:                      "claude-4",
				StartedAt:                  now.Add(offset + 15*time.Minute),
				ClientSessionID:            sql.NullString{String: "token-agg-session", Valid: true},
				ThreadRootInterceptionID:   uuid.NullUUID{UUID: root.ID, Valid: true},
				ThreadParentInterceptionID: uuid.NullUUID{UUID: root.ID, Valid: true},
			}, &childEndedAt)

			// Token usage on child: 200 input, 100 output, with cache metadata.
			dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				InterceptionID:     child.ID,
				ProviderResponseID: "resp-child",
				InputTokens:        200,
				OutputTokens:       100,
				Metadata:           json.RawMessage(`{"cache_read_input": 30}`),
				CreatedAt:          now.Add(offset + 15*time.Minute),
			})
		}

		// Request only the first thread (limit=1). The session-level
		// token summary must still reflect ALL 3 threads.
		res, err := client.AIBridgeGetSessionThreads(ctx, "token-agg-session", uuid.Nil, uuid.Nil, 1)
		require.NoError(t, err)
		require.Len(t, res.Threads, 1)
		require.Equal(t, firstThreadID, res.Threads[0].ID)

		// Per-thread token usage: root(100) + child(200) = 300 input.
		require.EqualValues(t, 300, res.Threads[0].TokenUsage.InputTokens)
		require.EqualValues(t, 150, res.Threads[0].TokenUsage.OutputTokens)

		// Session-level summary must include tokens from all 3 threads
		// (3 * 300 input, 3 * 150 output), not just the single page.
		require.EqualValues(t, 900, res.TokenUsageSummary.InputTokens)
		require.EqualValues(t, 450, res.TokenUsageSummary.OutputTokens)

		// Session-level metadata must aggregate across all 3 threads:
		// cache_read_input: 3 * (root 20 + child 30) = 150
		// cache_creation_input: 3 * (root 5) = 15
		require.NotEmpty(t, res.TokenUsageSummary.Metadata)
		require.EqualValues(t, int64(150), res.TokenUsageSummary.Metadata["cache_read_input"])
		require.EqualValues(t, int64(15), res.TokenUsageSummary.Metadata["cache_creation_input"])
	})

	t.Run("InvalidCursor", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		endedAt := now.Add(time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "anthropic",
			Model:           "claude-4",
			StartedAt:       now,
			ClientSessionID: sql.NullString{String: "cursor-test-session", Valid: true},
		}, &endedAt)

		// A completely nonexistent UUID as after_id should return 400.
		_, err := client.AIBridgeGetSessionThreads(ctx, "cursor-test-session", uuid.New(), uuid.Nil, 0)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid pagination cursor")

		// A nonexistent UUID as before_id should also return 400.
		_, err = client.AIBridgeGetSessionThreads(ctx, "cursor-test-session", uuid.Nil, uuid.New(), 0)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid pagination cursor")

		// An interception from a different session should also return 400.
		otherEndedAt := now.Add(time.Minute)
		otherInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "anthropic",
			Model:           "claude-4",
			StartedAt:       now,
			ClientSessionID: sql.NullString{String: "other-session", Valid: true},
		}, &otherEndedAt)

		_, err = client.AIBridgeGetSessionThreads(ctx, "cursor-test-session", otherInterception.ID, uuid.Nil, 0)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid pagination cursor")
		require.Contains(t, sdkErr.Detail, "does not belong to session")
	})

	t.Run("Authorization", func(t *testing.T) {
		t.Parallel()
		ownerClient, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, firstUser.OrganizationID)

		now := dbtime.Now()
		endedAt := now.Add(time.Minute)

		// Create a session owned by the owner.
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "anthropic",
			Model:           "claude-4",
			StartedAt:       now,
			ClientSessionID: sql.NullString{String: "owner-session", Valid: true},
		}, &endedAt)

		// Owner can see their own session.
		res, err := ownerClient.AIBridgeGetSessionThreads(ctx, "owner-session", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)
		require.Equal(t, "owner-session", res.ID)

		// Member cannot see the owner's session.
		_, err = memberClient.AIBridgeGetSessionThreads(ctx, "owner-session", uuid.Nil, uuid.Nil, 0)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		// Create a session owned by the member.
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     member.ID,
			Provider:        "anthropic",
			Model:           "claude-4",
			StartedAt:       now,
			ClientSessionID: sql.NullString{String: "member-session", Valid: true},
		}, &endedAt)

		// Member cannot see their own session either (no read permission).
		_, err = memberClient.AIBridgeGetSessionThreads(ctx, "member-session", uuid.Nil, uuid.Nil, 0)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}
