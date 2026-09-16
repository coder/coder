package dynamicparameters_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/provisioner/sandbox"
)

func TestSandboxParametersWithoutTerraformMetadata(t *testing.T) {
	t.Parallel()
	version := database.TemplateVersion{ID: uuid.New(), JobID: uuid.New()}
	job := database.ProvisionerJob{
		ID:          version.JobID,
		Provisioner: database.ProvisionerTypeSandbox,
		CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
	}
	// The native renderer must not query Terraform metadata, fetch archives,
	// or resolve workspace-owner expressions. Unexpected calls fail the test.
	db := dbmock.NewMockStore(gomock.NewController(t))
	render, err := dynamicparameters.Prepare(t.Context(), db, nil, version.ID,
		dynamicparameters.WithTemplateVersion(version),
		dynamicparameters.WithProvisionerJob(job),
	)
	require.NoError(t, err)
	defer render.Close()

	output, diags := render.Render(t.Context(), uuid.New(), nil)
	require.Empty(t, diags)
	require.Empty(t, output.Parameters)
	require.Equal(t, map[string]string{sandbox.HostTag: sandbox.HostID}, output.WorkspaceTags.Tags())

	_, diags = render.Render(t.Context(), uuid.New(), map[string]string{"cpu": "64"})
	require.True(t, diags.HasErrors())
}
