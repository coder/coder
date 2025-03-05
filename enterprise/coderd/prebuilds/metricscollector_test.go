package prebuilds_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
)

func TestMetricsCollector(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires postgres")
	}

	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())

	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	collector := prebuilds.NewMetricsCollector(db, logger)

	registry := prometheus.NewRegistry()
	registry.Register(collector)

	preset, err := db.InsertPreset(context.Background(), database.InsertPresetParams{
		TemplateVersionID: templateVersion.ID,
		Name:              "test",
	})
	require.NoError(t, err)
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		TemplateID:     template.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		CompletedAt:    sql.NullTime{Time: time.Now(), Valid: true},
		InitiatorID:    prebuilds.OwnerID,
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:             workspace.ID,
		TemplateVersionID:       templateVersion.ID,
		TemplateVersionPresetID: uuid.NullUUID{UUID: preset.ID, Valid: true},
		InitiatorID:             prebuilds.OwnerID,
		JobID:                   job.ID,
	})

	metrics, err := registry.Gather()
	require.NoError(t, err)
	require.Equal(t, 5, len(metrics))
}
