package coderd

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const exitNodeReplicaReaperInterval = 5 * time.Second

type exitNodeReplicaSession struct {
	cancel context.CancelFunc
}

type exitNodeReplicaSessionRegistry struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]*exitNodeReplicaSession
	stopped  map[uuid.UUID]struct{}
	live     map[uuid.UUID]uuid.UUID
}

func newExitNodeReplicaSessionRegistry() *exitNodeReplicaSessionRegistry {
	return &exitNodeReplicaSessionRegistry{
		sessions: make(map[uuid.UUID]*exitNodeReplicaSession),
		stopped:  make(map[uuid.UUID]struct{}),
		live:     make(map[uuid.UUID]uuid.UUID),
	}
}

func (r *exitNodeReplicaSessionRegistry) register(replicaID uuid.UUID, cancel context.CancelFunc) func() {
	session := &exitNodeReplicaSession{cancel: cancel}
	r.mu.Lock()
	if _, stopped := r.stopped[replicaID]; stopped {
		r.mu.Unlock()
		cancel()
		return func() {}
	}
	if previous := r.sessions[replicaID]; previous != nil {
		previous.cancel()
	}
	r.sessions[replicaID] = session
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		if r.sessions[replicaID] == session {
			delete(r.sessions, replicaID)
		}
		r.mu.Unlock()
	}
}

func (r *exitNodeReplicaSessionRegistry) stop(replicaID uuid.UUID) {
	r.mu.Lock()
	r.stopped[replicaID] = struct{}{}
	session := r.sessions[replicaID]
	delete(r.sessions, replicaID)
	delete(r.live, replicaID)
	r.mu.Unlock()
	if session != nil {
		session.cancel()
	}
}

func (r *exitNodeReplicaSessionRegistry) cancel(replicaID uuid.UUID) {
	r.mu.Lock()
	session := r.sessions[replicaID]
	delete(r.sessions, replicaID)
	r.mu.Unlock()
	if session != nil {
		session.cancel()
	}
}

func (r *exitNodeReplicaSessionRegistry) markLive(replicaID, exitNodeID uuid.UUID) {
	r.mu.Lock()
	r.live[replicaID] = exitNodeID
	r.mu.Unlock()
}

func (api *API) startExitNodeReplicaReaper(clock quartz.Clock) {
	if clock == nil {
		clock = quartz.NewReal()
	}
	go func() {
		ticker := clock.NewTicker(exitNodeReplicaReaperInterval, "exit_node_replica_reaper")
		defer ticker.Stop()
		for {
			select {
			case <-api.ctx.Done():
				return
			case <-ticker.C:
				api.reapExitNodeReplicas(clock.Now("exit_node_replica_reaper"))
			}
		}
	}()
}

func (api *API) reapExitNodeReplicas(now time.Time) {
	// The reaper spans organizations and only reads replica heartbeat state.
	//nolint:gocritic // Deployment-wide background maintenance requires system access.
	replicas, err := api.Database.GetAllLiveExitNodeReplicas(
		dbauthz.AsSystemRestricted(api.ctx),
		now.Add(-codersdk.ExitNodeReplicaStaleAfter),
	)
	if err != nil {
		api.Logger.Error(api.ctx, "failed to find live exit node replicas", slog.Error(err))
		return
	}
	current := make(map[uuid.UUID]uuid.UUID, len(replicas))
	for _, replica := range replicas {
		current[replica.ID] = replica.ExitNodeID
	}

	api.exitNodeReplicaSessions.mu.Lock()
	previous := api.exitNodeReplicaSessions.live
	api.exitNodeReplicaSessions.live = current
	var stale []struct {
		replicaID  uuid.UUID
		exitNodeID uuid.UUID
	}
	for replicaID, exitNodeID := range previous {
		if _, ok := current[replicaID]; !ok {
			stale = append(stale, struct {
				replicaID  uuid.UUID
				exitNodeID uuid.UUID
			}{replicaID: replicaID, exitNodeID: exitNodeID})
		}
	}
	api.exitNodeReplicaSessions.mu.Unlock()

	for _, replica := range stale {
		// A heartbeat may have landed after the live-set query. Recheck before
		// canceling so the reaper cannot revoke a newly refreshed session.
		row, err := api.Database.GetExitNodeReplicaByID(dbauthz.AsSystemRestricted(api.ctx), replica.replicaID) //nolint:gocritic // Deployment-wide reaper lookup.
		if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			api.Logger.Error(api.ctx, "failed to recheck exit node replica", slog.Error(err))
			api.exitNodeReplicaSessions.markLive(replica.replicaID, replica.exitNodeID)
			continue
		}
		if err == nil && !row.StoppedAt.Valid && row.UpdatedAt.After(now.Add(-codersdk.ExitNodeReplicaStaleAfter)) {
			api.exitNodeReplicaSessions.markLive(row.ID, row.ExitNodeID)
			continue
		}
		api.exitNodeReplicaSessions.cancel(replica.replicaID)
		if err := api.Pubsub.Publish(codersdk.ExitNodeReplicasPubsubChannel, []byte(replica.exitNodeID.String())); err != nil {
			api.Logger.Error(api.ctx, "failed to publish stale exit node replica", slog.Error(err))
		}
	}
}
