package healthcheck

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/healthcheck/health"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"

	"golang.org/x/xerrors"
)

// @typescript-generate WorkspaceProxyReport
type WorkspaceProxyReport struct {
	Healthy   bool             `json:"healthy"`
	Severity  health.Severity  `json:"severity"`
	Warnings  []health.Message `json:"warnings"`
	Dismissed bool             `json:"dismissed"`
	Error     *string          `json:"error"`

	WorkspaceProxies codersdk.RegionsResponse[codersdk.WorkspaceProxy] `json:"workspace_proxies"`
}

type WorkspaceProxyReportOptions struct {
	// CurrentVersion is the current server version.
	// We pass this in to make it easier to test.
	CurrentVersion               string
	WorkspaceProxiesFetchUpdater WorkspaceProxiesFetchUpdater

	Dismissed bool
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

	r.WorkspaceProxies = proxies
	if r.WorkspaceProxies.Regions == nil {
		r.WorkspaceProxies.Regions = make([]codersdk.WorkspaceProxy, 0)
	}

	// Stable sort based on create timestamp.
	sort.Slice(r.WorkspaceProxies.Regions, func(i int, j int) bool {
		return r.WorkspaceProxies.Regions[i].CreatedAt.Before(r.WorkspaceProxies.Regions[j].CreatedAt)
	})

	var total, healthy int
	var errs []string
	for _, proxy := range r.WorkspaceProxies.Regions {
		total++
		if proxy.Healthy {
			healthy++
		}

		if len(proxy.Status.Report.Errors) > 0 {
			for _, err := range proxy.Status.Report.Errors {
				errs = append(errs, fmt.Sprintf("%s: %s", proxy.Name, err))
			}
		}
	}

	r.Severity = calculateSeverity(total, healthy)
	r.Healthy = r.Severity.Value() < health.SeverityError.Value()
	for _, err := range errs {
		switch r.Severity {
		case health.SeverityWarning, health.SeverityOK:
			r.Warnings = append(r.Warnings, health.Messagef(health.CodeProxyUnhealthy, err))
		case health.SeverityError:
			r.appendError(*health.Errorf(health.CodeProxyUnhealthy, err))
		}
	}

	// Versions _must_ match. Perform this check last. This will clobber any other severity.
	for _, proxy := range r.WorkspaceProxies.Regions {
		if vErr := checkVersion(proxy, opts.CurrentVersion); vErr != nil {
			r.Healthy = false
			r.Severity = health.SeverityError
			r.appendError(*health.Errorf(health.CodeProxyVersionMismatch, vErr.Error()))
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
