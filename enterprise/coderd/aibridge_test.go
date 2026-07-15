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

	aiblib "github.com/coder/coder/v2/aibridge"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	entaudit "github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
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
	dv.Experiments = []string{string(codersdk.ExperimentAIGatewayCostControl)}
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
	dv.Experiments = []string{string(codersdk.ExperimentAIGatewayCostControl)}
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
		dv.Experiments = []string{string(codersdk.ExperimentAIGatewayCostControl)}
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

	t.Run("RequiresExperiment", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAIBridge: 1,
				},
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
		wantSpendLimitMicros   *int64
		wantLimitSource        *codersdk.AIBudgetLimitSource
		wantCurrentSpendMicros int64
	}{
		{
			name:                   "NoEffectiveGroup",
			wantHasEffectiveGroup:  false,
			wantSpendLimitMicros:   nil,
			wantLimitSource:        nil,
			wantCurrentSpendMicros: 0,
		},
		{
			name:                  "GroupBudget/ZeroSpend",
			groupBudget:           ptr.Ref(int64(1_000_000_000)),
			wantHasEffectiveGroup: true,
			wantSpendLimitMicros:  ptr.Ref(int64(1_000_000_000)),
			wantLimitSource:       ptr.Ref(codersdk.AIBudgetLimitSourceGroup),
		},
		{
			name:                   "GroupBudget/PartialSpend",
			groupBudget:            ptr.Ref(int64(1_000_000_000)),
			spent:                  250_000_000,
			wantHasEffectiveGroup:  true,
			wantSpendLimitMicros:   ptr.Ref(int64(1_000_000_000)),
			wantLimitSource:        ptr.Ref(codersdk.AIBudgetLimitSourceGroup),
			wantCurrentSpendMicros: 250_000_000,
		},
		{
			name:                   "GroupBudget/SpendExceedsLimit",
			groupBudget:            ptr.Ref(int64(1_000_000_000)),
			spent:                  1_500_000_000,
			wantHasEffectiveGroup:  true,
			wantSpendLimitMicros:   ptr.Ref(int64(1_000_000_000)),
			wantLimitSource:        ptr.Ref(codersdk.AIBudgetLimitSourceGroup),
			wantCurrentSpendMicros: 1_500_000_000,
		},
		{
			name:                  "UserOverride/ZeroSpend",
			groupBudget:           ptr.Ref(int64(5_000_000_000)),
			overrideLimit:         ptr.Ref(int64(200_000_000)),
			wantHasEffectiveGroup: true,
			wantSpendLimitMicros:  ptr.Ref(int64(200_000_000)),
			wantLimitSource:       ptr.Ref(codersdk.AIBudgetLimitSourceUserOverride),
		},
		{
			name:                   "UserOverride/PartialSpend",
			groupBudget:            ptr.Ref(int64(5_000_000_000)),
			overrideLimit:          ptr.Ref(int64(200_000_000)),
			spent:                  50_000_000,
			wantHasEffectiveGroup:  true,
			wantSpendLimitMicros:   ptr.Ref(int64(200_000_000)),
			wantLimitSource:        ptr.Ref(codersdk.AIBudgetLimitSourceUserOverride),
			wantCurrentSpendMicros: 50_000_000,
		},
		{
			name:                   "UserOverride/SpendExceedsLimit",
			groupBudget:            ptr.Ref(int64(5_000_000_000)),
			overrideLimit:          ptr.Ref(int64(200_000_000)),
			spent:                  350_000_000,
			wantHasEffectiveGroup:  true,
			wantSpendLimitMicros:   ptr.Ref(int64(200_000_000)),
			wantLimitSource:        ptr.Ref(codersdk.AIBudgetLimitSourceUserOverride),
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
			require.Equal(t, tt.wantSpendLimitMicros, got.SpendLimitMicros)
			require.Equal(t, tt.wantLimitSource, got.LimitSource)
		})
	}
}

func TestUserAISpendStatusRoleAccess(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	dv.Experiments = []string{string(codersdk.ExperimentAIGatewayCostControl)}
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
		dv.Experiments = []string{string(codersdk.ExperimentAIGatewayCostControl)}
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

	t.Run("RequiresExperiment", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{DeploymentValues: dv},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureTemplateRBAC: 1,
					codersdk.FeatureAIBridge:     1,
				},
			},
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Owner role is irrelevant here; the request is blocked before RBAC.
		_, err := client.OrganizationGroupsAISpend(ctx, owner.OrganizationID, []uuid.UUID{uuid.New()})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "ai-gateway-cost-control")
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

	tests := []struct {
		name             string
		setBudget        bool
		spendLimit       int64
		spent            int64
		wantSpendLimit   *int64
		wantCurrentSpend int64
	}{
		{
			name: "NoBudgetNoSpend",
		},
		{
			name:             "ZeroLimitBudget",
			setBudget:        true,
			spendLimit:       0,
			wantSpendLimit:   ptr.Ref(int64(0)),
			wantCurrentSpend: 0,
		},
		{
			name:             "BudgetZeroSpend",
			setBudget:        true,
			spendLimit:       1_000_000_000,
			wantSpendLimit:   ptr.Ref(int64(1_000_000_000)),
			wantCurrentSpend: 0,
		},
		{
			name:             "BudgetWithSpend",
			setBudget:        true,
			spendLimit:       1_000_000_000,
			spent:            250_000_000,
			wantSpendLimit:   ptr.Ref(int64(1_000_000_000)),
			wantCurrentSpend: 250_000_000,
		},
		{
			name:             "SpendExceedsLimit",
			setBudget:        true,
			spendLimit:       1_000_000_000,
			spent:            1_500_000_000,
			wantSpendLimit:   ptr.Ref(int64(1_000_000_000)),
			wantCurrentSpend: 1_500_000_000,
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
			require.Equal(t, tt.wantCurrentSpend, got.Groups[0].CurrentSpendMicros)
		})
	}
}

func TestOrganizationGroupsAISpendRoleAccess(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	dv.Experiments = []string{string(codersdk.ExperimentAIGatewayCostControl)}
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

// aiCostControlTestOptions configures the setup of an AI cost control test
// deployment. GroupName is required. Clock, Database, and Pubsub are
// optional overrides (leave nil for defaults).
type aiCostControlTestOptions struct {
	GroupName string
	Clock     quartz.Clock
	Database  database.Store
	Pubsub    pubsub.Pubsub
}

// setupAICostControlTest builds a deployment with FeatureAIBridge licensed
// and the AI Gateway cost control experiment enabled, creates an admin
// client and target user, adds the target user to a group, and returns
// the admin client, target user, and group.
func setupAICostControlTest(t *testing.T, opts aiCostControlTestOptions) (*codersdk.Client, codersdk.User, codersdk.Group) {
	t.Helper()

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	dv.Experiments = []string{string(codersdk.ExperimentAIGatewayCostControl)}
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
	dv.Experiments = []string{string(codersdk.ExperimentAIGatewayCostControl)}
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
