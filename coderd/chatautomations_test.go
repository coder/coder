package coderd_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

//nolint:tparallel,paralleltest // Subtests share one deployment and auditor, and delete rows in order.
func TestChatAutomations(t *testing.T) {
	t.Parallel()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentChatAutomations)}
	auditor := audit.NewMock()
	ownerClient := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: values, Database: db, Pubsub: ps, Auditor: auditor, ChatWorkerDisabled: true,
	})
	first := coderdtest.CreateFirstUser(t, ownerClient)
	org := first.OrganizationID
	memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, org)
	_, other := coderdtest.CreateAnotherUser(t, ownerClient, org)
	adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, org, rbac.ScopedRoleOrgAdmin(org))
	asMember := codersdk.NewExperimentalClient(memberClient)
	asAdmin := codersdk.NewExperimentalClient(adminClient)

	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org})
	target := dbgen.Chat(t, db, database.Chat{OrganizationID: org, OwnerID: member.ID, LastModelConfigID: model.ID, Title: "Target"})
	// A daily schedule in a zone with daylight saving time.
	mine := dbgen.ChatAutomation(t, db, database.ChatAutomation{
		OrganizationID: org, OwnerID: member.ID, Enabled: true,
		TargetChatID:     uuid.NullUUID{UUID: target.ID, Valid: true},
		ScheduleCron:     sql.NullString{String: "30 9 * * *", Valid: true},
		ScheduleTimeZone: sql.NullString{String: "America/New_York", Valid: true},
	})
	theirs := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: org, OwnerID: other.ID})
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	// The member can read its row there, so only the path's organization
	// hides it.
	dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: otherOrg.ID, UserID: member.ID})
	elsewhere := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: otherOrg.ID, OwnerID: member.ID})
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := sqlDB.ExecContext(ctx, `UPDATE chat_automations SET last_run_at = now(), last_run_outcome = 'skipped',
		last_run_reason = 'chat_busy', last_run_chat_id = $1 WHERE id = $2`, target.ID, mine.ID)
	require.NoError(t, err)

	listIDs := func(t *testing.T, client *codersdk.ExperimentalClient, owner string) []uuid.UUID {
		t.Helper()
		list, err := client.ChatAutomations(ctx, org, owner)
		require.NoError(t, err)
		out := []uuid.UUID{}
		for _, a := range list {
			out = append(out, a.ID)
		}
		return out
	}
	status := func(t *testing.T, err error) int {
		t.Helper()
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		return sdkErr.StatusCode()
	}

	t.Run("List", func(t *testing.T) {
		// RBAC limits a member to its own rows, and the path limits every
		// caller to one organization.
		require.Equal(t, []uuid.UUID{mine.ID}, listIDs(t, asMember, ""))
		require.ElementsMatch(t, []uuid.UUID{mine.ID, theirs.ID}, listIDs(t, asAdmin, ""))
		for owner, want := range map[string][]uuid.UUID{
			codersdk.Me:        {},
			member.ID.String(): {mine.ID},
			other.Username:     {theirs.ID},
			uuid.Nil.String():  {},
		} {
			require.Equal(t, want, listIDs(t, asAdmin, owner), "owner %q", owner)
		}
	})

	t.Run("Get", func(t *testing.T) {
		got, err := asMember.ChatAutomation(ctx, org, mine.ID)
		require.NoError(t, err)
		require.Equal(t, member.Username, got.Owner.Username)
		require.Equal(t, &codersdk.ChatAutomationChat{ID: target.ID, Title: "Target"}, got.TargetChat)
		require.NotNil(t, got.LastRun)
		require.Equal(t, codersdk.ChatAutomationRunOutcomeSkipped, got.LastRun.Outcome)
		require.Equal(t, "chat_busy", got.LastRun.Reason)
		zone, err := time.LoadLocation("America/New_York")
		require.NoError(t, err)
		require.Len(t, got.NextRunTimes, 5)
		for i, next := range got.NextRunTimes {
			local := next.In(zone)
			require.Equal(t, "09:30", local.Format("15:04"), "run %d", i)
			if i > 0 {
				prev := got.NextRunTimes[i-1].In(zone)
				require.Equal(t, prev.AddDate(0, 0, 1).Format(time.DateOnly), local.Format(time.DateOnly), "run %d", i)
			}
		}

		_, err = asMember.ChatAutomation(ctx, org, theirs.ID)
		require.Equal(t, http.StatusNotFound, status(t, err))
		_, err = asMember.ChatAutomation(ctx, org, elsewhere.ID)
		require.Equal(t, http.StatusNotFound, status(t, err))
	})

	t.Run("Delete", func(t *testing.T) {
		require.Equal(t, http.StatusNotFound, status(t, asMember.DeleteChatAutomation(ctx, org, theirs.ID)))
		require.Equal(t, http.StatusNotFound, status(t, asMember.DeleteChatAutomation(ctx, org, elsewhere.ID)))
		auditor.ResetLogs()
		require.NoError(t, asMember.DeleteChatAutomation(ctx, org, mine.ID))
		_, err := asMember.ChatAutomation(ctx, org, mine.ID)
		require.Equal(t, http.StatusNotFound, status(t, err))
		logs := auditor.AuditLogs()
		require.Len(t, logs, 1)
		require.Equal(t, database.AuditActionDelete, logs[0].Action)
		require.Equal(t, database.ResourceTypeChatAutomation, logs[0].ResourceType)
		require.Equal(t, mine.ID, logs[0].ResourceID)
		require.Equal(t, []uuid.UUID{theirs.ID}, listIDs(t, asAdmin, ""))
		_, err = db.GetChatAutomationByID(ctx, elsewhere.ID)
		require.NoError(t, err)
	})

	t.Run("ExperimentOff", func(t *testing.T) {
		kept := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: org, OwnerID: member.ID})
		_, err := codersdk.NewExperimentalClient(ownerClient).PutExperimentRule(ctx, codersdk.ExperimentChatAutomations, codersdk.PutExperimentRuleRequest{
			Mode:      codersdk.ExperimentRuleModeCondition,
			Condition: fmt.Sprintf("user.username != %q", member.Username),
		})
		require.NoError(t, err)
		_, err = asMember.ChatAutomations(ctx, org, "")
		require.Equal(t, http.StatusNotFound, status(t, err))
		_, err = asMember.ChatAutomation(ctx, org, kept.ID)
		require.Equal(t, http.StatusNotFound, status(t, err))
		require.Equal(t, http.StatusNotFound, status(t, asMember.DeleteChatAutomation(ctx, org, kept.ID)))
		require.ElementsMatch(t, []uuid.UUID{kept.ID, theirs.ID}, listIDs(t, asAdmin, ""))
	})
}

func TestCreateChatAutomation(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentChatAutomations)}
	auditor := audit.NewMock()
	ownerClient := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: values, Database: db, Pubsub: ps, Auditor: auditor, ChatWorkerDisabled: true,
	})
	org := coderdtest.CreateFirstUser(t, ownerClient).OrganizationID
	memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, org)
	cappedClient, capped := coderdtest.CreateAnotherUser(t, ownerClient, org)
	_, other := coderdtest.CreateAnotherUser(t, ownerClient, org)
	adminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, org, rbac.ScopedRoleOrgAdmin(org))
	asMember := codersdk.NewExperimentalClient(memberClient)

	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org})
	chat := func(orgID, ownerID uuid.UUID, parent uuid.NullUUID) uuid.UUID {
		return dbgen.Chat(t, db, database.Chat{OrganizationID: orgID, OwnerID: ownerID, LastModelConfigID: model.ID, ParentChatID: parent}).ID
	}
	own := chat(org, member.ID, uuid.NullUUID{})
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: otherOrg.ID, UserID: member.ID})
	ownElsewhere := chat(otherOrg.ID, member.ID, uuid.NullUUID{})
	subAgent := chat(org, member.ID, uuid.NullUUID{UUID: own, Valid: true})
	theirs := chat(org, other.ID, uuid.NullUUID{})
	cappedChat := chat(org, capped.ID, uuid.NullUUID{})
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org, OwnerID: member.ID})
	otherProject := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: otherOrg.ID, OwnerID: member.ID})
	request := func(target uuid.UUID, spec string) codersdk.CreateChatAutomationRequest {
		return codersdk.CreateChatAutomationRequest{
			Name: "heartbeat", TargetMode: codersdk.ChatAutomationTargetModeExistingChat, TargetChatID: &target,
			ScheduleCron: spec, ScheduleTimeZone: "Europe/Berlin", Prompt: "Check the build.",
		}
	}
	newChat := func(projectID uuid.UUID) codersdk.CreateChatAutomationRequest {
		return codersdk.CreateChatAutomationRequest{
			Name: "report", TargetMode: codersdk.ChatAutomationTargetModeNewChat, NewChatModelConfigID: &model.ID, ProjectID: &projectID,
			ScheduleCron: "0 9 * * 1-5", ScheduleTimeZone: "UTC", Prompt: "Write the report.",
		}
	}
	status := func(t *testing.T, err error) *codersdk.Error {
		t.Helper()
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		return sdkErr
	}
	ctx := testutil.Context(t, testutil.WaitLong)

	t.Run("Created", func(t *testing.T) {
		t.Parallel()
		got, err := asMember.CreateChatAutomation(ctx, org, request(own, "*/5 * * * *"))
		require.NoError(t, err)
		require.True(t, got.Enabled)
		require.Equal(t, member.ID, got.OwnerID)
		require.Len(t, got.NextRunTimes, 5)
		require.WithinDuration(t, got.NextRunTimes[0], *got.ScheduleNextRunAt, 0)
		for i := 1; i < len(got.NextRunTimes); i++ {
			require.Equal(t, 5*time.Minute, got.NextRunTimes[i].Sub(got.NextRunTimes[i-1]))
		}
		created := 0
		for _, l := range auditor.AuditLogs() {
			if l.ResourceID == got.ID && l.Action == database.AuditActionCreate {
				created++
			}
		}
		require.Equal(t, 1, created)

		got, err = asMember.CreateChatAutomation(ctx, org, newChat(project.ID))
		require.NoError(t, err)
		require.Equal(t, &model.ID, got.NewChatModelConfigID)
		require.Equal(t, &project.ID, got.ProjectID)
	})

	t.Run("Rejected", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name   string
			client *codersdk.Client
			req    codersdk.CreateChatAutomationRequest
			status int
			field  string
		}{
			{"EveryMinute", memberClient, request(own, "* * * * *"), http.StatusBadRequest, "schedule_cron"},
			{"ChatInOtherOrganization", memberClient, request(ownElsewhere, "0 9 * * *"), http.StatusBadRequest, "target_chat_id"},
			{"SubAgentChat", memberClient, request(subAgent, "0 9 * * *"), http.StatusBadRequest, "target_chat_id"},
			{"ProjectInOtherOrganization", memberClient, newChat(otherProject.ID), http.StatusBadRequest, "project_id"},
			{"OtherMembersChat", memberClient, request(theirs, "0 9 * * *"), http.StatusForbidden, ""},
			// An org admin can update the chat, but a run would send into it
			// as the admin.
			{"AdminTargetsMembersChat", adminClient, request(own, "0 9 * * *"), http.StatusForbidden, ""},
		} {
			_, err := codersdk.NewExperimentalClient(tc.client).CreateChatAutomation(ctx, org, tc.req)
			sdkErr := status(t, err)
			require.Equal(t, tc.status, sdkErr.StatusCode(), tc.name)
			if tc.field != "" {
				require.Len(t, sdkErr.Validations, 1, tc.name)
				require.Equal(t, tc.field, sdkErr.Validations[0].Field, tc.name)
			}
		}
	})

	t.Run("Limit", func(t *testing.T) {
		t.Parallel()
		// The limit counts every organization of the owner.
		dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: otherOrg.ID, OwnerID: capped.ID})
		for range chatautomation.MaxAutomationsPerOwner - 2 {
			dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: org, OwnerID: capped.ID})
		}
		asCapped := codersdk.NewExperimentalClient(cappedClient)
		_, err := asCapped.CreateChatAutomation(ctx, org, request(cappedChat, "0 9 * * *"))
		require.NoError(t, err)
		_, err = asCapped.CreateChatAutomation(ctx, org, request(cappedChat, "0 9 * * *"))
		require.Equal(t, http.StatusBadRequest, status(t, err).StatusCode())
	})
}
