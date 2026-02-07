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

	"cdr.dev/slog/v3"
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

					agent, err := api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agentID)
					require.NoError(t, err)

					assert.Equal(t, tt.agentName, agent.Name)
					assert.Equal(t, tt.agentDir, agent.Directory)
					assert.Equal(t, tt.agentArch, agent.Architecture)
					assert.Equal(t, tt.agentOS, agent.OperatingSystem)
				}
			})
		}
	})

	type expectedAppError struct {
		index int32
		field string
		error string
	}

	t.Run("CreateSubAgentWithApps", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name              string
			apps              []*proto.CreateSubAgentRequest_App
			expectApps        []database.WorkspaceApp
			expectedAppErrors []expectedAppError
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
						Slug:                 "fdqf0lpd-code-server",
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
						Slug:         "547knu0f-vim",
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
				expectApps: []database.WorkspaceApp{},
				expectedAppErrors: []expectedAppError{
					{
						index: 0,
						field: "slug",
						error: "must not be empty",
					},
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
				expectApps: []database.WorkspaceApp{},
				expectedAppErrors: []expectedAppError{
					{
						index: 0,
						field: "slug",
						error: "\"invalid_slug_with_underscores\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
					},
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
				expectApps: []database.WorkspaceApp{},
				expectedAppErrors: []expectedAppError{
					{
						index: 0,
						field: "slug",
						error: "\"InvalidSlug\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
					},
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
				expectApps: []database.WorkspaceApp{},
				expectedAppErrors: []expectedAppError{
					{
						index: 0,
						field: "slug",
						error: "\"-invalid-app\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
					},
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
				expectApps: []database.WorkspaceApp{},
				expectedAppErrors: []expectedAppError{
					{
						index: 0,
						field: "slug",
						error: "\"invalid-app-\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
					},
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
				expectApps: []database.WorkspaceApp{},
				expectedAppErrors: []expectedAppError{
					{
						index: 0,
						field: "slug",
						error: "\"invalid--app\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
					},
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
				expectApps: []database.WorkspaceApp{},
				expectedAppErrors: []expectedAppError{
					{
						index: 0,
						field: "slug",
						error: "\"invalid app\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
					},
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
				expectApps: []database.WorkspaceApp{
					{
						Slug:         "511ctirn-valid-app",
						DisplayName:  "Valid App",
						SharingLevel: database.AppSharingLevelOwner,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
				},
				expectedAppErrors: []expectedAppError{
					{
						index: 1,
						field: "slug",
						error: "\"Invalid_App\" does not match regex \"^[a-z0-9](-?[a-z0-9])*$\"",
					},
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
						Slug:         "atpt261l-authenticated-app",
						SharingLevel: database.AppSharingLevelAuthenticated,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
					{
						Slug:         "eh5gp1he-owner-app",
						SharingLevel: database.AppSharingLevelOwner,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
					{
						Slug:         "oopjevf1-public-app",
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
						Slug:         "ci9500rm-tab-app",
						SharingLevel: database.AppSharingLevelOwner,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInTab,
					},
					{
						Slug:         "p17s76re-window-app",
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
						Slug:                 "0ccdbg39-full-app",
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
						Slug:                 "nphrhbh6-no-health-app",
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
				expectApps: []database.WorkspaceApp{
					{
						Slug:         "uiklfckv-duplicate-app",
						DisplayName:  "First App",
						SharingLevel: database.AppSharingLevelOwner,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
				},
				expectedAppErrors: []expectedAppError{
					{
						index: 1,
						field: "slug",
						error: "\"duplicate-app\" is already in use",
					},
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
				expectApps: []database.WorkspaceApp{
					{
						Slug:         "uiklfckv-duplicate-app",
						DisplayName:  "First Duplicate",
						SharingLevel: database.AppSharingLevelOwner,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
					{
						Slug:         "511ctirn-valid-app",
						DisplayName:  "Valid App",
						SharingLevel: database.AppSharingLevelOwner,
						Health:       database.WorkspaceAppHealthDisabled,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					},
				},
				expectedAppErrors: []expectedAppError{
					{
						index: 2,
						field: "slug",
						error: "\"duplicate-app\" is already in use",
					},
					{
						index: 3,
						field: "slug",
						error: "\"duplicate-app\" is already in use",
					},
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
				require.NoError(t, err)

				agentID, err := uuid.FromBytes(createResp.Agent.Id)
				require.NoError(t, err)

				apps, err := api.Database.GetWorkspaceAppsByAgentID(dbauthz.AsSystemRestricted(ctx), agentID)
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

				// Verify expected app creation errors
				require.Len(t, createResp.AppCreationErrors, len(tt.expectedAppErrors), "Number of app creation errors should match expected")

				// Build a map of actual errors by index for easier testing
				actualErrorMap := make(map[int32]*proto.CreateSubAgentResponse_AppCreationError)
				for _, appErr := range createResp.AppCreationErrors {
					actualErrorMap[appErr.Index] = appErr
				}

				// Verify each expected error
				for _, expectedErr := range tt.expectedAppErrors {
					actualErr, exists := actualErrorMap[expectedErr.index]
					require.True(t, exists, "Expected app creation error at index %d", expectedErr.index)

					require.NotNil(t, actualErr.Field, "Field should be set for validation error at index %d", expectedErr.index)
					require.Equal(t, expectedErr.field, *actualErr.Field, "Field name should match for error at index %d", expectedErr.index)
					require.Contains(t, actualErr.Error, expectedErr.error, "Error message should contain expected text for error at index %d", expectedErr.index)
				}
			})
		}

		t.Run("ValidationErrorFieldMapping", func(t *testing.T) {
			t.Parallel()

			log := testutil.Logger(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)

			db, org := newDatabaseWithOrg(t)
			user, agent := newUserWithWorkspaceAgent(t, db, org)
			api := newAgentAPI(t, log, db, clock, user, org, agent)

			// Test different types of validation errors to ensure field mapping works correctly
			createResp, err := api.CreateSubAgent(ctx, &proto.CreateSubAgentRequest{
				Name:            "validation-test-agent",
				Directory:       "/workspace",
				Architecture:    "amd64",
				OperatingSystem: "linux",
				Apps: []*proto.CreateSubAgentRequest_App{
					{
						Slug:        "", // Empty slug - should error on apps[0].slug
						DisplayName: ptr.Ref("Empty Slug App"),
					},
					{
						Slug:        "Invalid_Slug_With_Underscores", // Invalid characters - should error on apps[1].slug
						DisplayName: ptr.Ref("Invalid Characters App"),
					},
					{
						Slug:        "duplicate-slug", // First occurrence - should succeed
						DisplayName: ptr.Ref("First Duplicate"),
					},
					{
						Slug:        "duplicate-slug", // Duplicate - should error on apps[3].slug
						DisplayName: ptr.Ref("Second Duplicate"),
					},
					{
						Slug:        "-invalid-start", // Invalid start character - should error on apps[4].slug
						DisplayName: ptr.Ref("Invalid Start App"),
					},
				},
			})

			// Agent should be created successfully
			require.NoError(t, err)
			require.NotNil(t, createResp.Agent)

			// Should have 4 app creation errors (indices 0, 1, 3, 4)
			require.Len(t, createResp.AppCreationErrors, 4)

			errorMap := make(map[int32]*proto.CreateSubAgentResponse_AppCreationError)
			for _, appErr := range createResp.AppCreationErrors {
				errorMap[appErr.Index] = appErr
			}

			// Verify each specific validation error and its field
			require.Contains(t, errorMap, int32(0))
			require.NotNil(t, errorMap[0].Field)
			require.Equal(t, "slug", *errorMap[0].Field)
			require.Contains(t, errorMap[0].Error, "must not be empty")

			require.Contains(t, errorMap, int32(1))
			require.NotNil(t, errorMap[1].Field)
			require.Equal(t, "slug", *errorMap[1].Field)
			require.Contains(t, errorMap[1].Error, "Invalid_Slug_With_Underscores")

			require.Contains(t, errorMap, int32(3))
			require.NotNil(t, errorMap[3].Field)
			require.Equal(t, "slug", *errorMap[3].Field)
			require.Contains(t, errorMap[3].Error, "duplicate-slug")

			require.Contains(t, errorMap, int32(4))
			require.NotNil(t, errorMap[4].Field)
			require.Equal(t, "slug", *errorMap[4].Field)
			require.Contains(t, errorMap[4].Error, "-invalid-start")

			// Verify only the valid app (index 2) was created
			agentID, err := uuid.FromBytes(createResp.Agent.Id)
			require.NoError(t, err)

			apps, err := db.GetWorkspaceAppsByAgentID(dbauthz.AsSystemRestricted(ctx), agentID)
			require.NoError(t, err)
			require.Len(t, apps, 1)
			require.Equal(t, "k5jd7a99-duplicate-slug", apps[0].Slug)
			require.Equal(t, "First Duplicate", apps[0].DisplayName)
		})
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
			_, err = db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), childAgent.ID)
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
			_, err = api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), childAgentOne.ID)
			require.ErrorIs(t, err, sql.ErrNoRows)

			_, err = api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), childAgentTwo.ID)
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

			_, err = db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), childAgentOne.ID)
			require.NoError(t, err)
		})

		t.Run("DeleteRetainsWorkspaceApps", func(t *testing.T) {
			t.Parallel()

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
			apps, err := api.Database.GetWorkspaceAppsByAgentID(dbauthz.AsSystemRestricted(ctx), subAgentID)
			require.NoError(t, err)
			require.Len(t, apps, 2)

			// When: We delete the sub agent
			_, err = api.DeleteSubAgent(ctx, &proto.DeleteSubAgentRequest{
				Id: createResp.Agent.Id,
			})
			require.NoError(t, err)

			// Then: The agent is deleted
			_, err = api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), subAgentID)
			require.ErrorIs(t, err, sql.ErrNoRows)

			// And: The apps are *retained* to avoid causing issues
			// where the resources are expected to be present.
			appsAfterDeletion, err := db.GetWorkspaceAppsByAgentID(ctx, subAgentID)
			require.NoError(t, err)
			require.NotEmpty(t, appsAfterDeletion)
		})
	})

	t.Run("CreateSubAgentWithDisplayApps", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name          string
			displayApps   []proto.CreateSubAgentRequest_DisplayApp
			expectedApps  []database.DisplayApp
			expectedError *codersdk.ValidationError
		}{
			{
				name:         "NoDisplayApps",
				displayApps:  []proto.CreateSubAgentRequest_DisplayApp{},
				expectedApps: []database.DisplayApp{},
			},
			{
				name: "SingleDisplayApp_VSCode",
				displayApps: []proto.CreateSubAgentRequest_DisplayApp{
					proto.CreateSubAgentRequest_VSCODE,
				},
				expectedApps: []database.DisplayApp{
					database.DisplayAppVscode,
				},
			},
			{
				name: "SingleDisplayApp_VSCodeInsiders",
				displayApps: []proto.CreateSubAgentRequest_DisplayApp{
					proto.CreateSubAgentRequest_VSCODE_INSIDERS,
				},
				expectedApps: []database.DisplayApp{
					database.DisplayAppVscodeInsiders,
				},
			},
			{
				name: "SingleDisplayApp_WebTerminal",
				displayApps: []proto.CreateSubAgentRequest_DisplayApp{
					proto.CreateSubAgentRequest_WEB_TERMINAL,
				},
				expectedApps: []database.DisplayApp{
					database.DisplayAppWebTerminal,
				},
			},
			{
				name: "SingleDisplayApp_SSHHelper",
				displayApps: []proto.CreateSubAgentRequest_DisplayApp{
					proto.CreateSubAgentRequest_SSH_HELPER,
				},
				expectedApps: []database.DisplayApp{
					database.DisplayAppSSHHelper,
				},
			},
			{
				name: "SingleDisplayApp_PortForwardingHelper",
				displayApps: []proto.CreateSubAgentRequest_DisplayApp{
					proto.CreateSubAgentRequest_PORT_FORWARDING_HELPER,
				},
				expectedApps: []database.DisplayApp{
					database.DisplayAppPortForwardingHelper,
				},
			},
			{
				name: "MultipleDisplayApps",
				displayApps: []proto.CreateSubAgentRequest_DisplayApp{
					proto.CreateSubAgentRequest_VSCODE,
					proto.CreateSubAgentRequest_WEB_TERMINAL,
					proto.CreateSubAgentRequest_SSH_HELPER,
				},
				expectedApps: []database.DisplayApp{
					database.DisplayAppVscode,
					database.DisplayAppWebTerminal,
					database.DisplayAppSSHHelper,
				},
			},
			{
				name: "AllDisplayApps",
				displayApps: []proto.CreateSubAgentRequest_DisplayApp{
					proto.CreateSubAgentRequest_VSCODE,
					proto.CreateSubAgentRequest_VSCODE_INSIDERS,
					proto.CreateSubAgentRequest_WEB_TERMINAL,
					proto.CreateSubAgentRequest_SSH_HELPER,
					proto.CreateSubAgentRequest_PORT_FORWARDING_HELPER,
				},
				expectedApps: []database.DisplayApp{
					database.DisplayAppVscode,
					database.DisplayAppVscodeInsiders,
					database.DisplayAppWebTerminal,
					database.DisplayAppSSHHelper,
					database.DisplayAppPortForwardingHelper,
				},
			},
			{
				name: "InvalidDisplayApp",
				displayApps: []proto.CreateSubAgentRequest_DisplayApp{
					proto.CreateSubAgentRequest_DisplayApp(9999), // Invalid enum value
				},
				expectedError: &codersdk.ValidationError{
					Field: "display_apps[0]",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				log := testutil.Logger(t)
				ctx := testutil.Context(t, testutil.WaitLong)
				clock := quartz.NewMock(t)

				db, org := newDatabaseWithOrg(t)
				user, agent := newUserWithWorkspaceAgent(t, db, org)
				api := newAgentAPI(t, log, db, clock, user, org, agent)

				createResp, err := api.CreateSubAgent(ctx, &proto.CreateSubAgentRequest{
					Name:            "test-agent",
					Directory:       "/workspaces/test",
					Architecture:    "amd64",
					OperatingSystem: "linux",
					DisplayApps:     tt.displayApps,
				})
				if tt.expectedError != nil {
					require.Error(t, err)
					require.Nil(t, createResp)

					var validationErr codersdk.ValidationError
					require.ErrorAs(t, err, &validationErr)
					require.Equal(t, tt.expectedError.Field, validationErr.Field)
					require.Contains(t, validationErr.Detail, "is not a valid display app")
				} else {
					require.NoError(t, err)
					require.NotNil(t, createResp.Agent)

					agentID, err := uuid.FromBytes(createResp.Agent.Id)
					require.NoError(t, err)

					subAgent, err := api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agentID)
					require.NoError(t, err)

					require.Equal(t, len(tt.expectedApps), len(subAgent.DisplayApps), "display apps count mismatch")

					for i, expectedApp := range tt.expectedApps {
						require.Equal(t, expectedApp, subAgent.DisplayApps[i], "display app at index %d doesn't match", i)
					}
				}
			})
		}
	})

	t.Run("CreateSubAgentWithDisplayAppsAndApps", func(t *testing.T) {
		t.Parallel()

		log := testutil.Logger(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		clock := quartz.NewMock(t)

		db, org := newDatabaseWithOrg(t)
		user, agent := newUserWithWorkspaceAgent(t, db, org)
		api := newAgentAPI(t, log, db, clock, user, org, agent)

		// Test that display apps and regular apps can coexist
		createResp, err := api.CreateSubAgent(ctx, &proto.CreateSubAgentRequest{
			Name:            "test-agent",
			Directory:       "/workspaces/test",
			Architecture:    "amd64",
			OperatingSystem: "linux",
			DisplayApps: []proto.CreateSubAgentRequest_DisplayApp{
				proto.CreateSubAgentRequest_VSCODE,
				proto.CreateSubAgentRequest_WEB_TERMINAL,
			},
			Apps: []*proto.CreateSubAgentRequest_App{
				{
					Slug:        "custom-app",
					DisplayName: ptr.Ref("Custom App"),
					Url:         ptr.Ref("http://localhost:8080"),
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, createResp.Agent)
		require.Empty(t, createResp.AppCreationErrors)

		agentID, err := uuid.FromBytes(createResp.Agent.Id)
		require.NoError(t, err)

		// Verify display apps
		subAgent, err := api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agentID)
		require.NoError(t, err)
		require.Len(t, subAgent.DisplayApps, 2)
		require.Equal(t, database.DisplayAppVscode, subAgent.DisplayApps[0])
		require.Equal(t, database.DisplayAppWebTerminal, subAgent.DisplayApps[1])

		// Verify regular apps
		apps, err := api.Database.GetWorkspaceAppsByAgentID(dbauthz.AsSystemRestricted(ctx), agentID)
		require.NoError(t, err)
		require.Len(t, apps, 1)
		require.Equal(t, "v4qhkq17-custom-app", apps[0].Slug)
		require.Equal(t, "Custom App", apps[0].DisplayName)
	})

	t.Run("CreateSubAgentUpdatesExisting", func(t *testing.T) {
		t.Parallel()

		baseChildAgent := database.WorkspaceAgent{
			Name:            "existing-child-agent",
			Directory:       "/workspaces/test",
			Architecture:    "amd64",
			OperatingSystem: "linux",
			DisplayApps:     []database.DisplayApp{database.DisplayAppVscode},
		}

		type testCase struct {
			name    string
			setup   func(t *testing.T, db database.Store, agent database.WorkspaceAgent) *proto.CreateSubAgentRequest
			wantErr string
			check   func(t *testing.T, ctx context.Context, db database.Store, resp *proto.CreateSubAgentResponse, agent database.WorkspaceAgent)
		}

		tests := []testCase{
			{
				name: "OK",
				setup: func(t *testing.T, db database.Store, agent database.WorkspaceAgent) *proto.CreateSubAgentRequest {
					// Given: An existing child agent with some display apps.
					childAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
						ParentID:        uuid.NullUUID{Valid: true, UUID: agent.ID},
						ResourceID:      agent.ResourceID,
						Name:            baseChildAgent.Name,
						Directory:       baseChildAgent.Directory,
						Architecture:    baseChildAgent.Architecture,
						OperatingSystem: baseChildAgent.OperatingSystem,
						DisplayApps:     baseChildAgent.DisplayApps,
					})

					// When: We call CreateSubAgent with the existing agent's ID and new display apps.
					return &proto.CreateSubAgentRequest{
						Id: childAgent.ID[:],
						DisplayApps: []proto.CreateSubAgentRequest_DisplayApp{
							proto.CreateSubAgentRequest_WEB_TERMINAL,
							proto.CreateSubAgentRequest_SSH_HELPER,
						},
					}
				},
				check: func(t *testing.T, ctx context.Context, db database.Store, resp *proto.CreateSubAgentResponse, agent database.WorkspaceAgent) {
					// Then: The response contains the existing agent's details.
					require.NotNil(t, resp.Agent)
					require.Equal(t, baseChildAgent.Name, resp.Agent.Name)

					agentID, err := uuid.FromBytes(resp.Agent.Id)
					require.NoError(t, err)

					// And: The database agent's display apps are updated.
					updatedAgent, err := db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agentID)
					require.NoError(t, err)
					require.Len(t, updatedAgent.DisplayApps, 2)
					require.Contains(t, updatedAgent.DisplayApps, database.DisplayAppWebTerminal)
					require.Contains(t, updatedAgent.DisplayApps, database.DisplayAppSSHHelper)
				},
			},
			{
				name: "OK_OtherFieldsNotModified",
				setup: func(t *testing.T, db database.Store, agent database.WorkspaceAgent) *proto.CreateSubAgentRequest {
					// Given: An existing child agent with specific properties.
					childAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
						ParentID:        uuid.NullUUID{Valid: true, UUID: agent.ID},
						ResourceID:      agent.ResourceID,
						Name:            baseChildAgent.Name,
						Directory:       baseChildAgent.Directory,
						Architecture:    baseChildAgent.Architecture,
						OperatingSystem: baseChildAgent.OperatingSystem,
						DisplayApps:     baseChildAgent.DisplayApps,
					})

					// When: We call CreateSubAgent with different values for name, directory, arch, and OS.
					return &proto.CreateSubAgentRequest{
						Id:              childAgent.ID[:],
						Name:            "different-name",
						Directory:       "/different/path",
						Architecture:    "arm64",
						OperatingSystem: "darwin",
						DisplayApps: []proto.CreateSubAgentRequest_DisplayApp{
							proto.CreateSubAgentRequest_WEB_TERMINAL,
						},
					}
				},
				check: func(t *testing.T, ctx context.Context, db database.Store, resp *proto.CreateSubAgentResponse, agent database.WorkspaceAgent) {
					// Then: The response contains the original agent name, not the new one.
					require.NotNil(t, resp.Agent)
					require.Equal(t, baseChildAgent.Name, resp.Agent.Name)

					agentID, err := uuid.FromBytes(resp.Agent.Id)
					require.NoError(t, err)

					// And: The database agent's other fields are unchanged.
					updatedAgent, err := db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), agentID)
					require.NoError(t, err)
					require.Equal(t, baseChildAgent.Name, updatedAgent.Name)
					require.Equal(t, baseChildAgent.Directory, updatedAgent.Directory)
					require.Equal(t, baseChildAgent.Architecture, updatedAgent.Architecture)
					require.Equal(t, baseChildAgent.OperatingSystem, updatedAgent.OperatingSystem)

					// But display apps should be updated.
					require.Len(t, updatedAgent.DisplayApps, 1)
					require.Equal(t, database.DisplayAppWebTerminal, updatedAgent.DisplayApps[0])
				},
			},
			{
				name: "Error/MalformedID",
				setup: func(t *testing.T, db database.Store, agent database.WorkspaceAgent) *proto.CreateSubAgentRequest {
					// When: We call CreateSubAgent with malformed ID bytes (not 16 bytes).
					// uuid.FromBytes requires exactly 16 bytes, so we provide fewer.
					return &proto.CreateSubAgentRequest{
						Id: []byte("short"),
					}
				},
				wantErr: "parse agent id",
			},
			{
				name: "Error/AgentNotFound",
				setup: func(t *testing.T, db database.Store, agent database.WorkspaceAgent) *proto.CreateSubAgentRequest {
					// When: We call CreateSubAgent with a non-existent agent ID.
					nonExistentID := uuid.New()
					return &proto.CreateSubAgentRequest{
						Id: nonExistentID[:],
					}
				},
				wantErr: "get workspace agent by id",
			},
			{
				name: "Error/ParentMismatch",
				setup: func(t *testing.T, db database.Store, agent database.WorkspaceAgent) *proto.CreateSubAgentRequest {
					// Create a second agent (sibling) within the same workspace/resource.
					// This sibling has a different parent ID (or no parent).
					siblingAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
						ParentID:        uuid.NullUUID{Valid: false}, // No parent - it's a top-level agent
						ResourceID:      agent.ResourceID,
						Name:            "sibling-agent",
						Directory:       "/workspaces/sibling",
						Architecture:    "amd64",
						OperatingSystem: "linux",
					})

					// Create a child of the sibling agent (not our agent).
					childOfSibling := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
						ParentID:        uuid.NullUUID{Valid: true, UUID: siblingAgent.ID},
						ResourceID:      agent.ResourceID,
						Name:            "child-of-sibling",
						Directory:       "/workspaces/test",
						Architecture:    "amd64",
						OperatingSystem: "linux",
					})

					// When: Our API (which is for `agent`) tries to update the child of `siblingAgent`.
					return &proto.CreateSubAgentRequest{
						Id: childOfSibling.ID[:],
						DisplayApps: []proto.CreateSubAgentRequest_DisplayApp{
							proto.CreateSubAgentRequest_VSCODE,
						},
					}
				},
				wantErr: "subagent does not belong to this parent agent",
			},

			{
				name: "Error/NoParentID",
				setup: func(t *testing.T, db database.Store, agent database.WorkspaceAgent) *proto.CreateSubAgentRequest {
					// Given: An agent without a parent (a top-level agent).
					topLevelAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
						ParentID:        uuid.NullUUID{Valid: false}, // No parent
						ResourceID:      agent.ResourceID,
						Name:            "top-level-agent",
						Directory:       "/workspaces/test",
						Architecture:    "amd64",
						OperatingSystem: "linux",
					})

					// When: We try to update this agent as if it were a subagent.
					return &proto.CreateSubAgentRequest{
						Id: topLevelAgent.ID[:],
						DisplayApps: []proto.CreateSubAgentRequest_DisplayApp{
							proto.CreateSubAgentRequest_VSCODE,
						},
					}
				},
				wantErr: "subagent does not belong to this parent agent",
			},
		}

		for _, tc := range tests {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var (
					log   = testutil.Logger(t)
					clock = quartz.NewMock(t)

					db, org     = newDatabaseWithOrg(t)
					user, agent = newUserWithWorkspaceAgent(t, db, org)
					api         = newAgentAPI(t, log, db, clock, user, org, agent)
				)

				req := tc.setup(t, db, agent)
				ctx := testutil.Context(t, testutil.WaitShort)
				resp, err := api.CreateSubAgent(ctx, req)

				if tc.wantErr != "" {
					require.Error(t, err)
					require.Contains(t, err.Error(), tc.wantErr)
					return
				}

				require.NoError(t, err)
				if tc.check != nil {
					tc.check(t, ctx, db, resp, agent)
				}
			})
		}
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
			listResp, err := api.ListSubAgents(ctx, &proto.ListSubAgentsRequest{})
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
