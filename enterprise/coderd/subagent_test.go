package coderd_test

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	agpldbauthz "github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	agplportsharing "github.com/coder/coder/v2/coderd/portsharing"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	entdbauthz "github.com/coder/coder/v2/enterprise/coderd/dbauthz"
	entportsharing "github.com/coder/coder/v2/enterprise/coderd/portsharing"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestSubAgentAPICreateSubAgentAppShareRespectsEnterpriseMaxPortShareLevel(t *testing.T) {
	t.Parallel()

	type expectedApp struct {
		slugSuffix   string
		sharingLevel database.AppSharingLevel
	}

	tests := []struct {
		name               string
		maxPortShareLevel  database.AppSharingLevel
		apps               []*proto.CreateSubAgentRequest_App
		expectedStoredApps []expectedApp
	}{
		{
			name:              "AuthenticatedClampsPublicOnly",
			maxPortShareLevel: database.AppSharingLevelAuthenticated,
			apps: []*proto.CreateSubAgentRequest_App{
				{
					Slug:  "public-app",
					Share: proto.CreateSubAgentRequest_App_PUBLIC.Enum(),
					Url:   ptr.Ref("http://localhost:8080"),
				},
				{
					Slug:  "authenticated-app",
					Share: proto.CreateSubAgentRequest_App_AUTHENTICATED.Enum(),
					Url:   ptr.Ref("http://localhost:8081"),
				},
				{
					Slug:  "owner-app",
					Share: proto.CreateSubAgentRequest_App_OWNER.Enum(),
					Url:   ptr.Ref("http://localhost:8082"),
				},
				{
					Slug:  "organization-app",
					Share: proto.CreateSubAgentRequest_App_ORGANIZATION.Enum(),
					Url:   ptr.Ref("http://localhost:8083"),
				},
			},
			expectedStoredApps: []expectedApp{
				{
					slugSuffix:   "-authenticated-app",
					sharingLevel: database.AppSharingLevelAuthenticated,
				},
				{
					slugSuffix:   "-organization-app",
					sharingLevel: database.AppSharingLevelOrganization,
				},
				{
					slugSuffix:   "-owner-app",
					sharingLevel: database.AppSharingLevelOwner,
				},
				{
					slugSuffix:   "-public-app",
					sharingLevel: database.AppSharingLevelAuthenticated,
				},
			},
		},
		{
			name:              "PublicAllowsPublicAuthenticatedOrganizationAndOwner",
			maxPortShareLevel: database.AppSharingLevelPublic,
			apps: []*proto.CreateSubAgentRequest_App{
				{
					Slug:  "public-app",
					Share: proto.CreateSubAgentRequest_App_PUBLIC.Enum(),
					Url:   ptr.Ref("http://localhost:8080"),
				},
				{
					Slug:  "authenticated-app",
					Share: proto.CreateSubAgentRequest_App_AUTHENTICATED.Enum(),
					Url:   ptr.Ref("http://localhost:8081"),
				},
				{
					Slug:  "owner-app",
					Share: proto.CreateSubAgentRequest_App_OWNER.Enum(),
					Url:   ptr.Ref("http://localhost:8082"),
				},
				{
					Slug:  "organization-app",
					Share: proto.CreateSubAgentRequest_App_ORGANIZATION.Enum(),
					Url:   ptr.Ref("http://localhost:8083"),
				},
			},
			expectedStoredApps: []expectedApp{
				{
					slugSuffix:   "-authenticated-app",
					sharingLevel: database.AppSharingLevelAuthenticated,
				},
				{
					slugSuffix:   "-organization-app",
					sharingLevel: database.AppSharingLevelOrganization,
				},
				{
					slugSuffix:   "-owner-app",
					sharingLevel: database.AppSharingLevelOwner,
				},
				{
					slugSuffix:   "-public-app",
					sharingLevel: database.AppSharingLevelPublic,
				},
			},
		},
		{
			name:              "OrganizationClampsAuthenticatedAndPublic",
			maxPortShareLevel: database.AppSharingLevelOrganization,
			apps: []*proto.CreateSubAgentRequest_App{
				{
					Slug:  "authenticated-app",
					Share: proto.CreateSubAgentRequest_App_AUTHENTICATED.Enum(),
					Url:   ptr.Ref("http://localhost:8080"),
				},
				{
					Slug:  "public-app",
					Share: proto.CreateSubAgentRequest_App_PUBLIC.Enum(),
					Url:   ptr.Ref("http://localhost:8081"),
				},
				{
					Slug:  "owner-app",
					Share: proto.CreateSubAgentRequest_App_OWNER.Enum(),
					Url:   ptr.Ref("http://localhost:8082"),
				},
				{
					Slug:  "organization-app",
					Share: proto.CreateSubAgentRequest_App_ORGANIZATION.Enum(),
					Url:   ptr.Ref("http://localhost:8083"),
				},
			},
			expectedStoredApps: []expectedApp{
				{
					slugSuffix:   "-authenticated-app",
					sharingLevel: database.AppSharingLevelOrganization,
				},
				{
					slugSuffix:   "-organization-app",
					sharingLevel: database.AppSharingLevelOrganization,
				},
				{
					slugSuffix:   "-owner-app",
					sharingLevel: database.AppSharingLevelOwner,
				},
				{
					slugSuffix:   "-public-app",
					sharingLevel: database.AppSharingLevelOrganization,
				},
			},
		},
		{
			name:              "OwnerClampsOrganizationAuthenticatedAndPublic",
			maxPortShareLevel: database.AppSharingLevelOwner,
			apps: []*proto.CreateSubAgentRequest_App{
				{
					Slug:  "authenticated-app",
					Share: proto.CreateSubAgentRequest_App_AUTHENTICATED.Enum(),
					Url:   ptr.Ref("http://localhost:8080"),
				},
				{
					Slug:  "public-app",
					Share: proto.CreateSubAgentRequest_App_PUBLIC.Enum(),
					Url:   ptr.Ref("http://localhost:8081"),
				},
				{
					Slug:  "owner-app",
					Share: proto.CreateSubAgentRequest_App_OWNER.Enum(),
					Url:   ptr.Ref("http://localhost:8082"),
				},
				{
					Slug:  "organization-app",
					Share: proto.CreateSubAgentRequest_App_ORGANIZATION.Enum(),
					Url:   ptr.Ref("http://localhost:8083"),
				},
			},
			expectedStoredApps: []expectedApp{
				{
					slugSuffix:   "-authenticated-app",
					sharingLevel: database.AppSharingLevelOwner,
				},
				{
					slugSuffix:   "-organization-app",
					sharingLevel: database.AppSharingLevelOwner,
				},
				{
					slugSuffix:   "-owner-app",
					sharingLevel: database.AppSharingLevelOwner,
				},
				{
					slugSuffix:   "-public-app",
					sharingLevel: database.AppSharingLevelOwner,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, api, upsertedApps := newMockSubAgentAPIWithMaxPortShareLevel(t, tt.maxPortShareLevel, len(tt.apps))
			resp, err := api.CreateSubAgent(ctx, &proto.CreateSubAgentRequest{
				Name:            "child-agent",
				Directory:       "/workspaces/coder",
				Architecture:    "amd64",
				OperatingSystem: "linux",
				Apps:            tt.apps,
			})
			require.NoError(t, err)
			require.NotNil(t, resp.Agent)
			require.Empty(t, resp.AppCreationErrors)
			require.Len(t, *upsertedApps, len(tt.expectedStoredApps))

			slices.SortFunc(*upsertedApps, func(a, b database.UpsertWorkspaceAppParams) int {
				return cmp.Compare(appSlugSuffix(a.Slug), appSlugSuffix(b.Slug))
			})
			slices.SortFunc(tt.expectedStoredApps, func(a, b expectedApp) int {
				return cmp.Compare(a.slugSuffix, b.slugSuffix)
			})

			for i, expectedApp := range tt.expectedStoredApps {
				require.Equal(t, expectedApp.slugSuffix, appSlugSuffix((*upsertedApps)[i].Slug))
				require.Equal(t, expectedApp.sharingLevel, (*upsertedApps)[i].SharingLevel)
			}
		})
	}
}

func appSlugSuffix(slug string) string {
	_, suffix, ok := strings.Cut(slug, "-")
	if !ok {
		return slug
	}
	return "-" + suffix
}

func newMockSubAgentAPIWithMaxPortShareLevel(
	t *testing.T,
	maxPortShareLevel database.AppSharingLevel,
	appCount int,
) (context.Context, *agentapi.SubAgentAPI, *[]database.UpsertWorkspaceAppParams) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := testutil.Logger(t)
	clock := quartz.NewMock(t)
	ownerID := uuid.New()
	organizationID := uuid.New()
	templateID := uuid.New()
	parentAgent := database.WorkspaceAgent{
		ID:         uuid.New(),
		ResourceID: uuid.New(),
	}
	workspace := database.Workspace{
		ID:             uuid.New(),
		OwnerID:        ownerID,
		OrganizationID: organizationID,
		TemplateID:     templateID,
	}
	template := database.Template{
		ID:                  templateID,
		MaxPortSharingLevel: maxPortShareLevel,
	}
	upsertedApps := []database.UpsertWorkspaceAppParams{}

	db := dbmock.NewMockStore(gomock.NewController(t))
	db.EXPECT().GetWorkspaceByAgentID(gomock.Any(), parentAgent.ID).Return(workspace, nil)
	db.EXPECT().GetTemplateByID(gomock.Any(), templateID).Return(template, nil)
	db.EXPECT().InsertWorkspaceAgent(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, params database.InsertWorkspaceAgentParams) (database.WorkspaceAgent, error) {
			require.True(t, params.ParentID.Valid)
			require.Equal(t, parentAgent.ID, params.ParentID.UUID)

			return database.WorkspaceAgent{
				ID:        params.ID,
				Name:      params.Name,
				AuthToken: params.AuthToken,
			}, nil
		},
	)
	db.EXPECT().UpsertWorkspaceApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, params database.UpsertWorkspaceAppParams) (database.WorkspaceApp, error) {
			upsertedApps = append(upsertedApps, params)
			return database.WorkspaceApp{
				ID:           params.ID,
				AgentID:      params.AgentID,
				Slug:         params.Slug,
				SharingLevel: params.SharingLevel,
			}, nil
		},
	).Times(appCount)

	portSharer := &atomic.Pointer[agplportsharing.PortSharer]{}
	var ps agplportsharing.PortSharer = entportsharing.NewEnterprisePortSharer()
	portSharer.Store(&ps)
	api := &agentapi.SubAgentAPI{
		OwnerID:        ownerID,
		OrganizationID: organizationID,
		AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
			return parentAgent, nil
		},
		Log:        log,
		Clock:      clock,
		Database:   db,
		PortSharer: portSharer,
	}

	return ctx, api, &upsertedApps
}

func TestDevcontainerSubAgentAppShareClampedByEnterpriseTemplateMaxPortShareLevel(t *testing.T) {
	t.Parallel()

	ctx, db, client := newDevcontainerSubAgentClientWithMaxPortShareLevel(t, database.AppSharingLevelAuthenticated)
	subAgent, err := client.Create(ctx, agentcontainers.SubAgent{
		Name:            "devcontainer",
		Directory:       "/workspaces/coder",
		Architecture:    "amd64",
		OperatingSystem: "linux",
		Apps: []agentcontainers.SubAgentApp{
			{
				Slug:  "public-app",
				URL:   "http://localhost:8080",
				Share: codersdk.WorkspaceAppSharingLevelPublic,
			},
			{
				Slug:  "owner-app",
				URL:   "http://localhost:8081",
				Share: codersdk.WorkspaceAppSharingLevelOwner,
			},
		},
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, subAgent.ID)

	apps, err := db.GetWorkspaceAppsByAgentID(ctx, subAgent.ID)
	require.NoError(t, err)
	require.Len(t, apps, 2)
	slices.SortFunc(apps, func(a, b database.WorkspaceApp) int {
		return cmp.Compare(appSlugSuffix(a.Slug), appSlugSuffix(b.Slug))
	})
	require.Equal(t, "-owner-app", appSlugSuffix(apps[0].Slug))
	require.Equal(t, database.AppSharingLevelOwner, apps[0].SharingLevel)
	require.Equal(t, "-public-app", appSlugSuffix(apps[1].Slug))
	require.Equal(t, database.AppSharingLevelAuthenticated, apps[1].SharingLevel)
}

func TestDevcontainerCoderAppShareClampedWithGroupRestrictedEnterpriseTemplateACL(t *testing.T) {
	t.Parallel()

	ctx, db, client := newDevcontainerSubAgentClientWithMaxPortShareLevel(t,
		database.AppSharingLevelAuthenticated,
		withGroupRestrictedTemplateACL,
	)
	subAgent, err := client.Create(ctx, agentcontainers.SubAgent{
		Name:            "devcontainer",
		Directory:       "/workspaces/coder",
		Architecture:    "amd64",
		OperatingSystem: "linux",
		Apps: []agentcontainers.SubAgentApp{
			{
				Slug:  "public-app",
				URL:   "http://localhost:8080",
				Share: codersdk.WorkspaceAppSharingLevelPublic,
			},
		},
	})
	require.NoError(t, err)

	apps, err := db.GetWorkspaceAppsByAgentID(ctx, subAgent.ID)
	require.NoError(t, err)
	require.Len(t, apps, 1)
	require.Equal(t, "-public-app", appSlugSuffix(apps[0].Slug))
	require.Equal(t, database.AppSharingLevelAuthenticated, apps[0].SharingLevel)
}

type devcontainerSubAgentClientOption func(testing.TB, database.Store, database.Organization, database.User, *database.Template)

func newDevcontainerSubAgentClientWithMaxPortShareLevel(
	t *testing.T,
	maxPortShareLevel database.AppSharingLevel,
	options ...devcontainerSubAgentClientOption,
) (context.Context, database.Store, agentcontainers.SubAgentClient) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitShort)
	log := testutil.Logger(t)
	clock := quartz.NewMock(t)

	rawDB, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, rawDB, database.Organization{})
	user := dbgen.User(t, rawDB, database.User{})
	template := dbgen.Template(t, rawDB, database.Template{
		OrganizationID:      org.ID,
		CreatedBy:           user.ID,
		MaxPortSharingLevel: maxPortShareLevel,
	})
	for _, option := range options {
		option(t, rawDB, org, user, &template)
	}
	templateVersion := dbgen.TemplateVersion(t, rawDB, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{Valid: true, UUID: template.ID},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	workspace := dbgen.Workspace(t, rawDB, database.WorkspaceTable{
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		OwnerID:        user.ID,
	})
	job := dbgen.ProvisionerJob(t, rawDB, nil, database.ProvisionerJob{
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		OrganizationID: org.ID,
	})
	build := dbgen.WorkspaceBuild(t, rawDB, database.WorkspaceBuild{
		JobID:             job.ID,
		WorkspaceID:       workspace.ID,
		TemplateVersionID: templateVersion.ID,
	})
	resource := dbgen.WorkspaceResource(t, rawDB, database.WorkspaceResource{
		JobID: build.JobID,
	})
	parentAgent := dbgen.WorkspaceAgent(t, rawDB, database.WorkspaceAgent{
		ResourceID: resource.ID,
	})

	auth := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	accessControlStore := &atomic.Pointer[agpldbauthz.AccessControlStore]{}
	var acs agpldbauthz.AccessControlStore = entdbauthz.EnterpriseTemplateAccessControlStore{}
	accessControlStore.Store(&acs)
	db := agpldbauthz.New(rawDB, auth, log, accessControlStore)
	portSharer := &atomic.Pointer[agplportsharing.PortSharer]{}
	var ps agplportsharing.PortSharer = entportsharing.NewEnterprisePortSharer()
	portSharer.Store(&ps)
	api := &agentapi.SubAgentAPI{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
			return parentAgent, nil
		},
		Log:        log,
		Clock:      clock,
		Database:   db,
		PortSharer: portSharer,
	}

	client := agentcontainers.NewSubAgentClientFromAPI(log, devcontainerSubAgentDRPCClient{api: api})
	return ctx, rawDB, client
}

func withGroupRestrictedTemplateACL(t testing.TB, db database.Store, org database.Organization, user database.User, template *database.Template) {
	t.Helper()

	group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{
		GroupID: group.ID,
		UserID:  user.ID,
	})
	template.GroupACL = database.TemplateACL{
		group.ID.String(): db2sdk.TemplateRoleActions(codersdk.TemplateRoleUse),
	}
	template.UserACL = database.TemplateACL{}
	require.NoError(t, db.UpdateTemplateACLByID(context.Background(), database.UpdateTemplateACLByIDParams{
		ID:       template.ID,
		GroupACL: template.GroupACL,
		UserACL:  template.UserACL,
	}))
}

type devcontainerSubAgentDRPCClient struct {
	proto.DRPCAgentClient28
	api *agentapi.SubAgentAPI
}

func (c devcontainerSubAgentDRPCClient) CreateSubAgent(ctx context.Context, req *proto.CreateSubAgentRequest) (*proto.CreateSubAgentResponse, error) {
	return c.api.CreateSubAgent(ctx, req)
}

func (c devcontainerSubAgentDRPCClient) DeleteSubAgent(ctx context.Context, req *proto.DeleteSubAgentRequest) (*proto.DeleteSubAgentResponse, error) {
	return c.api.DeleteSubAgent(ctx, req)
}

func (c devcontainerSubAgentDRPCClient) ListSubAgents(ctx context.Context, req *proto.ListSubAgentsRequest) (*proto.ListSubAgentsResponse, error) {
	return c.api.ListSubAgents(ctx, req)
}
