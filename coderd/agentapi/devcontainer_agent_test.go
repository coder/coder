package agentapi_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func devContainerAgentAPI(t *testing.T, log slog.Logger) *agentapi.DevContainerAgentAPI {
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

	accessControlStore := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var acs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
	accessControlStore.Store(&acs)

	auth := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())

	return &agentapi.DevContainerAgentAPI{
		AgentID: agent.ID,
		AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
			return agent, nil
		},
		Clock:    clock,
		Database: dbauthz.New(db, auth, log, accessControlStore),
	}
}

func TestDevContainerAgentAPI(t *testing.T) {
	t.Parallel()

	log := testutil.Logger(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	api := devContainerAgentAPI(t, log)

	// Given: There are no dev container agents.
	listResp, err := api.ListDevContainerAgents(ctx, &proto.ListDevContainerAgentsRequest{})
	require.NoError(t, err)
	require.Len(t, listResp.Agents, 0)

	// When: We create a dev container agent.
	createResp, err := api.CreateDevContainerAgent(ctx, &proto.CreateDevContainerAgentRequest{
		Name:            "some-child-agent",
		Directory:       "/workspaces/coder",
		Architecture:    "amd64",
		OperatingSystem: "linux",
	})
	require.NoError(t, err)

	agentID, err := uuid.FromBytes(createResp.Id)
	require.NoError(t, err)

	// Then: We expect this dev container agent to be created.
	agent, err := api.Database.GetWorkspaceAgentByID(ctx, agentID)
	require.NoError(t, err)
	require.Equal(t, "/workspaces/coder", agent.Directory)
	require.Equal(t, "amd64", agent.Architecture)
	require.Equal(t, "linux", agent.OperatingSystem)
	require.Equal(t, "some-child-agent", agent.Name)

	// Then: We expect to be able to list this dev container agent.
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
