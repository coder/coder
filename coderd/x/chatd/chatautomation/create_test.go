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
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/schedule/cron"
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

// recorder lists the store methods that Create calls.
type recorder struct {
	mu    sync.Mutex
	calls []string
	// afterCount runs after the count of the owner, when it is set.
	afterCount func()
}

func (r *recorder) add(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name)
}

func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

// recordingStore records the methods that Create calls on a dbauthz store. It
// embeds the store, so it still reports dbauthz in Wrappers.
type recordingStore struct {
	database.Store
	rec *recorder
}

func (s recordingStore) GetChatByID(ctx context.Context, id uuid.UUID) (database.Chat, error) {
	s.rec.add("GetChatByID")
	return s.Store.GetChatByID(ctx, id)
}

func (s recordingStore) GetChatModelConfigByID(ctx context.Context, id uuid.UUID) (database.ChatModelConfig, error) {
	s.rec.add("GetChatModelConfigByID")
	return s.Store.GetChatModelConfigByID(ctx, id)
}

func (s recordingStore) GetChatProjectByID(ctx context.Context, id uuid.UUID) (database.ChatProject, error) {
	s.rec.add("GetChatProjectByID")
	return s.Store.GetChatProjectByID(ctx, id)
}

func (s recordingStore) AcquireLock(ctx context.Context, id int64) error {
	s.rec.add("AcquireLock")
	return s.Store.AcquireLock(ctx, id)
}

func (s recordingStore) CountChatAutomationsByOwnerID(ctx context.Context, id uuid.UUID) (int64, error) {
	s.rec.add("CountChatAutomationsByOwnerID")
	count, err := s.Store.CountChatAutomationsByOwnerID(ctx, id)
	if s.rec.afterCount != nil {
		s.rec.afterCount()
	}
	return count, err
}

func (s recordingStore) InsertChatAutomation(ctx context.Context, arg database.InsertChatAutomationParams) (database.ChatAutomation, error) {
	s.rec.add("InsertChatAutomation")
	return s.Store.InsertChatAutomation(ctx, arg)
}

func (s recordingStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	s.rec.add("InTx")
	return s.Store.InTx(func(tx database.Store) error { return fn(recordingStore{Store: tx, rec: s.rec}) }, opts)
}

// harness is a Creator over a real authorizer and a recording store.
type harness struct {
	creator *chatautomation.Creator
	rec     *recorder
	// store is the dbauthz store without the recorder.
	store database.Store
}

func newHarness(t testing.TB, db database.Store, limits chatautomation.Limits) harness {
	t.Helper()
	authorizer := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	store := dbauthz.New(db, authorizer, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), coderdtest.AccessControlStorePointer())
	rec := &recorder{}
	return harness{creator: chatautomation.NewCreator(recordingStore{Store: store, rec: rec}, authorizer, limits), rec: rec, store: store}
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

// rfcExample is the request body of the RFC, without its comments.
const rfcExample = `{
  "name": "CI failures",
  "kind": "webhook",
  "enabled": true,
  "target_mode": "existing_chat",
  "target_chat_id": "$TARGET",
  "when_busy": "queue",
  "new_chat_model_config_id": null,
  "project_id": null,
  "reasoning_effort": null,
  "schedule_cron": null,
  "schedule_time_zone": null,
  "webhook_auth": "bearer",
  "webhook_secret": null,
  "prompt": "Investigate the failed CI run in the event data."
}`

// TestCreate creates one automation per row as its owner, reads the row back
// as the owner and compares every column.
func TestCreate(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	f := newFixture(t, db, nil)
	target, createdBy := f.chat(t), f.chat(t)
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
			name: "ScheduleEnabled", patch: patch(newYork, `"schedule_cron":"  0  9 * * 1-5 "`),
			want: func(a *database.ChatAutomation) { nyc(a, "0 9 * * 1-5", time.Date(2031, 3, 4, 14, 0, 0, 0, time.UTC)) },
		},
		{
			name: "ScheduleDisabledAtMinimum", patch: patch(newYork, `"schedule_cron":"*/5 * * * *","enabled":false`),
			want: func(a *database.ChatAutomation) { nyc(a, "*/5 * * * *", time.Time{}); a.Enabled = false },
		},
		{
			// MinInterval measures 57 minutes here, and the runs of 2026-03-08 are 3 minutes apart.
			name: "ScheduleAcrossDaylightSaving", patch: patch(newYork, `"schedule_cron":"1,58 1,3 * * *"`),
			want: func(a *database.ChatAutomation) {
				nyc(a, "1,58 1,3 * * *", time.Date(2031, 3, 4, 6, 1, 0, 0, time.UTC))
			},
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
		{
			name: "RFCExample", patch: rfcExample,
			want: func(a *database.ChatAutomation) {
				a.Name, a.Prompt, a.CreatedByChatID = "CI failures", "Investigate the failed CI run in the event data.", uuid.NullUUID{}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			name := testutil.GetRandomName(t)
			req := request(t, strings.NewReplacer("$NAME", name, "$TARGET", target.ID.String(), "$MODEL", f.model.ID.String(), "$PROJECT", project.ID.String()), base, tc.patch)
			want := database.ChatAutomation{
				OrganizationID: f.org.ID, OwnerID: f.owner.ID, Name: name, CreatedByChatID: nullID(createdBy.ID),
				Kind: database.ChatAutomationKindWebhook, Enabled: true, Prompt: prompt,
				TargetMode: database.ChatAutomationTargetModeExistingChat, TargetChatID: nullID(target.ID),
				WhenBusy: busy(database.ChatAutomationWhenBusyQueue), WebhookAuth: auth(database.ChatAutomationWebhookAuthBearer),
				ScheduleRevision: 1, // The database default.
			}
			if tc.want != nil {
				tc.want(&want)
			}
			h, actor := newHarness(t, db, testLimits), userActor(t, db, f.owner.ID)
			p := createParams(f.org.ID, req)
			p.CreatedByChatID = want.CreatedByChatID

			result, err := h.creator.Create(ctx, actor, p)
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
// returns every violation of the row, and never inserts. A request that breaks
// a field rule reads nothing from the store.
func TestCreateRejected(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	setup := testutil.Context(t, testutil.WaitMedium)
	f, other := newFixture(t, db, nil), newFixture(t, db, nil)
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
		// reference is true when a read of a chat, model or project gave the
		// violation.
		reference bool
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
		{name: "ZoneFactory", patch: patch(asSchedule, `"schedule_time_zone":"Factory"`), want: bad("schedule_time_zone", "Factory")},
		{name: "ScheduleBelowMinimum", patch: patch(asSchedule, `"schedule_cron":"*/2 * * * *"`), want: bad("schedule_cron", "5m0s")},
		// MinInterval measures 2 minutes here, across midnight.
		{name: "ScheduleOverStrict", patch: patch(asSchedule, `"schedule_cron":"0,58 0,23 * * 1"`), want: bad("schedule_cron", "5m0s")},
		{name: "BearerWithSecret", patch: `{"webhook_secret":"$CANARY"}`, want: bad("webhook_secret", "")},
		{name: "GitHubWithWhiteSpaceSecret", patch: `{"webhook_auth":"github","webhook_secret":" \n"}`, want: bad("webhook_secret", "")},
		{name: "StandardWebhooksWithWhiteSpaceSecret", patch: `{"webhook_auth":"standard_webhooks","webhook_secret":" \n"}`, want: bad("webhook_secret", "Required")},
		{name: "StandardWebhooksWithMalformedSecret", patch: `{"webhook_auth":"standard_webhooks","webhook_secret":"whsec_$CANARY!"}`, want: bad("webhook_secret", "Standard Webhooks secret")},
		{name: "TwoViolations", patch: `{"name":"","target_chat_id":null}`, want: bad("name", "", "target_chat_id", "")},
		{name: "TargetChatMissing", patch: `{"target_chat_id":"$MISSING"}`, want: bad("target_chat_id", "Chat not found."), reference: true},
		// The same detail as a missing chat: it does not tell the actor that the chat exists.
		{name: "TargetChatOfAnotherUser", patch: `{"target_chat_id":"$STRANGER"}`, want: bad("target_chat_id", "Chat not found."), reference: true},
		{name: "TargetChatInAnotherOrganization", patch: `{"target_chat_id":"$FOREIGN_CHAT"}`, want: bad("target_chat_id", "not in this organization"), reference: true},
		{name: "TargetSubChat", patch: `{"target_chat_id":"$SUB"}`, want: bad("target_chat_id", "not a root chat"), reference: true},
		{name: "TargetChatReadableNotUpdatable", patch: `{"target_chat_id":"$SHARED"}`, want: bad("target_chat_id", "cannot send messages"), reference: true},
		{name: "ModelMissing", patch: patch(asNewChat, `"new_chat_model_config_id":"$MISSING"`), want: bad("new_chat_model_config_id", "not found"), reference: true},
		{name: "ModelInAnotherOrganization", patch: patch(asNewChat, `"new_chat_model_config_id":"$FOREIGN_MODEL"`), want: bad("new_chat_model_config_id", "not in this organization"), reference: true},
		{name: "ProjectMissing", patch: patch(asNewChat, `"project_id":"$MISSING"`), want: bad("project_id", "not found"), reference: true},
		{name: "ProjectInAnotherOrganization", patch: patch(asNewChat, `"project_id":"$FOREIGN_PROJECT"`), want: bad("project_id", "not in this organization"), reference: true},
	}
	// The detail of each is the error text of cron.Standard.
	for _, spec := range []string{"0 9 L * *"} {
		_, err := cron.Standard(spec, "UTC")
		require.Error(t, err)
		rows = append(rows, rejection{name: "Cron " + spec, patch: patch(asSchedule, `"schedule_cron":"`+spec+`"`), want: bad("schedule_cron", err.Error())})
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
			h := newHarness(t, db, testLimits)

			_, err := h.creator.Create(ctx, actor, createParams(f.org.ID, req))

			var invalid *chatautomation.InvalidRequestError
			require.ErrorAs(t, err, &invalid)
			require.Len(t, invalid.Validations, len(tc.want))
			for i, want := range tc.want {
				require.Equal(t, want.Field, invalid.Validations[i].Field)
				require.Contains(t, invalid.Validations[i].Detail, want.Detail)
				require.NotContains(t, invalid.Validations[i].Detail, canary)
			}
			calls := h.rec.names()
			require.NotContains(t, calls, "InTx")
			require.NotContains(t, calls, "InsertChatAutomation")
			if !tc.reference {
				require.Empty(t, calls)
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
		a, b := newFixture(t, db, nil), newFixture(t, db, nil)
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
		f, other := newFixture(t, db, nil), newFixture(t, db, nil)
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: f.owner.ID, OrganizationID: other.org.ID})
		first, second, actor := newHarness(t, db, limits), newHarness(t, db, limits), userActor(t, db, f.owner.ID)
		_, err := second.creator.Create(ctx, actor, createParams(f.org.ID, webhookToNewChat(t, f)))
		require.NoError(t, err)
		counted, release, results := make(chan struct{}, 1), make(chan struct{}), make(chan error, 2)
		first.rec.afterCount = func() {
			counted <- struct{}{}
			<-release
		}
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

	// Nothing runs in a deleted organization and its owners cannot list or
	// delete its automations, so they do not count against the cap.
	t.Run("DeletedOrganization", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		a, b := newFixture(t, db, nil), newFixture(t, db, nil)
		h, actor := newHarness(t, db, limits), userActor(t, db, a.owner.ID)
		for range 2 {
			dbgen.ChatAutomation(t, db, database.ChatAutomation{OrganizationID: b.org.ID, OwnerID: a.owner.ID})
		}
		_, err := h.creator.Create(ctx, actor, createParams(a.org.ID, webhookToNewChat(t, a)))
		var limitErr *chatautomation.LimitError
		require.ErrorAs(t, err, &limitErr)

		require.NoError(t, db.UpdateOrganizationDeletedByID(ctx, database.UpdateOrganizationDeletedByIDParams{ID: b.org.ID, UpdatedAt: dbtime.Now()}))

		_, err = h.creator.Create(ctx, actor, createParams(a.org.ID, webhookToNewChat(t, a)))
		require.NoError(t, err)
	})
}

// TestCreateOwner creates as a hand-built subject that passes the create
// check, so that the owner guard of the insert is what refuses a system or
// deleted owner.
func TestCreateOwner(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)

	for _, tc := range []struct {
		name  string
		owner func(ctx context.Context, t *testing.T, f fixture) uuid.UUID
		want  error
	}{
		{name: "Active", owner: func(_ context.Context, _ *testing.T, f fixture) uuid.UUID { return f.owner.ID }},
		{name: "System", want: chatautomation.ErrOwnerUnavailable, owner: func(context.Context, *testing.T, fixture) uuid.UUID { return database.PrebuildsSystemUserID }},
		{
			name: "Deleted", want: chatautomation.ErrOwnerUnavailable,
			owner: func(ctx context.Context, t *testing.T, f fixture) uuid.UUID {
				require.NoError(t, db.UpdateUserDeletedByID(ctx, f.owner.ID))
				return f.owner.ID
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			f := newFixture(t, db, nil)
			owner := tc.owner(ctx, t, f)
			actor := rbac.Subject{
				Type: rbac.SubjectTypeUser, ID: owner.String(), Scope: rbac.ScopeAll,
				Roles: rbac.Roles{{
					Identifier: rbac.RoleIdentifier{Name: "automation-creator"},
					Site: rbac.Permissions(map[string][]policy.Action{
						rbac.ResourceChatAutomation.Type:  {policy.ActionCreate},
						rbac.ResourceChatModelConfig.Type: {policy.ActionRead},
					}),
				}},
			}

			_, err := newHarness(t, db, testLimits).creator.Create(ctx, actor, createParams(f.org.ID, webhookToNewChat(t, f)))

			count, countErr := db.CountChatAutomationsByOwnerID(ctx, owner)
			require.NoError(t, countErr)
			if tc.want == nil {
				require.NoError(t, err)
				require.EqualValues(t, 1, count)
				return
			}
			require.ErrorIs(t, err, tc.want)
			require.Zero(t, count)
		})
	}
}

// TestCreateAuthorization checks the actor before anything else: the store sees
// no call, even for a request that is also invalid.
func TestCreateAuthorization(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	// Members of this organization hold no agents-access role by default.
	org := dbgen.Organization(t, db, database.Organization{DefaultOrgMemberRoles: []string{rbac.RoleOrgWorkspaceAccess()}})
	f := fixture{db: db, org: org, model: dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID})}

	for _, tc := range []struct {
		name  string
		roles []string
		// invalid breaks a field rule of the request.
		invalid bool
	}{
		{name: "MemberWithoutAgentsAccess", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			req := webhookToNewChat(t, f)
			if tc.invalid {
				req.Name = ""
			}
			h := newHarness(t, db, testLimits)

			_, err := h.creator.Create(ctx, userActor(t, db, member(t, db, org.ID, tc.roles...).ID), createParams(org.ID, req))

			require.ErrorIs(t, err, chatautomation.ErrCreateForbidden)
			require.Empty(t, h.rec.names())
		})
	}

	t.Run("UnwrappedStore", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		valid := newFixture(t, db, nil)
		creator := chatautomation.NewCreator(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), testLimits)

		_, err := creator.Create(ctx, userActor(t, db, valid.owner.ID), createParams(valid.org.ID, webhookToNewChat(t, valid)))

		require.ErrorContains(t, err, "dbauthz")
		count, err := db.CountChatAutomationsByOwnerID(ctx, valid.owner.ID)
		require.NoError(t, err)
		require.Zero(t, count)
	})
}

// TestCreateCreatingChat sets the creating chat that the agent tool passes. The
// API never sets it, so these are errors for the tool and not field violations.
func TestCreateCreatingChat(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	f, other := newFixture(t, db, nil), newFixture(t, db, nil)
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: f.owner.ID, OrganizationID: other.org.ID})
	actor := userActor(t, db, f.owner.ID)
	stranger := dbgen.Chat(t, db, database.Chat{OrganizationID: f.org.ID, OwnerID: member(t, db, f.org.ID).ID, LastModelConfigID: f.model.ID})
	foreign := dbgen.Chat(t, db, database.Chat{OrganizationID: other.org.ID, OwnerID: f.owner.ID, LastModelConfigID: other.model.ID})

	for _, tc := range []struct {
		name  string
		chat  uuid.UUID
		check func(t *testing.T, err error)
	}{
		// The insert would report a missing chat as a missing owner.
		{name: "Missing", chat: uuid.New(), check: func(t *testing.T, err error) {
			require.ErrorIs(t, err, sql.ErrNoRows)
			require.NotErrorIs(t, err, chatautomation.ErrOwnerUnavailable)
		}},
		{name: "OfAnotherUser", chat: stranger.ID, check: func(t *testing.T, err error) { require.ErrorIs(t, err, chatautomation.ErrCreateForbidden) }},
		{name: "InAnotherOrganization", chat: foreign.ID, check: func(t *testing.T, err error) {
			require.ErrorContains(t, err, "not in this organization")
			require.NotErrorIs(t, err, chatautomation.ErrCreateForbidden)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			p := createParams(f.org.ID, webhookToNewChat(t, f))
			p.CreatedByChatID = nullID(tc.chat)
			h := newHarness(t, db, testLimits)

			_, err := h.creator.Create(ctx, actor, p)

			tc.check(t, err)
			require.NotContains(t, h.rec.names(), "InsertChatAutomation")
		})
	}
}

// TestCreateTargetOwnership gives the actor create and update on every chat of
// the organization, as an organization admin who also holds agents-access has,
// so that only the ownership rule can refuse another user's chat.
func TestCreateTargetOwnership(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	f := newFixture(t, db, nil)
	admin := member(t, db, f.org.ID, rbac.ScopedRoleOrgAdmin(f.org.ID).Name)
	actor := userActor(t, db, admin.ID)
	toChat := func(chatID uuid.UUID) codersdk.CreateChatAutomationRequest {
		req := webhookToNewChat(t, f)
		req.TargetMode, req.NewChatModelConfigID, req.TargetChatID = codersdk.ChatAutomationTargetModeExistingChat, nil, &chatID
		return req
	}

	t.Run("ChatOfAnotherUser", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		h := newHarness(t, db, testLimits)

		_, err := h.creator.Create(ctx, actor, createParams(f.org.ID, toChat(f.chat(t).ID)))

		var invalid *chatautomation.InvalidRequestError
		require.ErrorAs(t, err, &invalid)
		require.Len(t, invalid.Validations, 1)
		require.Equal(t, "target_chat_id", invalid.Validations[0].Field)
		require.Contains(t, invalid.Validations[0].Detail, "Only the owner of a chat")
		require.NotContains(t, h.rec.names(), "InsertChatAutomation")
		count, err := db.CountChatAutomationsByOwnerID(ctx, admin.ID)
		require.NoError(t, err)
		require.Zero(t, count)
	})

	t.Run("ChatOfTheOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		own := dbgen.Chat(t, db, database.Chat{OrganizationID: f.org.ID, OwnerID: admin.ID, LastModelConfigID: f.model.ID})

		res, err := newHarness(t, db, testLimits).creator.Create(ctx, actor, createParams(f.org.ID, toChat(own.ID)))

		require.NoError(t, err)
		require.Equal(t, own.ID, res.Automation.TargetChatID.UUID)
	})
}

// TestCreateScheduleNeedsNow checks that an enabled schedule is refused when
// the caller passes no time for its first run. The API always passes one, and
// the agent tool is the other caller.
func TestCreateScheduleNeedsNow(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	f := newFixture(t, db, nil)
	req := webhookToNewChat(t, f)
	req.Kind, req.WebhookAuth, req.ScheduleCron, req.ScheduleTimeZone = codersdk.ChatAutomationKindSchedule, "", "0 9 * * *", "UTC"
	p := createParams(f.org.ID, req)
	p.Now = time.Time{}

	_, err := newHarness(t, db, testLimits).creator.Create(testutil.Context(t, testutil.WaitMedium), userActor(t, db, f.owner.ID), p)

	require.ErrorContains(t, err, "no time for the first run")
	var invalid *chatautomation.InvalidRequestError
	require.NotErrorAs(t, err, &invalid)
}

// TestCreateReferenceDeleted deletes the target chat after Create has checked
// it, so that the insert is what finds it missing. The owner is fine, so the
// answer must not say that the owner is unavailable.
func TestCreateReferenceDeleted(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	f := newFixture(t, db, nil)
	target := f.chat(t)
	req := webhookToNewChat(t, f)
	req.TargetMode, req.NewChatModelConfigID, req.TargetChatID = codersdk.ChatAutomationTargetModeExistingChat, nil, &target.ID
	h := newHarness(t, db, testLimits)
	h.rec.afterCount = func() {
		_, err := sqlDB.ExecContext(ctx, "DELETE FROM chats WHERE id = $1", target.ID)
		require.NoError(t, err)
	}

	_, err := h.creator.Create(ctx, userActor(t, db, f.owner.ID), createParams(f.org.ID, req))

	require.ErrorIs(t, err, chatautomation.ErrReferenceUnavailable)
	require.NotErrorIs(t, err, chatautomation.ErrOwnerUnavailable)
	require.Contains(t, h.rec.names(), "InsertChatAutomation")
}
