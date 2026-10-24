package chatd_test

import (
	"database/sql"
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

func TestProjectSkillsFollowProjectAccess(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	recorder, openAIURL := newSkillRecordingOpenAI(t)
	owner, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	// The teammate is an organization admin, whose role can read every
	// project in the organization. Only the project share may grant
	// project skills, as for project instructions and memory.
	teammate := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID: teammate.ID, OrganizationID: org.ID, Roles: []string{rbac.RoleOrgAdmin()},
	})
	project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: owner.ID})
	projectACL := database.ChatACL{teammate.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}}
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID: project.ID, UserACL: projectACL, GroupACL: database.ChatACL{},
	}))

	skillMarkdown := func(name, description, body string) string {
		return "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body + "\n"
	}
	for _, skill := range []struct{ name, description string }{
		{"project-review", "Project review process"},
		{"shared", "Project shared process"},
		{"project-disabled", "Disabled project process"},
		{"project-manual", "Manual project process"},
	} {
		_, err := db.InsertProjectSkill(ctx, database.InsertProjectSkillParams{
			ID:             uuid.New(),
			ProjectID:      project.ID,
			Name:           skill.name,
			Description:    skill.description,
			Content:        skillMarkdown(skill.name, skill.description, "Instructions for "+skill.name+"."),
			ModelInvocable: sql.NullBool{Bool: skill.name != "project-manual", Valid: true},
		})
		require.NoError(t, err)
	}
	_, err := db.UpdateProjectSkillByProjectIDAndName(ctx, database.UpdateProjectSkillByProjectIDAndNameParams{
		Enabled:   sql.NullBool{Bool: false, Valid: true},
		ProjectID: project.ID,
		Name:      "project-disabled",
	})
	require.NoError(t, err)
	insertPersonalSkill := func(userID uuid.UUID, name, description string) {
		t.Helper()
		_, err := db.InsertUserSkill(ctx, database.InsertUserSkillParams{
			ID:          uuid.New(),
			UserID:      userID,
			Name:        name,
			Description: description,
			Content:     skillMarkdown(name, description, "Personal instructions."),
		})
		require.NoError(t, err)
	}
	insertPersonalSkill(owner.ID, "shared", "Personal shared process")

	// The dbauthz wrapper applies the project ACL in production.
	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		coderdtest.AccessControlStorePointer(),
	)
	server := newActiveTestServer(t, authzDB, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	// runTurn creates a chat from opts when chatID is nil, otherwise it
	// sends text to chatID, and returns the model calls of that turn.
	runTurn := func(t *testing.T, userID, chatID uuid.UUID, opts chatd.CreateOptions, text string) (uuid.UUID, []skillRecordedCall) {
		t.Helper()
		subject, _, err := httpmw.UserRBACSubject(dbauthz.AsSystemRestricted(ctx), db, userID, rbac.ScopeAll)
		require.NoError(t, err)
		userCtx := dbauthz.As(ctx, subject)
		before := len(recorder.recorded())
		content := []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}
		if chatID == uuid.Nil {
			opts.OrganizationID = org.ID
			opts.OwnerID = userID
			opts.Title = "project-skills"
			opts.ModelConfigID = model.ID
			opts.InitialUserContent = content
			chat, err := server.CreateChat(userCtx, opts)
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
		calls := recorder.recorded()[before:]
		require.Len(t, calls, 2)
		return chatID, calls
	}
	lastToolResults := func(call skillRecordedCall, n int) []string {
		require.GreaterOrEqual(t, len(call.toolResults), n)
		return call.toolResults[len(call.toolResults)-n:]
	}
	inProject := chatd.CreateOptions{ProjectID: uuid.NullUUID{UUID: project.ID, Valid: true}}

	// A project chat lists enabled project skills. A name shared with a
	// personal skill gets qualified aliases, and disabled skills and skills
	// that are not model-invocable are hidden.
	projectChatID, calls := runTurn(t, owner.ID, uuid.Nil, inProject, readSkillsDirective+"project-review,project/shared,project-disabled,project-manual")
	require.Contains(t, calls[0].system, "- project-review: Project review process\n")
	require.Contains(t, calls[0].system, "- project/shared: Project shared process\n")
	require.Contains(t, calls[0].system, "- personal/shared: Personal shared process\n")
	require.NotContains(t, calls[0].system, "project-disabled")
	require.NotContains(t, calls[0].system, "project-manual")
	results := lastToolResults(calls[1], 4)
	require.Contains(t, results[0], "Instructions for project-review.")
	require.Contains(t, results[1], "Instructions for shared.")
	require.Equal(t, `skill "project-disabled" not found`, results[2])
	require.Equal(t, `skill "project-manual" not found`, results[3])

	// A chat outside the project does not see project skills.
	_, calls = runTurn(t, owner.ID, uuid.Nil, chatd.CreateOptions{}, readSkillsDirective+"project-review")
	require.NotContains(t, calls[0].system, "project-review")
	require.Contains(t, calls[0].system, "- shared: Personal shared process\n")
	require.Equal(t, []string{`skill "project-review" not found`}, lastToolResults(calls[1], 1))

	// A subagent inherits the project of its root chat.
	rootChat := uuid.NullUUID{UUID: projectChatID, Valid: true}
	_, calls = runTurn(t, owner.ID, uuid.Nil, chatd.CreateOptions{ParentChatID: rootChat, RootChatID: rootChat}, readSkillsDirective+"project-review")
	require.Contains(t, calls[0].system, "- project-review: Project review process\n")
	require.Contains(t, lastToolResults(calls[1], 1)[0], "Instructions for project-review.")

	// A user the project is shared with gets its skills, with bare names
	// because they have no colliding personal skill. The teammate has no
	// other skills yet, so project skills alone enable read_skill.
	teammateChatID, calls := runTurn(t, teammate.ID, uuid.Nil, inProject, readSkillsDirective+"shared")
	require.Contains(t, calls[0].system, "- shared: Project shared process\n")
	require.Contains(t, lastToolResults(calls[1], 1)[0], "Instructions for shared.")

	// Revoking the share removes project skills on the next turn, even
	// though the admin role still reads the project. The teammate's personal
	// skill keeps read_skill available to observe that.
	insertPersonalSkill(teammate.ID, "teammate-notes", "Teammate notes")
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID: project.ID, UserACL: database.ChatACL{}, GroupACL: database.ChatACL{},
	}))
	_, calls = runTurn(t, teammate.ID, teammateChatID, chatd.CreateOptions{}, readSkillsDirective+"shared")
	require.NotContains(t, calls[0].system, "Project shared process")
	require.Equal(t, []string{`skill "shared" not found`}, lastToolResults(calls[1], 1))

	// A deleted project skill is gone on the next turn.
	_, err = db.DeleteProjectSkillByProjectIDAndName(ctx, database.DeleteProjectSkillByProjectIDAndNameParams{
		ProjectID: project.ID,
		Name:      "project-review",
	})
	require.NoError(t, err)
	_, calls = runTurn(t, owner.ID, projectChatID, chatd.CreateOptions{}, readSkillsDirective+"project-review")
	require.NotContains(t, calls[0].system, "project-review: Project review process")
	require.Equal(t, []string{`skill "project-review" not found`}, lastToolResults(calls[1], 1))
}
