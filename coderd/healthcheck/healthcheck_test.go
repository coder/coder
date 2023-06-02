package healthcheck_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/coderd/healthcheck"
)

type testChecker struct {
	DERPReport      healthcheck.DERPReport
	AccessURLReport healthcheck.AccessURLReport
	WebsocketReport healthcheck.WebsocketReport
}

func (c *testChecker) DERP(context.Context, *healthcheck.DERPReportOptions) healthcheck.DERPReport {
	return c.DERPReport
}

func (c *testChecker) AccessURL(context.Context, *healthcheck.AccessURLOptions) healthcheck.AccessURLReport {
	return c.AccessURLReport
}

func (c *testChecker) Websocket(context.Context, *healthcheck.WebsocketReportOptions) healthcheck.WebsocketReport {
	return c.WebsocketReport
}

func TestHealthcheck(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name            string
		checker         *testChecker
		healthy         bool
		failingSections []string
	}{{
		name: "OK",
		checker: &testChecker{
			DERPReport: healthcheck.DERPReport{
				Healthy: true,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy: true,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy: true,
			},
		},
		healthy:         true,
		failingSections: nil,
	}, {
		name: "DERPFail",
		checker: &testChecker{
			DERPReport: healthcheck.DERPReport{
				Healthy: false,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy: true,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy: true,
			},
		},
		healthy:         false,
		failingSections: []string{healthcheck.SectionDERP},
	}, {
		name: "AccessURLFail",
		checker: &testChecker{
			DERPReport: healthcheck.DERPReport{
				Healthy: true,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy: false,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy: true,
			},
		},
		healthy:         false,
		failingSections: []string{healthcheck.SectionAccessURL},
	}, {
		name: "WebsocketFail",
		checker: &testChecker{
			DERPReport: healthcheck.DERPReport{
				Healthy: true,
			},
			AccessURLReport: healthcheck.AccessURLReport{
				Healthy: true,
			},
			WebsocketReport: healthcheck.WebsocketReport{
				Healthy: false,
			},
		},
		healthy:         false,
		failingSections: []string{healthcheck.SectionWebsocket},
	}, {
		name:            "AllFail",
		checker:         &testChecker{},
		healthy:         false,
		failingSections: []string{healthcheck.SectionDERP, healthcheck.SectionAccessURL, healthcheck.SectionWebsocket},
	}} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			report := healthcheck.Run(context.Background(), &healthcheck.ReportOptions{
				Checker: c.checker,
			})

			assert.Equal(t, c.healthy, report.Healthy)
			assert.Equal(t, c.failingSections, report.FailingSections)
			assert.Equal(t, c.checker.DERPReport.Healthy, report.DERP.Healthy)
			assert.Equal(t, c.checker.AccessURLReport.Healthy, report.AccessURL.Healthy)
			assert.Equal(t, c.checker.WebsocketReport.Healthy, report.Websocket.Healthy)
			assert.NotZero(t, report.Time)
		})
	}
}
