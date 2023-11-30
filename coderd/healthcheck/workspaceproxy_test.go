package healthcheck_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk"
)

func TestWorkspaceProxies(t *testing.T) {
	t.Parallel()

	var (
		newerPatchVersion = "v2.34.6"
		currentVersion    = "v2.34.5"
		olderVersion      = "v2.33.0"
	)

	for _, tt := range []struct {
		name                  string
		fetchWorkspaceProxies func(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error)
		updateProxyHealth     func(context.Context) error
		expectedHealthy       bool
		expectedError         string
		expectedWarning       string
		expectedSeverity      health.Severity
	}{
		{
			name:             "NotEnabled",
			expectedHealthy:  true,
			expectedSeverity: health.SeverityOK,
		},
		{
			name:                  "Enabled/NoProxies",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(),
			updateProxyHealth:     fakeUpdateProxyHealth(nil),
			expectedHealthy:       true,
			expectedSeverity:      health.SeverityOK,
		},
		{
			name:                  "Enabled/OneHealthy",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(fakeWorkspaceProxy("alpha", true, currentVersion)),
			updateProxyHealth:     fakeUpdateProxyHealth(nil),
			expectedHealthy:       true,
			expectedSeverity:      health.SeverityOK,
		},
		{
			name:                  "Enabled/OneUnhealthy",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(fakeWorkspaceProxy("alpha", false, currentVersion)),
			updateProxyHealth:     fakeUpdateProxyHealth(nil),
			expectedHealthy:       false,
			expectedSeverity:      health.SeverityError,
			expectedError:         string(health.CodeProxyUnhealthy),
		},
		{
			name: "Enabled/OneUnreachable",
			fetchWorkspaceProxies: func(ctx context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
				return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{
					Regions: []codersdk.WorkspaceProxy{
						{
							Region: codersdk.Region{
								Name:    "gone",
								Healthy: false,
							},
							Version: currentVersion,
							Status: codersdk.WorkspaceProxyStatus{
								Status: codersdk.ProxyUnreachable,
								Report: codersdk.ProxyHealthReport{
									Errors: []string{
										"request to proxy failed: Get \"http://127.0.0.1:3001/healthz-report\": dial tcp 127.0.0.1:3001: connect: connection refused",
									},
								},
							},
						},
					},
				}, nil
			},
			updateProxyHealth: fakeUpdateProxyHealth(nil),
			expectedHealthy:   false,
			expectedSeverity:  health.SeverityError,
			expectedError:     string(health.CodeProxyUnhealthy),
		},
		{
			name: "Enabled/AllHealthy",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(
				fakeWorkspaceProxy("alpha", true, currentVersion),
				fakeWorkspaceProxy("beta", true, currentVersion),
			),
			updateProxyHealth: func(ctx context.Context) error {
				return nil
			},
			expectedHealthy:  true,
			expectedSeverity: health.SeverityOK,
		},
		{
			name: "Enabled/OneHealthyOneUnhealthy",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(
				fakeWorkspaceProxy("alpha", false, currentVersion),
				fakeWorkspaceProxy("beta", true, currentVersion),
			),
			updateProxyHealth: fakeUpdateProxyHealth(nil),
			expectedHealthy:   true,
			expectedSeverity:  health.SeverityWarning,
			expectedWarning:   string(health.CodeProxyUnhealthy),
		},
		{
			name: "Enabled/AllUnhealthy",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(
				fakeWorkspaceProxy("alpha", false, currentVersion),
				fakeWorkspaceProxy("beta", false, currentVersion),
			),
			updateProxyHealth: fakeUpdateProxyHealth(nil),
			expectedHealthy:   false,
			expectedSeverity:  health.SeverityError,
			expectedError:     string(health.CodeProxyUnhealthy),
		},
		{
			name: "Enabled/OneOutOfDate",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(
				fakeWorkspaceProxy("alpha", true, currentVersion),
				fakeWorkspaceProxy("beta", true, olderVersion),
			),
			updateProxyHealth: fakeUpdateProxyHealth(nil),
			expectedHealthy:   false,
			expectedSeverity:  health.SeverityError,
			expectedError:     `proxy "beta" version "v2.33.0" does not match primary server version "v2.34.5"`,
		},
		{
			name: "Enabled/OneSlightlyNewerButStillOK",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(
				fakeWorkspaceProxy("alpha", true, currentVersion),
				fakeWorkspaceProxy("beta", true, newerPatchVersion),
			),
			updateProxyHealth: fakeUpdateProxyHealth(nil),
			expectedHealthy:   true,
			expectedSeverity:  health.SeverityOK,
		},
		{
			name: "Enabled/NotConnectedYet",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(
				fakeWorkspaceProxy("slowpoke", true, ""),
			),
			updateProxyHealth: fakeUpdateProxyHealth(nil),
			expectedHealthy:   true,
			expectedSeverity:  health.SeverityOK,
		},
		{
			name:                  "Enabled/ErrFetchWorkspaceProxy",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxiesErr(assert.AnError),
			updateProxyHealth:     fakeUpdateProxyHealth(nil),
			expectedHealthy:       false,
			expectedSeverity:      health.SeverityError,
			expectedError:         string(health.CodeProxyFetch),
		},
		{
			name:                  "Enabled/ErrUpdateProxyHealth",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(fakeWorkspaceProxy("alpha", true, currentVersion)),
			updateProxyHealth:     fakeUpdateProxyHealth(assert.AnError),
			expectedHealthy:       true,
			expectedSeverity:      health.SeverityWarning,
			expectedWarning:       string(health.CodeProxyUpdate),
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var rpt healthcheck.WorkspaceProxyReport
			var opts healthcheck.WorkspaceProxyReportOptions
			opts.CurrentVersion = currentVersion
			if tt.fetchWorkspaceProxies != nil && tt.updateProxyHealth != nil {
				opts.WorkspaceProxiesFetchUpdater = &fakeWorkspaceProxyFetchUpdater{
					fetchFunc:  tt.fetchWorkspaceProxies,
					updateFunc: tt.updateProxyHealth,
				}
			} else {
				opts.WorkspaceProxiesFetchUpdater = &healthcheck.AGPLWorkspaceProxiesFetchUpdater{}
			}

			rpt.Run(context.Background(), &opts)

			assert.Equal(t, tt.expectedHealthy, rpt.Healthy)
			assert.Equal(t, tt.expectedSeverity, rpt.Severity)
			if tt.expectedError != "" && assert.NotNil(t, rpt.Error) {
				assert.Contains(t, *rpt.Error, tt.expectedError)
			} else {
				assert.Nil(t, rpt.Error)
			}
			if tt.expectedWarning != "" && assert.NotEmpty(t, rpt.Warnings) {
				var found bool
				for _, w := range rpt.Warnings {
					if strings.Contains(w, tt.expectedWarning) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected warning %s not found in %v", tt.expectedWarning, rpt.Warnings)
			} else {
				assert.Empty(t, rpt.Warnings)
			}
		})
	}
}

func TestWorkspaceProxy_ErrorDismissed(t *testing.T) {
	t.Parallel()

	var report healthcheck.WorkspaceProxyReport
	report.Run(context.Background(), &healthcheck.WorkspaceProxyReportOptions{
		WorkspaceProxiesFetchUpdater: &fakeWorkspaceProxyFetchUpdater{
			fetchFunc:  fakeFetchWorkspaceProxiesErr(assert.AnError),
			updateFunc: fakeUpdateProxyHealth(assert.AnError),
		},
		Dismissed: true,
	})

	assert.True(t, report.Dismissed)
	assert.Equal(t, health.SeverityWarning, report.Severity)
}

// yet another implementation of the thing
type fakeWorkspaceProxyFetchUpdater struct {
	fetchFunc  func(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error)
	updateFunc func(context.Context) error
}

func (u *fakeWorkspaceProxyFetchUpdater) Fetch(ctx context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
	return u.fetchFunc(ctx)
}

func (u *fakeWorkspaceProxyFetchUpdater) Update(ctx context.Context) error {
	return u.updateFunc(ctx)
}

//nolint:revive // yes, this is a control flag, and that is OK in a unit test.
func fakeWorkspaceProxy(name string, healthy bool, version string) codersdk.WorkspaceProxy {
	var status codersdk.WorkspaceProxyStatus
	if !healthy {
		status = codersdk.WorkspaceProxyStatus{
			Status: codersdk.ProxyUnreachable,
			Report: codersdk.ProxyHealthReport{
				Errors: []string{assert.AnError.Error()},
			},
		}
	}
	return codersdk.WorkspaceProxy{
		Region: codersdk.Region{
			Name:    name,
			Healthy: healthy,
		},
		Version: version,
		Status:  status,
	}
}

func fakeFetchWorkspaceProxies(ps ...codersdk.WorkspaceProxy) func(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
	return func(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
		return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{
			Regions: ps,
		}, nil
	}
}

func fakeFetchWorkspaceProxiesErr(err error) func(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
	return func(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
		return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{
			Regions: []codersdk.WorkspaceProxy{},
		}, err
	}
}

func fakeUpdateProxyHealth(err error) func(context.Context) error {
	return func(context.Context) error {
		return err
	}
}
