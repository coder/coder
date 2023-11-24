package healthcheck

import (
	"context"
	"sort"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"

	"github.com/hashicorp/go-multierror"
)

type WorkspaceProxyReportOptions struct {
	// CurrentVersion is the current server version.
	// We pass this in to make it easier to test.
	CurrentVersion string
	// FetchWorkspaceProxies is a function that returns the available workspace proxies.
	FetchWorkspaceProxies *func(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error)
	// UpdateProxyHealth is a function called when healthcheck is run.
	// This would normally be ProxyHealth.ForceUpdate().
	// We do this because if someone mashes the healthcheck refresh button
	// they would expect up-to-date data.
	UpdateProxyHealth *func(context.Context) error
}

// @typescript-generate WorkspaceProxyReport
type WorkspaceProxyReport struct {
	Healthy  bool            `json:"healthy"`
	Severity health.Severity `json:"severity"`
	Warnings []string        `json:"warnings"`
	Error    *string         `json:"error"`

	WorkspaceProxies codersdk.RegionsResponse[codersdk.WorkspaceProxy] `json:"workspace_proxies"`
}

func (r *WorkspaceProxyReport) Run(ctx context.Context, opts *WorkspaceProxyReportOptions) {
	r.Healthy = true
	r.Severity = health.SeverityOK
	r.Warnings = []string{}

	if opts.FetchWorkspaceProxies == nil {
		return
	}
	fetchWorkspaceProxiesFunc := *opts.FetchWorkspaceProxies

	if opts.UpdateProxyHealth == nil {
		err := "opts.UpdateProxyHealth must not be nil if opts.FetchWorkspaceProxies is not nil"
		r.Error = ptr.Ref(err)
		return
	}
	updateProxyHealthFunc := *opts.UpdateProxyHealth

	// If this fails, just mark it as a warning. It is still updated in the background.
	if err := updateProxyHealthFunc(ctx); err != nil {
		r.Severity = health.SeverityWarning
		r.Warnings = append(r.Warnings, xerrors.Errorf("update proxy health: %w", err).Error())
		return
	}

	proxies, err := fetchWorkspaceProxiesFunc(ctx)
	if err != nil {
		r.Healthy = false
		r.Severity = health.SeverityError
		r.Error = ptr.Ref(err.Error())
		return
	}

	r.WorkspaceProxies = proxies
	// Stable sort based on create timestamp.
	sort.Slice(r.WorkspaceProxies.Regions, func(i int, j int) bool {
		return r.WorkspaceProxies.Regions[i].CreatedAt.Before(r.WorkspaceProxies.Regions[j].CreatedAt)
	})

	var total, healthy int
	for _, proxy := range r.WorkspaceProxies.Regions {
		total++
		if proxy.Healthy {
			healthy++
		}

		if len(proxy.Status.Report.Errors) > 0 {
			for _, err := range proxy.Status.Report.Errors {
				r.appendError(xerrors.New(err))
			}
		}
	}

	r.Severity = calculateSeverity(total, healthy)
	r.Healthy = r.Severity != health.SeverityError

	// Versions _must_ match. Perform this check last. This will clobber any other severity.
	for _, proxy := range r.WorkspaceProxies.Regions {
		if vErr := checkVersion(proxy, opts.CurrentVersion); vErr != nil {
			r.Healthy = false
			r.Severity = health.SeverityError
			r.appendError(vErr)
		}
	}
}

// appendError multierror-appends err onto r.Error.
// We only have one error, so multiple errors need to be squashed in there.
func (r *WorkspaceProxyReport) appendError(errs ...error) {
	var prevErr error
	if r.Error != nil {
		prevErr = xerrors.New(*r.Error)
	}
	r.Error = ptr.Ref(multierror.Append(prevErr, errs...).Error())
}

func checkVersion(proxy codersdk.WorkspaceProxy, currentVersion string) error {
	if proxy.Version == "" {
		return nil // may have not connected yet, this is OK
	}
	if buildinfo.VersionsMatch(proxy.Version, currentVersion) {
		return nil
	}

	return xerrors.Errorf("proxy %q version %q does not match primary server version %q",
		proxy.Name,
		proxy.Version,
		currentVersion,
	)
}

// calculateSeverity returns:
// health.SeverityError if all proxies are unhealthy,
// health.SeverityOK if all proxies are healthy,
// health.SeverityWarning otherwise.
func calculateSeverity(total, healthy int) health.Severity {
	if total == 0 || total == healthy {
		return health.SeverityOK
	}
	if total-healthy == total {
		return health.SeverityError
	}
	return health.SeverityWarning
}
