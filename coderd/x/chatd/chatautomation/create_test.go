package chatautomation_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

var (
	testLimits = chatautomation.Limits{MaxPerOwner: 50, MinInterval: 5 * time.Minute, MaxPromptBytes: 64}
	// testNow is far from the real clock, so a Create that reads the real
	// clock for the first run of a schedule gives another time.
	testNow = time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)
)

// hookStore runs a hook around two database calls of a create. It sits below
// dbauthz, so dbauthz has already read the chat, model configuration and
// project again when beforeInsert runs.
type hookStore struct {
	database.Store
	// afterCount runs after the count of the owner's automations.
	afterCount func()
	// beforeInsert runs before an insert reaches the database.
	beforeInsert func()
}

func (s hookStore) CountChatAutomationsByOwnerID(ctx context.Context, id uuid.UUID) (int64, error) {
	count, err := s.Store.CountChatAutomationsByOwnerID(ctx, id)
	if s.afterCount != nil {
		s.afterCount()
	}
	return count, err
}

func (s hookStore) InsertChatAutomation(ctx context.Context, arg database.InsertChatAutomationParams) (database.ChatAutomation, error) {
	if s.beforeInsert != nil {
		s.beforeInsert()
	}
	return s.Store.InsertChatAutomation(ctx, arg)
}

func (s hookStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		hooked := s
		hooked.Store = tx
		return fn(hooked)
	}, opts)
}

// harness is a Creator over a real authorizer.
type harness struct {
	creator *chatautomation.Creator
	// store is the dbauthz store that the creator uses.
	store database.Store
}

func newHarness(t testing.TB, db database.Store, limits chatautomation.Limits) harness {
	t.Helper()
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	store := dbauthz.New(db, authorizer, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), coderdtest.AccessControlStorePointer())
	return harness{creator: chatautomation.NewCreator(store, authorizer, limits), store: store}
}

// userActor is the subject of a user, with the roles that the database gives it.
func userActor(t testing.TB, db database.Store, userID uuid.UUID) rbac.Subject {
	t.Helper()
	actor, _, err := httpmw.UserRBACSubject(testutil.Context(t, testutil.WaitMedium), db, userID, rbac.ScopeAll)
	require.NoError(t, err)
	return actor
}

// member adds a user to the organization with the default roles of its members
// and the given roles.
func member(t testing.TB, db database.Store, orgID uuid.UUID, roles ...string) database.User {
	t.Helper()
	user := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: orgID, Roles: roles})
	return user
}

func nullID(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }

func createParams(orgID uuid.UUID, req codersdk.CreateChatAutomationRequest) chatautomation.CreateParams {
	return chatautomation.CreateParams{OrganizationID: orgID, Request: req, Now: testNow}
}

// webhookToNewChat is a valid request that needs no chat.
func webhookToNewChat(t testing.TB, f fixture) codersdk.CreateChatAutomationRequest {
	t.Helper()
	return codersdk.CreateChatAutomationRequest{
		Name: testutil.GetRandomName(t), Kind: codersdk.ChatAutomationKindWebhook, TargetMode: codersdk.ChatAutomationTargetModeNewChat,
		NewChatModelConfigID: &f.model.ID, WebhookAuth: codersdk.ChatAutomationWebhookAuthBearer, Prompt: "Do the task.",
	}
}

// patch is a JSON object of the given fields.
func patch(fields ...string) string { return "{" + strings.Join(fields, ",") + "}" }

// request decodes JSON objects over each other, as the API reads a body. The
// replacer fills the placeholders of each object. A request that the API
// decodes owns its pointers, so a decode never changes a shared value.
func request(t testing.TB, placeholders *strings.Replacer, objects ...string) codersdk.CreateChatAutomationRequest {
	t.Helper()
	var req codersdk.CreateChatAutomationRequest
	for _, object := range objects {
		if object != "" {
			require.NoError(t, json.Unmarshal([]byte(placeholders.Replace(object)), &req))
		}
	}
	return req
}

// TestCreate creates one automation per row as its owner, reads the row back
// as the owner and compares every column.
func TestCreate(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	f := newFixture(t, db)
	target := f.chat(t)
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: f.org.ID, OwnerID: f.owner.ID})
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte(uuid.NewString()))
	prompt := strings.Repeat("p", testLimits.MaxPromptBytes)
	base := patch(`"name":" \t$NAME\n "`, `"kind":"webhook"`, `"target_mode":"existing_chat"`, `"target_chat_id":"$TARGET"`, `"webhook_auth":"bearer"`, `"prompt":"`+prompt+`"`)

	str := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	auth := func(a database.ChatAutomationWebhookAuth) database.NullChatAutomationWebhookAuth {
		return database.NullChatAutomationWebhookAuth{ChatAutomationWebhookAuth: a, Valid: true}
	}
	busy := func(w database.ChatAutomationWhenBusy) database.NullChatAutomationWhenBusy {
		return database.NullChatAutomationWhenBusy{ChatAutomationWhenBusy: w, Valid: true}
	}
	// A schedule that runs in New York, with a webhook secret of no kind.
	const newYork = `"kind":"schedule","webhook_auth":"","schedule_time_zone":"America/New_York"`
	nyc := func(a *database.ChatAutomation, spec string, next time.Time) {
		a.Kind, a.WebhookAuth, a.WhenBusy = database.ChatAutomationKindSchedule, database.NullChatAutomationWebhookAuth{}, busy(database.ChatAutomationWhenBusySkip)
		a.ScheduleCron, a.ScheduleTimeZone = str(spec), str("America/New_York")
		a.ScheduleNextRunAt = sql.NullTime{Time: next, Valid: !next.IsZero()}
	}
	longName := strings.Repeat("é", 128) // 256 bytes: the limit counts characters.

	for _, tc := range []struct {
		name string
		// patch replaces fields of the base request.
		patch string
		// want changes the row that the base request stores.
		want func(*database.ChatAutomation)
	}{
		{name: "WebhookBearerDefaults"},
		{
			name: "WebhookGitHub", patch: `{"webhook_auth":"github","enabled":false,"when_busy":"skip"}`,
			want: func(a *database.ChatAutomation) {
				a.WebhookAuth, a.Enabled, a.WhenBusy = auth(database.ChatAutomationWebhookAuthGithub), false, busy(database.ChatAutomationWhenBusySkip)
			},
		},
		{
			name: "WebhookStandardWebhooksTrimsSecret", patch: `{"webhook_auth":"standard_webhooks","webhook_secret":"` + secret + `\n"}`,
			want: func(a *database.ChatAutomation) {
				a.WebhookAuth, a.WebhookSecret = auth(database.ChatAutomationWebhookAuthStandardWebhooks), str(secret)
			},
		},
		{
			// 00:06 in New York, so 09:00 on the same day is 14:00 UTC.
			name: "ScheduleEnabled", patch: patch(newYork, `"schedule_cron":"  0  9 * * 1-5 ","enabled":true`),
			want: func(a *database.ChatAutomation) { nyc(a, "0 9 * * 1-5", time.Date(2031, 3, 4, 14, 0, 0, 0, time.UTC)) },
		},
		{
			name: "ScheduleDisabledAtMinimum", patch: patch(newYork, `"schedule_cron":"*/5 * * * *","enabled":false`),
			want: func(a *database.ChatAutomation) { nyc(a, "*/5 * * * *", time.Time{}); a.Enabled = false },
		},
		{
			name: "NewChat", patch: `{"target_mode":"new_chat","target_chat_id":null,"new_chat_model_config_id":"$MODEL","project_id":"$PROJECT","reasoning_effort":"high"}`,
			want: func(a *database.ChatAutomation) {
				a.TargetMode, a.TargetChatID, a.WhenBusy = database.ChatAutomationTargetModeNewChat, uuid.NullUUID{}, database.NullChatAutomationWhenBusy{}
				a.NewChatModelConfigID, a.ProjectID = nullID(f.model.ID), nullID(project.ID)
				a.ReasoningEffort = database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffortHigh, Valid: true}
			},
		},
		{name: "NameAtLimit", patch: `{"name":"` + longName + `"}`, want: func(a *database.ChatAutomation) { a.Name = longName }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			name := testutil.GetRandomName(t)
			req := request(t, strings.NewReplacer("$NAME", name, "$TARGET", target.ID.String(), "$MODEL", f.model.ID.String(), "$PROJECT", project.ID.String()), base, tc.patch)
			want := database.ChatAutomation{
				OrganizationID: f.org.ID, OwnerID: f.owner.ID, Name: name,
				Kind: database.ChatAutomationKindWebhook, Enabled: true, Prompt: prompt,
				TargetMode: database.ChatAutomationTargetModeExistingChat, TargetChatID: nullID(target.ID),
				WhenBusy: busy(database.ChatAutomationWhenBusyQueue), WebhookAuth: auth(database.ChatAutomationWebhookAuthBearer),
				ScheduleRevision: 1, // The database default.
			}
			if tc.want != nil {
				tc.want(&want)
			}
			h, actor := newHarness(t, db, testLimits), userActor(t, db, f.owner.ID)

			result, err := h.creator.Create(ctx, actor, createParams(f.org.ID, req))
			require.NoError(t, err)

			got, err := h.store.GetChatAutomationByID(dbauthz.As(ctx, actor), result.Automation.ID)
			require.NoError(t, err)
			// Coder creates the secret of a webhook, unless the sender supplied it.
			if want.WebhookAuth.Valid && !want.WebhookSecret.Valid {
				require.NotEmpty(t, result.GeneratedSecret)
				want.WebhookSecret = str(result.GeneratedSecret)
			} else {
				require.Empty(t, result.GeneratedSecret)
			}
			want.ID, want.CreatedAt, want.UpdatedAt = got.ID, got.CreatedAt, got.UpdatedAt
			got.ScheduleNextRunAt.Time = got.ScheduleNextRunAt.Time.UTC()
			require.Equal(t, want, got)
		})
	}
}

const (
	asSchedule = `"kind":"schedule","webhook_auth":"","schedule_cron":"0 9 * * *","schedule_time_zone":"UTC"`
	asNewChat  = `"target_mode":"new_chat","target_chat_id":null,"new_chat_model_config_id":"$MODEL"`
	canaryLen  = len("canary-") + 36
)

// TestCreateRejected changes one thing of a valid request per row. Create
// returns every violation of the row.
func TestCreateRejected(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	setup := testutil.Context(t, testutil.WaitMedium)
	f, other := newFixture(t, db), newFixture(t, db)
	// The actor can read what it owns in both organizations, so the
	// organization rule is what refuses the references of the other one.
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: f.owner.ID, OrganizationID: other.org.ID})
	actor := userActor(t, db, f.owner.ID)
	chat := func(org database.Organization, owner, model uuid.UUID, seed database.Chat) database.Chat {
		seed.OrganizationID, seed.OwnerID, seed.LastModelConfigID = org.ID, owner, model
		return dbgen.Chat(t, db, seed)
	}
	own := f.chat(t)
	stranger := chat(f.org, member(t, db, f.org.ID).ID, f.model.ID, database.Chat{})
	shared := chat(f.org, member(t, db, f.org.ID).ID, f.model.ID, database.Chat{})
	require.NoError(t, db.UpdateChatACLByID(setup, database.UpdateChatACLByIDParams{
		ID: shared.ID, UserACL: database.ChatACL{f.owner.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}}, GroupACL: database.ChatACL{},
	}))
	sub := chat(f.org, f.owner.ID, f.model.ID, database.Chat{ParentChatID: nullID(own.ID), RootChatID: nullID(own.ID)})
	foreign := chat(other.org, f.owner.ID, other.model.ID, database.Chat{})
	foreignProject := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: other.org.ID, OwnerID: f.owner.ID})
	ids := []string{
		"$MISSING", uuid.NewString(), "$OWN", own.ID.String(), "$STRANGER", stranger.ID.String(), "$SHARED", shared.ID.String(),
		"$SUB", sub.ID.String(), "$FOREIGN_CHAT", foreign.ID.String(), "$FOREIGN_MODEL", other.model.ID.String(),
		"$FOREIGN_PROJECT", foreignProject.ID.String(), "$MODEL", f.model.ID.String(),
	}
	base := patch(`"name":"$NAME"`, `"kind":"webhook"`, `"target_mode":"existing_chat"`, `"target_chat_id":"$OWN"`, `"webhook_auth":"bearer"`, `"prompt":"Do the task."`)

	bad := func(pairs ...string) []codersdk.ValidationError {
		var out []codersdk.ValidationError
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, codersdk.ValidationError{Field: pairs[i], Detail: pairs[i+1]})
		}
		return out
	}
	type rejection struct {
		name string
		// patch replaces fields of the valid request. $CANARY is a random
		// string that no detail may hold.
		patch string
		// edit changes what JSON cannot hold.
		edit func(*codersdk.CreateChatAutomationRequest)
		// want lists each violation: the field, and a part of the detail where
		// the rule has a limit or a reason ("" means any detail).
		want []codersdk.ValidationError
	}
	rows := []rejection{
		{name: "NameBlank", patch: `{"name":" \t "}`, want: bad("name", "")},
		{name: "NameTooLong", patch: `{"name":"` + strings.Repeat("é", 129) + `"}`, want: bad("name", "128")},
		{name: "NameNUL", patch: `{"name":"a\u0000b"}`, want: bad("name", "")},
		{name: "NameInvalidUTF8", edit: func(r *codersdk.CreateChatAutomationRequest) { r.Name = "a\xffb" }, want: bad("name", "")},
		{name: "PromptBlank", patch: `{"prompt":" \n"}`, want: bad("prompt", "")},
		{name: "PromptTooLong", patch: `{"prompt":"` + strings.Repeat("p", testLimits.MaxPromptBytes+1) + `"}`, want: bad("prompt", "64")},
		{name: "PromptNUL", patch: `{"prompt":"a\u0000b"}`, want: bad("prompt", "")},
		{name: "KindOutsideEnum", patch: `{"kind":"$CANARY"}`, want: bad("kind", "")},
		{name: "TargetModeOutsideEnum", patch: `{"target_mode":"$CANARY"}`, want: bad("target_mode", "")},
		{name: "WhenBusyOutsideEnum", patch: `{"when_busy":"$CANARY"}`, want: bad("when_busy", "")},
		{name: "ReasoningEffortOutsideEnum", patch: patch(asNewChat, `"reasoning_effort":"$CANARY"`), want: bad("reasoning_effort", "")},
		{name: "WebhookAuthOutsideEnum", patch: `{"webhook_auth":"$CANARY"}`, want: bad("webhook_auth", "")},
		{name: "ExistingChatWithoutTarget", patch: `{"target_chat_id":null}`, want: bad("target_chat_id", "")},
		{name: "ExistingChatWithModel", patch: `{"new_chat_model_config_id":"$MODEL"}`, want: bad("new_chat_model_config_id", "existing chat keeps")},
		{name: "ExistingChatWithProject", patch: `{"project_id":"$MISSING"}`, want: bad("project_id", "only a new chat has a project")},
		{name: "ExistingChatWithReasoningEffort", patch: `{"reasoning_effort":"high"}`, want: bad("reasoning_effort", "existing chat keeps")},
		{name: "NewChatWithoutModel", patch: patch(asNewChat, `"new_chat_model_config_id":null`), want: bad("new_chat_model_config_id", "")},
		{name: "NewChatWithTarget", patch: patch(asNewChat, `"target_chat_id":"$OWN"`), want: bad("target_chat_id", "")},
		{name: "NewChatWithWhenBusy", patch: patch(asNewChat, `"when_busy":"skip"`), want: bad("when_busy", "")},
		{name: "WebhookWithSchedule", patch: `{"schedule_cron":"0 9 * * *","schedule_time_zone":"UTC"}`, want: bad("schedule_cron", "", "schedule_time_zone", "")},
		{name: "ScheduleWithWebhookFields", patch: patch(asSchedule, `"webhook_auth":"bearer","webhook_secret":"x"`), want: bad("webhook_auth", "", "webhook_secret", "")},
		{name: "ScheduleWithoutCronAndZone", patch: patch(asSchedule, `"schedule_cron":"","schedule_time_zone":""`), want: bad("schedule_cron", "", "schedule_time_zone", "")},
		// Neither value reaches cron.Standard, whose errors quote the input.
		{name: "CronTooLong", patch: patch(asSchedule, `"schedule_cron":"$CANARY`+strings.Repeat("*", 257-canaryLen)+`"`), want: bad("schedule_cron", "256")},
		{name: "ZoneTooLong", patch: patch(asSchedule, `"schedule_time_zone":"$CANARY`+strings.Repeat("a", 65-canaryLen)+`"`), want: bad("schedule_time_zone", "64")},
		// The limits are exact: a value at the limit reaches cron.Standard.
		{name: "CronAtLimit", patch: patch(asSchedule, `"schedule_cron":"`+strings.Repeat("*", 256)+`"`), want: bad("schedule_cron", "expected 5 fields")},
		{name: "ZoneAtLimit", patch: patch(asSchedule, `"schedule_time_zone":"`+strings.Repeat("a", 64)+`"`), want: bad("schedule_time_zone", "unknown time zone")},
		{name: "ScheduleBelowMinimum", patch: patch(asSchedule, `"schedule_cron":"*/2 * * * *"`), want: bad("schedule_cron", "5m0s")},
		{name: "BearerWithSecret", patch: `{"webhook_secret":"$CANARY"}`, want: bad("webhook_secret", "")},
		{name: "GitHubWithWhiteSpaceSecret", patch: `{"webhook_auth":"github","webhook_secret":" \n"}`, want: bad("webhook_secret", "")},
		{name: "StandardWebhooksWithWhiteSpaceSecret", patch: `{"webhook_auth":"standard_webhooks","webhook_secret":" \n"}`, want: bad("webhook_secret", "Required")},
		{name: "StandardWebhooksWithMalformedSecret", patch: `{"webhook_auth":"standard_webhooks","webhook_secret":"whsec_$CANARY!"}`, want: bad("webhook_secret", "Standard Webhooks secret")},
		{name: "TargetChatMissing", patch: `{"target_chat_id":"$MISSING"}`, want: bad("target_chat_id", "Chat not found.")},
		// The same detail as a missing chat: it does not tell the actor that the chat exists.
		{name: "TargetChatOfAnotherUser", patch: `{"target_chat_id":"$STRANGER"}`, want: bad("target_chat_id", "Chat not found.")},
		{name: "TargetChatInAnotherOrganization", patch: `{"target_chat_id":"$FOREIGN_CHAT"}`, want: bad("target_chat_id", "not in this organization")},
		{name: "TargetSubChat", patch: `{"target_chat_id":"$SUB"}`, want: bad("target_chat_id", "not a root chat")},
		{name: "TargetChatReadableNotUpdatable", patch: `{"target_chat_id":"$SHARED"}`, want: bad("target_chat_id", "cannot send messages")},
		{name: "ModelMissing", patch: patch(asNewChat, `"new_chat_model_config_id":"$MISSING"`), want: bad("new_chat_model_config_id", "not found")},
		{name: "ModelInAnotherOrganization", patch: patch(asNewChat, `"new_chat_model_config_id":"$FOREIGN_MODEL"`), want: bad("new_chat_model_config_id", "not in this organization")},
		{name: "ProjectMissing", patch: patch(asNewChat, `"project_id":"$MISSING"`), want: bad("project_id", "not found")},
		{name: "ProjectInAnotherOrganization", patch: patch(asNewChat, `"project_id":"$FOREIGN_PROJECT"`), want: bad("project_id", "not in this organization")},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			canary := "canary-" + uuid.NewString()
			req := request(t, strings.NewReplacer(append([]string{"$CANARY", canary, "$NAME", testutil.GetRandomName(t)}, ids...)...), base, tc.patch)
			if tc.edit != nil {
				tc.edit(&req)
			}

			_, err := newHarness(t, db, testLimits).creator.Create(ctx, actor, createParams(f.org.ID, req))

			var invalid *chatautomation.InvalidRequestError
			require.ErrorAs(t, err, &invalid)
			require.Len(t, invalid.Validations, len(tc.want))
			for i, want := range tc.want {
				require.Equal(t, want.Field, invalid.Validations[i].Field)
				require.Contains(t, invalid.Validations[i].Detail, want.Detail)
				require.NotContains(t, invalid.Validations[i].Detail, canary)
			}
		})
	}
}

// TestCreateCap sets the limit of an owner to 2.
func TestCreateCap(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	limits := testLimits
	limits.MaxPerOwner = 2

	// The owner counts across organizations, and another owner is not affected.
	t.Run("AcrossOrganizations", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		a, b := newFixture(t, db), newFixture(t, db)
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: a.owner.ID, OrganizationID: b.org.ID})
		h, actor := newHarness(t, db, limits), userActor(t, db, a.owner.ID)

		for _, f := range []fixture{a, b} {
			_, err := h.creator.Create(ctx, actor, createParams(f.org.ID, webhookToNewChat(t, f)))
			require.NoError(t, err)
		}
		_, err := h.creator.Create(ctx, actor, createParams(a.org.ID, webhookToNewChat(t, a)))
		var limitErr *chatautomation.LimitError
		require.ErrorAs(t, err, &limitErr)
		require.EqualValues(t, 2, limitErr.Max)

		_, err = h.creator.Create(ctx, userActor(t, db, member(t, db, a.org.ID).ID), createParams(a.org.ID, webhookToNewChat(t, a)))
		require.NoError(t, err)
	})

	// Two creates with one slot left, in two organizations of one owner. The
	// first stops after its count, so the second runs while the first holds
	// the owner lock: it must wait for the lock, count after the first has
	// inserted, and be refused. Without the lock, or with a lock per
	// organization, it counts the same automations and both insert.
	t.Run("Concurrent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		f, other := newFixture(t, db), newFixture(t, db)
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: f.owner.ID, OrganizationID: other.org.ID})
		counted, release, results := make(chan struct{}, 1), make(chan struct{}), make(chan error, 2)
		first := newHarness(t, hookStore{Store: db, afterCount: func() {
			counted <- struct{}{}
			<-release
		}}, limits)
		second, actor := newHarness(t, db, limits), userActor(t, db, f.owner.ID)
		_, err := second.creator.Create(ctx, actor, createParams(f.org.ID, webhookToNewChat(t, f)))
		require.NoError(t, err)
		var once sync.Once
		unblock := func() { once.Do(func() { close(release) }) }
		t.Cleanup(unblock)
		create := func(h harness, in fixture) {
			_, err := h.creator.Create(ctx, actor, createParams(in.org.ID, webhookToNewChat(t, in)))
			results <- err
		}

		go create(first, f)
		testutil.RequireReceive(ctx, t, counted)
		go create(second, other)
		// The second create has either finished or waits for the advisory lock.
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			var waiting int
			err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted
				AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&waiting)
			return len(results) > 0 || (err == nil && waiting > 0)
		}, testutil.IntervalFast, "the second create neither finished nor waited for the owner lock")
		unblock()

		var limitErr *chatautomation.LimitError
		created, limited := 0, 0
		for range 2 {
			switch err := testutil.RequireReceive(ctx, t, results); {
			case err == nil:
				created++
			case errors.As(err, &limitErr):
				limited++
			}
		}
		require.Equal(t, [2]int{1, 1}, [2]int{created, limited})
		count, err := db.CountChatAutomationsByOwnerID(ctx, f.owner.ID)
		require.NoError(t, err)
		require.EqualValues(t, 2, count)
	})
}

// TestCreateUnwrappedStore refuses a store that dbauthz does not wrap, which
// would skip every authorization check of the reads and of the insert.
func TestCreateUnwrappedStore(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	f := newFixture(t, db)
	creator := chatautomation.NewCreator(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), testLimits)

	_, err := creator.Create(ctx, userActor(t, db, f.owner.ID), createParams(f.org.ID, webhookToNewChat(t, f)))

	require.ErrorContains(t, err, "dbauthz")
	count, err := db.CountChatAutomationsByOwnerID(ctx, f.owner.ID)
	require.NoError(t, err)
	require.Zero(t, count)
}

// TestCreateReferenceDeleted deletes the target chat or the project after
// dbauthz has read it again for the insert, so that a trigger of the table finds
// it missing. The owner is fine, so the answer must not say that the owner is
// unavailable.
func TestCreateReferenceDeleted(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// project deletes the project of a new chat instead of the target chat.
		project bool
	}{
		{name: "ChatAfterRecheck"},
		{name: "ProjectAfterRecheck", project: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
			ctx := testutil.Context(t, testutil.WaitMedium)
			f := newFixture(t, db)
			req := webhookToNewChat(t, f)
			deleteSQL, id := "DELETE FROM chats WHERE id = $1", uuid.UUID{}
			if tc.project {
				project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: f.org.ID, OwnerID: f.owner.ID})
				req.ProjectID, deleteSQL, id = &project.ID, "DELETE FROM chat_projects WHERE id = $1", project.ID
			} else {
				target := f.chat(t)
				req.TargetMode, req.NewChatModelConfigID, req.TargetChatID = codersdk.ChatAutomationTargetModeExistingChat, nil, &target.ID
				id = target.ID
			}
			remove := func() {
				_, err := sqlDB.ExecContext(ctx, deleteSQL, id)
				require.NoError(t, err)
			}
			h := newHarness(t, hookStore{Store: db, beforeInsert: remove}, testLimits)

			_, err := h.creator.Create(ctx, userActor(t, db, f.owner.ID), createParams(f.org.ID, req))

			require.ErrorIs(t, err, chatautomation.ErrReferenceUnavailable)
			require.NotErrorIs(t, err, chatautomation.ErrOwnerUnavailable)
		})
	}
}

// TestCreateReferenceDeletedWhileInserting leaves a delete of the target chat
// or the project open until the insert waits on it. The trigger of the table
// still reads the row, so the foreign key check is what fails when the delete
// commits. dbpurge deletes chats in one long transaction, so that window is not
// short.
func TestCreateReferenceDeletedWhileInserting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// reference sets the chat or project that the request names and returns
		// the statement that deletes it.
		reference func(t *testing.T, f fixture, req *codersdk.CreateChatAutomationRequest) (deleteSQL string, id uuid.UUID)
	}{
		{name: "TargetChat", reference: func(t *testing.T, f fixture, req *codersdk.CreateChatAutomationRequest) (string, uuid.UUID) {
			target := f.chat(t)
			req.TargetMode, req.NewChatModelConfigID, req.TargetChatID = codersdk.ChatAutomationTargetModeExistingChat, nil, &target.ID
			return "DELETE FROM chats WHERE id = $1", target.ID
		}},
		{name: "Project", reference: func(t *testing.T, f fixture, req *codersdk.CreateChatAutomationRequest) (string, uuid.UUID) {
			project := dbgen.ChatProject(t, f.db, database.ChatProject{OrganizationID: f.org.ID, OwnerID: f.owner.ID})
			req.ProjectID = &project.ID
			return "DELETE FROM chat_projects WHERE id = $1", project.ID
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newFixture(t, db)
			req := webhookToNewChat(t, f)
			deleteSQL, id := tc.reference(t, f, &req)
			p := createParams(f.org.ID, req)
			h, actor := newHarness(t, db, testLimits), userActor(t, db, f.owner.ID)

			tx, err := sqlDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, deleteSQL, id)
			require.NoError(t, err)
			errc := make(chan error, 1)
			go func() {
				_, err := h.creator.Create(ctx, actor, p)
				errc <- err
			}()
			require.Eventually(t, func() bool {
				var n int
				err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
					WHERE datname = current_database() AND wait_event_type = 'Lock'
					AND query LIKE '%INSERT INTO chat_automations%'`).Scan(&n)
				return err == nil && n > 0
			}, testutil.WaitMedium, testutil.IntervalFast, "the insert never waited on the open delete")
			require.NoError(t, tx.Commit())

			require.ErrorIs(t, <-errc, chatautomation.ErrReferenceUnavailable)
			var rows int
			require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM chat_automations").Scan(&rows))
			require.Zero(t, rows)
		})
	}
}
