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
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
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
	Interval time.Duration
	DB       database.Store
	Logger   slog.Logger
	client   *http.Client
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
}

func New(opts *Options) (*ProxyHealth, error) {
	if opts.Interval <= 0 {
		opts.Interval = time.Minute
	}
	if opts.DB == nil {
		return nil, xerrors.Errorf("db is required")
	}

	client := opts.client
	if opts.client == nil {
		client = http.DefaultClient
	}
	// Set a timeout on the client so we don't wait forever for a healthz response.
	tmp := *client
	tmp.Timeout = time.Second * 5
	client = &tmp

	return &ProxyHealth{
		db:       opts.DB,
		interval: opts.Interval,
		logger:   opts.Logger,
		client:   opts.client,
		cache:    &atomic.Pointer[map[uuid.UUID]ProxyStatus]{},
	}, nil
}

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

func (p *ProxyHealth) HealthStatus() map[uuid.UUID]ProxyStatus {
	ptr := p.cache.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

type ProxyStatus struct {
	Proxy     database.WorkspaceProxy
	Status    ProxyHealthStatus
	CheckedAt time.Time
}

// runOnce runs the health check for all workspace proxies. If there is an
// unexpected error, an error is returned. Expected errors will mark a proxy as
// unreachable.
func (p *ProxyHealth) runOnce(ctx context.Context, t time.Time) (map[uuid.UUID]ProxyStatus, error) {
	proxies, err := p.db.GetWorkspaceProxies(ctx)
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
			CheckedAt: t,
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
			statusMu.Lock()
			defer statusMu.Unlock()
			if err != nil {
				status.Status = Unreachable
				return nil
			}

			if resp.StatusCode != http.StatusOK {
				status.Status = Unreachable
				return nil
			}

			status.Status = Reachable
			return nil
		})
	}

	err = grp.Wait()
	if err != nil {
		return nil, xerrors.Errorf("group run: %w", err)
	}

	return nil, nil
}
