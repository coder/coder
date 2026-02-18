package chatd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/database/pubsub/psmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
)

type templateSelectionModel struct {
	generateCall   *fantasy.Call
	generateBlocks []fantasy.Content
}

func (*templateSelectionModel) Provider() string {
	return "fake"
}

func (*templateSelectionModel) Model() string {
	return "fake"
}

func (m *templateSelectionModel) Generate(_ context.Context, call fantasy.Call) (*fantasy.Response, error) {
	captured := call
	m.generateCall = &captured
	return &fantasy.Response{Content: m.generateBlocks}, nil
}

func (*templateSelectionModel) Stream(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (*templateSelectionModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (*templateSelectionModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func TestSelectTemplateWithModel_SetsToolChoiceNone(t *testing.T) {
	t.Parallel()

	candidateID := uuid.New()
	candidates := []database.Template{
		{
			ID:          candidateID,
			Name:        "starter",
			DisplayName: "Starter",
			Description: "Starter template",
		},
	}

	model := &templateSelectionModel{
		generateBlocks: []fantasy.Content{
			fantasy.TextContent{
				Text: fmt.Sprintf(`{"template_id":"%s","reason":"best match"}`, candidateID),
			},
		},
	}

	selection, err := selectTemplateWithModel(context.Background(), model, "create a workspace", candidates)
	require.NoError(t, err)
	require.Equal(t, candidateID, selection.ID)
	require.NotNil(t, model.generateCall)
	require.NotNil(t, model.generateCall.ToolChoice)
	require.Equal(t, fantasy.ToolChoiceNone, *model.generateCall.ToolChoice)
}

func TestWorkspaceCreator_CreateWorkspace_MultiplePromptMatchesWithoutModel(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	creator := newTestWorkspaceCreator(
		t,
		db,
		nil,
		[]database.Template{
			{ID: uuid.New(), Name: "python-starter", DisplayName: "Python Starter"},
			{ID: uuid.New(), Name: "python-web", DisplayName: "Python Web"},
		},
		nil,
	)

	result, err := creator.CreateWorkspace(context.Background(), CreateWorkspaceToolRequest{
		Chat: database.Chat{
			OwnerID: uuid.New(),
		},
		Prompt: "create a python workspace for web development",
	})
	require.NoError(t, err)
	require.False(t, result.Created)
	require.Equal(t, "multiple templates matched and no model is available to disambiguate", result.Reason)
}

func TestWorkspaceCreator_CreateWorkspace_UsesModelToDisambiguatePromptMatches(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	templateStarterID := uuid.New()
	templateWebID := uuid.New()
	workspaceID := uuid.New()
	workspaceAgentID := uuid.New()
	jobID := uuid.New()

	db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).Return(
		[]database.WorkspaceAgent{
			{ID: workspaceAgentID},
		},
		nil,
	)

	var capturedCreateReq codersdk.CreateWorkspaceRequest
	creator := newTestWorkspaceCreator(
		t,
		db,
		nil,
		[]database.Template{
			{ID: templateStarterID, Name: "python-starter", DisplayName: "Python Starter"},
			{ID: templateWebID, Name: "python-web", DisplayName: "Python Web"},
		},
		func(_ context.Context, _ *http.Request, _ uuid.UUID, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
			capturedCreateReq = req
			return codersdk.Workspace{
				ID:        workspaceID,
				OwnerName: "alice",
				Name:      "python-web-alice",
				LatestBuild: codersdk.WorkspaceBuild{
					Job: codersdk.ProvisionerJob{ID: jobID},
				},
			}, nil
		},
	)

	model := &templateSelectionModel{
		generateBlocks: []fantasy.Content{
			fantasy.TextContent{
				Text: fmt.Sprintf(`{"template_id":"%s","reason":"web stack"}`, templateWebID),
			},
		},
	}

	result, err := creator.CreateWorkspace(context.Background(), CreateWorkspaceToolRequest{
		Chat: database.Chat{
			OwnerID: uuid.New(),
		},
		Model:  model,
		Prompt: "create a python web workspace",
	})
	require.NoError(t, err)
	require.True(t, result.Created)
	require.Equal(t, workspaceID, result.WorkspaceID)
	require.Equal(t, workspaceAgentID, result.WorkspaceAgentID)
	require.Equal(t, "alice/python-web-alice", result.WorkspaceName)
	require.Equal(t, "https://coder.example/@alice/python-web-alice", result.WorkspaceURL)

	require.Equal(t, templateWebID, capturedCreateReq.TemplateID)
	require.Equal(t, uuid.Nil, capturedCreateReq.TemplateVersionID)
	require.NotEmpty(t, capturedCreateReq.Name)
	require.NotNil(t, model.generateCall)
	require.NotNil(t, model.generateCall.ToolChoice)
	require.Equal(t, fantasy.ToolChoiceNone, *model.generateCall.ToolChoice)
}

func TestWorkspaceCreator_CreateWorkspace_RejectsMismatchedTemplateAndVersion(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	templateID := uuid.New()
	templateVersionTemplateID := uuid.New()
	templateVersionID := uuid.New()

	db.EXPECT().GetTemplateVersionByID(gomock.Any(), templateVersionID).Return(database.TemplateVersion{
		ID: templateVersionID,
		TemplateID: uuid.NullUUID{
			UUID:  templateVersionTemplateID,
			Valid: true,
		},
	}, nil)
	db.EXPECT().GetTemplateByID(gomock.Any(), templateVersionTemplateID).Return(database.Template{
		ID:   templateVersionTemplateID,
		Name: "python-starter",
	}, nil)

	creator := newTestWorkspaceCreator(t, db, nil, nil, nil)

	result, err := creator.CreateWorkspace(context.Background(), CreateWorkspaceToolRequest{
		Chat: database.Chat{
			OwnerID: uuid.New(),
		},
		Prompt: "create workspace",
		Spec: json.RawMessage(
			fmt.Sprintf(`{"name":"proj","template_id":"%s","template_version_id":"%s"}`, templateID, templateVersionID),
		),
	})
	require.NoError(t, err)
	require.False(t, result.Created)
	require.Equal(t, "template_id does not match template_version_id", result.Reason)
}

func TestWorkspaceCreator_StreamWorkspaceBuildLogs_InitialAndNotification(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ps := psmock.NewMockPubsub(ctrl)

	creator := &workspaceCreator{
		adapter: WorkspaceCreatorAdapterFuncs{
			DatabaseStore: db,
			PubsubStore:   ps,
			LoggerStore:   testutil.Logger(t),
		},
	}

	jobID := uuid.New()
	running := database.ProvisionerJob{
		ID:        jobID,
		JobStatus: database.ProvisionerJobStatusRunning,
	}
	initialLog := database.ProvisionerJobLog{
		ID:     1,
		Source: database.LogSourceProvisioner,
		Level:  database.LogLevelInfo,
		Stage:  "plan",
		Output: "planning infrastructure",
	}
	notificationLog := database.ProvisionerJobLog{
		ID:     2,
		Source: database.LogSourceProvisionerDaemon,
		Level:  database.LogLevelDebug,
		Stage:  "apply",
		Output: "apply complete",
	}

	msg, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{
		EndOfLogs: true,
	})
	require.NoError(t, err)

	subscribeCall := ps.EXPECT().SubscribeWithErr(
		provisionersdk.ProvisionerJobLogsNotifyChannel(jobID),
		gomock.Any(),
	).DoAndReturn(func(_ string, listener pubsub.ListenerWithErr) (func(), error) {
		listener(context.Background(), msg, nil)
		return func() {}, nil
	})

	gomock.InOrder(
		db.EXPECT().GetProvisionerLogsAfterID(gomock.Any(), database.GetProvisionerLogsAfterIDParams{
			JobID:        jobID,
			CreatedAfter: 0,
		}).Return([]database.ProvisionerJobLog{initialLog}, nil),
		db.EXPECT().GetProvisionerJobByID(gomock.Any(), jobID).Return(running, nil),
		subscribeCall,
		db.EXPECT().GetProvisionerLogsAfterID(gomock.Any(), database.GetProvisionerLogsAfterIDParams{
			JobID:        jobID,
			CreatedAfter: 1,
		}).Return([]database.ProvisionerJobLog{}, nil),
		db.EXPECT().GetProvisionerJobByID(gomock.Any(), jobID).Return(running, nil),
		db.EXPECT().GetProvisionerLogsAfterID(gomock.Any(), database.GetProvisionerLogsAfterIDParams{
			JobID:        jobID,
			CreatedAfter: 1,
		}).Return([]database.ProvisionerJobLog{notificationLog}, nil),
	)

	var emitted []CreateWorkspaceBuildLog
	creator.streamWorkspaceBuildLogs(context.Background(), jobID, func(log CreateWorkspaceBuildLog) {
		emitted = append(emitted, log)
	})

	require.Equal(t, []CreateWorkspaceBuildLog{
		{
			Source: string(initialLog.Source),
			Level:  string(initialLog.Level),
			Stage:  initialLog.Stage,
			Output: initialLog.Output,
		},
		{
			Source: string(notificationLog.Source),
			Level:  string(notificationLog.Level),
			Stage:  notificationLog.Stage,
			Output: notificationLog.Output,
		},
	}, emitted)
}

func newTestWorkspaceCreator(
	t *testing.T,
	db database.Store,
	ps pubsub.Pubsub,
	templates []database.Template,
	createWorkspace func(context.Context, *http.Request, uuid.UUID, codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error),
) *workspaceCreator {
	t.Helper()

	return &workspaceCreator{
		adapter: WorkspaceCreatorAdapterFuncs{
			PrepareWorkspaceCreateFunc: func(ctx context.Context, _ database.Chat) (context.Context, *http.Request, string, error) {
				return ctx, httptest.NewRequest(http.MethodPost, "/api/v2/workspaces", nil), "https://coder.example", nil
			},
			AuthorizedTemplatesFunc: func(context.Context, *http.Request) ([]database.Template, error) {
				return templates, nil
			},
			CreateWorkspaceFunc: func(
				ctx context.Context,
				r *http.Request,
				ownerID uuid.UUID,
				req codersdk.CreateWorkspaceRequest,
			) (codersdk.Workspace, error) {
				if createWorkspace == nil {
					return codersdk.Workspace{}, xerrors.New("unexpected create workspace call")
				}
				return createWorkspace(ctx, r, ownerID, req)
			},
			DatabaseStore: db,
			PubsubStore:   ps,
			LoggerStore:   testutil.Logger(t),
		},
	}
}
