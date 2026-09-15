package focus

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

// baseRollupRow is a fully-populated GetOrganizationAIFOCUSUsageRollupRow used
// as the starting point for the tests below, which each mutate exactly one
// field and check whether rollupResourceID changes.
func baseRollupRow() database.GetOrganizationAIFOCUSUsageRollupRow {
	return database.GetOrganizationAIFOCUSUsageRollupRow{
		BucketStart:           time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC),
		InitiatorID:           uuid.MustParse("a26a1adc-48a9-4823-a58d-8e0b7e9c60f2"),
		InitiatorUsername:     "callum",
		EffectiveGroupID:      uuid.NullUUID{UUID: uuid.MustParse("3ea6ae38-ff77-446a-ad06-fb1ba0c9e1b1"), Valid: true},
		GroupName:             "Vibes",
		OrganizationID:        uuid.MustParse("bafc64b2-1276-4518-8cd7-d8133eb6b789"),
		Model:                 "claude-haiku-4-5",
		WireProtocol:          "anthropic",
		ProviderInstance:      "anthropic",
		CredentialKind:        database.CredentialKindCentralized,
		VendorType:            database.NullAIProviderType{AIProviderType: database.AIProviderTypeAnthropic, Valid: true},
		VendorDisplayName:     sql.NullString{String: "anthropic", Valid: true},
		ClientTool:            sql.NullString{String: "Mux", Valid: true},
		InputPriceMicros:      sql.NullInt64{Int64: 1_000_000, Valid: true},
		OutputPriceMicros:     sql.NullInt64{Int64: 5_000_000, Valid: true},
		CacheReadPriceMicros:  sql.NullInt64{Int64: 100_000, Valid: true},
		CacheWritePriceMicros: sql.NullInt64{Int64: 1_250_000, Valid: true},
		InterceptionIDMin:     uuid.MustParse("edeea219-190d-4494-b2f1-0f5746639178"),
		SessionIDMin:          "edeea219-190d-4494-b2f1-0f5746639178",
		ProviderResponseIDMin: "msg_01Bk9iY843EAmrQYyYVgVZmP",
		RowCount:              2,
		InputTokens:           3,
		OutputTokens:          51,
		CacheReadInputTokens:  0,
		CacheWriteInputTokens: 9278,
	}
}

// TestRollupResourceID_MeasuresDoNotAffectResourceID is the regression test
// referenced by rollupGroupKeyExcludedFields' doc comment: mutating a field
// that is an aggregate computed OVER the GROUP BY key (a *_min/distinct_count
// identifier pair, row_count, or a summed token count) must never change the
// synthetic ResourceId. If one of these leaked into the key, the "same
// bucket, same ResourceId" determinism guarantee would flap on quantity
// alone, not just on a genuine dimension change.
//
//nolint:testpackage // exercises the unexported rollupResourceID directly.
func TestRollupResourceID_MeasuresDoNotAffectResourceID(t *testing.T) {
	t.Parallel()

	base := baseRollupRow()
	want := rollupResourceID(base.OrganizationID, 3600, base)

	mutations := map[string]func(r *database.GetOrganizationAIFOCUSUsageRollupRow){
		"InputTokens":                     func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.InputTokens = 999_999 },
		"OutputTokens":                    func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.OutputTokens = 999_999 },
		"CacheReadInputTokens":            func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.CacheReadInputTokens = 999_999 },
		"CacheWriteInputTokens":           func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.CacheWriteInputTokens = 999_999 },
		"RowCount":                        func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.RowCount = 999 },
		"InterceptionIDMin":               func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.InterceptionIDMin = uuid.New() },
		"InterceptionIDDistinctCount":     func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.InterceptionIDDistinctCount = 999 },
		"SessionIDMin":                    func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.SessionIDMin = uuid.NewString() },
		"SessionIDDistinctCount":          func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.SessionIDDistinctCount = 999 },
		"ProviderResponseIDMin":           func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.ProviderResponseIDMin = "different" },
		"ProviderResponseIDDistinctCount": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.ProviderResponseIDDistinctCount = 999 },
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			row := baseRollupRow()
			mutate(&row)
			got := rollupResourceID(row.OrganizationID, 3600, row)
			require.Equal(t, want, got, "mutating measure field %s must not change ResourceId", name)
		})
	}
}

// TestRollupResourceID_ExcludedLabelFieldsDoNotAffectResourceID is the
// regression test for a rename-stability fix: InitiatorUsername, GroupName,
// and VendorDisplayName are mutable, human-readable labels functionally
// determined by an ID already in the key (InitiatorID, EffectiveGroupID, and
// ProviderInstance/VendorType respectively), so a rename of a user, group, or
// provider display name must not change a bucket's ResourceId. Without this
// exclusion, re-exporting the same historical billing period after any of
// those three renames would silently break the "same bucket, same
// ResourceId" determinism guarantee.
//
//nolint:testpackage // exercises the unexported rollupResourceID directly.
func TestRollupResourceID_ExcludedLabelFieldsDoNotAffectResourceID(t *testing.T) {
	t.Parallel()

	base := baseRollupRow()
	want := rollupResourceID(base.OrganizationID, 3600, base)

	mutations := map[string]func(r *database.GetOrganizationAIFOCUSUsageRollupRow){
		"InitiatorUsername": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.InitiatorUsername = "renamed-user" },
		"GroupName":         func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.GroupName = "renamed-group" },
		"VendorDisplayName": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) {
			r.VendorDisplayName = sql.NullString{String: "renamed-display-name", Valid: true}
		},
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			row := baseRollupRow()
			mutate(&row)
			got := rollupResourceID(row.OrganizationID, 3600, row)
			require.Equal(t, want, got, "renaming %s must not change ResourceId", name)
		})
	}
}

// TestRollupResourceID_DimensionsDoAffectResourceID is the complement of the
// two tests above: every field NOT in rollupGroupKeyExcludedFields is a real
// GROUP BY dimension, and mutating any one of them must change the
// ResourceId. This is the mechanical guarantee rollupGroupKey's reflection
// approach is supposed to provide; it is what would have caught the
// CacheWritePriceMicros collision this whole mechanism replaced.
//
//nolint:testpackage // exercises the unexported rollupResourceID directly.
func TestRollupResourceID_DimensionsDoAffectResourceID(t *testing.T) {
	t.Parallel()

	base := baseRollupRow()
	want := rollupResourceID(base.OrganizationID, 3600, base)

	mutations := map[string]func(r *database.GetOrganizationAIFOCUSUsageRollupRow){
		"BucketStart": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.BucketStart = r.BucketStart.Add(time.Hour) },
		"InitiatorID": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.InitiatorID = uuid.New() },
		"EffectiveGroupID": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) {
			r.EffectiveGroupID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		},
		"Model":            func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.Model = "different-model" },
		"WireProtocol":     func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.WireProtocol = "openai" },
		"ProviderInstance": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.ProviderInstance = "different-instance" },
		"CredentialKind":   func(r *database.GetOrganizationAIFOCUSUsageRollupRow) { r.CredentialKind = database.CredentialKindByok },
		"VendorType": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) {
			r.VendorType = database.NullAIProviderType{AIProviderType: database.AIProviderTypeOpenai, Valid: true}
		},
		"ClientTool": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) {
			r.ClientTool = sql.NullString{String: "Different", Valid: true}
		},
		"ErrorType": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) {
			r.ErrorType = database.NullAIBridgeInterceptionErrorType{AIBridgeInterceptionErrorType: database.AibridgeInterceptionErrorTypeServerError, Valid: true}
		},
		"InputPriceMicros": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) {
			r.InputPriceMicros = sql.NullInt64{Int64: 999, Valid: true}
		},
		"OutputPriceMicros": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) {
			r.OutputPriceMicros = sql.NullInt64{Int64: 999, Valid: true}
		},
		"CacheReadPriceMicros": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) {
			r.CacheReadPriceMicros = sql.NullInt64{Int64: 999, Valid: true}
		},
		"CacheWritePriceMicros": func(r *database.GetOrganizationAIFOCUSUsageRollupRow) {
			r.CacheWritePriceMicros = sql.NullInt64{Int64: 999, Valid: true}
		},
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			row := baseRollupRow()
			mutate(&row)
			got := rollupResourceID(row.OrganizationID, 3600, row)
			require.NotEqual(t, want, got, "mutating dimension field %s must change ResourceId", name)
		})
	}

	t.Run("GranularitySeconds", func(t *testing.T) {
		t.Parallel()
		row := baseRollupRow()
		got := rollupResourceID(row.OrganizationID, 7200, row)
		require.NotEqual(t, want, got, "a different granularity must change ResourceId")
	})

	t.Run("OrganizationID", func(t *testing.T) {
		t.Parallel()
		row := baseRollupRow()
		got := rollupResourceID(uuid.New(), 3600, row)
		require.NotEqual(t, want, got, "a different organization must change ResourceId")
	})
}
