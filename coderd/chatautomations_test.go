package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	experimentrules "github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// chatAutomationEnv is a deployment with chat-automations on for everyone.
type chatAutomationEnv struct {
	owner   *codersdk.Client
	first   codersdk.CreateFirstUserResponse
	api     *coderd.API
	db      database.Store
	sqlDB   *sql.DB
	auditor *audit.MockAuditor
	model   database.ChatModelConfig
}

func newChatAutomationEnv(t *testing.T, wrap func(database.Store) database.Store) chatAutomationEnv {
	t.Helper()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	store := db
	if wrap != nil {
		store = wrap(db)
	}
	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentChatAutomations)}
	auditor := audit.NewMock()
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:   values,
		Database:           store,
		Pubsub:             ps,
		Auditor:            auditor,
		ChatWorkerDisabled: true,
	})
	first := coderdtest.CreateFirstUser(t, client)
	return chatAutomationEnv{
		owner: client, first: first, api: api, db: db, sqlDB: sqlDB, auditor: auditor,
		model: dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: first.OrganizationID}),
	}
}

func (e chatAutomationEnv) chat(t *testing.T, ownerID uuid.UUID, title string) database.Chat {
	t.Helper()
	return dbgen.Chat(t, e.db, database.Chat{
		OrganizationID: e.first.OrganizationID, OwnerID: ownerID, LastModelConfigID: e.model.ID, Title: title,
	})
}

func (e chatAutomationEnv) automation(t *testing.T, seed database.ChatAutomation) database.ChatAutomation {
	t.Helper()
	if seed.OrganizationID == uuid.Nil {
		seed.OrganizationID = e.first.OrganizationID
	}
	return dbgen.ChatAutomation(t, e.db, seed)
}

func (e chatAutomationEnv) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	_, err := e.sqlDB.ExecContext(testutil.Context(t, testutil.WaitShort), query, args...)
	require.NoError(t, err)
}

func (e chatAutomationEnv) exists(t *testing.T, id uuid.UUID) bool {
	t.Helper()
	var n int
	require.NoError(t, e.sqlDB.QueryRowContext(testutil.Context(t, testutil.WaitShort),
		"SELECT count(*) FROM chat_automations WHERE id = $1", id).Scan(&n))
	return n == 1
}

func chatAutomationPath(orgID uuid.UUID, suffix string) string {
	return fmt.Sprintf("/api/experimental/organizations/%s/chats/automations%s", orgID, suffix)
}

// doChatAutomation sends a raw request so that tests can read the bytes the
// server wrote.
func doChatAutomation(t *testing.T, client *codersdk.Client, method, path string) (int, []byte) {
	t.Helper()
	res, err := client.Request(testutil.Context(t, testutil.WaitMedium), method, path, nil)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, body
}

func listChatAutomationIDs(t *testing.T, client *codersdk.Client, orgID uuid.UUID, query string) []uuid.UUID {
	t.Helper()
	status, body := doChatAutomation(t, client, http.MethodGet, chatAutomationPath(orgID, query))
	require.Equal(t, http.StatusOK, status, string(body))
	require.True(t, bytes.HasPrefix(bytes.TrimSpace(body), []byte("[")), "list must be an array: %s", body)
	var list []codersdk.ChatAutomation
	require.NoError(t, json.Unmarshal(body, &list))
	ids := make([]uuid.UUID, 0, len(list))
	for _, a := range list {
		ids = append(ids, a.ID)
	}
	return ids
}

// failingRulesStore fails the experiment rule read for requests of one user.
type failingRulesStore struct {
	database.Store
	failFor *atomic.Pointer[uuid.UUID]
}

func (s failingRulesStore) GetExperimentRules(ctx context.Context) ([]database.GetExperimentRulesRow, error) {
	key, ok := httpmw.APIKeyOptional((&http.Request{}).WithContext(ctx))
	if id := s.failFor.Load(); ok && id != nil && key.UserID == *id {
		return nil, xerrors.New("experiment rules are unavailable")
	}
	return s.Store.GetExperimentRules(ctx)
}

func TestChatAutomationsExperimentGate(t *testing.T) {
	t.Parallel()

	// requireGated checks that every route answers 404 to client.
	requireGated := func(t *testing.T, client *codersdk.Client, orgID, id uuid.UUID) {
		t.Helper()
		for _, req := range []struct{ method, suffix string }{
			{http.MethodGet, ""},
			{http.MethodPost, ""},
			{http.MethodGet, "/" + id.String()},
			{http.MethodDelete, "/" + id.String()},
		} {
			status, body := doChatAutomation(t, client, req.method, chatAutomationPath(orgID, req.suffix))
			require.Equal(t, http.StatusNotFound, status, "%s %q: %s", req.method, req.suffix, body)
		}
	}

	t.Run("OffForCaller", func(t *testing.T) {
		t.Parallel()
		env := newChatAutomationEnv(t, nil)
		offClient, off := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
		onClient, on := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
		offAutomation := env.automation(t, database.ChatAutomation{OwnerID: off.ID})
		onAutomation := env.automation(t, database.ChatAutomation{OwnerID: on.ID})

		ctx := testutil.Context(t, testutil.WaitShort)
		_, _, _, err := experimentrules.WriteRule(dbauthz.AsSystemRestricted(ctx), env.api.Database, env.first.UserID,
			codersdk.ExperimentChatAutomations, experimentrules.Rule{
				Mode:      experimentrules.ModeCondition,
				Condition: fmt.Sprintf("user.username != %q", off.Username),
			}, 0)
		require.NoError(t, err)

		requireGated(t, offClient, env.first.OrganizationID, offAutomation.ID)
		require.True(t, env.exists(t, offAutomation.ID), "a gated delete must not delete")
		require.Equal(t, []uuid.UUID{onAutomation.ID}, listChatAutomationIDs(t, onClient, env.first.OrganizationID, ""))
	})

	t.Run("EvaluationError", func(t *testing.T) {
		t.Parallel()
		failFor := &atomic.Pointer[uuid.UUID]{}
		env := newChatAutomationEnv(t, func(db database.Store) database.Store {
			return failingRulesStore{Store: db, failFor: failFor}
		})
		failClient, failing := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
		otherClient, other := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
		failAutomation := env.automation(t, database.ChatAutomation{OwnerID: failing.ID})
		otherAutomation := env.automation(t, database.ChatAutomation{OwnerID: other.ID})
		// The static list turns the experiment on before the read fails.
		require.Equal(t, []uuid.UUID{failAutomation.ID}, listChatAutomationIDs(t, failClient, env.first.OrganizationID, ""))

		failFor.Store(&failing.ID)
		requireGated(t, failClient, env.first.OrganizationID, failAutomation.ID)
		require.True(t, env.exists(t, failAutomation.ID))
		require.Equal(t, []uuid.UUID{otherAutomation.ID}, listChatAutomationIDs(t, otherClient, env.first.OrganizationID, ""))
	})
}

func TestChatAutomationsOwnerLifecycle(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	client, member := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
	org := env.first.OrganizationID

	status, body := doChatAutomation(t, client, http.MethodGet, chatAutomationPath(org, ""))
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "[]", string(bytes.TrimSpace(body)))

	older := env.automation(t, database.ChatAutomation{OwnerID: member.ID})
	newer := env.automation(t, database.ChatAutomation{OwnerID: member.ID})
	env.exec(t, "UPDATE chat_automations SET created_at = now() - interval '2 hours' WHERE id = $1", older.ID)
	env.exec(t, "UPDATE chat_automations SET created_at = now() - interval '1 hour' WHERE id = $1", newer.ID)
	require.Equal(t, []uuid.UUID{newer.ID, older.ID}, listChatAutomationIDs(t, client, org, ""))

	got, err := codersdk.NewExperimentalClient(client).ChatAutomation(testutil.Context(t, testutil.WaitShort), org, older.ID)
	require.NoError(t, err)
	require.Equal(t, older.ID, got.ID)

	one := "/" + older.ID.String()
	for _, step := range []struct {
		method string
		want   int
	}{
		{http.MethodDelete, http.StatusNoContent},
		{http.MethodDelete, http.StatusNotFound},
		{http.MethodGet, http.StatusNotFound},
	} {
		status, body := doChatAutomation(t, client, step.method, chatAutomationPath(org, one))
		require.Equal(t, step.want, status, "%s: %s", step.method, body)
	}
	require.False(t, env.exists(t, older.ID))
	require.Equal(t, []uuid.UUID{newer.ID}, listChatAutomationIDs(t, client, org, ""))
}

func TestChatAutomationsAuthorization(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	org := env.first.OrganizationID
	// Without the default, agents-access reaches only the members that
	// hold it explicitly, so one member of the organization can lack it.
	env.exec(t, "UPDATE organizations SET default_org_member_roles = '{organization-workspace-access}' WHERE id = $1", org)
	agentsAccess := rbac.ScopedRoleAgentsAccess(org)
	memberClient, member := coderdtest.CreateAnotherUser(t, env.owner, org, agentsAccess)
	_, other := coderdtest.CreateAnotherUser(t, env.owner, org, agentsAccess)
	noAccessClient, noAccess := coderdtest.CreateAnotherUser(t, env.owner, org)
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, env.owner, org, rbac.ScopedRoleOrgAdmin(org))
	auditorClient, _ := coderdtest.CreateAnotherUser(t, env.owner, org, rbac.RoleAuditor())
	userAdminClient, _ := coderdtest.CreateAnotherUser(t, env.owner, org, rbac.RoleUserAdmin())
	templateAdminClient, _ := coderdtest.CreateAnotherUser(t, env.owner, org, rbac.RoleTemplateAdmin())
	token, err := memberClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		// organization:read lets the request reach the handler, whose SQL
		// filter then drops every automation.
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead, codersdk.APIKeyScopeOrganizationRead},
	})
	require.NoError(t, err)
	scopedClient := codersdk.New(env.owner.URL, codersdk.WithSessionToken(token.Key))

	otherOrg := dbgen.Organization(t, env.db, database.Organization{})
	_, otherOrgMember := coderdtest.CreateAnotherUser(t, env.owner, otherOrg.ID)
	dbgen.OrganizationMember(t, env.db, database.OrganizationMember{UserID: member.ID, OrganizationID: otherOrg.ID})

	// The keys name the owners of the automations that each row reaches.
	owners := map[string]database.ChatAutomation{
		"member":    {OwnerID: member.ID},
		"other":     {OwnerID: other.ID},
		"no_agents": {OwnerID: noAccess.ID},
		"other_org": {OwnerID: otherOrgMember.ID, OrganizationID: otherOrg.ID},
	}
	shared := map[string]database.ChatAutomation{}
	for key, seed := range owners {
		shared[key] = env.automation(t, seed)
	}

	rows := map[string]struct {
		client *codersdk.Client
		reach  []string
	}{
		"Member":            {client: memberClient, reach: []string{"member"}},
		"OrganizationAdmin": {client: orgAdminClient, reach: []string{"member", "other", "no_agents"}},
		"DeploymentOwner":   {client: env.owner, reach: []string{"member", "other", "no_agents", "other_org"}},
		"MemberNoAgents":    {client: noAccessClient},
		"Auditor":           {client: auditorClient},
		"UserAdmin":         {client: userAdminClient},
		"TemplateAdmin":     {client: templateAdminClient},
		"ScopedToken":       {client: scopedClient},
	}
	// Reads finish before any delete seeds a copy that a list would show.
	t.Run("Read", func(t *testing.T) { //nolint:paralleltest // The phases run in order.
		for name, tc := range rows {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				wantList := []uuid.UUID{}
				for _, key := range []string{"member", "other", "no_agents"} {
					if slices.Contains(tc.reach, key) {
						wantList = append(wantList, shared[key].ID)
					}
				}
				require.ElementsMatch(t, wantList, listChatAutomationIDs(t, tc.client, org, ""))
				for key, a := range shared {
					want := http.StatusNotFound
					if slices.Contains(tc.reach, key) {
						want = http.StatusOK
					}
					status, body := doChatAutomation(t, tc.client, http.MethodGet, chatAutomationPath(a.OrganizationID, "/"+a.ID.String()))
					require.Equal(t, want, status, "get %s: %s", key, body)
				}
			})
		}
	})
	t.Run("Delete", func(t *testing.T) { //nolint:paralleltest // The phases run in order.
		for name, tc := range rows {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				// Each row deletes its own copies.
				for key, seed := range owners {
					a := env.automation(t, seed)
					want := http.StatusNotFound
					if slices.Contains(tc.reach, key) {
						want = http.StatusNoContent
					}
					status, body := doChatAutomation(t, tc.client, http.MethodDelete, chatAutomationPath(a.OrganizationID, "/"+a.ID.String()))
					require.Equal(t, want, status, "delete %s: %s", key, body)
					require.Equal(t, !slices.Contains(tc.reach, key), env.exists(t, a.ID), "delete %s", key)
				}
			})
		}
		t.Run("WrongOrganizationPath", func(t *testing.T) {
			t.Parallel()
			a := env.automation(t, database.ChatAutomation{OwnerID: member.ID})
			for _, client := range []*codersdk.Client{memberClient, env.owner} {
				for _, method := range []string{http.MethodGet, http.MethodDelete} {
					status, body := doChatAutomation(t, client, method, chatAutomationPath(otherOrg.ID, "/"+a.ID.String()))
					require.Equal(t, http.StatusNotFound, status, "%s: %s", method, body)
				}
			}
			require.True(t, env.exists(t, a.ID))
		})
	})
}

func TestChatAutomationsOwnerFilter(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	org := env.first.OrganizationID
	memberClient, member := coderdtest.CreateAnotherUser(t, env.owner, org)
	_, other := coderdtest.CreateAnotherUser(t, env.owner, org)
	adminClient, _ := coderdtest.CreateAnotherUser(t, env.owner, org, rbac.ScopedRoleOrgAdmin(org))
	mine := env.automation(t, database.ChatAutomation{OwnerID: member.ID})
	theirs := env.automation(t, database.ChatAutomation{OwnerID: other.ID})

	for name, tc := range map[string]struct {
		client *codersdk.Client
		owner  string
		want   []uuid.UUID
	}{
		"Me":               {memberClient, codersdk.Me, []uuid.UUID{mine.ID}},
		"UserID":           {memberClient, member.ID.String(), []uuid.UUID{mine.ID}},
		"Username":         {memberClient, member.Username, []uuid.UUID{mine.ID}},
		"UsernameUpper":    {memberClient, strings.ToUpper(member.Username), []uuid.UUID{mine.ID}},
		"UnknownUsername":  {memberClient, "no-such-user-" + uuid.NewString()[:8], nil},
		"NilUUID":          {adminClient, uuid.Nil.String(), nil},
		"Email":            {adminClient, member.Email, nil},
		"AdminByUsername":  {adminClient, member.Username, []uuid.UUID{mine.ID}},
		"AdminEveryOwner":  {adminClient, "", []uuid.UUID{mine.ID, theirs.ID}},
		"MemberOtherOwner": {memberClient, other.ID.String(), nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			query := ""
			if tc.owner != "" {
				query = "?" + url.Values{"owner": {tc.owner}}.Encode()
			}
			require.ElementsMatch(t, tc.want, listChatAutomationIDs(t, tc.client, org, query))
		})
	}
}

// wantChatAutomation builds the expected response from a row. chats holds
// the chats that the caller can read.
func wantChatAutomation(a database.ChatAutomation, owner *codersdk.MinimalUser, chats map[uuid.UUID]database.Chat, next []time.Time) codersdk.ChatAutomation {
	id := func(n uuid.NullUUID) *uuid.UUID {
		if !n.Valid {
			return nil
		}
		return &n.UUID
	}
	str := func(n sql.NullString) *string {
		if !n.Valid {
			return nil
		}
		return &n.String
	}
	chat := func(n uuid.NullUUID) *codersdk.ChatAutomationChat {
		c, ok := chats[n.UUID]
		if !n.Valid || !ok {
			return nil
		}
		return &codersdk.ChatAutomationChat{ID: c.ID, Title: c.Title, Archived: c.Archived}
	}
	want := codersdk.ChatAutomation{
		ID: a.ID, OrganizationID: a.OrganizationID, OwnerID: a.OwnerID, Owner: owner, Name: a.Name,
		Kind: codersdk.ChatAutomationKind(a.Kind), Enabled: a.Enabled, TargetMode: codersdk.ChatAutomationTargetMode(a.TargetMode),
		TargetChatID: id(a.TargetChatID), TargetChat: chat(a.TargetChatID),
		CreatedByChatID: id(a.CreatedByChatID), CreatedByChat: chat(a.CreatedByChatID),
		NewChatModelConfigID: id(a.NewChatModelConfigID), ProjectID: id(a.ProjectID),
		WebhookSecretVersion: a.WebhookSecretVersion, Prompt: a.Prompt,
		ScheduleCron: str(a.ScheduleCron), ScheduleTimeZone: str(a.ScheduleTimeZone),
		NextRunTimes: next, PausedReasons: []codersdk.ChatAutomationPausedReason{},
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
	if a.ScheduleNextRunAt.Valid {
		want.ScheduleNextRunAt = &a.ScheduleNextRunAt.Time
	}
	if a.WhenBusy.Valid {
		v := codersdk.ChatAutomationWhenBusy(a.WhenBusy.ChatAutomationWhenBusy)
		want.WhenBusy = &v
	}
	if a.ReasoningEffort.Valid {
		v := string(a.ReasoningEffort.ChatReasoningEffort)
		want.ReasoningEffort = &v
	}
	if a.WebhookAuth.Valid {
		v := codersdk.ChatAutomationWebhookAuth(a.WebhookAuth.ChatAutomationWebhookAuth)
		want.WebhookAuth = &v
	}
	if a.LastRunAt.Valid {
		want.LastRun = &codersdk.ChatAutomationLastRun{
			At: a.LastRunAt.Time, Outcome: codersdk.ChatAutomationRunOutcome(a.LastRunOutcome.ChatAutomationRunOutcome),
			Reason: str(a.LastRunReason), ChatID: id(a.LastRunChatID),
		}
	}
	return want
}

func TestChatAutomationsResponse(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	org := env.first.OrganizationID
	memberClient, member := coderdtest.CreateAnotherUser(t, env.owner, org)
	_, other := coderdtest.CreateAnotherUser(t, env.owner, org)
	adminClient, _ := coderdtest.CreateAnotherUser(t, env.owner, org, rbac.ScopedRoleOrgAdmin(org))
	owner := &codersdk.MinimalUser{ID: member.ID, Username: member.Username, Name: member.Name, AvatarURL: member.AvatarURL}

	target := env.chat(t, member.ID, "Target chat")
	creator := env.chat(t, member.ID, "Creating chat")
	env.exec(t, "UPDATE chats SET archived = true WHERE id = $1", creator.ID)
	creator.Archived = true
	readable := map[uuid.UUID]database.Chat{target.ID: target, creator.ID: creator}
	project := dbgen.ChatProject(t, env.db, database.ChatProject{OrganizationID: org, OwnerID: member.ID})
	valid := func(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }
	schedule := func(zone string, enabled bool) database.ChatAutomation {
		return env.automation(t, database.ChatAutomation{
			OwnerID: member.ID, Kind: database.ChatAutomationKindSchedule, Enabled: enabled,
			ScheduleCron:     sql.NullString{String: "0 9 * * *", Valid: true},
			ScheduleTimeZone: sql.NullString{String: zone, Valid: true},
		})
	}

	webhook := env.automation(t, database.ChatAutomation{
		OwnerID: member.ID, Enabled: true, Prompt: "Summarize the pull request.",
		TargetChatID: valid(target.ID), CreatedByChatID: valid(creator.ID),
		WhenBusy: database.NullChatAutomationWhenBusy{ChatAutomationWhenBusy: database.ChatAutomationWhenBusySkip, Valid: true},
	})
	// The member cannot read the chat of another member.
	otherChat := env.chat(t, other.ID, "Other member chat")
	unreadableChats := env.automation(t, database.ChatAutomation{
		OwnerID: member.ID, TargetChatID: valid(otherChat.ID), CreatedByChatID: valid(otherChat.ID),
	})
	lastRun := env.automation(t, database.ChatAutomation{OwnerID: member.ID})
	env.exec(t, `UPDATE chat_automations SET last_run_at = now() - interval '5 minutes', last_run_outcome = 'skipped',
		last_run_reason = 'target chat busy', last_run_chat_id = $1, last_run_unaccepted_count = 3 WHERE id = $2`, target.ID, lastRun.ID)
	newChat := env.automation(t, database.ChatAutomation{
		OwnerID: member.ID, TargetMode: database.ChatAutomationTargetModeNewChat,
		NewChatModelConfigID: valid(env.model.ID), ProjectID: valid(project.ID),
		ReasoningEffort: database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffortHigh, Valid: true},
	})
	// The allow list reaches the organization and this automation only, so
	// the caller reads the automation but neither its owner nor its chats.
	narrow, err := memberClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{AllowList: []codersdk.APIAllowListTarget{
		{Type: codersdk.ResourceOrganization, ID: org.String()},
		{Type: codersdk.ResourceWildcard, ID: webhook.ID.String()},
	}})
	require.NoError(t, err)
	narrowClient := codersdk.New(env.owner.URL, codersdk.WithSessionToken(narrow.Key))

	for name, tc := range map[string]struct {
		automation database.ChatAutomation
		client     *codersdk.Client
		owner      *codersdk.MinimalUser
		chats      map[uuid.UUID]database.Chat
		scheduled  bool
	}{
		"Webhook":             {automation: webhook, client: memberClient, owner: owner, chats: readable},
		"WebhookAsOrgAdmin":   {automation: webhook, client: adminClient, owner: owner, chats: readable},
		"UnreadableChats":     {automation: unreadableChats, client: memberClient, owner: owner, chats: readable},
		"OwnerUnreadable":     {automation: webhook, client: narrowClient},
		"LastRun":             {automation: lastRun, client: memberClient, owner: owner, chats: readable},
		"NewChat":             {automation: newChat, client: memberClient, owner: owner},
		"ScheduleEnabled":     {automation: schedule("America/New_York", true), client: memberClient, owner: owner, scheduled: true},
		"ScheduleDisabled":    {automation: schedule("America/New_York", false), client: memberClient, owner: owner},
		"ScheduleUnknownZone": {automation: schedule("Not/AZone", true), client: memberClient, owner: owner},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			//nolint:gocritic // The test reads the stored row to build the expectation.
			row, err := env.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitShort)), tc.automation.ID)
			require.NoError(t, err)

			before := time.Now()
			status, body := doChatAutomation(t, tc.client, http.MethodGet, chatAutomationPath(org, "/"+row.ID.String()))
			after := time.Now()
			require.Equal(t, http.StatusOK, status, string(body))
			require.Regexp(t, `"paused_reasons":\s*\[\]`, string(body))
			var got codersdk.ChatAutomation
			require.NoError(t, json.Unmarshal(body, &got))

			next := []time.Time{}
			if tc.scheduled {
				sched, err := cron.Standard("0 9 * * *", "America/New_York")
				require.NoError(t, err)
				require.Len(t, got.NextRunTimes, 5)
				require.True(t, got.NextRunTimes[0].After(before))
				require.False(t, got.NextRunTimes[0].Before(sched.Next(before)))
				require.False(t, got.NextRunTimes[0].After(sched.Next(after)))
				for i := 1; i < len(got.NextRunTimes); i++ {
					require.True(t, got.NextRunTimes[i].After(got.NextRunTimes[i-1]))
					require.True(t, got.NextRunTimes[i].Equal(sched.Next(got.NextRunTimes[i-1])), "run %d", i)
				}
				next = got.NextRunTimes
			} else {
				require.Regexp(t, `"next_run_times":\s*\[\]`, string(body))
			}

			wantJSON, err := json.Marshal(wantChatAutomation(row, tc.owner, tc.chats, next))
			require.NoError(t, err)
			var want, gotMap map[string]any
			require.NoError(t, json.Unmarshal(wantJSON, &want))
			require.NoError(t, json.Unmarshal(body, &gotMap))
			require.Equal(t, want, gotMap)
			if row.LastRunAt.Valid {
				lastRunMap, ok := gotMap["last_run"].(map[string]any)
				require.True(t, ok)
				require.ElementsMatch(t, []string{"at", "outcome", "reason", "chat_id"}, slices.Collect(maps.Keys(lastRunMap)))
			}
		})
	}
}

func TestChatAutomationsSecretNotReturned(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	org := env.first.OrganizationID
	memberClient, member := coderdtest.CreateAnotherUser(t, env.owner, org)
	adminClient, _ := coderdtest.CreateAnotherUser(t, env.owner, org, rbac.ScopedRoleOrgAdmin(org))
	strangerClient, _ := coderdtest.CreateAnotherUser(t, env.owner, org)
	secret := "canary-secret-" + strings.Repeat(uuid.NewString(), 2)
	prompt := "canary-prompt-" + uuid.NewString()
	a := env.automation(t, database.ChatAutomation{
		OwnerID: member.ID, Prompt: prompt, WebhookSecret: sql.NullString{String: secret, Valid: true},
	})
	one := chatAutomationPath(org, "/"+a.ID.String())

	for name, tc := range map[string]struct {
		client *codersdk.Client
		method string
		path   string
		status int
		prompt bool
	}{
		"OwnerGet":       {memberClient, http.MethodGet, one, http.StatusOK, true},
		"OwnerList":      {memberClient, http.MethodGet, chatAutomationPath(org, ""), http.StatusOK, true},
		"AdminList":      {adminClient, http.MethodGet, chatAutomationPath(org, ""), http.StatusOK, true},
		"StrangerGet":    {strangerClient, http.MethodGet, one, http.StatusNotFound, false},
		"StrangerDelete": {strangerClient, http.MethodDelete, one, http.StatusNotFound, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			status, body := doChatAutomation(t, tc.client, tc.method, tc.path)
			require.Equal(t, tc.status, status, string(body))
			require.Equal(t, 0, bytes.Count(body, []byte(secret)))
			require.Equal(t, tc.prompt, bytes.Contains(body, []byte(prompt)))
			require.Equal(t, tc.prompt, bytes.Contains(body, []byte(`"webhook_secret_version":`)))
		})
	}
}

func TestChatAutomationsDeleteAudit(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	org := env.first.OrganizationID
	client, member := coderdtest.CreateAnotherUser(t, env.owner, org)
	a := env.automation(t, database.ChatAutomation{OwnerID: member.ID})
	one := chatAutomationPath(org, "/"+a.ID.String())
	env.auditor.ResetLogs()

	status, body := doChatAutomation(t, client, http.MethodDelete, one)
	require.Equal(t, http.StatusNoContent, status, string(body))
	logs := env.auditor.AuditLogs()
	require.Len(t, logs, 1)
	require.Equal(t, database.AuditActionDelete, logs[0].Action)
	require.Equal(t, database.ResourceTypeChatAutomation, logs[0].ResourceType)
	require.Equal(t, a.ID, logs[0].ResourceID)
	require.Equal(t, org, logs[0].OrganizationID)
	require.EqualValues(t, http.StatusNoContent, logs[0].StatusCode)

	env.auditor.ResetLogs()
	for _, req := range []struct{ method, path string }{
		{http.MethodDelete, one},
		{http.MethodGet, one},
		{http.MethodGet, chatAutomationPath(org, "")},
	} {
		_, _ = doChatAutomation(t, client, req.method, req.path)
	}
	require.Empty(t, env.auditor.AuditLogs())
}
