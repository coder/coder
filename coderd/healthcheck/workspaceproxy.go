package healthcheck

import (
	"context"
	"fmt"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

type WorkspaceProxyReportOptions struct {
	// UpdateProxyHealth is a function called when healthcheck is run.
	// This would normally be ProxyHealth.ForceUpdate().
	// We do this because if someone mashes the healthcheck refresh button
	// they would expect up-to-date data.
	UpdateProxyHealth func(context.Context) error
	// FetchWorkspaceProxies is a function that returns the available workspace proxies.
	FetchWorkspaceProxies func(context.Context) (codersdk.RegionsResponse[codersdk.WorkspaceProxy], error)
	// CurrentVersion is the current server version.
	// We pass this in to make it easier to test.
	CurrentVersion string
	Logger         slog.Logger
}

// @typescript-generate Report
type WorkspaceProxyReport struct {
	Healthy  bool     `json:"healthy"`
	Warnings []string `json:"warnings"`
	Error    *string  `json:"error"`

	WorkspaceProxies codersdk.RegionsResponse[codersdk.WorkspaceProxy]
}

func (r *WorkspaceProxyReport) Run(ctx context.Context, opts *WorkspaceProxyReportOptions) {
	r.Healthy = true
	r.Warnings = []string{}

	if opts.FetchWorkspaceProxies == nil {
		opts.Logger.Debug(ctx, "no workspace proxies configured")
		return
	}

	if opts.UpdateProxyHealth == nil {
		err := "opts.UpdateProxyHealth must not be nil if opts.FetchWorkspaceProxies is not nil"
		opts.Logger.Error(ctx, "developer error: "+err)
		r.Error = ptr.Ref(err)
		return
	}

	if err := opts.UpdateProxyHealth(ctx); err != nil {
		opts.Logger.Error(ctx, "failed to update proxy health: %w", err)
		r.Error = ptr.Ref(err.Error())
		return
	}

	proxies, err := opts.FetchWorkspaceProxies(ctx)
	if err != nil {
		opts.Logger.Error(ctx, "failed to fetch workspace proxies", slog.Error(err))
		r.Healthy = false
		r.Error = ptr.Ref(err.Error())
		return
	}

	r.WorkspaceProxies = proxies

	var numProxies int
	var healthyProxies int
	for _, proxy := range r.WorkspaceProxies.Regions {
		numProxies++
		if proxy.Healthy {
			healthyProxies++
		}

		// check versions
		if !buildinfo.VersionsMatch(proxy.Version, opts.CurrentVersion) {
			opts.Logger.Warn(ctx, "proxy version mismatch",
				slog.F("version", opts.CurrentVersion),
				slog.F("proxy_version", proxy.Version),
				slog.F("proxy_name", proxy.Name),
			)
			r.Healthy = false
			r.Warnings = append(r.Warnings, fmt.Sprintf("Proxy %q version %q does not match primary server version %q", proxy.Name, proxy.Version, opts.CurrentVersion))
		}
	}
}
