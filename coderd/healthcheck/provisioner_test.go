package healthcheck_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/provisionersdk"

	gomock "go.uber.org/mock/gomock"
)

func TestProvisionerDaemonReport(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name                   string
		currentVersion         string
		currentAPIMajorVersion int
		provisionerDaemons     []database.ProvisionerDaemon
		provisionerDaemonsErr  error
		expectedSeverity       health.Severity
		expectedWarningCode    health.Code
		expectedError          string
	}{
		{
			name:             "current version empty",
			currentVersion:   "",
			expectedSeverity: health.SeverityError,
			expectedError:    "Developer error: CurrentVersion is empty",
		},
		{
			name:                   "no daemons",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedError:          "No active provisioner daemons found!",
		},
		{
			name:                   "error fetching daemons",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			provisionerDaemonsErr:  assert.AnError,
			expectedSeverity:       health.SeverityError,
			expectedError:          assert.AnError.Error(),
		},
		{
			name:                   "one daemon up to date",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityOK,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "1.0")},
		},
		{
			name:                   "one daemon out of date",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-old", "v1.1.2", "1.0")},
		},
		{
			name:                   "invalid daemon version",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedWarningCode:    health.CodeUnknown,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-invalid-version", "invalid", "1.0")},
		},
		{
			name:                   "invalid daemon api version",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedWarningCode:    health.CodeUnknown,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-new-minor", "v1.2.3", "invalid")},
		},
		{
			name:                   "api version backward compat",
			currentVersion:         "v2.3.4",
			currentAPIMajorVersion: 2,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonAPIMajorVersionDeprecated,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-old-api", "v2.3.4", "1.0")},
		},
		{
			name:                   "one up to date, one out of date",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "1.0"), fakeProvisionerDaemon(t, "pd-old", "v1.1.2", "1.0")},
		},
		{
			name:                   "one up to date, one newer",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "1.0"), fakeProvisionerDaemon(t, "pd-new", "v2.3.4", "1.0")},
		},
		{
			name:                   "one up to date, one stale older",
			currentVersion:         "v2.3.4",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityOK,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemonStale(t, "pd-ok", "v1.2.3", "0.9", dbtime.Now().Add(-5*time.Minute)), fakeProvisionerDaemon(t, "pd-new", "v2.3.4", "1.0")},
		},
		{
			name:                   "one stale",
			currentVersion:         "v2.3.4",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedError:          "No active provisioner daemons found!",
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemonStale(t, "pd-ok", "v1.2.3", "0.9", dbtime.Now().Add(-5*time.Minute))},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var rpt healthcheck.ProvisionerDaemonsReport
			var deps healthcheck.ProvisionerDaemonsReportDeps
			deps.CurrentVersion = tt.currentVersion
			deps.CurrentAPIMajorVersion = tt.currentAPIMajorVersion
			if tt.currentAPIMajorVersion == 0 {
				deps.CurrentAPIMajorVersion = provisionersdk.CurrentMajor
			}
			now := dbtime.Now()
			deps.TimeNow = func() time.Time {
				return now
			}

			ctrl := gomock.NewController(t)
			mDB := dbmock.NewMockStore(ctrl)
			mDB.EXPECT().GetProvisionerDaemons(gomock.Any()).AnyTimes().Return(tt.provisionerDaemons, tt.provisionerDaemonsErr)
			deps.Store = mDB

			rpt.Run(context.Background(), &deps)

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

func fakeProvisionerDaemonStale(t *testing.T, name, version, apiVersion string, lastSeenAt time.Time) database.ProvisionerDaemon {
	t.Helper()
	d := fakeProvisionerDaemon(t, name, version, apiVersion)
	d.LastSeenAt.Valid = true
	d.LastSeenAt.Time = lastSeenAt
	return d
}
