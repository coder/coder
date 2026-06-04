package coderd_test

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	agplportsharing "github.com/coder/coder/v2/coderd/portsharing"
	"github.com/coder/coder/v2/coderd/util/ptr"
	entportsharing "github.com/coder/coder/v2/enterprise/coderd/portsharing"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestSubAgentAPICreateSubAgentAppShareRespectsMaxPortShareLevel(t *testing.T) {
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
			name:              "AuthenticatedClampsOrganizationAndPublic",
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
				require.True(t, strings.HasSuffix((*upsertedApps)[i].Slug, expectedApp.slugSuffix))
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
