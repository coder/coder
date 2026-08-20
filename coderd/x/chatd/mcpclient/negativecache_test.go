package mcpclient_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/quartz"
)

func TestNegativeCache(t *testing.T) {
	t.Parallel()

	newConfig := func(slug string) database.MCPServerConfig {
		cfg := makeConfig(slug, "http://127.0.0.1:1")
		cfg.UpdatedAt = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
		return cfg
	}
	timeoutSummary := func(cfg database.MCPServerConfig) mcpclient.ConnectSummary {
		return mcpclient.ConnectSummary{
			ConfigID: cfg.ID,
			Slug:     cfg.Slug,
			Outcome:  mcpclient.ConnectOutcomeTimeout,
			Error:    "connect: context deadline exceeded",
		}
	}

	t.Run("SkipsRecentTimeoutUntilTTL", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		clock := quartz.NewMock(t)
		cache := mcpclient.NewNegativeCache(clock)
		cfg := newConfig("srv")

		// Not cached: config passes through.
		connectable, skipped := cache.Filter(ctx, logger, []database.MCPServerConfig{cfg})
		require.Len(t, connectable, 1)
		require.Empty(t, skipped)

		cache.Record(connectable, []mcpclient.ConnectSummary{timeoutSummary(cfg)})

		// Within TTL: skipped with a summary for the debug run.
		connectable, skipped = cache.Filter(ctx, logger, []database.MCPServerConfig{cfg})
		require.Empty(t, connectable)
		require.Len(t, skipped, 1)
		require.Equal(t, mcpclient.ConnectOutcomeSkipped, skipped[0].Outcome)
		require.Equal(t, cfg.ID, skipped[0].ConfigID)
		require.NotEmpty(t, skipped[0].Error)

		// After TTL: retried.
		clock.Advance(mcpclient.NegativeCacheTTL + time.Second)
		connectable, skipped = cache.Filter(ctx, logger, []database.MCPServerConfig{cfg})
		require.Len(t, connectable, 1)
		require.Empty(t, skipped)
	})

	t.Run("ConfigEditBustsEntry", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		clock := quartz.NewMock(t)
		cache := mcpclient.NewNegativeCache(clock)
		cfg := newConfig("srv")

		cache.Record([]database.MCPServerConfig{cfg}, []mcpclient.ConnectSummary{timeoutSummary(cfg)})

		edited := cfg
		edited.UpdatedAt = cfg.UpdatedAt.Add(time.Minute)
		connectable, skipped := cache.Filter(ctx, logger, []database.MCPServerConfig{edited})
		require.Len(t, connectable, 1)
		require.Empty(t, skipped)
	})

	t.Run("OnlyTimeoutsAreCached", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		clock := quartz.NewMock(t)
		cache := mcpclient.NewNegativeCache(clock)
		cfg := newConfig("srv")

		cache.Record([]database.MCPServerConfig{cfg}, []mcpclient.ConnectSummary{
			{
				ConfigID: cfg.ID,
				Slug:     cfg.Slug,
				Outcome:  mcpclient.ConnectOutcomeError,
				Error:    "401 unauthorized",
			},
			{
				ConfigID: uuid.New(),
				Slug:     "unknown",
				Outcome:  mcpclient.ConnectOutcomeTimeout,
			},
		})

		connectable, skipped := cache.Filter(ctx, logger, []database.MCPServerConfig{cfg})
		require.Len(t, connectable, 1)
		require.Empty(t, skipped)
	})

	t.Run("NilCacheIsNoop", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		var cache *mcpclient.NegativeCache
		cfg := newConfig("srv")

		connectable, skipped := cache.Filter(ctx, logger, []database.MCPServerConfig{cfg})
		require.Len(t, connectable, 1)
		require.Empty(t, skipped)
		cache.Record(connectable, []mcpclient.ConnectSummary{timeoutSummary(cfg)})
	})
}
