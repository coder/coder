package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

var (
	uid1 = uuid.MustParse("11111111-0f2b-43e2-b657-d8af99418f79")
	uid2 = uuid.MustParse("22222222-d0b3-4d77-903b-385e6d0c97e4")
	uid3 = uuid.MustParse("33333333-085c-4fcd-9ed3-fca502dd0a03")
	uid4 = uuid.MustParse("44444444-591e-404b-b0cf-a425d74c3e25")
	uid5 = uuid.MustParse("55555555-848a-4afe-93bb-4af9639a1781")
	uid6 = uuid.MustParse("66666666-0870-4194-9e84-32c671e0f879")
	uid7 = uuid.MustParse("77777777-c81f-4cae-a9ac-ae76200f3a9d")
	uid8 = uuid.MustParse("88888888-9f6f-4cb0-9412-c35b1c30c72a")

	t0 = time.Unix(60, 0).UTC()
	t1 = time.Unix(50, 0).UTC()
	t2 = time.Unix(40, 0).UTC()
	t3 = time.Unix(30, 0).UTC()
	t4 = time.Unix(20, 0).UTC()
	t5 = time.Unix(10, 0).UTC()
)

func TestListInterceptions(t *testing.T) {
	t.Parallel()
	t.Run("EmptyDB", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)
		experimentalClient := codersdk.NewExperimentalClient(client)

		ctx := testutil.Context(t, testutil.WaitShort)

		inters, err := experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsRequest{})
		require.NoError(t, err)
		require.Empty(t, inters.Results)
	})

	t.Run("FilteredQueries", func(t *testing.T) {
		t.Parallel()
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		_ = coderdtest.CreateFirstUser(t, client)
		experimentalClient := codersdk.NewExperimentalClient(client)
		ctx := testutil.Context(t, testutil.WaitLong)
		ctx = dbauthz.AsAIBridged(ctx)

		expect := populateDB(ctx, t, db)

		t.Run("IterateCursor", func(t *testing.T) {
			t.Parallel()
			got, err := experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsRequest{
				Limit: 2,
			})
			ex := codersdk.AIBridgeListInterceptionsResponse{
				Results: []codersdk.AIBridgeListInterceptionsResult{expect[uid4], expect[uid3]},
				Cursor: codersdk.AIBridgeListInterceptionsCursor{
					ID:   uid3,
					Time: t2,
				},
			}
			require.NoError(t, err)
			require.Equal(t, ex, got)

			got, err = experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsRequest{
				Cursor: got.Cursor,
				Limit:  1,
			})
			ex = codersdk.AIBridgeListInterceptionsResponse{
				Results: []codersdk.AIBridgeListInterceptionsResult{expect[uid2]},
				Cursor: codersdk.AIBridgeListInterceptionsCursor{
					ID:   uid2,
					Time: t2,
				},
			}
			require.NoError(t, err)
			require.Equal(t, ex, got)

			got, err = experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsRequest{
				Cursor: got.Cursor,
				Limit:  1,
			})
			ex = codersdk.AIBridgeListInterceptionsResponse{
				Results: []codersdk.AIBridgeListInterceptionsResult{expect[uid1]},
				Cursor: codersdk.AIBridgeListInterceptionsCursor{
					ID:   uid1,
					Time: t2,
				},
			}
			require.NoError(t, err)
			require.Equal(t, ex, got)

			got, err = experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsRequest{
				Cursor: got.Cursor,
				Limit:  2,
			})
			ex = codersdk.AIBridgeListInterceptionsResponse{
				Results: []codersdk.AIBridgeListInterceptionsResult{expect[uid8], expect[uid7]},
				Cursor: codersdk.AIBridgeListInterceptionsCursor{
					ID:   uid7,
					Time: t4,
				},
			}
			require.NoError(t, err)
			require.Equal(t, ex, got)

			got, err = experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsRequest{
				Cursor: got.Cursor,
				Limit:  2,
			})
			ex = codersdk.AIBridgeListInterceptionsResponse{
				Results: []codersdk.AIBridgeListInterceptionsResult{expect[uid6], expect[uid5]},
				Cursor: codersdk.AIBridgeListInterceptionsCursor{
					ID:   uid5,
					Time: t5,
				},
			}
			require.NoError(t, err)
			require.Equal(t, ex, got)

			got, err = experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsRequest{
				Cursor: got.Cursor,
				Limit:  2,
			})
			ex = codersdk.AIBridgeListInterceptionsResponse{
				Results: []codersdk.AIBridgeListInterceptionsResult{},
				Cursor:  codersdk.AIBridgeListInterceptionsCursor{},
			}
			require.NoError(t, err)
			require.Equal(t, ex, got)
		})

		tests := []struct {
			name        string
			start       time.Time
			end         time.Time
			curTime     time.Time
			curID       uuid.UUID
			initiatorID uuid.UUID
			limit       int32
			expect      codersdk.AIBridgeListInterceptionsResponse
			expectErr   string
		}{
			{
				name:      "too_large_limit",
				limit:     1001,
				expectErr: "Invalid limit value",
			},
			{
				name:      "negative_limit",
				limit:     -1,
				expectErr: "Invalid limit value",
			},
			{
				name:      "wrong_period",
				start:     time.Unix(10, 0),
				end:       time.Unix(5, 0),
				expectErr: "Invalid time frame",
			},
			{
				name: "no_filter",
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{
						expect[uid4],
						expect[uid3],
						expect[uid2],
						expect[uid1],
						expect[uid8],
						expect[uid7],
						expect[uid6],
						expect[uid5],
					},
					Cursor: codersdk.AIBridgeListInterceptionsCursor{
						ID:   uid5,
						Time: t5,
					},
				},
			},
			{
				name:  "no_filter_limit",
				limit: 2,
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{expect[uid4], expect[uid3]},
					Cursor: codersdk.AIBridgeListInterceptionsCursor{
						ID:   uid3,
						Time: t2,
					},
				},
			},
			{
				name:  "no_match_time_start",
				start: time.Unix(1000, 0),
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{},
				},
			},
			{
				name: "no_match_time_end",
				end:  time.Unix(-1000, 0),
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{},
				},
			},
			{
				name:  "match_time",
				start: t3,
				end:   t1,
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{
						expect[uid3],
						expect[uid2],
						expect[uid1],
						expect[uid8],
					},
					Cursor: codersdk.AIBridgeListInterceptionsCursor{
						ID:   uid8,
						Time: t3,
					},
				},
			},
			{
				name:        "no_match_initiator",
				initiatorID: uid8,
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{},
				},
			},
			{
				name:        "match_initiator",
				initiatorID: uid1,
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{
						expect[uid4],
						expect[uid3],
						expect[uid1],
						expect[uid8],
						expect[uid5],
					},
					Cursor: codersdk.AIBridgeListInterceptionsCursor{
						ID:   uid5,
						Time: t5,
					},
				},
			},
			{
				name:        "match_time_and_initiator",
				initiatorID: uid1,
				start:       t4,
				end:         t1,
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{
						expect[uid3],
						expect[uid1],
						expect[uid8],
					},
					Cursor: codersdk.AIBridgeListInterceptionsCursor{
						ID:   uid8,
						Time: t3,
					},
				},
			},
			{
				name:    "match_cursor_and_time",
				start:   t4,
				end:     t0,
				curTime: t2,
				curID:   uid2,
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{
						expect[uid1],
						expect[uid8],
						expect[uid7],
					},
					Cursor: codersdk.AIBridgeListInterceptionsCursor{
						ID:   uid7,
						Time: t4,
					},
				},
			},
			{
				name:    "match_cursor_time_limit",
				start:   t4,
				end:     t0,
				curTime: t2,
				curID:   uid2,
				limit:   2,
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{
						expect[uid1],
						expect[uid8],
					},
					Cursor: codersdk.AIBridgeListInterceptionsCursor{
						ID:   uid8,
						Time: t3,
					},
				},
			},
			{
				name:        "match_cursor_time_user_limit",
				initiatorID: uid2,
				start:       t4,
				end:         t0,
				curTime:     t2,
				curID:       uid2,
				limit:       2,
				expect: codersdk.AIBridgeListInterceptionsResponse{
					Results: []codersdk.AIBridgeListInterceptionsResult{
						expect[uid7],
					},
					Cursor: codersdk.AIBridgeListInterceptionsCursor{
						ID:   uid7,
						Time: t4,
					},
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				got, err := experimentalClient.AIBridgeListInterceptions(ctx, codersdk.AIBridgeListInterceptionsRequest{
					PeriodStart: tc.start,
					PeriodEnd:   tc.end,
					Cursor: codersdk.AIBridgeListInterceptionsCursor{
						Time: tc.curTime,
						ID:   tc.curID,
					},
					InitiatorID: tc.initiatorID,
					Limit:       tc.limit,
				})
				if tc.expectErr == "" {
					require.NoError(t, err)
					require.Equal(t, tc.expect, got)
				} else {
					require.Error(t, err)
					require.Contains(t, err.Error(), tc.expectErr)
				}
			})
		}
	})
}

type testHelper struct {
	ctx context.Context
	db  database.Store
	t   *testing.T

	expectedResults map[uuid.UUID]codersdk.AIBridgeListInterceptionsResult
}

func populateDB(ctx context.Context, t *testing.T, db database.Store) map[uuid.UUID]codersdk.AIBridgeListInterceptionsResult {
	th := &testHelper{
		ctx:             ctx,
		db:              db,
		t:               t,
		expectedResults: map[uuid.UUID]codersdk.AIBridgeListInterceptionsResult{},
	}

	th.addInterception(t1, uid4, uid1)

	th.addInterception(t2, uid3, uid1)
	th.addInterception(t2, uid2, uid2)
	th.addInterception(t2, uid1, uid1)

	th.addInterception(t3, uid8, uid1)

	th.addInterception(t4, uid7, uid2)

	th.addInterception(t5, uid5, uid1)
	th.addInterception(t5, uid6, uid3)

	th.addToken(uid2, 20, 5)
	th.addToken(uid2, 30, 3)

	th.addToken(uid3, 20, 1)
	th.addToken(uid3, 30, 2)
	th.addToken(uid3, 40, 3)
	th.addToken(uid3, 50, 4)

	th.addPrompt(uid2, "prompt_i2")

	th.addTool(uid2, t1, sql.NullString{Valid: true, String: "server_i2_t2"}, "tool_i2_t2", "input_i2_t2")
	th.addTool(uid2, t2, sql.NullString{Valid: false}, "tool_i2_t3", "input_i2_t3")
	th.addTool(uid2, t3, sql.NullString{Valid: false}, "tool_i2_t1", "input_i2_t1")

	th.addTool(uid3, t4, sql.NullString{Valid: false}, "tool_i3_t1", "input_i3_t1")
	th.addTool(uid3, t5, sql.NullString{Valid: false}, "tool_i3_t2", "input_i3_t2")

	return th.expectedResults
}

func (th *testHelper) addInterception(startedAt time.Time, interID, userID uuid.UUID) {
	i, err := th.db.InsertAIBridgeInterception(th.ctx, database.InsertAIBridgeInterceptionParams{
		ID:          interID,
		InitiatorID: userID,
		Provider:    "provider " + interID.String(),
		Model:       "model " + interID.String(),
		StartedAt:   startedAt,
	})
	require.NoError(th.t, err, "failed to insert interception: %v", err)

	th.expectedResults[interID] = codersdk.AIBridgeListInterceptionsResult{
		InterceptionID: i.ID,
		UserID:         i.InitiatorID,
		Provider:       i.Provider,
		Model:          i.Model,
		StartedAt:      i.StartedAt.UTC(),
		Tokens:         codersdk.AIBridgeListInterceptionsTokens{},
		Tools:          []codersdk.AIBridgeListInterceptionsTool{},
	}
}

func (th *testHelper) addToken(interID uuid.UUID, input, output int64) {
	md, err := json.Marshal(map[string]any{})
	require.NoError(th.t, err, "failed to prepare insert token metadata")

	err = th.db.InsertAIBridgeTokenUsage(th.ctx, database.InsertAIBridgeTokenUsageParams{
		ID:                 uuid.New(),
		InterceptionID:     interID,
		ProviderResponseID: uuid.NewString(),
		InputTokens:        input,
		OutputTokens:       output,
		Metadata:           md,
	})
	require.NoError(th.t, err, "failed to insert tokens: %v", err)

	r := th.expectedResults[interID]
	r.Tokens.Input += input
	r.Tokens.Output += output
	th.expectedResults[interID] = r
}

func (th *testHelper) addTool(interID uuid.UUID, createdAt time.Time, server sql.NullString, tool, input string) {
	md, err := json.Marshal(map[string]any{})
	require.NoError(th.t, err, "failed to prepare insert token metadata")

	params := database.InsertAIBridgeToolUsageParams{
		ID:             uuid.New(),
		InterceptionID: interID,
		Tool:           tool,
		ServerUrl:      server,
		Input:          input,
		CreatedAt:      createdAt,
		Metadata:       md,
	}
	err = th.db.InsertAIBridgeToolUsage(th.ctx, params)
	require.NoError(th.t, err, "failed to insert tools: %v", err)

	r := th.expectedResults[interID]
	r.Tools = append(r.Tools, codersdk.AIBridgeListInterceptionsTool{
		Server: server.String,
		Tool:   tool,
		Input:  input,
	})
	th.expectedResults[interID] = r
}

func (th *testHelper) addPrompt(interID uuid.UUID, prompt string) {
	md, err := json.Marshal(map[string]any{})
	require.NoError(th.t, err, "failed to prepare insert token metadata")

	err = th.db.InsertAIBridgeUserPrompt(th.ctx, database.InsertAIBridgeUserPromptParams{
		ID:             uuid.New(),
		InterceptionID: interID,
		Prompt:         prompt,
		Metadata:       md,
	})
	require.NoError(th.t, err, "failed to insert prompt: %v", err)

	r := th.expectedResults[interID]
	r.Prompt = prompt
	th.expectedResults[interID] = r
}
