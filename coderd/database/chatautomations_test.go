package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

type chatAutomationOrg struct {
	org     database.Organization
	user    database.User
	model   database.ChatModelConfig
	chat    database.Chat
	project database.ChatProject
}

func newChatAutomationOrg(t *testing.T, db database.Store, user database.User) chatAutomationOrg {
	t.Helper()
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID})
	return chatAutomationOrg{
		org:   org,
		user:  user,
		model: model,
		chat: dbgen.Chat(t, db, database.Chat{
			OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: model.ID,
		}),
		project: dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID}),
	}
}

func (o chatAutomationOrg) newChat(t *testing.T, db database.Store) database.Chat {
	t.Helper()
	return dbgen.Chat(t, db, database.Chat{
		OrganizationID: o.org.ID, OwnerID: o.user.ID, LastModelConfigID: o.model.ID,
	})
}

func (o chatAutomationOrg) webhookParams() database.InsertChatAutomationParams {
	return database.InsertChatAutomationParams{
		OrganizationID: o.org.ID,
		OwnerID:        o.user.ID,
		Name:           "webhook",
		Kind:           database.ChatAutomationKindWebhook,
		Enabled:        true,
		Prompt:         "Investigate the event.",
		TargetMode:     database.ChatAutomationTargetModeExistingChat,
		TargetChatID:   uuid.NullUUID{UUID: o.chat.ID, Valid: true},
		WhenBusy:       database.NullChatAutomationWhenBusy{ChatAutomationWhenBusy: database.ChatAutomationWhenBusyQueue, Valid: true},
		WebhookAuth:    database.NullChatAutomationWebhookAuth{ChatAutomationWebhookAuth: database.ChatAutomationWebhookAuthBearer, Valid: true},
		WebhookSecret:  sql.NullString{String: "webhook-secret", Valid: true},
	}
}

func (o chatAutomationOrg) scheduleParams() database.InsertChatAutomationParams {
	return database.InsertChatAutomationParams{
		OrganizationID:       o.org.ID,
		OwnerID:              o.user.ID,
		Name:                 "schedule",
		Kind:                 database.ChatAutomationKindSchedule,
		Enabled:              true,
		Prompt:               "Post the daily report.",
		TargetMode:           database.ChatAutomationTargetModeNewChat,
		NewChatModelConfigID: uuid.NullUUID{UUID: o.model.ID, Valid: true},
		ScheduleCron:         sql.NullString{String: "0 9 * * *", Valid: true},
		ScheduleTimeZone:     sql.NullString{String: "UTC", Valid: true},
	}
}

func nullUUID(id uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id, Valid: true}
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

// requireCheckViolation asserts that err is a check violation that names
// the constraint. The organization trigger raises its own constraint names,
// which are not CHECK constraints of the table.
func requireCheckViolation(t *testing.T, err error, constraint string) {
	t.Helper()
	var pqErr *pq.Error
	require.ErrorAs(t, err, &pqErr)
	require.Equal(t, "check_violation", pqErr.Code.Name(), pqErr.Message)
	require.Equal(t, constraint, pqErr.Constraint, pqErr.Message)
}

func TestChatAutomationShapeChecks(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	o := newChatAutomationOrg(t, db, dbgen.User(t, db, database.User{}))

	whenBusy := database.NullChatAutomationWhenBusy{ChatAutomationWhenBusy: database.ChatAutomationWhenBusySkip, Valid: true}
	webhookAuth := database.NullChatAutomationWebhookAuth{ChatAutomationWebhookAuth: database.ChatAutomationWebhookAuthGithub, Valid: true}

	cases := []struct {
		name   string
		base   func() database.InsertChatAutomationParams
		mutate func(p *database.InsertChatAutomationParams)
		want   database.CheckConstraint
	}{
		{name: "Webhook", base: o.webhookParams},
		{name: "WebhookWithoutAuth", base: o.webhookParams, want: database.CheckChatAutomationsKindShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.WebhookAuth = database.NullChatAutomationWebhookAuth{}
		}},
		{name: "WebhookWithoutSecret", base: o.webhookParams, want: database.CheckChatAutomationsKindShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.WebhookSecret = sql.NullString{}
		}},
		// The empty string stays legal so that a secret can be blanked.
		{name: "WebhookWithEmptySecret", base: o.webhookParams, mutate: func(p *database.InsertChatAutomationParams) {
			p.WebhookSecret = nullString("")
		}},
		{name: "WebhookWithCron", base: o.webhookParams, want: database.CheckChatAutomationsKindShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.ScheduleCron = nullString("0 9 * * *")
		}},
		{name: "WebhookWithTimeZone", base: o.webhookParams, want: database.CheckChatAutomationsKindShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.ScheduleTimeZone = nullString("UTC")
		}},
		{name: "WebhookWithNextRun", base: o.webhookParams, want: database.CheckChatAutomationsKindShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.ScheduleNextRunAt = sql.NullTime{Time: time.Now(), Valid: true}
		}},
		{name: "Schedule", base: o.scheduleParams},
		{name: "ScheduleWithNextRun", base: o.scheduleParams, mutate: func(p *database.InsertChatAutomationParams) {
			p.ScheduleNextRunAt = sql.NullTime{Time: time.Now(), Valid: true}
		}},
		{name: "ScheduleWithoutCron", base: o.scheduleParams, want: database.CheckChatAutomationsKindShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.ScheduleCron = sql.NullString{}
		}},
		{name: "ScheduleWithoutTimeZone", base: o.scheduleParams, want: database.CheckChatAutomationsKindShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.ScheduleTimeZone = sql.NullString{}
		}},
		{name: "ScheduleWithWebhookAuth", base: o.scheduleParams, want: database.CheckChatAutomationsKindShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.WebhookAuth = webhookAuth
		}},
		{name: "ScheduleWithSecret", base: o.scheduleParams, want: database.CheckChatAutomationsKindShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.WebhookSecret = nullString("secret")
		}},
		{name: "ScheduleWithSecretKeyID", base: o.scheduleParams, want: database.CheckChatAutomationsKindShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.WebhookSecretKeyID = nullString("digest")
		}},
		// Deleting the target chat sets target_chat_id to NULL, so the
		// row must be legal without it.
		{name: "ExistingChatWithoutTarget", base: o.webhookParams, mutate: func(p *database.InsertChatAutomationParams) {
			p.TargetChatID = uuid.NullUUID{}
		}},
		{name: "ExistingChatWithoutWhenBusy", base: o.webhookParams, want: database.CheckChatAutomationsTargetShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.WhenBusy = database.NullChatAutomationWhenBusy{}
		}},
		{name: "ExistingChatWithModel", base: o.webhookParams, want: database.CheckChatAutomationsTargetShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.NewChatModelConfigID = nullUUID(o.model.ID)
		}},
		{name: "ExistingChatWithProject", base: o.webhookParams, want: database.CheckChatAutomationsProjectTargetMode, mutate: func(p *database.InsertChatAutomationParams) {
			p.ProjectID = nullUUID(o.project.ID)
		}},
		{name: "NewChatWithProject", base: o.scheduleParams, mutate: func(p *database.InsertChatAutomationParams) {
			p.ProjectID = nullUUID(o.project.ID)
		}},
		{name: "NewChatWithTarget", base: o.scheduleParams, want: database.CheckChatAutomationsTargetShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.TargetChatID = nullUUID(o.chat.ID)
		}},
		{name: "NewChatWithWhenBusy", base: o.scheduleParams, want: database.CheckChatAutomationsTargetShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.WhenBusy = whenBusy
		}},
		{name: "NewChatWithoutModel", base: o.scheduleParams, want: database.CheckChatAutomationsTargetShape, mutate: func(p *database.InsertChatAutomationParams) {
			p.NewChatModelConfigID = uuid.NullUUID{}
		}},
		{name: "EmptyName", base: o.webhookParams, want: database.CheckChatAutomationsNameCheck, mutate: func(p *database.InsertChatAutomationParams) {
			p.Name = ""
		}},
		{name: "LongestName", base: o.webhookParams, mutate: func(p *database.InsertChatAutomationParams) {
			p.Name = strings.Repeat("a", 128)
		}},
		{name: "TooLongName", base: o.webhookParams, want: database.CheckChatAutomationsNameCheck, mutate: func(p *database.InsertChatAutomationParams) {
			p.Name = strings.Repeat("a", 129)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			params := tc.base()
			if tc.mutate != nil {
				tc.mutate(&params)
			}
			automation, err := db.InsertChatAutomation(ctx, params)
			if tc.want != "" {
				require.True(t, database.IsCheckViolation(err, tc.want), "want %s, got %v", tc.want, err)
				return
			}
			require.NoError(t, err)

			require.NotEqual(t, uuid.Nil, automation.ID)
			require.Equal(t, params.Enabled, automation.Enabled)
			require.EqualValues(t, 0, automation.WebhookSecretVersion)
			require.EqualValues(t, 1, automation.ScheduleRevision)
			require.EqualValues(t, 0, automation.LastRunUnacceptedCount)
			require.False(t, automation.LastRunAt.Valid)
			require.False(t, automation.LastRunOutcome.Valid)
			require.False(t, automation.LastRunReason.Valid)
			require.False(t, automation.LastRunChatID.Valid)
			require.False(t, automation.CreatedAt.IsZero())
			require.Equal(t, automation.CreatedAt, automation.UpdatedAt)
		})
	}
}

func TestChatAutomationLastRunChecks(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	o := newChatAutomationOrg(t, db, dbgen.User(t, db, database.User{}))
	automation := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: o.org.ID, OwnerID: o.user.ID})
	ctx := testutil.Context(t, testutil.WaitLong)

	for _, tc := range []struct {
		name  string
		query string
		want  database.CheckConstraint
	}{
		{name: "TimeWithoutOutcome", query: `UPDATE chat_automations SET last_run_at = now() WHERE id = $1`, want: database.CheckChatAutomationsLastRunShape},
		{name: "OutcomeWithoutTime", query: `UPDATE chat_automations SET last_run_outcome = 'skipped' WHERE id = $1`, want: database.CheckChatAutomationsLastRunShape},
		{name: "NegativeCount", query: `UPDATE chat_automations SET last_run_unaccepted_count = -1 WHERE id = $1`, want: database.CheckChatAutomationsLastRunUnacceptedCountCheck},
		// last_run_chat_id has no foreign key, so any chat ID is accepted.
		{name: "SkippedRun", query: `UPDATE chat_automations SET last_run_at = now(), last_run_outcome = 'skipped', last_run_reason = 'chat_busy',
			last_run_chat_id = gen_random_uuid(), last_run_unaccepted_count = 3 WHERE id = $1`},
		{name: "AcceptedRun", query: `UPDATE chat_automations SET last_run_at = now(), last_run_outcome = 'accepted', last_run_reason = NULL,
			last_run_unaccepted_count = 0 WHERE id = $1`},
	} {
		_, err := sqlDB.ExecContext(ctx, tc.query, automation.ID)
		if tc.want != "" {
			require.True(t, database.IsCheckViolation(err, tc.want), "%s: want %s, got %v", tc.name, tc.want, err)
			continue
		}
		require.NoError(t, err, tc.name)
	}
}

func TestChatAutomationOrganizationTrigger(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	user := dbgen.User(t, db, database.User{})
	o := newChatAutomationOrg(t, db, user)
	other := newChatAutomationOrg(t, db, user)
	ctx := testutil.Context(t, testutil.WaitLong)

	const (
		chatConstraint    = "chat_automations_chat_organization"
		projectConstraint = "chat_automations_project_organization"
	)

	t.Run("Insert", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		params := o.webhookParams()
		params.TargetChatID = nullUUID(other.chat.ID)
		_, err := db.InsertChatAutomation(ctx, params)
		requireCheckViolation(t, err, chatConstraint)

		params = o.webhookParams()
		params.CreatedByChatID = nullUUID(other.chat.ID)
		_, err = db.InsertChatAutomation(ctx, params)
		requireCheckViolation(t, err, chatConstraint)

		params = o.scheduleParams()
		params.ProjectID = nullUUID(other.project.ID)
		_, err = db.InsertChatAutomation(ctx, params)
		requireCheckViolation(t, err, projectConstraint)

		params = o.webhookParams()
		params.CreatedByChatID = nullUUID(o.chat.ID)
		_, err = db.InsertChatAutomation(ctx, params)
		require.NoError(t, err, "references in the same organization")
	})

	// Sequential: each update below must leave the row unchanged.
	webhook, err := db.InsertChatAutomation(ctx, o.webhookParams())
	require.NoError(t, err)
	schedule, err := db.InsertChatAutomation(ctx, o.scheduleParams())
	require.NoError(t, err)
	for _, tc := range []struct {
		name  string
		id    uuid.UUID
		query string
		arg   uuid.UUID
		want  string
	}{
		{name: "TargetChat", id: webhook.ID, query: `UPDATE chat_automations SET target_chat_id = $2 WHERE id = $1`, arg: other.chat.ID, want: chatConstraint},
		{name: "CreatingChat", id: webhook.ID, query: `UPDATE chat_automations SET created_by_chat_id = $2 WHERE id = $1`, arg: other.chat.ID, want: chatConstraint},
		{name: "Project", id: schedule.ID, query: `UPDATE chat_automations SET project_id = $2 WHERE id = $1`, arg: other.project.ID, want: projectConstraint},
		// A change of the organization alone checks every reference again.
		{name: "OrganizationOfTargetChat", id: webhook.ID, query: `UPDATE chat_automations SET organization_id = $2 WHERE id = $1`, arg: other.org.ID, want: chatConstraint},
	} {
		_, err := sqlDB.ExecContext(ctx, tc.query, tc.id, tc.arg)
		requireCheckViolation(t, err, tc.want)
	}

	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_automations SET project_id = $2 WHERE id = $1`, schedule.ID, o.project.ID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_automations SET organization_id = $2, new_chat_model_config_id = $3 WHERE id = $1`,
		schedule.ID, other.org.ID, other.model.ID)
	requireCheckViolation(t, err, projectConstraint)

	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_automations SET project_id = NULL, created_by_chat_id = $2 WHERE id = $1`, schedule.ID, o.chat.ID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_automations SET organization_id = $2, new_chat_model_config_id = $3 WHERE id = $1`,
		schedule.ID, other.org.ID, other.model.ID)
	requireCheckViolation(t, err, chatConstraint)

	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_automations SET organization_id = $2, target_chat_id = $3 WHERE id = $1`,
		webhook.ID, other.org.ID, other.chat.ID)
	require.NoError(t, err)
}

func TestChatAutomationReferenceDeletes(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	o := newChatAutomationOrg(t, db, dbgen.User(t, db, database.User{}))
	ctx := testutil.Context(t, testutil.WaitLong)

	deleteChat := func(id uuid.UUID) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, `DELETE FROM chats WHERE id = $1`, id)
		require.NoError(t, err)
	}
	get := func(id uuid.UUID) database.ChatAutomation {
		t.Helper()
		automation, err := db.GetChatAutomationByID(ctx, id)
		require.NoError(t, err)
		return automation
	}

	target, creator, both := o.newChat(t, db), o.newChat(t, db), o.newChat(t, db)

	params := o.webhookParams()
	params.TargetChatID = nullUUID(target.ID)
	params.CreatedByChatID = nullUUID(creator.ID)
	separate, err := db.InsertChatAutomation(ctx, params)
	require.NoError(t, err)

	// An agent that creates a heartbeat for its own chat sets both
	// references to one chat.
	params = o.webhookParams()
	params.TargetChatID = nullUUID(both.ID)
	params.CreatedByChatID = nullUUID(both.ID)
	heartbeat, err := db.InsertChatAutomation(ctx, params)
	require.NoError(t, err)

	params = o.scheduleParams()
	params.ProjectID = nullUUID(o.project.ID)
	withProject, err := db.InsertChatAutomation(ctx, params)
	require.NoError(t, err)

	deleteChat(target.ID)
	got := get(separate.ID)
	require.False(t, got.TargetChatID.Valid)
	require.Equal(t, nullUUID(creator.ID), got.CreatedByChatID)

	deleteChat(creator.ID)
	got = get(separate.ID)
	require.False(t, got.TargetChatID.Valid)
	require.False(t, got.CreatedByChatID.Valid)

	deleteChat(both.ID)
	got = get(heartbeat.ID)
	require.False(t, got.TargetChatID.Valid)
	require.False(t, got.CreatedByChatID.Valid)

	require.NoError(t, db.DeleteChatProjectByID(ctx, o.project.ID))
	got = get(withProject.ID)
	require.False(t, got.ProjectID.Valid)
	require.Equal(t, database.ChatAutomationTargetModeNewChat, got.TargetMode)

	// DeleteOldChats deletes many chats in one statement. Each deleted chat
	// runs its own SET NULL update, and the row's other reference then
	// still points at a chat that the statement already deleted. The IDs
	// and the insert order fix which chat PostgreSQL deletes first.
	for _, targetFirst := range []bool{true, false} {
		firstID, secondID := uuid.New(), uuid.New()
		firstID[0], secondID[0] = 0x00, 0xff
		first := dbgen.Chat(t, db, database.Chat{
			ID: firstID, OrganizationID: o.org.ID, OwnerID: o.user.ID, LastModelConfigID: o.model.ID,
		})
		second := dbgen.Chat(t, db, database.Chat{
			ID: secondID, OrganizationID: o.org.ID, OwnerID: o.user.ID, LastModelConfigID: o.model.ID,
		})
		target, creator := first, second
		if !targetFirst {
			target, creator = second, first
		}

		params := o.webhookParams()
		params.TargetChatID = nullUUID(target.ID)
		params.CreatedByChatID = nullUUID(creator.ID)
		automation, err := db.InsertChatAutomation(ctx, params)
		require.NoError(t, err)

		_, err = sqlDB.ExecContext(ctx, `DELETE FROM chats WHERE id IN ($1, $2)`, target.ID, creator.ID)
		require.NoError(t, err, "targetFirst=%t", targetFirst)
		got = get(automation.ID)
		require.False(t, got.TargetChatID.Valid, "targetFirst=%t", targetFirst)
		require.False(t, got.CreatedByChatID.Valid, "targetFirst=%t", targetFirst)
	}
}

func TestChatAutomationCascadesAndProvenance(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	requireGone := func(ctx context.Context, t *testing.T, id uuid.UUID) {
		t.Helper()
		_, err := db.GetChatAutomationByID(ctx, id)
		require.ErrorIs(t, err, sql.ErrNoRows)
	}

	t.Run("Owner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		org := dbgen.Organization(t, db, database.Organization{})
		owner := dbgen.User(t, db, database.User{})
		automation := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: org.ID, OwnerID: owner.ID})
		// A hard delete needs the status history gone first.
		_, err := sqlDB.ExecContext(ctx, `DELETE FROM user_status_changes WHERE user_id = $1`, owner.ID)
		require.NoError(t, err)
		_, err = sqlDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, owner.ID)
		require.NoError(t, err)
		requireGone(ctx, t, automation.ID)
	})

	// Coder soft-deletes users, so the foreign key never cascades. The
	// user cleanup trigger deletes the automations instead.
	t.Run("SoftDeletedOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		org := dbgen.Organization(t, db, database.Organization{})
		owner := dbgen.User(t, db, database.User{})
		other := dbgen.User(t, db, database.User{})
		automation := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: org.ID, OwnerID: owner.ID})
		kept := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: org.ID, OwnerID: other.ID})
		require.NoError(t, db.UpdateUserDeletedByID(ctx, owner.ID))
		requireGone(ctx, t, automation.ID)
		_, err := db.GetChatAutomationByID(ctx, kept.ID)
		require.NoError(t, err, "the automation of another user stays")
	})

	t.Run("Organization", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		org := dbgen.Organization(t, db, database.Organization{})
		owner := dbgen.User(t, db, database.User{})
		automation := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: org.ID, OwnerID: owner.ID})
		_, err := sqlDB.ExecContext(ctx, `DELETE FROM organizations WHERE id = $1`, org.ID)
		require.NoError(t, err)
		requireGone(ctx, t, automation.ID)
	})

	// A queued message names its automation and its input together.
	t.Run("QueuedMessageProvenancePair", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		o := newChatAutomationOrg(t, db, dbgen.User(t, db, database.User{}))
		for _, tc := range []struct {
			name         string
			automationID uuid.NullUUID
			inputID      uuid.NullUUID
			wantErr      bool
		}{
			{name: "neither"},
			{name: "both", automationID: nullUUID(uuid.New()), inputID: nullUUID(uuid.New())},
			{name: "automation only", automationID: nullUUID(uuid.New()), wantErr: true},
			{name: "input only", inputID: nullUUID(uuid.New()), wantErr: true},
		} {
			_, err := sqlDB.ExecContext(ctx, `
				INSERT INTO chat_queued_messages (chat_id, content, created_by, automation_id, input_id)
				SELECT id, '[]', owner_id, $2::uuid, $3::uuid FROM chats WHERE id = $1`,
				o.chat.ID, tc.automationID, tc.inputID)
			if tc.wantErr {
				requireCheckViolation(t, err, "chat_queued_messages_automation_input_check")
				continue
			}
			require.NoError(t, err, tc.name)
		}
	})

	t.Run("Provenance", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		o := newChatAutomationOrg(t, db, dbgen.User(t, db, database.User{}))
		automation := dbgen.ChatAutomation(t, db, database.ChatAutomation{
			OrganizationID: o.org.ID, OwnerID: o.user.ID, TargetChatID: nullUUID(o.chat.ID),
		})
		inputID := uuid.New()
		_, err := sqlDB.ExecContext(ctx, `UPDATE chats SET automation_id = $2 WHERE id = $1`, o.chat.ID, automation.ID)
		require.NoError(t, err)
		for _, q := range []string{
			`INSERT INTO chat_messages (chat_id, role, content_version, automation_id, input_id, prompt_hooks_pending)
				VALUES ($1, 'user', 1, $2, $3, true)`,
			`INSERT INTO chat_queued_messages (chat_id, content, created_by, automation_id, input_id, prompt_hooks_pending)
				SELECT id, '[]', owner_id, $2, $3, true FROM chats WHERE id = $1`,
		} {
			_, err = sqlDB.ExecContext(ctx, q, o.chat.ID, automation.ID, inputID)
			require.NoError(t, err, q)
		}

		require.NoError(t, db.DeleteChatAutomationByID(ctx, automation.ID))
		requireGone(ctx, t, automation.ID)

		for _, table := range []string{"chat_messages", "chat_queued_messages"} {
			var gotAutomation, gotInput uuid.UUID
			var hooksPending bool
			require.NoError(t, sqlDB.QueryRowContext(ctx,
				`SELECT automation_id, input_id, prompt_hooks_pending FROM `+table+` WHERE chat_id = $1`, o.chat.ID,
			).Scan(&gotAutomation, &gotInput, &hooksPending), table)
			require.Equal(t, automation.ID, gotAutomation, table)
			require.Equal(t, inputID, gotInput, table)
			require.True(t, hooksPending, table)
		}
		chat, err := db.GetChatByID(ctx, o.chat.ID)
		require.NoError(t, err)
		require.Equal(t, nullUUID(automation.ID), chat.AutomationID)
	})
}

func TestChatAutomationForeignKeys(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	user := dbgen.User(t, db, database.User{})
	o := newChatAutomationOrg(t, db, user)
	other := newChatAutomationOrg(t, db, user)

	t.Run("ModelConfig", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		params := o.scheduleParams()
		params.NewChatModelConfigID = nullUUID(other.model.ID)
		_, err := db.InsertChatAutomation(ctx, params)
		require.True(t, database.IsForeignKeyViolation(err, database.ForeignKeyChatAutomationsNewChatModelConfig), "got %v", err)

		model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: o.org.ID})
		params = o.scheduleParams()
		params.NewChatModelConfigID = nullUUID(model.ID)
		automation, err := db.InsertChatAutomation(ctx, params)
		require.NoError(t, err)

		// Model configs are soft-deleted. The row stays, so the reference
		// stays valid. A hard delete of a referenced model is refused.
		_, err = db.DeleteChatModelConfigByID(ctx, model.ID)
		require.NoError(t, err)
		got, err := db.GetChatAutomationByID(ctx, automation.ID)
		require.NoError(t, err)
		require.Equal(t, nullUUID(model.ID), got.NewChatModelConfigID)

		_, err = sqlDB.ExecContext(ctx, `DELETE FROM chat_model_configs WHERE id = $1`, model.ID)
		require.True(t, database.IsForeignKeyViolation(err, database.ForeignKeyChatAutomationsNewChatModelConfig), "got %v", err)
	})

	t.Run("SecretKey", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		params := o.webhookParams()
		params.WebhookSecretKeyID = nullString("unknown-digest")
		_, err := db.InsertChatAutomation(ctx, params)
		require.True(t, database.IsForeignKeyViolation(err, database.ForeignKeyChatAutomationsWebhookSecretKeyID), "got %v", err)

		digest := "digest-" + uuid.NewString()
		require.NoError(t, db.InsertDBCryptKey(ctx, database.InsertDBCryptKeyParams{
			Number: 1, ActiveKeyDigest: digest, Test: "test",
		}))
		params.WebhookSecretKeyID = nullString(digest)
		automation, err := db.InsertChatAutomation(ctx, params)
		require.NoError(t, err)

		err = db.RevokeDBCryptKey(ctx, digest)
		require.True(t, database.IsForeignKeyViolation(err, database.ForeignKeyChatAutomationsWebhookSecretKeyID), "got %v", err)

		_, err = sqlDB.ExecContext(ctx,
			`UPDATE chat_automations SET webhook_secret = '', webhook_secret_key_id = NULL WHERE id = $1`, automation.ID)
		require.NoError(t, err, "a secret can be blanked")
		require.NoError(t, db.RevokeDBCryptKey(ctx, digest))
	})
}

func TestChatAutomationQueries(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	owner := dbgen.User(t, db, database.User{})
	otherOwner := dbgen.User(t, db, database.User{})
	o := newChatAutomationOrg(t, db, owner)
	other := newChatAutomationOrg(t, db, owner)
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: otherOwner.ID, OrganizationID: o.org.ID})
	ctx := testutil.Context(t, testutil.WaitLong)

	first := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: o.org.ID, OwnerID: owner.ID})
	second := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: o.org.ID, OwnerID: owner.ID})
	third := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: o.org.ID, OwnerID: otherOwner.ID})
	inOtherOrg := dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: other.org.ID, OwnerID: owner.ID})

	// Give the rows known times. first and second share one, so their
	// order comes from the ID.
	base := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	for id, createdAt := range map[uuid.UUID]time.Time{
		first.ID: base, second.ID: base, third.ID: base.Add(time.Hour),
	} {
		_, err := sqlDB.ExecContext(ctx, `UPDATE chat_automations SET created_at = $2 WHERE id = $1`, id, createdAt)
		require.NoError(t, err)
	}
	tied := []uuid.UUID{first.ID, second.ID}
	if strings.Compare(first.ID.String(), second.ID.String()) < 0 {
		tied = []uuid.UUID{second.ID, first.ID}
	}

	ids := func(rows []database.GetChatAutomationsByOrganizationIDRow) []uuid.UUID {
		out := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			out = append(out, row.ID)
		}
		return out
	}

	t.Run("ListEveryOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		rows, err := db.GetChatAutomationsByOrganizationID(ctx, database.GetChatAutomationsByOrganizationIDParams{
			OrganizationID: o.org.ID,
		})
		require.NoError(t, err)
		require.Equal(t, append([]uuid.UUID{third.ID}, tied...), ids(rows), "newest first, then by ID")
	})

	t.Run("ListOneOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		rows, err := db.GetChatAutomationsByOrganizationID(ctx, database.GetChatAutomationsByOrganizationIDParams{
			OrganizationID: o.org.ID, OwnerID: owner.ID,
		})
		require.NoError(t, err)
		require.Equal(t, tied, ids(rows))

		rows, err = db.GetChatAutomationsByOrganizationID(ctx, database.GetChatAutomationsByOrganizationIDParams{
			OrganizationID: other.org.ID, OwnerID: otherOwner.ID,
		})
		require.NoError(t, err)
		require.Empty(t, rows)
	})

	t.Run("CountAcrossOrganizations", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		count, err := db.CountChatAutomationsByOwnerID(ctx, owner.ID)
		require.NoError(t, err)
		require.EqualValues(t, 3, count)
		count, err = db.CountChatAutomationsByOwnerID(ctx, otherOwner.ID)
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
		count, err = db.CountChatAutomationsByOwnerID(ctx, uuid.New())
		require.NoError(t, err)
		require.Zero(t, count)
	})

	t.Run("GetReturnsTheInsertedRow", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		got, err := db.GetChatAutomationByID(ctx, inOtherOrg.ID)
		require.NoError(t, err)
		require.Equal(t, inOtherOrg, got)
		require.True(t, got.WebhookSecret.Valid)
	})

	// A list covers every automation of an organization, so it must not
	// load a secret. The row type has no field that could carry one.
	t.Run("ListRowHasNoSecret", func(t *testing.T) {
		t.Parallel()
		row := reflect.TypeOf(database.GetChatAutomationsByOrganizationIDRow{})
		for i := range row.NumField() {
			column := row.Field(i).Tag.Get("db")
			require.NotContains(t, []string{"webhook_secret", "webhook_secret_key_id"}, column)
		}
		full := reflect.TypeOf(database.ChatAutomation{})
		require.Equal(t, full.NumField()-2, row.NumField(), "the list leaves out the secret and its key ID only")
	})
}

// The chats queries name their columns one by one. A chat's automation
// columns must come back from them and from the hand-written scans.
func TestChatAutomationColumnsOnChats(t *testing.T) {
	t.Parallel()

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	user := dbgen.User(t, db, database.User{RBACRoles: []string{rbac.RoleOwner().String()}})
	o := newChatAutomationOrg(t, db, user)
	ctx := testutil.Context(t, testutil.WaitLong)

	require.False(t, o.chat.AutomationID.Valid, "a new chat has no automation")
	require.False(t, o.chat.ManageAutomationsEnabled)

	automationID := uuid.New()
	_, err := sqlDB.ExecContext(ctx,
		`UPDATE chats SET automation_id = $2, manage_automations_enabled = true WHERE id = $1`, o.chat.ID, automationID)
	require.NoError(t, err)

	requireColumns := func(name string, chat database.Chat, err error) {
		t.Helper()
		require.NoError(t, err, name)
		require.Equal(t, o.chat.ID, chat.ID, name)
		require.Equal(t, nullUUID(automationID), chat.AutomationID, name)
		require.True(t, chat.ManageAutomationsEnabled, name)
	}
	first := func(chats []database.Chat, err error) (database.Chat, error) {
		if err != nil || len(chats) != 1 {
			return database.Chat{}, err
		}
		return chats[0], nil
	}

	chat, err := db.GetChatByID(ctx, o.chat.ID)
	requireColumns("GetChatByID", chat, err)
	chat, err = db.GetChatByIDForUpdate(ctx, o.chat.ID)
	requireColumns("GetChatByIDForUpdate", chat, err)
	chat, err = db.GetChatByIDForShare(ctx, o.chat.ID)
	requireColumns("GetChatByIDForShare", chat, err)
	chat, err = db.LockChatAndBumpSnapshotVersion(ctx, o.chat.ID)
	requireColumns("LockChatAndBumpSnapshotVersion", chat, err)
	chat, err = first(db.ArchiveChatByID(ctx, o.chat.ID))
	requireColumns("ArchiveChatByID", chat, err)
	chat, err = first(db.UnarchiveChatByID(ctx, o.chat.ID))
	requireColumns("UnarchiveChatByID", chat, err)
	chat, err = db.UpdateChatTitleByID(ctx, database.UpdateChatTitleByIDParams{
		ID: o.chat.ID, Title: "renamed", TitleSource: database.ChatTitleSourceUser,
	})
	requireColumns("UpdateChatTitleByID", chat, err)
	chat, err = db.UpdateChatLabelsByID(ctx, database.UpdateChatLabelsByIDParams{ID: o.chat.ID, Labels: json.RawMessage(`{}`)})
	requireColumns("UpdateChatLabelsByID", chat, err)
	chat, err = db.UpdateChatLastModelConfigByID(ctx, database.UpdateChatLastModelConfigByIDParams{ID: o.chat.ID, LastModelConfigID: o.model.ID})
	requireColumns("UpdateChatLastModelConfigByID", chat, err)
	chat, err = db.UpdateChatMCPServerIDs(ctx, database.UpdateChatMCPServerIDsParams{ID: o.chat.ID, MCPServerIDs: []uuid.UUID{}})
	requireColumns("UpdateChatMCPServerIDs", chat, err)
	chat, err = db.UpdateChatPlanModeByID(ctx, database.UpdateChatPlanModeByIDParams{ID: o.chat.ID})
	requireColumns("UpdateChatPlanModeByID", chat, err)
	chat, err = db.UpdateChatRetryState(ctx, database.UpdateChatRetryStateParams{ID: o.chat.ID, RetryState: json.RawMessage(`{}`)})
	requireColumns("UpdateChatRetryState", chat, err)
	chat, err = db.UpdateChatBuildAgentBinding(ctx, database.UpdateChatBuildAgentBindingParams{ID: o.chat.ID})
	requireColumns("UpdateChatBuildAgentBinding", chat, err)
	chat, err = db.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{ID: o.chat.ID})
	requireColumns("UpdateChatWorkspaceBinding", chat, err)
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{ID: o.chat.ID, Status: database.ChatStatusWaiting})
	requireColumns("UpdateChatStatus", chat, err)
	chat, err = db.UpdateChatExecutionState(ctx, database.UpdateChatExecutionStateParams{ID: o.chat.ID, Status: database.ChatStatusWaiting})
	requireColumns("UpdateChatExecutionState", chat, err)

	// GetAuthorizedChats and GetAuthorizedChatsByChatFileID scan their rows
	// by hand.
	subject, _, err := httpmw.UserRBACSubject(ctx, db, user.ID, rbac.ExpandableScope(rbac.ScopeAll))
	require.NoError(t, err)
	prepared, err := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()).
		Prepare(ctx, subject, policy.ActionRead, rbac.ResourceChat.Type)
	require.NoError(t, err)
	rows, err := db.GetAuthorizedChats(ctx, database.GetChatsParams{}, prepared)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	requireColumns("GetAuthorizedChats", rows[0].Chat, nil)

	file, err := db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID: user.ID, OrganizationID: o.org.ID, Name: "notes.txt", Mimetype: "text/plain", Data: []byte("notes"),
	})
	require.NoError(t, err)
	_, err = db.LinkChatFilesAfterLock(ctx, database.LinkChatFilesAfterLockParams{
		ChatID: o.chat.ID, FileIds: []uuid.UUID{file.ID}, MaxFileLinks: 1,
	})
	require.NoError(t, err)
	chat, err = first(db.GetAuthorizedChatsByChatFileID(ctx, file.ID, prepared))
	requireColumns("GetAuthorizedChatsByChatFileID", chat, err)
}
