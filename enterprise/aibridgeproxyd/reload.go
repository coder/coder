package aibridgeproxyd

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/elazarl/goproxy"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridged"
)

// ReloadedProvider is the classification of one ai_providers row.
// Host is the routable hostname; it's populated only when the embedded
// outcome's Status == aibridged.ProviderStatusEnabled.
type ReloadedProvider struct {
	aibridged.ProviderOutcome
	Host string
}

// ProviderReload is the result of a single refresh pass: every
// configured provider with its classification.
type ProviderReload struct {
	Providers []ReloadedProvider
}

// RefreshProvidersFunc returns the live provider classification used
// by Reload to rebuild the proxy's routing snapshot.
type RefreshProvidersFunc func(ctx context.Context) (ProviderReload, error)

// Reload refreshes proxy routing from the configured provider source.
// A refresh failure leaves the previous snapshot in place.
func (s *Server) Reload(ctx context.Context) error {
	if s.refreshProviders == nil {
		return nil
	}
	s.recordReloadAttempt()
	reload, err := s.refreshProviders(ctx)
	if err != nil {
		return xerrors.Errorf("refresh ai providers for proxy routing: %w", err)
	}
	router, err := buildProviderRouter(reload, s.allowedPorts)
	if err != nil {
		return xerrors.Errorf("build provider router (provider_count=%d): %w", len(reload.Providers), err)
	}
	s.providerRouter.Store(router)
	for _, p := range reload.Providers {
		if p.Status == aibridged.ProviderStatusError {
			s.logger.Warn(s.ctx, "provider excluded from routing",
				slog.F("provider", p.Name),
				slog.Error(p.Err),
			)
		}
	}
	s.recordReloadSuccess(reload)
	s.logger.Debug(s.ctx, "aibridgeproxyd router reloaded",
		slog.F("provider_count", len(reload.Providers)),
		slog.F("mitm_host_count", len(router.mitmHosts)),
		slog.F("mitm_hosts", router.mitmHosts),
	)
	return nil
}

// recordReloadAttempt stamps the attempt-time gauge at the start of a
// Reload. A reload that hangs mid-flight is detected by watching the
// gap between this gauge and ProvidersLastReloadSuccessTimestampSeconds.
func (s *Server) recordReloadAttempt() {
	if s.metrics == nil {
		return
	}
	s.metrics.ProvidersLastReloadTimestampSeconds.Set(float64(time.Now().Unix()))
}

// recordReloadSuccess rewrites the provider_info GaugeVec from the
// classified reload and stamps the success-time gauge. Reset clears
// series for providers that have left the configuration so they don't
// linger as stale.
func (s *Server) recordReloadSuccess(reload ProviderReload) {
	if s.metrics == nil {
		return
	}
	outcomes := make([]aibridged.ProviderOutcome, len(reload.Providers))
	for i, p := range reload.Providers {
		outcomes[i] = p.ProviderOutcome
	}
	aibridged.WriteProviderInfoSnapshot(s.metrics.ProviderInfo, outcomes)
	s.metrics.ProvidersLastReloadSuccessTimestampSeconds.Set(float64(time.Now().Unix()))
}

func (s *Server) loadProviderRouter() *providerRouter {
	if p := s.providerRouter.Load(); p != nil {
		return p
	}
	return emptyProviderRouter
}

// mitmHostsCondition returns a goproxy ReqConditionFunc that reads the
// MITM host set from the atomic router on every match. Using a closure
// instead of goproxy.ReqHostIs(...) lets Reload affect every later
// CONNECT without re-registering handlers.
func (s *Server) mitmHostsCondition() goproxy.ReqConditionFunc {
	return func(req *http.Request, _ *goproxy.ProxyCtx) bool {
		if req == nil {
			return false
		}
		return slices.Contains(s.loadProviderRouter().mitmHosts, strings.ToLower(req.URL.Host))
	}
}

// buildProviderRouter constructs a router snapshot from a classified
// provider reload. Only providers with Status ==
// aibridged.ProviderStatusEnabled are included in the active routing
// tables; the refresh function is responsible for classifying disabled
// and errored rows. First entry wins on duplicate hostnames as a
// defense-in-depth measure even though the refresh function should
// mark duplicates as errors.
func buildProviderRouter(reload ProviderReload, allowedPorts []string) (*providerRouter, error) {
	nameByHost := make(map[string]string, len(reload.Providers))
	domains := make([]string, 0, len(reload.Providers))
	for _, p := range reload.Providers {
		if p.Status != aibridged.ProviderStatusEnabled {
			continue
		}
		host := strings.ToLower(p.Host)
		if host == "" {
			continue
		}
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
