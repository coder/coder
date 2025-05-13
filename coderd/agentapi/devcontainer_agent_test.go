package agentapi_test

import (
	"context"
	"testing"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func devContainerAgentAPI(t *testing.T) *agentapi.DevContainerAgentAPI {
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{Valid: true, UUID: template.ID},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		OwnerID:        user.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		Type: database.ProvisionerJobTypeWorkspaceBuild,
	})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		JobID:             job.ID,
		WorkspaceID:       workspace.ID,
		TemplateVersionID: templateVersion.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		JobID: build.JobID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})

	clock := quartz.NewMock(t)

	return &agentapi.DevContainerAgentAPI{
		AgentID: agent.ID,
		AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
			return agent, nil
		},
		Clock:    clock,
		Database: db,
	}
}

func TestDevContainerAgentAPI(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	api := devContainerAgentAPI(t)

	// Given: There are no dev container agents.
	listResp, err := api.ListDevContainerAgents(ctx, &proto.ListDevContainerAgentsRequest{})
	require.NoError(t, err)
	require.Len(t, listResp.Agents, 0)

	// When: We create a dev container agent.
	createResp, err := api.CreateDevContainerAgent(ctx, &proto.CreateDevContainerAgentRequest{
		Name:      "some-child-agent",
		Directory: "/workspaces/coder",
	})
	require.NoError(t, err)

	// Then: We expect this dev container agent to be created.
	listResp, err = api.ListDevContainerAgents(ctx, &proto.ListDevContainerAgentsRequest{})
	require.NoError(t, err)
	require.Len(t, listResp.Agents, 1)
	require.Equal(t, createResp.Id, listResp.Agents[0].Id)
	require.Equal(t, "some-child-agent", listResp.Agents[0].Name)

	// When: We delete a dev container agent.
	_, err = api.DeleteDevContainerAgent(ctx, &proto.DeleteDevContainerAgentRequest{
		Id: createResp.Id,
	})
	require.NoError(t, err)

	// Then: We expect this dev container agent to be deleted.
	listResp, err = api.ListDevContainerAgents(ctx, &proto.ListDevContainerAgentsRequest{})
	require.NoError(t, err)
	require.Len(t, listResp.Agents, 0)
}
