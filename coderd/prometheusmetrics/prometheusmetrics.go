package prometheusmetrics

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/tailnet"
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

// Agents tracks the total number of workspaces with labels on status.
func Agents(ctx context.Context, registerer prometheus.Registerer, db database.Store, coordinator *atomic.Pointer[tailnet.Coordinator], derpMap *tailcfg.DERPMap, duration time.Duration) (context.CancelFunc, error) {
	if duration == 0 {
		duration = 15 * time.Second // TODO 5 * time.Minute
	}

	agentsConnectionGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "agents",
		Name:      "connection",
		Help:      "The agent connection with a status.",
	}, []string{"agent_name", "workspace_name", "status"})
	err := registerer.Register(agentsConnectionGauge)
	if err != nil {
		return nil, err
	}

	agentsUserLatenciesGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "agents",
		Name:      "user_latencies_seconds",
		Help:      "The user's agent latency in seconds.",
	}, []string{"agent_id", "workspace_name", "derp_region", "preferred"})
	err = registerer.Register(agentsUserLatenciesGauge)
	if err != nil {
		return nil, err
	}

	// FIXME connection_type ide

	ctx, cancelFunc := context.WithCancel(ctx)
	ticker := time.NewTicker(duration)
	go func() {
		defer ticker.Stop()
		for {
			log.Println("Agents!!!")

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			// FIXME Optimize this routine: SQL db calls

			builds, err := db.GetLatestWorkspaceBuilds(ctx)
			if err != nil {
				log.Println("1", err)
				continue
			}

			agentsConnectionGauge.Reset()
			agentsUserLatenciesGauge.Reset()
			for _, build := range builds {
				workspace, err := db.GetWorkspaceByID(ctx, build.WorkspaceID)
				if err != nil {
					log.Println("2", err)
					continue
				}

				agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, build.WorkspaceID)
				if err != nil {
					log.Println("3", err)
					continue
				}

				if len(agents) == 0 {
					continue
				}

				// FIXME publish workspace even if no agents

				for _, agent := range agents {
					connectionStatus := agent.Status(6 * time.Second)

					// FIXME AgentInactiveDisconnectTimeout
					log.Println("with value " + agent.Name)
					agentsConnectionGauge.WithLabelValues(agent.Name, workspace.Name, string(connectionStatus.Status)).Set(1)

					node := (*coordinator.Load()).Node(agent.ID)
					if node != nil {
						log.Println("coordinator")

						for rawRegion, latency := range node.DERPLatency {
							log.Println(rawRegion, latency)

							regionParts := strings.SplitN(rawRegion, "-", 2)
							regionID, err := strconv.Atoi(regionParts[0])
							if err != nil {
								continue // xerrors.Errorf("convert derp region id %q: %w", rawRegion, err)
							}
							region, found := derpMap.Regions[regionID]
							if !found {
								// It's possible that a workspace agent is using an old DERPMap
								// and reports regions that do not exist. If that's the case,
								// report the region as unknown!
								region = &tailcfg.DERPRegion{
									RegionID:   regionID,
									RegionName: fmt.Sprintf("Unnamed %d", regionID),
								}
							}

							log.Println(region, latency)
							agentsUserLatenciesGauge.WithLabelValues(agent.Name, workspace.Name, region.RegionName, fmt.Sprintf("%v", node.PreferredDERP == regionID)).Set(latency)
						}
					} else {
						log.Println("node is null")
					}

					// FIXME publish agent even if DERP is missing
					// FIXME IDE?
				}
			}
		}
	}()
	return cancelFunc, nil
}
