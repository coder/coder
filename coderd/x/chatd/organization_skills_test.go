package chatd_test

import (
	"database/sql"
	"strings"
	"sync"
	"testing"

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
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const readSkillsDirective = "read skills: "

// skillRecordingOpenAI records the system prompt and tool results of every
// streamed call. A user message starting with readSkillsDirective makes the
// model call read_skill once per comma-separated name.
type skillRecordingOpenAI struct {
	mu    sync.Mutex
	calls []skillRecordedCall
}

type skillRecordedCall struct {
	system      string
	toolResults []string
}

func newSkillRecordingOpenAI(t *testing.T) (*skillRecordingOpenAI, string) {
	t.Helper()
	recorder := &skillRecordingOpenAI{}
	url := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		var call skillRecordedCall
		for _, msg := range req.Messages {
			switch msg.Role {
			case "system":
				call.system += msg.Content + "\n"
			case "tool":
				call.toolResults = append(call.toolResults, msg.Content)
			}
		}
		recorder.mu.Lock()
		recorder.calls = append(recorder.calls, call)
		recorder.mu.Unlock()

		last := req.Messages[len(req.Messages)-1]
		names, ok := strings.CutPrefix(last.Content, readSkillsDirective)
		if last.Role != "user" || !ok {
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("ok")...)
		}
		var chunk chattest.OpenAIChunk
		for i, name := range strings.Split(names, ",") {
			next := chattest.OpenAIToolCallChunk("read_skill", `{"name":"`+name+`"}`)
			if i == 0 {
				chunk = next
				continue
			}
			toolCall := next.Choices[0].ToolCalls[0]
			toolCall.Index = i
			chunk.Choices[0].ToolCalls = append(chunk.Choices[0].ToolCalls, toolCall)
		}
		return chattest.OpenAIStreamingResponse(chunk)
	})
	return recorder, url
}

func (r *skillRecordingOpenAI) recorded() []skillRecordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]skillRecordedCall(nil), r.calls...)
}

func TestOrganizationSkillsFollowChatOwnerAccess(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	recorder, openAIURL := newSkillRecordingOpenAI(t)
	member, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	outsider := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: outsider.ID, OrganizationID: org.ID})
	team := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: member.ID, GroupID: team.ID})

	everyone := database.ChatACL{org.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}}
	teamOnly := dbgen.OrganizationSkill(t, db, database.Skill{
		OrganizationID: uuid.NullUUID{UUID: org.ID, Valid: true},
		Name:           "team-review",
		Description:    "Team review process",
		Content:        "---\nname: team-review\ndescription: Team review process\n---\n\nTeam review instructions.\n",
		GroupACL:       database.ChatACL{team.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
		UserACL:        database.ChatACL{},
	})
	dbgen.OrganizationSkill(t, db, database.Skill{
		OrganizationID: uuid.NullUUID{UUID: org.ID, Valid: true},
		Name:           "org-wide",
		Description:    "Org-wide process",
		GroupACL:       everyone,
		UserACL:        database.ChatACL{},
	})
	dbgen.OrganizationSkill(t, db, database.Skill{
		OrganizationID: uuid.NullUUID{UUID: org.ID, Valid: true},
		Name:           "org-disabled",
		Description:    "Disabled org process",
		GroupACL:       everyone,
		UserACL:        database.ChatACL{},
	})
	_, err := db.UpdateOrganizationSkillByOrganizationIDAndName(ctx, database.UpdateOrganizationSkillByOrganizationIDAndNameParams{
		Enabled:        sql.NullBool{Bool: false, Valid: true},
		OrganizationID: org.ID,
		Name:           "org-disabled",
	})
	require.NoError(t, err)
	for _, name := range []string{"personal-on", "personal-off"} {
		_, err := db.InsertUserSkill(ctx, database.InsertUserSkillParams{
			ID:          uuid.New(),
			UserID:      member.ID,
			Name:        name,
			Description: "Personal process " + name,
			Content:     "---\nname: " + name + "\ndescription: Personal process " + name + "\n---\n\nPersonal instructions.\n",
		})
		require.NoError(t, err)
	}
	_, err = db.UpdateUserSkillByUserIDAndName(ctx, database.UpdateUserSkillByUserIDAndNameParams{
		Enabled: sql.NullBool{Bool: false, Valid: true},
		UserID:  member.ID,
		Name:    "personal-off",
	})
	require.NoError(t, err)

	// The dbauthz wrapper applies the per-skill ACL in production.
	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		coderdtest.AccessControlStorePointer(),
	)
	server := newActiveTestServer(t, authzDB, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	runTurn := func(t *testing.T, userID uuid.UUID, chatID uuid.UUID, text string) (uuid.UUID, []skillRecordedCall) {
		t.Helper()
		subject, _, err := httpmw.UserRBACSubject(dbauthz.AsSystemRestricted(ctx), db, userID, rbac.ScopeAll)
		require.NoError(t, err)
		userCtx := dbauthz.As(ctx, subject)
		before := len(recorder.recorded())
		content := []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}
		if chatID == uuid.Nil {
			chat, err := server.CreateChat(userCtx, chatd.CreateOptions{
				OrganizationID:     org.ID,
				OwnerID:            userID,
				Title:              "org-skills",
				ModelConfigID:      model.ID,
				InitialUserContent: content,
			})
			require.NoError(t, err)
			chatID = chat.ID
		} else {
			_, err := server.SendMessage(userCtx, chatd.SendMessageOptions{
				ChatID:    chatID,
				CreatedBy: userID,
				Content:   content,
			})
			require.NoError(t, err)
		}
		waitForChatProcessed(ctx, t, db, chatID, server)
		chat, err := db.GetChatByID(ctx, chatID)
		require.NoError(t, err)
		require.NotEqual(t, database.ChatStatusError, chat.Status, "last_error=%q", chatLastErrorMessage(chat.LastError))
		return chatID, recorder.recorded()[before:]
	}

	// A team member sees enabled personal and org skills, including the
	// team-only one, and can read it. Disabled skills are not offered.
	memberChatID, calls := runTurn(t, member.ID, uuid.Nil, readSkillsDirective+"team-review,org-disabled,personal-off")
	require.Len(t, calls, 2)
	require.Contains(t, calls[0].system, "- team-review: Team review process\n")
	require.Contains(t, calls[0].system, "- org-wide: Org-wide process\n")
	require.Contains(t, calls[0].system, "- personal-on: Personal process personal-on\n")
	require.NotContains(t, calls[0].system, "org-disabled")
	require.NotContains(t, calls[0].system, "personal-off")
	require.Len(t, calls[1].toolResults, 3)
	require.Contains(t, calls[1].toolResults[0], "Team review instructions.")
	require.Equal(t, `skill "org-disabled" not found`, calls[1].toolResults[1])
	require.Equal(t, `skill "personal-off" not found`, calls[1].toolResults[2])

	// An org member outside the team, with no personal or workspace skills,
	// gets read_skill for the org-wide skill only.
	_, calls = runTurn(t, outsider.ID, uuid.Nil, readSkillsDirective+"org-wide,team-review")
	require.Len(t, calls, 2)
	require.Contains(t, calls[0].system, "- org-wide: Org-wide process\n")
	require.NotContains(t, calls[0].system, "team-review")
	require.Len(t, calls[1].toolResults, 2)
	require.Contains(t, calls[1].toolResults[0], "Fixture instructions.")
	require.Equal(t, `skill "team-review" not found`, calls[1].toolResults[1])

	// Revoking the team grant takes effect on the member's next step.
	_, err = db.UpdateOrganizationSkillACLByID(ctx, database.UpdateOrganizationSkillACLByIDParams{
		ID:       teamOnly.ID,
		GroupACL: database.ChatACL{},
		UserACL:  database.ChatACL{},
	})
	require.NoError(t, err)
	_, calls = runTurn(t, member.ID, memberChatID, readSkillsDirective+"team-review")
	require.Len(t, calls, 2)
	require.NotContains(t, calls[0].system, "team-review")
	require.Contains(t, calls[0].system, "- org-wide: Org-wide process\n")
	require.Equal(t, `skill "team-review" not found`, calls[1].toolResults[len(calls[1].toolResults)-1])
}
