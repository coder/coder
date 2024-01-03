package healthcheck_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
)

func TestProvisionerDaemonReport(t *testing.T) {
	t.Parallel()

	var ()

	for _, tt := range []struct {
		name                 string
		currentVersion       string
		currentAPIVersion    string
		provisionerDaemonsFn func(context.Context) ([]database.ProvisionerDaemon, error)
		expectedSeverity     health.Severity
		expectedWarningCode  health.Code
	}{
		{
			name:                "current version empty",
			currentVersion:      "",
			expectedSeverity:    health.SeverityError,
			expectedWarningCode: health.CodeUnknown,
		},
		{
			name:                "current api version empty",
			currentVersion:      "v1.2.3",
			currentAPIVersion:   "",
			expectedSeverity:    health.SeverityError,
			expectedWarningCode: health.CodeUnknown,
		},
		{
			name:                "provisionerdaemonsfn nil",
			currentVersion:      "v1.2.3",
			currentAPIVersion:   "v1.0",
			expectedSeverity:    health.SeverityError,
			expectedWarningCode: health.CodeUnknown,
		},
		{
			name:                 "no daemons",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    "v1.0",
			expectedSeverity:     health.SeverityError,
			expectedWarningCode:  health.CodeProvisionerDaemonsNoProvisionerDaemons,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(),
		},
		{
			name:                 "one daemon up to date",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    "v1.0",
			expectedSeverity:     health.SeverityOK,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "v1.0")),
		},
		{
			name:                 "one daemon out of date",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    "v1.0",
			expectedSeverity:     health.SeverityWarning,
			expectedWarningCode:  health.CodeProvisionerDaemonVersionOutOfDate,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(fakeProvisionerDaemon(t, "pd-old", "v1.1.2", "v1.0")),
		},
		{
			name:                 "major api version not available",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    "v1.0",
			expectedSeverity:     health.SeverityError,
			expectedWarningCode:  health.CodeProvisionerDaemonAPIMajorVersionNotAvailable,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(fakeProvisionerDaemon(t, "pd-new-major", "v1.2.3", "v2.0")),
		},
		{
			name:                 "minor api version not available",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    "v1.0",
			expectedSeverity:     health.SeverityWarning,
			expectedWarningCode:  health.CodeProvisionerDaemonAPIMinorVersionNotAvailable,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(fakeProvisionerDaemon(t, "pd-new-minor", "v1.2.3", "v1.1")),
		},
		{
			name:                "one up to date, one out of date",
			currentVersion:      "v1.2.3",
			currentAPIVersion:   "v1.0",
			expectedSeverity:    health.SeverityWarning,
			expectedWarningCode: health.CodeProvisionerDaemonVersionOutOfDate,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(
				fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "v1.0"),
				fakeProvisionerDaemon(t, "pd-old", "v1.1.2", "v1.0")),
		},
		{
			name:              "one up to date, one newer",
			currentVersion:    "v1.2.3",
			currentAPIVersion: "v1.0",
			expectedSeverity:  health.SeverityOK,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(
				fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "v1.0"),
				fakeProvisionerDaemon(t, "pd-new", "v2.3.4", "v1.0")),
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var rpt healthcheck.ProvisionerDaemonReport
			var opts healthcheck.ProvisionerDaemonReportOptions
			opts.CurrentVersion = tt.currentVersion
			opts.CurrentAPIVersion = tt.currentAPIVersion
			if tt.provisionerDaemonsFn != nil {
				opts.ProvisionerDaemonsFn = tt.provisionerDaemonsFn
			}

			rpt.Run(context.Background(), &opts)

			assert.Equal(t, tt.expectedSeverity, rpt.Severity)
			if tt.expectedWarningCode != "" && assert.NotEmpty(t, rpt.Warnings) {
				var found bool
				for _, w := range rpt.Warnings {
					if w.Code == tt.expectedWarningCode {
						found = true
						break
					}
				}
				assert.True(t, found, "expected warning %s not found in %v", tt.expectedWarningCode, rpt.Warnings)
			} else {
				assert.Empty(t, rpt.Warnings)
			}
		})
	}
}

func fakeProvisionerDaemon(t *testing.T, name, version, apiVersion string) database.ProvisionerDaemon {
	t.Helper()
	return database.ProvisionerDaemon{
		ID:           uuid.New(),
		Name:         name,
		CreatedAt:    dbtime.Now(),
		LastSeenAt:   sql.NullTime{Time: dbtime.Now(), Valid: true},
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho, database.ProvisionerTypeTerraform},
		ReplicaID:    uuid.NullUUID{},
		Tags:         map[string]string{},
		Version:      version,
		APIVersion:   apiVersion,
	}
}

func fakeProvisionerDaemonsFn(pds ...database.ProvisionerDaemon) func(context.Context) ([]database.ProvisionerDaemon, error) {
	return func(context.Context) ([]database.ProvisionerDaemon, error) {
		return pds, nil
	}
}

func fakeProvisionerDaemonsFnErr(err error) func(context.Context) ([]database.ProvisionerDaemon, error) {
	return func(context.Context) ([]database.ProvisionerDaemon, error) {
		return nil, err
	}
}
