package healthcheck_test

import (
	"context"
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
			expectedError:     "connect: connection refused",
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
			expectedError:         assert.AnError.Error(),
		},
		{
			name:                  "Enabled/ErrUpdateProxyHealth",
			fetchWorkspaceProxies: fakeFetchWorkspaceProxies(fakeWorkspaceProxy("alpha", true, currentVersion)),
			updateProxyHealth:     fakeUpdateProxyHealth(assert.AnError),
			expectedHealthy:       true,
			expectedSeverity:      health.SeverityWarning,
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
			if tt.expectedError != "" {
				assert.NotNil(t, rpt.Error)
				assert.Contains(t, *rpt.Error, tt.expectedError)
			} else {
				if !assert.Nil(t, rpt.Error) {
					assert.Empty(t, *rpt.Error)
				}
			}
		})
	}
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

func fakeWorkspaceProxy(name string, healthy bool, version string) codersdk.WorkspaceProxy {
	return codersdk.WorkspaceProxy{
		Region: codersdk.Region{
			Name:    name,
			Healthy: healthy,
		},
		Version: version,
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
