package monitoring_test

import (
	"context"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/monitoring"
	"github.com/google/uuid"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestMonitoring(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db := databasefake.New()

	monitor := monitoring.New(ctx, &monitoring.Options{
		Database:        db,
		Logger:          slogtest.Make(t, nil),
		RefreshInterval: time.Minute,
		Telemetry:       monitoring.TelemetryNone,
	})

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

	err := monitor.Refresh()
	require.NoError(t, err)

	metrics, err := monitor.Gather()
	require.NoError(t, err)

	type labels struct {
		name  string
		value string
	}

	tests := []struct {
		name   string
		total  int
		labels []labels
	}{
		{
			name:  "coder_users",
			total: 1,
			labels: []labels{
				{name: "user_name", value: "kyle"},
			},
		},
		{
			name:  "coder_workspaces",
			total: 2,
			labels: []labels{
				{name: "workspace_name", value: "banana1"},
				{name: "workspace_name", value: "banana4"},
			},
		},
		{
			name:  "coder_workspace_resources",
			total: 2,
			labels: []labels{
				{name: "workspace_resource_type", value: "google_compute_instance"},
			},
		},
	}
	require.Len(t, metrics, len(tests))
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			metricFamily, err := findMetric(t, tt.name, metrics)
			require.NoError(t, err)

			require.Len(t, metricFamily.GetMetric(), tt.total)

			for _, l := range tt.labels {
				require.NoError(t, findMetricLabel(t, l.name, l.value, metricFamily))
			}
		})
	}
}

func findMetric(_ *testing.T, name string, metrics []*dto.MetricFamily) (*dto.MetricFamily, error) {
	for _, m := range metrics {
		if m.GetName() == name {
			return m, nil
		}
	}
	return nil, xerrors.Errorf("no metric %s in %v", name, metrics)
}

func findMetricLabel(_ *testing.T, name string, value string, metricFamily *dto.MetricFamily) error {
	for _, m := range metricFamily.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == name && l.GetValue() == value {
				return nil
			}
		}
	}
	return xerrors.Errorf("no metric label %s:%s in %v", name, value, metricFamily)
}
