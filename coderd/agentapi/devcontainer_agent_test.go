package agentapi_test

import (
	"cmp"
	"context"
	"database/sql"
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

func devContainerAgentAPI(t *testing.T, log slog.Logger) (*agentapi.DevContainerAgentAPI, database.WorkspaceResource) {
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
	}, resource
}

func TestDevContainerAgentAPI(t *testing.T) {
	t.Parallel()

	t.Run("CanCreateDevContainerAgent", func(t *testing.T) {
		t.Parallel()

		log := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		api, _ := devContainerAgentAPI(t, log)

		createResp, err := api.CreateDevContainerAgent(ctx, &proto.CreateDevContainerAgentRequest{
			Name:            "some-child-agent",
			Directory:       "/workspaces/wibble",
			Architecture:    "amd64",
			OperatingSystem: "linux",
		})
		require.NoError(t, err)

		agentID, err := uuid.FromBytes(createResp.Id)
		require.NoError(t, err)

		//nolint:gocritic // this is a test.
		agent, err := api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agentID)
		require.NoError(t, err)

		require.Equal(t, "some-child-agent", agent.Name)
		require.Equal(t, "/workspaces/wibble", agent.Directory)
		require.Equal(t, "amd64", agent.Architecture)
		require.Equal(t, "linux", agent.OperatingSystem)
	})

	t.Run("CanDeleteDevContainerAgent", func(t *testing.T) {
		t.Parallel()

		log := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		api, res := devContainerAgentAPI(t, log)

		// Given: A dev container agent.
		agent := dbgen.WorkspaceAgent(t, api.Database, database.WorkspaceAgent{
			ParentID:        uuid.NullUUID{Valid: true, UUID: api.AgentID},
			ResourceID:      res.ID,
			Name:            "some-child-agent",
			Directory:       "/workspaces/wibble",
			Architecture:    "amd64",
			OperatingSystem: "linux",
		})

		// When: We delete the dev container agent.
		_, err := api.DeleteDevContainerAgent(ctx, &proto.DeleteDevContainerAgentRequest{
			Id: agent.ID[:],
		})
		require.NoError(t, err)

		// Then: It is deleted.
		_, err = api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agent.ID)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("CanDeleteOneDevContainerAgentOfMany", func(t *testing.T) {
		t.Parallel()

		log := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		api, res := devContainerAgentAPI(t, log)

		// Given: Multiple dev container agents.
		agentOne := dbgen.WorkspaceAgent(t, api.Database, database.WorkspaceAgent{
			ParentID:        uuid.NullUUID{Valid: true, UUID: api.AgentID},
			ResourceID:      res.ID,
			Name:            "child-agent-one",
			Directory:       "/workspaces/wibble",
			Architecture:    "amd64",
			OperatingSystem: "linux",
		})

		agentTwo := dbgen.WorkspaceAgent(t, api.Database, database.WorkspaceAgent{
			ParentID:        uuid.NullUUID{Valid: true, UUID: api.AgentID},
			ResourceID:      res.ID,
			Name:            "child-agent-two",
			Directory:       "/workspaces/wobble",
			Architecture:    "amd64",
			OperatingSystem: "linux",
		})

		// When: We delete one of the dev container agents.
		_, err := api.DeleteDevContainerAgent(ctx, &proto.DeleteDevContainerAgentRequest{
			Id: agentOne.ID[:],
		})
		require.NoError(t, err)

		// Then: The correct one is deleted.
		_, err = api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agentOne.ID)
		require.ErrorIs(t, err, sql.ErrNoRows)

		_, err = api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agentTwo.ID)
		require.NoError(t, err)
	})

	t.Run("CanListDevContainerAgents", func(t *testing.T) {
		t.Parallel()

		log := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		api, res := devContainerAgentAPI(t, log)

		// Given: Multiple dev container agents.
		agentOne := dbgen.WorkspaceAgent(t, api.Database, database.WorkspaceAgent{
			ParentID:        uuid.NullUUID{Valid: true, UUID: api.AgentID},
			ResourceID:      res.ID,
			Name:            "child-agent-one",
			Directory:       "/workspaces/wibble",
			Architecture:    "amd64",
			OperatingSystem: "linux",
		})

		agentTwo := dbgen.WorkspaceAgent(t, api.Database, database.WorkspaceAgent{
			ParentID:        uuid.NullUUID{Valid: true, UUID: api.AgentID},
			ResourceID:      res.ID,
			Name:            "child-agent-two",
			Directory:       "/workspaces/wobble",
			Architecture:    "amd64",
			OperatingSystem: "linux",
		})

		agents := []database.WorkspaceAgent{agentOne, agentTwo}
		slices.SortFunc(agents, func(a, b database.WorkspaceAgent) int {
			return cmp.Compare(a.ID.String(), b.ID.String())
		})

		// When: We list the dev container agents.
		listResp, err := api.ListDevContainerAgents(ctx, &proto.ListDevContainerAgentsRequest{})
		require.NoError(t, err)

		listedAgents := listResp.Agents
		slices.SortFunc(listedAgents, func(a, b *proto.ListDevContainerAgentsResponse_DevContainerAgent) int {
			return cmp.Compare(string(a.Id), string(b.Id))
		})

		// Then: We expect to see all the agents listed.
		require.Len(t, listedAgents, len(agents))
		for i, agent := range listedAgents {
			require.Equal(t, agents[i].ID[:], agent.Id)
			require.Equal(t, agents[i].Name, agent.Name)
		}
	})
}
