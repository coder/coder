package coderd

import (
	"database/sql"
	"fmt"
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
// index is responsible for: agents come back sorted by display order, and the
// order does not depend on how many builds read the same rows. The fixture
// assigns display order in reverse of agent name, so the expected result is
// names in descending order.
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
