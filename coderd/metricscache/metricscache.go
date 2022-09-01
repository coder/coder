package metricscache

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/retry"
)

// Cache holds the DAU cache.
// The aggregation queries responsible for these values
// can take up to a minute on large deployments, but the cache has near zero
// effect on most deployments.
type Cache struct {
	database database.Store
	log      slog.Logger

	dausResponse atomic.Pointer[codersdk.DAUsResponse]

	doneCh chan struct{}
	cancel func()

	interval time.Duration
}

func New(db database.Store, log slog.Logger, interval time.Duration) *Cache {
	if interval <= 0 {
		interval = time.Hour
	}
	ctx, cancel := context.WithCancel(context.Background())

	c := &Cache{
		database: db,
		log:      log,
		doneCh:   make(chan struct{}),
		cancel:   cancel,
		interval: interval,
	}
	go c.run(ctx)
	return c
}

func fillEmptyDAUDays(rows []database.GetDAUsFromAgentStatsRow) []database.GetDAUsFromAgentStatsRow {
	var newRows []database.GetDAUsFromAgentStatsRow

	for i, row := range rows {
		if i == 0 {
			newRows = append(newRows, row)
			continue
		}

		last := rows[i-1]

		const day = time.Hour * 24
		diff := row.Date.Sub(last.Date)
		for diff > day {
			if diff <= day {
				break
			}
			last.Date = last.Date.Add(day)
			last.Daus = 0
			newRows = append(newRows, last)
			diff -= day
		}

		newRows = append(newRows, row)
		continue
	}

	return newRows
}

func (c *Cache) refresh(ctx context.Context) error {
	err := c.database.DeleteOldAgentStats(ctx)
	if err != nil {
		return xerrors.Errorf("delete old stats: %w", err)
	}

	daus, err := c.database.GetDAUsFromAgentStats(ctx)
	if err != nil {
		return err
	}

	var resp codersdk.DAUsResponse
	for _, ent := range fillEmptyDAUDays(daus) {
		resp.Entries = append(resp.Entries, codersdk.DAUEntry{
			Date: ent.Date,
			DAUs: int(ent.Daus),
		})
	}

	c.dausResponse.Store(&resp)
	return nil
}

func (c *Cache) run(ctx context.Context) {
	defer close(c.doneCh)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		for r := retry.New(time.Millisecond*100, time.Minute); r.Wait(ctx); {
			start := time.Now()
			err := c.refresh(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.log.Error(ctx, "refresh", slog.Error(err))
				continue
			}
			c.log.Debug(
				ctx,
				"metrics refreshed",
				slog.F("took", time.Since(start)),
				slog.F("interval", c.interval),
			)
			break
		}

		select {
		case <-ticker.C:
		case <-c.doneCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (c *Cache) Close() error {
	c.cancel()
	<-c.doneCh
	return nil
}

// DAUs returns the DAUs or nil if they aren't ready yet.
func (c *Cache) DAUs() codersdk.DAUsResponse {
	r := c.dausResponse.Load()
	if r == nil {
		return codersdk.DAUsResponse{}
	}
	return *r
}
