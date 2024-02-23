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
	DERP(ctx context.Context, opts *derphealth.ReportOptions) codersdk.DERPHealthReport
	AccessURL(ctx context.Context, opts *AccessURLReportOptions) codersdk.AccessURLReport
	Websocket(ctx context.Context, opts *WebsocketReportOptions) codersdk.WebsocketReport
	Database(ctx context.Context, opts *DatabaseReportOptions) codersdk.DatabaseReport
	WorkspaceProxy(ctx context.Context, opts *WorkspaceProxyReportOptions) codersdk.WorkspaceProxyReport
	ProvisionerDaemons(ctx context.Context, opts *ProvisionerDaemonsReportDeps) codersdk.ProvisionerDaemonsReport
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

func (defaultChecker) DERP(ctx context.Context, opts *derphealth.ReportOptions) codersdk.DERPHealthReport {
	var report derphealth.Report
	report.Run(ctx, opts)
	return codersdk.DERPHealthReport(report)
}

func (defaultChecker) AccessURL(ctx context.Context, opts *AccessURLReportOptions) codersdk.AccessURLReport {
	var report AccessURLReport
	report.Run(ctx, opts)
	return codersdk.AccessURLReport(report)
}

func (defaultChecker) Websocket(ctx context.Context, opts *WebsocketReportOptions) codersdk.WebsocketReport {
	var report WebsocketReport
	report.Run(ctx, opts)
	return codersdk.WebsocketReport(report)
}

func (defaultChecker) Database(ctx context.Context, opts *DatabaseReportOptions) codersdk.DatabaseReport {
	var report DatabaseReport
	report.Run(ctx, opts)
	return codersdk.DatabaseReport(report)
}

func (defaultChecker) WorkspaceProxy(ctx context.Context, opts *WorkspaceProxyReportOptions) codersdk.WorkspaceProxyReport {
	var report WorkspaceProxyReport
	report.Run(ctx, opts)
	return codersdk.WorkspaceProxyReport(report)
}

func (defaultChecker) ProvisionerDaemons(ctx context.Context, opts *ProvisionerDaemonsReportDeps) codersdk.ProvisionerDaemonsReport {
	var report ProvisionerDaemonsReport
	report.Run(ctx, opts)
	return codersdk.ProvisionerDaemonsReport(report)
}

func Run(ctx context.Context, opts *ReportOptions) *codersdk.HealthcheckReport {
	var (
		wg     sync.WaitGroup
		report codersdk.HealthcheckReport
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
