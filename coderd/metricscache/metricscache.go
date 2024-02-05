package metricscache

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

func OnlyDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// deploymentTimezoneOffsets are the timezones that are cached and supported.
// Any non-listed timezone offsets will need to use the closest supported one.
var deploymentTimezoneOffsets = []int{
	0, // UTC - is listed first intentionally.
	// Shortened list of 4 timezones that should encompass *most* users. Caching
	// all 25 timezones can be too computationally expensive for large
	// deployments. This is a stop-gap until more robust fixes can be made for
	// the deployment DAUs query.
	-6, 3, 6, 10,

	// -12, -11, -10, -9, -8, -7, -6, -5, -4, -3, -2, -1,
	// 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12,
}

// templateTimezoneOffsets are the timezones each template will use for it's DAU
// calculations. This is expensive as each template needs to do each timezone, so keep this list
// very small.
var templateTimezoneOffsets = []int{
	// Only do one for now. If people request more accurate template DAU, we can
	// fix this. But it adds too much cost, so optimization is needed first.
	0, // UTC - is listed first intentionally.
}

// Cache holds the template metrics.
// The aggregation queries responsible for these values can take up to a minute
// on large deployments. Even in small deployments, aggregation queries can
// take a few hundred milliseconds, which would ruin page load times and
// database performance if in the hot path.
type Cache struct {
	database  database.Store
	log       slog.Logger
	intervals Intervals

	deploymentDAUResponses   atomic.Pointer[map[int]codersdk.DAUsResponse]
	templateDAUResponses     atomic.Pointer[map[int]map[uuid.UUID]codersdk.DAUsResponse]
	templateUniqueUsers      atomic.Pointer[map[uuid.UUID]int]
	templateWorkspaceOwners  atomic.Pointer[map[uuid.UUID]int]
	templateAverageBuildTime atomic.Pointer[map[uuid.UUID]database.GetTemplateAverageBuildTimeRow]
	deploymentStatsResponse  atomic.Pointer[codersdk.DeploymentStats]

	done   chan struct{}
	cancel func()
}

type Intervals struct {
	TemplateDAUs    time.Duration
	DeploymentStats time.Duration
}

func New(db database.Store, log slog.Logger, intervals Intervals) *Cache {
	if intervals.TemplateDAUs <= 0 {
		intervals.TemplateDAUs = time.Hour
	}
	if intervals.DeploymentStats <= 0 {
		intervals.DeploymentStats = time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())

	c := &Cache{
		database:  db,
		intervals: intervals,
		log:       log,
		done:      make(chan struct{}),
		cancel:    cancel,
	}
	go func() {
		var wg sync.WaitGroup
		defer close(c.done)
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.run(ctx, "template daus", intervals.TemplateDAUs, c.refreshTemplateDAUs)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.run(ctx, "deployment stats", intervals.DeploymentStats, c.refreshDeploymentStats)
		}()
		wg.Wait()
	}()
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

type dauRow interface {
	database.GetTemplateDAUsRow |
		database.GetDeploymentDAUsRow
}

func convertDAUResponse[T dauRow](rows []T, tzOffset int) codersdk.DAUsResponse {
	respMap := make(map[time.Time][]uuid.UUID)
	for _, row := range rows {
		switch row := any(row).(type) {
		case database.GetDeploymentDAUsRow:
			respMap[row.Date] = append(respMap[row.Date], row.UserID)
		case database.GetTemplateDAUsRow:
			respMap[row.Date] = append(respMap[row.Date], row.UserID)
		default:
			// This should never happen.
			panic(fmt.Sprintf("%T not acceptable, developer error", row))
		}
	}

	dates := maps.Keys(respMap)
	slices.SortFunc(dates, func(a, b time.Time) int {
		if a.Before(b) {
			return -1
		} else if a.Equal(b) {
			return 0
		}
		return 1
	})

	var resp codersdk.DAUsResponse
	for _, date := range fillEmptyDays(dates) {
		resp.Entries = append(resp.Entries, codersdk.DAUEntry{
			// This date is truncated to 00:00:00 of the given day, so only
			// return date information.
			Date:   OnlyDate(date),
			Amount: len(respMap[date]),
		})
	}
	resp.TZHourOffset = tzOffset

	return resp
}

func countUniqueUsers(rows []database.GetTemplateDAUsRow) int {
	seen := make(map[uuid.UUID]struct{}, len(rows))
	for _, row := range rows {
		seen[row.UserID] = struct{}{}
	}
	return len(seen)
}

func (c *Cache) refreshDeploymentDAUs(ctx context.Context) error {
	//nolint:gocritic // This is a system service.
	ctx = dbauthz.AsSystemRestricted(ctx)

	deploymentDAUs := make(map[int]codersdk.DAUsResponse)
	for _, tzOffset := range deploymentTimezoneOffsets {
		rows, err := c.database.GetDeploymentDAUs(ctx, int32(tzOffset))
		if err != nil {
			return err
		}
		deploymentDAUs[tzOffset] = convertDAUResponse(rows, tzOffset)
	}

	c.deploymentDAUResponses.Store(&deploymentDAUs)
	return nil
}

func (c *Cache) refreshTemplateDAUs(ctx context.Context) error {
	//nolint:gocritic // This is a system service.
	ctx = dbauthz.AsSystemRestricted(ctx)

	templates, err := c.database.GetTemplates(ctx)
	if err != nil {
		return err
	}

	var (
		templateDAUs              = make(map[int]map[uuid.UUID]codersdk.DAUsResponse, len(templates))
		templateUniqueUsers       = make(map[uuid.UUID]int)
		templateWorkspaceOwners   = make(map[uuid.UUID]int)
		templateAverageBuildTimes = make(map[uuid.UUID]database.GetTemplateAverageBuildTimeRow)
	)

	err = c.refreshDeploymentDAUs(ctx)
	if err != nil {
		return xerrors.Errorf("deployment daus: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(templates))
	for _, template := range templates {
		ids = append(ids, template.ID)
		for _, tzOffset := range templateTimezoneOffsets {
			rows, err := c.database.GetTemplateDAUs(ctx, database.GetTemplateDAUsParams{
				TemplateID: template.ID,
				TzOffset:   int32(tzOffset),
			})
			if err != nil {
				return err
			}
			if templateDAUs[tzOffset] == nil {
				templateDAUs[tzOffset] = make(map[uuid.UUID]codersdk.DAUsResponse)
			}
			templateDAUs[tzOffset][template.ID] = convertDAUResponse(rows, tzOffset)
			if _, set := templateUniqueUsers[template.ID]; !set {
				// If the uniqueUsers has not been counted yet, set the unique count with the rows we have.
				// We only need to calculate this once.
				templateUniqueUsers[template.ID] = countUniqueUsers(rows)
			}
		}

		templateAvgBuildTime, err := c.database.GetTemplateAverageBuildTime(ctx, database.GetTemplateAverageBuildTimeParams{
			TemplateID: uuid.NullUUID{
				UUID:  template.ID,
				Valid: true,
			},
			StartTime: sql.NullTime{
				Time:  dbtime.Time(time.Now().AddDate(0, -30, 0)),
				Valid: true,
			},
		})
		if err != nil {
			return err
		}
		templateAverageBuildTimes[template.ID] = templateAvgBuildTime
	}

	owners, err := c.database.GetWorkspaceUniqueOwnerCountByTemplateIDs(ctx, ids)
	if err != nil {
		return xerrors.Errorf("get workspace unique owner count by template ids: %w", err)
	}

	for _, owner := range owners {
		templateWorkspaceOwners[owner.TemplateID] = int(owner.UniqueOwnersSum)
	}

	c.templateWorkspaceOwners.Store(&templateWorkspaceOwners)
	c.templateDAUResponses.Store(&templateDAUs)
	c.templateUniqueUsers.Store(&templateUniqueUsers)
	c.templateAverageBuildTime.Store(&templateAverageBuildTimes)

	return nil
}

func (c *Cache) refreshDeploymentStats(ctx context.Context) error {
	from := dbtime.Now().Add(-15 * time.Minute)
	agentStats, err := c.database.GetDeploymentWorkspaceAgentStats(ctx, from)
	if err != nil {
		return err
	}
	workspaceStats, err := c.database.GetDeploymentWorkspaceStats(ctx)
	if err != nil {
		return err
	}
	c.deploymentStatsResponse.Store(&codersdk.DeploymentStats{
		AggregatedFrom: from,
		CollectedAt:    dbtime.Now(),
		NextUpdateAt:   dbtime.Now().Add(c.intervals.DeploymentStats),
		Workspaces: codersdk.WorkspaceDeploymentStats{
			Pending:  workspaceStats.PendingWorkspaces,
			Building: workspaceStats.BuildingWorkspaces,
			Running:  workspaceStats.RunningWorkspaces,
			Failed:   workspaceStats.FailedWorkspaces,
			Stopped:  workspaceStats.StoppedWorkspaces,
			ConnectionLatencyMS: codersdk.WorkspaceConnectionLatencyMS{
				P50: agentStats.WorkspaceConnectionLatency50,
				P95: agentStats.WorkspaceConnectionLatency95,
			},
			RxBytes: agentStats.WorkspaceRxBytes,
			TxBytes: agentStats.WorkspaceTxBytes,
		},
		SessionCount: codersdk.SessionCountDeploymentStats{
			VSCode:          agentStats.SessionCountVSCode,
			SSH:             agentStats.SessionCountSSH,
			JetBrains:       agentStats.SessionCountJetBrains,
			ReconnectingPTY: agentStats.SessionCountReconnectingPTY,
		},
	})
	return nil
}

func (c *Cache) run(ctx context.Context, name string, interval time.Duration, refresh func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		for r := retry.New(time.Millisecond*100, time.Minute); r.Wait(ctx); {
			start := time.Now()
			err := refresh(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.log.Error(ctx, "refresh", slog.Error(err))
				continue
			}
			c.log.Debug(
				ctx,
				name+" metrics refreshed",
				slog.F("took", time.Since(start)),
				slog.F("interval", interval),
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

func (c *Cache) DeploymentDAUs(offset int) (int, *codersdk.DAUsResponse, bool) {
	m := c.deploymentDAUResponses.Load()
	if m == nil {
		return 0, nil, false
	}
	closestOffset, resp, ok := closest(*m, offset)
	if !ok {
		return 0, nil, false
	}
	return closestOffset, &resp, ok
}

// TemplateDAUs returns an empty response if the template doesn't have users
// or is loading for the first time.
// The cache will select the closest DAUs response to given timezone offset.
func (c *Cache) TemplateDAUs(id uuid.UUID, offset int) (int, *codersdk.DAUsResponse, bool) {
	m := c.templateDAUResponses.Load()
	if m == nil {
		// Data loading.
		return 0, nil, false
	}

	closestOffset, resp, ok := closest(*m, offset)
	if !ok {
		// Probably no data.
		return 0, nil, false
	}

	tpl, ok := resp[id]
	if !ok {
		// Probably no data.
		return 0, nil, false
	}

	return closestOffset, &tpl, true
}

// closest returns the value in the values map that has a key with the value most
// close to the requested key. This is so if a user requests a timezone offset that
// we do not have, we return the closest one we do have to the user.
func closest[V any](values map[int]V, offset int) (int, V, bool) {
	if len(values) == 0 {
		var v V
		return -1, v, false
	}

	v, ok := values[offset]
	if ok {
		// We have the exact offset, that was easy!
		return offset, v, true
	}

	var closest int
	var closestV V
	diff := math.MaxInt
	for k, v := range values {
		newDiff := abs(k - offset)
		// Take the closest value that is also the smallest value. We do this
		// to make the output deterministic
		if newDiff < diff || (newDiff == diff && k < closest) {
			// new closest
			closest = k
			closestV = v
			diff = newDiff
		}
	}
	return closest, closestV, true
}

func abs(a int) int {
	if a < 0 {
		return -1 * a
	}
	return a
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

func (c *Cache) TemplateWorkspaceOwners(id uuid.UUID) (int, bool) {
	m := c.templateWorkspaceOwners.Load()
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

func (c *Cache) DeploymentStats() (codersdk.DeploymentStats, bool) {
	deploymentStats := c.deploymentStatsResponse.Load()
	if deploymentStats == nil {
		return codersdk.DeploymentStats{}, false
	}
	return *deploymentStats, true
}
