package healthcheck

import (
	"context"
	"sync"
	"time"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

type Checker interface {
	DERP(ctx context.Context, opts *derphealth.ReportOptions) derphealth.Report
	AccessURL(ctx context.Context, opts *AccessURLReportOptions) AccessURLReport
	Websocket(ctx context.Context, opts *WebsocketReportOptions) WebsocketReport
	Database(ctx context.Context, opts *DatabaseReportOptions) DatabaseReport
	WorkspaceProxy(ctx context.Context, opts *WorkspaceProxyReportOptions) WorkspaceProxyReport
	ProvisionerDaemons(ctx context.Context, opts *ProvisionerDaemonsReportDeps) ProvisionerDaemonsReport
}

// @typescript-generate Report
type Report struct {
	// Time is the time the report was generated at.
	Time time.Time `json:"time"`
	// Healthy is true if the report returns no errors.
	// Deprecated: use `Severity` instead
	Healthy bool `json:"healthy"`
	// Severity indicates the status of Coder health.
	Severity health.Severity `json:"severity" enums:"ok,warning,error"`
	// FailingSections is a list of sections that have failed their healthcheck.
	FailingSections []codersdk.HealthSection `json:"failing_sections"`

	DERP               derphealth.Report        `json:"derp"`
	AccessURL          AccessURLReport          `json:"access_url"`
	Websocket          WebsocketReport          `json:"websocket"`
	Database           DatabaseReport           `json:"database"`
	WorkspaceProxy     WorkspaceProxyReport     `json:"workspace_proxy"`
	ProvisionerDaemons ProvisionerDaemonsReport `json:"provisioner_daemons"`

	// The Coder version of the server that the report was generated on.
	CoderVersion string `json:"coder_version"`
}

type ReportOptions struct {
	AccessURL          AccessURLReportOptions
	Database           DatabaseReportOptions
	DerpHealth         derphealth.ReportOptions
	Websocket          WebsocketReportOptions
	WorkspaceProxy     WorkspaceProxyReportOptions
	ProvisionerDaemons ProvisionerDaemonsReportDeps

	Checker Checker
}

type defaultChecker struct{}

func (defaultChecker) DERP(ctx context.Context, opts *derphealth.ReportOptions) derphealth.Report {
	var report derphealth.Report
	report.Run(ctx, opts)
	return report
}

func (defaultChecker) AccessURL(ctx context.Context, opts *AccessURLReportOptions) AccessURLReport {
	var report AccessURLReport
	report.Run(ctx, opts)
	return report
}

func (defaultChecker) Websocket(ctx context.Context, opts *WebsocketReportOptions) WebsocketReport {
	var report WebsocketReport
	report.Run(ctx, opts)
	return report
}

func (defaultChecker) Database(ctx context.Context, opts *DatabaseReportOptions) DatabaseReport {
	var report DatabaseReport
	report.Run(ctx, opts)
	return report
}

func (defaultChecker) WorkspaceProxy(ctx context.Context, opts *WorkspaceProxyReportOptions) WorkspaceProxyReport {
	var report WorkspaceProxyReport
	report.Run(ctx, opts)
	return report
}

func (defaultChecker) ProvisionerDaemons(ctx context.Context, opts *ProvisionerDaemonsReportDeps) ProvisionerDaemonsReport {
	var report ProvisionerDaemonsReport
	report.Run(ctx, opts)
	return report
}

func Run(ctx context.Context, opts *ReportOptions) *Report {
	var (
		wg     sync.WaitGroup
		report Report
	)

	if opts.Checker == nil {
		opts.Checker = defaultChecker{}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.DERP.Error = health.Errorf(health.CodeUnknown, "derp report panic: %s", err)
			}
		}()

		report.DERP = opts.Checker.DERP(ctx, &opts.DerpHealth)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.AccessURL.Error = health.Errorf(health.CodeUnknown, "access url report panic: %s", err)
			}
		}()

		report.AccessURL = opts.Checker.AccessURL(ctx, &opts.AccessURL)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.Websocket.Error = health.Errorf(health.CodeUnknown, "websocket report panic: %s", err)
			}
		}()

		report.Websocket = opts.Checker.Websocket(ctx, &opts.Websocket)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.Database.Error = health.Errorf(health.CodeUnknown, "database report panic: %s", err)
			}
		}()

		report.Database = opts.Checker.Database(ctx, &opts.Database)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.WorkspaceProxy.Error = health.Errorf(health.CodeUnknown, "proxy report panic: %s", err)
			}
		}()

		report.WorkspaceProxy = opts.Checker.WorkspaceProxy(ctx, &opts.WorkspaceProxy)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				report.ProvisionerDaemons.Error = health.Errorf(health.CodeUnknown, "provisioner daemon report panic: %s", err)
			}
		}()

		report.ProvisionerDaemons = opts.Checker.ProvisionerDaemons(ctx, &opts.ProvisionerDaemons)
	}()

	report.CoderVersion = buildinfo.Version()
	wg.Wait()

	report.Time = time.Now()
	report.FailingSections = []codersdk.HealthSection{}
	if report.DERP.Severity.Value() > health.SeverityWarning.Value() {
		report.FailingSections = append(report.FailingSections, codersdk.HealthSectionDERP)
	}
	if report.AccessURL.Severity.Value() > health.SeverityOK.Value() {
		report.FailingSections = append(report.FailingSections, codersdk.HealthSectionAccessURL)
	}
	if report.Websocket.Severity.Value() > health.SeverityWarning.Value() {
		report.FailingSections = append(report.FailingSections, codersdk.HealthSectionWebsocket)
	}
	if report.Database.Severity.Value() > health.SeverityWarning.Value() {
		report.FailingSections = append(report.FailingSections, codersdk.HealthSectionDatabase)
	}
	if report.WorkspaceProxy.Severity.Value() > health.SeverityWarning.Value() {
		report.FailingSections = append(report.FailingSections, codersdk.HealthSectionWorkspaceProxy)
	}
	if report.ProvisionerDaemons.Severity.Value() > health.SeverityWarning.Value() {
		report.FailingSections = append(report.FailingSections, codersdk.HealthSectionProvisionerDaemons)
	}

	report.Healthy = len(report.FailingSections) == 0

	// Review healthcheck sub-reports.
	report.Severity = health.SeverityOK

	if report.DERP.Severity.Value() > report.Severity.Value() {
		report.Severity = report.DERP.Severity
	}
	if report.AccessURL.Severity.Value() > report.Severity.Value() {
		report.Severity = report.AccessURL.Severity
	}
	if report.Websocket.Severity.Value() > report.Severity.Value() {
		report.Severity = report.Websocket.Severity
	}
	if report.Database.Severity.Value() > report.Severity.Value() {
		report.Severity = report.Database.Severity
	}
	if report.WorkspaceProxy.Severity.Value() > report.Severity.Value() {
		report.Severity = report.WorkspaceProxy.Severity
	}
	if report.ProvisionerDaemons.Severity.Value() > report.Severity.Value() {
		report.Severity = report.ProvisionerDaemons.Severity
	}
	return &report
}

func convertError(err error) *string {
	if err != nil {
		return ptr.Ref(err.Error())
	}

	return nil
}
