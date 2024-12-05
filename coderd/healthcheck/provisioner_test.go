package healthcheck_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerDaemonReport(t *testing.T) {
	t.Parallel()

	var (
		now            = dbtime.Now()
		oneHourAgo     = now.Add(-time.Hour)
		staleThreshold = now.Add(-provisionerdserver.StaleInterval).Add(-time.Second)
	)

	for _, tt := range []struct {
		name                   string
		currentVersion         string
		currentAPIMajorVersion int
		provisionerDaemons     []database.ProvisionerDaemon
		provisionerDaemonsErr  error
		expectedSeverity       health.Severity
		expectedWarningCode    health.Code
		expectedError          string
		expectedItems          []healthsdk.ProvisionerDaemonsReportItem
	}{
		{
			name:             "current version empty",
			currentVersion:   "",
			expectedSeverity: health.SeverityError,
			expectedError:    "Developer error: CurrentVersion is empty",
			expectedItems:    []healthsdk.ProvisionerDaemonsReportItem{},
		},
		{
			name:                   "no daemons",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: proto.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedItems:          []healthsdk.ProvisionerDaemonsReportItem{},
			expectedWarningCode:    health.CodeProvisionerDaemonsNoProvisionerDaemons,
		},
		{
			name:                   "error fetching daemons",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: proto.CurrentMajor,
			provisionerDaemonsErr:  assert.AnError,
			expectedSeverity:       health.SeverityError,
			expectedError:          assert.AnError.Error(),
			expectedItems:          []healthsdk.ProvisionerDaemonsReportItem{},
		},
		{
			name:                   "one daemon up to date",
			currentVersion:         "v1.2.3",
			currentAPIMajorVersion: proto.CurrentMajor,
			expectedSeverity:       health.SeverityOK,
			provisionerDaemons: []database.ProvisionerDaemon{
				fakeProvisionerDaemon(t, withName("pd-ok"), withVersion("v1.2.3"), withAPIVersion("1.0"), withCreatedAt(now), withLastSeenAt(now)),
			},
			expectedItems: []healthsdk.ProvisionerDaemonsReportItem{
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
			currentAPIMajorVersion: proto.CurrentMajor,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemons: []database.ProvisionerDaemon{
				fakeProvisionerDaemon(t, withName("pd-old"), withVersion("v1.1.2"), withAPIVersion("1.0"), withCreatedAt(now), withLastSeenAt(now)),
			},
			expectedItems: []healthsdk.ProvisionerDaemonsReportItem{
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
			currentAPIMajorVersion: proto.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedWarningCode:    health.CodeUnknown,
			provisionerDaemons: []database.ProvisionerDaemon{
				fakeProvisionerDaemon(t, withName("pd-invalid-version"), withVersion("invalid"), withAPIVersion("1.0"), withCreatedAt(now), withLastSeenAt(now)),
			},
			expectedItems: []healthsdk.ProvisionerDaemonsReportItem{
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
			currentAPIMajorVersion: proto.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedWarningCode:    health.CodeUnknown,
			provisionerDaemons: []database.ProvisionerDaemon{
				fakeProvisionerDaemon(t, withName("pd-invalid-api"), withVersion("v1.2.3"), withAPIVersion("invalid"), withCreatedAt(now), withLastSeenAt(now)),
			},
			expectedItems: []healthsdk.ProvisionerDaemonsReportItem{
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
			provisionerDaemons: []database.ProvisionerDaemon{
				fakeProvisionerDaemon(t, withName("pd-old-api"), withVersion("v2.3.4"), withAPIVersion("1.0"), withCreatedAt(now), withLastSeenAt(now)),
			},
			expectedItems: []healthsdk.ProvisionerDaemonsReportItem{
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
			currentAPIMajorVersion: proto.CurrentMajor,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemons: []database.ProvisionerDaemon{
				fakeProvisionerDaemon(t, withName("pd-ok"), withVersion("v1.2.3"), withAPIVersion("1.0"), withCreatedAt(now), withLastSeenAt(now)),
				fakeProvisionerDaemon(t, withName("pd-old"), withVersion("v1.1.2"), withAPIVersion("1.0"), withCreatedAt(now), withLastSeenAt(now)),
			},
			expectedItems: []healthsdk.ProvisionerDaemonsReportItem{
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
			currentAPIMajorVersion: proto.CurrentMajor,
			expectedSeverity:       health.SeverityWarning,
			expectedWarningCode:    health.CodeProvisionerDaemonVersionMismatch,
			provisionerDaemons: []database.ProvisionerDaemon{
				fakeProvisionerDaemon(t, withName("pd-ok"), withVersion("v1.2.3"), withAPIVersion("1.0"), withCreatedAt(now), withLastSeenAt(now)),
				fakeProvisionerDaemon(t, withName("pd-new"), withVersion("v2.3.4"), withAPIVersion("1.0"), withCreatedAt(now), withLastSeenAt(now)),
			},
			expectedItems: []healthsdk.ProvisionerDaemonsReportItem{
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
			currentAPIMajorVersion: proto.CurrentMajor,
			expectedSeverity:       health.SeverityOK,
			provisionerDaemons: []database.ProvisionerDaemon{
				fakeProvisionerDaemon(t, withName("pd-stale"), withVersion("v1.2.3"), withAPIVersion("0.9"), withCreatedAt(oneHourAgo), withLastSeenAt(staleThreshold)),
				fakeProvisionerDaemon(t, withName("pd-ok"), withVersion("v2.3.4"), withAPIVersion("1.0"), withCreatedAt(now), withLastSeenAt(now)),
			},
			expectedItems: []healthsdk.ProvisionerDaemonsReportItem{
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
			currentAPIMajorVersion: proto.CurrentMajor,
			expectedSeverity:       health.SeverityError,
			expectedWarningCode:    health.CodeProvisionerDaemonsNoProvisionerDaemons,
			provisionerDaemons: []database.ProvisionerDaemon{
				fakeProvisionerDaemon(t, withName("pd-stale"), withVersion("v1.2.3"), withAPIVersion("0.9"), withCreatedAt(oneHourAgo), withLastSeenAt(staleThreshold)),
			},
			expectedItems: []healthsdk.ProvisionerDaemonsReportItem{},
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
				deps.CurrentAPIMajorVersion = proto.CurrentMajor
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

func withName(s string) func(*database.ProvisionerDaemon) {
	return func(pd *database.ProvisionerDaemon) {
		pd.Name = s
	}
}

func withCreatedAt(at time.Time) func(*database.ProvisionerDaemon) {
	return func(pd *database.ProvisionerDaemon) {
		pd.CreatedAt = at
	}
}

func withLastSeenAt(at time.Time) func(*database.ProvisionerDaemon) {
	return func(pd *database.ProvisionerDaemon) {
		pd.LastSeenAt.Valid = true
		pd.LastSeenAt.Time = at
	}
}

func withVersion(v string) func(*database.ProvisionerDaemon) {
	return func(pd *database.ProvisionerDaemon) {
		pd.Version = v
	}
}

func withAPIVersion(v string) func(*database.ProvisionerDaemon) {
	return func(pd *database.ProvisionerDaemon) {
		pd.APIVersion = v
	}
}

func fakeProvisionerDaemon(t *testing.T, opts ...func(*database.ProvisionerDaemon)) database.ProvisionerDaemon {
	t.Helper()
	pd := database.ProvisionerDaemon{
		ID:           uuid.Nil,
		Name:         testutil.GetRandomName(t),
		CreatedAt:    time.Time{},
		LastSeenAt:   sql.NullTime{},
		Provisioners: []database.ProvisionerType{database.ProvisionerTypeEcho, database.ProvisionerTypeTerraform},
		ReplicaID:    uuid.NullUUID{},
		Tags:         map[string]string{},
		Version:      "",
		APIVersion:   "",
	}
	for _, o := range opts {
		o(&pd)
	}
	return pd
}
