package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/coderd/rbac"
)

func TestGetWorkspaceBuild(t *testing.T) {
	t.Parallel()
	if !dbtestutil.UsingRealDatabase() {
		t.Skip("Test only runs against a real database")
	}

	db, _ := dbtestutil.NewDB(t)

	// Seed the database with some workspace builds.
	var (
		now  = database.Now()
		org  = dbgen.Organization(t, db, database.Organization{})
		user = dbgen.User(t, db, database.User{
			RBACRoles: []string{rbac.RoleOwner()},
		})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		template = dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		version = dbgen.TemplateVersion(t, db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{
				UUID:  template.ID,
				Valid: true,
			},
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		workspace = dbgen.Workspace(t, db, database.Workspace{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     template.ID,
		})
		jobs = []database.ProvisionerJob{
			dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
				OrganizationID: org.ID,
				InitiatorID:    user.ID,
			}),
			dbgen.ProvisionerJob(t, db, database.ProvisionerJob{
				OrganizationID: org.ID,
				InitiatorID:    user.ID,
			}),
		}
		builds = []database.WorkspaceBuild{
			dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				WorkspaceID:       workspace.ID,
				TemplateVersionID: version.ID,
				BuildNumber:       1,
				Transition:        database.WorkspaceTransitionStart,
				InitiatorID:       user.ID,
				JobID:             jobs[0].ID,
				Reason:            database.BuildReasonInitiator,
				CreatedAt:         now,
			}),
			dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				WorkspaceID:       workspace.ID,
				TemplateVersionID: version.ID,
				BuildNumber:       2,
				Transition:        database.WorkspaceTransitionStart,
				InitiatorID:       user.ID,
				JobID:             jobs[1].ID,
				Reason:            database.BuildReasonInitiator,
				CreatedAt:         now.Add(time.Hour),
			}),
		}
		orderBuilds = []database.WorkspaceBuild{
			builds[1],
			builds[0],
		}
		ctx = context.Background()
	)

	t.Run("GetWorkspaceBuildByID", func(t *testing.T) {
		t.Parallel()
		for _, expected := range builds {
			build, err := db.GetWorkspaceBuildByID(ctx, expected.ID)
			if err != nil {
				t.Fatal(err)
			}
			require.Equal(t, expected, build.WorkspaceBuild, "builds should be equal")
		}
	})

	t.Run("GetWorkspaceBuildByJobID", func(t *testing.T) {
		t.Parallel()
		for i, job := range jobs {
			build, err := db.GetWorkspaceBuildByJobID(ctx, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			expected := builds[i]
			require.Equal(t, expected, build.WorkspaceBuild, "builds should be equal")
		}
	})

	t.Run("GetWorkspaceBuildsCreatedAfter", func(t *testing.T) {
		t.Parallel()
		found, err := db.GetWorkspaceBuildsCreatedAfter(ctx, builds[0].CreatedAt.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		expected := builds[1]
		require.Len(t, found, 1, "should only be one build")
		require.Equal(t, expected, found[0].WorkspaceBuild, "builds should be equal")
	})

	t.Run("GetWorkspaceBuildByWorkspaceIDAndBuildNumber", func(t *testing.T) {
		t.Parallel()
		for _, expected := range builds {
			build, err := db.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
				BuildNumber: expected.BuildNumber,
				WorkspaceID: expected.WorkspaceID,
			})
			if err != nil {
				t.Fatal(err)
			}
			require.Equal(t, expected, build.WorkspaceBuild, "builds should be equal")
		}
	})

	t.Run("GetWorkspaceBuildsByWorkspaceID", func(t *testing.T) {
		t.Parallel()
		found, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
			WorkspaceID: workspace.ID,
			Since:       builds[0].CreatedAt.Add(-1 * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		require.Len(t, found, 2, "should be two builds")
		require.Equal(t, orderBuilds, toThins(found), "builds should be equal")
	})

	t.Run("GetLatestWorkspaceBuildsByWorkspaceIDs", func(t *testing.T) {
		t.Parallel()
		found, err := db.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, []uuid.UUID{workspace.ID})
		if err != nil {
			t.Fatal(err)
		}
		require.Len(t, found, 1, "should be only one build")
		require.Equal(t, builds[1], found[0].WorkspaceBuild, "builds should be equal")
	})

	t.Run("GetLatestWorkspaceBuilds", func(t *testing.T) {
		t.Parallel()
		found, err := db.GetLatestWorkspaceBuilds(ctx)
		if err != nil {
			t.Fatal(err)
		}
		require.Len(t, found, 1, "should be only 1 build")
		require.Equal(t, builds[1], found[0].WorkspaceBuild, "builds should be equal")
	})

	t.Run("GetLatestWorkspaceBuildByWorkspaceID", func(t *testing.T) {
		t.Parallel()
		found, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
		if err != nil {
			t.Fatal(err)
		}
		require.Equal(t, builds[1], found.WorkspaceBuild, "builds should be equal")
	})
}

func toThins(builds []database.WorkspaceBuildRBAC) []database.WorkspaceBuild {
	thins := make([]database.WorkspaceBuild, len(builds))
	for i, build := range builds {
		thins[i] = build.WorkspaceBuild
	}
	return thins
}
