package focus_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge/focus"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestLoadUsageRecords_Raw(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	orgA := dbgen.Organization(t, db, database.Organization{})
	orgB := dbgen.Organization(t, db, database.Organization{})

	groupA := dbgen.Group(t, db, database.Group{OrganizationID: orgA.ID, Name: "group-a"})
	groupB := dbgen.Group(t, db, database.Group{OrganizationID: orgB.ID, Name: "group-b"})

	user := dbgen.User(t, db, database.User{})

	provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic, Name: "anthropic"})

	periodStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	inBounds := periodStart.Add(time.Hour)

	// (a) In scope: belongs to orgA via groupA.
	inScope := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
		StartedAt: inBounds,
	}, ptrTime(inBounds.Add(time.Second)))
	inScopeUsage := dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: inScope.ID, InputTokens: 10, OutputTokens: 20,
		CreatedAt:        inBounds.Add(time.Second),
		EffectiveGroupID: uuid.NullUUID{UUID: groupA.ID, Valid: true},
		InputPriceMicros: sqlNullInt64(1_000_000),
	})

	// (b) Out of scope: belongs to orgB via groupB.
	otherOrg := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
		StartedAt: inBounds,
	}, ptrTime(inBounds.Add(time.Second)))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: otherOrg.ID, InputTokens: 5, OutputTokens: 5,
		CreatedAt:        inBounds.Add(time.Second),
		EffectiveGroupID: uuid.NullUUID{UUID: groupB.ID, Valid: true},
	})

	// (c) Excluded entirely: NULL effective_group_id.
	noGroup := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
		StartedAt: inBounds,
	}, ptrTime(inBounds.Add(time.Second)))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: noGroup.ID, InputTokens: 1, OutputTokens: 1,
		CreatedAt: inBounds.Add(time.Second),
	})

	records, err := focus.LoadUsageRecords(ctx, db, orgA.ID, periodStart, periodEnd, 0)
	require.NoError(t, err)
	require.Len(t, records, 1, "only the orgA row should be returned")
	require.Equal(t, inScopeUsage.ID, records[0].TokenUsageID)
	require.Equal(t, "Anthropic", records[0].Vendor)
	require.NotNil(t, records[0].BudgetGroup)
	require.Equal(t, groupA.ID, records[0].BudgetGroup.ID)
	require.Len(t, records[0].Usage, 2) // input + output, both non-zero
}

// TestLoadUsageRecords_Rollup_OrgIsolation mirrors
// TestLoadUsageRecords_Raw's org A vs org B coverage, but for the rollup
// query: the raw query's tenant isolation is well-tested, but the rollup
// query has its own hand-written WHERE clause and its own hand-written
// RBACObject wiring, with no equivalent regression coverage before this.
func TestLoadUsageRecords_Rollup_OrgIsolation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	orgA := dbgen.Organization(t, db, database.Organization{})
	orgB := dbgen.Organization(t, db, database.Organization{})

	groupA := dbgen.Group(t, db, database.Group{OrganizationID: orgA.ID, Name: "group-a"})
	groupB := dbgen.Group(t, db, database.Group{OrganizationID: orgB.ID, Name: "group-b"})

	user := dbgen.User(t, db, database.User{})
	provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic, Name: "anthropic"})

	periodStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	at := periodStart.Add(time.Hour)

	inScope := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
		StartedAt: at,
	}, ptrTime(at.Add(time.Second)))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: inScope.ID, InputTokens: 10,
		CreatedAt:        at.Add(time.Second),
		EffectiveGroupID: uuid.NullUUID{UUID: groupA.ID, Valid: true},
	})

	otherOrg := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
		StartedAt: at,
	}, ptrTime(at.Add(time.Second)))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: otherOrg.ID, InputTokens: 999,
		CreatedAt:        at.Add(time.Second),
		EffectiveGroupID: uuid.NullUUID{UUID: groupB.ID, Valid: true},
	})

	records, err := focus.LoadUsageRecords(ctx, db, orgA.ID, periodStart, periodEnd, 3600)
	require.NoError(t, err)
	require.Len(t, records, 1, "only orgA's bucket should be returned")
	require.Equal(t, int64(10), usageForType(t, records[0].Usage, focus.TokenTypeInput).Tokens,
		"orgB's row must not be folded into orgA's bucket")
}

// TestLoadUsageRecords_Raw_BillingPeriodUsesTokenUsageCreatedAt is the
// regression test for the raw-grain half of the BillingPeriodStart/End
// source-column fix: the AI Gateway to FOCUS Mapping table specifies
// token_usages.created_at, not interceptions.started_at, as the source. A
// request that starts in one calendar month but whose response is recorded
// (token_usages.created_at) in the next must be billed to the month it was
// recorded in, not the month it started in.
func TestLoadUsageRecords_Raw_BillingPeriodUsesTokenUsageCreatedAt(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID, Name: "group"})
	user := dbgen.User(t, db, database.User{})
	provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic, Name: "anthropic"})

	// The request starts in the last second of September, but its response
	// (token_usages.created_at) isn't recorded until just after midnight, in
	// October.
	startedAt := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	createdAt := time.Date(2026, 10, 1, 0, 0, 1, 0, time.UTC)
	periodStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)

	interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
		StartedAt: startedAt,
	}, ptrTime(createdAt))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: interception.ID, InputTokens: 10,
		CreatedAt:        createdAt,
		EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
	})

	records, err := focus.LoadUsageRecords(ctx, db, org.ID, periodStart, periodEnd, 0)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.True(t, createdAt.Equal(records[0].BillingPeriodAnchor),
		"want %s (token_usages.created_at), got %s", createdAt, records[0].BillingPeriodAnchor)

	rows := focus.MapUsageRecord(records[0], "coder.example.com")
	require.NotEmpty(t, rows)
	wantPeriodStart := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	wantPeriodEnd := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, row := range rows {
		require.True(t, wantPeriodStart.Equal(row.BillingPeriodStart),
			"a response recorded in October must be billed to October even though its request started in September: want %s, got %s", wantPeriodStart, row.BillingPeriodStart)
		require.True(t, wantPeriodEnd.Equal(row.BillingPeriodEnd))
	}
}

func TestLoadUsageRecords_SoftDeletedProviderNameReuseDoesNotDoubleCount(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID, Name: "group"})
	user := dbgen.User(t, db, database.User{})

	// A soft-deleted provider whose name is later reused by a live provider.
	deleted := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeOpenai, Name: "shared-name"})
	require.NoError(t, db.DeleteAIProviderByID(ctx, deleted.ID))
	live := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic, Name: "shared-name"})

	periodStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	at := periodStart.Add(time.Hour)

	interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "anthropic", ProviderName: live.Name, Model: "claude-haiku-4-5",
		StartedAt: at,
	}, ptrTime(at.Add(time.Second)))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: interception.ID, InputTokens: 10, OutputTokens: 10,
		CreatedAt:        at.Add(time.Second),
		EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
	})

	records, err := focus.LoadUsageRecords(ctx, db, org.ID, periodStart, periodEnd, 0)
	require.NoError(t, err)
	require.Len(t, records, 1, "the LEFT JOIN on provider.deleted = false must resolve to exactly the live row, not fan out across both")
	require.Equal(t, "Anthropic", records[0].Vendor)
}

func TestLoadUsageRecords_Rollup(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID, Name: "group"})
	user := dbgen.User(t, db, database.User{})
	provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic, Name: "anthropic"})

	periodStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	hour := periodStart.Add(3 * time.Hour)

	newInterception := func(at time.Time) database.AIBridgeInterception {
		return dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
			StartedAt: at,
		}, ptrTime(at.Add(time.Second)))
	}

	// Two responses in the same UTC hour, same dimensions and price: expected
	// to fold into one rolled-up row.
	i1 := newInterception(hour.Add(5 * time.Minute))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: i1.ID, InputTokens: 100, OutputTokens: 0,
		CreatedAt:        hour.Add(5 * time.Minute),
		EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
		InputPriceMicros: sqlNullInt64(1_000_000),
	})
	i2 := newInterception(hour.Add(40 * time.Minute))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: i2.ID, InputTokens: 50, OutputTokens: 0,
		CreatedAt:        hour.Add(40 * time.Minute),
		EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
		InputPriceMicros: sqlNullInt64(1_000_000),
	})

	// A third response in the next hour must land in a separate bucket.
	i3 := newInterception(hour.Add(90 * time.Minute))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: i3.ID, InputTokens: 999, OutputTokens: 0,
		CreatedAt:        hour.Add(90 * time.Minute),
		EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
		InputPriceMicros: sqlNullInt64(1_000_000),
	})

	records, err := focus.LoadUsageRecords(ctx, db, org.ID, periodStart, periodEnd, 3600)
	require.NoError(t, err)
	require.Len(t, records, 2, "two hourly buckets")

	var foldedBucket, nextBucket *focus.UsageRecord
	for i := range records {
		if records[i].RowCount == 2 {
			foldedBucket = &records[i]
		} else {
			nextBucket = &records[i]
		}
	}
	require.NotNil(t, foldedBucket, "expected one bucket folding the two same-hour responses")
	require.NotNil(t, nextBucket)

	foldedInput := usageForType(t, foldedBucket.Usage, focus.TokenTypeInput)
	require.Equal(t, int64(150), foldedInput.Tokens, "150 = 100 + 50 input tokens summed within the bucket")
	// Two distinct interceptions folded together: the response-level
	// identifiers are not well-defined and must be omitted.
	require.Equal(t, uuid.Nil, foldedBucket.InterceptionID)
	require.Equal(t, "", foldedBucket.SessionID)

	require.Equal(t, int64(1), nextBucket.RowCount)
	require.Equal(t, int64(999), usageForType(t, nextBucket.Usage, focus.TokenTypeInput).Tokens)
	// A single-response bucket still has a well-defined interception.
	require.Equal(t, i3.ID, nextBucket.InterceptionID)

	// The two buckets must never collide on their synthetic ResourceId.
	require.NotEqual(t, foldedBucket.TokenUsageID, nextBucket.TokenUsageID)

	// Re-running the same query must reproduce the same synthetic ResourceId
	// for each bucket (the export's determinism requirement), matched
	// pairwise by BucketStart rather than by set membership: a determinism
	// regression that returned the same ResourceId twice for run 2 would
	// still pass a set-membership check, but must fail this one.
	again, err := focus.LoadUsageRecords(ctx, db, org.ID, periodStart, periodEnd, 3600)
	require.NoError(t, err)
	require.Len(t, again, 2)
	byBucketStart := func(recs []focus.UsageRecord) map[time.Time]uuid.UUID {
		m := make(map[time.Time]uuid.UUID, len(recs))
		for _, r := range recs {
			m[r.StartedAt] = r.TokenUsageID
		}
		return m
	}
	first := byBucketStart(records)
	second := byBucketStart(again)
	require.Equal(t, first, second, "each bucket must reproduce the exact same ResourceId across runs")
}

// TestLoadUsageRecords_Rollup_PreservesIdentifierWhenUnambiguous covers the
// complement of the fold case above: multiple token-usage rows sharing a
// single interception (so COUNT(DISTINCT interception_id) == 1) must retain
// that interception's identifier, not null it out. Without this, a
// regression that unconditionally nulled the response-level identifiers
// would still pass every other rollup test in this file.
func TestLoadUsageRecords_Rollup_PreservesIdentifierWhenUnambiguous(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID, Name: "group"})
	user := dbgen.User(t, db, database.User{})
	provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic, Name: "anthropic"})

	periodStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	hour := periodStart.Add(3 * time.Hour)

	// One interception, two token-usage rows (e.g. an input pass and a
	// separate output pass on the same response) in the same bucket.
	interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
		StartedAt: hour.Add(5 * time.Minute),
	}, ptrTime(hour.Add(6*time.Minute)))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: interception.ID, InputTokens: 100, OutputTokens: 0,
		CreatedAt:         hour.Add(5 * time.Minute),
		EffectiveGroupID:  uuid.NullUUID{UUID: group.ID, Valid: true},
		InputPriceMicros:  sqlNullInt64(1_000_000),
		OutputPriceMicros: sqlNullInt64(2_000_000),
	})
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: interception.ID, InputTokens: 0, OutputTokens: 200,
		CreatedAt:         hour.Add(10 * time.Minute),
		EffectiveGroupID:  uuid.NullUUID{UUID: group.ID, Valid: true},
		InputPriceMicros:  sqlNullInt64(1_000_000),
		OutputPriceMicros: sqlNullInt64(2_000_000),
	})

	records, err := focus.LoadUsageRecords(ctx, db, org.ID, periodStart, periodEnd, 3600)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, int64(2), records[0].RowCount)
	require.Equal(t, interception.ID, records[0].InterceptionID,
		"a bucket whose rows share one interception must preserve it, not null it out")
}

// TestLoadUsageRecords_Rollup_DiffersAcrossCacheWritePrice is the regression
// test for a ResourceId collision bug: cache_write_price_micros is part of
// the SQL GROUP BY key (it can legitimately differ between two buckets that
// otherwise share every other dimension, e.g. a mid-window price-book
// update), so two such buckets must resolve to distinct synthetic
// ResourceIds. A hand-maintained hash key previously omitted this column and
// collided.
//
// Both interceptions are placed in the SAME hourly bucket, sharing every
// other dimension, so cache_write_price_micros is the only thing that can
// possibly force them into separate rows: with two different buckets
// (bucket_start included in the key either way), this test would still pass
// against the old, broken key that omitted cache_write_price_micros.
func TestLoadUsageRecords_Rollup_DiffersAcrossCacheWritePrice(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID, Name: "group"})
	user := dbgen.User(t, db, database.User{})
	provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic, Name: "anthropic"})

	periodStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	hour := periodStart.Add(3 * time.Hour)

	newInterception := func(at time.Time) database.AIBridgeInterception {
		return dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
			StartedAt: at,
		}, ptrTime(at.Add(time.Second)))
	}

	iA := newInterception(hour.Add(5 * time.Minute))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: iA.ID, CacheWriteInputTokens: 100,
		CreatedAt:             hour.Add(5 * time.Minute),
		EffectiveGroupID:      uuid.NullUUID{UUID: group.ID, Valid: true},
		CacheWritePriceMicros: sqlNullInt64(1_000_000),
	})
	iB := newInterception(hour.Add(10 * time.Minute))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: iB.ID, CacheWriteInputTokens: 100,
		CreatedAt:             hour.Add(10 * time.Minute),
		EffectiveGroupID:      uuid.NullUUID{UUID: group.ID, Valid: true},
		CacheWritePriceMicros: sqlNullInt64(2_000_000),
	})

	records, err := focus.LoadUsageRecords(ctx, db, org.ID, periodStart, periodEnd, 3600)
	require.NoError(t, err)
	require.Len(t, records, 2, "one hourly bucket, split into two rows by cache_write_price_micros alone")
	require.True(t, records[0].StartedAt.Equal(records[1].StartedAt),
		"both rows must share the same bucket start; a difference here would let bucket_start, not price, explain distinct ResourceIds")
	require.NotEqual(t, records[0].TokenUsageID, records[1].TokenUsageID,
		"rows in the same bucket that differ only in cache_write_price_micros must not share a synthetic ResourceId")
}

// TestLoadUsageRecords_Rollup_BillingPeriodMatchesBucketMonth is the
// regression test for a billing-period bug: the last rollup bucket of any
// month has an exclusive end that falls on the first instant of the next
// month. Deriving BillingPeriodStart/End from that end (rather than the
// bucket's start) stamped the final hour of every month as spend in the
// following month's billing period.
func TestLoadUsageRecords_Rollup_BillingPeriodMatchesBucketMonth(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	org := dbgen.Organization(t, db, database.Organization{})
	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID, Name: "group"})
	user := dbgen.User(t, db, database.User{})
	provider := dbgen.AIProvider(t, db, database.AIProvider{Type: database.AIProviderTypeAnthropic, Name: "anthropic"})

	// The final hourly bucket of September: [2026-09-30T23:00:00Z,
	// 2026-10-01T00:00:00Z).
	lastHourStart := time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC)
	periodStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: user.ID, Provider: "anthropic", ProviderName: provider.Name, Model: "claude-haiku-4-5",
		StartedAt: lastHourStart.Add(5 * time.Minute),
	}, ptrTime(lastHourStart.Add(6*time.Minute)))
	dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
		InterceptionID: interception.ID, InputTokens: 100,
		CreatedAt:        lastHourStart.Add(5 * time.Minute),
		EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
	})

	records, err := focus.LoadUsageRecords(ctx, db, org.ID, periodStart, periodEnd, 3600)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.True(t, lastHourStart.Equal(records[0].StartedAt), "want %s, got %s", lastHourStart, records[0].StartedAt)
	require.True(t, periodEnd.Equal(records[0].EndedAt), "clamped to periodEnd since the bucket's raw end is exclusive of the requested period")

	rows := focus.MapUsageRecord(records[0], "coder.example.com")
	require.NotEmpty(t, rows)
	wantPeriodStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	wantPeriodEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, row := range rows {
		require.True(t, wantPeriodStart.Equal(row.BillingPeriodStart),
			"the final bucket of September must still be stamped as September spend: want %s, got %s", wantPeriodStart, row.BillingPeriodStart)
		require.True(t, wantPeriodEnd.Equal(row.BillingPeriodEnd), "want %s, got %s", wantPeriodEnd, row.BillingPeriodEnd)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func sqlNullInt64(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

func usageForType(t *testing.T, usage []focus.TokenUsage, tokenType focus.TokenType) focus.TokenUsage {
	t.Helper()
	for _, u := range usage {
		if u.Type == tokenType {
			return u
		}
	}
	t.Fatalf("no %s token usage found in %+v", tokenType, usage)
	return focus.TokenUsage{}
}
