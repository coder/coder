package mcpclient

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

// NegativeCacheTTL is how long a server that timed out during
// connect is skipped before being retried. Connects run on every
// generation step, so without this cache a black-holed server
// costs the full connect budget on every step of every chat on
// the pod.
const NegativeCacheTTL = 60 * time.Second

// ConnectOutcomeSkipped means the server was not dialed because a
// recent connect attempt timed out and the failure is cached.
const ConnectOutcomeSkipped ConnectOutcome = "skipped"

// NegativeCache is a per-process cache of MCP servers whose
// connect attempts recently timed out. Entries are keyed by config
// ID and bound to the config's UpdatedAt, so editing a server
// config busts its entry immediately.
type NegativeCache struct {
	clock quartz.Clock

	mu      sync.Mutex
	entries map[uuid.UUID]negativeCacheEntry
}

type negativeCacheEntry struct {
	configUpdatedAt time.Time
	expiresAt       time.Time
}

// NewNegativeCache creates a NegativeCache. A nil clock uses the
// real clock.
func NewNegativeCache(clock quartz.Clock) *NegativeCache {
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &NegativeCache{
		clock:   clock,
		entries: make(map[uuid.UUID]negativeCacheEntry),
	}
}

// Filter partitions configs into those that should be dialed and
// ConnectSummary values for those skipped due to a cached recent
// timeout. Expired and stale (config edited) entries are evicted.
func (c *NegativeCache) Filter(
	ctx context.Context,
	logger slog.Logger,
	configs []database.MCPServerConfig,
) ([]database.MCPServerConfig, []ConnectSummary) {
	if c == nil {
		return configs, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clock.Now()
	connectable := make([]database.MCPServerConfig, 0, len(configs))
	var skipped []ConnectSummary
	for _, cfg := range configs {
		entry, ok := c.entries[cfg.ID]
		if !ok {
			connectable = append(connectable, cfg)
			continue
		}
		if now.After(entry.expiresAt) || !entry.configUpdatedAt.Equal(cfg.UpdatedAt) {
			delete(c.entries, cfg.ID)
			connectable = append(connectable, cfg)
			continue
		}
		logger.Warn(ctx,
			"skipping MCP server due to recent connect timeout",
			slog.F("server_slug", cfg.Slug),
			slog.F("server_url", RedactURL(cfg.Url)),
			slog.F("retry_after", entry.expiresAt),
		)
		skipped = append(skipped, ConnectSummary{
			ConfigID: cfg.ID,
			Slug:     cfg.Slug,
			Outcome:  ConnectOutcomeSkipped,
			Error:    "recent connect timeout; retrying after " + entry.expiresAt.UTC().Format(time.RFC3339),
		})
	}
	return connectable, skipped
}

// Record caches connect timeouts from summaries. Only timeouts are
// cached: fast failures (auth, DNS) are cheap to retry every step
// and may be fixed mid-conversation, while a timeout costs the
// whole connect budget on every step until it recovers.
func (c *NegativeCache) Record(
	configs []database.MCPServerConfig,
	summaries []ConnectSummary,
) {
	if c == nil {
		return
	}

	updatedAtByID := make(map[uuid.UUID]time.Time, len(configs))
	for _, cfg := range configs {
		updatedAtByID[cfg.ID] = cfg.UpdatedAt
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clock.Now()
	for _, summary := range summaries {
		if summary.Outcome != ConnectOutcomeTimeout {
			continue
		}
		updatedAt, ok := updatedAtByID[summary.ConfigID]
		if !ok {
			continue
		}
		c.entries[summary.ConfigID] = negativeCacheEntry{
			configUpdatedAt: updatedAt,
			expiresAt:       now.Add(NegativeCacheTTL),
		}
	}
}
