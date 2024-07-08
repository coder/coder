package healthcheck

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
)

type WorkspaceProxyReport healthsdk.WorkspaceProxyReport

type WorkspaceProxyReportOptions struct {
	WorkspaceProxiesFetchUpdater WorkspaceProxiesFetchUpdater
	Dismissed                    bool
}

type WorkspaceProxiesFetchUpdater interface {
	Fetch(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error)
	Update(context.Context) error
}

// AGPLWorkspaceProxiesFetchUpdater implements WorkspaceProxiesFetchUpdater
// to the extent required by AGPL code. Which isn't that much.
type AGPLWorkspaceProxiesFetchUpdater struct{}

func (*AGPLWorkspaceProxiesFetchUpdater) Fetch(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error) {
	return codersdk.RegionsResponse[codersdk.WorkspaceProxy]{}, nil
}

func (*AGPLWorkspaceProxiesFetchUpdater) Update(context.Context) error {
	return nil
}

func (r *WorkspaceProxyReport) Run(ctx context.Context, opts *WorkspaceProxyReportOptions) {
	r.Healthy = true
	r.Severity = health.SeverityOK
	r.Warnings = make([]health.Message, 0)
	r.Dismissed = opts.Dismissed

	if opts.WorkspaceProxiesFetchUpdater == nil {
		opts.WorkspaceProxiesFetchUpdater = &AGPLWorkspaceProxiesFetchUpdater{}
	}

	// If this fails, just mark it as a warning. It is still updated in the background.
	if err := opts.WorkspaceProxiesFetchUpdater.Update(ctx); err != nil {
		r.Severity = health.SeverityWarning
		r.Warnings = append(r.Warnings, health.Messagef(health.CodeProxyUpdate, "update proxy health: %s", err))
		return
	}

	proxies, err := opts.WorkspaceProxiesFetchUpdater.Fetch(ctx)
	if err != nil {
		r.Healthy = false
		r.Severity = health.SeverityError
		r.Error = health.Errorf(health.CodeProxyFetch, "fetch workspace proxies: %s", err)
		return
	}

	for _, proxy := range proxies.Regions {
		if !proxy.Deleted {
			r.WorkspaceProxies.Regions = append(r.WorkspaceProxies.Regions, proxy)
		}
	}
	if r.WorkspaceProxies.Regions == nil {
		r.WorkspaceProxies.Regions = make([]codersdk.WorkspaceProxy, 0)
	}

	// Stable sort based on create timestamp.
	sort.Slice(r.WorkspaceProxies.Regions, func(i int, j int) bool {
		return r.WorkspaceProxies.Regions[i].CreatedAt.Before(r.WorkspaceProxies.Regions[j].CreatedAt)
	})

	var total, healthy, warning int
	var errs []string
	for _, proxy := range r.WorkspaceProxies.Regions {
		total++
		if proxy.Healthy {
			// Warnings in the report are not considered unhealthy, only errors.
			healthy++
		}
		if len(proxy.Status.Report.Warnings) > 0 {
			warning++
		}

		for _, err := range proxy.Status.Report.Warnings {
			r.Warnings = append(r.Warnings, health.Messagef(health.CodeProxyUnhealthy, "%s: %s", proxy.Name, err))
		}
		for _, err := range proxy.Status.Report.Errors {
			errs = append(errs, fmt.Sprintf("%s: %s", proxy.Name, err))
		}
	}

	r.Severity = calculateSeverity(total, healthy, warning)
	r.Healthy = r.Severity.Value() < health.SeverityError.Value()
	for _, err := range errs {
		switch r.Severity {
		case health.SeverityWarning, health.SeverityOK:
			r.Warnings = append(r.Warnings, health.Messagef(health.CodeProxyUnhealthy, err))
		case health.SeverityError:
			r.appendError(*health.Errorf(health.CodeProxyUnhealthy, err))
		}
	}
}

// appendError appends errs onto r.Error.
// We only have one error, so multiple errors need to be squashed in there.
func (r *WorkspaceProxyReport) appendError(es ...string) {
	if len(es) == 0 {
		return
	}
	if r.Error != nil {
		es = append([]string{*r.Error}, es...)
	}
	r.Error = ptr.Ref(strings.Join(es, "\n"))
}

// calculateSeverity returns:
// health.SeverityError if all proxies are unhealthy,
// health.SeverityOK if all proxies are healthy and there are no warnings,
// health.SeverityWarning otherwise.
func calculateSeverity(total, healthy, warning int) health.Severity {
	if total == 0 || (total == healthy && warning == 0) {
		return health.SeverityOK
	}
	if healthy == 0 {
		return health.SeverityError
	}
	return health.SeverityWarning
}
