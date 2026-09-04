package coderd_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/testutil"
)

// aibridgeOpts is defined in aibridge_test.go (same package) and reused here.

// TestAIBridgeGetSessionThreadsTokenUsageTruncatedBeyondDefaultThreadPageSize
// is a regression test capturing a customer-reported bug: sessions whose
// threads outnumber the session-threads endpoint's default page size (50)
// report zero token usage on every thread (and every nested
// agentic_actions[].token_usage) beyond the 50th, even though
// token_usage_summary at the session level is correct and
// aibridge_token_usages rows genuinely exist for every thread.
//
// Root cause: aiBridgeGetSessionThreads (enterprise/coderd/aibridge.go)
// fetches "all interceptions (unpaginated)" for token aggregation via:
//
//	allRows, err = db.ListAIBridgeSessionThreads(ctx, database.ListAIBridgeSessionThreadsParams{
//		SessionID: sessionIDParam,
//	})
//
// Limit is left at its Go zero value (0) because the intent is "no
// pagination, fetch everything". But the underlying SQL query
// (ListAIBridgeSessionThreads in coderd/database/queries/aibridge.sql) has
// no "unlimited" concept:
//
//	LIMIT COALESCE(NULLIF(@limit_::integer, 0), 50)
//
// NULLIF(0, 0) evaluates to NULL, so COALESCE falls back to the same
// default of 50 threads used for the *paginated* per-page fetch. The
// "fetch everything" call therefore silently returns only the first 50
// threads (ordered by started_at ASC), and db2sdk.AIBridgeSessionThreads
// only ever sees token usage rows for those 50 interceptions, leaving
// every later thread's token_usage (and every nested
// agentic_actions[].token_usage) at zero.
//
// This test seeds 55 independent threads (> the hardcoded 50 default) in a
// single session, each with a real aibridge_token_usages row, and asserts
// that every thread reports its own recorded tokens. It currently FAILS at
// thread index 50 (the 51st thread), proving the truncation. Once the
// underlying fetch is fixed to retrieve every thread's token usage
// regardless of session size, this test should pass unmodified.
func TestAIBridgeGetSessionThreadsTokenUsageTruncatedBeyondDefaultThreadPageSize(t *testing.T) {
	t.Parallel()

	client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
	ctx := testutil.Context(t, testutil.WaitLong)

	const (
		threadCount      = 55 // Anything > 50 (the hardcoded SQL default) reproduces the bug.
		inputTokens      = int64(1234)
		outputTokens     = int64(567)
		sessionIDForTest = "large-session"
	)

	now := dbtime.Now()
	for i := 0; i < threadCount; i++ {
		startedAt := now.Add(time.Duration(i) * time.Second)
		endedAt := startedAt.Add(time.Second)
		// Each interception is its own thread root (ThreadRootInterceptionID
		// is left unset/NULL), so this seeds threadCount independent threads
		// within a single session.
		intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "anthropic",
			Model:           "claude-4",
			StartedAt:       startedAt,
			ClientSessionID: sql.NullString{String: sessionIDForTest, Valid: true},
		}, &endedAt)

		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: intc.ID,
			InputTokens:    inputTokens,
			OutputTokens:   outputTokens,
		})
	}

	// Request a page large enough (max is 200) to return every thread, so the
	// thread *list* itself is not truncated -- isolating the bug to the
	// token-usage aggregation path.
	res, err := client.AIBridgeGetSessionThreads(ctx, sessionIDForTest, uuid.Nil, uuid.Nil, 200)
	require.NoError(t, err)
	require.Len(t, res.Threads, threadCount, "the thread list itself should not be truncated")

	// The session-level summary aggregates over every ended interception in
	// the session with no artificial cap, so it should already be correct
	// today.
	require.EqualValues(t, inputTokens*threadCount, res.TokenUsageSummary.InputTokens,
		"session-level token_usage_summary should reflect every thread's tokens")
	require.EqualValues(t, outputTokens*threadCount, res.TokenUsageSummary.OutputTokens,
		"session-level token_usage_summary should reflect every thread's tokens")

	// Every individual thread should also report its own (nonzero) token
	// usage, since a real aibridge_token_usages row was inserted for each one
	// of them. This is where the bug manifests: it fails at thread index 50
	// (the 51st thread) with actual=0, expected=1234.
	for i, thread := range res.Threads {
		require.EqualValuesf(t, inputTokens, thread.TokenUsage.InputTokens,
			"thread %d should report the input tokens recorded for its interception", i)
		require.EqualValuesf(t, outputTokens, thread.TokenUsage.OutputTokens,
			"thread %d should report the output tokens recorded for its interception", i)
	}
}
