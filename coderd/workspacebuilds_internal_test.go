package coderd

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/tailnet"
)

// buildFixture holds one batch of rows shaped like the output of
// workspaceBuildsData: a set of builds plus the flat resource, agent, app,
// script, log source, status, and daemon rows for all of them.
type buildFixture struct {
	builds           []database.WorkspaceBuild
	workspaces       []database.Workspace
	jobs             []database.GetProvisionerJobsByIDsWithQueuePositionRow
	templateVersions []database.TemplateVersion
	resources        []database.WorkspaceResource
	metadata         []database.WorkspaceResourceMetadatum
	agents           []database.WorkspaceAgent
	apps             []database.WorkspaceApp
	statuses         []database.WorkspaceAppStatus
	scripts          []database.GetWorkspaceAgentScriptsByAgentIDsRow
	logSources       []database.WorkspaceAgentLogSource
	daemons          []database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow
}

func newBuildFixture(builds, resourcesPerBuild int, agentsPerResource int32, appsPerAgent int) buildFixture {
	now := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	f := buildFixture{}
	for b := range builds {
		workspaceID := uuid.New()
		jobID := uuid.New()
		templateVersionID := uuid.New()

		f.workspaces = append(f.workspaces, database.Workspace{
			ID:            workspaceID,
			OwnerID:       uuid.New(),
			OwnerUsername: fmt.Sprintf("owner-%d", b),
			Name:          fmt.Sprintf("workspace-%d", b),
		})
		f.jobs = append(f.jobs, database.GetProvisionerJobsByIDsWithQueuePositionRow{
			ProvisionerJob: database.ProvisionerJob{
				ID:        jobID,
				CreatedAt: now,
				StartedAt: sql.NullTime{Time: now, Valid: true},
			},
		})
		f.templateVersions = append(f.templateVersions, database.TemplateVersion{
			ID:   templateVersionID,
			Name: fmt.Sprintf("version-%d", b),
		})
		f.builds = append(f.builds, database.WorkspaceBuild{
			ID:                uuid.New(),
			WorkspaceID:       workspaceID,
			JobID:             jobID,
			TemplateVersionID: templateVersionID,
			BuildNumber:       1,
			Transition:        database.WorkspaceTransitionStart,
		})
		f.daemons = append(f.daemons, database.GetEligibleProvisionerDaemonsByProvisionerJobIDsRow{
			JobID: jobID,
			ProvisionerDaemon: database.ProvisionerDaemon{
				ID:           uuid.New(),
				Name:         fmt.Sprintf("daemon-%d", b),
				LastSeenAt:   sql.NullTime{Time: now, Valid: true},
				Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho},
			},
		})

		for r := range resourcesPerBuild {
			resourceID := uuid.New()
			f.resources = append(f.resources, database.WorkspaceResource{
				ID:         resourceID,
				JobID:      jobID,
				Transition: database.WorkspaceTransitionStart,
				Type:       "example",
				Name:       fmt.Sprintf("resource-%d-%d", b, r),
			})
			f.metadata = append(f.metadata, database.WorkspaceResourceMetadatum{
				WorkspaceResourceID: resourceID,
				Key:                 "key",
				Value:               sql.NullString{String: "value", Valid: true},
			})

			for a := range agentsPerResource {
				agentID := uuid.New()
				f.agents = append(f.agents, database.WorkspaceAgent{
					ID:           agentID,
					ResourceID:   resourceID,
					Name:         fmt.Sprintf("agent-%d-%d-%d", b, r, a),
					DisplayOrder: agentsPerResource - a,
					APIKeyScope:  database.AgentKeyScopeEnumAll,
				})
				f.scripts = append(f.scripts, database.GetWorkspaceAgentScriptsByAgentIDsRow{
					WorkspaceAgentID: agentID,
					Script:           "echo hello",
				})
				f.logSources = append(f.logSources, database.WorkspaceAgentLogSource{
					WorkspaceAgentID: agentID,
					DisplayName:      "startup",
				})

				for p := range appsPerAgent {
					appID := uuid.New()
					f.apps = append(f.apps, database.WorkspaceApp{
						ID:           appID,
						AgentID:      agentID,
						Slug:         fmt.Sprintf("app-%d-%d-%d-%d", b, r, a, p),
						DisplayName:  "App",
						Health:       database.WorkspaceAppHealthHealthy,
						SharingLevel: database.AppSharingLevelOwner,
						OpenIn:       database.WorkspaceAppOpenInSlimWindow,
					})
					f.statuses = append(f.statuses, database.WorkspaceAppStatus{
						ID:      uuid.New(),
						AgentID: agentID,
						AppID:   appID,
						State:   database.WorkspaceAppStatusStateComplete,
					})
				}
			}
		}
	}
	return f
}

func newConvertAPI() *API {
	api := &API{
		Options: &Options{
			AgentInactiveDisconnectTimeout: time.Minute,
			DeploymentValues:               &codersdk.DeploymentValues{},
		},
	}
	coordinator := tailnet.NewCoordinator(slog.Logger{})
	api.TailnetCoordinator.Store(&coordinator)
	return api
}

func (f buildFixture) convert(api *API) ([]codersdk.WorkspaceBuild, error) {
	return api.convertWorkspaceBuilds(
		f.builds,
		f.workspaces,
		f.jobs,
		f.resources,
		f.metadata,
		f.agents,
		f.apps,
		f.statuses,
		f.scripts,
		f.logSources,
		f.templateVersions,
		f.daemons,
	)
}

// TestConvertWorkspaceBuildsAgentOrder covers the ordering that the shared
// index is responsible for: agents come back sorted by display order. The
// fixture assigns display order in reverse of agent name, so the expected
// result is names in descending order.
func TestConvertWorkspaceBuildsAgentOrder(t *testing.T) {
	t.Parallel()

	api := newConvertAPI()
	fixture := newBuildFixture(3, 2, 3, 1)

	builds, err := fixture.convert(api)
	require.NoError(t, err)
	require.Len(t, builds, 3)

	for _, build := range builds {
		require.NotEmpty(t, build.Resources)
		for _, resource := range build.Resources {
			require.Len(t, resource.Agents, 3)
			for i := 1; i < len(resource.Agents); i++ {
				require.Greater(t, resource.Agents[i-1].Name, resource.Agents[i].Name)
			}
		}
	}
}

// TestConvertWorkspaceBuildsAgentNameTiebreak covers the second half of the
// agent comparison: agents sharing a display order are ordered by name.
func TestConvertWorkspaceBuildsAgentNameTiebreak(t *testing.T) {
	t.Parallel()

	api := newConvertAPI()
	fixture := newBuildFixture(1, 1, 3, 1)
	for i := range fixture.agents {
		fixture.agents[i].DisplayOrder = 0
	}

	builds, err := fixture.convert(api)
	require.NoError(t, err)
	require.Len(t, builds, 1)
	require.Len(t, builds[0].Resources, 1)

	agents := builds[0].Resources[0].Agents
	require.Len(t, agents, 3)
	for i := 1; i < len(agents); i++ {
		require.Less(t, agents[i-1].Name, agents[i].Name)
	}
}

// TestConvertWorkspaceBuildsRowsPerBuild asserts that a build receives only the
// rows keyed to its own job and agents. The fixture encodes the build index in
// app slugs, so apps read from another build are detectable by slug.
func TestConvertWorkspaceBuildsRowsPerBuild(t *testing.T) {
	t.Parallel()

	const (
		buildCount      = 3
		resourcesPerJob = 2
		agentsPerRes    = 2
		appsPerAgent    = 2
	)

	api := newConvertAPI()
	fixture := newBuildFixture(buildCount, resourcesPerJob, agentsPerRes, appsPerAgent)

	builds, err := fixture.convert(api)
	require.NoError(t, err)
	require.Len(t, builds, buildCount)

	for b, build := range builds {
		require.Len(t, build.Resources, resourcesPerJob)

		for _, resource := range build.Resources {
			require.Equal(t, fixture.jobs[b].ProvisionerJob.ID, resource.JobID)
			require.Len(t, resource.Metadata, 1)
			require.Len(t, resource.Agents, agentsPerRes)

			for _, agent := range resource.Agents {
				require.Len(t, agent.Scripts, 1)
				require.Len(t, agent.LogSources, 1)
				require.Len(t, agent.Apps, appsPerAgent)

				for _, app := range agent.Apps {
					require.True(t, strings.HasPrefix(app.Slug, fmt.Sprintf("app-%d-", b)), app.Slug)
					require.Len(t, app.Statuses, 1)
				}
			}
		}
	}
}

// TestConvertWorkspaceBuildsMatchedProvisioners asserts that eligible daemons
// are matched to the build whose job they were fetched for. The last build in
// the batch has no daemon rows, so it reports zero counts while the others
// report one.
func TestConvertWorkspaceBuildsMatchedProvisioners(t *testing.T) {
	t.Parallel()

	api := newConvertAPI()
	fixture := newBuildFixture(3, 1, 1, 1)
	fixture.daemons = fixture.daemons[:len(fixture.daemons)-1]

	builds, err := fixture.convert(api)
	require.NoError(t, err)
	require.Len(t, builds, 3)

	for _, build := range builds[:2] {
		require.NotNil(t, build.MatchedProvisioners)
		require.Equal(t, 1, build.MatchedProvisioners.Count)
		require.Equal(t, 1, build.MatchedProvisioners.Available)
		require.True(t, build.MatchedProvisioners.MostRecentlySeen.Valid)
	}

	last := builds[2]
	require.NotNil(t, last.MatchedProvisioners)
	require.Equal(t, 0, last.MatchedProvisioners.Count)
	require.Equal(t, 0, last.MatchedProvisioners.Available)
	require.False(t, last.MatchedProvisioners.MostRecentlySeen.Valid)
}

func BenchmarkConvertWorkspaceBuilds(b *testing.B) {
	for _, builds := range []int{1, 25, 100} {
		b.Run(fmt.Sprintf("Builds%d", builds), func(b *testing.B) {
			api := newConvertAPI()
			fixture := newBuildFixture(builds, 5, 2, 4)

			b.ReportAllocs()
			for b.Loop() {
				if _, err := fixture.convert(api); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
