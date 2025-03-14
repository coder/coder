package proxyhealth
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/codersdk"
)
type Status string
const (
	// Unknown should never be returned by the proxy health check.
	Unknown Status = "unknown"
	// Healthy means the proxy access url is reachable and returns a healthy
	// status code.
	Healthy Status = "ok"
	// Unreachable means the proxy access url is not responding.
	Unreachable Status = "unreachable"
	// Unhealthy means the proxy access url is responding, but there is some
	// problem with the proxy. This problem may or may not be preventing functionality.
	Unhealthy Status = "unhealthy"
	// Unregistered means the proxy has not registered a url yet. This means
	// the proxy was created with the cli, but has not yet been started.
	Unregistered Status = "unregistered"
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
	// Cached values for quick access to the health of proxies.
	cache      *atomic.Pointer[map[uuid.UUID]ProxyStatus]
	proxyHosts *atomic.Pointer[[]string]
	// PromMetrics
	healthCheckDuration prometheus.Histogram
	healthCheckResults  *prometheusmetrics.CachedGaugeVec
}
func New(opts *Options) (*ProxyHealth, error) {
	if opts.Interval <= 0 {
		opts.Interval = time.Minute
	}
	if opts.DB == nil {
		return nil, fmt.Errorf("db is required")
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
	healthCheckResults := prometheusmetrics.NewCachedGaugeVec(prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "proxyhealth",
			Name:      "health_check_results",
			Help: "This endpoint returns a number to indicate the health status. " +
				"-3 (unknown), -2 (Unreachable), -1 (Unhealthy), 0 (Unregistered), 1 (Healthy)",
		}, []string{"proxy_id"}))
	opts.Prometheus.MustRegister(healthCheckResults)
	return &ProxyHealth{
		db:                  opts.DB,
		interval:            opts.Interval,
		logger:              opts.Logger,
		client:              client,
		cache:               &atomic.Pointer[map[uuid.UUID]ProxyStatus]{},
		proxyHosts:          &atomic.Pointer[[]string]{},
		healthCheckDuration: healthCheckDuration,
		healthCheckResults:  healthCheckResults,
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
			p.storeProxyHealth(statuses)
		}
	}
}
func (p *ProxyHealth) storeProxyHealth(statuses map[uuid.UUID]ProxyStatus) {
	var proxyHosts []string
	for _, s := range statuses {
		if s.ProxyHost != "" {
			proxyHosts = append(proxyHosts, s.ProxyHost)
		}
	}
	// Store the statuses in the cache before any other quick values.
	p.cache.Store(&statuses)
	p.proxyHosts.Store(&proxyHosts)
}
// ForceUpdate runs a single health check and updates the cache. If the health
// check fails, the cache is not updated and an error is returned. This is useful
// to trigger an update when a proxy is created or deleted.
func (p *ProxyHealth) ForceUpdate(ctx context.Context) error {
	statuses, err := p.runOnce(ctx, time.Now())
	if err != nil {
		return err
	}
	p.storeProxyHealth(statuses)
	return nil
}
// HealthStatus returns the current health status of all proxies stored in the
// cache.
func (p *ProxyHealth) HealthStatus() map[uuid.UUID]ProxyStatus {
	if p == nil {
		// This can happen because workspace proxies are still an experiment.
		// For the /regions endpoint, this will be nil in those cases.
		return map[uuid.UUID]ProxyStatus{}
	}
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
	Proxy database.WorkspaceProxy
	// ProxyHost is the host:port of the proxy url. This is included in the status
	// to make sure the proxy url is a valid URL. It also makes it easier to
	// escalate errors if the url.Parse errors (should never happen).
	ProxyHost string
	Status    Status
	Report    codersdk.ProxyHealthReport
	CheckedAt time.Time
}
// ProxyHosts returns the host:port of all healthy proxies.
// This can be computed from HealthStatus, but is cached to avoid the
// caller needing to loop over all proxies to compute this on all
// static web requests.
func (p *ProxyHealth) ProxyHosts() []string {
	ptr := p.proxyHosts.Load()
	if ptr == nil {
		return []string{}
	}
	return *ptr
}
// runOnce runs the health check for all workspace proxies. If there is an
// unexpected error, an error is returned. Expected errors will mark a proxy as
// unreachable.
func (p *ProxyHealth) runOnce(ctx context.Context, now time.Time) (map[uuid.UUID]ProxyStatus, error) {
	// Record from the given time.
	defer func() { p.healthCheckDuration.Observe(time.Since(now).Seconds()) }()
	//nolint:gocritic // Proxy health is a system service.
	proxies, err := p.db.GetWorkspaceProxies(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return nil, fmt.Errorf("get workspace proxies: %w", err)
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
			Status:    Unknown,
		}
		grp.Go(func() error {
			if proxy.Url == "" {
				// Empty URL means the proxy has not registered yet.
				// When the proxy is started, it will update the url.
				statusMu.Lock()
				defer statusMu.Unlock()
				p.healthCheckResults.WithLabelValues(prometheusmetrics.VectorOperationSet, 0, proxy.ID.String())
				status.Status = Unregistered
				proxyStatus[proxy.ID] = status
				return nil
			}
			// Try to hit the healthz-report endpoint for a comprehensive health check.
			reqURL := fmt.Sprintf("%s/healthz-report", strings.TrimSuffix(proxy.Url, "/"))
			req, err := http.NewRequestWithContext(gctx, http.MethodGet, reqURL, nil)
			if err != nil {
				return fmt.Errorf("new request: %w", err)
			}
			req = req.WithContext(gctx)
			resp, err := p.client.Do(req)
			if err == nil {
				defer resp.Body.Close()
			}
			// A switch statement felt easier to categorize the different cases than
			// if else statements or nested if statements.
			switch {
			case err == nil && resp.StatusCode == http.StatusOK:
				err := json.NewDecoder(resp.Body).Decode(&status.Report)
				if err != nil {
					isCoderErr := fmt.Errorf("proxy url %q is not a coder proxy instance, verify the url is correct", reqURL)
					if resp.Header.Get(codersdk.BuildVersionHeader) != "" {
						isCoderErr = fmt.Errorf("proxy url %q is a coder instance, but unable to decode the response payload. Could this be a primary coderd and not a proxy?", reqURL)
					}
					// If the response is not json, then the user likely input a bad url that returns status code 200.
					// This is very common, since most webpages do return a 200. So let's improve the error message.
					if notJSONErr := codersdk.ExpectJSONMime(resp); notJSONErr != nil {
						err = errors.Join(
							isCoderErr,
							fmt.Errorf("attempted to query health at %q but got back the incorrect content type: %w", reqURL, notJSONErr),
						)
						status.Report.Errors = []string{
							err.Error(),
						}
						status.Status = Unhealthy
						break
					}
					// If we cannot read the report, mark the proxy as unhealthy.
					status.Report.Errors = []string{
						errors.Join(
							isCoderErr,
							fmt.Errorf("received a status code 200, but failed to decode health report body: %w", err),
						).Error(),
					}
					status.Status = Unhealthy
					break
				}
				if len(status.Report.Errors) > 0 {
					status.Status = Unhealthy
					break
				}
				status.Status = Healthy
			case err == nil && resp.StatusCode != http.StatusOK:
				// Unhealthy as we did reach the proxy but it got an unexpected response.
				status.Status = Unhealthy
				var builder strings.Builder
				// This string is shown on the UI where newlines are respected.
				// This error message is not ever decoded programmatically, so keep it human-
				// readable.
				builder.WriteString(fmt.Sprintf("unexpected status code %d. ", resp.StatusCode))
				builder.WriteString(fmt.Sprintf("\nEncountered error, send a request to %q from the Coderd environment to debug this issue.", reqURL))
				// err will always be non-nil
				err := codersdk.ReadBodyAsError(resp)
				var apiErr *codersdk.Error
				if errors.As(err, &apiErr) {
					builder.WriteString(fmt.Sprintf("\nError Message: %s\nError Detail: %s", apiErr.Message, apiErr.Detail))
					for _, v := range apiErr.Validations {
						// Pretty sure this is not possible from the called endpoint, but just in case.
						builder.WriteString(fmt.Sprintf("\n\tValidation: %s=%s", v.Field, v.Detail))
					}
				}
				builder.WriteString(fmt.Sprintf("\nError: %s", err.Error()))
				status.Report.Errors = []string{builder.String()}
			case err != nil:
				// Request failed, mark the proxy as unreachable.
				status.Status = Unreachable
				status.Report.Errors = []string{fmt.Sprintf("request to proxy failed: %s", err.Error())}
			default:
				// This should never happen
				status.Status = Unknown
			}
			u, err := url.Parse(proxy.Url)
			if err != nil {
				// This should never happen. This would mean the proxy sent
				// us an invalid url?
				status.Report.Errors = append(status.Report.Errors, fmt.Sprintf("failed to parse proxy url: %s", err.Error()))
				status.Status = Unhealthy
			}
			status.ProxyHost = u.Host
			// Set the prometheus metric correctly.
			switch status.Status {
			case Healthy:
				p.healthCheckResults.WithLabelValues(prometheusmetrics.VectorOperationSet, 1, proxy.ID.String())
			case Unhealthy:
				p.healthCheckResults.WithLabelValues(prometheusmetrics.VectorOperationSet, -1, proxy.ID.String())
			case Unreachable:
				p.healthCheckResults.WithLabelValues(prometheusmetrics.VectorOperationSet, -2, proxy.ID.String())
			default:
				// Unknown
				p.healthCheckResults.WithLabelValues(prometheusmetrics.VectorOperationSet, -3, proxy.ID.String())
			}
			statusMu.Lock()
			defer statusMu.Unlock()
			proxyStatus[proxy.ID] = status
			return nil
		})
	}
	err = grp.Wait()
	if err != nil {
		return nil, fmt.Errorf("group run: %w", err)
	}
	p.healthCheckResults.Commit()
	return proxyStatus, nil
}
