package monitoring

import (
	"context"
	"database/sql"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
)

// Collector implements prometheus.Collector and collects statistics from the
// provided database.
type Collector struct {
	ctx                context.Context
	db                 database.Store
	users              *prometheus.Desc
	workspaces         *prometheus.Desc
	workspaceResources *prometheus.Desc
}

func NewCollector(ctx context.Context, db database.Store) *Collector {
	return &Collector{
		ctx: ctx,
		db:  db,
		users: prometheus.NewDesc(
			"coder_users",
			"The users in a Coder deployment.",
			nil,
			nil,
		),
		workspaces: prometheus.NewDesc(
			"coder_workspaces",
			"The workspaces in a Coder deployment.",
			nil,
			nil,
		),
		workspaceResources: prometheus.NewDesc(
			"coder_workspace_resources",
			"The workspace resources in a Coder deployment.",
			[]string{
				"workspace_resource_type",
			},
			nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.users
	ch <- c.workspaces
	ch <- c.workspaceResources
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		dbUsers, err := c.db.GetUsers(c.ctx, database.GetUsersParams{})
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			ch <- prometheus.NewInvalidMetric(c.users, err)
			return
		}

		ch <- prometheus.MustNewConstMetric(
			c.users,
			prometheus.GaugeValue,
			float64(len(dbUsers)),
		)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		dbWorkspaces, err := c.db.GetWorkspaces(c.ctx, false)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			ch <- prometheus.NewInvalidMetric(c.workspaces, err)
			return
		}

		ch <- prometheus.MustNewConstMetric(
			c.workspaces,
			prometheus.GaugeValue,
			float64(len(dbWorkspaces)),
		)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		dbWorkspaceResources, err := c.db.GetWorkspaceResources(c.ctx)
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			ch <- prometheus.NewInvalidMetric(c.workspaceResources, err)
			return
		}

		resourcesByType := map[string][]database.WorkspaceResource{}
		for _, dbwr := range dbWorkspaceResources {
			resourcesByType[dbwr.Type] = append(resourcesByType[dbwr.Type], dbwr)
		}

		for resourceType, resources := range resourcesByType {
			ch <- prometheus.MustNewConstMetric(
				c.workspaceResources,
				prometheus.GaugeValue,
				float64(len(resources)),
				resourceType,
			)
		}
	}()

	wg.Wait()
}
