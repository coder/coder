package healthcheck

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/healthcheck/derphealth"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/quartz"
)

// CheckProgress tracks the progress of healthcheck components for timeout
// diagnostics. It records which checks have started and completed, along with
// their durations, to provide useful information when a healthcheck times out.
type CheckProgress struct {
	clock  quartz.Clock
	mu     sync.Mutex
	checks map[string]*checkStatus
}

type checkStatus struct {
	started   time.Time
	completed time.Time
	done      bool
}

// NewCheckProgress creates a new CheckProgress tracker using the real clock.
func NewCheckProgress() *CheckProgress {
	return NewCheckProgressWithClock(quartz.NewReal())
}

// NewCheckProgressWithClock creates a new CheckProgress tracker with a custom
// clock for testing.
func NewCheckProgressWithClock(clock quartz.Clock) *CheckProgress {
	return &CheckProgress{
		clock:  clock,
		checks: make(map[string]*checkStatus),
	}
}

// Start records that a check has started.
func (p *CheckProgress) Start(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.checks[name] = &checkStatus{started: p.clock.Now()}
}

// Complete records that a check has finished.
func (p *CheckProgress) Complete(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if status, ok := p.checks[name]; ok {
		status.completed = p.clock.Now()
		status.done = true
	}
}

// Summary returns a human-readable summary of check progress.
// Example: "Completed: AccessURL (95ms), Database (120ms). Still running: DERP, Websocket"
func (p *CheckProgress) Summary() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	var completed, running []string
	for name, status := range p.checks {
		if status.done {
			duration := status.completed.Sub(status.started).Round(time.Millisecond)
			completed = append(completed, fmt.Sprintf("%s (%s)", name, duration))
		} else {
			running = append(running, name)
		}
	}

	// Sort for consistent output.
	slices.Sort(completed)
	slices.Sort(running)

	var parts []string
	if len(completed) > 0 {
		parts = append(parts, "Completed: "+strings.Join(completed, ", "))
	}
	if len(running) > 0 {
		parts = append(parts, "Still running: "+strings.Join(running, ", "))
	}
	return strings.Join(parts, ". ")
}

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

	// Progress optionally tracks healthcheck progress for timeout diagnostics.
	// If set, each check will record its start and completion time.
	Progress *CheckProgress
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

		if opts.Progress != nil {
			opts.Progress.Start("DERP")
			defer opts.Progress.Complete("DERP")
		}
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

		if opts.Progress != nil {
			opts.Progress.Start("AccessURL")
			defer opts.Progress.Complete("AccessURL")
		}
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

		if opts.Progress != nil {
			opts.Progress.Start("Websocket")
			defer opts.Progress.Complete("Websocket")
		}
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

		if opts.Progress != nil {
			opts.Progress.Start("Database")
			defer opts.Progress.Complete("Database")
		}
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

		if opts.Progress != nil {
			opts.Progress.Start("WorkspaceProxy")
			defer opts.Progress.Complete("WorkspaceProxy")
		}
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

		if opts.Progress != nil {
			opts.Progress.Start("ProvisionerDaemons")
			defer opts.Progress.Complete("ProvisionerDaemons")
		}
		report.ProvisionerDaemons = opts.Checker.ProvisionerDaemons(ctx, &opts.ProvisionerDaemons)
	}()

	report.CoderVersion = buildinfo.Version()
	wg.Wait()

	report.Time = time.Now()
	failingSections := []healthsdk.HealthSection{}
	if report.DERP.Severity.Value() > health.SeverityWarning.Value() {
		failingSections = append(failingSections, healthsdk.HealthSectionDERP)
	}
	if report.AccessURL.Severity.Value() > health.SeverityOK.Value() {
		failingSections = append(failingSections, healthsdk.HealthSectionAccessURL)
	}
	if report.Websocket.Severity.Value() > health.SeverityWarning.Value() {
		failingSections = append(failingSections, healthsdk.HealthSectionWebsocket)
	}
	if report.Database.Severity.Value() > health.SeverityWarning.Value() {
		failingSections = append(failingSections, healthsdk.HealthSectionDatabase)
	}
	if report.WorkspaceProxy.Severity.Value() > health.SeverityWarning.Value() {
		failingSections = append(failingSections, healthsdk.HealthSectionWorkspaceProxy)
	}
	if report.ProvisionerDaemons.Severity.Value() > health.SeverityWarning.Value() {
		failingSections = append(failingSections, healthsdk.HealthSectionProvisionerDaemons)
	}

	report.Healthy = len(failingSections) == 0

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
