package prometheusmetrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
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
func Agents(ctx context.Context, logger slog.Logger, registerer prometheus.Registerer, db database.Store, coordinator *atomic.Pointer[tailnet.Coordinator], derpMap *tailcfg.DERPMap, agentInactiveDisconnectTimeout, duration time.Duration) (context.CancelFunc, error) {
	if duration == 0 {
		duration = 15 * time.Second // TODO 5 * time.Minute
	}

	workspaceAgentsGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "agents",
		Name:      "up",
		Help:      "The number of active agents per workspace.",
	}, []string{"username", "workspace_name"})
	err := registerer.Register(workspaceAgentsGauge)
	if err != nil {
		return nil, err
	}

	agentsConnectionGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "agents",
		Name:      "connections",
		Help:      "Agent connections with statuses.",
	}, []string{"agent_name", "username", "workspace_name", "status", "lifecycle_state", "tailnet_node"})
	err = registerer.Register(agentsConnectionGauge)
	if err != nil {
		return nil, err
	}

	agentsConnectionLatenciesGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "agents",
		Name:      "connection_latencies_seconds",
		Help:      "Agent connection latencies in seconds.",
	}, []string{"agent_id", "username", "workspace_name", "derp_region", "preferred"})
	err = registerer.Register(agentsConnectionLatenciesGauge)
	if err != nil {
		return nil, err
	}

	// nolint:gocritic // Prometheus must collect metrics for all Coder users.
	ctx, cancelFunc := context.WithCancel(dbauthz.AsSystemRestricted(ctx))
	ticker := time.NewTicker(duration)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			logger.Info(ctx, "Collect agent metrics now")

			workspaceRows, err := db.GetWorkspaces(ctx, database.GetWorkspacesParams{
				AgentInactiveDisconnectTimeoutSeconds: int64(agentInactiveDisconnectTimeout.Seconds()),
			})
			if err != nil {
				logger.Error(ctx, "can't get workspace rows", slog.Error(err))
				continue
			}

			workspaceAgentsGauge.Reset()
			agentsConnectionGauge.Reset()
			agentsConnectionLatenciesGauge.Reset()

			for _, workspace := range workspaceRows {
				user, err := db.GetUserByID(ctx, workspace.OwnerID)
				if err != nil {
					logger.Error(ctx, "can't get user", slog.Error(err), slog.F("user_id", workspace.OwnerID))
					workspaceAgentsGauge.WithLabelValues(user.Username, workspace.Name).Add(0)
					continue
				}

				agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(ctx, workspace.ID)
				if err != nil {
					logger.Error(ctx, "can't get workspace agents", slog.F("workspace_name", workspace.Name), slog.Error(err))
					workspaceAgentsGauge.WithLabelValues(user.Username, workspace.Name).Add(0)
					continue
				}

				if len(agents) == 0 {
					logger.Info(ctx, "workspace agents are unavailable", slog.F("workspace_name", workspace.Name))
					workspaceAgentsGauge.WithLabelValues(user.Username, workspace.Name).Add(0)
					continue
				}

				for _, agent := range agents {
					// Collect information about agents
					workspaceAgentsGauge.WithLabelValues(user.Username, workspace.Name).Add(1)

					connectionStatus := agent.Status(agentInactiveDisconnectTimeout)
					node := (*coordinator.Load()).Node(agent.ID)

					tailnetNode := "unknown"
					if node != nil {
						tailnetNode = node.ID.String()
					}

					agentsConnectionGauge.WithLabelValues(agent.Name, user.Username, workspace.Name, string(connectionStatus.Status), string(agent.LifecycleState), tailnetNode).Set(1)

					if node == nil {
						logger.Info(ctx, "can't read in-memory node for agent", slog.F("workspace_name", workspace.Name), slog.F("agent_name", agent.Name))
						continue
					}

					// Collect information about connection latencies
					for rawRegion, latency := range node.DERPLatency {
						regionParts := strings.SplitN(rawRegion, "-", 2)
						regionID, err := strconv.Atoi(regionParts[0])
						if err != nil {
							logger.Error(ctx, "can't convert DERP region", slog.Error(err), slog.F("agent_name", agent.Name), slog.F("raw_region", rawRegion))
							continue
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

						agentsConnectionLatenciesGauge.WithLabelValues(agent.Name, user.Username, workspace.Name, region.RegionName, fmt.Sprintf("%v", node.PreferredDERP == regionID)).Set(latency)
					}

					// FIXME IDE?
					// FIXME connection_type ide
				}
			}
		}
	}()
	return cancelFunc, nil
}
