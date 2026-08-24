package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

// TestAllWorkspaceRelated verifies that allWorkspaceRelated selects every node
// in the related-data hierarchy.
func TestAllWorkspaceRelated(t *testing.T) {
	t.Parallel()

	all := allWorkspaceRelated()
	require.True(t, all.Template)
	require.NotNil(t, all.LatestBuild)
	require.NotNil(t, all.LatestBuild.Job)
	require.True(t, all.LatestBuild.Job.QueuePosition)
	require.True(t, all.LatestBuild.TemplateVersion)
	require.NotNil(t, all.LatestBuild.Resources)
	require.True(t, all.LatestBuild.Resources.Metadata)
	require.NotNil(t, all.LatestBuild.Resources.Agents)
	require.NotNil(t, all.LatestBuild.Resources.Agents.Apps)
	require.True(t, all.LatestBuild.Resources.Agents.Apps.Statuses)
	require.True(t, all.LatestBuild.Resources.Agents.Scripts)
	require.True(t, all.LatestBuild.Resources.Agents.LogSources)
	require.True(t, all.LatestBuild.appStatuses())
}

// TestLatestBuildRelatedAppStatuses verifies the nil-safe appStatuses accessor
// only reports true when the full apps.statuses path is present.
func TestLatestBuildRelatedAppStatuses(t *testing.T) {
	t.Parallel()

	require.False(t, (*latestBuildRelated)(nil).appStatuses())
	require.False(t, (&latestBuildRelated{}).appStatuses())
	require.False(t, (&latestBuildRelated{Resources: &resourcesRelated{}}).appStatuses())
	require.False(t, (&latestBuildRelated{Resources: &resourcesRelated{Agents: &agentsRelated{}}}).appStatuses())
	require.False(t, (&latestBuildRelated{Resources: &resourcesRelated{Agents: &agentsRelated{Apps: &appsRelated{}}}}).appStatuses())
	require.True(t, (&latestBuildRelated{Resources: &resourcesRelated{Agents: &agentsRelated{Apps: &appsRelated{Statuses: true}}}}).appStatuses())
}

// TestWorkspaceBuildsDataQueryGating asserts that workspaceBuildsData only
// issues the database queries implied by the selection. gomock is strict, so
// any query that is not set up here fails the test if invoked.
func TestWorkspaceBuildsDataQueryGating(t *testing.T) {
	t.Parallel()

	build := database.WorkspaceBuild{
		ID:                uuid.New(),
		JobID:             uuid.New(),
		WorkspaceID:       uuid.New(),
		TemplateVersionID: uuid.New(),
	}
	resource := database.WorkspaceResource{ID: uuid.New(), JobID: build.JobID}
	agent := database.WorkspaceAgent{ID: uuid.New(), ResourceID: resource.ID}
	app := database.WorkspaceApp{ID: uuid.New(), AgentID: agent.ID}

	expectJob := func(db *dbmock.MockStore) {
		db.EXPECT().GetProvisionerJobsByIDs(gomock.Any(), gomock.Any()).
			Return([]database.ProvisionerJob{}, nil)
		db.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), gomock.Any()).
			Return([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}, nil)
	}
	expectJobWithQueuePosition := func(db *dbmock.MockStore) {
		db.EXPECT().GetProvisionerJobsByIDsWithQueuePosition(gomock.Any(), gomock.Any()).
			Return([]database.GetProvisionerJobsByIDsWithQueuePositionRow{}, nil)
		db.EXPECT().GetEligibleProvisionerDaemonsByProvisionerJobIDs(gomock.Any(), gomock.Any()).
			Return([]database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{}, nil)
	}
	expectTemplateVersion := func(db *dbmock.MockStore) {
		db.EXPECT().GetTemplateVersionsByIDs(gomock.Any(), gomock.Any()).
			Return([]database.TemplateVersion{}, nil)
	}
	expectResources := func(db *dbmock.MockStore) {
		db.EXPECT().GetWorkspaceResourcesByJobIDs(gomock.Any(), gomock.Any()).
			Return([]database.WorkspaceResource{resource}, nil)
	}
	expectAgents := func(db *dbmock.MockStore) {
		db.EXPECT().GetWorkspaceAgentsByResourceIDs(gomock.Any(), gomock.Any()).
			Return([]database.WorkspaceAgent{agent}, nil)
	}
	expectApps := func(db *dbmock.MockStore) {
		db.EXPECT().GetWorkspaceAppsByAgentIDs(gomock.Any(), gomock.Any()).
			Return([]database.WorkspaceApp{app}, nil)
	}

	cases := []struct {
		name  string
		cfg   latestBuildRelated
		setup func(*dbmock.MockStore)
	}{
		{
			name:  "BuildOnly",
			cfg:   latestBuildRelated{},
			setup: func(*dbmock.MockStore) {},
		},
		{
			name:  "Job",
			cfg:   latestBuildRelated{Job: &jobRelated{}},
			setup: expectJob,
		},
		{
			name:  "JobWithQueuePosition",
			cfg:   latestBuildRelated{Job: &jobRelated{QueuePosition: true}},
			setup: expectJobWithQueuePosition,
		},
		{
			name:  "TemplateVersion",
			cfg:   latestBuildRelated{TemplateVersion: true},
			setup: expectTemplateVersion,
		},
		{
			name:  "Resources",
			cfg:   latestBuildRelated{Resources: &resourcesRelated{}},
			setup: expectResources,
		},
		{
			name: "Metadata",
			cfg:  latestBuildRelated{Resources: &resourcesRelated{Metadata: true}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				db.EXPECT().GetWorkspaceResourceMetadataByResourceIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceResourceMetadatum{}, nil)
			},
		},
		{
			name: "Agents",
			cfg:  latestBuildRelated{Resources: &resourcesRelated{Agents: &agentsRelated{}}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				expectAgents(db)
			},
		},
		{
			name: "Apps",
			cfg:  latestBuildRelated{Resources: &resourcesRelated{Agents: &agentsRelated{Apps: &appsRelated{}}}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				expectAgents(db)
				expectApps(db)
			},
		},
		{
			name: "AppStatuses",
			cfg:  latestBuildRelated{Resources: &resourcesRelated{Agents: &agentsRelated{Apps: &appsRelated{Statuses: true}}}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				expectAgents(db)
				expectApps(db)
				db.EXPECT().GetWorkspaceAppStatusesByAppIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceAppStatus{}, nil)
			},
		},
		{
			name: "Scripts",
			cfg:  latestBuildRelated{Resources: &resourcesRelated{Agents: &agentsRelated{Scripts: true}}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				expectAgents(db)
				db.EXPECT().GetWorkspaceAgentScriptsByAgentIDs(gomock.Any(), gomock.Any()).
					Return([]database.GetWorkspaceAgentScriptsByAgentIDsRow{}, nil)
			},
		},
		{
			name: "LogSources",
			cfg:  latestBuildRelated{Resources: &resourcesRelated{Agents: &agentsRelated{LogSources: true}}},
			setup: func(db *dbmock.MockStore) {
				expectResources(db)
				expectAgents(db)
				db.EXPECT().GetWorkspaceAgentLogSourcesByAgentIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceAgentLogSource{}, nil)
			},
		},
		{
			name: "All",
			cfg:  allLatestBuildRelated(),
			setup: func(db *dbmock.MockStore) {
				expectJobWithQueuePosition(db)
				expectTemplateVersion(db)
				expectResources(db)
				expectAgents(db)
				expectApps(db)
				db.EXPECT().GetWorkspaceResourceMetadataByResourceIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceResourceMetadatum{}, nil)
				db.EXPECT().GetWorkspaceAppStatusesByAppIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceAppStatus{}, nil)
				db.EXPECT().GetWorkspaceAgentScriptsByAgentIDs(gomock.Any(), gomock.Any()).
					Return([]database.GetWorkspaceAgentScriptsByAgentIDsRow{}, nil)
				db.EXPECT().GetWorkspaceAgentLogSourcesByAgentIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceAgentLogSource{}, nil)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			tc.setup(db)

			api := &API{Options: &Options{Database: db}}
			_, err := api.workspaceBuildsData(ctx, []database.WorkspaceBuild{build}, tc.cfg)
			require.NoError(t, err)
		})
	}
}

// TestWorkspaceBuildsDataResourcesShortCircuit verifies that when resources are
// requested but none exist, no agent-level queries are issued even though the
// deeper nodes are selected.
func TestWorkspaceBuildsDataResourcesShortCircuit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	build := database.WorkspaceBuild{ID: uuid.New(), JobID: uuid.New(), TemplateVersionID: uuid.New()}
	// No resources returned: the agent, app, and status queries must be skipped.
	db.EXPECT().GetWorkspaceResourcesByJobIDs(gomock.Any(), gomock.Any()).
		Return([]database.WorkspaceResource{}, nil)

	cfg := latestBuildRelated{Resources: &resourcesRelated{Agents: &agentsRelated{Apps: &appsRelated{Statuses: true}}}}
	api := &API{Options: &Options{Database: db}}
	_, err := api.workspaceBuildsData(ctx, []database.WorkspaceBuild{build}, cfg)
	require.NoError(t, err)
}

// TestWorkspaceDataQueryGating asserts that workspaceData gates its
// workspace-level queries (templates, latest builds, and latest app statuses)
// on the selection.
func TestWorkspaceDataQueryGating(t *testing.T) {
	t.Parallel()

	workspace := database.Workspace{ID: uuid.New(), TemplateID: uuid.New()}

	cases := []struct {
		name  string
		cfg   workspaceRelated
		setup func(*dbmock.MockStore)
	}{
		{
			name:  "None",
			cfg:   workspaceRelated{},
			setup: func(*dbmock.MockStore) {},
		},
		{
			name: "Template",
			cfg:  workspaceRelated{Template: true},
			setup: func(db *dbmock.MockStore) {
				db.EXPECT().GetTemplatesWithFilter(gomock.Any(), gomock.Any()).
					Return([]database.Template{}, nil)
			},
		},
		{
			name: "LatestBuild",
			cfg:  workspaceRelated{LatestBuild: &latestBuildRelated{}},
			setup: func(db *dbmock.MockStore) {
				// No builds returned, so no build-subtree queries run.
				db.EXPECT().GetLatestWorkspaceBuildsByWorkspaceIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceBuild{}, nil)
			},
		},
		{
			name: "AppStatuses",
			cfg: workspaceRelated{LatestBuild: &latestBuildRelated{
				Resources: &resourcesRelated{Agents: &agentsRelated{Apps: &appsRelated{Statuses: true}}},
			}},
			setup: func(db *dbmock.MockStore) {
				// The workspace-level latest app status query is gated on the
				// apps.statuses node. With no builds, only the workspace-level
				// status, build, and (empty) resource queries run.
				db.EXPECT().GetLatestWorkspaceAppStatusesByWorkspaceIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceAppStatus{}, nil)
				db.EXPECT().GetLatestWorkspaceBuildsByWorkspaceIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceBuild{}, nil)
				db.EXPECT().GetWorkspaceResourcesByJobIDs(gomock.Any(), gomock.Any()).
					Return([]database.WorkspaceResource{}, nil)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			tc.setup(db)

			api := &API{Options: &Options{Database: db}}
			_, err := api.workspaceData(ctx, []database.Workspace{workspace}, tc.cfg)
			require.NoError(t, err)
		})
	}
}
