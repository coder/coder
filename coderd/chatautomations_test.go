package coderd_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
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

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
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

func newChatAutomationEnv(t *testing.T, wrap func(database.Store) database.Store, opts ...func(*coderdtest.Options)) chatAutomationEnv {
	t.Helper()
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	store := db
	if wrap != nil {
		store = wrap(db)
	}
	values := coderdtest.DeploymentValues(t)
	values.Experiments = []string{string(codersdk.ExperimentChatAutomations)}
	auditor := audit.NewMock()
	options := &coderdtest.Options{
		DeploymentValues:   values,
		Database:           store,
		Pubsub:             ps,
		Auditor:            auditor,
		ChatWorkerDisabled: true,
	}
	for _, opt := range opts {
		opt(options)
	}
	client, _, api := coderdtest.NewWithAPI(t, options)
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
		{http.MethodPost, ""},
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
		{http.MethodPost, ""},
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

// TestChatAutomationsDeleteQueuedMessage deletes an automation that a queued
// message names: the answer is a 500 with no detail, and nothing changes.
func TestChatAutomationsDeleteQueuedMessage(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	chat := env.chat(t, env.first.UserID, "")
	automation := env.automation(t, database.ChatAutomation{OwnerID: env.first.UserID})
	env.exec(t, `INSERT INTO chat_queued_messages (chat_id, content, created_by, automation_id, input_id)
		VALUES ($1, '[]', $2, $3, $4)`, chat.ID, env.first.UserID, automation.ID, uuid.New())
	env.auditor.ResetLogs()

	status, body := doChatAutomation(t, env.owner, http.MethodDelete, chatAutomationPath(env.first.OrganizationID, "/"+automation.ID.String()))

	require.Equal(t, http.StatusInternalServerError, status, string(body))
	var res codersdk.Response
	require.NoError(t, json.Unmarshal(body, &res))
	require.Empty(t, res.Detail)
	require.True(t, env.exists(t, automation.ID))
	require.Empty(t, env.auditor.AuditLogs())
}

// chatAutomationActor is a user of the deployment and a client for it.
type chatAutomationActor struct {
	client *codersdk.Client
	id     uuid.UUID
}

func (e chatAutomationEnv) actor(t *testing.T, roles ...rbac.RoleIdentifier) chatAutomationActor {
	t.Helper()
	client, user := coderdtest.CreateAnotherUser(t, e.owner, e.first.OrganizationID, roles...)
	return chatAutomationActor{client: client, id: user.ID}
}

// automationAudit returns the entries that the requests of one user wrote for
// chat automations. Every row of a table owns its user, so rows that run in
// parallel do not see each other's entries.
func (e chatAutomationEnv) automationAudit(userID uuid.UUID) []database.AuditLog {
	var logs []database.AuditLog
	for _, log := range e.auditor.AuditLogs() {
		if log.UserID == userID && log.ResourceType == database.ResourceTypeChatAutomation {
			logs = append(logs, log)
		}
	}
	return logs
}

// postChatAutomation sends a create as a raw request, so that a test can read
// the header and the bytes that the server wrote. A []byte body is sent as it
// is.
func postChatAutomation(t *testing.T, client *codersdk.Client, orgID uuid.UUID, body any) (http.Header, int, []byte) {
	t.Helper()
	res, err := client.Request(testutil.Context(t, testutil.WaitMedium), http.MethodPost, chatAutomationPath(orgID, ""), body)
	require.NoError(t, err)
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.Header, res.StatusCode, raw
}

func TestCreateChatAutomation(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil)
	org := env.first.OrganizationID

	for _, tc := range []struct {
		name string
		req  codersdk.CreateChatAutomationRequest
		// generated is true when Coder creates the secret.
		generated bool
	}{
		{"BearerExistingChat", codersdk.CreateChatAutomationRequest{Kind: codersdk.ChatAutomationKindWebhook, TargetMode: codersdk.ChatAutomationTargetModeExistingChat, WebhookAuth: codersdk.ChatAutomationWebhookAuthBearer}, true},
		{"GitHubNewChat", codersdk.CreateChatAutomationRequest{Kind: codersdk.ChatAutomationKindWebhook, TargetMode: codersdk.ChatAutomationTargetModeNewChat, WebhookAuth: codersdk.ChatAutomationWebhookAuthGitHub}, true},
		{"StandardWebhooks", codersdk.CreateChatAutomationRequest{Kind: codersdk.ChatAutomationKindWebhook, TargetMode: codersdk.ChatAutomationTargetModeExistingChat, WebhookAuth: codersdk.ChatAutomationWebhookAuthStandardWebhooks}, false},
		{"ScheduleNewChat", codersdk.CreateChatAutomationRequest{Kind: codersdk.ChatAutomationKindSchedule, TargetMode: codersdk.ChatAutomationTargetModeNewChat, ScheduleCron: "0 9 * * *", ScheduleTimeZone: "America/New_York"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			member := env.actor(t, rbac.ScopedRoleAgentsAccess(org))
			req := tc.req
			req.Name = "automation-" + uuid.NewString()
			req.Prompt = "canary-prompt-" + uuid.NewString()
			var target database.Chat
			if req.TargetMode == codersdk.ChatAutomationTargetModeExistingChat {
				target = env.chat(t, member.id, "Target "+uuid.NewString())
				req.TargetChatID = &target.ID
			} else {
				req.NewChatModelConfigID = &env.model.ID
			}
			var supplied string
			if req.WebhookAuth == codersdk.ChatAutomationWebhookAuthStandardWebhooks {
				supplied = "whsec_" + base64.StdEncoding.EncodeToString([]byte("canary-"+uuid.NewString()))
				req.WebhookSecret = supplied
			}

			header, status, body := postChatAutomation(t, member.client, org, req)

			require.Equal(t, http.StatusCreated, status, string(body))
			require.Equal(t, "no-store", header.Get("Cache-Control"))
			var created codersdk.CreateChatAutomationResponse
			require.NoError(t, json.Unmarshal(body, &created))
			require.Equal(t, tc.generated, created.WebhookSecret != "")
			require.NotNil(t, created.Automation.Owner)
			require.Equal(t, member.id, created.Automation.Owner.ID)
			if req.TargetChatID != nil {
				require.Equal(t, &codersdk.ChatAutomationChat{ID: target.ID, Title: target.Title}, created.Automation.TargetChat)
			}
			// The response carries a generated secret once, and never carries
			// a secret that the caller supplied.
			secrets := []string{created.WebhookSecret, supplied}
			if tc.generated {
				require.Equal(t, 1, strings.Count(string(body), created.WebhookSecret))
			}
			if supplied != "" {
				require.Zero(t, strings.Count(string(body), supplied))
			}

			for _, path := range []string{"/" + created.Automation.ID.String(), "?owner=me"} {
				status, got := doChatAutomation(t, member.client, http.MethodGet, chatAutomationPath(org, path))
				require.Equal(t, http.StatusOK, status, string(got))
				require.Contains(t, string(got), created.Automation.ID.String())
				for _, secret := range secrets {
					if secret != "" {
						require.Zero(t, strings.Count(string(got), secret), path)
					}
				}
			}
			logs := env.automationAudit(member.id)
			require.Len(t, logs, 1)
			require.Equal(t, database.AuditActionCreate, logs[0].Action)
			require.Equal(t, org, logs[0].OrganizationID)
			require.Equal(t, created.Automation.ID, logs[0].ResourceID)
		})
	}
}

func TestCreateChatAutomationRejected(t *testing.T) {
	t.Parallel()
	// The limit of one lets a row hit it with a single seeded automation.
	env := newChatAutomationEnv(t, nil, func(o *coderdtest.Options) {
		o.DeploymentValues.AI.Chat.MaxAutomationsPerOwner = serpent.Int64(1)
	})
	org := env.first.OrganizationID
	// Without the default, agents-access reaches only the members that hold it
	// explicitly, so one member of the organization can lack it.
	env.exec(t, "UPDATE organizations SET default_org_member_roles = '{organization-workspace-access}' WHERE id = $1", org)
	agentsAccess := rbac.ScopedRoleAgentsAccess(org)
	member := env.actor(t, agentsAccess)
	atLimit := env.actor(t, agentsAccess)
	env.automation(t, database.ChatAutomation{OwnerID: atLimit.id})
	// The key reads the organization, so that its request reaches the handler.
	token, err := member.client.CreateToken(testutil.Context(t, testutil.WaitMedium), codersdk.Me, codersdk.CreateTokenRequest{
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead, codersdk.APIKeyScopeOrganizationRead},
	})
	require.NoError(t, err)
	scoped := chatAutomationActor{client: codersdk.New(env.owner.URL, codersdk.WithSessionToken(token.Key)), id: member.id}
	otherOrg := dbgen.Organization(t, env.db, database.Organization{})
	otherAdminClient, otherAdmin := coderdtest.CreateAnotherUser(t, env.owner, otherOrg.ID, rbac.ScopedRoleOrgAdmin(otherOrg.ID))

	// valid is a request that only the caller or the limit can make fail.
	valid := func(canary string) codersdk.CreateChatAutomationRequest {
		return codersdk.CreateChatAutomationRequest{
			Name: "rejected-" + canary, Kind: codersdk.ChatAutomationKindWebhook, Prompt: canary,
			TargetMode: codersdk.ChatAutomationTargetModeNewChat, NewChatModelConfigID: &env.model.ID,
			WebhookAuth: codersdk.ChatAutomationWebhookAuthBearer,
		}
	}
	type rejection struct {
		who     chatAutomationActor
		body    func(canary string) any
		status  int
		message string
		fields  []string
	}
	validBody := func(canary string) any { return valid(canary) }
	forbidden := func(who chatAutomationActor) rejection {
		return rejection{who, validBody, http.StatusForbidden, "Forbidden.", nil}
	}
	for name, tc := range map[string]rejection{
		"InvalidJSON": {
			member, func(canary string) any { return []byte(fmt.Sprintf(`{"enabled": %q}`, canary)) },
			http.StatusBadRequest, "Request body must be valid JSON.", nil,
		},
		"EmptyObject": {
			member, func(string) any { return []byte(`{}`) },
			http.StatusBadRequest, "Validation failed.",
			[]string{"name", "kind", "target_mode", "prompt"},
		},
		"SecretForBearer": {member, func(canary string) any {
			req := valid(canary)
			req.WebhookSecret = canary
			return req
		}, http.StatusBadRequest, "Validation failed.", []string{"webhook_secret"}},
		"BodyTooLarge": {member, func(canary string) any {
			req := valid(canary)
			// The limit is twice the prompt limit, and the prompt is most of the body.
			req.Prompt = canary + strings.Repeat("a", 2*codersdk.DefaultChatMaxPromptBytes)
			return req
		}, http.StatusRequestEntityTooLarge, "Request body too large.", nil},
		"AtLimit": {
			atLimit, validBody, http.StatusConflict,
			"You can have at most 1 chat automations across all organizations. Delete one to create another.", nil,
		},
		"OrganizationAdmin":      forbidden(env.actor(t, rbac.ScopedRoleOrgAdmin(org))),
		"TemplateAdmin":          forbidden(env.actor(t, rbac.RoleTemplateAdmin())),
		"UserAdmin":              forbidden(env.actor(t, rbac.RoleUserAdmin())),
		"Auditor":                forbidden(env.actor(t, rbac.RoleAuditor())),
		"NoAgentsAccess":         forbidden(env.actor(t)),
		"DeploymentOwner":        forbidden(chatAutomationActor{client: env.owner, id: env.first.UserID}),
		"ScopedToken":            forbidden(scoped),
		"OtherOrganizationAdmin": {chatAutomationActor{client: otherAdminClient, id: otherAdmin.ID}, validBody, http.StatusNotFound, "", nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			canary := "canary-" + uuid.NewString()

			_, status, body := postChatAutomation(t, tc.who.client, org, tc.body(canary))

			require.Equal(t, tc.status, status, string(body))
			require.Zero(t, bytes.Count(body, []byte(canary)), "the response echoes the request")
			var got codersdk.Response
			require.NoError(t, json.Unmarshal(body, &got))
			if tc.message != "" {
				require.Equal(t, tc.message, got.Message)
			}
			fields := []string{}
			for _, v := range got.Validations {
				fields = append(fields, v.Field)
			}
			require.ElementsMatch(t, tc.fields, fields)
			require.Empty(t, env.automationAudit(tc.who.id))
		})
	}
}

// TestCreateChatAutomationLimits checks that the prompt size and the shortest
// schedule interval come from the deployment settings.
func TestCreateChatAutomationLimits(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, nil, func(o *coderdtest.Options) {
		o.DeploymentValues.AI.Chat.MaxPromptBytes = serpent.Int64(10)
		o.DeploymentValues.AI.Chat.MinAutomationScheduleInterval = serpent.Duration(time.Hour)
	})
	org := env.first.OrganizationID
	member := env.actor(t, rbac.ScopedRoleAgentsAccess(org))

	_, status, body := postChatAutomation(t, member.client, org, codersdk.CreateChatAutomationRequest{
		Name: "limits", Kind: codersdk.ChatAutomationKindSchedule, Prompt: strings.Repeat("a", 11),
		TargetMode: codersdk.ChatAutomationTargetModeNewChat, NewChatModelConfigID: &env.model.ID,
		ScheduleCron: "*/30 * * * *", ScheduleTimeZone: "UTC",
	})

	require.Equal(t, http.StatusBadRequest, status, string(body))
	var got codersdk.Response
	require.NoError(t, json.Unmarshal(body, &got))
	fields := []string{}
	for _, v := range got.Validations {
		fields = append(fields, v.Field)
	}
	require.ElementsMatch(t, []string{"prompt", "schedule_cron"}, fields)
}

// createStore fails what a create does after its checks. InTx hands the
// wrapper to the callback so that the overrides reach the transaction.
type createStore struct {
	database.Store
	insertErr error
	usersErr  error
}

func (s createStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(createStore{Store: tx, insertErr: s.insertErr, usersErr: s.usersErr})
	}, opts)
}

func (s createStore) InsertChatAutomation(ctx context.Context, arg database.InsertChatAutomationParams) (database.ChatAutomation, error) {
	if s.insertErr != nil {
		return database.ChatAutomation{}, s.insertErr
	}
	return s.Store.InsertChatAutomation(ctx, arg)
}

func (s createStore) GetUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]database.User, error) {
	if s.usersErr != nil {
		return nil, s.usersErr
	}
	return s.Store.GetUsersByIDs(ctx, ids)
}

func bearerChatAutomation(env chatAutomationEnv) codersdk.CreateChatAutomationRequest {
	return codersdk.CreateChatAutomationRequest{
		Name: "automation-" + uuid.NewString(), Kind: codersdk.ChatAutomationKindWebhook, Prompt: "Review the alert.",
		TargetMode: codersdk.ChatAutomationTargetModeNewChat, NewChatModelConfigID: &env.model.ID,
		WebhookAuth: codersdk.ChatAutomationWebhookAuthBearer,
	}
}

// TestChatAutomationsCreateInternalError checks that a failed insert answers a
// 500 with no cause in the body and no audit entry, and that the cause goes to
// the log: the request logger logs the body of a 5xx response.
func TestChatAutomationsCreateInternalError(t *testing.T) {
	t.Parallel()
	canary := chatAutomationErrorCanary + "-" + uuid.NewString()
	sink := testutil.NewFakeSink(t)
	logger := sink.Logger()
	env := newChatAutomationEnv(t,
		func(db database.Store) database.Store { return createStore{Store: db, insertErr: xerrors.New(canary)} },
		func(o *coderdtest.Options) { o.Logger = &logger })
	member := env.actor(t, rbac.ScopedRoleAgentsAccess(env.first.OrganizationID))

	_, status, body := postChatAutomation(t, member.client, env.first.OrganizationID, bearerChatAutomation(env))

	require.Equal(t, http.StatusInternalServerError, status, string(body))
	require.Zero(t, bytes.Count(body, []byte(canary)))
	var got codersdk.Response
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "Internal error creating chat automation.", got.Message)
	require.Empty(t, got.Detail)
	require.Empty(t, env.automationAudit(member.id))
	logged := sink.Entries(func(e slog.SinkEntry) bool { return e.Message == got.Message })
	require.Len(t, logged, 1)
	require.Contains(t, fmt.Sprint(logged[0].Fields), canary)
}

// TestChatAutomationsCreateResponseWithoutRefs checks that a failed read of
// the owner after the commit does not fail the create: the generated secret
// exists only in this response.
func TestChatAutomationsCreateResponseWithoutRefs(t *testing.T) {
	t.Parallel()
	env := newChatAutomationEnv(t, func(db database.Store) database.Store {
		return createStore{Store: db, usersErr: xerrors.New("owners are unavailable")}
	})
	member := env.actor(t, rbac.ScopedRoleAgentsAccess(env.first.OrganizationID))

	created, err := codersdk.NewExperimentalClient(member.client).CreateChatAutomation(
		testutil.Context(t, testutil.WaitMedium), env.first.OrganizationID, bearerChatAutomation(env))

	require.NoError(t, err)
	require.Equal(t, member.id, created.Automation.OwnerID)
	require.Nil(t, created.Automation.Owner)
	var stored string
	require.NoError(t, env.sqlDB.QueryRowContext(testutil.Context(t, testutil.WaitShort),
		"SELECT webhook_secret FROM chat_automations WHERE id = $1", created.Automation.ID).Scan(&stored))
	require.NotEmpty(t, created.WebhookSecret)
	require.Equal(t, stored, created.WebhookSecret)
}
