package prometheusmetrics

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
)

// ActiveUsers tracks the number of users that have authenticated within the past hour.
func ActiveUsers(ctx context.Context, registerer prometheus.Registerer, db database.Store, duration time.Duration) (context.CancelFunc, error) {
	if duration == 0 {
		duration = 5 * time.Minute
	}

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "active_users_duration_hour",
		Help:      "The number of users that have been active within the last hour.",
	})
	err := registerer.Register(gauge)
	if err != nil {
		return nil, err
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	ticker := time.NewTicker(duration)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			apiKeys, err := db.GetAPIKeysLastUsedAfter(ctx, database.Now().Add(-1*time.Hour))
			if err != nil {
				continue
			}
			distinctUsers := map[uuid.UUID]struct{}{}
			for _, apiKey := range apiKeys {
				distinctUsers[apiKey.UserID] = struct{}{}
			}
			gauge.Set(float64(len(distinctUsers)))
		}
	}()
	return cancelFunc, nil
}

// Workspaces tracks the total number of workspaces with labels on status.
func Workspaces(ctx context.Context, registerer prometheus.Registerer, db database.Store, duration time.Duration) (context.CancelFunc, error) {
	if duration == 0 {
		duration = 5 * time.Minute
	}

	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "workspace_latest_build_total",
		Help:      "The latest workspace builds with a status.",
	}, []string{"status"})
	err := registerer.Register(gauge)
	if err != nil {
		return nil, err
	}
	// This exists so the prometheus metric exports immediately when set.
	// It helps with tests so they don't have to wait for a tick.
	gauge.WithLabelValues("pending").Set(0)

	ctx, cancelFunc := context.WithCancel(ctx)
	ticker := time.NewTicker(duration)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			builds, err := db.GetLatestWorkspaceBuilds(ctx)
			if err != nil {
				continue
			}
			jobIDs := make([]uuid.UUID, 0, len(builds))
			for _, build := range builds {
				jobIDs = append(jobIDs, build.JobID)
			}
			jobs, err := db.GetProvisionerJobsByIDs(ctx, jobIDs)
			if err != nil {
				continue
			}

			gauge.Reset()
			for _, job := range jobs {
				status := coderd.ConvertProvisionerJobStatus(job)
				gauge.WithLabelValues(string(status)).Add(1)
			}
		}
	}()
	return cancelFunc, nil
}
