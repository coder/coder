package chatd

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestResolveSkillInvocations(t *testing.T) {
	t.Parallel()

	requireUnavailable := func(t *testing.T, err error, want string) {
		t.Helper()
		var unavailable *SkillInvocationUnavailableError
		require.True(t, errors.As(err, &unavailable), "got %v", err)
		require.Equal(t, want, unavailable.Error())
	}

	t.Run("StoredSources", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)
		authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
		server := &Server{
			db:          authzDB,
			logger:      slogtest.Make(t, nil),
			experiments: codersdk.Experiments{codersdk.ExperimentChatProjects},
			modelConfigContext: func(ctx context.Context, ownerID uuid.UUID) (context.Context, error) {
				return callerModelConfigContext(ctx, authzDB, ownerID)
			},
		}

		skillMarkdown := func(name string) string {
			return "---\nname: " + name + "\ndescription: Test\n---\n\nInstructions for " + name + ".\n"
		}
		// Every invoked stored skill is hidden from the agent: users can
		// invoke skills the agent cannot use on its own.
		manualOnly := sql.NullBool{Bool: false, Valid: true}
		org := dbgen.Organization(t, db, database.Organization{})
		owner := dbgen.User(t, db, database.User{})
		other := dbgen.User(t, db, database.User{})
		for _, user := range []database.User{owner, other} {
			dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
		}
		for _, skill := range []struct {
			userID uuid.UUID
			name   string
		}{{owner.ID, "notes"}, {other.ID, "secret"}} {
			_, err := db.InsertUserSkill(ctx, database.InsertUserSkillParams{
				ID: uuid.New(), UserID: skill.userID, Name: skill.name, Content: skillMarkdown(skill.name), ModelInvocable: manualOnly,
			})
			require.NoError(t, err)
		}
		dbgen.OrganizationSkill(t, db, database.Skill{
			OrganizationID: uuid.NullUUID{UUID: org.ID, Valid: true},
			Name:           "team-review",
			Content:        skillMarkdown("team-review"),
			GroupACL:       database.ChatACL{org.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
			UserACL:        database.ChatACL{},
		})
		_, err := db.UpdateOrganizationSkillByOrganizationIDAndName(ctx, database.UpdateOrganizationSkillByOrganizationIDAndNameParams{
			ModelInvocable: manualOnly, OrganizationID: org.ID, Name: "team-review",
		})
		require.NoError(t, err)
		project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: owner.ID})
		_, err = db.InsertProjectSkill(ctx, database.InsertProjectSkillParams{
			ID: uuid.New(), ProjectID: project.ID, Name: "triage", Content: skillMarkdown("triage"), ModelInvocable: manualOnly,
		})
		require.NoError(t, err)

		projectChat := database.Chat{
			ID:             uuid.New(),
			OwnerID:        owner.ID,
			OrganizationID: org.ID,
			ProjectID:      uuid.NullUUID{UUID: project.ID, Valid: true},
		}
		parts := []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("go"),
			codersdk.ChatMessageSkillInvocation(codersdk.ChatSkillSourcePersonal, "notes"),
			codersdk.ChatMessageSkillInvocation(codersdk.ChatSkillSourceOrganization, "team-review"),
			codersdk.ChatMessageSkillInvocation(codersdk.ChatSkillSourceProject, "triage"),
		}
		resolved, err := server.resolveSkillInvocations(ctx, projectChat, parts)
		require.NoError(t, err)
		require.Equal(t, parts[0], resolved[0])
		for i, want := range []string{"notes", "team-review", "triage"} {
			require.Equal(t, "Instructions for "+want+".", resolved[i+1].SkillBody)
		}
		require.Empty(t, parts[1].SkillBody, "caller-owned parts must not change")

		// Another user's personal skill is not readable as the owner, and
		// a chat outside a project has no project skills.
		_, err = server.resolveSkillInvocations(ctx, projectChat, []codersdk.ChatMessagePart{
			codersdk.ChatMessageSkillInvocation(codersdk.ChatSkillSourcePersonal, "secret"),
		})
		requireUnavailable(t, err, "Skill personal/secret is not available.")
		plainChat := projectChat
		plainChat.ProjectID = uuid.NullUUID{}
		_, err = server.resolveSkillInvocations(ctx, plainChat, parts[3:])
		requireUnavailable(t, err, "Skill project/triage is not available.")
	})

	t.Run("WorkspaceFromPinnedContext", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := database.Chat{ID: uuid.New(), WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
		// The invoked workspace skill is hidden from the agent: users can
		// invoke skills the agent cannot use on its own.
		db.EXPECT().ListChatContextResourcesByChatID(gomock.Any(), chat.ID).
			Return([]database.ChatContextResource{{
				Source:   "/home/coder/.agents/skills/deploy",
				BodyKind: database.WorkspaceAgentContextBodyKindSkill,
				Body: mustMarshalContextBody(t, &agentproto.SkillMetaBody{
					Meta: []byte("---\nname: deploy\ndescription: Deploy\ndisable-model-invocation: true\n---\n\nDeploy steps.\n"),
					Name: "deploy",
				}),
				Status: database.WorkspaceAgentContextResourceStatusOk,
			}}, nil).Times(2)
		server := newPinServer(t, db)

		resolved, err := server.resolveSkillInvocations(context.Background(), chat, []codersdk.ChatMessagePart{
			codersdk.ChatMessageSkillInvocation(codersdk.ChatSkillSourceWorkspace, "deploy"),
		})
		require.NoError(t, err)
		require.Equal(t, "Deploy steps.", resolved[0].SkillBody)
		require.Equal(t, "/home/coder/.agents/skills/deploy", resolved[0].SkillDir)
		require.Equal(t, chat.WorkspaceID.UUID, resolved[0].SkillWorkspaceID)

		_, err = server.resolveSkillInvocations(context.Background(), chat, []codersdk.ChatMessagePart{
			codersdk.ChatMessageSkillInvocation(codersdk.ChatSkillSourceWorkspace, "missing"),
		})
		requireUnavailable(t, err, "Skill workspace/missing is not available.")
	})

	t.Run("RepeatedSkillLoadsOnce", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New()}
		db.EXPECT().GetUserSkillByUserIDAndName(gomock.Any(), database.GetUserSkillByUserIDAndNameParams{UserID: chat.OwnerID, Name: "notes"}).
			Return(database.Skill{Enabled: true, Content: "---\nname: notes\ndescription: Notes\n---\n\nTake notes.\n"}, nil).
			Times(1)
		server := newPinServer(t, db)
		invocation := codersdk.ChatMessageSkillInvocation(codersdk.ChatSkillSourcePersonal, "notes")

		resolved, err := server.resolveSkillInvocations(context.Background(), chat,
			slices.Repeat([]codersdk.ChatMessagePart{invocation}, MaxSkillInvocationsPerMessage))
		require.NoError(t, err)
		for _, part := range resolved {
			require.Equal(t, "Take notes.", part.SkillBody)
		}

		// One invocation over the cap is rejected before any skill loads.
		_, err = server.resolveSkillInvocations(context.Background(), chat,
			slices.Repeat([]codersdk.ChatMessagePart{invocation}, MaxSkillInvocationsPerMessage+1))
		require.ErrorIs(t, err, ErrTooManySkillInvocations)
	})
}
