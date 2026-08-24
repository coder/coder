package chattool_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type subjectCaptureStore struct {
	database.Store
	t        *testing.T
	subjects []rbac.Subject
}

func (s *subjectCaptureStore) capture(ctx context.Context) {
	s.t.Helper()
	subject, ok := dbauthz.ActorFromContext(ctx)
	require.True(s.t, ok, "platform operation must carry a database authorization subject")
	s.subjects = append(s.subjects, subject)
}

func (s *subjectCaptureStore) GetTemplatesWithFilter(ctx context.Context, arg database.GetTemplatesWithFilterParams) ([]database.Template, error) {
	s.capture(ctx)
	return s.Store.GetTemplatesWithFilter(ctx, arg)
}

func (s *subjectCaptureStore) GetTemplateByID(ctx context.Context, id uuid.UUID) (database.Template, error) {
	s.capture(ctx)
	return s.Store.GetTemplateByID(ctx, id)
}

func (s *subjectCaptureStore) GetTemplateVersionByID(ctx context.Context, id uuid.UUID) (database.TemplateVersion, error) {
	s.capture(ctx)
	return s.Store.GetTemplateVersionByID(ctx, id)
}

func (s *subjectCaptureStore) requireAIAgentSubjects() {
	s.t.Helper()
	require.NotEmpty(s.t, s.subjects)
	for _, subject := range s.subjects {
		require.Equal(s.t, rbac.SubjectTypeAIAgent, subject.Type)
	}
}

func TestPlatformToolsUseAIAgentSubject(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         owner.ID,
		OrganizationID: org.ID,
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chatID := uuid.New()
	_ = dbgen.Chat(t, db, database.Chat{
		ID:                chatID,
		OwnerID:           owner.ID,
		OrganizationID:    org.ID,
		LastModelConfigID: model.ID,
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      owner.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		CreatedBy:       owner.ID,
		ActiveVersionID: version.ID,
		AgentsAllowed:   true,
	})

	agentUser, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        owner.ID,
		OrganizationID: org.ID,
		OriginType:     entity.CreationSiteTypeChat,
		OriginID:       chatID,
	})
	require.NoError(t, err)
	actorCtx := aiagentidentity.WithActor(ctx, aiagentidentity.AIAgentActor{
		AgentUserID: agentUser.ID,
		OwnerUserID: owner.ID,
		OriginType:  entity.CreationSiteTypeChat,
		OriginID:    chatID,
	})

	tests := []struct {
		name string
		run  func(*testing.T, *subjectCaptureStore)
	}{
		{
			name: "list templates",
			run: func(t *testing.T, store *subjectCaptureStore) {
				tool := chattool.ListTemplates(store, org.ID, chattool.ListTemplatesOptions{
					OwnerID: owner.ID,
					Logger:  slogtest.Make(t, nil),
					Clock:   quartz.NewReal(),
				})
				resp, err := tool.Run(actorCtx, fantasy.ToolCall{ID: uuid.NewString(), Name: "list_templates", Input: "{}"})
				require.NoError(t, err)
				require.False(t, resp.IsError, resp.Content)
			},
		},
		{
			name: "read template",
			run: func(t *testing.T, store *subjectCaptureStore) {
				tool := chattool.ReadTemplate(store, org.ID, chattool.ReadTemplateOptions{OwnerID: owner.ID})
				resp, err := tool.Run(actorCtx, fantasy.ToolCall{
					ID:    uuid.NewString(),
					Name:  "read_template",
					Input: `{"template_id":"` + template.ID.String() + `"}`,
				})
				require.NoError(t, err)
				require.False(t, resp.IsError, resp.Content)
			},
		},
		{
			name: "create workspace reads",
			run: func(t *testing.T, store *subjectCaptureStore) {
				stopErr := xerrors.New("stop after scoped reads")
				tool := chattool.CreateWorkspace(store, org.ID, chatID, chattool.CreateWorkspaceOptions{
					OwnerID: owner.ID,
					CreateFn: func(ctx context.Context, _ uuid.UUID, _ codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
						store.capture(ctx)
						return codersdk.Workspace{}, stopErr
					},
					Logger: slogtest.Make(t, nil),
				})
				resp, err := tool.Run(actorCtx, fantasy.ToolCall{
					ID:    uuid.NewString(),
					Name:  "create_workspace",
					Input: `{"template_id":"` + template.ID.String() + `"}`,
				})
				require.NoError(t, err)
				require.True(t, resp.IsError)
				require.Contains(t, resp.Content, stopErr.Error())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := &subjectCaptureStore{Store: db, t: t}
			test.run(t, store)
			store.requireAIAgentSubjects()
		})
	}
}
