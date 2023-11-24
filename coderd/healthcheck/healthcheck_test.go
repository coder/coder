package healthcheck_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
)

type testChecker struct {
	DERPReport      derphealth.Report
	AccessURLReport healthcheck.AccessURLReport
	WebsocketReport healthcheck.WebsocketReport
	DatabaseReport  healthcheck.DatabaseReport
}

func (c *testChecker) DERP(context.Context, *derphealth.ReportOptions) derphealth.Report {
	return c.DERPReport
}

func (c *testChecker) AccessURL(context.Context, *healthcheck.AccessURLReportOptions) healthcheck.AccessURLReport {
	return c.AccessURLReport
}

func (c *testChecker) Websocket(context.Context, *healthcheck.WebsocketReportOptions) healthcheck.WebsocketReport {
	return c.WebsocketReport
}

func (c *testChecker) Database(context.Context, *healthcheck.DatabaseReportOptions) healthcheck.DatabaseReport {
	return c.DatabaseReport
}

func TestHealthcheck(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name            string
		checker         *testChecker
		healthy         bool
		severity        health.Severity
		failingSections []string
	}{{
		name: "OK",
		checker: &testChecker{
			DERPReport: derphealth.Report{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			DatabaseReport: healthcheck.DatabaseReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
		},
		healthy:         true,
		severity:        health.SeverityOK,
		failingSections: []string{},
	}, {
		name: "DERPFail",
		checker: &testChecker{
			DERPReport: derphealth.Report{
				Healthy:  false,
				Severity: health.SeverityError,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			DatabaseReport: healthcheck.DatabaseReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
		},
		healthy:         false,
		severity:        health.SeverityError,
		failingSections: []string{healthcheck.SectionDERP},
	}, {
		name: "DERPWarning",
		checker: &testChecker{
			DERPReport: derphealth.Report{
				Healthy:  true,
				Warnings: []string{"foobar"},
				Severity: health.SeverityWarning,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			DatabaseReport: healthcheck.DatabaseReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
		},
		healthy:         true,
		severity:        health.SeverityWarning,
		failingSections: []string{},
	}, {
		name: "AccessURLFail",
		checker: &testChecker{
			DERPReport: derphealth.Report{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy:  false,
				Severity: health.SeverityWarning,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			DatabaseReport: healthcheck.DatabaseReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
		},
		healthy:         false,
		severity:        health.SeverityWarning,
		failingSections: []string{healthcheck.SectionAccessURL},
	}, {
		name: "WebsocketFail",
		checker: &testChecker{
			DERPReport: derphealth.Report{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy:  false,
				Severity: health.SeverityError,
			},
			DatabaseReport: healthcheck.DatabaseReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
		},
		healthy:         false,
		severity:        health.SeverityError,
		failingSections: []string{healthcheck.SectionWebsocket},
	}, {
		name: "DatabaseFail",
		checker: &testChecker{
			DERPReport: derphealth.Report{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy:  true,
				Severity: health.SeverityOK,
			},
			DatabaseReport: healthcheck.DatabaseReport{
				Healthy:  false,
				Severity: health.SeverityError,
			},
		},
		healthy:         false,
		severity:        health.SeverityError,
		failingSections: []string{healthcheck.SectionDatabase},
	}, {
		name: "AllFail",
		checker: &testChecker{
			DERPReport: derphealth.Report{
				Healthy:  false,
				Severity: health.SeverityError,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy:  false,
				Severity: health.SeverityError,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy:  false,
				Severity: health.SeverityError,
			},
			DatabaseReport: healthcheck.DatabaseReport{
				Healthy:  false,
				Severity: health.SeverityError,
			},
		},
		healthy:  false,
		severity: health.SeverityError,
		failingSections: []string{
			healthcheck.SectionDERP,
			healthcheck.SectionAccessURL,
			healthcheck.SectionWebsocket,
			healthcheck.SectionDatabase,
		},
	}} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			report := healthcheck.Run(context.Background(), &healthcheck.ReportOptions{
				Checker: c.checker,
			})

			assert.Equal(t, c.healthy, report.Healthy)
			assert.Equal(t, c.severity, report.Severity)
			assert.Equal(t, c.failingSections, report.FailingSections)
			assert.Equal(t, c.checker.DERPReport.Healthy, report.DERP.Healthy)
			assert.Equal(t, c.checker.DERPReport.Severity, report.DERP.Severity)
			assert.Equal(t, c.checker.DERPReport.Warnings, report.DERP.Warnings)
			assert.Equal(t, c.checker.AccessURLReport.Healthy, report.AccessURL.Healthy)
			assert.Equal(t, c.checker.AccessURLReport.Severity, report.AccessURL.Severity)
			assert.Equal(t, c.checker.WebsocketReport.Healthy, report.Websocket.Healthy)
			assert.Equal(t, c.checker.WebsocketReport.Severity, report.Websocket.Severity)
			assert.Equal(t, c.checker.DatabaseReport.Healthy, report.Database.Healthy)
			assert.Equal(t, c.checker.DatabaseReport.Severity, report.Database.Severity)
			assert.NotZero(t, report.Time)
			assert.NotZero(t, report.CoderVersion)
		})
	}
}
