package healthcheck

import (
	"context"
	"sync"
	"time"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk/healthsdk"
)

type Checker interface {
	DERP(ctx context.Context, opts *derphealth.ReportOptions) healthsdk.DERPHealthReport
	AccessURL(ctx context.Context, opts *AccessURLReportOptions) healthsdk.AccessURLReport
	Websocket(ctx context.Context, opts *WebsocketReportOptions) healthsdk.WebsocketReport
	Database(ctx context.Context, opts *DatabaseReportOptions) healthsdk.DatabaseReport
	WorkspaceProxy(ctx context.Context, opts *WorkspaceProxyReportOptions) healthsdk.WorkspaceProxyReport
	ProvisionerDaemons(ctx context.Context, opts *ProvisionerDaemonsReportDeps) healthsdk.ProvisionerDaemonsReport
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

func (defaultChecker) DERP(ctx context.Context, opts *derphealth.ReportOptions) healthsdk.DERPHealthReport {
	var report derphealth.Report
	report.Run(ctx, opts)
	return healthsdk.DERPHealthReport(report)
}

func (defaultChecker) AccessURL(ctx context.Context, opts *AccessURLReportOptions) healthsdk.AccessURLReport {
	var report AccessURLReport
	report.Run(ctx, opts)
	return healthsdk.AccessURLReport(report)
}

func (defaultChecker) Websocket(ctx context.Context, opts *WebsocketReportOptions) healthsdk.WebsocketReport {
	var report WebsocketReport
	report.Run(ctx, opts)
	return healthsdk.WebsocketReport(report)
}

func (defaultChecker) Database(ctx context.Context, opts *DatabaseReportOptions) healthsdk.DatabaseReport {
	var report DatabaseReport
	report.Run(ctx, opts)
	return healthsdk.DatabaseReport(report)
}

func (defaultChecker) WorkspaceProxy(ctx context.Context, opts *WorkspaceProxyReportOptions) healthsdk.WorkspaceProxyReport {
	var report WorkspaceProxyReport
	report.Run(ctx, opts)
	return healthsdk.WorkspaceProxyReport(report)
}

func (defaultChecker) ProvisionerDaemons(ctx context.Context, opts *ProvisionerDaemonsReportDeps) healthsdk.ProvisionerDaemonsReport {
	var report ProvisionerDaemonsReport
	report.Run(ctx, opts)
	return healthsdk.ProvisionerDaemonsReport(report)
}

func Run(ctx context.Context, opts *ReportOptions) *healthsdk.HealthcheckReport {
	var (
		wg     sync.WaitGroup
		report healthsdk.HealthcheckReport
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
	report.FailingSections = []healthsdk.HealthSection{}
	if report.DERP.Severity.Value() > health.SeverityWarning.Value() {
		report.FailingSections = append(report.FailingSections, healthsdk.HealthSectionDERP)
	}
	if report.AccessURL.Severity.Value() > health.SeverityOK.Value() {
		report.FailingSections = append(report.FailingSections, healthsdk.HealthSectionAccessURL)
	}
	if report.Websocket.Severity.Value() > health.SeverityWarning.Value() {
		report.FailingSections = append(report.FailingSections, healthsdk.HealthSectionWebsocket)
	}
	if report.Database.Severity.Value() > health.SeverityWarning.Value() {
		report.FailingSections = append(report.FailingSections, healthsdk.HealthSectionDatabase)
	}
	if report.WorkspaceProxy.Severity.Value() > health.SeverityWarning.Value() {
		report.FailingSections = append(report.FailingSections, healthsdk.HealthSectionWorkspaceProxy)
	}
	if report.ProvisionerDaemons.Severity.Value() > health.SeverityWarning.Value() {
		report.FailingSections = append(report.FailingSections, healthsdk.HealthSectionProvisionerDaemons)
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
