package database_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

//nolint:tparallel,paralleltest // Cases share a database before deleting the group.
func TestOrganizationAISpendHourlyMatchesRaw(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	db := database.New(sqlDB)

	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	otherGroup := dbgen.Group(t, db, database.Group{OrganizationID: dbgen.Organization(t, db, database.Organization{}).ID})
	alice := dbgen.User(t, db, database.User{Username: "hourly-alice"})
	bob := dbgen.User(t, db, database.User{Username: "hourly-bob"})
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	type event struct {
		user         database.User
		group        uuid.UUID
		at           time.Time
		provider     string
		providerName string
		model        string
		client       sql.NullString
		cost         sql.NullInt64
	}
	priced := func(n int64) sql.NullInt64 { return sql.NullInt64{Int64: n, Valid: true} }
	client := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	for _, e := range []event{
		{alice, group.ID, start, "openai", "configured-openai", "gpt", sql.NullString{}, priced(100)},
		{alice, group.ID, start.Add(30 * time.Minute), "openai", "configured-openai", "gpt", client("Unknown"), sql.NullInt64{}},
		{alice, group.ID, start.Add(45 * time.Minute), "openai", "configured-openai", "gpt", client(""), priced(0)},
		{alice, group.ID, start.Add(time.Hour), "anthropic", "configured-anthropic", "claude", client("vscode"), priced(200)},
		{bob, group.ID, start.Add(time.Hour + 45*time.Minute + time.Second), "openai", "configured-openai", "gpt", client("cursor"), priced(300)},
		{alice, group.ID, start.Add(2*time.Hour + 30*time.Minute), "anthropic", "configured-anthropic", "claude", sql.NullString{}, sql.NullInt64{}},
		{bob, otherGroup.ID, start.Add(time.Hour), "openai", "configured-openai", "gpt", client("cursor"), priced(9999)},
	} {
		interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: e.user.ID, Provider: e.provider, ProviderName: e.providerName,
			Model: e.model, Client: e.client, StartedAt: start.Add(-24 * time.Hour),
		}, nil)
		for i := 0; i < 2; i++ {
			dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				InterceptionID: interception.ID, CreatedAt: e.at, EffectiveGroupID: uuid.NullUUID{UUID: e.group, Valid: true}, CostMicros: e.cost,
			})
			require.NoError(t, db.IncrementAIBridgeTokenUsageHourly(ctx, database.IncrementAIBridgeTokenUsageHourlyParams{
				CreatedAt: e.at, EffectiveGroupID: e.group, InitiatorID: e.user.ID,
				Provider: e.provider, ProviderName: e.providerName, Model: e.model,
				Client: e.client, CostMicros: e.cost,
			}))
		}
	}

	_, err := sqlDB.ExecContext(ctx, "UPDATE users SET username = 'renamed-hourly-alice', name = 'Renamed', deleted = true WHERE id = $1", alice.ID)
	require.NoError(t, err)

	// This oracle reads only the raw facts, never the hourly table.
	const raw = `
WITH spend AS (
 SELECT ai.initiator_id AS user_id, COALESCE(SUM(tu.cost_micros), 0)::bigint AS cost_micros,
  COUNT(*) FILTER (WHERE tu.cost_micros IS NULL)::bigint AS unpriced_usage_count,
  ARRAY_AGG(DISTINCT ai.provider ORDER BY ai.provider)::text[] AS providers,
  ARRAY_AGG(DISTINCT COALESCE(ai.client, 'Unknown') ORDER BY COALESCE(ai.client, 'Unknown'))::text[] AS clients,
  ARRAY_AGG(DISTINCT ai.model ORDER BY ai.model)::text[] AS models
 FROM aibridge_token_usages tu
 JOIN aibridge_interceptions ai ON ai.id = tu.interception_id
 JOIN groups g ON g.id = tu.effective_group_id
 WHERE g.organization_id = $1 AND tu.created_at >= $2 AND tu.created_at < $3
  AND ($4::text = '' OR ai.provider_name = $4)
  AND ($5::text = '' OR ai.model = $5)
  AND ($6::text = '' OR COALESCE(ai.client, 'Unknown') = $6)
 GROUP BY ai.initiator_id
)
SELECT spend.user_id, u.username, u.name, u.avatar_url, $1::uuid,
 spend.cost_micros, spend.unpriced_usage_count, spend.providers, spend.clients, spend.models,
 COUNT(*) OVER ()::bigint, COALESCE(SUM(spend.cost_micros) OVER (), 0)::bigint,
 COALESCE(SUM(spend.unpriced_usage_count) OVER (), 0)::bigint
FROM spend JOIN users u ON u.id = spend.user_id
ORDER BY spend.cost_micros DESC, LOWER(u.username), spend.user_id
LIMIT NULLIF($7::int, 0) OFFSET $8::int
`
	for _, tc := range []struct {
		name                        string
		from, to                    time.Time
		providerName, model, client string
		limit, offset               int32
	}{
		{"Aligned", start, start.Add(3 * time.Hour), "", "", "", 0, 0},
		{"PartialEdges", start.Add(30 * time.Minute), start.Add(2*time.Hour + 45*time.Minute), "", "", "", 0, 0},
		{"SameHourSubsecond", start.Add(30*time.Minute + time.Nanosecond), start.Add(45*time.Minute + time.Second), "", "", "", 0, 0},
		{"NoCompleteHour", start.Add(45 * time.Minute), start.Add(time.Hour + 30*time.Minute), "", "", "", 0, 0},
		{"ConfiguredProvider", start, start.Add(3 * time.Hour), "configured-openai", "", "", 0, 0},
		{"UnknownClient", start, start.Add(3 * time.Hour), "", "", "Unknown", 0, 0},
		{"EmptyClientUnfiltered", start, start.Add(time.Hour), "", "", "", 0, 0},
		{"CombinedFilters", start, start.Add(3 * time.Hour), "configured-anthropic", "claude", "vscode", 0, 0},
		{"SecondPage", start, start.Add(3 * time.Hour), "", "", "", 1, 1},
		{"EmptyPage", start, start.Add(3 * time.Hour), "", "", "", 1, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := database.ListOrganizationAISpendUsersParams{
				OrganizationID: org.ID,
				PeriodStart:    tc.from, PeriodEnd: tc.to, ProviderName: tc.providerName,
				Model: tc.model, Client: tc.client, LimitOpt: tc.limit, OffsetOpt: tc.offset,
			}
			got, err := db.ListOrganizationAISpendUsers(ctx, params)
			require.NoError(t, err)
			rows, err := sqlDB.QueryContext(ctx, raw, org.ID, tc.from, tc.to, tc.providerName, tc.model, tc.client, tc.limit, tc.offset)
			require.NoError(t, err)
			defer rows.Close()
			var want []database.ListOrganizationAISpendUsersRow
			for rows.Next() {
				var v database.ListOrganizationAISpendUsersRow
				require.NoError(t, rows.Scan(&v.UserID, &v.Username, &v.Name, &v.AvatarURL, &v.OrganizationID,
					&v.CostMicros, &v.UnpricedUsageCount, pq.Array(&v.Providers), pq.Array(&v.Clients), pq.Array(&v.Models),
					&v.Count, &v.TotalCostMicros, &v.TotalUnpricedUsageCount))
				want = append(want, v)
			}
			require.NoError(t, rows.Err())
			require.Equal(t, want, got)
		})
	}
	require.NoError(t, db.DeleteGroupByID(ctx, group.ID))
	rows, err := db.ListOrganizationAISpendUsers(ctx, database.ListOrganizationAISpendUsersParams{
		OrganizationID: org.ID, PeriodStart: start, PeriodEnd: start.Add(3 * time.Hour),
	})
	require.NoError(t, err)
	require.Empty(t, rows)
}
