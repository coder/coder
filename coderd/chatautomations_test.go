package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
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
	seed.OrganizationID = e.first.OrganizationID
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

// readOnlyClient returns a client of a new organization member whose only
// grant on chat automations is read, from a custom role. The custom role API
// needs a license, and the role store loads rows without one.
func (e chatAutomationEnv) readOnlyClient(t *testing.T) (*codersdk.Client, codersdk.User) {
	t.Helper()
	org := e.first.OrganizationID
	role, err := e.db.InsertCustomRole(testutil.Context(t, testutil.WaitShort), database.InsertCustomRoleParams{
		Name:           testutil.GetRandomName(t),
		DisplayName:    "Automation reader",
		OrganizationID: uuid.NullUUID{UUID: org, Valid: true},
		OrgPermissions: database.CustomRolePermissions{{
			ResourceType: rbac.ResourceChatAutomation.Type,
			Action:       policy.ActionRead,
		}},
	})
	require.NoError(t, err)
	client, user := coderdtest.CreateAnotherUser(t, e.owner, org)
	e.exec(t, "UPDATE organization_members SET roles = array_append(roles, $1) WHERE user_id = $2 AND organization_id = $3",
		role.Name, user.ID, org)
	return client, user
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

func TestChatAutomationsExperimentGate(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	client, user := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
	id := env.automation(t, database.ChatAutomation{OwnerID: user.ID}).ID
	ctx := testutil.Context(t, testutil.WaitShort)
	_, err := codersdk.NewExperimentalClient(env.owner).PutExperimentRule(ctx, codersdk.ExperimentChatAutomations, codersdk.PutExperimentRuleRequest{
		Mode:      codersdk.ExperimentRuleModeCondition,
		Condition: fmt.Sprintf("user.username != %q", user.Username),
	})
	require.NoError(t, err)

	for _, req := range []struct{ method, suffix string }{
		{http.MethodGet, ""},
		{http.MethodGet, "/" + id.String()},
		{http.MethodDelete, "/" + id.String()},
	} {
		status, body := doChatAutomation(t, client, req.method, chatAutomationPath(env.first.OrganizationID, req.suffix))
		require.Equal(t, http.StatusNotFound, status, "%s %q: %s", req.method, req.suffix, body)
	}
}

// failingRulesStore fails the read of the experiment rules.
type failingRulesStore struct{ database.Store }

func (failingRulesStore) GetExperimentRules(context.Context) ([]database.GetExperimentRulesRow, error) {
	return nil, xerrors.New("experiment rules are unavailable")
}

// TestChatAutomationsExperimentEvaluationError checks that the gate fails
// closed: the experiment is on in the static list, and the routes still answer
// 404 when the evaluator cannot read its rules.
func TestChatAutomationsExperimentEvaluationError(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, func(db database.Store) database.Store { return failingRulesStore{Store: db} })
	id := env.automation(t, database.ChatAutomation{OwnerID: env.first.UserID}).ID

	for _, req := range []struct{ method, suffix string }{
		{http.MethodGet, ""},
		{http.MethodGet, "/" + id.String()},
		{http.MethodDelete, "/" + id.String()},
	} {
		status, body := doChatAutomation(t, env.owner, req.method, chatAutomationPath(env.first.OrganizationID, req.suffix))
		require.Equal(t, http.StatusNotFound, status, "%s %q: %s", req.method, req.suffix, body)
	}
	require.True(t, env.exists(t, id))
}

func TestChatAutomationsOwnerLifecycle(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	client, member := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
	org := env.first.OrganizationID

	older := env.automation(t, database.ChatAutomation{OwnerID: member.ID})
	newer := env.automation(t, database.ChatAutomation{OwnerID: member.ID})
	env.exec(t, "UPDATE chat_automations SET created_at = now() - interval '2 hours' WHERE id = $1", older.ID)
	env.exec(t, "UPDATE chat_automations SET created_at = now() - interval '1 hour' WHERE id = $1", newer.ID)
	require.Equal(t, []uuid.UUID{newer.ID, older.ID}, listChatAutomationIDs(t, client, org, ""))

	got, err := codersdk.NewExperimentalClient(client).ChatAutomation(testutil.Context(t, testutil.WaitShort), org, older.ID)
	require.NoError(t, err)
	require.Equal(t, older.ID, got.ID)

	env.auditor.ResetLogs()
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
	// Only the delete that succeeded writes an entry, for the deleted row.
	logs := env.auditor.AuditLogs()
	require.Len(t, logs, 1)
	require.Equal(t, database.AuditActionDelete, logs[0].Action)
	require.Equal(t, older.ID, logs[0].ResourceID)
	require.Equal(t, org, logs[0].OrganizationID)
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
	token, err := memberClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
		// organization:read lets the request reach the handler, whose SQL
		// filter then drops every automation.
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead, codersdk.APIKeyScopeOrganizationRead},
	})
	require.NoError(t, err)
	scopedClient := codersdk.New(env.owner.URL, codersdk.WithSessionToken(token.Key))
	owners := map[string]uuid.UUID{"member": member.ID, "other": other.ID, "no_agents": noAccess.ID}

	for name, tc := range map[string]struct {
		client *codersdk.Client
		reach  []string
	}{
		"Member":            {client: memberClient, reach: []string{"member"}},
		"OrganizationAdmin": {client: orgAdminClient, reach: []string{"member", "other", "no_agents"}},
		"MemberNoAgents":    {client: noAccessClient},
		"Auditor":           {client: auditorClient},
		"ScopedToken":       {client: scopedClient},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Each row lists, reads and deletes its own copies.
			keys := map[uuid.UUID]string{}
			want := []uuid.UUID{}
			for key, ownerID := range owners {
				id := env.automation(t, database.ChatAutomation{OwnerID: ownerID}).ID
				keys[id] = key
				if slices.Contains(tc.reach, key) {
					want = append(want, id)
				}
			}
			listed := []uuid.UUID{}
			for _, id := range listChatAutomationIDs(t, tc.client, org, "") {
				if _, ok := keys[id]; ok {
					listed = append(listed, id)
				}
			}
			require.ElementsMatch(t, want, listed)
			for id, key := range keys {
				reach := slices.Contains(tc.reach, key)
				path := chatAutomationPath(org, "/"+id.String())
				status, body := doChatAutomation(t, tc.client, http.MethodGet, path)
				require.Equal(t, map[bool]int{true: http.StatusOK, false: http.StatusNotFound}[reach], status, "get %s: %s", key, body)
				status, body = doChatAutomation(t, tc.client, http.MethodDelete, path)
				require.Equal(t, map[bool]int{true: http.StatusNoContent, false: http.StatusNotFound}[reach], status, "delete %s: %s", key, body)
				require.Equal(t, !reach, env.exists(t, id), "delete %s", key)
			}
		})
	}
	t.Run("WrongOrganizationPath", func(t *testing.T) {
		t.Parallel()
		otherOrg := dbgen.Organization(t, env.db, database.Organization{})
		a := env.automation(t, database.ChatAutomation{OwnerID: member.ID})
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			status, body := doChatAutomation(t, env.owner, method, chatAutomationPath(otherOrg.ID, "/"+a.ID.String()))
			require.Equal(t, http.StatusNotFound, status, "%s: %s", method, body)
		}
		require.True(t, env.exists(t, a.ID))
	})
}

// TestChatAutomationsReadOnlyRole uses a role that can read automations and
// cannot read other users. The list shows the owner that the caller can read
// and leaves out the other. A delete answers 403 and the row stays.
func TestChatAutomationsReadOnlyRole(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	org := env.first.OrganizationID
	client, reader := env.readOnlyClient(t)
	mine := env.automation(t, database.ChatAutomation{OwnerID: reader.ID})
	theirs := env.automation(t, database.ChatAutomation{OwnerID: env.first.UserID})

	list, err := codersdk.NewExperimentalClient(client).ChatAutomations(testutil.Context(t, testutil.WaitShort), org, "")
	require.NoError(t, err)
	owners := map[uuid.UUID]*codersdk.MinimalUser{}
	for _, a := range list {
		owners[a.ID] = a.Owner
	}
	require.Len(t, owners, 2)
	require.NotNil(t, owners[mine.ID])
	require.Equal(t, reader.Username, owners[mine.ID].Username)
	require.Nil(t, owners[theirs.ID])

	status, body := doChatAutomation(t, client, http.MethodDelete, chatAutomationPath(org, "/"+theirs.ID.String()))
	require.Equal(t, http.StatusForbidden, status, string(body))
	require.True(t, env.exists(t, theirs.ID))
}

func TestChatAutomationsOwnerFilter(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	org := env.first.OrganizationID
	memberClient, member := coderdtest.CreateAnotherUser(t, env.owner, org)
	_, other := coderdtest.CreateAnotherUser(t, env.owner, org)
	// The caller reads every owner's automations, so each row shows whether
	// the filter applies.
	adminClient, _ := coderdtest.CreateAnotherUser(t, env.owner, org, rbac.ScopedRoleOrgAdmin(org))
	mine := env.automation(t, database.ChatAutomation{OwnerID: member.ID})
	env.automation(t, database.ChatAutomation{OwnerID: other.ID})
	// A username of 32 hexadecimal characters is the ID of another user
	// without its hyphens.
	hexName := strings.ReplaceAll(member.ID.String(), "-", "")
	hexUser := dbgen.User(t, env.db, database.User{Username: hexName})
	dbgen.OrganizationMember(t, env.db, database.OrganizationMember{UserID: hexUser.ID, OrganizationID: org})
	hexMine := env.automation(t, database.ChatAutomation{OwnerID: hexUser.ID})

	for name, tc := range map[string]struct {
		owner string
		// asMember lists as a member, who cannot read the user that owner names.
		asMember bool
		want     []uuid.UUID
	}{
		"Me":              {owner: codersdk.Me},
		"UserID":          {owner: member.ID.String(), want: []uuid.UUID{mine.ID}},
		"UsernameAnyCase": {owner: strings.ToUpper(member.Username), want: []uuid.UUID{mine.ID}},
		"UnknownUsername": {owner: "no-such-user-" + uuid.NewString()[:8]},
		"NilUUID":         {owner: uuid.Nil.String()},
		"Email":           {owner: member.Email},
		// Only the canonical form of an ID is an ID.
		"HexUsername":        {owner: hexName, want: []uuid.UUID{hexMine.ID}},
		"Braces":             {owner: "{" + member.ID.String() + "}"},
		"URN":                {owner: "urn:uuid:" + member.ID.String()},
		"UnreadableUsername": {owner: other.Username, asMember: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			query := "?" + url.Values{"owner": {tc.owner}}.Encode()
			client := adminClient
			if tc.asMember {
				client = memberClient
			}
			require.ElementsMatch(t, tc.want, listChatAutomationIDs(t, client, org, query))
		})
	}
}

func TestChatAutomationsResponse(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	org := env.first.OrganizationID
	memberClient, member := coderdtest.CreateAnotherUser(t, env.owner, org)
	_, other := coderdtest.CreateAnotherUser(t, env.owner, org)
	valid := func(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }
	target := env.chat(t, member.ID, "Target chat")
	creator := env.chat(t, member.ID, "Creating chat")
	env.exec(t, "UPDATE chats SET archived = true WHERE id = $1", creator.ID)
	secret := "canary-secret-" + uuid.NewString()
	webhook := env.automation(t, database.ChatAutomation{
		OwnerID: member.ID, TargetChatID: valid(target.ID), CreatedByChatID: valid(creator.ID),
		WhenBusy:      database.NullChatAutomationWhenBusy{ChatAutomationWhenBusy: database.ChatAutomationWhenBusySkip, Valid: true},
		WebhookSecret: sql.NullString{String: secret, Valid: true},
	})
	// The member cannot read the chat of another member.
	otherChat := env.chat(t, other.ID, "Other member chat")
	unreadableChats := env.automation(t, database.ChatAutomation{
		OwnerID: member.ID, TargetChatID: valid(otherChat.ID), CreatedByChatID: valid(otherChat.ID),
	})
	project := dbgen.ChatProject(t, env.db, database.ChatProject{OrganizationID: org, OwnerID: member.ID})
	newChat := env.automation(t, database.ChatAutomation{
		OwnerID: member.ID, TargetMode: database.ChatAutomationTargetModeNewChat,
		NewChatModelConfigID: valid(env.model.ID), ProjectID: valid(project.ID),
		ReasoningEffort: database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffortHigh, Valid: true},
	})
	env.exec(t, `UPDATE chat_automations SET last_run_at = TIMESTAMPTZ '2026-01-02 03:04:05+00', last_run_outcome = 'skipped',
		last_run_reason = 'target chat busy', last_run_chat_id = $1 WHERE id = $2`, target.ID, newChat.ID)
	// The allow list reaches the organization and this automation only, so
	// the caller reads the automation but neither its owner nor its chats.
	narrow, err := memberClient.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{AllowList: []codersdk.APIAllowListTarget{
		{Type: codersdk.ResourceOrganization, ID: org.String()},
		{Type: codersdk.ResourceWildcard, ID: webhook.ID.String()},
	}})
	require.NoError(t, err)
	narrowClient := codersdk.New(env.owner.URL, codersdk.WithSessionToken(narrow.Key))
	get := func(t *testing.T, client *codersdk.Client, id uuid.UUID) (codersdk.ChatAutomation, string) {
		t.Helper()
		status, body := doChatAutomation(t, client, http.MethodGet, chatAutomationPath(org, "/"+id.String()))
		require.Equal(t, http.StatusOK, status, string(body))
		var got codersdk.ChatAutomation
		require.NoError(t, json.Unmarshal(body, &got))
		return got, string(body)
	}

	t.Run("Webhook", func(t *testing.T) {
		t.Parallel()
		got, body := get(t, memberClient, webhook.ID)
		require.NotContains(t, body, secret)
		require.Regexp(t, `"paused_reasons":\s*\[\]`, body)
		require.Regexp(t, `"next_run_times":\s*\[\]`, body)
		require.Equal(t, &codersdk.MinimalUser{ID: member.ID, Username: member.Username, Name: member.Name, AvatarURL: member.AvatarURL}, got.Owner)
		require.Equal(t, &codersdk.ChatAutomationChat{ID: target.ID, Title: target.Title}, got.TargetChat)
		require.Equal(t, &codersdk.ChatAutomationChat{ID: creator.ID, Title: creator.Title, Archived: true}, got.CreatedByChat)
		require.Equal(t, codersdk.ChatAutomationWhenBusySkip, *got.WhenBusy)
		require.Equal(t, codersdk.ChatAutomationWebhookAuthBearer, *got.WebhookAuth)
		require.Nil(t, got.LastRun)
	})
	t.Run("Unreadable", func(t *testing.T) {
		t.Parallel()
		got, _ := get(t, memberClient, unreadableChats.ID)
		require.Equal(t, &otherChat.ID, got.TargetChatID)
		require.Nil(t, got.TargetChat)
		require.Nil(t, got.CreatedByChat)
		got, _ = get(t, narrowClient, webhook.ID)
		require.Equal(t, member.ID, got.OwnerID)
		require.Nil(t, got.Owner)
	})
	t.Run("NewChatWithLastRun", func(t *testing.T) {
		t.Parallel()
		got, _ := get(t, memberClient, newChat.ID)
		require.Equal(t, codersdk.ChatAutomationTargetModeNewChat, got.TargetMode)
		require.Equal(t, &env.model.ID, got.NewChatModelConfigID)
		require.Equal(t, &project.ID, got.ProjectID)
		require.Equal(t, string(database.ChatReasoningEffortHigh), *got.ReasoningEffort)
		require.NotNil(t, got.LastRun)
		require.True(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC).Equal(got.LastRun.At), got.LastRun.At)
		require.Equal(t, codersdk.ChatAutomationRunOutcomeSkipped, got.LastRun.Outcome)
		require.Equal(t, "target chat busy", *got.LastRun.Reason)
		require.Equal(t, &target.ID, got.LastRun.ChatID)
	})
	for name, tc := range map[string]struct {
		zone    string
		enabled bool
	}{
		"ScheduleEnabled":     {"America/New_York", true},
		"ScheduleDisabled":    {"America/New_York", false},
		"ScheduleUnknownZone": {"Not/AZone", true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := env.automation(t, database.ChatAutomation{
				OwnerID: member.ID, Kind: database.ChatAutomationKindSchedule, Enabled: tc.enabled,
				ScheduleCron:     sql.NullString{String: "0 9 * * *", Valid: true},
				ScheduleTimeZone: sql.NullString{String: tc.zone, Valid: true},
			})
			before := time.Now()
			got, body := get(t, memberClient, a.ID)
			after := time.Now()
			require.Equal(t, "0 9 * * *", *got.ScheduleCron)
			require.Equal(t, tc.zone, *got.ScheduleTimeZone)
			sched, err := cron.Standard("0 9 * * *", tc.zone)
			if err != nil || !tc.enabled {
				require.Regexp(t, `"next_run_times":\s*\[\]`, body)
				return
			}
			require.Len(t, got.NextRunTimes, 5)
			require.False(t, got.NextRunTimes[0].Before(sched.Next(before)))
			require.False(t, got.NextRunTimes[0].After(sched.Next(after)))
			for i := 1; i < len(got.NextRunTimes); i++ {
				require.True(t, got.NextRunTimes[i].Equal(sched.Next(got.NextRunTimes[i-1])), "run %d", i)
			}
		})
	}
}

const chatAutomationErrorCanary = "canary-internal-error"

// failingAutomationStore fails the automation reads with an error that holds
// the canary. InTx hands the wrapper to the callback so that the override
// reaches the transaction of a delete.
type failingAutomationStore struct{ database.Store }

func (s failingAutomationStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error { return fn(failingAutomationStore{Store: tx}) }, opts)
}

func (failingAutomationStore) GetAuthorizedChatAutomationsByOrganizationID(context.Context, database.GetChatAutomationsByOrganizationIDParams, rbac.PreparedAuthorized) ([]database.GetChatAutomationsByOrganizationIDRow, error) {
	return nil, xerrors.New(chatAutomationErrorCanary)
}

func (failingAutomationStore) GetChatAutomationByID(context.Context, uuid.UUID) (database.ChatAutomation, error) {
	return database.ChatAutomation{}, xerrors.New(chatAutomationErrorCanary)
}

// TestChatAutomationsInternalError checks that a 500 names no cause: the
// request logger logs the body of a 5xx response.
func TestChatAutomationsInternalError(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, func(db database.Store) database.Store { return failingAutomationStore{Store: db} })
	one := chatAutomationPath(env.first.OrganizationID, "/"+uuid.NewString())
	for name, req := range map[string]struct{ method, path string }{
		"List":   {http.MethodGet, chatAutomationPath(env.first.OrganizationID, "")},
		"Get":    {http.MethodGet, one},
		"Delete": {http.MethodDelete, one},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			status, body := doChatAutomation(t, env.owner, req.method, req.path)
			require.Equal(t, http.StatusInternalServerError, status, string(body))
			require.NotContains(t, string(body), chatAutomationErrorCanary)
			var res codersdk.Response
			require.NoError(t, json.Unmarshal(body, &res))
			require.NotEmpty(t, res.Message)
			require.Empty(t, res.Detail)
		})
	}
}

// queuedChat inserts a chat of the owner in the given status and queues one
// message per automation, head first. uuid.Nil stands for a message of a
// person. The head is under edit when the status is paused.
func (e chatAutomationEnv) queuedChat(t *testing.T, ownerID uuid.UUID, status database.ChatStatus, automationIDs ...uuid.UUID) database.Chat {
	t.Helper()
	chat := dbgen.Chat(t, e.db, database.Chat{
		OrganizationID: e.first.OrganizationID, OwnerID: ownerID, LastModelConfigID: e.model.ID, Status: status,
	})
	for i, automationID := range automationIDs {
		var automation, input uuid.NullUUID
		if automationID != uuid.Nil {
			automation = uuid.NullUUID{UUID: automationID, Valid: true}
			input = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		}
		e.exec(t, `INSERT INTO chat_queued_messages (chat_id, content, created_by, model_config_id, automation_id, input_id, editing_since)
			VALUES ($1, '[]', $2, $3, $4, $5, CASE WHEN $6 THEN now() END)`,
			chat.ID, ownerID, e.model.ID, automation, input, i == 0 && status == database.ChatStatusPaused)
	}
	return chat
}

// queued returns the number of queued messages of the chat and its status.
func (e chatAutomationEnv) queued(t *testing.T, chatID uuid.UUID) (int, database.ChatStatus) {
	t.Helper()
	var n int
	var status database.ChatStatus
	require.NoError(t, e.sqlDB.QueryRowContext(testutil.Context(t, testutil.WaitShort),
		`SELECT (SELECT count(*) FROM chat_queued_messages WHERE chat_id = chats.id), status FROM chats WHERE id = $1`,
		chatID).Scan(&n, &status))
	return n, status
}

// TestChatAutomationsDeleteQueuedMessage deletes an automation whose queued
// message blocks a paused chat. The message goes, the chat leaves paused, and
// the chat list of its owner hears of it. The chat worker is off.
func TestChatAutomationsDeleteQueuedMessage(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	env := newChatAutomationEnv(t, nil)
	automation := env.automation(t, database.ChatAutomation{OwnerID: env.first.UserID})
	chat := env.queuedChat(t, env.first.UserID, database.ChatStatusPaused, automation.ID)
	events := make(chan codersdk.ChatWatchEvent, 4)
	cancel, err := env.api.Pubsub.SubscribeWithErr(coderdpubsub.ChatWatchEventChannel(env.first.UserID),
		coderdpubsub.HandleChatWatchEvent(func(_ context.Context, event codersdk.ChatWatchEvent, err error) {
			if err == nil {
				events <- event
			}
		}))
	require.NoError(t, err)
	defer cancel()
	env.auditor.ResetLogs()

	status, body := doChatAutomation(t, env.owner, http.MethodDelete, chatAutomationPath(env.first.OrganizationID, "/"+automation.ID.String()))

	require.Equal(t, http.StatusNoContent, status, string(body))
	require.False(t, env.exists(t, automation.ID))
	left, chatStatus := env.queued(t, chat.ID)
	require.Zero(t, left)
	require.Equal(t, database.ChatStatusWaiting, chatStatus)
	logs := env.auditor.AuditLogs()
	require.Len(t, logs, 1)
	require.Equal(t, database.AuditActionDelete, logs[0].Action)
	require.Equal(t, automation.ID, logs[0].ResourceID)
	event := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, codersdk.ChatWatchEventKindStatusChange, event.Kind)
	require.Equal(t, chat.ID, event.Chat.ID)
	require.Equal(t, codersdk.ChatStatusWaiting, event.Chat.Status)
}

// TestChatAutomationsDeleteInvalidChat deletes an automation whose queued
// message sits in a chat in an invalid state. The delete answers 409, names
// the chat, and changes nothing.
func TestChatAutomationsDeleteInvalidChat(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	automation := env.automation(t, database.ChatAutomation{OwnerID: env.first.UserID})
	chat := env.queuedChat(t, env.first.UserID, database.ChatStatusWaiting, automation.ID)
	env.auditor.ResetLogs()

	status, body := doChatAutomation(t, env.owner, http.MethodDelete, chatAutomationPath(env.first.OrganizationID, "/"+automation.ID.String()))

	require.Equal(t, http.StatusConflict, status, string(body))
	var res codersdk.Response
	require.NoError(t, json.Unmarshal(body, &res))
	require.Contains(t, res.Message, "invalid state")
	require.Contains(t, res.Detail, chat.ID.String())
	require.True(t, env.exists(t, automation.ID))
	left, _ := env.queued(t, chat.ID)
	require.Equal(t, 1, left)
	require.Empty(t, env.auditor.AuditLogs())
}

// TestDeleteUserChatAutomations deletes a user. The queued messages of the
// user's automations go from every chat, and the chats end in the state that
// the queue cleanup defines.
func TestDeleteUserChatAutomations(t *testing.T) {
	t.Parallel()

	t.Run("NoAutomations", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		env := newChatAutomationEnv(t, nil)
		_, user := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
		env.auditor.ResetLogs()

		require.NoError(t, env.owner.DeleteUser(ctx, user.ID))

		logs := env.auditor.AuditLogs()
		require.Len(t, logs, 1)
		require.Equal(t, database.AuditActionDelete, logs[0].Action)
		require.Equal(t, user.ID, logs[0].ResourceID)
	})

	// A user admin can delete a user and can read neither that user's
	// automations nor chats: the cleanup does not depend on the caller.
	t.Run("QueuedMessages", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		env := newChatAutomationEnv(t, nil)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID, rbac.RoleUserAdmin())
		_, user := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
		first := env.automation(t, database.ChatAutomation{OwnerID: user.ID})
		second := env.automation(t, database.ChatAutomation{OwnerID: user.ID})
		running := env.queuedChat(t, user.ID, database.ChatStatusRunning, first.ID, uuid.Nil, second.ID)
		paused := env.queuedChat(t, user.ID, database.ChatStatusPaused, second.ID)
		updates := make(chan struct{}, 4)
		cancel, err := env.api.Pubsub.Subscribe(coderdpubsub.ChatStateUpdateChannel(paused.ID), func(context.Context, []byte) {
			updates <- struct{}{}
		})
		require.NoError(t, err)
		defer cancel()

		require.NoError(t, userAdmin.DeleteUser(ctx, user.ID))

		testutil.RequireReceive(ctx, t, updates)
		require.False(t, env.exists(t, first.ID))
		require.False(t, env.exists(t, second.ID))
		left, status := env.queued(t, running.ID)
		require.Equal(t, 1, left, "the message of a person stays")
		require.Equal(t, database.ChatStatusRunning, status)
		left, status = env.queued(t, paused.ID)
		require.Zero(t, left)
		require.Equal(t, database.ChatStatusWaiting, status)
	})

	// A caller who cannot delete the user is refused before the delete
	// takes an automation lock: it does not wait for a lock that is held.
	t.Run("NoPermission", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		env := newChatAutomationEnv(t, nil)
		// An auditor can read the user and cannot delete it.
		auditor, _ := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID, rbac.RoleAuditor())
		_, user := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
		automation := env.automation(t, database.ChatAutomation{OwnerID: user.ID})
		holder, err := env.sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = holder.Rollback() }()
		_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, chatautomation.LockID(automation.ID))
		require.NoError(t, err)

		err = auditor.DeleteUser(testutil.Context(t, testutil.WaitShort), user.ID)

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
		require.True(t, env.exists(t, automation.ID))
	})

	// A chat that refuses the cleanup keeps its queued message. The delete
	// succeeds as it does without automations, and the other chat is
	// cleaned.
	t.Run("InvalidChat", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		env := newChatAutomationEnv(t, nil)
		_, user := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
		automation := env.automation(t, database.ChatAutomation{OwnerID: user.ID})
		// A waiting chat with a queue is in an invalid state.
		invalid := env.queuedChat(t, user.ID, database.ChatStatusWaiting, automation.ID)
		valid := env.queuedChat(t, user.ID, database.ChatStatusRunning, automation.ID)

		require.NoError(t, env.owner.DeleteUser(ctx, user.ID))

		require.False(t, env.exists(t, automation.ID))
		left, _ := env.queued(t, invalid.ID)
		require.Equal(t, 1, left)
		left, _ = env.queued(t, valid.ID)
		require.Zero(t, left)
	})
}

// TestDeleteUserCommitsBeforeChatLock deletes a user while another
// transaction holds a chat with a queued message of the user's automation, as
// the purge of old chats does when it deletes that chat. The soft delete must
// commit without the chat row: the purge goes on to update the automations
// that name the chat, which the soft delete removes, so a soft delete that
// waited for the chat would form a lock cycle with it.
func TestDeleteUserCommitsBeforeChatLock(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	env := newChatAutomationEnv(t, nil)
	_, user := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
	chat := env.queuedChat(t, user.ID, database.ChatStatusRunning)
	automation := env.automation(t, database.ChatAutomation{
		OwnerID: user.ID, TargetChatID: uuid.NullUUID{UUID: chat.ID, Valid: true},
	})
	env.exec(t, `INSERT INTO chat_queued_messages (chat_id, content, created_by, automation_id, input_id)
		VALUES ($1, '[]', $2, $3, $4)`, chat.ID, user.ID, automation.ID, uuid.New())

	holder, err := env.sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = holder.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, chat.ID)
	require.NoError(t, err)
	done, finished := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(finished)
		done <- env.owner.DeleteUser(ctx, user.ID)
	}()
	// A failed assertion releases the chat, so the delete returns.
	t.Cleanup(func() {
		_ = holder.Rollback()
		<-finished
	})
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var deleted bool
		var automations int
		err := env.sqlDB.QueryRowContext(ctx, `SELECT deleted, (SELECT count(*) FROM chat_automations WHERE owner_id = $1)
			FROM users WHERE id = $1`, user.ID).Scan(&deleted, &automations)
		return err == nil && deleted && automations == 0
	}, testutil.IntervalFast, "the soft delete did not commit while another transaction held the chat")

	// What DeleteOldChats does to one chat row. The statement also sets
	// target_chat_id to NULL in the automations that name the chat.
	_, err = holder.ExecContext(ctx, `DELETE FROM chats WHERE id = $1`, chat.ID)
	require.NoError(t, err)
	require.NoError(t, holder.Commit())
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}

// TestDeleteUserChatAutomationLocks deletes a user while another transaction
// holds a lock that the delete needs, and then takes a chat row lock itself.
// The delete must wait before it holds a chat row. Otherwise the two
// transactions deadlock.
func TestDeleteUserChatAutomationLocks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// first is the lock that the other transaction holds when the
		// delete starts. $1 is the user and $2 the automation lock.
		first string
		// second is what it does while the delete waits. $1 is the chat.
		second string
	}{
		{
			// A run holds the automation lock and the chat row, and queues
			// another message next to an older one.
			name:   "RunQueuesMessage",
			first:  `SELECT pg_advisory_xact_lock($2::bigint) FROM users WHERE id = $1`,
			second: `SELECT id FROM chats WHERE id = $1 FOR UPDATE`,
		},
		{
			// The purge deletes expired API keys and then old chats.
			name:   "APIKeyThenChat",
			first:  `SELECT id FROM api_keys WHERE user_id = $1 AND $2::bigint IS NOT NULL FOR UPDATE`,
			second: `SELECT id FROM chats WHERE id = $1 FOR UPDATE`,
		},
		{
			// A create guards the owner and then checks the target chat.
			name:   "UserThenChat",
			first:  `SELECT id FROM users WHERE id = $1 AND $2::bigint IS NOT NULL FOR SHARE`,
			second: `SELECT id FROM chats WHERE id = $1 FOR KEY SHARE`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			env := newChatAutomationEnv(t, nil)
			_, user := coderdtest.CreateAnotherUser(t, env.owner, env.first.OrganizationID)
			automation := env.automation(t, database.ChatAutomation{OwnerID: user.ID})
			chat := env.queuedChat(t, user.ID, database.ChatStatusRunning, automation.ID)

			holder, err := env.sqlDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			var pid int
			require.NoError(t, holder.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
			_, err = holder.ExecContext(ctx, tc.first, user.ID, chatautomation.LockID(automation.ID))
			require.NoError(t, err)
			done, finished := make(chan error, 1), make(chan struct{})
			go func() {
				defer close(finished)
				done <- env.owner.DeleteUser(ctx, user.ID)
			}()
			// A failed assertion releases the locks, so the delete returns.
			t.Cleanup(func() {
				_ = holder.Rollback()
				<-finished
			})
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				var n int
				err := env.sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
					WHERE wait_event_type = 'Lock' AND $1 = ANY(pg_blocking_pids(pid))`, pid).Scan(&n)
				return err == nil && n > 0
			}, testutil.IntervalFast, "the delete never waited for the other transaction")

			_, err = holder.ExecContext(ctx, tc.second, chat.ID)
			require.NoError(t, err)
			_, err = holder.ExecContext(ctx, `INSERT INTO chat_queued_messages (chat_id, content, created_by, automation_id, input_id)
				VALUES ($1, '[]', $2, $3, $4)`, chat.ID, user.ID, automation.ID, uuid.New())
			require.NoError(t, err)
			require.NoError(t, holder.Commit())

			require.NoError(t, testutil.RequireReceive(ctx, t, done))
			left, _ := env.queued(t, chat.ID)
			require.Zero(t, left)
		})
	}
}
