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
		builds = []database.WorkspaceBuildThin{
			dbgen.WorkspaceBuild(t, db, database.WorkspaceBuildThin{
				WorkspaceID:       workspace.ID,
				TemplateVersionID: version.ID,
				BuildNumber:       1,
				Transition:        database.WorkspaceTransitionStart,
				InitiatorID:       user.ID,
				JobID:             jobs[0].ID,
				Reason:            database.BuildReasonInitiator,
				CreatedAt:         time.Now(),
			}),
			dbgen.WorkspaceBuild(t, db, database.WorkspaceBuildThin{
				WorkspaceID:       workspace.ID,
				TemplateVersionID: version.ID,
				BuildNumber:       2,
				Transition:        database.WorkspaceTransitionStart,
				InitiatorID:       user.ID,
				JobID:             jobs[1].ID,
				Reason:            database.BuildReasonInitiator,
				CreatedAt:         time.Now().Add(time.Hour),
			}),
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
			require.Equal(t, expected, build.WorkspaceBuildThin, "builds should be equal")
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
			require.Equal(t, expected, build.WorkspaceBuildThin, "builds should be equal")
		}
	})

	t.Run("GetWorkspaceBuildsCreatedAfter", func(t *testing.T) {
		t.Parallel()
		builds, err := db.GetWorkspaceBuildsCreatedAfter(ctx, jobs[0].CreatedAt)
		if err != nil {
			t.Fatal(err)
		}
		expected := builds[1]
		require.Len(t, builds, 1, "should only be one build")
		require.Equal(t, expected, builds[0], "builds should be equal")
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
			require.Equal(t, expected, build.WorkspaceBuildThin, "builds should be equal")
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
		exp := []database.WorkspaceBuildThin{
			builds[1],
			builds[0],
		}
		require.Equal(t, exp, toThins(found), "builds should be equal")
	})

	t.Run("GetLatestWorkspaceBuildsByWorkspaceIDs", func(t *testing.T) {
		t.Parallel()
		found, err := db.GetLatestWorkspaceBuildsByWorkspaceIDs(ctx, []uuid.UUID{workspace.ID})
		if err != nil {
			t.Fatal(err)
		}
		require.Len(t, found, 2, "should be two builds")
		require.Equal(t, builds, found, "builds should be equal")
	})

	t.Run("GetLatestWorkspaceBuilds", func(t *testing.T) {
		t.Parallel()
		found, err := db.GetLatestWorkspaceBuilds(ctx)
		if err != nil {
			t.Fatal(err)
		}
		require.Len(t, found, 1, "should be only 1 build")
		require.Equal(t, builds[1], toThins(found), "builds should be equal")
	})

	t.Run("GetLatestWorkspaceBuildByWorkspaceID", func(t *testing.T) {
		t.Parallel()
		found, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
		if err != nil {
			t.Fatal(err)
		}
		require.Equal(t, builds[1], found, "builds should be equal")
	})
}

func toThins(builds []database.WorkspaceBuild) []database.WorkspaceBuildThin {
	thins := make([]database.WorkspaceBuildThin, len(builds))
	for i, build := range builds {
		thins[i] = build.WorkspaceBuildThin
	}
	return thins
}
