package healthcheck_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/quartz"
)

type testChecker struct {
	DERPReport               healthsdk.DERPHealthReport
	AccessURLReport          healthsdk.AccessURLReport
	WebsocketReport          healthsdk.WebsocketReport
	DatabaseReport           healthsdk.DatabaseReport
	WorkspaceProxyReport     healthsdk.WorkspaceProxyReport
	ProvisionerDaemonsReport healthsdk.ProvisionerDaemonsReport
}

func (c *testChecker) DERP(context.Context, *derphealth.ReportOptions) healthsdk.DERPHealthReport {
	return c.DERPReport
}

func (c *testChecker) AccessURL(context.Context, *healthcheck.AccessURLReportOptions) healthsdk.AccessURLReport {
	return c.AccessURLReport
}

func (c *testChecker) Websocket(context.Context, *healthcheck.WebsocketReportOptions) healthsdk.WebsocketReport {
	return c.WebsocketReport
}

func (c *testChecker) Database(context.Context, *healthcheck.DatabaseReportOptions) healthsdk.DatabaseReport {
	return c.DatabaseReport
}

func (c *testChecker) WorkspaceProxy(context.Context, *healthcheck.WorkspaceProxyReportOptions) healthsdk.WorkspaceProxyReport {
	return c.WorkspaceProxyReport
}

func (c *testChecker) ProvisionerDaemons(context.Context, *healthcheck.ProvisionerDaemonsReportDeps) healthsdk.ProvisionerDaemonsReport {
	return c.ProvisionerDaemonsReport
}

func TestHealthcheck(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name     string
		checker  *testChecker
		healthy  bool
		severity health.Severity
	}{{
		name: "OK",
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
		},
		healthy:  true,
		severity: health.SeverityOK,
	}, {
		name: "DERPFail",
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: false,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
		},
		healthy:  false,
		severity: health.SeverityError,
	}, {
		name: "DERPWarning",
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Warnings: []health.Message{{Message: "foobar", Code: "EFOOBAR"}},
					Severity: health.SeverityWarning,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
		},
		healthy:  true,
		severity: health.SeverityWarning,
	}, {
		name: "AccessURLFail",
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: false,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityWarning,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
		},
		healthy:  false,
		severity: health.SeverityWarning,
	}, {
		name: "WebsocketFail",
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: false,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
		},
		healthy:  false,
		severity: health.SeverityError,
	}, {
		name: "DatabaseFail",
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: false,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
		},
		healthy:  false,
		severity: health.SeverityError,
	}, {
		name: "ProxyFail",
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: false,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
		},
		severity: health.SeverityError,
		healthy:  false,
	}, {
		name: "ProxyWarn",
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Warnings: []health.Message{{Message: "foobar", Code: "EFOOBAR"}},
					Severity: health.SeverityWarning,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
		},
		severity: health.SeverityWarning,
		healthy:  true,
	}, {
		name: "ProvisionerDaemonsFail",
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
		},
		severity: health.SeverityError,
		healthy:  false,
	}, {
		name: "ProvisionerDaemonsWarn",
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: true,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityOK,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityWarning,
					Warnings: []health.Message{{Message: "foobar", Code: "EFOOBAR"}},
				},
			},
		},
		severity: health.SeverityWarning,
		healthy:  true,
	}, {
		name:    "AllFail",
		healthy: false,
		checker: &testChecker{
			DERPReport: healthsdk.DERPHealthReport{
				Healthy: false,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
			AccessURLReport: healthsdk.AccessURLReport{
				Healthy: false,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
			WebsocketReport: healthsdk.WebsocketReport{
				Healthy: false,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
			DatabaseReport: healthsdk.DatabaseReport{
				Healthy: false,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
			WorkspaceProxyReport: healthsdk.WorkspaceProxyReport{
				Healthy: false,
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
			ProvisionerDaemonsReport: healthsdk.ProvisionerDaemonsReport{
				BaseReport: healthsdk.BaseReport{
					Severity: health.SeverityError,
				},
			},
		},
		severity: health.SeverityError,
	}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			report := healthcheck.Run(context.Background(), &healthcheck.ReportOptions{
				Checker: c.checker,
			})

			assert.Equal(t, c.healthy, report.Healthy)
			assert.Equal(t, c.severity, report.Severity)
			assert.Equal(t, c.checker.DERPReport.Healthy, report.DERP.Healthy)
			assert.Equal(t, c.checker.DERPReport.Severity, report.DERP.Severity)
			assert.Equal(t, c.checker.DERPReport.Warnings, report.DERP.Warnings)
			assert.Equal(t, c.checker.AccessURLReport.Healthy, report.AccessURL.Healthy)
			assert.Equal(t, c.checker.AccessURLReport.Severity, report.AccessURL.Severity)
			assert.Equal(t, c.checker.WebsocketReport.Healthy, report.Websocket.Healthy)
			assert.Equal(t, c.checker.WorkspaceProxyReport.Healthy, report.WorkspaceProxy.Healthy)
			assert.Equal(t, c.checker.WorkspaceProxyReport.Warnings, report.WorkspaceProxy.Warnings)
			assert.Equal(t, c.checker.WebsocketReport.Severity, report.Websocket.Severity)
			assert.Equal(t, c.checker.DatabaseReport.Healthy, report.Database.Healthy)
			assert.Equal(t, c.checker.DatabaseReport.Severity, report.Database.Severity)
			assert.NotZero(t, report.Time)
			assert.NotZero(t, report.CoderVersion)
		})
	}
}

func TestCheckProgress(t *testing.T) {
	t.Parallel()

	t.Run("Summary", func(t *testing.T) {
		t.Parallel()

		mClock := quartz.NewMock(t)
		progress := healthcheck.Progress{Clock: mClock}

		// Start some checks
		progress.Start("Database")
		progress.Start("DERP")
		progress.Start("AccessURL")

		// Advance time to simulate check duration
		mClock.Advance(100 * time.Millisecond)

		// Complete some checks
		progress.Complete("Database")
		progress.Complete("AccessURL")

		summary := progress.Summary()

		// Verify completed and running checks are listed with duration / elapsed
		assert.Equal(t, summary, "Completed: AccessURL (100ms), Database (100ms). Still running: DERP (elapsed: 100ms)")
	})

	t.Run("EmptyProgress", func(t *testing.T) {
		t.Parallel()

		mClock := quartz.NewMock(t)
		progress := healthcheck.Progress{Clock: mClock}
		summary := progress.Summary()

		// Should be empty string when nothing tracked
		assert.Empty(t, summary)
	})

	t.Run("AllCompleted", func(t *testing.T) {
		t.Parallel()

		mClock := quartz.NewMock(t)
		progress := healthcheck.Progress{Clock: mClock}
		progress.Start("Database")
		progress.Start("DERP")
		mClock.Advance(50 * time.Millisecond)
		progress.Complete("Database")
		progress.Complete("DERP")

		summary := progress.Summary()
		assert.Equal(t, summary, "Completed: DERP (50ms), Database (50ms)")
	})

	t.Run("AllRunning", func(t *testing.T) {
		t.Parallel()

		mClock := quartz.NewMock(t)
		progress := healthcheck.Progress{Clock: mClock}
		progress.Start("Database")
		progress.Start("DERP")

		summary := progress.Summary()
		assert.Equal(t, summary, "Still running: DERP (elapsed: 0ms), Database (elapsed: 0ms)")
	})
}
