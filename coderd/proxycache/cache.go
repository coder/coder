package proxycache

import (
	"context"
	"regexp"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/coder/coder/coderd/database/dbauthz"

	"github.com/coder/coder/coderd/httpapi"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
)

// Cache is used to cache workspace proxies to prevent having to do a database
// call each time the list of workspace proxies is required. Workspace proxies
// are very infrequently updated, so this cache should rarely change.
//
// The accessor functions on the cache are intended to optimize the hot path routes
// in the API. Meaning, this cache can implement the specific logic required to
// using the cache in the API, instead of just returning the slice of proxies.
type Cache struct {
	db       database.Store
	log      slog.Logger
	interval time.Duration

	// ctx controls the lifecycle of the cache.
	ctx    context.Context
	cancel func()

	// Data
	mu sync.RWMutex
	// cachedValues is the list of workspace proxies that are currently cached.
	// This is the raw data from the database.
	cachedValues []database.WorkspaceProxy
	// cachedPatterns is a map of the workspace proxy patterns to their compiled
	// regular expressions.
	cachedPatterns map[string]*regexp.Regexp
}

func New(ctx context.Context, log slog.Logger, db database.Store, interval time.Duration) *Cache {
	if interval == 0 {
		interval = 5 * time.Minute
	}
	ctx, cancel := context.WithCancel(ctx)
	c := &Cache{
		ctx:      ctx,
		db:       db,
		log:      log,
		cancel:   cancel,
		interval: interval,

		cachedPatterns: map[string]*regexp.Regexp{},
	}
	return c
}

// ExecuteHostnamePattern is used to determine if a given hostname matches
// any of the workspace proxy patterns. If it does, the subdomain for the app
// is returned. If it does not, an empty string is returned with 'false'.
func (c *Cache) ExecuteHostnamePattern(host string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rg := range c.cachedPatterns {
		sub, ok := httpapi.ExecuteHostnamePattern(rg, host)
		if ok {
			return sub, ok
		}
	}
	return "", false
}

func (c *Cache) run() {
	// Load the initial cache.
	c.updateCache()
	ticker := time.NewTicker(c.interval)
	pprof.Do(c.ctx, pprof.Labels("service", "proxy-cache"), func(ctx context.Context) {
		for {
			select {
			case <-ticker.C:
				c.updateCache()
			case <-c.ctx.Done():
				return
			}
		}
	})
}

// ForceUpdate can be called externally to force an update of the cache.
// The regular update interval will still be used.
func (c *Cache) ForceUpdate() {
	c.updateCache()
}

// updateCache is used to update the cache with the latest values from the database.
func (c *Cache) updateCache() {
	c.mu.Lock()
	defer c.mu.Unlock()

	proxies, err := c.db.GetWorkspaceProxies(dbauthz.AsSystemRestricted(c.ctx))
	if err != nil {
		c.log.Error(c.ctx, "failed to get workspace proxies", slog.Error(err))
		return
	}

	c.cachedValues = proxies

	keep := make(map[string]struct{})
	for _, p := range proxies {
		if p.WildcardHostname == "" {
			// It is possible some moons do not support subdomain apps.
			continue
		}

		keep[p.WildcardHostname] = struct{}{}
		if _, ok := c.cachedPatterns[p.WildcardHostname]; ok {
			// pattern is already cached
			continue
		}

		rg, err := httpapi.CompileHostnamePattern(p.WildcardHostname)
		if err != nil {
			c.log.Error(c.ctx, "failed to compile workspace proxy pattern",
				slog.Error(err),
				slog.F("proxy_id", p.ID),
				slog.F("proxy_name", p.Name),
				slog.F("proxy_hostname", p.WildcardHostname),
			)
			continue
		}
		c.cachedPatterns[p.WildcardHostname] = rg
	}

	// Remove any excess patterns
	for k := range c.cachedPatterns {
		if _, ok := keep[k]; !ok {
			delete(c.cachedPatterns, k)
		}
	}
}

func (c *Cache) Close() {
	c.cancel()
}
