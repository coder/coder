package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	aiblib "github.com/coder/coder/v2/aibridge"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	entcoderd "github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

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

// auditLogsByAction indexes audit rows by their action so callers can assert
// on a specific entry without relying on row order, which GetAuditLogsOffset
// does not guarantee. It requires every action among the rows to be unique.
func auditLogsByAction(t *testing.T, rows []database.GetAuditLogsOffsetRow) map[database.AuditAction]database.AuditLog {
	t.Helper()
	byAction := make(map[database.AuditAction]database.AuditLog, len(rows))
	for _, r := range rows {
		_, dup := byAction[r.AuditLog.Action]
		require.Falsef(t, dup, "duplicate audit action %q: helper assumes distinct actions", r.AuditLog.Action)
		byAction[r.AuditLog.Action] = r.AuditLog
	}
	return byAction
}

// auditLogByNewSpendLimit selects the audit row whose diff sets spend_limit to
// the given value so callers can assert on a specific entry without relying on
// row order, which GetAuditLogsOffset does not guarantee. It requires the
// resulting spend_limit among the rows to be unique.
func auditLogByNewSpendLimit(t *testing.T, rows []database.GetAuditLogsOffsetRow, newSpendLimit string) database.AuditLog {
	t.Helper()
	var matches []database.AuditLog
	for _, r := range rows {
		var diff audit.Map
		require.NoError(t, json.Unmarshal(r.AuditLog.Diff, &diff))
		if field, ok := diff["spend_limit"]; ok && field.New == newSpendLimit {
			matches = append(matches, r.AuditLog)
		}
	}
	require.Lenf(t, matches, 1, "want exactly one audit entry setting spend_limit to %q, got %d", newSpendLimit, len(matches))
	return matches[0]
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
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: s2i1.ID,
			Prompt:         "prompt from session 2",
			CreatedAt:      now.Add(-30 * time.Minute),
		})

		// Session 3: Standalone interception (no client_session_id, no thread_root_id).
		// No prompt; last_active_at falls back to started_at.
		s3EndedAt := now.Add(-2*time.Hour + time.Minute)
		s3i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			Provider:    "anthropic",
			Model:       "claude-4",
			StartedAt:   now.Add(-2 * time.Hour),
		}, &s3EndedAt)

		// Session 4: Two distinct thread roots in one client_session_id.
		s4i1EndedAt := now.Add(-3*time.Hour + time.Minute)
		s4i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
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
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: s4i1.ID,
			Prompt:         "prompt from session 4",
			CreatedAt:      now.Add(-150 * time.Minute),
		})

		//nolint:gocritic // Owner role is irrelevant here.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.EqualValues(t, 4, res.Count)
		require.Len(t, res.Sessions, 4)

		// Sessions ordered by last_active_at DESC:
		// session-A (now+1m), thread-based (now-30m), standalone
		// (now-2h via started_at fallback), multi-thread (now-150m).
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

		// Verify session 3 (standalone, no prompts).
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

	t.Run("NetworkCalls", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		opts := aibridgeOpts(t)
		opts.Database = db
		opts.Pubsub = ps
		client, _, firstUser := coderdenttest.NewWithDatabase(t, opts)
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		makeInterception := func(clientSessionID string, startOffset time.Duration, fw *uuid.UUID, seq int32) {
			endedAt := now.Add(startOffset + time.Minute)
			params := database.InsertAIBridgeInterceptionParams{
				InitiatorID:     firstUser.UserID,
				StartedAt:       now.Add(startOffset),
				ClientSessionID: sql.NullString{String: clientSessionID, Valid: true},
			}
			if fw != nil {
				params.AgentFirewallSessionID = uuid.NullUUID{UUID: *fw, Valid: true}
				params.AgentFirewallSequenceNumber = sql.NullInt32{Int32: seq, Valid: true}
			}
			dbgen.AIBridgeInterception(t, db, params, &endedAt)
		}

		type logSeed struct {
			seq     int32
			allowed bool
		}
		sysCtx := dbauthz.AsSystemRestricted(ctx)
		insertLogs := func(fw uuid.UUID, seeds []logSeed) {
			params := database.InsertBoundaryLogsParams{
				SessionID: fw,
				OwnerID:   firstUser.UserID,
			}
			for _, s := range seeds {
				rule := ""
				if s.allowed {
					rule = "allow example.com"
				}
				params.ID = append(params.ID, uuid.New())
				params.SequenceNumber = append(params.SequenceNumber, s.seq)
				params.CapturedAt = append(params.CapturedAt, now)
				params.CreatedAt = append(params.CreatedAt, now)
				params.Proto = append(params.Proto, "http")
				params.Method = append(params.Method, "GET")
				params.Detail = append(params.Detail, "https://example.com")
				params.MatchedRule = append(params.MatchedRule, rule)
			}
			_, err := db.InsertBoundaryLogs(sysCtx, params)
			require.NoError(t, err, "insert boundary logs")
		}

		fw1, fw2, fw3, fw4 := uuid.New(), uuid.New(), uuid.New(), uuid.New()

		// Sessions A and B share firewall session fw1. A is marked at seq 0, B at
		// seq 3, so A's window is (0,3) and B's is (3, +inf).
		makeInterception("sess-A", -time.Minute, &fw1, 0)
		makeInterception("sess-B", -2*time.Minute, &fw1, 3)
		insertLogs(fw1, []logSeed{
			{0, true},  // LLM call for A, excluded
			{1, true},  // A egress
			{2, false}, // A egress, blocked
			{3, true},  // LLM call for B, excluded
			{4, true},  // B egress
			{5, true},  // B egress
		})

		// Session C spans two firewall sessions (agent restarted): fw2 and fw3.
		// Its counts sum across both windows.
		makeInterception("sess-C", -3*time.Minute, &fw2, 0)
		makeInterception("sess-C", -4*time.Minute, &fw3, 0)
		insertLogs(fw2, []logSeed{
			{0, true}, // LLM call, excluded
			{1, true},
			{2, true},
		})
		insertLogs(fw3, []logSeed{
			{0, true},  // LLM call, excluded
			{1, false}, // blocked
		})

		// Session D never passed through the firewall: NetworkCalls stays nil.
		makeInterception("sess-D", -5*time.Minute, nil, 0)

		// Session E is firewall-active but has no logs in range: counts are zero.
		makeInterception("sess-E", -6*time.Minute, &fw4, 0)

		//nolint:gocritic // Owner role is irrelevant here.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)

		byID := make(map[string]codersdk.AIBridgeSession, len(res.Sessions))
		for _, s := range res.Sessions {
			byID[s.ID] = s
		}

		// A: seq 1,2 in (0,3); seq 2 blocked. LLM calls at 0 and 3 excluded.
		a := byID["sess-A"]
		require.NotNil(t, a.NetworkCalls)
		require.EqualValues(t, 2, a.NetworkCalls.Total)
		require.EqualValues(t, 1, a.NetworkCalls.Blocked)

		// B: seq 4,5 in (3, +inf); none blocked. No bleed from A's window.
		b := byID["sess-B"]
		require.NotNil(t, b.NetworkCalls)
		require.EqualValues(t, 2, b.NetworkCalls.Total)
		require.EqualValues(t, 0, b.NetworkCalls.Blocked)

		// C: fw2 contributes seq 1,2; fw3 contributes seq 1 (blocked).
		c := byID["sess-C"]
		require.NotNil(t, c.NetworkCalls)
		require.EqualValues(t, 3, c.NetworkCalls.Total)
		require.EqualValues(t, 1, c.NetworkCalls.Blocked)

		// D: no firewall session, so monitoring was not active.
		d := byID["sess-D"]
		require.Nil(t, d.NetworkCalls)

		// E: firewall-active but no logs in range.
		e := byID["sess-E"]
		require.NotNil(t, e.NetworkCalls)
		require.EqualValues(t, 0, e.NetworkCalls.Total)
		require.EqualValues(t, 0, e.NetworkCalls.Blocked)
	})

	t.Run("Pagination", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		// Create 5 standalone sessions with different start times.
		// Without prompts, last_active_at falls back to started_at, so the
		// expected descending order is preserved.
		allSessionIDs := make([]string, 5)
		for i := range 5 {
			startedAt := now.Add(-time.Duration(i) * time.Hour)
			endedAt := startedAt.Add(time.Minute)
			intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID: firstUser.UserID,
				StartedAt:   startedAt,
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
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: i1.ID,
			Prompt:         "prompt",
			CreatedAt:      now,
		})
		i2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: auditorUser.ID,
			StartedAt:   now.Add(-time.Hour),
		}, &now)
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: i2.ID,
			Prompt:         "prompt",
			CreatedAt:      now.Add(-time.Hour),
		})

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

	t.Run("StartedBeforeFilter", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Session started recently.
		recentEndedAt := now.Add(time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now,
		}, &recentEndedAt)

		// Session started 2 hours ago.
		oldEndedAt := now.Add(-2*time.Hour + time.Minute)
		old := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now.Add(-2 * time.Hour),
		}, &oldEndedAt)

		// Only the old session should be returned when started_before
		// is set to 1 hour ago.
		//nolint:gocritic // Owner role is irrelevant; testing filter.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			StartedBefore: now.Add(-time.Hour),
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Len(t, res.Sessions, 1)
		require.Equal(t, old.ID.String(), res.Sessions[0].ID)
	})

	t.Run("NullClientCoalescesToUnknown", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Session with explicit client.
		withClientEndedAt := now.Add(time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now,
			Client:      sql.NullString{String: "claude-code", Valid: true},
		}, &withClientEndedAt)

		// Session with NULL client (should COALESCE to ClientUnknown).
		nullClientEndedAt := now.Add(-time.Hour + time.Minute)
		nullClient := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			StartedAt:   now.Add(-time.Hour),
			// Client field deliberately omitted (NULL).
		}, &nullClientEndedAt)

		// Filtering by ClientUnknown should return only the NULL-client
		// session.
		//nolint:gocritic // Owner role is irrelevant; testing COALESCE.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Client: string(aiblib.ClientUnknown),
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Len(t, res.Sessions, 1)
		require.Equal(t, nullClient.ID.String(), res.Sessions[0].ID)
	})

	t.Run("MetadataFromFirstInterception", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// First interception (chronologically) carries the expected
		// metadata for the session.
		i1EndedAt := now.Add(time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       now,
			Metadata:        json.RawMessage(`{"editor":"vscode"}`),
			Client:          sql.NullString{String: "claude-code", Valid: true},
			ClientSessionID: sql.NullString{String: "meta-session", Valid: true},
		}, &i1EndedAt)

		// Second interception has different metadata.
		i2EndedAt := now.Add(2 * time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       now.Add(time.Minute),
			Metadata:        json.RawMessage(`{"editor":"jetbrains"}`),
			Client:          sql.NullString{String: "claude-code", Valid: true},
			ClientSessionID: sql.NullString{String: "meta-session", Valid: true},
		}, &i2EndedAt)

		//nolint:gocritic // Owner role is irrelevant; testing metadata.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Sessions, 1)
		// Metadata should come from the first interception.
		require.Equal(t, "vscode", res.Sessions[0].Metadata["editor"])
	})

	t.Run("SessionTimestamps", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Two interceptions in the same session with different
		// started_at and ended_at values. The session should report
		// MIN(started_at) and MAX(ended_at).
		i1StartedAt := now
		i1EndedAt := now.Add(time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       i1StartedAt,
			ClientSessionID: sql.NullString{String: "ts-session", Valid: true},
		}, &i1EndedAt)

		i2StartedAt := now.Add(2 * time.Minute)
		i2EndedAt := now.Add(5 * time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       i2StartedAt,
			ClientSessionID: sql.NullString{String: "ts-session", Valid: true},
		}, &i2EndedAt)

		//nolint:gocritic // Owner role is irrelevant; testing timestamps.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Sessions, 1)
		s := res.Sessions[0]
		require.WithinDuration(t, i1StartedAt, s.StartedAt, time.Millisecond,
			"session started_at should be MIN of interception started_at values")
		require.NotNil(t, s.EndedAt)
		require.WithinDuration(t, i2EndedAt, *s.EndedAt, time.Millisecond,
			"session ended_at should be MAX of interception ended_at values")
	})

	t.Run("LastPromptAcrossInterceptions", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Two interceptions in the same session.
		i1EndedAt := now.Add(time.Minute)
		i1 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       now,
			ClientSessionID: sql.NullString{String: "prompt-session", Valid: true},
		}, &i1EndedAt)
		i2EndedAt := now.Add(3 * time.Minute)
		i2 := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       now.Add(2 * time.Minute),
			ClientSessionID: sql.NullString{String: "prompt-session", Valid: true},
		}, &i2EndedAt)

		// Add prompts to both interceptions. The most recent prompt
		// overall belongs to the second interception.
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: i1.ID,
			Prompt:         "early prompt from i1",
			CreatedAt:      now,
		})
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: i2.ID,
			Prompt:         "latest prompt from i2",
			CreatedAt:      now.Add(2 * time.Minute),
		})

		//nolint:gocritic // Owner role is irrelevant; testing lateral join.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Sessions, 1)
		require.NotNil(t, res.Sessions[0].LastPrompt)
		require.Equal(t, "latest prompt from i2", *res.Sessions[0].LastPrompt,
			"last_prompt should be the most recent prompt across all interceptions in the session")
	})

	t.Run("CombinedFilters", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		_, user2 := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

		now := dbtime.Now()

		// Session A: user1, anthropic, claude-4, started now.
		aEndedAt := now.Add(time.Minute)
		a := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			Provider:    "anthropic",
			Model:       "claude-4",
			StartedAt:   now,
		}, &aEndedAt)

		// Session B: user1, anthropic, gpt-4, started 2h ago.
		bEndedAt := now.Add(-2*time.Hour + time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			Provider:    "anthropic",
			Model:       "gpt-4",
			StartedAt:   now.Add(-2 * time.Hour),
		}, &bEndedAt)

		// Session C: user2, anthropic, claude-4, started 1h ago.
		cEndedAt := now.Add(-time.Hour + time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user2.ID,
			Provider:    "anthropic",
			Model:       "claude-4",
			StartedAt:   now.Add(-time.Hour),
		}, &cEndedAt)

		// Combining provider + model + started_after should return
		// only session A (user1, anthropic, claude-4, recent).
		//nolint:gocritic // Owner role is irrelevant; testing combined filters.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Provider:     "anthropic",
			Model:        "claude-4",
			StartedAfter: now.Add(-30 * time.Minute),
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, res.Count)
		require.Len(t, res.Sessions, 1)
		require.Equal(t, a.ID.String(), res.Sessions[0].ID)
	})

	t.Run("CursorPaginationWithTiedStartedAt", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Create 3 standalone sessions all starting and with a prompt at
		// the same time. The tie-breaker on last_active_at is session_id DESC.
		for range 3 {
			endedAt := now.Add(time.Minute)
			interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID: firstUser.UserID,
				StartedAt:   now,
			}, &endedAt)
			dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
				InterceptionID: interception.ID,
				Prompt:         "prompt",
				CreatedAt:      now,
			})
		}

		// Fetch all to learn the sort order (last_active_at DESC,
		// session_id DESC).
		//nolint:gocritic // Owner role is irrelevant; testing cursor.
		all, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.Len(t, all.Sessions, 3)

		// Use the first result as cursor. The remaining 2 should be
		// returned.
		afterID := all.Sessions[0].ID
		page, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{
			Pagination:     codersdk.Pagination{Limit: 10},
			AfterSessionID: afterID,
		})
		require.NoError(t, err)
		require.Len(t, page.Sessions, 2)
		require.Equal(t, all.Sessions[1].ID, page.Sessions[0].ID)
		require.Equal(t, all.Sessions[2].ID, page.Sessions[1].ID)
	})

	t.Run("DefaultLimit", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		// Create 3 sessions. Without an explicit limit the default of
		// 100 should apply and return all 3.
		for i := range 3 {
			endedAt := now.Add(-time.Duration(i)*time.Hour + time.Minute)
			dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID: firstUser.UserID,
				StartedAt:   now.Add(-time.Duration(i) * time.Hour),
			}, &endedAt)
		}

		// No Pagination.Limit set.
		//nolint:gocritic // Owner role is irrelevant; testing default limit.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Sessions, 3)
		require.EqualValues(t, 3, res.Count)
	})

	// LastActiveAtAlwaysSet verifies that last_active_at is always non-zero,
	// even for sessions without prompts. Prompted sessions use the latest
	// prompt timestamp; promptless sessions fall back to started_at.
	t.Run("LastActiveAtAlwaysSet", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		sessionIDs := []string{"session-a", "session-b", "session-c"}
		promptOffsets := []time.Duration{0, -30 * time.Minute, -time.Hour}
		for i, sid := range sessionIDs {
			endedAt := now.Add(time.Minute)
			interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID:     firstUser.UserID,
				StartedAt:       now.Add(-time.Duration(i) * time.Hour),
				ClientSessionID: sql.NullString{String: sid, Valid: true},
			}, &endedAt)
			dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
				InterceptionID: interception.ID,
				Prompt:         "prompt",
				CreatedAt:      now.Add(promptOffsets[i]),
			})
		}

		//nolint:gocritic // Owner role is irrelevant; testing last_active_at.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Sessions, 3)

		for i, s := range res.Sessions {
			require.NotZero(t, s.LastActiveAt, "session %d (%s) should have last_active_at set", i, s.ID)
		}

		// Sorted by last_active_at DESC: a (now), b (now-30m), c (now-1h).
		require.Equal(t, "session-a", res.Sessions[0].ID)
		require.Equal(t, "session-b", res.Sessions[1].ID)
		require.Equal(t, "session-c", res.Sessions[2].ID)
	})

	// PromptlessSessionSortsByStartedAt verifies that a session whose root
	// interception has no associated user prompts still appears in results and
	// sorts by MIN(started_at) as a fallback. Without the COALESCE fallback a
	// NULL last_active_at would cause the HAVING row-value comparison to
	// evaluate to NULL (not false), silently dropping the session from all
	// result pages.
	//
	// Three sessions are arranged so that the promptless session sits between
	// two prompted sessions in sort order:
	//
	//   A: started=now,    prompt=now      → last_active_at=now
	//   B: started=now-1h, NO prompt       → last_active_at=now-1h (fallback)
	//   C: started=now-2h, prompt=now-30m  → last_active_at=now-30m
	//
	// Sort order by last_active_at DESC: C (now-30m) > B (now-1h), so: A, C, B.
	// B disappearing would indicate the fallback is broken.
	t.Run("PromptlessSessionSortsByStartedAt", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Session A: has a prompt.
		aEndedAt := now.Add(time.Minute)
		aInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       now,
			ClientSessionID: sql.NullString{String: "session-a", Valid: true},
		}, &aEndedAt)
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: aInterception.ID,
			Prompt:         "prompt from session a",
			CreatedAt:      now,
		})

		// Session B: no prompt at all, exercises the MIN(started_at) fallback.
		bEndedAt := now.Add(time.Minute)
		bInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       now.Add(-1 * time.Hour),
			ClientSessionID: sql.NullString{String: "session-b", Valid: true},
		}, &bEndedAt)

		// Session C: has a prompt more recent than B's started_at, so C sorts
		// above B even though C started earlier.
		cEndedAt := now.Add(time.Minute)
		cInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       now.Add(-2 * time.Hour),
			ClientSessionID: sql.NullString{String: "session-c", Valid: true},
		}, &cEndedAt)
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: cInterception.ID,
			Prompt:         "prompt from session c",
			CreatedAt:      now.Add(-30 * time.Minute),
		})

		//nolint:gocritic // Owner role is irrelevant; testing sort fallback.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Sessions, 3, "promptless session B must appear in results")

		// Expected order: A (last_active_at=now), C (last_active_at=now-30m), B (last_active_at=now-1h via fallback).
		require.Equal(t, aInterception.SessionID, res.Sessions[0].ID, "session A should be first")
		require.Equal(t, cInterception.SessionID, res.Sessions[1].ID, "session C should be second (prompt=now-30m beats B's started_at=now-1h)")
		require.Equal(t, bInterception.SessionID, res.Sessions[2].ID, "session B should be last (no prompt, falls back to started_at=now-1h)")

		// All sessions have last_active_at; session B falls back to started_at.
		require.NotZero(t, res.Sessions[0].LastActiveAt, "session A should have last_active_at set")
		require.NotZero(t, res.Sessions[1].LastActiveAt, "session C should have last_active_at set")
		require.WithinDuration(t, bInterception.StartedAt, res.Sessions[2].LastActiveAt, time.Millisecond, "session B has no prompts, last_active_at should equal started_at")
	})

	// SortsByLastActive verifies that sessions are ordered by last_active_at.
	// Every session here has at least one prompt, so last_active_at equals
	// the latest prompt timestamp rather than the started_at fallback.
	//
	// Three sessions are created with intentionally crossing timestamps so that
	// the "prompt time" order differs from the "started_at" order:
	//
	//   X: started=now,   prompt=now      → last_active_at = now
	//   Y: started=now-2h, prompt=now-30m  → last_active_at = now-30m
	//   Z: started=now-1h, prompt=now-1h   → last_active_at = now-1h
	//
	// Order by started_at DESC: X, Z, Y
	// Order by last_active_at DESC: X, Y, Z
	t.Run("SortsByLastActive", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()

		// Session X: started now, prompt now.
		xEndedAt := now.Add(time.Minute)
		xInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       now,
			ClientSessionID: sql.NullString{String: "session-x", Valid: true},
		}, &xEndedAt)
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: xInterception.ID,
			Prompt:         "prompt from session x",
			CreatedAt:      now,
		})

		// Session Y: started 2 hours ago, prompt 30 minutes ago.
		yEndedAt := now.Add(time.Minute)
		yInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       now.Add(-2 * time.Hour),
			ClientSessionID: sql.NullString{String: "session-y", Valid: true},
		}, &yEndedAt)
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: yInterception.ID,
			Prompt:         "prompt from session y",
			CreatedAt:      now.Add(-30 * time.Minute),
		})

		// Session Z: started 1 hour ago, prompt 1 hour ago.
		zEndedAt := now.Add(time.Minute)
		zInterception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			StartedAt:       now.Add(-1 * time.Hour),
			ClientSessionID: sql.NullString{String: "session-z", Valid: true},
		}, &zEndedAt)
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: zInterception.ID,
			Prompt:         "prompt from session z",
			CreatedAt:      now.Add(-1 * time.Hour),
		})

		//nolint:gocritic // Owner role is irrelevant; testing sort order.
		res, err := client.AIBridgeListSessions(ctx, codersdk.AIBridgeListSessionsFilter{})
		require.NoError(t, err)
		require.Len(t, res.Sessions, 3)

		// Expected order: X (now), Y (now-30m), Z (now-1h).
		// If sorted by started_at the order would be X, Z, Y.
		require.Equal(t, xInterception.SessionID, res.Sessions[0].ID, "session X should be first (prompt=now)")
		require.Equal(t, yInterception.SessionID, res.Sessions[1].ID, "session Y should be second (prompt=now-30m beats Z's now-1h)")
		require.Equal(t, zInterception.SessionID, res.Sessions[2].ID, "session Z should be last (prompt=now-1h)")

		// All sessions have LastActiveAt populated.
		require.NotNil(t, res.Sessions[0].LastActiveAt, "session X should have last_active_at set")
		require.NotNil(t, res.Sessions[1].LastActiveAt, "session Y should have last_active_at set")
		require.NotNil(t, res.Sessions[2].LastActiveAt, "session Z should have last_active_at set")
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

	// Completed interception with no client. Should appear as "Unknown".
	dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: firstUser.UserID,
		StartedAt:   now,
	}, &endedAt)

	// Duplicate client. Should be deduplicated in results.
	dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
		InitiatorID: firstUser.UserID,
		StartedAt:   now,
		Client:      sql.NullString{String: string(aiblib.ClientCursor), Valid: true},
	}, &endedAt)

	// In-flight interception (no ended_at). Must NOT appear in results.
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
	api.AGPL.RegisterInMemoryAIBridgedHTTPHandler(testHandler)

	cases := []struct {
		name         string
		path         string
		expectedPath string
	}{
		{
			name:         "StablePrefix",
			path:         "/api/v2/ai-gateway/openai/v1/chat/completions",
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
	api.AGPL.RegisterInMemoryAIBridgedHTTPHandler(testHandler)

	ctx := testutil.Context(t, testutil.WaitLong)
	httpClient := &http.Client{}
	url := client.URL.String() + "/api/v2/ai-gateway/test"

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
	api.AGPL.RegisterInMemoryAIBridgedHTTPHandler(testHandler)

	ctx := testutil.Context(t, testutil.WaitLong)
	httpClient := &http.Client{}
	url := client.URL.String() + "/api/v2/ai-gateway/test"

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

type boundaryLogSeed struct {
	seq     int32
	proto   string
	detail  string
	allowed bool
}

// seedBoundaryLogs writes boundary logs for a firewall session via the raw
// store. A non-empty matched_rule marks a call allowed; a blocked call stores a
// NULL rule. No RBAC role grants boundary_log:create, so tests seed directly.
func seedBoundaryLogs(t *testing.T, db database.Store, fw, ownerID uuid.UUID, at time.Time, seeds []boundaryLogSeed) {
	t.Helper()
	logs := make([]database.BoundaryLog, 0, len(seeds))
	for _, s := range seeds {
		rule := ""
		if s.allowed {
			rule = "allow " + s.detail
		}
		logs = append(logs, database.BoundaryLog{
			SessionID:      fw,
			OwnerID:        uuid.NullUUID{UUID: ownerID, Valid: true},
			SequenceNumber: s.seq,
			CapturedAt:     at,
			CreatedAt:      at,
			Proto:          s.proto,
			Method:         "GET",
			Detail:         s.detail,
			MatchedRule:    sql.NullString{String: rule, Valid: rule != ""},
		})
	}
	dbgen.BoundaryLogs(t, db, logs)
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
			InitiatorID:    firstUser.UserID,
			Provider:       "openai",
			Model:          "gpt-4",
			StartedAt:      now,
			CredentialKind: database.CredentialKindByok,
			CredentialHint: "sk-a...efgh",
		}, &endedAt)

		// When no client session ID is set, the interception ID becomes the session identifier.
		res, err := client.AIBridgeGetSessionThreads(ctx, i1.ID.String(), uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)
		require.Equal(t, i1.ID.String(), res.ID)
		require.Len(t, res.Threads, 1)
		require.Equal(t, "byok", res.Threads[0].CredentialKind)
		require.Equal(t, "sk-a...efgh", res.Threads[0].CredentialHint)
	})

	t.Run("ThreadsWithAgentFirewallCorrelation", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		fwSessionID := uuid.New()

		// Thread with firewall correlation on the root interception.
		rootEndedAt := now.Add(time.Minute)
		root := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:                 firstUser.UserID,
			Provider:                    "anthropic",
			Model:                       "claude-sonnet-4-20250514",
			StartedAt:                   now,
			ClientSessionID:             sql.NullString{String: "fw-session", Valid: true},
			AgentFirewallSessionID:      uuid.NullUUID{UUID: fwSessionID, Valid: true},
			AgentFirewallSequenceNumber: sql.NullInt32{Int32: 5, Valid: true},
		}, &rootEndedAt)

		// Thread without firewall correlation in the same session.
		noFWEndedAt := now.Add(2 * time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "openai",
			Model:           "gpt-4",
			StartedAt:       now.Add(time.Minute),
			ClientSessionID: sql.NullString{String: "fw-session", Valid: true},
		}, &noFWEndedAt)

		res, err := client.AIBridgeGetSessionThreads(ctx, "fw-session", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)
		require.Equal(t, "fw-session", res.ID)
		require.Len(t, res.Threads, 2)

		// First thread has firewall correlation.
		require.Equal(t, root.ID, res.Threads[0].ID)
		require.NotNil(t, res.Threads[0].AgentFirewallSessionID)
		require.Equal(t, fwSessionID, *res.Threads[0].AgentFirewallSessionID)
		require.NotNil(t, res.Threads[0].AgentFirewallSequenceNumber)
		require.Equal(t, int32(5), *res.Threads[0].AgentFirewallSequenceNumber)

		// Second thread has no firewall correlation.
		require.Nil(t, res.Threads[1].AgentFirewallSessionID)
		require.Nil(t, res.Threads[1].AgentFirewallSequenceNumber)
	})

	t.Run("NetworkSummary", func(t *testing.T) {
		t.Parallel()
		// Use the raw store so boundary logs can be seeded directly. No RBAC
		// role grants boundary_log:create; they are written by the agent path.
		db, ps := dbtestutil.NewDB(t)
		opts := aibridgeOpts(t)
		opts.Database = db
		opts.Pubsub = ps
		client, _, firstUser := coderdenttest.NewWithDatabase(t, opts)
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		fw := uuid.New()

		// One interception marked at firewall seq 0, so its window is (0, +inf)
		// and the LLM-provider call logged at seq 0 is excluded.
		endedAt := now.Add(time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:                 firstUser.UserID,
			Provider:                    "anthropic",
			Model:                       "claude-sonnet-4-20250514",
			StartedAt:                   now,
			ClientSessionID:             sql.NullString{String: "net-session", Valid: true},
			AgentFirewallSessionID:      uuid.NullUUID{UUID: fw, Valid: true},
			AgentFirewallSequenceNumber: sql.NullInt32{Int32: 0, Valid: true},
		}, &endedAt)

		seedBoundaryLogs(t, db, fw, firstUser.UserID, now, []boundaryLogSeed{
			{0, "http", "https://api.github.com/llm", true},         // LLM call, excluded
			{1, "http", "https://api.github.com/repos/coder", true}, // github egress
			{2, "http", "https://api.github.com/repos/other", true}, // github egress
			{3, "http", "https://registry.npmjs.org/lodash", false}, // npm egress, blocked
			{4, "http", "https://api.github.com/repos/more", true},  // github egress
			{5, "dns", "example.com", true},                         // non-http, ignored by top domains
			{6, "http", "https://api.github.com:8080/repos", true},  // port-suffixed; host stripped to api.github.com
		})

		res, err := client.AIBridgeGetSessionThreads(ctx, "net-session", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)

		// total counts seq 1-6 (LLM call at seq 0 excluded); one blocked.
		require.NotNil(t, res.NetworkCalls)
		require.EqualValues(t, 6, res.NetworkCalls.Total)
		require.EqualValues(t, 1, res.NetworkCalls.Blocked)

		// Top domains covers HTTP egress only and is capped at one row (the
		// summary card renders a single domain). github wins with 4 HTTP calls:
		// seqs 1, 2, 4, and the port-suffixed seq 6 whose host strips to
		// api.github.com (proving the port is not treated as a separate host).
		// The dns log (seq 5) is excluded from domains. NetworkDomainCount is a
		// window aggregate independent of the row cap, so it still reports the
		// two distinct HTTP domains (github, npm).
		require.Len(t, res.NetworkTopDomains, 1)
		require.Equal(t, "api.github.com", res.NetworkTopDomains[0].Domain)
		require.EqualValues(t, 4, res.NetworkTopDomains[0].Count)
		require.EqualValues(t, 2, res.NetworkDomainCount)

		// The per-call list spans the same window as the summary (all protos,
		// seq 0 LLM call excluded), ordered chronologically. Its length and
		// blocked count agree with the summary counts.
		gotSeqs := make([]int32, len(res.NetworkCallLogs))
		blocked := 0
		for i, c := range res.NetworkCallLogs {
			gotSeqs[i] = c.SequenceNumber
			if !c.Allowed {
				blocked++
			}
		}
		require.Equal(t, []int32{1, 2, 3, 4, 5, 6}, gotSeqs)
		require.EqualValues(t, res.NetworkCalls.Total, len(res.NetworkCallLogs))
		require.EqualValues(t, res.NetworkCalls.Blocked, blocked)

		// The blocked npm call (seq 3, index 2) has no matched rule; allowed
		// calls do.
		npm := res.NetworkCallLogs[2]
		require.Equal(t, int32(3), npm.SequenceNumber)
		require.Equal(t, "https://registry.npmjs.org/lodash", npm.Detail)
		require.False(t, npm.Allowed)
		require.Nil(t, npm.MatchedRule)
		require.True(t, res.NetworkCallLogs[0].Allowed)
		require.NotNil(t, res.NetworkCallLogs[0].MatchedRule)

		// Non-http protocols appear in the list (unlike top domains): seq 5 dns.
		require.Equal(t, "dns", res.NetworkCallLogs[4].Proto)
	})

	t.Run("NetworkMultipleInterceptions", func(t *testing.T) {
		t.Parallel()
		// Two interceptions in the same firewall session must produce two
		// consecutive, non-overlapping windows: (0, 5) for the first and
		// (5, +inf) for the second. Each interception's own LLM call (logged at
		// its own sequence) is excluded by the exclusive lower bound.
		db, ps := dbtestutil.NewDB(t)
		opts := aibridgeOpts(t)
		opts.Database = db
		opts.Pubsub = ps
		client, _, firstUser := coderdenttest.NewWithDatabase(t, opts)
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		fw := uuid.New()

		for _, seq := range []int32{0, 5} {
			endedAt := now.Add(time.Minute)
			dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID:                 firstUser.UserID,
				Provider:                    "anthropic",
				Model:                       "claude-sonnet-4-20250514",
				StartedAt:                   now,
				ClientSessionID:             sql.NullString{String: "multi-net", Valid: true},
				AgentFirewallSessionID:      uuid.NullUUID{UUID: fw, Valid: true},
				AgentFirewallSequenceNumber: sql.NullInt32{Int32: seq, Valid: true},
			}, &endedAt)
		}

		seedBoundaryLogs(t, db, fw, firstUser.UserID, now, []boundaryLogSeed{
			{0, "http", "https://api.github.com/llm", true},    // interception 1 LLM call, excluded
			{1, "http", "https://api.github.com/a", true},      // window (0,5)
			{2, "http", "https://api.github.com/b", true},      // window (0,5)
			{3, "http", "https://registry.npmjs.org/x", false}, // window (0,5), blocked
			{5, "http", "https://api.github.com/llm2", true},   // interception 2 LLM call, excluded
			{6, "http", "https://api.github.com/c", true},      // window (5,+inf)
			{7, "http", "https://registry.npmjs.org/y", false}, // window (5,+inf), blocked
		})

		res, err := client.AIBridgeGetSessionThreads(ctx, "multi-net", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)

		// Both windows contribute: seqs 1,2,3,6,7. The two LLM calls (0, 5) are
		// excluded. Blocked = seqs 3 and 7.
		require.NotNil(t, res.NetworkCalls)
		require.EqualValues(t, 5, res.NetworkCalls.Total)
		require.EqualValues(t, 2, res.NetworkCalls.Blocked)
		// github: seqs 1,2,6 = 3; npm: seqs 3,7 = 2. Two distinct domains.
		require.Len(t, res.NetworkTopDomains, 1)
		require.Equal(t, "api.github.com", res.NetworkTopDomains[0].Domain)
		require.EqualValues(t, 3, res.NetworkTopDomains[0].Count)
		require.EqualValues(t, 2, res.NetworkDomainCount)

		// The per-call list spans both windows in chronological order, and its
		// length and blocked count agree with the summary (three-way agreement
		// across summary, top domains, and list).
		gotSeqs := make([]int32, len(res.NetworkCallLogs))
		blocked := 0
		for i, c := range res.NetworkCallLogs {
			gotSeqs[i] = c.SequenceNumber
			if !c.Allowed {
				blocked++
			}
		}
		require.Equal(t, []int32{1, 2, 3, 6, 7}, gotSeqs)
		require.EqualValues(t, res.NetworkCalls.Total, len(res.NetworkCallLogs))
		require.EqualValues(t, res.NetworkCalls.Blocked, blocked)
	})

	t.Run("NetworkSharedFirewallSessionNoBleed", func(t *testing.T) {
		t.Parallel()
		// Two AI sessions share one firewall session. next_seq considers every
		// interception in the firewall session, so session A's window is bounded
		// by session B's interception and B's calls never bleed into A's counts.
		db, ps := dbtestutil.NewDB(t)
		opts := aibridgeOpts(t)
		opts.Database = db
		opts.Pubsub = ps
		client, _, firstUser := coderdenttest.NewWithDatabase(t, opts)
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		fw := uuid.New()

		// Session A anchored at firewall seq 0; session B at seq 10.
		for _, s := range []struct {
			session string
			seq     int32
		}{{"sess-a", 0}, {"sess-b", 10}} {
			endedAt := now.Add(time.Minute)
			dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID:                 firstUser.UserID,
				Provider:                    "anthropic",
				Model:                       "claude-sonnet-4-20250514",
				StartedAt:                   now,
				ClientSessionID:             sql.NullString{String: s.session, Valid: true},
				AgentFirewallSessionID:      uuid.NullUUID{UUID: fw, Valid: true},
				AgentFirewallSequenceNumber: sql.NullInt32{Int32: s.seq, Valid: true},
			}, &endedAt)
		}

		seedBoundaryLogs(t, db, fw, firstUser.UserID, now, []boundaryLogSeed{
			{0, "http", "https://api.github.com/llm-a", true},    // A's LLM call, excluded
			{1, "http", "https://api.github.com/a1", true},       // A's window (0,10)
			{2, "http", "https://registry.npmjs.org/a2", false},  // A's window (0,10), blocked
			{10, "http", "https://api.github.com/llm-b", true},   // B's LLM call, excluded
			{11, "http", "https://api.github.com/b1", true},      // B's window (10,+inf)
			{12, "http", "https://api.github.com/b2", true},      // B's window (10,+inf)
			{13, "http", "https://registry.npmjs.org/b3", false}, // B's window (10,+inf), blocked
		})

		// Session A sees only its own two calls (seqs 1, 2), not B's, in both
		// the summary and the per-call list.
		resA, err := client.AIBridgeGetSessionThreads(ctx, "sess-a", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)
		require.NotNil(t, resA.NetworkCalls)
		require.EqualValues(t, 2, resA.NetworkCalls.Total)
		require.EqualValues(t, 1, resA.NetworkCalls.Blocked)
		seqsA := make([]int32, len(resA.NetworkCallLogs))
		for i, c := range resA.NetworkCallLogs {
			seqsA[i] = c.SequenceNumber
		}
		require.Equal(t, []int32{1, 2}, seqsA)

		// Session B sees only its own three calls (seqs 11, 12, 13), not A's.
		resB, err := client.AIBridgeGetSessionThreads(ctx, "sess-b", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)
		require.NotNil(t, resB.NetworkCalls)
		require.EqualValues(t, 3, resB.NetworkCalls.Total)
		require.EqualValues(t, 1, resB.NetworkCalls.Blocked)
		seqsB := make([]int32, len(resB.NetworkCallLogs))
		for i, c := range resB.NetworkCallLogs {
			seqsB[i] = c.SequenceNumber
		}
		require.Equal(t, []int32{11, 12, 13}, seqsB)
	})

	t.Run("NetworkCallsTruncated", func(t *testing.T) {
		t.Parallel()
		// The per-call list is capped server-side while the summary total
		// reflects the whole session. When a session exceeds the cap the list is
		// truncated but the summary total stays authoritative.
		db, ps := dbtestutil.NewDB(t)
		opts := aibridgeOpts(t)
		opts.Database = db
		opts.Pubsub = ps
		client, _, firstUser := coderdenttest.NewWithDatabase(t, opts)
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		fw := uuid.New()

		endedAt := now.Add(time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:                 firstUser.UserID,
			Provider:                    "anthropic",
			Model:                       "claude-sonnet-4-20250514",
			StartedAt:                   now,
			ClientSessionID:             sql.NullString{String: "trunc-net", Valid: true},
			AgentFirewallSessionID:      uuid.NullUUID{UUID: fw, Valid: true},
			AgentFirewallSequenceNumber: sql.NullInt32{Int32: 0, Valid: true},
		}, &endedAt)

		// Allowed HTTP calls at seqs 1..total, all in the interception's
		// window (0, +inf), seeded past the server-side cap.
		const listCap = 1000
		const total = listCap + 5
		seeds := make([]boundaryLogSeed, 0, total)
		for seq := int32(1); seq <= total; seq++ {
			seeds = append(seeds, boundaryLogSeed{seq, "http", "https://api.github.com/x", true})
		}
		seedBoundaryLogs(t, db, fw, firstUser.UserID, now, seeds)

		res, err := client.AIBridgeGetSessionThreads(ctx, "trunc-net", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)

		// The summary reflects every call; the list stops at the cap.
		require.NotNil(t, res.NetworkCalls)
		require.EqualValues(t, total, res.NetworkCalls.Total)
		require.Len(t, res.NetworkCallLogs, listCap)
		// The cap keeps the earliest calls in chronological order.
		require.EqualValues(t, 1, res.NetworkCallLogs[0].SequenceNumber)
		require.EqualValues(t, listCap, res.NetworkCallLogs[listCap-1].SequenceNumber)
	})

	t.Run("NetworkSummaryDisabled", func(t *testing.T) {
		t.Parallel()
		client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
		ctx := testutil.Context(t, testutil.WaitLong)

		now := dbtime.Now()
		endedAt := now.Add(time.Minute)
		// No firewall correlation: network monitoring was not active.
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     firstUser.UserID,
			Provider:        "anthropic",
			Model:           "claude-sonnet-4-20250514",
			StartedAt:       now,
			ClientSessionID: sql.NullString{String: "no-fw-session", Valid: true},
		}, &endedAt)

		res, err := client.AIBridgeGetSessionThreads(ctx, "no-fw-session", uuid.Nil, uuid.Nil, 0)
		require.NoError(t, err)
		require.Nil(t, res.NetworkCalls)
		require.Empty(t, res.NetworkTopDomains)
		require.EqualValues(t, 0, res.NetworkDomainCount)
		require.Empty(t, res.NetworkCallLogs)
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
			InterceptionID:        root.ID,
			ProviderResponseID:    providerRespID,
			InputTokens:           100,
			OutputTokens:          50,
			CacheReadInputTokens:  20,
			CacheWriteInputTokens: 10,
			Metadata:              json.RawMessage(`{"cache_read_input": 20, "cache_creation_input": 10}`),
			CreatedAt:             now,
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
			InterceptionID:       child.ID,
			ProviderResponseID:   "resp-2",
			InputTokens:          200,
			OutputTokens:         100,
			CacheReadInputTokens: 30,
			Metadata:             json.RawMessage(`{"cache_read_input": 30}`),
			CreatedAt:            now.Add(time.Minute),
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

		// Thread-level token aggregation
		require.EqualValues(t, 300, thread.TokenUsage.InputTokens)
		require.EqualValues(t, 150, thread.TokenUsage.OutputTokens)
		require.EqualValues(t, 50, thread.TokenUsage.CacheReadInputTokens)
		require.EqualValues(t, 10, thread.TokenUsage.CacheWriteInputTokens)
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

			// Token usage on root: 100 input, 50 output, 20 cache read, 5 cache write.
			dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				InterceptionID:        root.ID,
				ProviderResponseID:    "resp-root",
				InputTokens:           100,
				OutputTokens:          50,
				CacheReadInputTokens:  20,
				CacheWriteInputTokens: 5,
				Metadata:              json.RawMessage(`{"cache_read_input": 20, "cache_creation_input": 5}`),
				CreatedAt:             now.Add(offset),
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

			// Token usage on child: 200 input, 100 output, 30 cache read.
			dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				InterceptionID:       child.ID,
				ProviderResponseID:   "resp-child",
				InputTokens:          200,
				OutputTokens:         100,
				CacheReadInputTokens: 30,
				Metadata:             json.RawMessage(`{"cache_read_input": 30}`),
				CreatedAt:            now.Add(offset + 15*time.Minute),
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

		// Session-level cache tokens: 3 * (root 20 + child 30) = 150 read,
		// 3 * root 5 = 15 write.
		require.EqualValues(t, 150, res.TokenUsageSummary.CacheReadInputTokens)
		require.EqualValues(t, 15, res.TokenUsageSummary.CacheWriteInputTokens)
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

func TestAIBridgeAllowBYOK(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		allowBYOK      bool
		reqHeaders     map[string]string
		expectedStatus int
	}{
		{
			name:      "byok_enabled/centralized_request",
			allowBYOK: true,
			reqHeaders: map[string]string{
				"Authorization": "Bearer coder-token",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "byok_enabled/byok_request",
			allowBYOK: true,
			reqHeaders: map[string]string{
				agplaibridge.HeaderCoderToken: "coder-token",
				"Authorization":               "Bearer user-llm-key",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "byok_disabled/centralized_request",
			allowBYOK: false,
			reqHeaders: map[string]string{
				"Authorization": "Bearer coder-token",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "byok_disabled/byok_request",
			allowBYOK: false,
			reqHeaders: map[string]string{
				agplaibridge.HeaderCoderToken: "coder-token",
				"Authorization":               "Bearer user-llm-key",
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dv := coderdtest.DeploymentValues(t)
			dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
			dv.AI.BridgeConfig.AllowBYOK = serpent.Bool(tc.allowBYOK)

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

			testHandler := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(http.StatusOK)
			})
			api.AGPL.RegisterInMemoryAIBridgedHTTPHandler(testHandler)

			ctx := testutil.Context(t, testutil.WaitLong)
			reqURL := client.URL.String() + "/api/v2/ai-gateway/test"
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
			require.NoError(t, err)
			req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
			for k, v := range tc.reqHeaders {
				req.Header.Set(k, v)
			}

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, tc.expectedStatus, resp.StatusCode)

			if tc.expectedStatus == http.StatusForbidden {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Contains(t, string(body), "Bring Your Own Key (BYOK) mode is not enabled.")
			}
		})
	}
}

func TestGroupAIBudget(t *testing.T) {
	t.Parallel()

	t.Run("Upsert", func(t *testing.T) {
		t.Parallel()

		adminClient, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		// First upsert creates the budget.
		newBudget, err := adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)
		require.Equal(t, group.ID, newBudget.GroupID)
		require.EqualValues(t, 500_000_000, newBudget.SpendLimitMicros)

		// Second upsert updates the existing budget.
		updatedBudget, err := adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 1_000_000_000,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1_000_000_000, updatedBudget.SpendLimitMicros)

		// GET returns the latest value.
		currentBudget, err := adminClient.GroupAIBudget(ctx, group.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1_000_000_000, currentBudget.SpendLimitMicros)
	})

	t.Run("GetWhenAbsent_404", func(t *testing.T) {
		t.Parallel()

		adminClient, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := adminClient.GroupAIBudget(ctx, group.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("DeleteWhenAbsent_404", func(t *testing.T) {
		t.Parallel()

		adminClient, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		err := adminClient.DeleteGroupAIBudget(ctx, group.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("DeleteWhenPresent", func(t *testing.T) {
		t.Parallel()

		adminClient, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)

		require.NoError(t, adminClient.DeleteGroupAIBudget(ctx, group.ID))

		_, err = adminClient.GroupAIBudget(ctx, group.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("RejectsNegativeSpendLimit", func(t *testing.T) {
		t.Parallel()

		adminClient, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: -1,
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("SpendLimitMaximum", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name      string
			limit     int64
			wantError bool
		}{
			{name: "AtMaximum", limit: codersdk.MaxAISpendLimitMicros},
			{name: "AboveMaximum", limit: codersdk.MaxAISpendLimitMicros + 1, wantError: true},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				adminClient, group := setupGroupAIBudgetTest(t)
				ctx := testutil.Context(t, testutil.WaitLong)

				budget, err := adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
					SpendLimitMicros: tc.limit,
				})
				if tc.wantError {
					var sdkErr *codersdk.Error
					require.ErrorAs(t, err, &sdkErr)
					require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.limit, budget.SpendLimitMicros)
			})
		}
	})

	t.Run("AcceptsZeroSpendLimitToBlock", func(t *testing.T) {
		t.Parallel()

		adminClient, group := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		// 0 is a valid value: it blocks all spend for the group's members.
		budget, err := adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 0,
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, budget.SpendLimitMicros)
	})

	t.Run("UnknownGroup_404", func(t *testing.T) {
		t.Parallel()

		adminClient, _ := setupGroupAIBudgetTest(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := adminClient.GroupAIBudget(ctx, uuid.New())
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("GroupMemberCanReadButNotWrite", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
					codersdk.FeatureAIBridge:     1,
				},
			},
		})
		adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
		memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "budget-group",
		})
		require.NoError(t, err)

		// Add the member to the group so the Group.RBACObject ACL grants them read.
		_, err = adminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{member.ID.String()},
		})
		require.NoError(t, err)

		// Admin sets the budget so there is a row to read.
		_, err = adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)

		// Group members can read the budget.
		got, err := memberClient.GroupAIBudget(ctx, group.ID)
		require.NoError(t, err)
		require.EqualValues(t, 500_000_000, got.SpendLimitMicros)

		// Group members cannot write the budget.
		_, err = memberClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 1_000_000_000,
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		// Group members cannot delete the budget.
		err = memberClient.DeleteGroupAIBudget(ctx, group.ID)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		// The failed upsert and delete left the budget untouched.
		got, err = memberClient.GroupAIBudget(ctx, group.ID)
		require.NoError(t, err)
		require.EqualValues(t, 500_000_000, got.SpendLimitMicros)
	})

	t.Run("Audit", func(t *testing.T) {
		t.Parallel()

		// The enterprise auditor is needed because the mock auditor does
		// not compute diffs. We read straight from the audit_logs table to
		// validate the diff content.
		db, ps := dbtestutil.NewDB(t)
		auditor := entaudit.NewAuditor(
			db,
			entaudit.DefaultFilter,
			backends.NewPostgres(db, true),
		)
		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			AuditLogging: true,
			Options: &coderdtest.Options{
				DeploymentValues: dv,
				Database:         db,
				Pubsub:           ps,
				Auditor:          auditor,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
					codersdk.FeatureAIBridge:     1,
					codersdk.FeatureAuditLog:     1,
				},
			},
		})
		adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "budget-audit",
		})
		require.NoError(t, err)

		// Upsert (create-or-update) emits an AuditActionWrite entry.
		_, err = adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)

		// Delete emits an AuditActionDelete entry against the same resource.
		require.NoError(t, adminClient.DeleteGroupAIBudget(ctx, group.ID))
		rows, err := db.GetAuditLogsOffset(
			ctx,
			database.GetAuditLogsOffsetParams{
				ResourceType: string(database.ResourceTypeGroupAIBudget),
				LimitOpt:     10,
			},
		)
		require.NoError(t, err)
		require.Len(t, rows, 2, "expected one upsert and one delete audit entry")
		// Match rows by action, not position. GetAuditLogsOffset does not
		// guarantee row order.
		byAction := auditLogsByAction(t, rows)
		upsertLog := byAction[database.AuditActionWrite]
		deleteLog := byAction[database.AuditActionDelete]

		require.Equal(t, database.AuditActionWrite, upsertLog.Action)
		require.Equal(t, group.ID, upsertLog.ResourceID)
		require.Equal(t, database.ResourceTypeGroupAIBudget, upsertLog.ResourceType)
		require.Equal(t, group.Name, upsertLog.ResourceTarget)
		require.Equal(t, owner.OrganizationID, upsertLog.OrganizationID)

		var upsertDiff audit.Map
		require.NoError(t, json.Unmarshal(upsertLog.Diff, &upsertDiff))
		require.Contains(t, upsertDiff, "spend_limit")
		require.Equal(t, "$0.00", upsertDiff["spend_limit"].Old)
		require.Equal(t, "$500.00", upsertDiff["spend_limit"].New)
		// Fields marked ActionIgnore must not appear in the diff.
		require.NotContains(t, upsertDiff, "group_id")
		require.NotContains(t, upsertDiff, "group_name")
		require.NotContains(t, upsertDiff, "spend_limit_micros")
		require.NotContains(t, upsertDiff, "created_at")
		require.NotContains(t, upsertDiff, "updated_at")

		require.Equal(t, database.AuditActionDelete, deleteLog.Action)
		require.Equal(t, group.ID, deleteLog.ResourceID)
		require.Equal(t, database.ResourceTypeGroupAIBudget, deleteLog.ResourceType)
		require.Equal(t, group.Name, deleteLog.ResourceTarget)
		require.Equal(t, owner.OrganizationID, deleteLog.OrganizationID)

		var deleteDiff audit.Map
		require.NoError(t, json.Unmarshal(deleteLog.Diff, &deleteDiff))
		require.Contains(t, deleteDiff, "spend_limit")
		require.Equal(t, "$500.00", deleteDiff["spend_limit"].Old)
		require.Equal(t, "", deleteDiff["spend_limit"].New)
	})
}

func TestUserAIBudgetOverride(t *testing.T) {
	t.Parallel()

	t.Run("Upsert/CreatesAndUpdates", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// First upsert creates the override.
		newOverride, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          group.ID,
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)
		require.Equal(t, targetUser.ID, newOverride.UserID)
		require.Equal(t, group.ID, newOverride.GroupID)
		require.EqualValues(t, 500_000_000, newOverride.SpendLimitMicros)

		// Second upsert updates the existing override.
		updatedOverride, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          group.ID,
			SpendLimitMicros: 1_000_000_000,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1_000_000_000, updatedOverride.SpendLimitMicros)

		// GET returns the latest value.
		currentOverride, err := adminClient.UserAIBudgetOverride(ctx, targetUser.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1_000_000_000, currentOverride.SpendLimitMicros)
	})

	t.Run("Upsert/ReassignsGroup", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, groupA := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// First upsert: attribute spend to groupA.
		_, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          groupA.ID,
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)

		// Create groupB in the same org and add the target user.
		groupB, err := adminClient.CreateGroup(ctx, targetUser.OrganizationIDs[0], codersdk.CreateGroupRequest{
			Name: "reassign-test-group-b",
		})
		require.NoError(t, err)
		_, err = adminClient.PatchGroup(ctx, groupB.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{targetUser.ID.String()},
		})
		require.NoError(t, err)

		// Reassign the override's attribution to groupB.
		updated, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          groupB.ID,
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)
		require.Equal(t, groupB.ID, updated.GroupID, "upsert should change attributed group")

		// GET reflects the new group.
		got, err := adminClient.UserAIBudgetOverride(ctx, targetUser.ID)
		require.NoError(t, err)
		require.Equal(t, groupB.ID, got.GroupID, "GET should reflect new group")
	})

	t.Run("Upsert/EveryoneGroup", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, _ := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// The Everyone group has id == organization_id, and the target user
		// is implicitly a member via organization_members rather than
		// group_members. The membership trigger queries
		// group_members_expanded (a UNION of both tables), so this case
		// exercises the organization_members branch.
		everyoneGroupID := targetUser.OrganizationIDs[0]

		override, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          everyoneGroupID,
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err, "should be able to attribute override to Everyone group")
		require.Equal(t, targetUser.ID, override.UserID)
		require.Equal(t, everyoneGroupID, override.GroupID)
		require.EqualValues(t, 500_000_000, override.SpendLimitMicros)
	})

	t.Run("Upsert/AcceptsZeroSpendLimit", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// 0 is a valid value: it blocks all spend for the user.
		override, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          group.ID,
			SpendLimitMicros: 0,
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, override.SpendLimitMicros)
	})

	t.Run("Upsert/RejectsNegativeSpend", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          group.ID,
			SpendLimitMicros: -1,
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("Upsert/SpendLimitMaximum", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name      string
			limit     int64
			wantError bool
		}{
			{name: "AtMaximum", limit: codersdk.MaxAISpendLimitMicros},
			{name: "AboveMaximum", limit: codersdk.MaxAISpendLimitMicros + 1, wantError: true},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-max-group"})
				ctx := testutil.Context(t, testutil.WaitLong)

				override, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
					GroupID:          group.ID,
					SpendLimitMicros: tc.limit,
				})
				if tc.wantError {
					var sdkErr *codersdk.Error
					require.ErrorAs(t, err, &sdkErr)
					require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.limit, override.SpendLimitMicros)
			})
		}
	})

	t.Run("Upsert/RejectsUnknownGroup", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, _ := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// A group_id that doesn't exist (or that the caller can't see)
		// is rejected by the visibility check before the membership check.
		_, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          uuid.New(),
			SpendLimitMicros: 500_000_000,
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("Upsert/RejectsNonMemberGroup", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, _ := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Create a second group the target is NOT a member of.
		outsiderGroup, err := adminClient.CreateGroup(ctx, targetUser.OrganizationIDs[0], codersdk.CreateGroupRequest{
			Name: "outsider-group",
		})
		require.NoError(t, err)

		_, err = adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          outsiderGroup.ID,
			SpendLimitMicros: 500_000_000,
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("Get/AbsentReturns404", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, _ := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := adminClient.UserAIBudgetOverride(ctx, targetUser.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("Get/UnknownUserReturns404", func(t *testing.T) {
		t.Parallel()

		adminClient, _, _ := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := adminClient.UserAIBudgetOverride(ctx, uuid.New())
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("Delete/RoundTrip", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          group.ID,
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)

		require.NoError(t, adminClient.DeleteUserAIBudgetOverride(ctx, targetUser.ID))

		_, err = adminClient.UserAIBudgetOverride(ctx, targetUser.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("Delete/AbsentReturns404", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, _ := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "override-test-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		err := adminClient.DeleteUserAIBudgetOverride(ctx, targetUser.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("Audit/CreatesAndDeletes", func(t *testing.T) {
		t.Parallel()

		db, adminClient, owner, targetUser := setupUserAIBudgetOverrideAuditTest(t)

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "override-audit",
		})
		require.NoError(t, err)
		_, err = adminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{targetUser.ID.String()},
		})
		require.NoError(t, err)

		// Upsert (create-or-update) emits an AuditActionWrite entry.
		_, err = adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          group.ID,
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)

		// Delete emits an AuditActionDelete entry against the same resource.
		require.NoError(t, adminClient.DeleteUserAIBudgetOverride(ctx, targetUser.ID))

		rows, err := db.GetAuditLogsOffset(
			ctx,
			database.GetAuditLogsOffsetParams{
				ResourceType: string(database.ResourceTypeUserAIBudgetOverride),
				LimitOpt:     10,
			},
		)
		require.NoError(t, err)
		require.Len(t, rows, 2, "expected one upsert and one delete audit entry")
		// Match rows by action, not position. GetAuditLogsOffset does not
		// guarantee row order.
		byAction := auditLogsByAction(t, rows)
		upsertLog := byAction[database.AuditActionWrite]
		deleteLog := byAction[database.AuditActionDelete]

		require.Equal(t, database.AuditActionWrite, upsertLog.Action)
		require.Equal(t, targetUser.ID, upsertLog.ResourceID)
		require.Equal(t, database.ResourceTypeUserAIBudgetOverride, upsertLog.ResourceType)
		require.Equal(t, targetUser.Username, upsertLog.ResourceTarget)
		require.Equal(t, owner.OrganizationID, upsertLog.OrganizationID)

		var upsertDiff audit.Map
		require.NoError(t, json.Unmarshal(upsertLog.Diff, &upsertDiff))
		require.Contains(t, upsertDiff, "spend_limit")
		require.Equal(t, "$0.00", upsertDiff["spend_limit"].Old)
		require.Equal(t, "$500.00", upsertDiff["spend_limit"].New)
		require.Contains(t, upsertDiff, "group_name")
		require.Equal(t, "", upsertDiff["group_name"].Old)
		require.Equal(t, group.Name, upsertDiff["group_name"].New)
		require.Contains(t, upsertDiff, "group_id")
		require.Equal(t, "", upsertDiff["group_id"].Old)
		require.Equal(t, group.ID.String(), upsertDiff["group_id"].New)
		// Fields marked ActionIgnore must not appear in the diff.
		require.NotContains(t, upsertDiff, "user_id")
		require.NotContains(t, upsertDiff, "username")
		require.NotContains(t, upsertDiff, "spend_limit_micros")
		require.NotContains(t, upsertDiff, "created_at")
		require.NotContains(t, upsertDiff, "updated_at")

		require.Equal(t, database.AuditActionDelete, deleteLog.Action)
		require.Equal(t, targetUser.ID, deleteLog.ResourceID)
		require.Equal(t, database.ResourceTypeUserAIBudgetOverride, deleteLog.ResourceType)
		require.Equal(t, targetUser.Username, deleteLog.ResourceTarget)
		require.Equal(t, owner.OrganizationID, deleteLog.OrganizationID)

		var deleteDiff audit.Map
		require.NoError(t, json.Unmarshal(deleteLog.Diff, &deleteDiff))
		require.Contains(t, deleteDiff, "spend_limit")
		require.Equal(t, "$500.00", deleteDiff["spend_limit"].Old)
		require.Equal(t, "", deleteDiff["spend_limit"].New)
		require.Contains(t, deleteDiff, "group_name")
		require.Equal(t, group.Name, deleteDiff["group_name"].Old)
		require.Equal(t, "", deleteDiff["group_name"].New)
		require.Contains(t, deleteDiff, "group_id")
		require.Equal(t, group.ID.String(), deleteDiff["group_id"].Old)
		require.Equal(t, "", deleteDiff["group_id"].New)
	})

	t.Run("Audit/DeleteAbsentEmitsNoEntry", func(t *testing.T) {
		t.Parallel()

		// Deleting an override that does not exist must not emit an audit log entry.
		db, adminClient, _, targetUser := setupUserAIBudgetOverrideAuditTest(t)

		ctx := testutil.Context(t, testutil.WaitLong)

		err := adminClient.DeleteUserAIBudgetOverride(ctx, targetUser.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		rows, err := db.GetAuditLogsOffset(
			ctx,
			database.GetAuditLogsOffsetParams{
				ResourceType: string(database.ResourceTypeUserAIBudgetOverride),
				LimitOpt:     10,
			},
		)
		require.NoError(t, err)
		require.Empty(t, rows, "no audit entry expected when delete returns 404")
	})

	t.Run("Audit/UpsertEverything", func(t *testing.T) {
		t.Parallel()

		// A second upsert that reassigns the attributed group and changes
		// the spend limit must record the prior state as the audit
		// before-state.
		db, adminClient, owner, targetUser := setupUserAIBudgetOverrideAuditTest(t)

		ctx := testutil.Context(t, testutil.WaitLong)
		groupA, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "reassign-audit-a",
		})
		require.NoError(t, err)
		_, err = adminClient.PatchGroup(ctx, groupA.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{targetUser.ID.String()},
		})
		require.NoError(t, err)

		groupB, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "reassign-audit-b",
		})
		require.NoError(t, err)
		_, err = adminClient.PatchGroup(ctx, groupB.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{targetUser.ID.String()},
		})
		require.NoError(t, err)

		// First upsert: create the override attributed to groupA.
		_, err = adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          groupA.ID,
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)

		// Second upsert: reassign to groupB and raise the spend limit.
		_, err = adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          groupB.ID,
			SpendLimitMicros: 1_000_000_000,
		})
		require.NoError(t, err)

		rows, err := db.GetAuditLogsOffset(
			ctx,
			database.GetAuditLogsOffsetParams{
				ResourceType: string(database.ResourceTypeUserAIBudgetOverride),
				LimitOpt:     10,
			},
		)
		require.NoError(t, err)
		require.Len(t, rows, 2, "expected one create and one update audit entry")
		// Both upserts emit AuditActionWrite; select the update by the spend
		// limit it results in rather than row order, which GetAuditLogsOffset
		// does not guarantee.
		updateLog := auditLogByNewSpendLimit(t, rows, "$1000.00")

		var updateDiff audit.Map
		require.NoError(t, json.Unmarshal(updateLog.Diff, &updateDiff))
		require.Contains(t, updateDiff, "group_name")
		require.Equal(t, groupA.Name, updateDiff["group_name"].Old)
		require.Equal(t, groupB.Name, updateDiff["group_name"].New)
		require.Contains(t, updateDiff, "group_id")
		require.Equal(t, groupA.ID.String(), updateDiff["group_id"].Old)
		require.Equal(t, groupB.ID.String(), updateDiff["group_id"].New)
		require.Contains(t, updateDiff, "spend_limit")
		require.Equal(t, "$500.00", updateDiff["spend_limit"].Old)
		require.Equal(t, "$1000.00", updateDiff["spend_limit"].New)
	})

	t.Run("Audit/UpsertSpendLimit", func(t *testing.T) {
		t.Parallel()

		// A second upsert that keeps the same group and only changes the
		// spend limit must produce a diff that contains spend_limit and omits
		// the unchanged group_name and group_id.
		db, adminClient, owner, targetUser := setupUserAIBudgetOverrideAuditTest(t)

		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "spend-only-audit",
		})
		require.NoError(t, err)
		_, err = adminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{targetUser.ID.String()},
		})
		require.NoError(t, err)

		// First upsert: create the override attributed to the group.
		_, err = adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          group.ID,
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err)

		// Second upsert: keep the same group, raise only the spend limit.
		_, err = adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          group.ID,
			SpendLimitMicros: 1_000_000_000,
		})
		require.NoError(t, err)

		rows, err := db.GetAuditLogsOffset(
			ctx,
			database.GetAuditLogsOffsetParams{
				ResourceType: string(database.ResourceTypeUserAIBudgetOverride),
				LimitOpt:     10,
			},
		)
		require.NoError(t, err)
		require.Len(t, rows, 2, "expected one create and one update audit entry")
		// Both upserts emit AuditActionWrite; select the update by the spend
		// limit it results in rather than row order, which GetAuditLogsOffset
		// does not guarantee.
		updateLog := auditLogByNewSpendLimit(t, rows, "$1000.00")

		var updateDiff audit.Map
		require.NoError(t, json.Unmarshal(updateLog.Diff, &updateDiff))
		require.Contains(t, updateDiff, "spend_limit")
		require.Equal(t, "$500.00", updateDiff["spend_limit"].Old)
		require.Equal(t, "$1000.00", updateDiff["spend_limit"].New)
		require.NotContains(t, updateDiff, "group_name")
		require.NotContains(t, updateDiff, "group_id")
		require.NotContains(t, updateDiff, "spend_limit_micros")
	})
}

// TestUserAIBudgetOverrideRoleAccess verifies the authz matrix for the roles
// expected to interact with user budget overrides:
//
//   - Owner / UserAdmin: full CRUD.
//   - OrgAdmin / OrgUserAdmin: read-only. Writes require ActionUpdate on the
//     User resource (site-scoped), which neither role has.
//
//nolint:tparallel // Subtests run sequentially: they share the same deployment and group, and parallel PatchGroup calls on the same group race.
func TestUserAIBudgetOverrideRoleAccess(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAIBridge:     1,
			},
		},
	})
	userAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	orgUserAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgUserAdmin(owner.OrganizationID))

	setupCtx := testutil.Context(t, testutil.WaitLong)
	group, err := userAdminClient.CreateGroup(setupCtx, owner.OrganizationID, codersdk.CreateGroupRequest{
		Name: "role-access-group",
	})
	require.NoError(t, err)

	cases := []struct {
		Name     string
		Client   *codersdk.Client
		CanWrite bool
	}{
		{Name: "Owner", Client: ownerClient, CanWrite: true},
		{Name: "UserAdmin", Client: userAdminClient, CanWrite: true},
		{Name: "OrgAdmin", Client: orgAdminClient, CanWrite: false},
		{Name: "OrgUserAdmin", Client: orgUserAdminClient, CanWrite: false},
	}

	//nolint:paralleltest // Subtests run sequentially: they share the same deployment and group, and parallel PatchGroup calls on the same group race.
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := testutil.Context(t, testutil.WaitLong)

			// Each case gets a fresh target user.
			_, targetUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
			_, err := userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
				AddUsers: []string{targetUser.ID.String()},
			})
			require.NoError(t, err)

			upsertReq := codersdk.UpsertUserAIBudgetOverrideRequest{
				GroupID:          group.ID,
				SpendLimitMicros: 500_000_000,
			}

			if tc.CanWrite {
				// Full CRUD lifecycle.
				override, err := tc.Client.UpsertUserAIBudgetOverride(ctx, targetUser.ID, upsertReq)
				require.NoError(t, err, "PUT")
				require.Equal(t, group.ID, override.GroupID)

				got, err := tc.Client.UserAIBudgetOverride(ctx, targetUser.ID)
				require.NoError(t, err, "GET")
				require.EqualValues(t, 500_000_000, got.SpendLimitMicros)

				err = tc.Client.DeleteUserAIBudgetOverride(ctx, targetUser.ID)
				require.NoError(t, err, "DELETE")
			} else {
				// PUT rejected.
				_, err := tc.Client.UpsertUserAIBudgetOverride(ctx, targetUser.ID, upsertReq)
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, http.StatusNotFound, sdkErr.StatusCode(), "PUT")

				// Seed a row via UserAdmin so we can verify read access still works.
				_, err = userAdminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, upsertReq)
				require.NoError(t, err)

				// GET still works (all roles have ActionRead on User).
				got, err := tc.Client.UserAIBudgetOverride(ctx, targetUser.ID)
				require.NoError(t, err, "GET")
				require.EqualValues(t, 500_000_000, got.SpendLimitMicros)

				// DELETE rejected.
				err = tc.Client.DeleteUserAIBudgetOverride(ctx, targetUser.ID)
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, http.StatusNotFound, sdkErr.StatusCode(), "DELETE")
			}
		})
	}
}

// TestUserAIBudgetOverrideDeletedOnMembershipRemoval verifies that a per-user
// override is deleted automatically when the user loses membership in the
// attributed group. Two paths are exercised:
//
//   - RegularGroup: membership stored in group_members; removed via
//     PatchGroup with RemoveUsers.
//   - EveryoneGroup: membership stored in organization_members; removed
//     via DeleteOrganizationMember.
func TestUserAIBudgetOverrideDeletedOnMembershipRemoval(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAIBridge:     1,
			},
		},
	})
	adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

	// "Regular group" means any group except "Everyone".
	t.Run("RegularGroup", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		_, targetUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		group, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "cascade-regular-group",
		})
		require.NoError(t, err)

		_, err = adminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{targetUser.ID.String()},
		})
		require.NoError(t, err)

		_, err = adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          group.ID,
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err, "set override")

		// Sanity-check the override exists.
		_, err = adminClient.UserAIBudgetOverride(ctx, targetUser.ID)
		require.NoError(t, err, "override should exist before removal")

		_, err = adminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			RemoveUsers: []string{targetUser.ID.String()},
		})
		require.NoError(t, err, "remove user from group")

		_, err = adminClient.UserAIBudgetOverride(ctx, targetUser.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode(),
			"override should be deleted after user is removed from the attributed group")
	})

	t.Run("EveryoneGroup", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		_, targetUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		// The Everyone group has id == organization_id.
		everyoneGroupID := owner.OrganizationID

		_, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
			GroupID:          everyoneGroupID,
			SpendLimitMicros: 500_000_000,
		})
		require.NoError(t, err, "set override")

		// Sanity-check the override exists.
		_, err = adminClient.UserAIBudgetOverride(ctx, targetUser.ID)
		require.NoError(t, err, "override should exist before removal")

		err = adminClient.DeleteOrganizationMember(ctx, owner.OrganizationID, targetUser.ID.String())
		require.NoError(t, err, "remove user from organization")

		_, err = adminClient.UserAIBudgetOverride(ctx, targetUser.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode(),
			"override should be deleted after user is removed from the organization")
	})
}

func TestUserAISpendStatus(t *testing.T) {
	t.Parallel()

	t.Run("RequiresLicenseFeature", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is irrelevant here; the request is blocked before RBAC.
		_, err := client.UserAISpendStatus(ctx, uuid.New())
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	tests := []struct {
		name                   string
		groupBudget            *int64 // nil = no group budget configured
		overrideLimit          *int64 // nil = no user override configured
		spent                  int64  // 0 = no spend seeded
		wantHasEffectiveGroup  bool
		wantEffectiveBudget    *codersdk.AIBudgetLimit
		wantCurrentSpendMicros int64
	}{
		{
			name:                  "GroupBudget/ZeroSpend",
			groupBudget:           new(int64(1_000_000_000)),
			wantHasEffectiveGroup: true,
			wantEffectiveBudget: &codersdk.AIBudgetLimit{
				SpendLimitMicros: 1_000_000_000,
				LimitSource:      codersdk.AIBudgetLimitSourceGroup,
			},
		},
		{
			name:                  "GroupBudget/PartialSpend",
			groupBudget:           new(int64(1_000_000_000)),
			spent:                 250_000_000,
			wantHasEffectiveGroup: true,
			wantEffectiveBudget: &codersdk.AIBudgetLimit{
				SpendLimitMicros: 1_000_000_000,
				LimitSource:      codersdk.AIBudgetLimitSourceGroup,
			},
			wantCurrentSpendMicros: 250_000_000,
		},
		{
			name:                  "GroupBudget/SpendExceedsLimit",
			groupBudget:           new(int64(1_000_000_000)),
			spent:                 1_500_000_000,
			wantHasEffectiveGroup: true,
			wantEffectiveBudget: &codersdk.AIBudgetLimit{
				SpendLimitMicros: 1_000_000_000,
				LimitSource:      codersdk.AIBudgetLimitSourceGroup,
			},
			wantCurrentSpendMicros: 1_500_000_000,
		},
		{
			name:                  "UserOverride/ZeroSpend",
			groupBudget:           new(int64(5_000_000_000)),
			overrideLimit:         new(int64(200_000_000)),
			wantHasEffectiveGroup: true,
			wantEffectiveBudget: &codersdk.AIBudgetLimit{
				SpendLimitMicros: 200_000_000,
				LimitSource:      codersdk.AIBudgetLimitSourceUserOverride,
			},
		},
		{
			name:                  "UserOverride/PartialSpend",
			groupBudget:           new(int64(5_000_000_000)),
			overrideLimit:         new(int64(200_000_000)),
			spent:                 50_000_000,
			wantHasEffectiveGroup: true,
			wantEffectiveBudget: &codersdk.AIBudgetLimit{
				SpendLimitMicros: 200_000_000,
				LimitSource:      codersdk.AIBudgetLimitSourceUserOverride,
			},
			wantCurrentSpendMicros: 50_000_000,
		},
		{
			name:                  "UserOverride/SpendExceedsLimit",
			groupBudget:           new(int64(5_000_000_000)),
			overrideLimit:         new(int64(200_000_000)),
			spent:                 350_000_000,
			wantHasEffectiveGroup: true,
			wantEffectiveBudget: &codersdk.AIBudgetLimit{
				SpendLimitMicros: 200_000_000,
				LimitSource:      codersdk.AIBudgetLimitSourceUserOverride,
			},
			wantCurrentSpendMicros: 350_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clock := quartz.NewMock(t)
			db, ps := dbtestutil.NewDB(t)
			adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
				GroupName: "spend-test-group",
				Clock:     clock,
				Database:  db,
				Pubsub:    ps,
			})
			ctx := testutil.Context(t, testutil.WaitLong)

			// Use fixed dates to keep the test deterministic.
			clock.Set(time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))
			wantPeriodStart := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
			wantPeriodEnd := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)

			if tt.groupBudget != nil {
				_, err := adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
					SpendLimitMicros: *tt.groupBudget,
				})
				require.NoError(t, err)
			}
			if tt.overrideLimit != nil {
				_, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
					GroupID:          group.ID,
					SpendLimitMicros: *tt.overrideLimit,
				})
				require.NoError(t, err)
			}
			if tt.spent > 0 {
				_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
					UserID:           targetUser.ID,
					EffectiveGroupID: group.ID,
					Day:              clock.Now(),
					CostMicros:       tt.spent,
				})
				require.NoError(t, err)
			}

			got, err := adminClient.UserAISpendStatus(ctx, targetUser.ID)
			require.NoError(t, err)
			require.Equal(t, targetUser.ID, got.UserID)
			require.Equal(t, wantPeriodStart, got.PeriodStart)
			require.Equal(t, wantPeriodEnd, got.PeriodEnd)
			require.Equal(t, tt.wantCurrentSpendMicros, got.CurrentSpendMicros)

			var wantEffectiveGroupID *uuid.UUID
			if tt.wantHasEffectiveGroup {
				wantEffectiveGroupID = &group.ID
			}
			require.Equal(t, wantEffectiveGroupID, got.EffectiveGroupID)
			require.Equal(t, tt.wantEffectiveBudget, got.EffectiveBudget)
		})
	}

	t.Run("UnbudgetedFallsBackToEveryone", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		db, ps := dbtestutil.NewDB(t)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "spend-test-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		clock.Set(time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))

		// With no override or group budget, the effective group is the org's
		// Everyone group (id == org id) with no limit. The reported current
		// spend is the amount attributed to that Everyone group.
		everyoneGroupID := group.OrganizationID
		_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID:           targetUser.ID,
			EffectiveGroupID: everyoneGroupID,
			Day:              clock.Now(),
			CostMicros:       100_000_000,
		})
		require.NoError(t, err)

		got, err := adminClient.UserAISpendStatus(ctx, targetUser.ID)
		require.NoError(t, err)
		require.Equal(t, &everyoneGroupID, got.EffectiveGroupID)
		require.Nil(t, got.EffectiveBudget)
		require.Equal(t, int64(100_000_000), got.CurrentSpendMicros)
	})

	t.Run("NoOrgReturnsNull", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		db, ps := dbtestutil.NewDB(t)
		adminClient, _, _ := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "spend-test-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		clock.Set(time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))

		// A user with no organization membership resolves to no effective group.
		orglessUser := dbgen.User(t, db, database.User{})

		got, err := adminClient.UserAISpendStatus(ctx, orglessUser.ID)
		require.NoError(t, err)
		require.Nil(t, got.EffectiveGroupID)
		require.Nil(t, got.EffectiveBudget)
		require.Equal(t, int64(0), got.CurrentSpendMicros)
	})
}

func TestUserAISpendStatusRoleAccess(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAIBridge:     1,
			},
		},
	})
	userAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	orgUserAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgUserAdmin(owner.OrganizationID))
	memberClient, memberUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	_, targetUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	cases := []struct {
		Name     string
		Client   *codersdk.Client
		Target   uuid.UUID
		WantCode int
	}{
		{Name: "Owner", Client: ownerClient, Target: targetUser.ID, WantCode: http.StatusOK},
		{Name: "UserAdmin", Client: userAdminClient, Target: targetUser.ID, WantCode: http.StatusOK},
		{Name: "OrgAdmin", Client: orgAdminClient, Target: targetUser.ID, WantCode: http.StatusOK},
		{Name: "OrgUserAdmin", Client: orgUserAdminClient, Target: targetUser.ID, WantCode: http.StatusOK},
		{Name: "MemberReadsSelf", Client: memberClient, Target: memberUser.ID, WantCode: http.StatusOK},
		{Name: "MemberReadsOther", Client: memberClient, Target: targetUser.ID, WantCode: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			_, err := tc.Client.UserAISpendStatus(ctx, tc.Target)
			if tc.WantCode == http.StatusOK {
				require.NoError(t, err)
				return
			}
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, tc.WantCode, sdkErr.StatusCode())
		})
	}
}

func TestOrganizationGroupsAISpend(t *testing.T) {
	t.Parallel()

	t.Run("RequiresLicenseFeature", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is irrelevant here; the request is blocked before RBAC.
		_, err := client.OrganizationGroupsAISpend(ctx, owner.OrganizationID, []uuid.UUID{uuid.New()})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "AI Gateway is a Premium feature")
	})

	t.Run("MissingGroupIDs", func(t *testing.T) {
		t.Parallel()

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "missing-ids-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: no group_ids query parameter.
		// When: querying spend.
		_, err := adminClient.OrganizationGroupsAISpend(ctx, group.OrganizationID, nil)

		// Then: request fails with 400.
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("InclusiveMaxGroupIDs", func(t *testing.T) {
		t.Parallel()

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "inclusive-max-group-ids-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: 100 group_ids, exactly at the cap.
		ids := make([]uuid.UUID, 100)
		for i := range ids {
			ids[i] = uuid.New()
		}

		// When: querying spend.
		_, err := adminClient.OrganizationGroupsAISpend(ctx, group.OrganizationID, ids)

		// Then: request succeeds.
		require.NoError(t, err)
	})

	t.Run("TooManyGroupIDs", func(t *testing.T) {
		t.Parallel()

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "too-many-group-ids-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: 101 group_ids, above the cap of 100.
		ids := make([]uuid.UUID, 101)
		for i := range ids {
			ids[i] = uuid.New()
		}

		// When: querying spend.
		_, err := adminClient.OrganizationGroupsAISpend(ctx, group.OrganizationID, ids)

		// Then: request fails with 400.
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("MalformedGroupID", func(t *testing.T) {
		t.Parallel()

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "malformed-group-id-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: a malformed UUID passed via raw HTTP.
		// When: querying spend.
		res, err := adminClient.Request(ctx, http.MethodGet,
			"/api/v2/organizations/"+group.OrganizationID.String()+"/groups/ai/spend",
			nil,
			func(r *http.Request) {
				q := r.URL.Query()
				q.Set("group_ids", "not-a-uuid")
				r.URL.RawQuery = q.Encode()
			},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		// Then: 400.
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("GroupInOtherOrgExcluded", func(t *testing.T) {
		t.Parallel()

		// Given: two groups, one in the queried org and one in a different org.
		db, ps := dbtestutil.NewDB(t)
		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "primary-org-group",
			Database:  db,
			Pubsub:    ps,
		})
		otherOrg := dbgen.Organization(t, db, database.Organization{})
		otherOrgGroup := dbgen.Group(t, db, database.Group{OrganizationID: otherOrg.ID})
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: querying the primary org with both group IDs.
		resp, err := adminClient.OrganizationGroupsAISpend(ctx, group.OrganizationID, []uuid.UUID{group.ID, otherOrgGroup.ID})
		require.NoError(t, err)

		// Then: only the primary-org group is returned.
		require.Len(t, resp.Groups, 1)
		require.Equal(t, group.ID, resp.Groups[0].GroupID)
	})

	// The group has a single member, so a budgeted group's total matches its
	// per-member limit and an unbudgeted group reports null.
	tests := []struct {
		name                string
		setBudget           bool
		spendLimit          int64
		spent               int64
		wantSpendLimit      *int64
		wantTotalSpendLimit *int64
		wantCurrentSpend    int64
	}{
		{
			name: "NoBudgetNoSpend",
		},
		{
			name:                "ZeroLimitBudget",
			setBudget:           true,
			spendLimit:          0,
			wantSpendLimit:      new(int64(0)),
			wantTotalSpendLimit: new(int64(0)),
			wantCurrentSpend:    0,
		},
		{
			name:                "BudgetZeroSpend",
			setBudget:           true,
			spendLimit:          1_000_000_000,
			wantSpendLimit:      new(int64(1_000_000_000)),
			wantTotalSpendLimit: new(int64(1_000_000_000)),
			wantCurrentSpend:    0,
		},
		{
			name:                "BudgetWithSpend",
			setBudget:           true,
			spendLimit:          1_000_000_000,
			spent:               250_000_000,
			wantSpendLimit:      new(int64(1_000_000_000)),
			wantTotalSpendLimit: new(int64(1_000_000_000)),
			wantCurrentSpend:    250_000_000,
		},
		{
			name:                "SpendExceedsLimit",
			setBudget:           true,
			spendLimit:          1_000_000_000,
			spent:               1_500_000_000,
			wantSpendLimit:      new(int64(1_000_000_000)),
			wantTotalSpendLimit: new(int64(1_000_000_000)),
			wantCurrentSpend:    1_500_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Given: an admin, a group, and optionally a budget and seeded spend.
			clock := quartz.NewMock(t)
			db, ps := dbtestutil.NewDB(t)
			adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
				GroupName: "spend-test-group",
				Clock:     clock,
				Database:  db,
				Pubsub:    ps,
			})
			ctx := testutil.Context(t, testutil.WaitLong)
			clock.Set(time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))
			wantPeriodStart := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
			wantPeriodEnd := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)

			if tt.setBudget {
				_, err := adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
					SpendLimitMicros: tt.spendLimit,
				})
				require.NoError(t, err)
			}
			if tt.spent > 0 {
				_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
					UserID:           targetUser.ID,
					EffectiveGroupID: group.ID,
					Day:              clock.Now(),
					CostMicros:       tt.spent,
				})
				require.NoError(t, err)
			}

			// When: querying the group's spend.
			got, err := adminClient.OrganizationGroupsAISpend(ctx, group.OrganizationID, []uuid.UUID{group.ID})
			require.NoError(t, err)

			// Then: the response contains one row with the expected fields.
			require.Equal(t, wantPeriodStart, got.PeriodStart)
			require.Equal(t, wantPeriodEnd, got.PeriodEnd)
			require.Len(t, got.Groups, 1)
			require.Equal(t, group.ID, got.Groups[0].GroupID)
			require.Equal(t, tt.wantSpendLimit, got.Groups[0].SpendLimitMicros)
			require.Equal(t, tt.wantTotalSpendLimit, got.Groups[0].TotalSpendLimitMicros)
			require.Equal(t, tt.wantCurrentSpend, got.Groups[0].CurrentSpendMicros)
		})
	}

	t.Run("TotalCombinesAllMembers", func(t *testing.T) {
		t.Parallel()

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "total-all-members-org-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: a second member in a group with a per-member budget.
		_, second := coderdtest.CreateAnotherUser(t, adminClient, group.OrganizationID)
		_, err := adminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{second.ID.String()},
		})
		require.NoError(t, err)
		_, err = adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 1_000_000_000,
		})
		require.NoError(t, err)

		// When: querying the group's spend.
		got, err := adminClient.OrganizationGroupsAISpend(ctx, group.OrganizationID, []uuid.UUID{group.ID})
		require.NoError(t, err)

		// Then: the total covers both members while the per-member limit is unchanged.
		require.Len(t, got.Groups, 1)
		require.Equal(t, new(int64(1_000_000_000)), got.Groups[0].SpendLimitMicros)
		require.Equal(t, new(int64(2_000_000_000)), got.Groups[0].TotalSpendLimitMicros)
		require.Equal(t, int64(0), got.Groups[0].CurrentSpendMicros)
	})
}

func TestOrganizationGroupsAISpendRoleAccess(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC:          1,
				codersdk.FeatureAIBridge:              1,
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	userAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	orgUserAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgUserAdmin(owner.OrganizationID))
	memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	otherOrg := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{})
	otherOrgMemberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, otherOrg.ID)

	ctx := testutil.Context(t, testutil.WaitLong)
	group, err := userAdminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
		Name: "role-access-group",
	})
	require.NoError(t, err)

	cases := []struct {
		name      string
		client    *codersdk.Client
		wantGroup bool
	}{
		{name: "Owner", client: ownerClient, wantGroup: true},
		{name: "UserAdmin", client: userAdminClient, wantGroup: true},
		{name: "OrgAdmin", client: orgAdminClient, wantGroup: true},
		{name: "OrgUserAdmin", client: orgUserAdminClient, wantGroup: true},
		{name: "Member", client: memberClient, wantGroup: true},
		{name: "OtherOrgMember", client: otherOrgMemberClient, wantGroup: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)

			resp, err := tc.client.OrganizationGroupsAISpend(ctx, owner.OrganizationID, []uuid.UUID{group.ID})
			if !tc.wantGroup {
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
				return
			}
			require.NoError(t, err)
			require.Len(t, resp.Groups, 1)
			require.Equal(t, group.ID, resp.Groups[0].GroupID)
		})
	}
}

// readAISpendExportCSV parses a CSV export body into its records.
func readAISpendExportCSV(t *testing.T, body io.Reader) [][]string {
	t.Helper()
	records, err := csv.NewReader(body).ReadAll()
	require.NoError(t, err)
	return records
}

// readAISpendExportResponse asserts the response is sent once with an accurate
// Content-Length and returns the parsed CSV records.
func readAISpendExportResponse(t *testing.T, res *http.Response) [][]string {
	t.Helper()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, strconv.Itoa(len(body)), res.Header.Get("Content-Length"))
	return readAISpendExportCSV(t, bytes.NewReader(body))
}

// requestAISpendExport issues a raw export request so callers can inspect the
// status code, headers, and CSV body directly.
func requestAISpendExport(ctx context.Context, t *testing.T, client *codersdk.Client, orgID uuid.UUID, params map[string]string) *http.Response {
	t.Helper()
	res, err := client.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/ai/spend/export", orgID),
		nil,
		func(r *http.Request) {
			q := r.URL.Query()
			for k, v := range params {
				q.Set(k, v)
			}
			r.URL.RawQuery = q.Encode()
		},
	)
	require.NoError(t, err)
	return res
}

func TestExportOrganizationAISpend(t *testing.T) {
	t.Parallel()

	t.Run("Enablement", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name            string
			features        license.Features
			wantMsgContains string
		}{
			{
				name:            "RequiresLicenseFeature",
				features:        license.Features{},
				wantMsgContains: "AI Gateway is a Premium feature",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				dv := coderdtest.DeploymentValues(t)
				dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
				client, owner := coderdenttest.New(t, &coderdenttest.Options{
					Options:        &coderdtest.Options{DeploymentValues: dv},
					LicenseOptions: &coderdenttest.LicenseOptions{Features: tc.features},
				})
				ctx := testutil.Context(t, testutil.WaitLong)

				//nolint:gocritic // Owner role is irrelevant because the request is blocked before RBAC.
				_, err := client.ExportOrganizationAISpend(ctx, owner.OrganizationID, codersdk.AISpendPeriodWindow{})
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
				require.Contains(t, sdkErr.Message, tc.wantMsgContains)
			})
		}
	})

	t.Run("PeriodValidation", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-period-validation-group",
			Clock:     clock,
		})
		start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
		// Older than the default 60d retention window relative to the clock.
		beforeRetention := time.Date(2025, time.December, 1, 0, 0, 0, 0, time.UTC)

		// wantMsgContains pins the branch each case exercises, since every
		// rejection returns 400 and would otherwise be indistinguishable.
		cases := []struct {
			name            string
			params          map[string]string
			wantStatus      int
			wantMsgContains string
		}{
			{
				name:            "OnlyStart",
				params:          map[string]string{"period_start": start.Format(time.RFC3339Nano)},
				wantStatus:      http.StatusBadRequest,
				wantMsgContains: "must be provided together",
			},
			{
				name:            "OnlyEnd",
				params:          map[string]string{"period_end": start.Format(time.RFC3339Nano)},
				wantStatus:      http.StatusBadRequest,
				wantMsgContains: "must be provided together",
			},
			{
				name:            "StartEqualsEnd",
				params:          map[string]string{"period_start": start.Format(time.RFC3339Nano), "period_end": start.Format(time.RFC3339Nano)},
				wantStatus:      http.StatusBadRequest,
				wantMsgContains: `"period_start" must be before "period_end"`,
			},
			{
				name:            "InvalidFormat",
				params:          map[string]string{"period_start": "not-a-date", "period_end": start.AddDate(0, 0, 1).Format(time.RFC3339Nano)},
				wantStatus:      http.StatusBadRequest,
				wantMsgContains: "have invalid values",
			},
			{
				name:            "PeriodTooLong",
				params:          map[string]string{"period_start": start.Format(time.RFC3339Nano), "period_end": start.AddDate(0, 0, 32).Format(time.RFC3339Nano)},
				wantStatus:      http.StatusBadRequest,
				wantMsgContains: "must not exceed 31 days",
			},
			{
				name:       "MaxPeriodAllowed",
				params:     map[string]string{"period_start": start.Format(time.RFC3339Nano), "period_end": start.AddDate(0, 0, 31).Format(time.RFC3339Nano)},
				wantStatus: http.StatusOK,
			},
			{
				// period_start predates the retention window, so the raw
				// token usage would be purged and results incomplete.
				name:            "BeforeRetentionWindow",
				params:          map[string]string{"period_start": beforeRetention.Format(time.RFC3339Nano), "period_end": beforeRetention.AddDate(0, 0, 1).Format(time.RFC3339Nano)},
				wantStatus:      http.StatusBadRequest,
				wantMsgContains: "retention window",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, tc.params)
				defer res.Body.Close()
				require.Equal(t, tc.wantStatus, res.StatusCode)
				if tc.wantMsgContains == "" {
					return
				}
				var sdkErr *codersdk.Error
				require.ErrorAs(t, codersdk.ReadBodyAsError(res), &sdkErr)
				require.Contains(t, sdkErr.Message, tc.wantMsgContains)
			})
		}
	})

	t.Run("DefaultsToCurrentMonthAndAggregates", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		db, ps := dbtestutil.NewDB(t)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-default-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		inMonth := time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC)
		groupID := uuid.NullUUID{UUID: group.ID, Valid: true}

		// Two claude-4 interceptions for the same user aggregate into one row.
		for _, tu := range []database.InsertAIBridgeTokenUsageParams{
			{InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 10, CacheWriteInputTokens: 5, CostMicros: sql.NullInt64{Int64: 1000, Valid: true}},
			{InputTokens: 200, OutputTokens: 100, CacheReadInputTokens: 20, CacheWriteInputTokens: 10, CostMicros: sql.NullInt64{Int64: 2000, Valid: true}},
		} {
			intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID: targetUser.ID, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: inMonth,
			}, nil)
			tu.InterceptionID = intc.ID
			tu.CreatedAt = inMonth
			tu.EffectiveGroupID = groupID
			dbgen.AIBridgeTokenUsage(t, db, tu)
		}

		// Now: 15 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		require.Equal(t, entcoderd.AISpendExportCSVHeader, records[0])
		require.Len(t, records, 2) // header + single aggregated row

		// The default window echoes the current UTC month.
		require.Equal(t, []string{
			targetUser.ID.String(), targetUser.Username,
			group.ID.String(), group.Name,
			group.OrganizationID.String(), group.OrganizationName,
			"claude-4", "anthropic", "anthropic-prod", "300", "150", "30", "15", "3000",
			"2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z",
		}, records[1])
	})

	t.Run("DefaultPeriodClampedToRetention", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		db, ps := dbtestutil.NewDB(t)
		retention := 14 * 24 * time.Hour
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-retention-clamp-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
			Retention: &retention,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		// Retention starts on 6 March 2026 12:00 UTC.
		retentionStart := now.Add(-retention)
		groupID := uuid.NullUUID{UUID: group.ID, Valid: true}

		// Usage before the retention start falls outside the narrowed period.
		for _, seed := range []struct {
			at           time.Time
			inputTokens  int64
			outputTokens int64
			costMicros   int64
		}{
			{at: retentionStart.Add(-time.Hour), inputTokens: 999, outputTokens: 999, costMicros: 9999},
			{at: retentionStart.Add(time.Hour), inputTokens: 100, outputTokens: 50, costMicros: 1000},
		} {
			intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID: targetUser.ID, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: seed.at,
			}, nil)
			dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				InterceptionID: intc.ID, CreatedAt: seed.at, EffectiveGroupID: groupID,
				InputTokens: seed.inputTokens, OutputTokens: seed.outputTokens,
				CostMicros: sql.NullInt64{Int64: seed.costMicros, Valid: true},
			})
		}

		// Start: 6 March 2026 12:00 UTC (inclusive).
		// End:   1 April 2026 00:00 UTC (exclusive).
		// Now:   20 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		require.Len(t, records, 2) // header + only the retained row
		require.Equal(t, []string{
			targetUser.ID.String(), targetUser.Username,
			group.ID.String(), group.Name,
			group.OrganizationID.String(), group.OrganizationName,
			"claude-4", "anthropic", "anthropic-prod", "100", "50", "0", "0", "1000",
			"2026-03-06T12:00:00Z", "2026-04-01T00:00:00Z",
		}, records[1])
	})

	t.Run("CustomPeriodHalfOpen", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		db, ps := dbtestutil.NewDB(t)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-custom-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		effectiveGroupID := uuid.NullUUID{UUID: group.ID, Valid: true}
		start := time.Date(2026, time.March, 10, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC)

		// Usage at start is included, usage at end is excluded.
		for _, at := range []time.Time{start, end} {
			intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID: targetUser.ID, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: at,
			}, nil)
			dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				InterceptionID: intc.ID, CreatedAt: at, EffectiveGroupID: effectiveGroupID,
				InputTokens: 100, OutputTokens: 50, CostMicros: sql.NullInt64{Int64: 1000, Valid: true},
			})
		}

		// Start: 10 March 2026 00:00 UTC (inclusive).
		// End:   11 March 2026 00:00 UTC (exclusive).
		// Now:   15 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, map[string]string{
			"period_start": start.Format(time.RFC3339Nano),
			"period_end":   end.Format(time.RFC3339Nano),
		})
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		require.Len(t, records, 2) // header + only the start-boundary row
		require.Equal(t, []string{
			targetUser.ID.String(), targetUser.Username,
			group.ID.String(), group.Name,
			group.OrganizationID.String(), group.OrganizationName,
			"claude-4", "anthropic", "anthropic-prod", "100", "50", "0", "0", "1000",
			start.Format(time.RFC3339), end.Format(time.RFC3339),
		}, records[1])
	})

	t.Run("ExplicitPeriodBeforeRetentionRejected", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		retention := 14 * 24 * time.Hour
		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-retention-reject-group",
			Clock:     clock,
			Retention: &retention,
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		// The requested start is an hour before the retention window begins, so
		// the request fails instead of being shortened like the default period.
		start := now.Add(-retention).Add(-time.Hour)

		_, err := adminClient.ExportOrganizationAISpend(ctx, group.OrganizationID, codersdk.AISpendPeriodWindow{
			PeriodStart: start,
			PeriodEnd:   now,
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "retention window")
		require.Contains(t, sdkErr.Message, retention.String())
	})

	t.Run("RetentionDisabledAllowsOldPeriod", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		db, ps := dbtestutil.NewDB(t)
		// A retention of zero disables purging, so no period is too old to
		// export and neither the narrowing nor the rejection applies.
		retention := time.Duration(0)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-retention-disabled-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
			Retention: &retention,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		// Two years before the request, far outside any retention window.
		at := time.Date(2024, time.March, 10, 8, 0, 0, 0, time.UTC)
		effectiveGroupID := uuid.NullUUID{UUID: group.ID, Valid: true}

		intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: targetUser.ID, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: at,
		}, nil)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: intc.ID, CreatedAt: at, EffectiveGroupID: effectiveGroupID,
			InputTokens: 100, OutputTokens: 50, CostMicros: sql.NullInt64{Int64: 1000, Valid: true},
		})

		// Start: 10 March 2024 07:00 UTC (inclusive).
		// End:   10 March 2024 09:00 UTC (exclusive).
		// Now:   20 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, map[string]string{
			"period_start": at.Add(-time.Hour).Format(time.RFC3339Nano),
			"period_end":   at.Add(time.Hour).Format(time.RFC3339Nano),
		})
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		require.Len(t, records, 2) // header + the retained row
		require.Equal(t, []string{
			targetUser.ID.String(), targetUser.Username,
			group.ID.String(), group.Name,
			group.OrganizationID.String(), group.OrganizationName,
			"claude-4", "anthropic", "anthropic-prod", "100", "50", "0", "0", "1000",
			"2024-03-10T07:00:00Z", "2024-03-10T09:00:00Z",
		}, records[1])
	})

	t.Run("SeparateRowPerModel", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		db, ps := dbtestutil.NewDB(t)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-per-model-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		inMonth := time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC)
		effectiveGroupID := uuid.NullUUID{UUID: group.ID, Valid: true}

		// Usage for two different models produces one row each.
		claudeIntc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: targetUser.ID, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: inMonth,
		}, nil)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: claudeIntc.ID, CreatedAt: inMonth, EffectiveGroupID: effectiveGroupID,
			InputTokens: 100, OutputTokens: 50, CostMicros: sql.NullInt64{Int64: 1000, Valid: true},
		})
		gptIntc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: targetUser.ID, Provider: "openai", ProviderName: "openai-prod", Model: "gpt-4", StartedAt: inMonth,
		}, nil)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: gptIntc.ID, CreatedAt: inMonth, EffectiveGroupID: effectiveGroupID,
			InputTokens: 500, OutputTokens: 250, CostMicros: sql.NullInt64{Int64: 5000, Valid: true},
		})

		// Now: 15 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		require.Len(t, records, 3) // header + one row per model

		userID := targetUser.ID.String()
		username := targetUser.Username
		groupID := group.ID.String()
		groupName := group.Name
		orgID := group.OrganizationID.String()
		orgName := group.OrganizationName
		periodStart := "2026-03-01T00:00:00Z"
		periodEnd := "2026-04-01T00:00:00Z"
		// Ordered by provider then model: anthropic/claude-4, then openai/gpt-4.
		require.Equal(t, []string{userID, username, groupID, groupName, orgID, orgName, "claude-4", "anthropic", "anthropic-prod", "100", "50", "0", "0", "1000", periodStart, periodEnd}, records[1])
		require.Equal(t, []string{userID, username, groupID, groupName, orgID, orgName, "gpt-4", "openai", "openai-prod", "500", "250", "0", "0", "5000", periodStart, periodEnd}, records[2])
	})

	t.Run("SeparateRowPerProviderName", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		db, ps := dbtestutil.NewDB(t)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-per-provider-name-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		inMonth := time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC)
		effectiveGroupID := uuid.NullUUID{UUID: group.ID, Valid: true}

		// Two configurations of the same provider, same model. Spend is reported
		// per configuration rather than merged into one provider row.
		for _, seed := range []struct {
			providerName string
			inputTokens  int64
			outputTokens int64
			costMicros   int64
		}{
			{providerName: "anthropic-dev", inputTokens: 100, outputTokens: 50, costMicros: 1000},
			{providerName: "anthropic-prod", inputTokens: 500, outputTokens: 250, costMicros: 5000},
		} {
			intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID: targetUser.ID, Provider: "anthropic", ProviderName: seed.providerName, Model: "claude-4", StartedAt: inMonth,
			}, nil)
			dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				InterceptionID: intc.ID, CreatedAt: inMonth, EffectiveGroupID: effectiveGroupID,
				InputTokens: seed.inputTokens, OutputTokens: seed.outputTokens,
				CostMicros: sql.NullInt64{Int64: seed.costMicros, Valid: true},
			})
		}

		// Now: 15 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		require.Len(t, records, 3) // header + one row per provider name

		userID := targetUser.ID.String()
		username := targetUser.Username
		groupID := group.ID.String()
		groupName := group.Name
		orgID := group.OrganizationID.String()
		orgName := group.OrganizationName
		periodStart := "2026-03-01T00:00:00Z"
		periodEnd := "2026-04-01T00:00:00Z"
		// Ordered by provider name: anthropic-dev, then anthropic-prod.
		require.Equal(t, []string{userID, username, groupID, groupName, orgID, orgName, "claude-4", "anthropic", "anthropic-dev", "100", "50", "0", "0", "1000", periodStart, periodEnd}, records[1])
		require.Equal(t, []string{userID, username, groupID, groupName, orgID, orgName, "claude-4", "anthropic", "anthropic-prod", "500", "250", "0", "0", "5000", periodStart, periodEnd}, records[2])
	})

	t.Run("SeparateRowPerGroup", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		db, ps := dbtestutil.NewDB(t)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-per-group-first",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		inMonth := time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC)

		// A second group in the same organization. The user is not added to it:
		// the effective group is snapshotted on each token usage, so spend from
		// before a membership or budget change stays attributed to the old group.
		secondGroup, err := adminClient.CreateGroup(ctx, group.OrganizationID, codersdk.CreateGroupRequest{
			Name: "export-per-group-second",
		})
		require.NoError(t, err)

		for _, seed := range []struct {
			groupID      uuid.UUID
			inputTokens  int64
			outputTokens int64
			costMicros   int64
		}{
			{groupID: group.ID, inputTokens: 100, outputTokens: 50, costMicros: 1000},
			{groupID: secondGroup.ID, inputTokens: 500, outputTokens: 250, costMicros: 5000},
		} {
			intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID: targetUser.ID, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: inMonth,
			}, nil)
			dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				InterceptionID: intc.ID, CreatedAt: inMonth,
				EffectiveGroupID: uuid.NullUUID{UUID: seed.groupID, Valid: true},
				InputTokens:      seed.inputTokens, OutputTokens: seed.outputTokens,
				CostMicros: sql.NullInt64{Int64: seed.costMicros, Valid: true},
			})
		}

		// Now: 15 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		require.Len(t, records, 3) // header + one row per group

		userID := targetUser.ID.String()
		username := targetUser.Username
		orgID := group.OrganizationID.String()
		orgName := group.OrganizationName
		periodStart := "2026-03-01T00:00:00Z"
		periodEnd := "2026-04-01T00:00:00Z"
		// Rows are ordered by group ID, which is a random UUID, so compare
		// without depending on which group sorts first.
		require.ElementsMatch(t, [][]string{
			{userID, username, group.ID.String(), group.Name, orgID, orgName, "claude-4", "anthropic", "anthropic-prod", "100", "50", "0", "0", "1000", periodStart, periodEnd},
			{userID, username, secondGroup.ID.String(), secondGroup.Name, orgID, orgName, "claude-4", "anthropic", "anthropic-prod", "500", "250", "0", "0", "5000", periodStart, periodEnd},
		}, records[1:])
	})

	t.Run("PreviousMonthExcluded", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		db, ps := dbtestutil.NewDB(t)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-prev-month-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		prevMonth := time.Date(2026, time.February, 20, 8, 0, 0, 0, time.UTC)
		inMonth := time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC)
		groupID := uuid.NullUUID{UUID: group.ID, Valid: true}

		// Previous-month usage falls outside the default window and is excluded.
		prevIntc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: targetUser.ID, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: prevMonth,
		}, nil)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: prevIntc.ID, CreatedAt: prevMonth, EffectiveGroupID: groupID,
			InputTokens: 999, OutputTokens: 999, CostMicros: sql.NullInt64{Int64: 9999, Valid: true},
		})

		// Current-month usage is included.
		intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: targetUser.ID, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: inMonth,
		}, nil)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: intc.ID, CreatedAt: inMonth, EffectiveGroupID: groupID,
			InputTokens: 100, OutputTokens: 50, CostMicros: sql.NullInt64{Int64: 1000, Valid: true},
		})

		// Now: 15 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		// Only the current-month usage is present, so the row carries none of the
		// previous month's tokens or cost.
		require.Len(t, records, 2) // header + current-month row
		require.Equal(t, []string{
			targetUser.ID.String(), targetUser.Username,
			group.ID.String(), group.Name,
			group.OrganizationID.String(), group.OrganizationName,
			"claude-4", "anthropic", "anthropic-prod", "100", "50", "0", "0", "1000",
			"2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z",
		}, records[1])
	})

	t.Run("ExcludesNullEffectiveGroup", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		db, ps := dbtestutil.NewDB(t)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-null-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		at := time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC)

		// Usage with no effective group cannot be attributed to an organization,
		// so it is excluded entirely and the export returns no rows at all.
		nullIntc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: targetUser.ID, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: at,
		}, nil)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: nullIntc.ID, CreatedAt: at,
			InputTokens: 500, CostMicros: sql.NullInt64{Int64: 5000, Valid: true},
		})

		// Now: 15 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		require.Equal(t, entcoderd.AISpendExportCSVHeader, records[0])
		require.Len(t, records, 1) // header only
	})

	t.Run("ExcludesOtherOrganizations", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		// Built inline rather than through setupAICostControlTest, which
		// licenses a single organization and returns no owner client.
		db, ps := dbtestutil.NewDB(t)
		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv, Database: db, Pubsub: ps, Clock: clock},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC:          1,
					codersdk.FeatureAIBridge:              1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		inMonth := time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC)

		otherOrg := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{})
		_, otherOrgMember := coderdtest.CreateAnotherUser(t, ownerClient, otherOrg.ID)
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
		group, err := userAdminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "export-org-scope-group",
		})
		require.NoError(t, err)
		otherOrgGroup, err := userAdminClient.CreateGroup(ctx, otherOrg.ID, codersdk.CreateGroupRequest{
			Name: "export-org-scope-other-org-group",
		})
		require.NoError(t, err)

		// Usage in each organization, attributed through that organization's
		// group.
		for _, seed := range []struct {
			initiator uuid.UUID
			groupID   uuid.UUID
		}{
			{initiator: owner.UserID, groupID: group.ID},
			{initiator: otherOrgMember.ID, groupID: otherOrgGroup.ID},
		} {
			intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
				InitiatorID: seed.initiator, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: inMonth,
			}, nil)
			dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
				InterceptionID: intc.ID, CreatedAt: inMonth,
				EffectiveGroupID: uuid.NullUUID{UUID: seed.groupID, Valid: true},
				InputTokens:      100, OutputTokens: 50, CostMicros: sql.NullInt64{Int64: 1000, Valid: true},
			})
		}

		// The owner can read group members in both organizations, so only the
		// query's organization filter keeps the other organization out.
		// Now: 15 March 2026 12:00 UTC.
		//nolint:gocritic // The owner is required to rule out RBAC filtering.
		res := requestAISpendExport(ctx, t, ownerClient, owner.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		require.Len(t, records, 2) // header + the requested organization's row
		require.Equal(t, []string{
			owner.UserID.String(), coderdtest.FirstUserParams.Username,
			group.ID.String(), group.Name,
			owner.OrganizationID.String(), group.OrganizationName,
			"claude-4", "anthropic", "anthropic-prod", "100", "50", "0", "0", "1000",
			"2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z",
		}, records[1])
	})

	t.Run("EscapesFormulaCells", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		db, ps := dbtestutil.NewDB(t)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-formula-escape-group",
			Clock:     clock,
			Database:  db,
			Pubsub:    ps,
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		inMonth := time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC)
		groupID := uuid.NullUUID{UUID: group.ID, Valid: true}

		// Model, provider, and provider name are recorded verbatim from the
		// intercepted request, so a leading formula character must be escaped.
		intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:  targetUser.ID,
			Provider:     "+openai",
			ProviderName: "@prod",
			Model:        `=HYPERLINK("http://insecure/","invoice")`,
			StartedAt:    inMonth,
		}, nil)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID: intc.ID, CreatedAt: inMonth, EffectiveGroupID: groupID,
			InputTokens: 100, OutputTokens: 50, CostMicros: sql.NullInt64{Int64: 1000, Valid: true},
		})

		// Now: 15 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, nil)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		records := readAISpendExportResponse(t, res)
		require.Len(t, records, 2) // header + the escaped row
		require.Equal(t, []string{
			targetUser.ID.String(), targetUser.Username,
			group.ID.String(), group.Name,
			group.OrganizationID.String(), group.OrganizationName,
			`'=HYPERLINK("http://insecure/","invoice")`, "'+openai", "'@prod",
			"100", "50", "0", "0", "1000",
			"2026-03-01T00:00:00Z", "2026-04-01T00:00:00Z",
		}, records[1])
	})

	t.Run("DownloadHeaders", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
		clock := quartz.NewMock(t)
		clock.Set(now)

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "export-download-headers-group",
			Clock:     clock,
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Start: 10 March 2026 00:00 UTC (inclusive).
		// End:   11 March 2026 00:00 UTC (exclusive).
		// Now:   15 March 2026 12:00 UTC.
		res := requestAISpendExport(ctx, t, adminClient, group.OrganizationID, map[string]string{
			"period_start": time.Date(2026, time.March, 10, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			"period_end":   time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		})
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		require.Equal(t, "text/csv; charset=utf-8", res.Header.Get("Content-Type"))
		// The filename carries the organization name and the exported period.
		require.Equal(t,
			fmt.Sprintf(`attachment; filename="ai-spend-export-%s-2026-03-10-to-2026-03-11.csv"`, group.OrganizationName),
			res.Header.Get("Content-Disposition"))

		// No usage is seeded, so only the column header is written. Spelled out
		// rather than compared against the handler's own variable, since the
		// column names are a published contract that a rename would break.
		records := readAISpendExportResponse(t, res)
		require.Equal(t, []string{
			"user_id", "username", "group_id", "group_name", "organization_id", "organization_name",
			"model", "provider", "provider_name",
			"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens",
			"cost_micros", "period_start", "period_end",
		}, records[0])
		require.Len(t, records, 1)
	})
}

func TestGroupAISpend(t *testing.T) {
	t.Parallel()

	t.Run("RequiresLicenseFeature", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "req-license-feature-spend-group",
		})
		require.NoError(t, err)

		_, err = adminClient.GroupAISpend(ctx, group.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "AI Gateway is a Premium feature")
	})

	t.Run("MalformedGroupID", func(t *testing.T) {
		t.Parallel()

		adminClient, _, _ := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "malformed-group-id-spend-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: a malformed UUID in the path.
		// When: querying spend.
		res, err := adminClient.Request(ctx, http.MethodGet, "/api/v2/groups/not-a-uuid/ai/spend", nil)
		require.NoError(t, err)
		defer res.Body.Close()

		// Then: 400.
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("UnknownGroup", func(t *testing.T) {
		t.Parallel()

		adminClient, _, _ := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "unknown-group-spend-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: a group ID that does not exist.
		// When: querying spend.
		_, err := adminClient.GroupAISpend(ctx, uuid.New())

		// Then: request fails with 404.
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	// The group has a single member, so a budgeted group's total matches its
	// per-member limit and an unbudgeted group reports null.
	tests := []struct {
		name                string
		setBudget           bool
		spendLimit          int64
		spent               int64
		wantSpendLimit      *int64
		wantTotalSpendLimit *int64
		wantCurrentSpend    int64
	}{
		{
			name: "NoBudgetNoSpend",
		},
		{
			name:                "ZeroLimitBudget",
			setBudget:           true,
			spendLimit:          0,
			wantSpendLimit:      new(int64(0)),
			wantTotalSpendLimit: new(int64(0)),
			wantCurrentSpend:    0,
		},
		{
			name:                "BudgetZeroSpend",
			setBudget:           true,
			spendLimit:          1_000_000_000,
			wantSpendLimit:      new(int64(1_000_000_000)),
			wantTotalSpendLimit: new(int64(1_000_000_000)),
			wantCurrentSpend:    0,
		},
		{
			name:                "BudgetWithSpend",
			setBudget:           true,
			spendLimit:          1_000_000_000,
			spent:               250_000_000,
			wantSpendLimit:      new(int64(1_000_000_000)),
			wantTotalSpendLimit: new(int64(1_000_000_000)),
			wantCurrentSpend:    250_000_000,
		},
		{
			name:                "SpendExceedsLimit",
			setBudget:           true,
			spendLimit:          1_000_000_000,
			spent:               1_500_000_000,
			wantSpendLimit:      new(int64(1_000_000_000)),
			wantTotalSpendLimit: new(int64(1_000_000_000)),
			wantCurrentSpend:    1_500_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Given: an admin, a group, and optionally a budget and seeded spend.
			clock := quartz.NewMock(t)
			db, ps := dbtestutil.NewDB(t)
			adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
				GroupName: "group-spend-test-group",
				Clock:     clock,
				Database:  db,
				Pubsub:    ps,
			})
			ctx := testutil.Context(t, testutil.WaitLong)
			clock.Set(time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))
			wantPeriodStart := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
			wantPeriodEnd := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)

			if tt.setBudget {
				_, err := adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
					SpendLimitMicros: tt.spendLimit,
				})
				require.NoError(t, err)
			}
			if tt.spent > 0 {
				_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
					UserID:           targetUser.ID,
					EffectiveGroupID: group.ID,
					Day:              clock.Now(),
					CostMicros:       tt.spent,
				})
				require.NoError(t, err)
			}

			// When: querying the group's spend.
			got, err := adminClient.GroupAISpend(ctx, group.ID)
			require.NoError(t, err)

			// Then: the response reports the expected budget and spend.
			require.Equal(t, wantPeriodStart, got.PeriodStart)
			require.Equal(t, wantPeriodEnd, got.PeriodEnd)
			require.Equal(t, group.ID, got.GroupID)
			require.Equal(t, tt.wantSpendLimit, got.SpendLimitMicros)
			require.Equal(t, tt.wantTotalSpendLimit, got.TotalSpendLimitMicros)
			require.Equal(t, tt.wantCurrentSpend, got.CurrentSpendMicros)
		})
	}

	t.Run("TotalCombinesAllMembers", func(t *testing.T) {
		t.Parallel()

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "total-all-members-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: a second member in a group with a per-member budget.
		_, second := coderdtest.CreateAnotherUser(t, adminClient, group.OrganizationID)
		_, err := adminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{second.ID.String()},
		})
		require.NoError(t, err)
		_, err = adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 1_000_000_000,
		})
		require.NoError(t, err)

		// When: querying the group's spend.
		got, err := adminClient.GroupAISpend(ctx, group.ID)
		require.NoError(t, err)

		// Then: the total covers both members while the per-member limit is unchanged.
		require.Equal(t, new(int64(1_000_000_000)), got.SpendLimitMicros)
		require.Equal(t, new(int64(2_000_000_000)), got.TotalSpendLimitMicros)
		require.Equal(t, int64(0), got.CurrentSpendMicros)
	})
}

func TestGroupAISpendRoleAccess(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC:          1,
				codersdk.FeatureAIBridge:              1,
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	userAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	orgUserAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgUserAdmin(owner.OrganizationID))
	memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	otherOrg := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{})
	otherOrgMemberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, otherOrg.ID)

	ctx := testutil.Context(t, testutil.WaitLong)
	group, err := userAdminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
		Name: "group-spend-role-access-group",
	})
	require.NoError(t, err)

	cases := []struct {
		name      string
		client    *codersdk.Client
		wantGroup bool
	}{
		{name: "Owner", client: ownerClient, wantGroup: true},
		{name: "UserAdmin", client: userAdminClient, wantGroup: true},
		{name: "OrgAdmin", client: orgAdminClient, wantGroup: true},
		{name: "OrgUserAdmin", client: orgUserAdminClient, wantGroup: true},
		{name: "Member", client: memberClient, wantGroup: true},
		{name: "OtherOrgMember", client: otherOrgMemberClient, wantGroup: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)

			resp, err := tc.client.GroupAISpend(ctx, group.ID)
			if !tc.wantGroup {
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
				return
			}
			require.NoError(t, err)
			require.Equal(t, group.ID, resp.GroupID)
		})
	}
}

func TestExportOrganizationAISpendRoleAccess(t *testing.T) {
	t.Parallel()

	// Use fixed dates to keep the test deterministic. Seeding at time.Now()
	// against the default month period fails when a run crosses a UTC month
	// boundary.
	now := time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
	inMonth := time.Date(2026, time.March, 10, 8, 0, 0, 0, time.UTC)
	clock := quartz.NewMock(t)
	clock.Set(now)

	db, ps := dbtestutil.NewDB(t)
	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv, Database: db, Pubsub: ps, Clock: clock},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC:          1,
				codersdk.FeatureAIBridge:              1,
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	userAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	orgUserAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgUserAdmin(owner.OrganizationID))
	memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	otherOrg := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{})
	otherOrgMemberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, otherOrg.ID)

	ctx := testutil.Context(t, testutil.WaitLong)
	group, err := userAdminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
		Name: "export-role-access-group",
	})
	require.NoError(t, err)

	// Seed spend for two users in the current month: the owner and a regular
	// member. Both are attributed to the group.
	for _, initiator := range []uuid.UUID{owner.UserID, member.ID} {
		intc := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: initiator, Provider: "anthropic", ProviderName: "anthropic-prod", Model: "claude-4", StartedAt: inMonth,
		}, nil)
		dbgen.AIBridgeTokenUsage(t, db, database.InsertAIBridgeTokenUsageParams{
			InterceptionID:   intc.ID,
			CreatedAt:        inMonth,
			EffectiveGroupID: uuid.NullUUID{UUID: group.ID, Valid: true},
			InputTokens:      100,
			OutputTokens:     50,
			CostMicros:       sql.NullInt64{Int64: 1000, Valid: true},
		})
	}

	cases := []struct {
		name        string
		client      *codersdk.Client
		wantUserIDs []string // expected user_id column values when wantStatus is unset
		wantStatus  int      // non-zero means the request is rejected with this status
	}{
		// Admins see every user's rows.
		{name: "Owner", client: ownerClient, wantUserIDs: []string{owner.UserID.String(), member.ID.String()}},
		{name: "UserAdmin", client: userAdminClient, wantUserIDs: []string{owner.UserID.String(), member.ID.String()}},
		{name: "OrgAdmin", client: orgAdminClient, wantUserIDs: []string{owner.UserID.String(), member.ID.String()}},
		{name: "OrgUserAdmin", client: orgUserAdminClient, wantUserIDs: []string{owner.UserID.String(), member.ID.String()}},
		// The export covers the whole organization, so a regular member is
		// rejected rather than served their own rows.
		{name: "Member", client: memberClient, wantStatus: http.StatusForbidden},
		// A member of another org cannot read this org at all, so it fails
		// earlier, when the organization is resolved.
		{name: "OtherOrgMember", client: otherOrgMemberClient, wantStatus: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			body, err := tc.client.ExportOrganizationAISpend(ctx, owner.OrganizationID, codersdk.AISpendPeriodWindow{})
			if tc.wantStatus != 0 {
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, tc.wantStatus, sdkErr.StatusCode())
				return
			}
			require.NoError(t, err)
			defer body.Close()

			records := readAISpendExportCSV(t, body)
			var gotUserIDs []string
			for _, row := range records[1:] {
				gotUserIDs = append(gotUserIDs, row[0])
			}
			require.ElementsMatch(t, tc.wantUserIDs, gotUserIDs)
		})
	}
}

func TestGroupMembersAISpend(t *testing.T) {
	t.Parallel()

	t.Run("RequiresLicenseFeature", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
				},
			},
		})
		adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)
		group, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "req-license-feature-members-group",
		})
		require.NoError(t, err)

		_, err = adminClient.GroupMembersAISpend(ctx, group.ID, []uuid.UUID{uuid.New()})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "AI Gateway is a Premium feature")
	})

	t.Run("MissingUserIDs", func(t *testing.T) {
		t.Parallel()

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "missing-ids-members-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: querying with no user_ids.
		_, err := adminClient.GroupMembersAISpend(ctx, group.ID, nil)

		// Then: request fails with 400.
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("InclusiveMaxUserIDs", func(t *testing.T) {
		t.Parallel()

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "inclusive-max-user-ids-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: 100 user_ids, exactly at the cap.
		ids := make([]uuid.UUID, 100)
		for i := range ids {
			ids[i] = uuid.New()
		}

		// When: querying spend.
		_, err := adminClient.GroupMembersAISpend(ctx, group.ID, ids)

		// Then: request succeeds.
		require.NoError(t, err)
	})

	t.Run("TooManyUserIDs", func(t *testing.T) {
		t.Parallel()

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "too-many-user-ids-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: 101 user_ids, above the cap of 100.
		ids := make([]uuid.UUID, 101)
		for i := range ids {
			ids[i] = uuid.New()
		}

		// When: querying spend.
		_, err := adminClient.GroupMembersAISpend(ctx, group.ID, ids)

		// Then: request fails with 400.
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("MalformedUserID", func(t *testing.T) {
		t.Parallel()

		adminClient, _, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "malformed-user-id-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: a malformed UUID passed via raw HTTP.
		res, err := adminClient.Request(ctx, http.MethodGet,
			"/api/v2/groups/"+group.ID.String()+"/members/ai/spend",
			nil,
			func(r *http.Request) {
				q := r.URL.Query()
				q.Set("user_ids", "not-a-uuid")
				r.URL.RawQuery = q.Encode()
			},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		// Then: 400.
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("UserInOtherOrgExcluded", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
			GroupName: "primary-org-members-group",
			Database:  db,
			Pubsub:    ps,
		})
		otherOrg := dbgen.Organization(t, db, database.Organization{})
		otherOrgUser := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: otherOrgUser.ID, OrganizationID: otherOrg.ID})
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: querying the group with a user from the primary org and one from another org.
		resp, err := adminClient.GroupMembersAISpend(ctx, group.ID, []uuid.UUID{targetUser.ID, otherOrgUser.ID})
		require.NoError(t, err)

		// Then: only the primary-org user is returned.
		require.Len(t, resp.Members, 1)
		require.Equal(t, targetUser.ID, resp.Members[0].UserID)
		require.Equal(t, &group.OrganizationID, resp.Members[0].EffectiveGroupID)
		require.Nil(t, resp.Members[0].GroupBudget)
		require.Equal(t, int64(0), resp.Members[0].GroupSpendMicros)
	})

	tests := []struct {
		name                  string
		groupLimit            int64
		overrideLimit         int64
		spent                 int64
		wantEffectiveGroup    bool
		wantEffectiveEveryone bool
		wantGroupBudget       *codersdk.AIBudgetLimit
		wantSpendMicros       int64
	}{
		{
			name:               "BudgetZeroSpend",
			groupLimit:         1_000_000_000,
			wantEffectiveGroup: true,
			wantGroupBudget: &codersdk.AIBudgetLimit{
				SpendLimitMicros: 1_000_000_000,
				LimitSource:      codersdk.AIBudgetLimitSourceGroup,
			},
		},
		{
			name:               "BudgetWithSpend",
			groupLimit:         1_000_000_000,
			spent:              250_000_000,
			wantEffectiveGroup: true,
			wantGroupBudget: &codersdk.AIBudgetLimit{
				SpendLimitMicros: 1_000_000_000,
				LimitSource:      codersdk.AIBudgetLimitSourceGroup,
			},
			wantSpendMicros: 250_000_000,
		},
		{
			name:               "OverrideBudget",
			overrideLimit:      500_000_000,
			wantEffectiveGroup: true,
			wantGroupBudget: &codersdk.AIBudgetLimit{
				SpendLimitMicros: 500_000_000,
				LimitSource:      codersdk.AIBudgetLimitSourceUserOverride,
			},
		},
		{
			// With no budget, an in-org member falls back to the Everyone group.
			name:                  "FallbackToEveryoneNoSpend",
			wantEffectiveEveryone: true,
		},
		{
			// The fallback effective group is the Everyone group, while spend is
			// still attributed to the queried group.
			name:                  "FallbackToEveryoneWithSpend",
			spent:                 100_000_000,
			wantEffectiveEveryone: true,
			wantSpendMicros:       100_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Given: an admin, a group with a member, optionally a group budget,
			// a user override, and seeded spend attributed to the group.
			clock := quartz.NewMock(t)
			db, ps := dbtestutil.NewDB(t)
			adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{
				GroupName: "members-spend-test-group",
				Clock:     clock,
				Database:  db,
				Pubsub:    ps,
			})
			ctx := testutil.Context(t, testutil.WaitLong)
			clock.Set(time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC))
			wantPeriodStart := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
			wantPeriodEnd := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)

			if tt.groupLimit > 0 {
				_, err := adminClient.UpsertGroupAIBudget(ctx, group.ID, codersdk.UpsertGroupAIBudgetRequest{
					SpendLimitMicros: tt.groupLimit,
				})
				require.NoError(t, err)
			}
			if tt.overrideLimit > 0 {
				_, err := adminClient.UpsertUserAIBudgetOverride(ctx, targetUser.ID, codersdk.UpsertUserAIBudgetOverrideRequest{
					GroupID:          group.ID,
					SpendLimitMicros: tt.overrideLimit,
				})
				require.NoError(t, err)
			}
			if tt.spent > 0 {
				_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
					UserID:           targetUser.ID,
					EffectiveGroupID: group.ID,
					Day:              clock.Now(),
					CostMicros:       tt.spent,
				})
				require.NoError(t, err)
			}

			// When: querying the group's member spend.
			got, err := adminClient.GroupMembersAISpend(ctx, group.ID, []uuid.UUID{targetUser.ID})
			require.NoError(t, err)

			// Then: one row is returned with the expected fields.
			require.Equal(t, wantPeriodStart, got.PeriodStart)
			require.Equal(t, wantPeriodEnd, got.PeriodEnd)
			require.Len(t, got.Members, 1)
			require.Equal(t, targetUser.ID, got.Members[0].UserID)
			switch {
			case tt.wantEffectiveGroup:
				require.NotNil(t, got.Members[0].EffectiveGroupID)
				require.Equal(t, group.ID, *got.Members[0].EffectiveGroupID)
			case tt.wantEffectiveEveryone:
				require.NotNil(t, got.Members[0].EffectiveGroupID)
				require.Equal(t, group.OrganizationID, *got.Members[0].EffectiveGroupID)
			default:
				require.Nil(t, got.Members[0].EffectiveGroupID)
			}
			require.Equal(t, tt.wantGroupBudget, got.Members[0].GroupBudget)
			require.Equal(t, tt.wantSpendMicros, got.Members[0].GroupSpendMicros)
		})
	}

	t.Run("CrossOrgEffectiveGroupMasked", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		db, ps := dbtestutil.NewDB(t)
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv, Database: db, Pubsub: ps},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC:          1,
					codersdk.FeatureAIBridge:              1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: a member of the queried group whose highest-limit budget
		// group lives in a different org.
		otherOrg := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{})
		_, targetUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: targetUser.ID, OrganizationID: otherOrg.ID})
		queried, err := userAdminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "queried-cross-org-mask-group",
		})
		require.NoError(t, err)
		_, err = userAdminClient.PatchGroup(ctx, queried.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{targetUser.ID.String()},
		})
		require.NoError(t, err)
		crossOrgGroup, err := userAdminClient.CreateGroup(ctx, otherOrg.ID, codersdk.CreateGroupRequest{
			Name: "cross-org-budget-group",
		})
		require.NoError(t, err)
		_, err = userAdminClient.PatchGroup(ctx, crossOrgGroup.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{targetUser.ID.String()},
		})
		require.NoError(t, err)
		_, err = userAdminClient.UpsertGroupAIBudget(ctx, crossOrgGroup.ID, codersdk.UpsertGroupAIBudgetRequest{
			SpendLimitMicros: 9_999_999,
		})
		require.NoError(t, err)

		// When: the owner, who can read both orgs, queries the primary group.
		//nolint:gocritic // The test asserts that even an owner sees the mask.
		resp, err := ownerClient.GroupMembersAISpend(ctx, queried.ID, []uuid.UUID{targetUser.ID})
		require.NoError(t, err)

		// Then: effective_group_id is nil, even though the caller can read both orgs.
		require.Len(t, resp.Members, 1)
		require.Equal(t, targetUser.ID, resp.Members[0].UserID)
		require.Nil(t, resp.Members[0].EffectiveGroupID, "cross-org effective group must be masked even for the owner")
		require.Nil(t, resp.Members[0].GroupBudget)
		require.Equal(t, int64(0), resp.Members[0].GroupSpendMicros)
	})

	t.Run("CrossOrgFallbackEveryoneMasked", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		db, ps := dbtestutil.NewDB(t)
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv, Database: db, Pubsub: ps},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC:          1,
					codersdk.FeatureAIBridge:              1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		userAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
		ctx := testutil.Context(t, testutil.WaitLong)

		// Given: a member of the default org whose queried group lives in a
		// non-default org, with no budget or override. The fallback prefers the
		// default org's Everyone group, which lives in a different org than the
		// queried group.
		queriedOrg := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{})
		_, targetUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: targetUser.ID, OrganizationID: queriedOrg.ID})
		queried, err := userAdminClient.CreateGroup(ctx, queriedOrg.ID, codersdk.CreateGroupRequest{
			Name: "queried-cross-org-fallback-group",
		})
		require.NoError(t, err)
		_, err = userAdminClient.PatchGroup(ctx, queried.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{targetUser.ID.String()},
		})
		require.NoError(t, err)

		// Spend is attributed to the queried group.
		_, err = db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID:           targetUser.ID,
			EffectiveGroupID: queried.ID,
			Day:              dbtime.Now(),
			CostMicros:       100_000_000,
		})
		require.NoError(t, err)

		// When: the owner, who can read both orgs, queries the group.
		//nolint:gocritic // The test asserts that even an owner sees the mask.
		resp, err := ownerClient.GroupMembersAISpend(ctx, queried.ID, []uuid.UUID{targetUser.ID})
		require.NoError(t, err)

		// Then: effective_group_id is masked because the fallback Everyone group
		// lives in the default org, while the queried group's spend still returns.
		require.Len(t, resp.Members, 1)
		require.Equal(t, targetUser.ID, resp.Members[0].UserID)
		require.Nil(t, resp.Members[0].EffectiveGroupID, "cross-org fallback effective group must be masked")
		require.Nil(t, resp.Members[0].GroupBudget)
		require.Equal(t, int64(100_000_000), resp.Members[0].GroupSpendMicros)
	})

	t.Run("OrgScopedRoute", func(t *testing.T) {
		t.Parallel()

		adminClient, targetUser, group := setupAICostControlTest(t, aiCostControlTestOptions{GroupName: "org-scoped-route-group"})
		ctx := testutil.Context(t, testutil.WaitLong)

		// When: hitting the org-scoped alias route via raw HTTP.
		res, err := adminClient.Request(ctx, http.MethodGet,
			"/api/v2/organizations/"+group.OrganizationID.String()+"/groups/"+group.Name+"/members/ai/spend",
			nil,
			func(r *http.Request) {
				q := r.URL.Query()
				q.Set("user_ids", targetUser.ID.String())
				r.URL.RawQuery = q.Encode()
			},
		)
		require.NoError(t, err)
		defer res.Body.Close()

		// Then: 200 with the target user in the response.
		require.Equal(t, http.StatusOK, res.StatusCode)
		var got codersdk.GroupMembersAISpend
		require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
		require.Len(t, got.Members, 1)
		require.Equal(t, targetUser.ID, got.Members[0].UserID)
		require.Equal(t, &group.OrganizationID, got.Members[0].EffectiveGroupID)
		require.Nil(t, got.Members[0].GroupBudget)
		require.Equal(t, int64(0), got.Members[0].GroupSpendMicros)
	})
}

func TestGroupMembersAISpendRoleAccess(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC:          1,
				codersdk.FeatureAIBridge:              1,
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	userAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
	orgUserAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.ScopedRoleOrgUserAdmin(owner.OrganizationID))
	memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	_, otherMember := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	otherOrg := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{})
	otherOrgMemberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, otherOrg.ID)

	ctx := testutil.Context(t, testutil.WaitLong)
	group, err := userAdminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
		Name: "role-access-members-group",
	})
	require.NoError(t, err)
	_, err = userAdminClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
		AddUsers: []string{member.ID.String(), otherMember.ID.String()},
	})
	require.NoError(t, err)

	cases := []struct {
		name       string
		client     *codersdk.Client
		wantMember bool
	}{
		{name: "Owner", client: ownerClient, wantMember: true},
		{name: "UserAdmin", client: userAdminClient, wantMember: true},
		{name: "OrgAdmin", client: orgAdminClient, wantMember: true},
		{name: "OrgUserAdmin", client: orgUserAdminClient, wantMember: true},
		{name: "Member", client: memberClient, wantMember: true},
		{name: "OtherOrgMember", client: otherOrgMemberClient, wantMember: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)

			resp, err := tc.client.GroupMembersAISpend(ctx, group.ID, []uuid.UUID{member.ID})
			if !tc.wantMember {
				var sdkErr *codersdk.Error
				require.ErrorAs(t, err, &sdkErr)
				require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
				return
			}
			require.NoError(t, err)
			require.Len(t, resp.Members, 1)
			require.Equal(t, member.ID, resp.Members[0].UserID)
		})
	}

	t.Run("MemberCanOnlyReadOwnRow", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)

		// When: a member queries spend for themselves and another member.
		resp, err := memberClient.GroupMembersAISpend(ctx, group.ID, []uuid.UUID{member.ID, otherMember.ID})
		require.NoError(t, err)

		// Then: only the caller's own row is returned.
		require.Len(t, resp.Members, 1)
		require.Equal(t, member.ID, resp.Members[0].UserID)
	})
}

// aiCostControlTestOptions configures the setup of an AI cost control test
// deployment. GroupName is required. Clock, Database, and Pubsub are
// optional overrides (leave nil for defaults).
type aiCostControlTestOptions struct {
	GroupName string
	Clock     quartz.Clock
	Database  database.Store
	Pubsub    pubsub.Pubsub
	// Retention overrides the AI Gateway data retention duration. Nil leaves the
	// deployment default in place, and zero disables purging.
	Retention *time.Duration
}

// setupAICostControlTest builds a deployment with FeatureAIBridge licensed,
// creates an admin client and target user, adds the target user to a group,
// and returns the admin client, target user, and group.
func setupAICostControlTest(t *testing.T, opts aiCostControlTestOptions) (*codersdk.Client, codersdk.User, codersdk.Group) {
	t.Helper()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	if opts.Retention != nil {
		dv.AI.BridgeConfig.Retention = serpent.Duration(*opts.Retention)
	}
	coderdOpts := &coderdtest.Options{DeploymentValues: dv}
	if opts.Clock != nil {
		coderdOpts.Clock = opts.Clock
	}
	if opts.Database != nil {
		coderdOpts.Database = opts.Database
	}
	if opts.Pubsub != nil {
		coderdOpts.Pubsub = opts.Pubsub
	}
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: coderdOpts,
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAIBridge:     1,
			},
		},
	})
	adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
	_, targetUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	ctx := testutil.Context(t, testutil.WaitLong)
	g, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
		Name: opts.GroupName,
	})
	require.NoError(t, err)
	g, err = adminClient.PatchGroup(ctx, g.ID, codersdk.PatchGroupRequest{
		AddUsers: []string{targetUser.ID.String()},
	})
	require.NoError(t, err)
	return adminClient, targetUser, g
}

// setupUserAIBudgetOverrideAuditTest builds a deployment wired with the
// enterprise auditor (the mock auditor does not compute diffs) so audit
// entries can be read straight from the audit_logs table.
func setupUserAIBudgetOverrideAuditTest(t *testing.T) (database.Store, *codersdk.Client, codersdk.CreateFirstUserResponse, codersdk.User) {
	t.Helper()

	db, ps := dbtestutil.NewDB(t)
	auditor := entaudit.NewAuditor(
		db,
		entaudit.DefaultFilter,
		backends.NewPostgres(db, true),
	)
	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging: true,
		Options: &coderdtest.Options{
			DeploymentValues: dv,
			Database:         db,
			Pubsub:           ps,
			Auditor:          auditor,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAIBridge:     1,
				codersdk.FeatureAuditLog:     1,
			},
		},
	})
	adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())
	_, targetUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	return db, adminClient, owner, targetUser
}

// setupGroupAIBudgetTest returns an Admin client along with a newly created group inside it.
func setupGroupAIBudgetTest(t *testing.T) (adminClient *codersdk.Client, group codersdk.Group) {
	t.Helper()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
				codersdk.FeatureAIBridge:     1,
			},
		},
	})
	adminClient, _ = coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

	ctx := testutil.Context(t, testutil.WaitLong)
	g, err := adminClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
		Name: "budget-test-group",
	})
	require.NoError(t, err)
	return adminClient, g
}
