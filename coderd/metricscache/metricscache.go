package metricscache

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/codersdk"
	"github.com/coder/retry"
)

// Cache holds the template metrics.
// The aggregation queries responsible for these values can take up to a minute
// on large deployments. Even in small deployments, aggregation queries can
// take a few hundred milliseconds, which would ruin page load times and
// database performance if in the hot path.
type Cache struct {
	database database.Store
	log      slog.Logger

	deploymentDAUResponses   atomic.Pointer[codersdk.DeploymentDAUsResponse]
	templateDAUResponses     atomic.Pointer[map[uuid.UUID]codersdk.TemplateDAUsResponse]
	templateUniqueUsers      atomic.Pointer[map[uuid.UUID]int]
	templateAverageBuildTime atomic.Pointer[map[uuid.UUID]database.GetTemplateAverageBuildTimeRow]

	done   chan struct{}
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
		done:     make(chan struct{}),
		cancel:   cancel,
		interval: interval,
	}
	go c.run(ctx)
	return c
}

func fillEmptyDays(sortedDates []time.Time) []time.Time {
	var newDates []time.Time

	for i, ti := range sortedDates {
		if i == 0 {
			newDates = append(newDates, ti)
			continue
		}

		last := sortedDates[i-1]

		const day = time.Hour * 24
		diff := ti.Sub(last)
		for diff > day {
			if diff <= day {
				break
			}
			last = last.Add(day)
			newDates = append(newDates, last)
			diff -= day
		}

		newDates = append(newDates, ti)
		continue
	}

	return newDates
}

func convertDAUResponse(rows []database.GetTemplateDAUsRow) codersdk.TemplateDAUsResponse {
	respMap := make(map[time.Time][]uuid.UUID)
	for _, row := range rows {
		uuids := respMap[row.Date]
		if uuids == nil {
			uuids = make([]uuid.UUID, 0, 8)
		}
		uuids = append(uuids, row.UserID)
		respMap[row.Date] = uuids
	}

	dates := maps.Keys(respMap)
	slices.SortFunc(dates, func(a, b time.Time) bool {
		return a.Before(b)
	})

	var resp codersdk.TemplateDAUsResponse
	for _, date := range fillEmptyDays(dates) {
		resp.Entries = append(resp.Entries, codersdk.DAUEntry{
			Date:   date,
			Amount: len(respMap[date]),
		})
	}

	return resp
}

func convertDeploymentDAUResponse(rows []database.GetDeploymentDAUsRow) codersdk.DeploymentDAUsResponse {
	respMap := make(map[time.Time][]uuid.UUID)
	for _, row := range rows {
		respMap[row.Date] = append(respMap[row.Date], row.UserID)
	}

	dates := maps.Keys(respMap)
	slices.SortFunc(dates, func(a, b time.Time) bool {
		return a.Before(b)
	})

	var resp codersdk.DeploymentDAUsResponse
	for _, date := range fillEmptyDays(dates) {
		resp.Entries = append(resp.Entries, codersdk.DAUEntry{
			Date:   date,
			Amount: len(respMap[date]),
		})
	}

	return resp
}

func countUniqueUsers(rows []database.GetTemplateDAUsRow) int {
	seen := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		seen[row.UserID] = struct{}{}
	}
	return len(seen)
}

func (c *Cache) refresh(ctx context.Context) error {
	//nolint:gocritic // This is a system service.
	ctx = dbauthz.AsSystemRestricted(ctx)
	err := c.database.DeleteOldWorkspaceAgentStats(ctx)
	if err != nil {
		return xerrors.Errorf("delete old stats: %w", err)
	}

	templates, err := c.database.GetTemplates(ctx)
	if err != nil {
		return err
	}

	var (
		deploymentDAUs            = codersdk.DeploymentDAUsResponse{}
		templateDAUs              = make(map[uuid.UUID]codersdk.TemplateDAUsResponse, len(templates))
		templateUniqueUsers       = make(map[uuid.UUID]int)
		templateAverageBuildTimes = make(map[uuid.UUID]database.GetTemplateAverageBuildTimeRow)
	)

	rows, err := c.database.GetDeploymentDAUs(ctx)
	if err != nil {
		return err
	}
	deploymentDAUs = convertDeploymentDAUResponse(rows)
	c.deploymentDAUResponses.Store(&deploymentDAUs)

	for _, template := range templates {
		rows, err := c.database.GetTemplateDAUs(ctx, template.ID)
		if err != nil {
			return err
		}
		templateDAUs[template.ID] = convertDAUResponse(rows)
		templateUniqueUsers[template.ID] = countUniqueUsers(rows)

		templateAvgBuildTime, err := c.database.GetTemplateAverageBuildTime(ctx, database.GetTemplateAverageBuildTimeParams{
			TemplateID: uuid.NullUUID{
				UUID:  template.ID,
				Valid: true,
			},
			StartTime: sql.NullTime{
				Time:  database.Time(time.Now().AddDate(0, -30, 0)),
				Valid: true,
			},
		})
		if err != nil {
			return err
		}
		templateAverageBuildTimes[template.ID] = templateAvgBuildTime
	}
	c.templateDAUResponses.Store(&templateDAUs)
	c.templateUniqueUsers.Store(&templateUniqueUsers)
	c.templateAverageBuildTime.Store(&templateAverageBuildTimes)

	return nil
}

func (c *Cache) run(ctx context.Context) {
	defer close(c.done)

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
		case <-c.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (c *Cache) Close() error {
	c.cancel()
	<-c.done
	return nil
}

func (c *Cache) DeploymentDAUs() (*codersdk.DeploymentDAUsResponse, bool) {
	m := c.deploymentDAUResponses.Load()
	return m, m != nil
}

// TemplateDAUs returns an empty response if the template doesn't have users
// or is loading for the first time.
func (c *Cache) TemplateDAUs(id uuid.UUID) (*codersdk.TemplateDAUsResponse, bool) {
	m := c.templateDAUResponses.Load()
	if m == nil {
		// Data loading.
		return nil, false
	}

	resp, ok := (*m)[id]
	if !ok {
		// Probably no data.
		return nil, false
	}
	return &resp, true
}

// TemplateUniqueUsers returns the number of unique Template users
// from all Cache data.
func (c *Cache) TemplateUniqueUsers(id uuid.UUID) (int, bool) {
	m := c.templateUniqueUsers.Load()
	if m == nil {
		// Data loading.
		return -1, false
	}

	resp, ok := (*m)[id]
	if !ok {
		// Probably no data.
		return -1, false
	}
	return resp, true
}

func (c *Cache) TemplateBuildTimeStats(id uuid.UUID) codersdk.TemplateBuildTimeStats {
	unknown := codersdk.TemplateBuildTimeStats{
		codersdk.WorkspaceTransitionStart:  {},
		codersdk.WorkspaceTransitionStop:   {},
		codersdk.WorkspaceTransitionDelete: {},
	}

	m := c.templateAverageBuildTime.Load()
	if m == nil {
		// Data loading.
		return unknown
	}

	resp, ok := (*m)[id]
	if !ok {
		// No data or not enough builds.
		return unknown
	}

	convertMillis := func(m float64) *int64 {
		if m <= 0 {
			return nil
		}
		i := int64(m * 1000)
		return &i
	}

	return codersdk.TemplateBuildTimeStats{
		codersdk.WorkspaceTransitionStart: {
			P50: convertMillis(resp.Start50),
			P95: convertMillis(resp.Start95),
		},
		codersdk.WorkspaceTransitionStop: {
			P50: convertMillis(resp.Stop50),
			P95: convertMillis(resp.Stop95),
		},
		codersdk.WorkspaceTransitionDelete: {
			P50: convertMillis(resp.Delete50),
			P95: convertMillis(resp.Delete95),
		},
	}
}
