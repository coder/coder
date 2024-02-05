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
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"

	gomock "go.uber.org/mock/gomock"
)

func TestProvisionerDaemonReport(t *testing.T) {
	t.Parallel()

	now := dbtime.Now()

	for _, tt := range []struct {
		name                   string
		currentVersion         string
		currentAPIMajorVersion int
		provisionerDaemons     []database.ProvisionerDaemon
		provisionerDaemonsErr  error
		expectedSeverity       health.Severity
		expectedWarningCode    health.Code
		expectedError          string
		expectedItems          []healthcheck.ProvisionerDaemonsReportItem
	}{
		{
			name:             "current version empty",
			currentVersion:   "",
			expectedSeverity: health.SeverityError,
			expectedError:    "Developer error: CurrentVersion is empty",
			expectedItems:    []healthcheck.ProvisionerDaemonsReportItem{},
		},
		{
			name:                   "no daemons",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedItems:          []healthcheck.ProvisionerDaemonsReportItem{},
			expectedWarningCode:    health.CodeProvisionerDaemonsNoProvisionerDaemons,
		},
		{
			name:                   "error fetching daemons",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			provisionerDaemonsErr:  assert.AnError,
			expectedSeverity:       health.SeverityError,
			expectedError:          assert.AnError.Error(),
			expectedItems:          []healthcheck.ProvisionerDaemonsReportItem{},
		},
		{
			name:                   "one daemon up to date",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityOK,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "1.0", now)},
			expectedItems: []healthcheck.ProvisionerDaemonsReportItem{
				{
					ProvisionerDaemon: codersdk.ProvisionerDaemon{
						ID:           uuid.Nil,
						Name:         "pd-ok",
						CreatedAt:    now,
						LastSeenAt:   codersdk.NewNullTime(now, true),
						Version:      "v1.2.3",
						APIVersion:   "1.0",
						Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho, codersdk.ProvisionerTypeTerraform},
						Tags:         map[string]string{},
					},
					Warnings: []health.Message{},
				},
			},
		},
		{
			name:                   "one daemon out of date",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-old", "v1.1.2", "1.0", now)},
			expectedItems: []healthcheck.ProvisionerDaemonsReportItem{
				{
					ProvisionerDaemon: codersdk.ProvisionerDaemon{
						ID:           uuid.Nil,
						Name:         "pd-old",
						CreatedAt:    now,
						LastSeenAt:   codersdk.NewNullTime(now, true),
						Version:      "v1.1.2",
						APIVersion:   "1.0",
						Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho, codersdk.ProvisionerTypeTerraform},
						Tags:         map[string]string{},
					},
					Warnings: []health.Message{
						{
							Code:    health.CodeProvisionerDaemonVersionMismatch,
							Message: `Mismatched version "v1.1.2"`,
						},
					},
				},
			},
		},
		{
			name:                   "invalid daemon version",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedWarningCode:    health.CodeUnknown,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-invalid-version", "invalid", "1.0", now)},
			expectedItems: []healthcheck.ProvisionerDaemonsReportItem{
				{
					ProvisionerDaemon: codersdk.ProvisionerDaemon{
						ID:           uuid.Nil,
						Name:         "pd-invalid-version",
						CreatedAt:    now,
						LastSeenAt:   codersdk.NewNullTime(now, true),
						Version:      "invalid",
						APIVersion:   "1.0",
						Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho, codersdk.ProvisionerTypeTerraform},
						Tags:         map[string]string{},
					},
					Warnings: []health.Message{
						{
							Code:    health.CodeUnknown,
							Message: `Invalid version "invalid"`,
						},
					},
				},
			},
		},
		{
			name:                   "invalid daemon api version",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedWarningCode:    health.CodeUnknown,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-invalid-api", "v1.2.3", "invalid", now)},
			expectedItems: []healthcheck.ProvisionerDaemonsReportItem{
				{
					ProvisionerDaemon: codersdk.ProvisionerDaemon{
						ID:           uuid.Nil,
						Name:         "pd-invalid-api",
						CreatedAt:    now,
						LastSeenAt:   codersdk.NewNullTime(now, true),
						Version:      "v1.2.3",
						APIVersion:   "invalid",
						Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho, codersdk.ProvisionerTypeTerraform},
						Tags:         map[string]string{},
					},
					Warnings: []health.Message{
						{
							Code:    health.CodeUnknown,
							Message: `Invalid API version: invalid version string: invalid`,
						},
					},
				},
			},
		},
		{
			name:                   "api version backward compat",
			currentVersion:         "v2.3.4",
			currentAPIMajorVersion: 2,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonAPIMajorVersionDeprecated,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-old-api", "v2.3.4", "1.0", now)},
			expectedItems: []healthcheck.ProvisionerDaemonsReportItem{
				{
					ProvisionerDaemon: codersdk.ProvisionerDaemon{
						ID:           uuid.Nil,
						Name:         "pd-old-api",
						CreatedAt:    now,
						LastSeenAt:   codersdk.NewNullTime(now, true),
						Version:      "v2.3.4",
						APIVersion:   "1.0",
						Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho, codersdk.ProvisionerTypeTerraform},
						Tags:         map[string]string{},
					},
					Warnings: []health.Message{
						{
							Code:    health.CodeProvisionerDaemonAPIMajorVersionDeprecated,
							Message: "Deprecated major API version 1.",
						},
					},
				},
			},
		},
		{
			name:                   "one up to date, one out of date",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "1.0", now), fakeProvisionerDaemon(t, "pd-old", "v1.1.2", "1.0", now)},
			expectedItems: []healthcheck.ProvisionerDaemonsReportItem{
				{
					ProvisionerDaemon: codersdk.ProvisionerDaemon{
						ID:           uuid.Nil,
						Name:         "pd-ok",
						CreatedAt:    now,
						LastSeenAt:   codersdk.NewNullTime(now, true),
						Version:      "v1.2.3",
						APIVersion:   "1.0",
						Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho, codersdk.ProvisionerTypeTerraform},
						Tags:         map[string]string{},
					},
					Warnings: []health.Message{},
				},
				{
					ProvisionerDaemon: codersdk.ProvisionerDaemon{
						ID:           uuid.Nil,
						Name:         "pd-old",
						CreatedAt:    now,
						LastSeenAt:   codersdk.NewNullTime(now, true),
						Version:      "v1.1.2",
						APIVersion:   "1.0",
						Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho, codersdk.ProvisionerTypeTerraform},
						Tags:         map[string]string{},
					},
					Warnings: []health.Message{
						{
							Code:    health.CodeProvisionerDaemonVersionMismatch,
							Message: `Mismatched version "v1.1.2"`,
						},
					},
				},
			},
		},
		{
			name:                   "one up to date, one newer",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemon(t, "pd-ok", "v1.2.3", "1.0", now), fakeProvisionerDaemon(t, "pd-new", "v2.3.4", "1.0", now)},
			expectedItems: []healthcheck.ProvisionerDaemonsReportItem{
				{
					ProvisionerDaemon: codersdk.ProvisionerDaemon{
						ID:           uuid.Nil,
						Name:         "pd-new",
						CreatedAt:    now,
						LastSeenAt:   codersdk.NewNullTime(now, true),
						Version:      "v2.3.4",
						APIVersion:   "1.0",
						Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho, codersdk.ProvisionerTypeTerraform},
						Tags:         map[string]string{},
					},
					Warnings: []health.Message{
						{
							Code:    health.CodeProvisionerDaemonVersionMismatch,
							Message: `Mismatched version "v2.3.4"`,
						},
					},
				},
				{
					ProvisionerDaemon: codersdk.ProvisionerDaemon{
						ID:           uuid.Nil,
						Name:         "pd-ok",
						CreatedAt:    now,
						LastSeenAt:   codersdk.NewNullTime(now, true),
						Version:      "v1.2.3",
						APIVersion:   "1.0",
						Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho, codersdk.ProvisionerTypeTerraform},
						Tags:         map[string]string{},
					},
					Warnings: []health.Message{},
				},
			},
		},
		{
			name:                   "one up to date, one stale older",
			currentVersion:         "v2.3.4",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityOK,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemonStale(t, "pd-stale", "v1.2.3", "0.9", now.Add(-5*time.Minute), now), fakeProvisionerDaemon(t, "pd-ok", "v2.3.4", "1.0", now)},
			expectedItems: []healthcheck.ProvisionerDaemonsReportItem{
				{
					ProvisionerDaemon: codersdk.ProvisionerDaemon{
						ID:           uuid.Nil,
						Name:         "pd-ok",
						CreatedAt:    now,
						LastSeenAt:   codersdk.NewNullTime(now, true),
						Version:      "v2.3.4",
						APIVersion:   "1.0",
						Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho, codersdk.ProvisionerTypeTerraform},
						Tags:         map[string]string{},
					},
					Warnings: []health.Message{},
				},
			},
		},
		{
			name:                   "one stale",
			currentVersion:         "v2.3.4",
			currentAPIMajorVersion: provisionersdk.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedWarningCode:    health.CodeProvisionerDaemonsNoProvisionerDaemons,
			provisionerDaemons:     []database.ProvisionerDaemon{fakeProvisionerDaemonStale(t, "pd-ok", "v1.2.3", "0.9", now.Add(-5*time.Minute), now)},
			expectedItems:          []healthcheck.ProvisionerDaemonsReportItem{},
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
			if tt.expectedItems != nil {
				assert.Equal(t, tt.expectedItems, rpt.Items)
			}
		})
	}
}

func fakeProvisionerDaemon(t *testing.T, name, version, apiVersion string, now time.Time) database.ProvisionerDaemon {
	t.Helper()
	return database.ProvisionerDaemon{
		ID:           uuid.Nil,
		Name:         name,
		CreatedAt:    now,
		LastSeenAt:   sql.NullTime{Time: now, Valid: true},
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho, database.ProvisionerTypeTerraform},
		ReplicaID:    uuid.NullUUID{},
		Tags:         map[string]string{},
		Version:      version,
		APIVersion:   apiVersion,
	}
}

func fakeProvisionerDaemonStale(t *testing.T, name, version, apiVersion string, lastSeenAt, now time.Time) database.ProvisionerDaemon {
	t.Helper()
	d := fakeProvisionerDaemon(t, name, version, apiVersion, now)
	d.LastSeenAt.Valid = true
	d.LastSeenAt.Time = lastSeenAt
	return d
}
