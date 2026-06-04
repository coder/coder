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

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	agpldbauthz "github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	agplportsharing "github.com/coder/coder/v2/coderd/portsharing"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	entdbauthz "github.com/coder/coder/v2/enterprise/coderd/dbauthz"
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
			name:              "AuthenticatedClampsPublic",
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
			},
			expectedStoredApps: []expectedApp{
				{
					slugSuffix:   "-authenticated-app",
					sharingLevel: database.AppSharingLevelAuthenticated,
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
			name:              "PublicAllowsPublicAuthenticatedAndOwner",
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
			},
			expectedStoredApps: []expectedApp{
				{
					slugSuffix:   "-authenticated-app",
					sharingLevel: database.AppSharingLevelAuthenticated,
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
			name:              "OwnerClampsAuthenticatedAndPublic",
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
			},
			expectedStoredApps: []expectedApp{
				{
					slugSuffix:   "-authenticated-app",
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

			ctx, db, api := newSubAgentAPIWithMaxPortShareLevel(t, tt.maxPortShareLevel)
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

			agentID, err := uuid.FromBytes(resp.Agent.Id)
			require.NoError(t, err)
			apps, err := db.GetWorkspaceAppsByAgentID(agpldbauthz.AsSystemRestricted(ctx), agentID)
			require.NoError(t, err)
			require.Len(t, apps, len(tt.expectedStoredApps))

			slices.SortFunc(apps, func(a, b database.WorkspaceApp) int {
				return cmp.Compare(a.Slug, b.Slug)
			})
			slices.SortFunc(tt.expectedStoredApps, func(a, b expectedApp) int {
				return cmp.Compare(a.slugSuffix, b.slugSuffix)
			})

			for i, expectedApp := range tt.expectedStoredApps {
				require.True(t, strings.HasSuffix(apps[i].Slug, expectedApp.slugSuffix))
				require.Equal(t, expectedApp.sharingLevel, apps[i].SharingLevel)
			}
		})
	}
}

func newSubAgentAPIWithMaxPortShareLevel(t *testing.T, maxPortShareLevel database.AppSharingLevel) (context.Context, database.Store, *agentapi.SubAgentAPI) {
	t.Helper()

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
	ctx := testutil.Context(t, testutil.WaitShort)
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

	return ctx, db, api
}
