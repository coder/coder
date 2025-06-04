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
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
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
			name          string
			agentName     string
			agentDir      string
			agentArch     string
			agentOS       string
			expectedError *codersdk.ValidationError
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
				expectedError: &codersdk.ValidationError{
					Field:  "name",
					Detail: "agent name \"some_child_agent\" does not match regex \"(?i)^[a-z0-9](-?[a-z0-9])*$\"",
				},
			},
			{
				name:      "EmptyName",
				agentName: "",
				agentDir:  "/workspaces/wibble",
				agentArch: "amd64",
				agentOS:   "linux",
				expectedError: &codersdk.ValidationError{
					Field:  "name",
					Detail: "agent name cannot be empty",
				},
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
				if tt.expectedError != nil {
					require.Error(t, err)
					var validationErr codersdk.ValidationError
					require.ErrorAs(t, err, &validationErr)
					require.Equal(t, *tt.expectedError, validationErr)
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

		t.Run("TransactionRollbackOnAppError", func(t *testing.T) {
			t.Parallel()

			// Skip test on in-memory database since transactions are not fully supported
			if !dbtestutil.WillUsePostgres() {
				t.Skip("Transaction behavior requires PostgreSQL")
			}

			log := testutil.Logger(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)

			db, org := newDatabaseWithOrg(t)
			user, agent := newUserWithWorkspaceAgent(t, db, org)
			api := newAgentAPI(t, log, db, clock, user, org, agent)

			// When: We create a sub agent with valid name but invalid app that will cause transaction to fail
			_, err := api.CreateSubAgent(ctx, &proto.CreateSubAgentRequest{
				Name:            "test-agent",
				Directory:       "/workspaces/test",
				Architecture:    "amd64",
				OperatingSystem: "linux",
				Apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "valid-app",
						DisplayName: ptr.Ref("Valid App"),
					},
					{
						Slug:        "Invalid_App_Slug", // This will cause validation error inside transaction
						DisplayName: ptr.Ref("Invalid App"),
					},
				},
			})

			// Then: The request should fail with validation error
			require.Error(t, err)
			var validationErr codersdk.ValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, "apps[1].slug", validationErr.Field)

			// And: No sub agents should be created (transaction rolled back)
			subAgents, err := db.GetWorkspaceAgentsByParentID(dbauthz.AsSystemRestricted(ctx), agent.ID) //nolint:gocritic // this is a test.
			require.NoError(t, err)
			require.Empty(t, subAgents, "Expected no sub agents to be created after transaction rollback")
		})
	})

	t.Run("CreateSubAgentWithApps", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name          string
			apps          []*proto.CreateSubAgentRequest_App
			expectApps    []database.WorkspaceApp
			expectedError *codersdk.ValidationError
		}{
			{
				name: "OK",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "code-server",
						DisplayName: ptr.Ref("VS Code"),
						Icon:        ptr.Ref("/icon/code.svg"),
						Url:         ptr.Ref("http://localhost:13337"),
						Share:       proto.CreateSubAgentRequest_App_OWNER.Enum(),
						Subdomain:   ptr.Ref(false),
						OpenIn:      proto.CreateSubAgentRequest_App_SLIM_WINDOW.Enum(),
						Healthcheck: &proto.CreateSubAgentRequest_App_Healthcheck{
							Interval:  5,
							Threshold: 6,
							Url:       "http://localhost:13337/healthz",
						},
					},
					{
						Slug:        "vim",
						Command:     ptr.Ref("vim"),
						DisplayName: ptr.Ref("Vim"),
						Icon:        ptr.Ref("/icon/vim.svg"),
					},
				},
				expectApps: []database.WorkspaceApp{
					{
						Slug:                 "code-server",
						DisplayName:          "VS Code",
						Icon:                 "/icon/code.svg",
						Command:              sql.NullString{},
						Url:                  sql.NullString{Valid: true, String: "http://localhost:13337"},
						HealthcheckUrl:       "http://localhost:13337/healthz",
						HealthcheckInterval:  5,
						HealthcheckThreshold: 6,
						Health:               database.WorkspaceAppHealthInitializing,
						Subdomain:            false,
						SharingLevel:         database.AppSharingLevelOwner,
						External:             false,
						DisplayOrder:         0,
						Hidden:               false,
						OpenIn:               database.WorkspaceAppOpenInSlimWindow,
						DisplayGroup:         sql.NullString{},
					},
					{
						Slug:         "vim",
						DisplayName:  "Vim",
						Icon:         "/icon/vim.svg",
						Command:      sql.NullString{Valid: true, String: "vim"},
						Health:       database.WorkspaceAppHealthDisabled,
						SharingLevel: database.AppSharingLevelOwner,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
				},
			},
			{
				name: "EmptyAppSlug",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "",
						DisplayName: ptr.Ref("App"),
					},
				},
				expectedError: &codersdk.ValidationError{
					Field:  "apps[0].slug",
					Detail: "app must have a slug or name set",
				},
			},
			{
				name: "InvalidAppSlugWithUnderscores",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "invalid_slug_with_underscores",
						DisplayName: ptr.Ref("App"),
					},
				},
				expectedError: &codersdk.ValidationError{
					Field:  "apps[0].slug",
					Detail: "app slug \"invalid_slug_with_underscores\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
				},
			},
			{
				name: "InvalidAppSlugWithUppercase",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "InvalidSlug",
						DisplayName: ptr.Ref("App"),
					},
				},
				expectedError: &codersdk.ValidationError{
					Field:  "apps[0].slug",
					Detail: "app slug \"InvalidSlug\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
				},
			},
			{
				name: "InvalidAppSlugStartsWithHyphen",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "-invalid-app",
						DisplayName: ptr.Ref("App"),
					},
				},
				expectedError: &codersdk.ValidationError{
					Field:  "apps[0].slug",
					Detail: "app slug \"-invalid-app\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
				},
			},
			{
				name: "InvalidAppSlugEndsWithHyphen",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "invalid-app-",
						DisplayName: ptr.Ref("App"),
					},
				},
				expectedError: &codersdk.ValidationError{
					Field:  "apps[0].slug",
					Detail: "app slug \"invalid-app-\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
				},
			},
			{
				name: "InvalidAppSlugWithDoubleHyphens",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "invalid--app",
						DisplayName: ptr.Ref("App"),
					},
				},
				expectedError: &codersdk.ValidationError{
					Field:  "apps[0].slug",
					Detail: "app slug \"invalid--app\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
				},
			},
			{
				name: "InvalidAppSlugWithSpaces",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "invalid app",
						DisplayName: ptr.Ref("App"),
					},
				},
				expectedError: &codersdk.ValidationError{
					Field:  "apps[0].slug",
					Detail: "app slug \"invalid app\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
				},
			},
			{
				name: "MultipleAppsWithErrorInSecond",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "valid-app",
						DisplayName: ptr.Ref("Valid App"),
					},
					{
						Slug:        "Invalid_App",
						DisplayName: ptr.Ref("Invalid App"),
					},
				},
				expectedError: &codersdk.ValidationError{
					Field:  "apps[1].slug",
					Detail: "app slug \"Invalid_App\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
				},
			},
			{
				name: "AppWithAllSharingLevels",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:  "owner-app",
						Share: proto.CreateSubAgentRequest_App_OWNER.Enum(),
					},
					{
						Slug:  "authenticated-app",
						Share: proto.CreateSubAgentRequest_App_AUTHENTICATED.Enum(),
					},
					{
						Slug:  "public-app",
						Share: proto.CreateSubAgentRequest_App_PUBLIC.Enum(),
					},
				},
				expectApps: []database.WorkspaceApp{
					{
						Slug:         "authenticated-app",
						SharingLevel: database.AppSharingLevelAuthenticated,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
					{
						Slug:         "owner-app",
						SharingLevel: database.AppSharingLevelOwner,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
					{
						Slug:         "public-app",
						SharingLevel: database.AppSharingLevelPublic,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
				},
			},
			{
				name: "AppWithDifferentOpenInOptions",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:   "window-app",
						OpenIn: proto.CreateSubAgentRequest_App_SLIM_WINDOW.Enum(),
					},
					{
						Slug:   "tab-app",
						OpenIn: proto.CreateSubAgentRequest_App_TAB.Enum(),
					},
				},
				expectApps: []database.WorkspaceApp{
					{
						Slug:         "tab-app",
						SharingLevel: database.AppSharingLevelOwner,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInTab,
					},
					{
						Slug:         "window-app",
						SharingLevel: database.AppSharingLevelOwner,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
				},
			},
			{
				name: "AppWithAllOptionalFields",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "full-app",
						Command:     ptr.Ref("echo hello"),
						DisplayName: ptr.Ref("Full Featured App"),
						External:    ptr.Ref(true),
						Group:       ptr.Ref("Development"),
						Hidden:      ptr.Ref(true),
						Icon:        ptr.Ref("/icon/app.svg"),
						Order:       ptr.Ref(int32(10)),
						Subdomain:   ptr.Ref(true),
						Url:         ptr.Ref("http://localhost:8080"),
						Healthcheck: &proto.CreateSubAgentRequest_App_Healthcheck{
							Interval:  30,
							Threshold: 3,
							Url:       "http://localhost:8080/health",
						},
					},
				},
				expectApps: []database.WorkspaceApp{
					{
						Slug:                 "full-app",
						Command:              sql.NullString{Valid: true, String: "echo hello"},
						DisplayName:          "Full Featured App",
						External:             true,
						DisplayGroup:         sql.NullString{Valid: true, String: "Development"},
						Hidden:               true,
						Icon:                 "/icon/app.svg",
						DisplayOrder:         10,
						Subdomain:            true,
						Url:                  sql.NullString{Valid: true, String: "http://localhost:8080"},
						HealthcheckUrl:       "http://localhost:8080/health",
						HealthcheckInterval:  30,
						HealthcheckThreshold: 3,
						Health:               database.WorkspaceAppHealthInitializing,
						SharingLevel:         database.AppSharingLevelOwner,
						OpenIn:               database.WorkspaceAppOpenInSlimWindow,
					},
				},
			},
			{
				name: "AppWithoutHealthcheck",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug: "no-health-app",
					},
				},
				expectApps: []database.WorkspaceApp{
					{
						Slug:                 "no-health-app",
						Health:               database.WorkspaceAppHealthDisabled,
						SharingLevel:         database.AppSharingLevelOwner,
						OpenIn:               database.WorkspaceAppOpenInSlimWindow,
						HealthcheckUrl:       "",
						HealthcheckInterval:  0,
						HealthcheckThreshold: 0,
					},
				},
			},
			{
				name: "DuplicateAppSlugs",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "duplicate-app",
						DisplayName: ptr.Ref("First App"),
					},
					{
						Slug:        "duplicate-app",
						DisplayName: ptr.Ref("Second App"),
					},
				},
				expectedError: &codersdk.ValidationError{
					Field:  "apps[1].slug",
					Detail: "app slug \"duplicate-app\" is already in use",
				},
			},
			{
				name: "MultipleDuplicateAppSlugs",
				apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "valid-app",
						DisplayName: ptr.Ref("Valid App"),
					},
					{
						Slug:        "duplicate-app",
						DisplayName: ptr.Ref("First Duplicate"),
					},
					{
						Slug:        "duplicate-app",
						DisplayName: ptr.Ref("Second Duplicate"),
					},
					{
						Slug:        "duplicate-app",
						DisplayName: ptr.Ref("Third Duplicate"),
					},
				},
				expectedError: &codersdk.ValidationError{
					Field:  "apps[2].slug",
					Detail: "app slug \"duplicate-app\" is already in use",
				},
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
					Name:            "child-agent",
					Directory:       "/workspaces/coder",
					Architecture:    "amd64",
					OperatingSystem: "linux",
					Apps:            tt.apps,
				})
				if tt.expectedError != nil {
					require.Error(t, err)
					var validationErr codersdk.ValidationError
					require.ErrorAs(t, err, &validationErr)
					require.Equal(t, *tt.expectedError, validationErr)
				} else {
					require.NoError(t, err)

					agentID, err := uuid.FromBytes(createResp.Agent.Id)
					require.NoError(t, err)

					apps, err := api.Database.GetWorkspaceAppsByAgentID(dbauthz.AsSystemRestricted(ctx), agentID) //nolint:gocritic // this is a test.
					require.NoError(t, err)

					// Sort the apps for determinism
					slices.SortFunc(apps, func(a, b database.WorkspaceApp) int {
						return cmp.Compare(a.Slug, b.Slug)
					})
					slices.SortFunc(tt.expectApps, func(a, b database.WorkspaceApp) int {
						return cmp.Compare(a.Slug, b.Slug)
					})

					require.Len(t, apps, len(tt.expectApps))

					for idx, app := range apps {
						assert.Equal(t, tt.expectApps[idx].Slug, app.Slug)
						assert.Equal(t, tt.expectApps[idx].Command, app.Command)
						assert.Equal(t, tt.expectApps[idx].DisplayName, app.DisplayName)
						assert.Equal(t, tt.expectApps[idx].External, app.External)
						assert.Equal(t, tt.expectApps[idx].DisplayGroup, app.DisplayGroup)
						assert.Equal(t, tt.expectApps[idx].HealthcheckInterval, app.HealthcheckInterval)
						assert.Equal(t, tt.expectApps[idx].HealthcheckThreshold, app.HealthcheckThreshold)
						assert.Equal(t, tt.expectApps[idx].HealthcheckUrl, app.HealthcheckUrl)
						assert.Equal(t, tt.expectApps[idx].Hidden, app.Hidden)
						assert.Equal(t, tt.expectApps[idx].Icon, app.Icon)
						assert.Equal(t, tt.expectApps[idx].OpenIn, app.OpenIn)
						assert.Equal(t, tt.expectApps[idx].DisplayOrder, app.DisplayOrder)
						assert.Equal(t, tt.expectApps[idx].SharingLevel, app.SharingLevel)
						assert.Equal(t, tt.expectApps[idx].Subdomain, app.Subdomain)
						assert.Equal(t, tt.expectApps[idx].Url, app.Url)
					}
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

		t.Run("DeletesWorkspaceApps", func(t *testing.T) {
			t.Parallel()

			// Skip test on in-memory database since CASCADE DELETE is not implemented
			if !dbtestutil.WillUsePostgres() {
				t.Skip("CASCADE DELETE behavior requires PostgreSQL")
			}

			log := testutil.Logger(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)

			db, org := newDatabaseWithOrg(t)
			user, agent := newUserWithWorkspaceAgent(t, db, org)
			api := newAgentAPI(t, log, db, clock, user, org, agent)

			// Given: A sub agent with workspace apps
			createResp, err := api.CreateSubAgent(ctx, &proto.CreateSubAgentRequest{
				Name:            "child-agent-with-apps",
				Directory:       "/workspaces/coder",
				Architecture:    "amd64",
				OperatingSystem: "linux",
				Apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "code-server",
						DisplayName: ptr.Ref("VS Code"),
						Icon:        ptr.Ref("/icon/code.svg"),
						Url:         ptr.Ref("http://localhost:13337"),
					},
					{
						Slug:        "vim",
						Command:     ptr.Ref("vim"),
						DisplayName: ptr.Ref("Vim"),
					},
				},
			})
			require.NoError(t, err)

			subAgentID, err := uuid.FromBytes(createResp.Agent.Id)
			require.NoError(t, err)

			// Verify that the apps were created
			apps, err := api.Database.GetWorkspaceAppsByAgentID(dbauthz.AsSystemRestricted(ctx), subAgentID) //nolint:gocritic // this is a test.
			require.NoError(t, err)
			require.Len(t, apps, 2)

			// When: We delete the sub agent
			_, err = api.DeleteSubAgent(ctx, &proto.DeleteSubAgentRequest{
				Id: createResp.Agent.Id,
			})
			require.NoError(t, err)

			// Then: The agent is deleted
			_, err = api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), subAgentID) //nolint:gocritic // this is a test.
			require.ErrorIs(t, err, sql.ErrNoRows)

			// And: The apps are also deleted (due to CASCADE DELETE)
			// Use raw database since authorization layer requires agent to exist
			appsAfterDeletion, err := db.GetWorkspaceAppsByAgentID(ctx, subAgentID)
			require.NoError(t, err)
			require.Empty(t, appsAfterDeletion)
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
