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
	"github.com/stretchr/testify/assert"
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

func TestSubAgentAPI(t *testing.T) {
	t.Parallel()

	newDatabaseWithOrg := func(t *testing.T) (database.Store, database.Organization) {
		db, _ := dbtestutil.NewDB(t)
		org := dbgen.Organization(t, db, database.Organization{})
		return db, org
	}

	newUserWithWorkspaceAgent := func(t *testing.T, db database.Store, org database.Organization) (database.User, database.WorkspaceAgent) {
		user := dbgen.User(t, db, database.User{})
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
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			OrganizationID: org.ID,
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

		return user, agent
	}

	newAgentAPI := func(t *testing.T, logger slog.Logger, db database.Store, clock quartz.Clock, user database.User, org database.Organization, agent database.WorkspaceAgent) *agentapi.SubAgentAPI {
		auth := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())

		accessControlStore := &atomic.Pointer[dbauthz.AccessControlStore]{}
		var acs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
		accessControlStore.Store(&acs)

		return &agentapi.SubAgentAPI{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			AgentID:        agent.ID,
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Clock:    clock,
			Database: dbauthz.New(db, auth, logger, accessControlStore),
		}
	}

	t.Run("CreateSubAgent", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			agentName string
			agentDir  string
			agentArch string
			agentOS   string
			shouldErr bool
		}{
			{
				name:      "Ok",
				agentName: "some-child-agent",
				agentDir:  "/workspaces/wibble",
				agentArch: "amd64",
				agentOS:   "linux",
			},
			{
				name:      "NameWithUnderscore",
				agentName: "some_child_agent",
				agentDir:  "/workspaces/wibble",
				agentArch: "amd64",
				agentOS:   "linux",
				shouldErr: true,
			},
			{
				name:      "EmptyName",
				agentName: "",
				agentDir:  "/workspaces/wibble",
				agentArch: "amd64",
				agentOS:   "linux",
				shouldErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				log := testutil.Logger(t)
				ctx := testutil.Context(t, testutil.WaitShort)
				clock := quartz.NewMock(t)

				db, org := newDatabaseWithOrg(t)
				user, agent := newUserWithWorkspaceAgent(t, db, org)
				api := newAgentAPI(t, log, db, clock, user, org, agent)

				createResp, err := api.CreateSubAgent(ctx, &proto.CreateSubAgentRequest{
					Name:            tt.agentName,
					Directory:       tt.agentDir,
					Architecture:    tt.agentArch,
					OperatingSystem: tt.agentOS,
				})
				if tt.shouldErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)

					require.NotNil(t, createResp.Agent)

					agentID, err := uuid.FromBytes(createResp.Agent.Id)
					require.NoError(t, err)

					agent, err := api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agentID) //nolint:gocritic // this is a test.
					require.NoError(t, err)

					assert.Equal(t, tt.agentName, agent.Name)
					assert.Equal(t, tt.agentDir, agent.Directory)
					assert.Equal(t, tt.agentArch, agent.Architecture)
					assert.Equal(t, tt.agentOS, agent.OperatingSystem)
				}
			})
		}
	})

	t.Run("DeleteSubAgent", func(t *testing.T) {
		t.Parallel()

		t.Run("WhenOnlyOne", func(t *testing.T) {
			t.Parallel()
			log := testutil.Logger(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)

			db, org := newDatabaseWithOrg(t)
			user, agent := newUserWithWorkspaceAgent(t, db, org)
			api := newAgentAPI(t, log, db, clock, user, org, agent)

			// Given: A sub agent.
			childAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ParentID:        uuid.NullUUID{Valid: true, UUID: agent.ID},
				ResourceID:      agent.ResourceID,
				Name:            "some-child-agent",
				Directory:       "/workspaces/wibble",
				Architecture:    "amd64",
				OperatingSystem: "linux",
			})

			// When: We delete the sub agent.
			_, err := api.DeleteSubAgent(ctx, &proto.DeleteSubAgentRequest{
				Id: childAgent.ID[:],
			})
			require.NoError(t, err)

			// Then: It is deleted.
			_, err = db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), childAgent.ID) //nolint:gocritic // this is a test.
			require.ErrorIs(t, err, sql.ErrNoRows)
		})

		t.Run("WhenOneOfMany", func(t *testing.T) {
			t.Parallel()

			log := testutil.Logger(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)

			db, org := newDatabaseWithOrg(t)
			user, agent := newUserWithWorkspaceAgent(t, db, org)
			api := newAgentAPI(t, log, db, clock, user, org, agent)

			// Given: Multiple sub agents.
			childAgentOne := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ParentID:        uuid.NullUUID{Valid: true, UUID: agent.ID},
				ResourceID:      agent.ResourceID,
				Name:            "child-agent-one",
				Directory:       "/workspaces/wibble",
				Architecture:    "amd64",
				OperatingSystem: "linux",
			})

			childAgentTwo := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ParentID:        uuid.NullUUID{Valid: true, UUID: agent.ID},
				ResourceID:      agent.ResourceID,
				Name:            "child-agent-two",
				Directory:       "/workspaces/wobble",
				Architecture:    "amd64",
				OperatingSystem: "linux",
			})

			// When: We delete one of the sub agents.
			_, err := api.DeleteSubAgent(ctx, &proto.DeleteSubAgentRequest{
				Id: childAgentOne.ID[:],
			})
			require.NoError(t, err)

			// Then: The correct one is deleted.
			_, err = api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), childAgentOne.ID) //nolint:gocritic // this is a test.
			require.ErrorIs(t, err, sql.ErrNoRows)

			_, err = api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), childAgentTwo.ID) //nolint:gocritic // this is a test.
			require.NoError(t, err)
		})

		t.Run("CannotDeleteOtherAgentsChild", func(t *testing.T) {
			t.Parallel()

			log := testutil.Logger(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)

			db, org := newDatabaseWithOrg(t)

			userOne, agentOne := newUserWithWorkspaceAgent(t, db, org)
			_ = newAgentAPI(t, log, db, clock, userOne, org, agentOne)

			userTwo, agentTwo := newUserWithWorkspaceAgent(t, db, org)
			apiTwo := newAgentAPI(t, log, db, clock, userTwo, org, agentTwo)

			// Given: Both workspaces have child agents
			childAgentOne := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ParentID:        uuid.NullUUID{Valid: true, UUID: agentOne.ID},
				ResourceID:      agentOne.ResourceID,
				Name:            "child-agent-one",
				Directory:       "/workspaces/wibble",
				Architecture:    "amd64",
				OperatingSystem: "linux",
			})

			// When: An agent API attempts to delete an agent it doesn't own
			_, err := apiTwo.DeleteSubAgent(ctx, &proto.DeleteSubAgentRequest{
				Id: childAgentOne.ID[:],
			})

			// Then: We expect it to fail and for the agent to still exist.
			var notAuthorizedError dbauthz.NotAuthorizedError
			require.ErrorAs(t, err, &notAuthorizedError)

			_, err = db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), childAgentOne.ID) //nolint:gocritic // this is a test.
			require.NoError(t, err)
		})
	})

	t.Run("ListSubAgents", func(t *testing.T) {
		t.Parallel()

		t.Run("Empty", func(t *testing.T) {
			t.Parallel()

			log := testutil.Logger(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)

			db, org := newDatabaseWithOrg(t)
			user, agent := newUserWithWorkspaceAgent(t, db, org)
			api := newAgentAPI(t, log, db, clock, user, org, agent)

			// When: We list sub agents with no children
			listResp, err := api.ListSubAgents(ctx, &proto.ListSubAgentsRequest{})
			require.NoError(t, err)

			// Then: We expect an empty list
			require.Empty(t, listResp.Agents)
		})

		t.Run("Ok", func(t *testing.T) {
			t.Parallel()

			log := testutil.Logger(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)

			db, org := newDatabaseWithOrg(t)
			user, agent := newUserWithWorkspaceAgent(t, db, org)
			api := newAgentAPI(t, log, db, clock, user, org, agent)

			// Given: Multiple sub agents.
			childAgentOne := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ParentID:        uuid.NullUUID{Valid: true, UUID: agent.ID},
				ResourceID:      agent.ResourceID,
				Name:            "child-agent-one",
				Directory:       "/workspaces/wibble",
				Architecture:    "amd64",
				OperatingSystem: "linux",
			})

			childAgentTwo := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ParentID:        uuid.NullUUID{Valid: true, UUID: agent.ID},
				ResourceID:      agent.ResourceID,
				Name:            "child-agent-two",
				Directory:       "/workspaces/wobble",
				Architecture:    "amd64",
				OperatingSystem: "linux",
			})

			childAgents := []database.WorkspaceAgent{childAgentOne, childAgentTwo}
			slices.SortFunc(childAgents, func(a, b database.WorkspaceAgent) int {
				return cmp.Compare(a.ID.String(), b.ID.String())
			})

			// When: We list the sub agents.
			listResp, err := api.ListSubAgents(ctx, &proto.ListSubAgentsRequest{}) //nolint:gocritic // this is a test.
			require.NoError(t, err)

			listedChildAgents := listResp.Agents
			slices.SortFunc(listedChildAgents, func(a, b *proto.SubAgent) int {
				return cmp.Compare(string(a.Id), string(b.Id))
			})

			// Then: We expect to see all the agents listed.
			require.Len(t, listedChildAgents, len(childAgents))
			for i, listedAgent := range listedChildAgents {
				require.Equal(t, childAgents[i].ID[:], listedAgent.Id)
				require.Equal(t, childAgents[i].Name, listedAgent.Name)
			}
		})

		t.Run("DoesNotListOtherAgentsChildren", func(t *testing.T) {
			t.Parallel()

			log := testutil.Logger(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)

			db, org := newDatabaseWithOrg(t)

			// Create two users with their respective agents
			userOne, agentOne := newUserWithWorkspaceAgent(t, db, org)
			apiOne := newAgentAPI(t, log, db, clock, userOne, org, agentOne)

			userTwo, agentTwo := newUserWithWorkspaceAgent(t, db, org)
			apiTwo := newAgentAPI(t, log, db, clock, userTwo, org, agentTwo)

			// Given: Both parent agents have child agents
			childAgentOne := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ParentID:        uuid.NullUUID{Valid: true, UUID: agentOne.ID},
				ResourceID:      agentOne.ResourceID,
				Name:            "agent-one-child",
				Directory:       "/workspaces/wibble",
				Architecture:    "amd64",
				OperatingSystem: "linux",
			})

			childAgentTwo := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ParentID:        uuid.NullUUID{Valid: true, UUID: agentTwo.ID},
				ResourceID:      agentTwo.ResourceID,
				Name:            "agent-two-child",
				Directory:       "/workspaces/wobble",
				Architecture:    "amd64",
				OperatingSystem: "linux",
			})

			// When: We list the sub agents for the first user
			listRespOne, err := apiOne.ListSubAgents(ctx, &proto.ListSubAgentsRequest{})
			require.NoError(t, err)

			// Then: We should only see the first user's child agent
			require.Len(t, listRespOne.Agents, 1)
			require.Equal(t, childAgentOne.ID[:], listRespOne.Agents[0].Id)
			require.Equal(t, childAgentOne.Name, listRespOne.Agents[0].Name)

			// When: We list the sub agents for the second user
			listRespTwo, err := apiTwo.ListSubAgents(ctx, &proto.ListSubAgentsRequest{})
			require.NoError(t, err)

			// Then: We should only see the second user's child agent
			require.Len(t, listRespTwo.Agents, 1)
			require.Equal(t, childAgentTwo.ID[:], listRespTwo.Agents[0].Id)
			require.Equal(t, childAgentTwo.Name, listRespTwo.Agents[0].Name)
		})
	})
}
