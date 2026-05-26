package aibridgeproxyd

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/elazarl/goproxy"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// Reload refreshes proxy routing from the configured provider source.
// A refresh failure leaves the previous snapshot in place.
func (s *Server) Reload(ctx context.Context) error {
	if s.refreshProviders == nil {
		return nil
	}
	providers, err := s.refreshProviders(ctx)
	if err != nil {
		return xerrors.Errorf("refresh ai providers for proxy routing: %w", err)
	}
	router, err := buildProviderRouter(ctx, s.logger, providers, s.allowedPorts)
	if err != nil {
		return xerrors.Errorf("build provider router (provider_count=%d): %w", len(providers), err)
	}
	s.providerRouter.Store(router)
	s.logger.Debug(s.ctx, "aibridgeproxyd router reloaded",
		slog.F("mitm_host_count", len(router.mitmHosts)),
	)
	return nil
}

func (s *Server) loadProviderRouter() *providerRouter {
	if p := s.providerRouter.Load(); p != nil {
		return p
	}
	return emptyProviderRouter
}

// mitmHostsCondition returns a goproxy ReqConditionFunc that reads the
// allowlist from the atomic router on every match. Using a closure
// instead of goproxy.ReqHostIs(...) lets Reload affect every later
// CONNECT without re-registering handlers.
func (s *Server) mitmHostsCondition() goproxy.ReqConditionFunc {
	return func(req *http.Request, _ *goproxy.ProxyCtx) bool {
		if req == nil {
			return false
		}
		return slices.Contains(s.loadProviderRouter().mitmHosts, req.URL.Host)
	}
}

// buildProviderRouter constructs a router snapshot from a refreshed
// provider list. First provider wins on duplicate hostnames.
func buildProviderRouter(ctx context.Context, logger slog.Logger, providers []ProviderRoute, allowedPorts []string) (*providerRouter, error) {
	nameByHost := make(map[string]string, len(providers))
	var domains []string
	for _, p := range providers {
		if p.BaseURL == "" {
			logger.Warn(ctx, "skipping ai provider without base url",
				slog.F("provider_name", p.Name),
			)
			continue
		}
		u, err := url.Parse(p.BaseURL)
		if err != nil {
			logger.Warn(ctx, "skipping ai provider with invalid base url",
				slog.F("provider_name", p.Name),
				slog.F("base_url", p.BaseURL),
				slog.Error(err),
			)
			continue
		}
		if u.Hostname() == "" {
			logger.Warn(ctx, "skipping ai provider base url without hostname",
				slog.F("provider_name", p.Name),
				slog.F("base_url", p.BaseURL),
			)
			continue
		}
		host := strings.ToLower(u.Hostname())
		if _, exists := nameByHost[host]; exists {
			continue
		}
		nameByHost[host] = p.Name
		domains = append(domains, host)
	}
	mitmHosts, err := convertDomainsToHosts(domains, allowedPorts)
	if err != nil {
		return nil, err
	}
	return &providerRouter{mitmHosts: mitmHosts, nameByHost: nameByHost}, nil
}

// buildBootRouter seeds the providerRouter from the boot-time inputs.
// The lookup function is consulted only for hosts in the allowlist; a
// nil function with an empty allowlist is fine and yields an empty
// router (the proxy fails closed until Reload populates it).
func buildBootRouter(domainAllowlist []string, providerFromHost func(string) string, allowedPorts []string) (*providerRouter, error) {
	mitmHosts, err := convertDomainsToHosts(domainAllowlist, allowedPorts)
	if err != nil {
		return nil, xerrors.Errorf("invalid domain allowlist: %w", err)
	}
	nameByHost := make(map[string]string, len(domainAllowlist))
	for _, domain := range domainAllowlist {
		domain = strings.TrimSpace(strings.ToLower(domain))
		if domain == "" {
			continue
		}
		var name string
		if providerFromHost != nil {
			name = providerFromHost(domain)
		}
		if name == "" {
			return nil, xerrors.Errorf("domain %q is in allowlist but has no provider mapping", domain)
		}
		nameByHost[domain] = name
	}
	return &providerRouter{mitmHosts: mitmHosts, nameByHost: nameByHost}, nil
}
