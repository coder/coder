package chatd

import (
	"context"
	"database/sql"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

func TestACPSessionDatabaseBindings(t *testing.T) {
	t.Parallel()
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID, ActiveVersionID: version.ID})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{OwnerID: user.ID, OrganizationID: org.ID, TemplateID: template.ID})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: job.ID})
	agentA := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: resource.ID})
	agentB := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: resource.ID})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{OwnerID: user.ID, OrganizationID: org.ID, LastModelConfigID: model.ID, WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true}, AgentID: uuid.NullUUID{UUID: agentA.ID, Valid: true}})
	args := database.InsertAgentsACPSessionParams{ID: uuid.New(), ChatID: chat.ID, OrganizationID: org.ID, WorkspaceID: ws.ID, AgentID: agentA.ID, HarnessSlug: "test", WorkingDirectory: "/work", SessionID: "native"}
	session, err := db.InsertAgentsACPSession(ctx, args)
	require.NoError(t, err)
	retry, err := db.InsertAgentsACPSession(ctx, args)
	require.NoError(t, err)
	require.Equal(t, session, retry)
	collision := args
	collision.ID = uuid.New()
	_, err = db.InsertAgentsACPSession(ctx, collision)
	require.True(t, database.IsUniqueViolation(err))
	other := dbgen.Chat(t, db, database.Chat{OwnerID: user.ID, OrganizationID: org.ID, LastModelConfigID: model.ID, WorkspaceID: chat.WorkspaceID, AgentID: chat.AgentID})
	_, err = db.GetAgentsACPSessionByIDAndChatID(ctx, database.GetAgentsACPSessionByIDAndChatIDParams{ID: session.ID, ChatID: other.ID})
	require.ErrorIs(t, err, sql.ErrNoRows)
	collision = args
	collision.ChatID = other.ID
	_, err = db.InsertAgentsACPSession(ctx, collision)
	require.ErrorIs(t, err, sql.ErrNoRows)
	// Timestamp updates are guarded by the current chat agent.
	chat, err = db.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{ID: chat.ID, WorkspaceID: chat.WorkspaceID, AgentID: uuid.NullUUID{UUID: agentB.ID, Valid: true}})
	require.NoError(t, err)
	_, err = db.UpdateAgentsACPSessionUpdatedAt(ctx, database.UpdateAgentsACPSessionUpdatedAtParams{ID: session.ID, ChatID: chat.ID, AgentID: agentA.ID})
	require.ErrorIs(t, err, sql.ErrNoRows)
	// Physical deletion preserves the mapping and its native recovery identity.
	_, err = sqlDB.ExecContext(ctx, "DELETE FROM workspace_agents WHERE id = $1", agentA.ID)
	require.NoError(t, err)
	session, err = db.GetAgentsACPSessionByIDAndChatID(ctx, database.GetAgentsACPSessionByIDAndChatIDParams{ID: session.ID, ChatID: chat.ID})
	require.NoError(t, err)
	require.Equal(t, "native", session.SessionID)
	conn := agentconnmock.NewMockAgentConn(gomock.NewController(t))
	opts := acpToolOptions{db: db, chatID: chat.ID, resolve: func(context.Context) (workspacesdk.AgentConn, database.Chat, uuid.UUID, error) {
		return conn, chat, agentB.ID, nil
	}}
	_, resolvedAgentID, err := opts.connection(ctx, session)
	require.NoError(t, err)
	require.Equal(t, agentB.ID, resolvedAgentID)
	native := acpNativeID(session)
	conn.EXPECT().SendACPMessage(gomock.Any(), native, gomock.Any()).Return(workspacesdk.ACPMessageResponse{}, xerrors.New("native session cannot be restored"))
	result, err := opts.message(workspacesdk.WithToolCallID(ctx, uuid.New()), acpMessageArgs{SessionID: session.ID.String(), Message: "resume"}, fantasy.ToolCall{})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content, "native session cannot be restored")
	count, err := db.CountAgentsACPSessionsByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	second := args
	second.ID = uuid.New()
	second.SessionID = "second"
	second.AgentID = agentB.ID
	_, err = db.InsertAgentsACPSession(ctx, second)
	require.NoError(t, err)
	page1, err := db.ListAgentsACPSessionsByChatID(ctx, database.ListAgentsACPSessionsByChatIDParams{ChatID: chat.ID, LimitValue: 1})
	require.NoError(t, err)
	require.Len(t, page1, 1)
	_, _, err = opts.connection(ctx, page1[0])
	require.NoError(t, err)
	page2, err := db.ListAgentsACPSessionsByChatID(ctx, database.ListAgentsACPSessionsByChatIDParams{ChatID: chat.ID, LimitValue: 1, OffsetValue: 1})
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.NotEqual(t, page1[0].ID, page2[0].ID)
	// Switching workspaces rejects both stale updates and routed operations.
	newWorkspace := dbgen.Workspace(t, db, database.WorkspaceTable{OwnerID: user.ID, OrganizationID: org.ID, TemplateID: template.ID})
	chat, err = db.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{ID: chat.ID, WorkspaceID: uuid.NullUUID{UUID: newWorkspace.ID, Valid: true}, AgentID: chat.AgentID})
	require.NoError(t, err)
	_, err = db.UpdateAgentsACPSessionUpdatedAt(ctx, database.UpdateAgentsACPSessionUpdatedAtParams{ID: session.ID, ChatID: chat.ID, AgentID: agentB.ID})
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, _, err = opts.connection(ctx, session)
	require.ErrorIs(t, err, errACPWorkspaceChanged)
}
