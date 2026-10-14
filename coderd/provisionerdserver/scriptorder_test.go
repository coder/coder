package provisionerdserver_test

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestCompleteJobPersistsScriptOrderCounts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	srv, db, _, daemon := setup(t, false, nil)
	user := dbgen.User(t, db, database.User{})

	// Given a template version with an acquired import job.
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		CreatedBy:      user.ID,
		OrganizationID: daemon.OrganizationID,
		JobID:          uuid.New(),
	})
	input, err := json.Marshal(provisionerdserver.TemplateVersionImportJob{
		TemplateVersionID: version.ID,
	})
	require.NoError(t, err)
	job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             version.JobID,
		OrganizationID: daemon.OrganizationID,
		InitiatorID:    user.ID,
		Provisioner:    database.ProvisionerTypeEcho,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
		Input:          input,
		Tags:           daemon.Tags,
	})
	require.NoError(t, err)
	tags, err := json.Marshal(job.Tags)
	require.NoError(t, err)
	_, err = db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
		OrganizationID:  daemon.OrganizationID,
		WorkerID:        uuid.NullUUID{UUID: daemon.ID, Valid: true},
		Types:           []database.ProvisionerType{database.ProvisionerTypeEcho},
		StartedAt:       sql.NullTime{Time: dbtime.Now(), Valid: true},
		ProvisionerTags: tags,
	})
	require.NoError(t, err)

	// When the import completes with script-order counts.
	_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
		JobId: job.ID.String(),
		Type: &proto.CompletedJob_TemplateImport_{
			TemplateImport: &proto.CompletedJob_TemplateImport{
				ScriptOrderDataSourceCount: 2,
				ScriptOrderRuleCount:       3,
			},
		},
	})
	require.NoError(t, err)

	// Then counts are persisted but excluded from the telemetry summary.
	version, err = db.GetTemplateVersionByID(ctx, version.ID)
	require.NoError(t, err)
	require.Equal(t, int32(2), version.ScriptOrderDataSourceCount)
	require.Equal(t, int32(3), version.ScriptOrderRuleCount)

	usage, err := db.GetScriptOrderTelemetrySummary(ctx)
	require.NoError(t, err)
	require.Equal(t, database.GetScriptOrderTelemetrySummaryRow{}, usage)

	// When the version becomes a template's active version.
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  daemon.OrganizationID,
		CreatedBy:       user.ID,
		ActiveVersionID: version.ID,
	})
	err = db.UpdateTemplateVersionByID(ctx, database.UpdateTemplateVersionByIDParams{
		ID:         version.ID,
		TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true},
		UpdatedAt:  dbtime.Now(),
		Name:       version.Name,
		Message:    version.Message,
	})
	require.NoError(t, err)

	// Then its counts are included in the telemetry summary.
	usage, err = db.GetScriptOrderTelemetrySummary(ctx)
	require.NoError(t, err)
	require.Equal(t, database.GetScriptOrderTelemetrySummaryRow{
		TemplatesWithScriptOrder: 1,
		DataSourceCount:          2,
		RuleCount:                3,
	}, usage)
}

// Defensively ignore negative telemetry counts from provisioner bugs
// so they do not fail an otherwise successful template import.
func TestCompleteJobSkipsNegativeScriptOrderCounts(t *testing.T) {
	t.Parallel()

	srv, db, _, daemon := setup(t, false, nil)
	user := dbgen.User(t, db, database.User{})

	for _, tt := range []struct {
		name            string
		dataSources     int32
		rules           int32
		wantDataSources int32
	}{
		{name: "ZeroCounts"},
		{name: "ZeroRules", dataSources: 2, wantDataSources: 2},
		{name: "NegativeDataSourceCount", dataSources: -1, rules: 3},
		{name: "NegativeRuleCount", dataSources: 2, rules: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			// Given a template version with an acquired import job.
			version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
				CreatedBy:      user.ID,
				OrganizationID: daemon.OrganizationID,
				JobID:          uuid.New(),
			})
			input, err := json.Marshal(provisionerdserver.TemplateVersionImportJob{
				TemplateVersionID: version.ID,
			})
			require.NoError(t, err)
			job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				ID:             version.JobID,
				OrganizationID: daemon.OrganizationID,
				InitiatorID:    user.ID,
				Type:           database.ProvisionerJobTypeTemplateVersionImport,
				Input:          input,
				Tags:           daemon.Tags,
				StartedAt:      sql.NullTime{Time: dbtime.Now(), Valid: true},
				WorkerID:       uuid.NullUUID{UUID: daemon.ID, Valid: true},
			})

			// When the import completes with the supplied script-order counts.
			_, err = srv.CompleteJob(ctx, &proto.CompletedJob{
				JobId: job.ID.String(),
				Type: &proto.CompletedJob_TemplateImport_{
					TemplateImport: &proto.CompletedJob_TemplateImport{
						ScriptOrderDataSourceCount: tt.dataSources,
						ScriptOrderRuleCount:       tt.rules,
					},
				},
			})
			require.NoError(t, err)

			// Then the import succeeds, skipping both counts if either is negative.
			job, err = db.GetProvisionerJobByID(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, job.CompletedAt.Valid)
			require.Empty(t, job.Error.String)

			version, err = db.GetTemplateVersionByID(ctx, version.ID)
			require.NoError(t, err)
			require.Equal(t, tt.wantDataSources, version.ScriptOrderDataSourceCount)
			require.Zero(t, version.ScriptOrderRuleCount)
		})
	}
}
