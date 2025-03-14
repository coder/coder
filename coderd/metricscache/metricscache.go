package metricscache
import (
	"fmt"
	"errors"
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"
	"github.com/google/uuid"
	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
	"github.com/coder/retry"
)
// Cache holds the template metrics.
// The aggregation queries responsible for these values can take up to a minute
// on large deployments. Even in small deployments, aggregation queries can
// take a few hundred milliseconds, which would ruin page load times and
// database performance if in the hot path.
type Cache struct {
	database  database.Store
	log       slog.Logger
	clock     quartz.Clock
	intervals Intervals
	templateWorkspaceOwners  atomic.Pointer[map[uuid.UUID]int]
	templateAverageBuildTime atomic.Pointer[map[uuid.UUID]database.GetTemplateAverageBuildTimeRow]
	deploymentStatsResponse  atomic.Pointer[codersdk.DeploymentStats]
	done   chan struct{}
	cancel func()
	// usage is a experiment flag to enable new workspace usage tracking behavior and will be
	// removed when the experiment is complete.
	usage bool
}
type Intervals struct {
	TemplateBuildTimes time.Duration
	DeploymentStats    time.Duration
}
func New(db database.Store, log slog.Logger, clock quartz.Clock, intervals Intervals, usage bool) *Cache {
	if intervals.TemplateBuildTimes <= 0 {
		intervals.TemplateBuildTimes = time.Hour
	}
	if intervals.DeploymentStats <= 0 {
		intervals.DeploymentStats = time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Cache{
		clock:     clock,
		database:  db,
		intervals: intervals,
		log:       log,
		done:      make(chan struct{}),
		cancel:    cancel,
		usage:     usage,
	}
	go func() {
		var wg sync.WaitGroup
		defer close(c.done)
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.run(ctx, "template build times", intervals.TemplateBuildTimes, c.refreshTemplateBuildTimes)
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
func (c *Cache) refreshTemplateBuildTimes(ctx context.Context) error {
	//nolint:gocritic // This is a system service.
	ctx = dbauthz.AsSystemRestricted(ctx)
	templates, err := c.database.GetTemplates(ctx)
	if err != nil {
		return err
	}
	var (
		templateWorkspaceOwners   = make(map[uuid.UUID]int)
		templateAverageBuildTimes = make(map[uuid.UUID]database.GetTemplateAverageBuildTimeRow)
	)
	ids := make([]uuid.UUID, 0, len(templates))
	for _, template := range templates {
		ids = append(ids, template.ID)
		templateAvgBuildTime, err := c.database.GetTemplateAverageBuildTime(ctx, database.GetTemplateAverageBuildTimeParams{
			TemplateID: uuid.NullUUID{
				UUID:  template.ID,
				Valid: true,
			},
			StartTime: sql.NullTime{
				Time:  dbtime.Time(c.clock.Now().AddDate(0, 0, -30)),
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
		return fmt.Errorf("get workspace unique owner count by template ids: %w", err)
	}
	for _, owner := range owners {
		templateWorkspaceOwners[owner.TemplateID] = int(owner.UniqueOwnersSum)
	}
	c.templateWorkspaceOwners.Store(&templateWorkspaceOwners)
	c.templateAverageBuildTime.Store(&templateAverageBuildTimes)
	return nil
}
func (c *Cache) refreshDeploymentStats(ctx context.Context) error {
	var (
		from       = c.clock.Now().Add(-15 * time.Minute)
		agentStats database.GetDeploymentWorkspaceAgentStatsRow
		err        error
	)
	if c.usage {
		agentUsageStats, err := c.database.GetDeploymentWorkspaceAgentUsageStats(ctx, from)
		if err != nil {
			return err
		}
		agentStats = database.GetDeploymentWorkspaceAgentStatsRow(agentUsageStats)
	} else {
		agentStats, err = c.database.GetDeploymentWorkspaceAgentStats(ctx, from)
		if err != nil {
			return err
		}
	}
	workspaceStats, err := c.database.GetDeploymentWorkspaceStats(ctx)
	if err != nil {
		return err
	}
	c.deploymentStatsResponse.Store(&codersdk.DeploymentStats{
		AggregatedFrom: from,
		CollectedAt:    dbtime.Time(c.clock.Now()),
		NextUpdateAt:   dbtime.Time(c.clock.Now().Add(c.intervals.DeploymentStats)),
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
	logger := c.log.With(slog.F("name", name), slog.F("interval", interval))
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
				if errors.Is(err, sql.ErrNoRows) {
					break
				}
				logger.Error(ctx, "refresh metrics failed", slog.Error(err))
				continue
			}
			logger.Debug(ctx, "metrics refreshed", slog.F("took", time.Since(start)))
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
