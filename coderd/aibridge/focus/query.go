package focus

import (
	"context"
	"crypto/sha1" //nolint:gosec // used only as a deterministic (non-cryptographic) UUIDv5 name-hash input, not for security.
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// Store is the subset of database.Store needed to load FOCUS usage records.
type Store interface {
	GetOrganizationAIFOCUSUsage(ctx context.Context, arg database.GetOrganizationAIFOCUSUsageParams) ([]database.GetOrganizationAIFOCUSUsageRow, error)
	GetOrganizationAIFOCUSUsageRollup(ctx context.Context, arg database.GetOrganizationAIFOCUSUsageRollupParams) ([]database.GetOrganizationAIFOCUSUsageRollupRow, error)
}

// MaxGranularitySeconds bounds the rollup bucket width to at most one
// calendar day. It does NOT guarantee a bucket stays within a single
// calendar month: buckets are anchored to the UNIX epoch in UTC (see
// GetOrganizationAIFOCUSUsageRollup), so the final bucket of every month
// straddles the month boundary at any granularity, including granularities
// that evenly divide a day. BillingPeriodStart/End is derived from each
// record's bucket-start instant specifically to make this deterministic
// (row.go's MapUsageRecord), not from the bucket's exclusive end. At large
// granularities (e.g. tens of thousands of seconds) a single bucket can span
// most of two calendar months; its spend is then wholly attributed to the
// month containing the bucket's start. That is an accepted trade-off of
// coarse rollups, not a bug: callers who need month-accurate billing periods
// should request a granularity of at most a few hours.
const MaxGranularitySeconds = 24 * 60 * 60

// rollupUUIDNamespace namespaces the deterministic synthetic ResourceId
// generated for a rolled-up UsageRecord (see loadRollupUsageRecords).
// Generated once and fixed; changing it would change every rolled-up
// ResourceId, which is intentionally treated the same as any other
// mapping-version change.
var rollupUUIDNamespace = uuid.MustParse("6a6c9b7e-8f4b-4f1c-9c0e-6a6c9b7e8f4b")

// LoadUsageRecords returns AI Gateway usage for the organization over
// [periodStart, periodEnd), resolved into UsageRecords ready for
// MapUsageRecord.
//
// granularitySeconds selects the row grain: 0 (or negative) requests raw,
// unrolled per-response grain (one UsageRecord per priced provider
// response); a positive value requests rollup into fixed-width UTC time
// buckets of that many seconds (e.g. 3600 for hourly), up to
// MaxGranularitySeconds.
func LoadUsageRecords(ctx context.Context, store Store, organizationID uuid.UUID, periodStart, periodEnd time.Time, granularitySeconds int64) ([]UsageRecord, error) {
	if granularitySeconds <= 0 {
		return loadRawUsageRecords(ctx, store, organizationID, periodStart, periodEnd)
	}
	return loadRollupUsageRecords(ctx, store, organizationID, periodStart, periodEnd, granularitySeconds)
}

func loadRawUsageRecords(ctx context.Context, store Store, organizationID uuid.UUID, periodStart, periodEnd time.Time) ([]UsageRecord, error) {
	rows, err := store.GetOrganizationAIFOCUSUsage(ctx, database.GetOrganizationAIFOCUSUsageParams{
		OrganizationID: organizationID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
	})
	if err != nil {
		return nil, xerrors.Errorf("load raw FOCUS usage: %w", err)
	}

	records := make([]UsageRecord, 0, len(rows))
	for _, row := range rows {
		vendor := resolveVendor(nullProviderTypePtr(row.VendorType), nullStringPtr(row.VendorDisplayName), row.WireProtocol, row.Model)
		rec := UsageRecord{
			TokenUsageID:   row.TokenUsageID,
			ResponseID:     row.ProviderResponseID,
			InterceptionID: row.InterceptionID,
			SessionID:      row.SessionID,
			ClientTool:     nullStringOr(row.ClientTool, "Unknown"),
			ErrorType:      nullErrorTypePtr(row.ErrorType),
			StartedAt:      row.StartedAt,
			EndedAt:        row.EndedAt,
			// token_usages.created_at, per the AI Gateway to FOCUS Mapping
			// table's BillingPeriodStart/End source column (see
			// UsageRecord.BillingPeriodAnchor). Already selected by this
			// query for exactly this purpose.
			BillingPeriodAnchor: row.CreatedAt,
			InitiatorID:         row.InitiatorID,
			InitiatorUsername:   row.InitiatorUsername,
			Model:               row.Model,
			Vendor:              vendor.Label,
			VendorTypeRaw:       vendorTypeRaw(nullProviderTypePtr(row.VendorType), row.WireProtocol),
			Publisher:           vendor.Publisher,
			WireProtocol:        row.WireProtocol,
			ProviderInstance:    row.ProviderInstance,
			Credential:          row.CredentialKind,
			Usage: tokenUsagesFromCounts(
				row.InputTokens, row.OutputTokens, row.CacheReadInputTokens, row.CacheWriteInputTokens,
				row.InputPriceMicros, row.OutputPriceMicros, row.CacheReadPriceMicros, row.CacheWritePriceMicros,
			),
			RowCount: 1,
		}
		if row.EffectiveGroupID.Valid {
			rec.BudgetGroup = &BudgetGroup{ID: row.EffectiveGroupID.UUID, Name: row.GroupName}
		}
		records = append(records, rec)
	}
	return records, nil
}

func loadRollupUsageRecords(ctx context.Context, store Store, organizationID uuid.UUID, periodStart, periodEnd time.Time, granularitySeconds int64) ([]UsageRecord, error) {
	if granularitySeconds > MaxGranularitySeconds {
		return nil, xerrors.Errorf("granularity_seconds %d exceeds the maximum of %d", granularitySeconds, MaxGranularitySeconds)
	}

	rows, err := store.GetOrganizationAIFOCUSUsageRollup(ctx, database.GetOrganizationAIFOCUSUsageRollupParams{
		GranularitySeconds: granularitySeconds,
		OrganizationID:     organizationID,
		PeriodStart:        periodStart,
		PeriodEnd:          periodEnd,
	})
	if err != nil {
		return nil, xerrors.Errorf("load rolled-up FOCUS usage: %w", err)
	}

	records := make([]UsageRecord, 0, len(rows))
	for _, row := range rows {
		// bucketStart/bucketEnd are clamped symmetrically to the requested
		// period: Postgres floors bucket_start to the epoch-anchored boundary
		// regardless of where periodStart falls inside that bucket, so without
		// this the first bucket of a request can report a ChargePeriodStart
		// earlier than periodStart and claim to cover time the export was
		// never asked for and holds no data for.
		bucketStart := row.BucketStart
		if bucketStart.Before(periodStart) {
			bucketStart = periodStart
		}
		bucketEnd := row.BucketStart.Add(time.Duration(granularitySeconds) * time.Second)
		if bucketEnd.After(periodEnd) {
			bucketEnd = periodEnd
		}

		vendor := resolveVendor(nullProviderTypePtr(row.VendorType), nullStringPtr(row.VendorDisplayName), row.WireProtocol, row.Model)
		rec := UsageRecord{
			TokenUsageID: rollupResourceID(organizationID, granularitySeconds, row),
			ClientTool:   nullStringOr(row.ClientTool, "Unknown"),
			ErrorType:    nullErrorTypePtr(row.ErrorType),
			StartedAt:    bucketStart,
			EndedAt:      bucketEnd,
			// The bucket's (clamped) start: there is no per-bucket analog of
			// token_usages.created_at once more than one response has been
			// folded together, so the bucket's start is the least-wrong
			// single instant to anchor the billing period to (see
			// UsageRecord.BillingPeriodAnchor). Using bucketEnd instead would
			// misattribute the final bucket of every month to the following
			// month, since bucketEnd is an exclusive boundary that can land
			// on the first instant of the next month.
			BillingPeriodAnchor: bucketStart,
			InitiatorID:         row.InitiatorID,
			InitiatorUsername:   row.InitiatorUsername,
			Model:               row.Model,
			Vendor:              vendor.Label,
			VendorTypeRaw:       vendorTypeRaw(nullProviderTypePtr(row.VendorType), row.WireProtocol),
			Publisher:           vendor.Publisher,
			WireProtocol:        row.WireProtocol,
			ProviderInstance:    row.ProviderInstance,
			Credential:          row.CredentialKind,
			Usage: tokenUsagesFromCounts(
				row.InputTokens, row.OutputTokens, row.CacheReadInputTokens, row.CacheWriteInputTokens,
				row.InputPriceMicros, row.OutputPriceMicros, row.CacheReadPriceMicros, row.CacheWritePriceMicros,
			),
			RowCount: row.RowCount,
		}
		if row.InterceptionIDDistinctCount == 1 {
			rec.InterceptionID = row.InterceptionIDMin
		}
		if row.SessionIDDistinctCount == 1 {
			rec.SessionID = row.SessionIDMin
		}
		if row.ProviderResponseIDDistinctCount == 1 {
			rec.ResponseID = row.ProviderResponseIDMin
		}
		if row.EffectiveGroupID.Valid {
			rec.BudgetGroup = &BudgetGroup{ID: row.EffectiveGroupID.UUID, Name: row.GroupName}
		}
		records = append(records, rec)
	}
	return records, nil
}

// rollupResourceID deterministically derives a synthetic ResourceId for a
// rolled-up UsageRecord from its full grouping key, so re-exporting the same
// billing period at the same granularity and mapping version reproduces the
// same ResourceId every time (the export's determinism requirement), and two
// buckets that differ in any GROUP BY dimension get distinct ResourceIds.
// This holds across a user, group, or provider display-name rename: the key
// folds in IDs (InitiatorID, EffectiveGroupID, ProviderInstance/VendorType),
// never their mutable display labels (see rollupGroupKeyExcludedFields), so
// renaming any of them does not change a bucket's ResourceId.
func rollupResourceID(organizationID uuid.UUID, granularitySeconds int64, row database.GetOrganizationAIFOCUSUsageRollupRow) uuid.UUID {
	key := rollupGroupKey(organizationID, granularitySeconds, row)
	return uuid.NewHash(sha1.New(), rollupUUIDNamespace, []byte(key), 5) //nolint:gosec // deterministic name-hash, not a security boundary.
}

// rollupGroupKeyExcludedFields names every GetOrganizationAIFOCUSUsageRollupRow
// field rollupGroupKey must NOT fold into the hash key, for one of two
// reasons:
//
//  1. It is an aggregate computed OVER the GROUP BY key rather than a member
//     OF it: the three MIN(...)/COUNT(DISTINCT ...) identifier pairs,
//     row_count, and the four summed token counts. Including one of these
//     would make the "same bucket, same ResourceId" determinism guarantee
//     flap on quantity alone instead of only on a genuine dimension change
//     (TestRollupResourceID_MeasuresDoNotAffectResourceID, query_internal_test.go).
//  2. It is a mutable, human-readable display label that is functionally
//     redundant with an ID already folded into the key: InitiatorUsername
//     with InitiatorID, GroupName with EffectiveGroupID, VendorDisplayName
//     with ProviderInstance/VendorType. Folding these in as well would make a
//     user rename, group rename, or provider display-name edit change the
//     ResourceId of every historical bucket that user/group/provider
//     appears in, without changing anything the bucket actually
//     aggregates over (TestRollupResourceID_ExcludedLabelFieldsDoNotAffectResourceID,
//     query_internal_test.go).
//
// rollupGroupKey folds in every field NOT named here, so it never needs to
// be told about a new GROUP BY dimension by name; it only needs to be told
// when a new aggregate or redundant display label is added. A hand-maintained
// allowlist of dimensions drifted silently once already (CacheWritePriceMicros
// was grouped on in SQL but missing from the old hash key, colliding
// ResourceIds across distinct cache-write prices); this denylist is far
// shorter and, if it drifts by omission, fails loudly instead of silently.
var rollupGroupKeyExcludedFields = map[string]bool{
	// Aggregates (reason 1, above).
	"InterceptionIDMin":               true,
	"InterceptionIDDistinctCount":     true,
	"SessionIDMin":                    true,
	"SessionIDDistinctCount":          true,
	"ProviderResponseIDMin":           true,
	"ProviderResponseIDDistinctCount": true,
	"RowCount":                        true,
	"InputTokens":                     true,
	"OutputTokens":                    true,
	"CacheReadInputTokens":            true,
	"CacheWriteInputTokens":           true,
	// Mutable display labels redundant with an ID already in the key
	// (reason 2, above).
	"InitiatorUsername": true,
	"GroupName":         true,
	"VendorDisplayName": true,
}

// rollupGroupKey stringifies organizationID, granularitySeconds, and every
// GROUP BY dimension field of row (i.e. every field not in
// rollupGroupKeyExcludedFields), via reflection, in the row struct's
// declared field order. Deriving the key mechanically from the row's own
// fields, instead of naming each dimension by hand, is the fix for the
// collision described on rollupGroupKeyExcludedFields: a future dimension
// added to the SQL SELECT/GROUP BY list is automatically included the
// moment sqlc regenerates the row struct, with no second call site to
// remember to update.
func rollupGroupKey(organizationID uuid.UUID, granularitySeconds int64, row database.GetOrganizationAIFOCUSUsageRollupRow) string {
	var b strings.Builder
	// strings.Builder's Write* methods never return an error (see its own
	// docs), so these results are safe to discard.
	_, _ = fmt.Fprintf(&b, "%s|%d", organizationID, granularitySeconds)
	v := reflect.ValueOf(row)
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			// sqlc only emits exported fields on generated row structs, so
			// this never fires today; guarded because Interface() panics on
			// an unexported field, and this runs on a request path.
			continue
		}
		if rollupGroupKeyExcludedFields[field.Name] {
			continue
		}
		val := v.Field(i).Interface()
		if ts, ok := val.(time.Time); ok {
			// Normalize location and precision so the same instant always
			// stringifies identically regardless of the driver-returned
			// time.Time's monotonic reading or zone.
			val = ts.UTC().Format(time.RFC3339Nano)
		}
		_ = b.WriteByte('|')
		_, _ = fmt.Fprintf(&b, "%v", val)
	}
	return b.String()
}

// tokenUsagesFromCounts builds the non-zero TokenUsage entries for a
// UsageRecord from its four raw counts and matching *_price_micros snapshots.
func tokenUsagesFromCounts(input, output, cacheRead, cacheWrite int64, inputPrice, outputPrice, cacheReadPrice, cacheWritePrice sql.NullInt64) []TokenUsage {
	var usage []TokenUsage
	add := func(tokenType TokenType, tokens int64, price sql.NullInt64) {
		if tokens == 0 {
			return
		}
		tu := TokenUsage{Type: tokenType, Tokens: tokens}
		if price.Valid {
			v := price.Int64
			tu.UnitPriceMicros = &v
		}
		usage = append(usage, tu)
	}
	add(TokenTypeInput, input, inputPrice)
	add(TokenTypeOutput, output, outputPrice)
	add(TokenTypeCacheRead, cacheRead, cacheReadPrice)
	add(TokenTypeCacheWrite, cacheWrite, cacheWritePrice)
	return usage
}

func nullStringOr(s sql.NullString, fallback string) string {
	if !s.Valid || s.String == "" {
		return fallback
	}
	return s.String
}

func nullStringPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}

func nullProviderTypePtr(t database.NullAIProviderType) *database.AIProviderType {
	if !t.Valid {
		return nil
	}
	return &t.AIProviderType
}

// vendorTypeRaw returns the short, machine-oriented vendor code used inside
// SkuId/SkuPriceId: the raw ai_providers.type string when a provider row
// resolved, else the wire protocol (matching resolveVendor's own fallback).
func vendorTypeRaw(vendorType *database.AIProviderType, wireProtocol string) string {
	if vendorType != nil {
		return string(*vendorType)
	}
	return wireProtocol
}

func nullErrorTypePtr(e database.NullAIBridgeInterceptionErrorType) *string {
	if !e.Valid {
		return nil
	}
	v := string(e.AIBridgeInterceptionErrorType)
	return &v
}
