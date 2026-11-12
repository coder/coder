package chatautomation_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/testutil"
)

// toolEnv is an organization with an administrator and two members, each of
// whom owns automations whose webhook secret is a canary.
type toolEnv struct {
	db     database.Store
	sqlDB  *sql.DB
	org    database.Organization
	admin  database.User
	member database.User
	other  database.User
	canary string
}

func newToolEnv(t *testing.T) toolEnv {
	t.Helper()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := func(roles ...string) database.User {
		u := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u.ID, OrganizationID: org.ID, Roles: roles})
		return u
	}
	return toolEnv{
		db: db, sqlDB: sqlDB, org: org, canary: "canary-secret-" + uuid.NewString(),
		admin: user(rbac.RoleOrgAdmin()), member: user(), other: user(),
	}
}

func (e toolEnv) automation(t *testing.T, owner database.User, prompt string) database.ChatAutomation {
	t.Helper()
	return dbgen.ChatAutomation(t, e.db, database.ChatAutomation{
		OrganizationID: e.org.ID,
		OwnerID:        owner.ID,
		Prompt:         prompt,
		WebhookSecret:  sql.NullString{String: e.canary, Valid: true},
	})
}

// run calls the tool as the owner of a chat in the organization. Every result
// must be free of the webhook secret.
func (e toolEnv) run(t *testing.T, owner database.User, input string) fantasy.ToolResponse {
	t.Helper()
	tool := chatautomation.Tool(chatautomation.ToolOptions{
		DB:             authorized(t, e.db),
		OwnerID:        owner.ID,
		OrganizationID: e.org.ID,
		Logger:         testutil.Logger(t),
	})
	resp, err := tool.Run(testutil.Context(t, testutil.WaitLong), fantasy.ToolCall{ID: "call", Name: chatautomation.ToolName, Input: input})
	require.NoError(t, err)
	require.Zero(t, strings.Count(resp.Content, e.canary), "the webhook secret is in a tool result")
	return resp
}

type toolItem struct {
	ID              uuid.UUID `json:"id"`
	Prompt          string    `json:"prompt"`
	PromptTruncated bool      `json:"prompt_truncated"`
	Owner           *struct {
		ID uuid.UUID `json:"id"`
	} `json:"owner"`
}

type toolList struct {
	Automations []toolItem `json:"automations"`
	Total       int        `json:"total"`
	Note        string     `json:"note"`
}

func ids(items []toolItem) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

// TestToolList lists as the owner of the chat: an administrator's chat shows
// only the administrator's own automations, though the administrator can read
// every automation in the organization.
func TestToolList(t *testing.T) {
	t.Parallel()
	e := newToolEnv(t)
	mine := []uuid.UUID{e.automation(t, e.member, "a").ID, e.automation(t, e.member, "b").ID}
	theirs := e.automation(t, e.other, "c")
	adminOwn := e.automation(t, e.admin, "d")

	for name, tc := range map[string]struct {
		owner database.User
		want  []uuid.UUID
	}{
		"Member":        {e.member, mine},
		"Administrator": {e.admin, []uuid.UUID{adminOwn.ID}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := e.run(t, tc.owner, `{"action":"list"}`)
			require.False(t, resp.IsError, resp.Content)
			var got toolList
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &got))
			require.ElementsMatch(t, tc.want, ids(got.Automations))
			require.NotContains(t, ids(got.Automations), theirs.ID)
			require.Equal(t, len(tc.want), got.Total)
			require.Empty(t, got.Note)
			for _, item := range got.Automations {
				require.NotNil(t, item.Owner)
				require.Equal(t, tc.owner.ID, item.Owner.ID)
			}
		})
	}
}

// TestToolListBound lists more automations than a result holds, with prompts
// longer than a list shows.
func TestToolListBound(t *testing.T) {
	t.Parallel()
	e := newToolEnv(t)
	long := strings.Repeat("é", 300)
	for i := range 25 {
		prompt := fmt.Sprintf("short %d", i)
		if i%2 == 0 {
			prompt = long
		}
		e.automation(t, e.member, prompt)
	}

	resp := e.run(t, e.member, `{"action":"list"}`)

	require.False(t, resp.IsError, resp.Content)
	var got toolList
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &got))
	require.Len(t, got.Automations, 20)
	require.Equal(t, 25, got.Total)
	require.Contains(t, got.Note, "newest 20 of 25")
	require.Contains(t, got.Note, "prompt_truncated")
	cut, whole := 0, 0
	for _, item := range got.Automations {
		if item.PromptTruncated {
			cut++
			require.Len(t, []rune(item.Prompt), 200)
		} else {
			whole++
			require.True(t, strings.HasPrefix(item.Prompt, "short "), item.Prompt)
		}
	}
	require.NotZero(t, cut)
	require.NotZero(t, whole)
}

// TestToolListBytes lists automations whose rows are large, so that the byte
// bound cuts the list before the count bound does.
func TestToolListBytes(t *testing.T) {
	t.Parallel()
	e := newToolEnv(t)
	for range 10 {
		e.automation(t, e.member, "prompt")
	}
	_, err := e.sqlDB.ExecContext(testutil.Context(t, testutil.WaitLong),
		`UPDATE chat_automations SET last_run_at = now(), last_run_outcome = 'skipped', last_run_reason = repeat('x', 10000)
		WHERE organization_id = $1`, e.org.ID)
	require.NoError(t, err)

	resp := e.run(t, e.member, `{"action":"list"}`)

	require.False(t, resp.IsError, resp.Content)
	var got toolList
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &got))
	require.Equal(t, 10, got.Total)
	require.NotEmpty(t, got.Automations)
	require.Less(t, len(got.Automations), 10)
	require.Contains(t, got.Note, fmt.Sprintf("newest %d of 10", len(got.Automations)))
	require.Less(t, len(resp.Content), 33<<10)
}

// TestToolGet reads one automation as the chat owner: any automation of the
// chat's organization that the owner can read, and nothing else. Each failure gives the same answer.
func TestToolGet(t *testing.T) {
	t.Parallel()
	e := newToolEnv(t)
	own := e.automation(t, e.member, "own prompt")
	other := e.automation(t, e.other, "other prompt")
	gone := dbgen.User(t, e.db, database.User{})
	dbgen.OrganizationMember(t, e.db, database.OrganizationMember{UserID: gone.ID, OrganizationID: e.org.ID})
	goneAutomation := e.automation(t, gone, "gone prompt")
	// The member also belongs to another organization and owns an automation
	// there. The chat is in the first organization.
	otherOrg := dbgen.Organization(t, e.db, database.Organization{})
	dbgen.OrganizationMember(t, e.db, database.OrganizationMember{UserID: e.member.ID, OrganizationID: otherOrg.ID})
	otherOrgAutomation := dbgen.ChatAutomation(t, e.db, database.ChatAutomation{OrganizationID: otherOrg.ID, OwnerID: e.member.ID})
	//nolint:gocritic // The soft delete trigger removes the user's automations.
	require.NoError(t, e.db.UpdateUserDeletedByID(dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong)), gone.ID))

	for name, tc := range map[string]struct {
		owner  database.User
		id     string
		prompt string
	}{
		"Own":               {owner: e.member, id: own.ID.String(), prompt: "own prompt"},
		"AdministratorChat": {owner: e.admin, id: other.ID.String(), prompt: "other prompt"},
		"MemberChat":        {owner: e.member, id: other.ID.String()},
		"Unknown":           {owner: e.admin, id: uuid.NewString()},
		"Malformed":         {owner: e.admin, id: "not-an-id"},
		"Missing":           {owner: e.admin},
		"SoftDeletedOwner":  {owner: e.admin, id: goneAutomation.ID.String()},
		"OtherOrganization": {owner: e.member, id: otherOrgAutomation.ID.String()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input, err := json.Marshal(map[string]string{"action": "get", "id": tc.id})
			require.NoError(t, err)
			resp := e.run(t, tc.owner, string(input))
			if tc.prompt == "" {
				require.True(t, resp.IsError)
				require.Equal(t, "automation not found", resp.Content)
				return
			}
			require.False(t, resp.IsError, resp.Content)
			var got toolItem
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &got))
			require.Equal(t, tc.id, got.ID.String())
			require.Equal(t, tc.prompt, got.Prompt)
		})
	}
}

// TestToolUnknownAction answers every action but list and get with an error
// that names the actions, and changes no row.
func TestToolUnknownAction(t *testing.T) {
	t.Parallel()
	e := newToolEnv(t)
	own := e.automation(t, e.member, "own prompt")

	for _, action := range []string{"create", "update", "enable", "disable", "delete", "run_now", "", "LIST"} {
		t.Run("Action"+action, func(t *testing.T) {
			t.Parallel()
			input, err := json.Marshal(map[string]string{"action": action, "id": own.ID.String()})
			require.NoError(t, err)
			resp := e.run(t, e.member, string(input))
			require.True(t, resp.IsError)
			require.Contains(t, resp.Content, fmt.Sprintf("unknown action %q", action))
			require.Contains(t, resp.Content, "the actions are list and get")
			got, err := e.db.GetChatAutomationByID(dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong)), own.ID) //nolint:gocritic // Read back the row.
			require.NoError(t, err)
			require.Equal(t, own, got)
		})
	}
}
