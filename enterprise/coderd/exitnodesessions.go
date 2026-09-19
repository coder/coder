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

const (
	exitNodeReplicaReaperInterval = 5 * time.Second
	// exitNodeStoppedTombstoneTTL bounds how long a deregistered replica ID
	// is remembered in memory. It only needs to outlive coordinate requests
	// that were in flight during deregistration; the database row rejects
	// later registrations.
	exitNodeStoppedTombstoneTTL = time.Minute
)

type exitNodeReplicaSession struct {
	cancel context.CancelFunc
}

type exitNodeReplicaSessionRegistry struct {
	clock    quartz.Clock
	mu       sync.Mutex
	sessions map[uuid.UUID]*exitNodeReplicaSession
	stopped  map[uuid.UUID]time.Time
	live     map[uuid.UUID]uuid.UUID
	// heartbeats counts markLive calls per replica so the reaper can detect a
	// heartbeat that landed after its snapshot.
	heartbeats map[uuid.UUID]uint64
}

func newExitNodeReplicaSessionRegistry(clock quartz.Clock) *exitNodeReplicaSessionRegistry {
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &exitNodeReplicaSessionRegistry{
		clock:      clock,
		sessions:   make(map[uuid.UUID]*exitNodeReplicaSession),
		stopped:    make(map[uuid.UUID]time.Time),
		live:       make(map[uuid.UUID]uuid.UUID),
		heartbeats: make(map[uuid.UUID]uint64),
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
	now := r.clock.Now("exit_node_session_stop")
	r.mu.Lock()
	for id, stoppedAt := range r.stopped {
		if now.Sub(stoppedAt) > exitNodeStoppedTombstoneTTL {
			delete(r.stopped, id)
		}
	}
	r.stopped[replicaID] = now
	session := r.sessions[replicaID]
	delete(r.sessions, replicaID)
	delete(r.live, replicaID)
	delete(r.heartbeats, replicaID)
	r.mu.Unlock()
	if session != nil {
		session.cancel()
	}
}

// cancelUnlessHeartbeat cancels the replica's session unless a heartbeat was
// recorded after generation, in which case the replica is kept live.
func (r *exitNodeReplicaSessionRegistry) cancelUnlessHeartbeat(replicaID, exitNodeID uuid.UUID, generation uint64) bool {
	r.mu.Lock()
	if r.heartbeats[replicaID] != generation {
		r.live[replicaID] = exitNodeID
		r.mu.Unlock()
		return false
	}
	session := r.sessions[replicaID]
	delete(r.sessions, replicaID)
	r.mu.Unlock()
	if session != nil {
		session.cancel()
	}
	return true
}

func (r *exitNodeReplicaSessionRegistry) markLive(replicaID, exitNodeID uuid.UUID) {
	r.mu.Lock()
	r.live[replicaID] = exitNodeID
	r.heartbeats[replicaID]++
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
	type staleReplica struct {
		replicaID  uuid.UUID
		exitNodeID uuid.UUID
		generation uint64
	}
	var stale []staleReplica
	for replicaID, exitNodeID := range previous {
		if _, ok := current[replicaID]; !ok {
			stale = append(stale, staleReplica{
				replicaID:  replicaID,
				exitNodeID: exitNodeID,
				generation: api.exitNodeReplicaSessions.heartbeats[replicaID],
			})
		}
	}
	api.exitNodeReplicaSessions.mu.Unlock()

	for _, replica := range stale {
		// A heartbeat handled by another coderd replica may have landed after
		// the live-set query. Recheck the row before canceling; a heartbeat
		// handled by this process is caught by the generation check below.
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
		if !api.exitNodeReplicaSessions.cancelUnlessHeartbeat(replica.replicaID, replica.exitNodeID, replica.generation) {
			continue
		}
		if err := api.Pubsub.Publish(codersdk.ExitNodeReplicasPubsubChannel, []byte(replica.exitNodeID.String())); err != nil {
			api.Logger.Error(api.ctx, "failed to publish stale exit node replica", slog.Error(err))
		}
	}
}
