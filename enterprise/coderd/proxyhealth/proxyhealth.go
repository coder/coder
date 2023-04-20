package proxyhealth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
)

type ProxyHealthStatus string

const (
	// Reachable means the proxy access url is reachable and returns a healthy
	// status code.
	Reachable ProxyHealthStatus = "reachable"
	// Unreachable means the proxy access url is not responding.
	Unreachable ProxyHealthStatus = "unreachable"
	// Unregistered means the proxy has not registered a url yet. This means
	// the proxy was created with the cli, but has not yet been started.
	Unregistered ProxyHealthStatus = "unregistered"
)

type Options struct {
	// Interval is the interval at which the proxy health is checked.
	Interval   time.Duration
	DB         database.Store
	Logger     slog.Logger
	Client     *http.Client
	Prometheus *prometheus.Registry
}

// ProxyHealth runs a go routine that periodically checks the health of all
// workspace proxies. This information is stored in memory, so each coderd
// replica has its own view of the health of the proxies. These views should be
// consistent, and if they are not, it indicates a problem.
type ProxyHealth struct {
	db       database.Store
	interval time.Duration
	logger   slog.Logger
	client   *http.Client

	cache *atomic.Pointer[map[uuid.UUID]ProxyStatus]

	// PromMetrics
	healthCheckDuration prometheus.Histogram
}

func New(opts *Options) (*ProxyHealth, error) {
	if opts.Interval <= 0 {
		opts.Interval = time.Minute
	}
	if opts.DB == nil {
		return nil, xerrors.Errorf("db is required")
	}
	if opts.Prometheus == nil {
		opts.Prometheus = prometheus.NewRegistry()
	}

	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}
	// Set a timeout on the client, so we don't wait forever for a healthz response.
	tmp := *client
	tmp.Timeout = time.Second * 5
	client = &tmp

	// Prometheus metrics
	healthCheckDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "proxyhealth",
		Name:      "health_check_duration_seconds",
		Help:      "Histogram for duration of proxy health collection in seconds.",
		Buckets:   []float64{0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.500, 1, 5, 10, 30},
	})
	opts.Prometheus.MustRegister(healthCheckDuration)

	return &ProxyHealth{
		db:                  opts.DB,
		interval:            opts.Interval,
		logger:              opts.Logger,
		client:              client,
		cache:               &atomic.Pointer[map[uuid.UUID]ProxyStatus]{},
		healthCheckDuration: healthCheckDuration,
	}, nil
}

// Run will block until the context is canceled. It will periodically check the
// health of all proxies and store the results in the cache.
func (p *ProxyHealth) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			statuses, err := p.runOnce(ctx, now)
			if err != nil {
				p.logger.Error(ctx, "proxy health check failed", slog.Error(err))
				continue
			}
			// Store the statuses in the cache.
			p.cache.Store(&statuses)
		}
	}
}

// ForceUpdate runs a single health check and updates the cache. If the health
// check fails, the cache is not updated and an error is returned. This is useful
// to trigger an update when a proxy is created or deleted.
func (p *ProxyHealth) ForceUpdate(ctx context.Context) error {
	statuses, err := p.runOnce(ctx, time.Now())
	if err != nil {
		return err
	}

	// Store the statuses in the cache.
	p.cache.Store(&statuses)
	return nil
}

// HealthStatus returns the current health status of all proxies stored in the
// cache.
func (p *ProxyHealth) HealthStatus() map[uuid.UUID]ProxyStatus {
	ptr := p.cache.Load()
	if ptr == nil {
		return map[uuid.UUID]ProxyStatus{}
	}
	return *ptr
}

type ProxyStatus struct {
	// ProxyStatus includes the value of the proxy at the time of checking. This is
	// useful to know as it helps determine if the proxy checked has different values
	// then the proxy in hand. AKA if the proxy was updated, and the status was for
	// an older proxy.
	Proxy  database.WorkspaceProxy
	Status ProxyHealthStatus
	// StatusError is the error message returned when the proxy is unreachable.
	StatusError string
	CheckedAt   time.Time
}

// runOnce runs the health check for all workspace proxies. If there is an
// unexpected error, an error is returned. Expected errors will mark a proxy as
// unreachable.
func (p *ProxyHealth) runOnce(ctx context.Context, now time.Time) (map[uuid.UUID]ProxyStatus, error) {
	// Record from the given time.
	defer p.healthCheckDuration.Observe(time.Since(now).Seconds())

	proxies, err := p.db.GetWorkspaceProxies(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return nil, xerrors.Errorf("get workspace proxies: %w", err)
	}

	// Just use a mutex to protect map writes.
	var statusMu sync.Mutex
	proxyStatus := map[uuid.UUID]ProxyStatus{}

	grp, gctx := errgroup.WithContext(ctx)
	// Arbitrary parallelism limit.
	grp.SetLimit(5)

	for _, proxy := range proxies {
		if proxy.Deleted {
			// Ignore deleted proxies.
			continue
		}
		// Each proxy needs to have a status set. Make a local copy for the
		// call to be run async.
		proxy := proxy
		status := ProxyStatus{
			Proxy:     proxy,
			CheckedAt: now,
		}

		grp.Go(func() error {
			if proxy.Url == "" {
				// Empty URL means the proxy has not registered yet.
				// When the proxy is started, it will update the url.
				statusMu.Lock()
				defer statusMu.Unlock()
				status.Status = Unregistered
				proxyStatus[proxy.ID] = status
				return nil
			}

			// Try to hit the healthz endpoint.
			reqURL := fmt.Sprintf("%s/healthz", strings.TrimSuffix(proxy.Url, "/"))
			req, err := http.NewRequestWithContext(gctx, http.MethodGet, reqURL, nil)
			if err != nil {
				return xerrors.Errorf("new request: %w", err)
			}
			req = req.WithContext(gctx)

			resp, err := p.client.Do(req)
			if err == nil && resp.StatusCode != http.StatusOK {
				// No error but the status code is incorrect.
				status.Status = Unreachable
			} else if err == nil {
				status.Status = Reachable
			} else {
				// Any form of error is considered unreachable.
				status.Status = Unreachable
				status.StatusError = err.Error()
			}

			statusMu.Lock()
			defer statusMu.Unlock()
			proxyStatus[proxy.ID] = status
			return nil
		})
	}

	err = grp.Wait()
	if err != nil {
		return nil, xerrors.Errorf("group run: %w", err)
	}

	return proxyStatus, nil
}
