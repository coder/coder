package metricscache

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/retry"
)

// Cache holds the template DAU cache.
// The aggregation queries responsible for these values can take up to a minute
// on large deployments. Even in small deployments, aggregation queries can
// take a few hundred milliseconds, which would ruin page load times and
// database performance if in the hot path.
type Cache struct {
	database database.Store
	log      slog.Logger

	templateDAUResponses atomic.Pointer[map[string]codersdk.TemplateDAUsResponse]

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

func fillEmptyDays(rows []database.GetTemplateDAUsRow) []database.GetTemplateDAUsRow {
	var newRows []database.GetTemplateDAUsRow

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
			last.Amount = 0
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

	templates, err := c.database.GetTemplates(ctx)
	if err != nil {
		return err
	}

	templateDAUs := make(map[string]codersdk.TemplateDAUsResponse, len(templates))

	for _, template := range templates {
		daus, err := c.database.GetTemplateDAUs(ctx, template.ID)
		if err != nil {
			return err
		}

		var resp codersdk.TemplateDAUsResponse
		for _, ent := range fillEmptyDays(daus) {
			resp.Entries = append(resp.Entries, codersdk.DAUEntry{
				Date:   ent.Date,
				Amount: int(ent.Amount),
			})
		}
		templateDAUs[template.ID.String()] = resp
	}

	c.templateDAUResponses.Store(&templateDAUs)
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

// TemplateDAUs returns an empty response if the template doesn't have users
// or is loading for the first time.
func (c *Cache) TemplateDAUs(id uuid.UUID) codersdk.TemplateDAUsResponse {
	m := c.templateDAUResponses.Load()
	if m == nil {
		// Data loading.
		return codersdk.TemplateDAUsResponse{}
	}

	resp, ok := (*m)[id.String()]
	if !ok {
		// Probably no data.
		return codersdk.TemplateDAUsResponse{}
	}
	return resp
}
