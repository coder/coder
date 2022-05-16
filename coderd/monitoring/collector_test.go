package monitoring_test

import (
	"context"
	"strings"
	"testing"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/monitoring"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestCollector(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db := databasefake.New()
	populateDB(ctx, db)

	collector := monitoring.NewCollector(ctx, db)
	expected := `
		# HELP coder_users The users in a Coder deployment.
        # TYPE coder_users gauge
        coder_users 1
        # HELP coder_workspace_resources The workspace resources in a Coder deployment.
        # TYPE coder_workspace_resources gauge
        coder_workspace_resources{workspace_resource_type="google_compute_instance"} 2
        # HELP coder_workspaces The workspaces in a Coder deployment.
        # TYPE coder_workspaces gauge
        coder_workspaces 2
	`
	require.NoError(t, testutil.CollectAndCompare(collector, strings.NewReader(expected)))
}

func populateDB(ctx context.Context, db database.Store) {
	user, _ := db.InsertUser(ctx, database.InsertUserParams{
		ID:       uuid.New(),
		Username: "kyle",
	})
	org, _ := db.InsertOrganization(ctx, database.InsertOrganizationParams{
		ID:   uuid.New(),
		Name: "potato",
	})
	template, _ := db.InsertTemplate(ctx, database.InsertTemplateParams{
		ID:             uuid.New(),
		Name:           "something",
		OrganizationID: org.ID,
	})
	workspace, _ := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
		ID:             uuid.New(),
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		Name:           "banana1",
	})
	job, _ := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             uuid.New(),
		OrganizationID: org.ID,
	})
	version, _ := db.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
		ID: uuid.New(),
		TemplateID: uuid.NullUUID{
			UUID:  template.ID,
			Valid: true,
		},
		CreatedAt:      database.Now(),
		OrganizationID: org.ID,
		JobID:          job.ID,
	})
	db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
		ID:                uuid.New(),
		JobID:             job.ID,
		WorkspaceID:       workspace.ID,
		TemplateVersionID: version.ID,
		Transition:        database.WorkspaceTransitionStart,
	})
	db.InsertWorkspaceResource(ctx, database.InsertWorkspaceResourceParams{
		ID:    uuid.New(),
		JobID: job.ID,
		Type:  "google_compute_instance",
		Name:  "banana2",
	})
	db.InsertWorkspaceResource(ctx, database.InsertWorkspaceResourceParams{
		ID:    uuid.New(),
		JobID: job.ID,
		Type:  "google_compute_instance",
		Name:  "banana3",
	})
	db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
		ID:             uuid.New(),
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     template.ID,
		Name:           "banana4",
	})
}
