package healthcheck_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/apiversion"
	"github.com/coder/coder/v2/provisionersdk"
)

func TestProvisionerDaemonReport(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name                 string
		currentVersion       string
		currentAPIVersion    *apiversion.APIVersion
		provisionerDaemonsFn func(context.Context) ([]database.ProvisionerDaemon, error)
		expectedSeverity     health.Severity
		expectedWarningCode  health.Code
		expectedError        string
	}{
		{
			name:             "current version empty",
			currentVersion:   "",
			expectedSeverity: health.SeverityError,
			expectedError:    "Developer error: CurrentVersion is empty",
		},
		{
			name:              "provisionerdaemonsfn nil",
			currentVersion:    "v1.2.3",
			currentAPIVersion: provisionersdk.VersionCurrent,
			expectedSeverity:  health.SeverityError,
			expectedError:     "Developer error: ProvisionerDaemonsFn is nil",
		},
		{
			name:                 "no daemons",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    provisionersdk.VersionCurrent,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(),
			expectedSeverity:     health.SeverityError,
			expectedError:        "No provisioner daemons found!",
		},
		{
			name:                 "error fetching daemons",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    provisionersdk.VersionCurrent,
			provisionerDaemonsFn: fakeProvisionerDaemonsFnErr(assert.AnError),
			expectedSeverity:     health.SeverityError,
			expectedError:        assert.AnError.Error(),
		},
		{
			name:                 "one daemon up to date",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    provisionersdk.VersionCurrent,
			expectedSeverity:     health.SeverityOK,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "1.0")),
		},
		{
			name:                 "one daemon out of date",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    provisionersdk.VersionCurrent,
			expectedSeverity:     health.SeverityWarning,
			expectedWarningCode:  health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(fakeProvisionerDaemon(t, "pd-old", "v1.1.2", "1.0")),
		},
		{
			name:                 "major api version not available",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    provisionersdk.VersionCurrent,
			expectedSeverity:     health.SeverityError,
			expectedWarningCode:  health.CodeProvisionerDaemonAPIVersionIncompatible,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(fakeProvisionerDaemon(t, "pd-new-major", "v1.2.3", "2.0")),
		},
		{
			name:                 "minor api version not available",
			currentVersion:       "v1.2.3",
			currentAPIVersion:    provisionersdk.VersionCurrent,
			expectedSeverity:     health.SeverityError,
			expectedWarningCode:  health.CodeProvisionerDaemonAPIVersionIncompatible,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(fakeProvisionerDaemon(t, "pd-new-minor", "v1.2.3", "1.1")),
		},
		{
			name:              "api version backward compat",
			currentVersion:    "v2.3.4",
			currentAPIVersion: apiversion.New(2, 0).WithBackwardCompat(1),
			expectedSeverity:  health.SeverityOK,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(
				fakeProvisionerDaemon(t, "pd-old-api", "v2.3.4", "1.0")),
		},
		{
			name:                "one up to date, one out of date",
			currentVersion:      "v1.2.3",
			currentAPIVersion:   provisionersdk.VersionCurrent,
			expectedSeverity:    health.SeverityWarning,
			expectedWarningCode: health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(
				fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "1.0"),
				fakeProvisionerDaemon(t, "pd-old", "v1.1.2", "1.0")),
		},
		{
			name:                "one up to date, one newer",
			currentVersion:      "v1.2.3",
			currentAPIVersion:   provisionersdk.VersionCurrent,
			expectedSeverity:    health.SeverityWarning,
			expectedWarningCode: health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(
				fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "1.0"),
				fakeProvisionerDaemon(t, "pd-new", "v2.3.4", "1.0")),
		},
		{
			name:              "one up to date, one stale older",
			currentVersion:    "v2.3.4",
			currentAPIVersion: provisionersdk.VersionCurrent,
			expectedSeverity:  health.SeverityOK,
			provisionerDaemonsFn: fakeProvisionerDaemonsFn(
				fakeProvisionerDaemonStale(t, "pd-ok", "v1.2.3", "0.9", dbtime.Now().Add(-5*time.Minute)),
				fakeProvisionerDaemon(t, "pd-new", "v2.3.4", "1.0")),
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var rpt healthcheck.ProvisionerDaemonsReport
			var opts healthcheck.ProvisionerDaemonsReportOptions
			opts.CurrentVersion = tt.currentVersion
			if tt.currentAPIVersion == nil {
				opts.CurrentAPIVersion = provisionersdk.VersionCurrent
			} else {
				opts.CurrentAPIVersion = tt.currentAPIVersion
			}
			if tt.provisionerDaemonsFn != nil {
				opts.ProvisionerDaemonsFn = tt.provisionerDaemonsFn
			}
			now := dbtime.Now()
			opts.TimeNowFn = func() time.Time {
				return now
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
			if tt.expectedError != "" && assert.NotNil(t, rpt.Error) {
				assert.Contains(t, *rpt.Error, tt.expectedError)
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

func fakeProvisionerDaemonStale(t *testing.T, name, version, apiVersion string, lastSeenAt time.Time) database.ProvisionerDaemon {
	t.Helper()
	d := fakeProvisionerDaemon(t, name, version, apiVersion)
	d.LastSeenAt.Valid = true
	d.LastSeenAt.Time = lastSeenAt
	return d
}
