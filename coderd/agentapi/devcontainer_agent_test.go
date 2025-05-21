package agentapi_test

import (
	"cmp"
	"context"
	"slices"
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

	// When: We create two dev container agents.
	createResp, err := api.CreateDevContainerAgent(ctx, &proto.CreateDevContainerAgentRequest{
		Name:            "some-child-agent",
		Directory:       "/workspaces/wibble",
		Architecture:    "amd64",
		OperatingSystem: "linux",
	})
	require.NoError(t, err)

	childAgentOneID, err := uuid.FromBytes(createResp.Id)
	require.NoError(t, err)

	createResp, err = api.CreateDevContainerAgent(ctx, &proto.CreateDevContainerAgentRequest{
		Name:            "some-other-child-agent",
		Directory:       "/workspaces/wobble",
		Architecture:    "amd64",
		OperatingSystem: "linux",
	})
	require.NoError(t, err)

	childAgentTwoID, err := uuid.FromBytes(createResp.Id)
	require.NoError(t, err)

	// Then: We expect these dev container agents to be created.
	agentOne, err := api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), childAgentOneID) //nolint:gocritic // this is a test
	require.NoError(t, err)
	require.Equal(t, "/workspaces/wibble", agentOne.Directory)
	require.Equal(t, "amd64", agentOne.Architecture)
	require.Equal(t, "linux", agentOne.OperatingSystem)
	require.Equal(t, "some-child-agent", agentOne.Name)

	agentTwo, err := api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), childAgentTwoID) //nolint:gocritic // this is a test
	require.NoError(t, err)
	require.Equal(t, "/workspaces/wobble", agentTwo.Directory)
	require.Equal(t, "amd64", agentTwo.Architecture)
	require.Equal(t, "linux", agentTwo.OperatingSystem)
	require.Equal(t, "some-other-child-agent", agentTwo.Name)

	// Then: We expect to be able to list these dev container agents.
	createdAgents := []database.WorkspaceAgent{agentOne, agentTwo}
	slices.SortFunc(createdAgents, func(a, b database.WorkspaceAgent) int {
		return cmp.Compare(a.ID.String(), b.ID.String())
	})

	listResp, err = api.ListDevContainerAgents(ctx, &proto.ListDevContainerAgentsRequest{})
	require.NoError(t, err)
	require.Len(t, listResp.Agents, len(createdAgents))

	listedAgents := listResp.Agents
	slices.SortFunc(listedAgents, func(a, b *proto.ListDevContainerAgentsResponse_DevContainerAgent) int {
		return cmp.Compare(string(a.Id), string(b.Id))
	})

	for i, agent := range listedAgents {
		require.Equal(t, createdAgents[i].ID[:], agent.Id)
		require.Equal(t, createdAgents[i].Name, agent.Name)
	}

	// When: We delete a dev container agent.
	_, err = api.DeleteDevContainerAgent(ctx, &proto.DeleteDevContainerAgentRequest{
		Id: agentOne.ID[:],
	})
	require.NoError(t, err)

	// Then: We expect this dev container agent to be deleted.
	listResp, err = api.ListDevContainerAgents(ctx, &proto.ListDevContainerAgentsRequest{})
	require.NoError(t, err)
	require.Len(t, listResp.Agents, 1)
	require.Equal(t, agentTwo.ID[:], listResp.Agents[0].Id)

	// When: We delete the other dev container agent.
	_, err = api.DeleteDevContainerAgent(ctx, &proto.DeleteDevContainerAgentRequest{
		Id: agentTwo.ID[:],
	})
	require.NoError(t, err)

	// Then: We expect this other dev container agent to be deleted.
	listResp, err = api.ListDevContainerAgents(ctx, &proto.ListDevContainerAgentsRequest{})
	require.NoError(t, err)
	require.Len(t, listResp.Agents, 0)
}
